# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:nonroot
# FROM debian:stable-slim
COPY manager /
USER 2000:2000
ENTRYPOINT ["/manager"]
