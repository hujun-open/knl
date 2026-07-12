#!/usr/bin/env bash
# setup-k3s.sh — install and configure a single-node k3s cluster for KNL.
#
# Exports KNL_* env vars consumed by install-knl.sh, then optionally chains
# into that script.
#
# Usage:
#   sudo ./setup-k3s.sh [--install] [--k3s-version VERSION] [--storage-class CLASS]
#
# Options:
#   --install              run install-knl.sh after cluster setup
#   --k3s-version VER      pin k3s version (e.g. v1.31.2+k3s1)
#   --storage-class SC     storage class for knl-pvc (default: local-path)
#   --registry-nodeport P  NodePort for local registry (default: 30500)
#   --registry-size SIZE   PVC size for registry storage (default: 5Gi)
#   --skip-registry        skip registry deployment
#   -h, --help             show help

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

K3S_VERSION=""
STORAGE_CLASS="local-path"
RUN_INSTALL=false
REGISTRY_NODEPORT=30500
REGISTRY_SIZE=5Gi
SKIP_REGISTRY=false

log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*"
}

die() {
    log "ERROR: $*" >&2
    exit 1
}

usage() {
    sed -n '2,17p' "$0" | sed 's/^# \{0,1\}//'
}

require_root() {
    [[ "${EUID:-$(id -u)}" -eq 0 ]] || die "this script must be run as root (use sudo)"
}

detect_ipv6_capable() {
    local iface
    iface=$(ip route 2>/dev/null | awk '/^default/ {print $5; exit}')
    if [[ -z "$iface" ]]; then
        log "no default route found; assuming IPv4-only k3s install"
        return 1
    fi
    if ip -6 addr show dev "$iface" scope global 2>/dev/null | grep -q "inet6"; then
        log "global unicast IPv6 found on ${iface}; will install k3s dual-stack"
        return 0
    else
        log "no global unicast IPv6 on ${iface}; will install k3s IPv4-only"
        return 1
    fi
}

install_k3s() {
    if command -v k3s >/dev/null 2>&1 && systemctl is-active --quiet k3s 2>/dev/null; then
        log "k3s is already installed and running; skipping install"
        return 0
    fi

    log "installing k3s..."
    local install_args=(--disable=traefik)

    if detect_ipv6_capable; then
        install_args+=(
            --cluster-cidr=10.42.0.0/16,fd42::/48
            --service-cidr=10.43.0.0/16,fd43::/112
        )
    else
        install_args+=(
            --cluster-cidr=10.42.0.0/16
            --service-cidr=10.43.0.0/16
        )
    fi

    if [[ -n "$K3S_VERSION" ]]; then
        export INSTALL_K3S_VERSION="$K3S_VERSION"
        log "  k3s version: ${K3S_VERSION}"
    fi

    export INSTALL_K3S_EXEC="${install_args[*]}"
    curl -sfL https://get.k3s.io | sh -

    log "waiting for k3s service..."
    systemctl enable k3s
    systemctl start k3s
}

wait_k3s_ready() {
    export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
    log "waiting for k3s node to become Ready..."
    local end=$((SECONDS + 300))
    while (( SECONDS < end )); do
        if kubectl get nodes -o jsonpath='{.items[0].status.conditions[?(@.type=="Ready")].status}' 2>/dev/null | grep -q True; then
            kubectl get nodes
            return 0
        fi
        sleep 3
    done
    die "k3s node did not become Ready within 300s"
}

configure_registry_insecure() {
    local registry_host="localhost:${REGISTRY_NODEPORT}"
    log "configuring k3s to use insecure local registry at ${registry_host}..."
    cat >/etc/rancher/k3s/registries.yaml <<EOF
mirrors:
  "${registry_host}":
    endpoint:
      - "http://${registry_host}"
configs:
  "${registry_host}":
    tls:
      insecure_skip_verify: true
EOF
    systemctl restart k3s
    wait_k3s_ready
}

