#!/usr/bin/env bash
# install-knl.sh — install KNL prerequisites and the KNL operator on an existing cluster.
#
# Distribution-agnostic: cluster setup scripts (e.g. setup-k3s.sh) must export the
# KNL_* env vars documented below before running this script.
#
# Usage:
#   ./install-knl.sh [options]
#   source ./install-knl.sh   # load functions only; main() is not run
#
# Required env vars (set by setup-*.sh):
#   KNL_CNI_BIN_DIR          host path for CNI plugin binaries
#   KNL_MULTUS_CONF_DIR      Multus CNI config directory
#   KNL_MULTUS_KUBECONFIG    Multus kubeconfig path
#   KNL_MULTUS_AUTOCONF_DIR  Multus autoconfig directory
#
# Optional env vars (defaults shown):
#   KNL_CERTMANAGER_VERSION  v1.17.2
#   KNL_KUBEVIRT_VERSION     v1.6.3
#   KNL_CDI_VERSION          v1.63.1
#   KNL_NAMESPACE            knl-system
#   KNL_PVC_SIZE             10Gi
#   KNL_STORAGE_CLASS        local-path
#   KNL_ROLLOUT_TIMEOUT      600s

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
_KNL_ENV_FILE="${SCRIPT_DIR}/knl-env.sh"
if [[ -f "$_KNL_ENV_FILE" ]]; then
    # shellcheck source=/dev/null
    source "$_KNL_ENV_FILE"
fi
unset _KNL_ENV_FILE

# ---------------------------------------------------------------------------
# Defaults
# ---------------------------------------------------------------------------
: "${KNL_CERTMANAGER_VERSION:=v1.17.2}"
: "${KNL_KUBEVIRT_VERSION:=v1.6.3}"
: "${KNL_CDI_VERSION:=v1.63.1}"
: "${KNL_NAMESPACE:=knl-system}"
: "${KNL_PVC_SIZE:=10Gi}"
: "${KNL_STORAGE_CLASS:=local-path}"
: "${KNL_ROLLOUT_TIMEOUT:=600s}"

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------
knl_log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*"
}

knl_die() {
    knl_log "ERROR: $*" >&2
    exit 1
}

knl_require_cmd() {
    local cmd
    for cmd in "$@"; do
        command -v "$cmd" >/dev/null 2>&1 || knl_die "required command not found: $cmd"
    done
}

knl_require_env() {
    local var
    for var in "$@"; do
        [[ -n "${!var:-}" ]] || knl_die "required env var not set: $var (run a cluster setup script first)"
    done
}

knl_wait_rollout() {
    local kind="$1" name="$2" ns="$3"
    local timeout="${4:-$KNL_ROLLOUT_TIMEOUT}"
    knl_log "waiting for ${kind}/${name} in ${ns} (timeout ${timeout})..."
    kubectl -n "$ns" rollout status "$kind/$name" --timeout="$timeout"
}

knl_wait_kubevirt_available() {
    local timeout="${1:-$KNL_ROLLOUT_TIMEOUT}"
    knl_log "waiting for KubeVirt CR to become Available (timeout ${timeout})..."
    kubectl -n kubevirt wait kubevirt/kubevirt --for=condition=Available --timeout="$timeout"
}

# ---------------------------------------------------------------------------
# Component installers
# ---------------------------------------------------------------------------
knl_install_cert_manager() {
    knl_log "installing cert-manager ${KNL_CERTMANAGER_VERSION}..."
    kubectl apply -f "https://github.com/cert-manager/cert-manager/releases/download/${KNL_CERTMANAGER_VERSION}/cert-manager.yaml"
    knl_wait_rollout deployment cert-manager-webhook cert-manager
    knl_log "cert-manager installed"
}

knl_install_kubevirt() {
    knl_log "installing KubeVirt ${KNL_KUBEVIRT_VERSION}..."
    kubectl apply -f "https://github.com/kubevirt/kubevirt/releases/download/${KNL_KUBEVIRT_VERSION}/kubevirt-operator.yaml"
    kubectl apply -f "https://github.com/kubevirt/kubevirt/releases/download/${KNL_KUBEVIRT_VERSION}/kubevirt-cr.yaml"
    knl_wait_rollout deployment virt-operator kubevirt
    knl_wait_kubevirt_available
    knl_log "KubeVirt installed"
}

