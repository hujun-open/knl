FROM debian:bookworm-slim
RUN apt update
RUN apt-get install -y ftp ssh tcpdump telnet iputils-ping iproute2 vim
COPY shim /
COPY onDefineDomain /usr/bin/
COPY ftpserver /usr/bin/
CMD /bin/bash
ENTRYPOINT /shim --version v1alpha1