deploy_registry() {
    export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
    log "deploying registry:2 (NodePort ${REGISTRY_NODEPORT}, PVC ${REGISTRY_SIZE})..."
    kubectl apply -f - <<EOF
apiVersion: v1
kind: Namespace
metadata:
  name: registry
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: registry-pvc
  namespace: registry
spec:
  accessModes:
    - ReadWriteOnce
  storageClassName: ${STORAGE_CLASS}
  resources:
    requests:
      storage: ${REGISTRY_SIZE}
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: registry
  namespace: registry
spec:
  replicas: 1
  selector:
    matchLabels:
      app: registry
  template:
    metadata:
      labels:
        app: registry
    spec:
      containers:
        - name: registry
          image: registry:2
          ports:
            - containerPort: 5000
          volumeMounts:
            - name: data
              mountPath: /var/lib/registry
      volumes:
        - name: data
          persistentVolumeClaim:
            claimName: registry-pvc
---
apiVersion: v1
kind: Service
metadata:
  name: registry
  namespace: registry
spec:
  type: NodePort
  selector:
    app: registry
  ports:
    - port: 5000
      targetPort: 5000
      nodePort: ${REGISTRY_NODEPORT}
EOF
    kubectl rollout status deployment/registry -n registry --timeout=120s
    log "registry is available at localhost:${REGISTRY_NODEPORT}"
}

wait_all_pods_ready() {
    export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
    local timeout="${1:-300}"
    log "waiting for all pods to become Ready (timeout ${timeout}s)..."
    local end=$((SECONDS + timeout))
    while (( SECONDS < end )); do
        local pods not_ready
        pods=$(kubectl get pods -A --no-headers 2>/dev/null || true)
        if [[ -z "$pods" ]]; then
            sleep 3
            continue
        fi
        not_ready=$(kubectl get pods -A -o jsonpath='{range .items[*]}{.status.phase}{"\t"}{range .status.conditions[?(@.type=="Ready")]}{.status}{end}{"\t"}{.metadata.namespace}{"/"}{.metadata.name}{"\n"}{end}' 2>/dev/null \
            | awk -F'\t' '$1 != "Succeeded" && $1 != "Failed" && ($1 != "Running" || $2 != "True") {print $3}')
        if [[ -z "$not_ready" ]]; then
            kubectl get pods -A
            log "all pods are Ready"
            return 0
        fi
        sleep 5
    done
    kubectl get pods -A
    die "not all pods became Ready within ${timeout}s"
}

enable_ipv6_forwarding() {
    log "enabling IPv6 forwarding (required by k8slan)..."
    sysctl -w net.ipv6.conf.all.forwarding=1
    cat >/etc/sysctl.d/99-knl.conf <<'EOF'
# KNL / k8slan: IPv6 multicast forwarding for VXLAN underlay
net.ipv6.conf.all.forwarding=1
EOF
    sysctl --system >/dev/null 2>&1 || true
}

install_helm() {
    if command -v helm >/dev/null 2>&1; then
        log "helm already installed: $(helm version --short 2>/dev/null || helm version)"
        return 0
    fi

    log "installing Helm..."
    curl -fsSL https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash
}

install_k9s() {
    if command -v k9s >/dev/null 2>&1; then
        log "k9s already installed: $(k9s version 2>/dev/null | head -1)"
        return 0
    fi

    log "installing k9s..."
    local arch os version tmpdir tarball
    arch=$(uname -m)
    case "$arch" in
        x86_64)  arch=amd64 ;;
        aarch64) arch=arm64 ;;
        armv7l)  arch=armv7 ;;
        *) die "unsupported architecture for k9s: $arch" ;;
    esac
    os=Linux
    version=$(curl -fsSL https://api.github.com/repos/derailed/k9s/releases/latest | sed -n 's/.*"tag_name": "v\([^"]*\)".*/\1/p')
    [[ -n "$version" ]] || die "failed to determine latest k9s version"

    tmpdir=$(mktemp -d)
    tarball="${tmpdir}/k9s.tar.gz"
    curl -fsSL "https://github.com/derailed/k9s/releases/download/v${version}/k9s_${os}_${arch}.tar.gz" -o "$tarball"
    tar -xzf "$tarball" -C "$tmpdir" k9s
    install -m 755 "${tmpdir}/k9s" /usr/local/bin/k9s
    rm -rf "$tmpdir"

    log "k9s installed: $(k9s version 2>/dev/null | head -1)"
}