knl_install_cdi() {
    knl_log "installing CDI ${KNL_CDI_VERSION}..."
    kubectl apply -f "https://github.com/kubevirt/containerized-data-importer/releases/download/${KNL_CDI_VERSION}/cdi-operator.yaml"
    kubectl apply -f "https://github.com/kubevirt/containerized-data-importer/releases/download/${KNL_CDI_VERSION}/cdi-cr.yaml"
    knl_wait_rollout deployment cdi-operator cdi
    knl_log "CDI installed"
}

knl_configure_kubevirt() {
    knl_log "configuring KubeVirt feature gates and macvtap binding..."
    kubectl patch kubevirt -n kubevirt kubevirt --type=merge -p '{
  "spec": {
    "configuration": {
      "developerConfiguration": {
        "featureGates": [
          "Sidecar",
          "NetworkBindingPlugins",
          "HostDevices",
          "Root",
          "CpuManager"
        ]
      },
      "network": {
        "binding": {
          "macvtap": {
            "domainAttachmentType": "tap"
          }
        }
      }
    }
  }
}'
    knl_log "KubeVirt configured"
}

knl_install_multus() {
    knl_require_env KNL_CNI_BIN_DIR KNL_MULTUS_CONF_DIR KNL_MULTUS_KUBECONFIG KNL_MULTUS_AUTOCONF_DIR
    knl_require_cmd helm

    knl_log "installing Multus via Helm (host-local IPAM)..."
    helm repo add rke2-charts https://rke2-charts.rancher.io 2>/dev/null || true
    helm repo update rke2-charts

    helm upgrade --install multus rke2-charts/rke2-multus \
        --namespace kube-system \
        --create-namespace \
        --wait \
        --timeout "${KNL_ROLLOUT_TIMEOUT}" \
        --set-string "config.fullnameOverride=multus" \
        --set-string "config.cni_conf.confDir=${KNL_MULTUS_CONF_DIR}" \
        --set-string "config.cni_conf.binDir=${KNL_CNI_BIN_DIR}" \
        --set-string "config.cni_conf.kubeconfig=${KNL_MULTUS_KUBECONFIG}" \
        --set-string "config.cni_conf.multusAutoconfigDir=${KNL_MULTUS_AUTOCONF_DIR}"

    knl_log "Multus installed"
}

knl_install_cni_plugins() {
    knl_require_env KNL_CNI_BIN_DIR

    knl_log "installing containernetworking CNI plugins into ${KNL_CNI_BIN_DIR}..."

    # Adapted from https://github.com/hujun-open/installcni — hostPath uses KNL_CNI_BIN_DIR.
    kubectl apply -f - <<EOF
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: knl-install-cni
  namespace: kube-system
  labels:
    app: knl-install-cni
spec:
  selector:
    matchLabels:
      app: knl-install-cni
  template:
    metadata:
      labels:
        app: knl-install-cni
    spec:
      tolerations:
        - key: node-role.kubernetes.io/control-plane
          operator: Exists
          effect: NoSchedule
        - key: node-role.kubernetes.io/master
          operator: Exists
          effect: NoSchedule
        - operator: Exists
      volumes:
        - name: cni
          hostPath:
            path: ${KNL_CNI_BIN_DIR}
            type: DirectoryOrCreate
      initContainers:
        - name: install
          image: ghcr.io/hujun-open/installcni:latest
          command: ['sh', '-c', '/getcni.sh ${KNL_CNI_BIN_DIR}']
          volumeMounts:
            - mountPath: ${KNL_CNI_BIN_DIR}
              name: cni
      containers:
        - name: pause
          image: registry.k8s.io/pause:3.9
EOF

    knl_wait_rollout daemonset knl-install-cni kube-system
    knl_log "CNI plugins installed"
}

knl_install_k8slan() {
    knl_log "installing k8slan..."
    kubectl apply -f https://github.com/hujun-open/k8slan/releases/latest/download/all.yaml
    knl_wait_rollout deployment k8slan-controller-manager k8slan-system
    knl_log "k8slan installed"
}

knl_install_knl_operator() {
    knl_log "installing KNL operator in namespace ${KNL_NAMESPACE}..."

    kubectl create namespace "${KNL_NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f -

    kubectl apply -f - <<EOF
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: knl-pvc
  namespace: ${KNL_NAMESPACE}
spec:
  accessModes:
    - ReadWriteOncePod
  resources:
    requests:
      storage: ${KNL_PVC_SIZE}
  storageClassName: ${KNL_STORAGE_CLASS}
EOF

    kubectl apply -f https://github.com/hujun-open/knl/releases/latest/download/all.yaml
    knl_wait_rollout deployment knl-controller-manager "${KNL_NAMESPACE}"
    knl_log "KNL operator installed"
}

