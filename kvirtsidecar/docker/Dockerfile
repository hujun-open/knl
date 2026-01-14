FROM debian:bookworm-slim AS builder

WORKDIR /workspace
RUN apt update
RUN apt-get install -y curl
RUN curl -sSL https://github.com/hujun-open/ftpserver/releases/download/v0.0.1/ftpserver_0.0.1_linux_amd64.tar.gz | tar -xz -O ftpserver > ./ftpserver && chmod +x ./ftpserver


FROM debian:bookworm-slim

COPY --from=quay.io/kubevirt/sidecar-shim:v1.7.0 /sidecar-shim /shim
COPY onDefineDomain /usr/bin/
COPY --from=builder /workspace/ftpserver /usr/bin/ftpserver
CMD /bin/bash
ENTRYPOINT /shim --version v1alpha1