setup_kubeconfig_symlink() {
    local real_user real_home kube_dir kube_cfg
    # When invoked via sudo, SUDO_USER is the original user; fallback to current user.
    real_user="${SUDO_USER:-$(id -un)}"
    real_home=$(getent passwd "$real_user" | cut -d: -f6)
    [[ -n "$real_home" ]] || { log "cannot determine home for ${real_user}; skipping kubeconfig symlink"; return 0; }

    kube_dir="${real_home}/.kube"
    kube_cfg="${kube_dir}/config"

    mkdir -p "$kube_dir"
    chown "${real_user}:" "$kube_dir"

    if [[ -e "$kube_cfg" || -L "$kube_cfg" ]]; then
        log "${kube_cfg} already exists; skipping symlink (manual setup may be needed)"
        return 0
    fi

    ln -s /etc/rancher/k3s/k3s.yaml "$kube_cfg"
    chown -h "${real_user}:" "$kube_cfg"
    log "created symlink: ${kube_cfg} -> /etc/rancher/k3s/k3s.yaml (for user ${real_user})"
}

write_env_file() {
    local env_file="${SCRIPT_DIR}/knl-env.sh"
    cat >"$env_file" <<EOF
# KNL environment — generated by setup-k3s.sh on $(date)
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
export KNL_CNI_BIN_DIR=/var/lib/rancher/k3s/data/cni/
export KNL_MULTUS_CONF_DIR=/var/lib/rancher/k3s/agent/etc/cni/net.d
export KNL_MULTUS_KUBECONFIG=/var/lib/rancher/k3s/agent/etc/cni/net.d/multus.d/multus.kubeconfig
export KNL_MULTUS_AUTOCONF_DIR=/var/lib/rancher/k3s/agent/etc/cni/net.d
export KNL_STORAGE_CLASS=${STORAGE_CLASS}
export KNL_REGISTRY=localhost:${REGISTRY_NODEPORT}
EOF
    chmod 644 "$env_file"
    # shellcheck source=/dev/null
    source "$env_file"
    log "KNL env written to ${env_file}"
}

main() {
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --install)              RUN_INSTALL=true ;;
            --k3s-version)          K3S_VERSION="$2"; shift ;;
            --storage-class)        STORAGE_CLASS="$2"; shift ;;
            --registry-nodeport)    REGISTRY_NODEPORT="$2"; shift ;;
            --registry-size)        REGISTRY_SIZE="$2"; shift ;;
            --skip-registry)        SKIP_REGISTRY=true ;;
            -h|--help)              usage; exit 0 ;;
            *)                      die "unknown option: $1 (try --help)" ;;
        esac
        shift
    done

    require_root
    command -v curl >/dev/null 2>&1 || die "curl is required"

    install_k3s
    wait_k3s_ready
    $SKIP_REGISTRY || configure_registry_insecure
    $SKIP_REGISTRY || deploy_registry
    enable_ipv6_forwarding
    install_helm
    install_k9s
    setup_kubeconfig_symlink
    write_env_file

    if $RUN_INSTALL; then
        log "chaining to install-knl.sh..."
        "${SCRIPT_DIR}/install-knl.sh"
    fi

    wait_all_pods_ready

    cat <<EOF

================================================================================
k3s cluster is ready for KNL.

KNL environment variables are written to ${SCRIPT_DIR}/knl-env.sh. To install KNL
components, run:

  source ${SCRIPT_DIR}/knl-env.sh
  ${SCRIPT_DIR}/install-knl.sh

Or re-run with --install:

  sudo ${SCRIPT_DIR}/setup-k3s.sh --install

================================================================================
EOF
}

main "$@"