knl_install_knlcli() {
    knl_log "installing knlcli to /usr/local/bin/knlcli..."
    local tmp
    tmp="$(mktemp)"
    trap 'rm -f "$tmp"' RETURN
    curl -fsSL -o "$tmp" https://github.com/hujun-open/knlcli/releases/latest/download/knlcli-linux-x86-64
    install -m 0755 "$tmp" /usr/local/bin/knlcli
    trap - RETURN
    knl_log "knlcli installed: $(knlcli version 2>/dev/null || echo 'knlcli')"
}

knl_print_post_install() {
    cat <<EOF

================================================================================
KNL installation complete.

Next steps:

1. Create a KNLConfig CR in namespace ${KNL_NAMESPACE}:
     kubectl -n ${KNL_NAMESPACE} apply -f knlcfg.yaml

   See: https://hujun-open.github.io/knldoc/docs/usage/knlconfig/

2. (Optional) Update the knl-sftp secret credentials:
     kubectl -n ${KNL_NAMESPACE} edit secret knl-sftp

3. Provision licenses for node types that require them, e.g.:
     kubectl -n ${KNL_NAMESPACE} create secret generic vsimlic --from-file=license=<lic_file>

4. Create a Lab CR to deploy your topology:
     kubectl apply -f lab.yaml

================================================================================
EOF
}

# ---------------------------------------------------------------------------
# Main orchestrator
# ---------------------------------------------------------------------------
usage() {
    cat <<EOF
Usage: $(basename "$0") [options]

Install KNL prerequisites and the KNL operator on an existing Kubernetes cluster.
Cluster setup scripts must export KNL_* env vars before running this script.

Options:
  --skip-cert-manager   skip cert-manager installation
  --skip-kubevirt       skip KubeVirt, CDI, and KubeVirt configuration
  --skip-multus         skip Multus installation
  --skip-cni            skip containernetworking CNI plugins installation
  --skip-k8slan         skip k8slan installation
  --skip-knl            skip KNL operator installation
  --skip-knlcli         skip knlcli download
  -h, --help            show this help

Environment:
  KNL_CNI_BIN_DIR, KNL_MULTUS_CONF_DIR, KNL_MULTUS_KUBECONFIG,
  KNL_MULTUS_AUTOCONF_DIR, KNL_STORAGE_CLASS, KNL_NAMESPACE, ...
EOF
}

main() {
    local skip_cert_manager=false
    local skip_kubevirt=false
    local skip_multus=false
    local skip_cni=false
    local skip_k8slan=false
    local skip_knl=false
    local skip_knlcli=false

    while [[ $# -gt 0 ]]; do
        case "$1" in
            --skip-cert-manager) skip_cert_manager=true ;;
            --skip-kubevirt)     skip_kubevirt=true ;;
            --skip-multus)       skip_multus=true ;;
            --skip-cni)          skip_cni=true ;;
            --skip-k8slan)       skip_k8slan=true ;;
            --skip-knl)          skip_knl=true ;;
            --skip-knlcli)       skip_knlcli=true ;;
            -h|--help)           usage; exit 0 ;;
            *)                   knl_die "unknown option: $1 (try --help)" ;;
        esac
        shift
    done

    knl_require_cmd kubectl curl
    knl_require_env KNL_CNI_BIN_DIR KNL_MULTUS_CONF_DIR KNL_MULTUS_KUBECONFIG KNL_MULTUS_AUTOCONF_DIR

    knl_log "starting KNL component installation..."
    knl_log "  KNL_CNI_BIN_DIR=${KNL_CNI_BIN_DIR}"
    knl_log "  KNL_STORAGE_CLASS=${KNL_STORAGE_CLASS}"
    knl_log "  KNL_NAMESPACE=${KNL_NAMESPACE}"

    $skip_cert_manager || knl_install_cert_manager
    if ! $skip_kubevirt; then
        knl_install_kubevirt
        knl_install_cdi
        knl_configure_kubevirt
    fi
    $skip_multus || knl_install_multus
    $skip_cni || knl_install_cni_plugins
    $skip_k8slan || knl_install_k8slan
    $skip_knl || knl_install_knl_operator
    $skip_knlcli || knl_install_knlcli

    knl_print_post_install
}

# Run main only when executed directly; skip when sourced.
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi
