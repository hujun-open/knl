Following are idea explored but won't work

1. use pod net as the managment interface for vsim, and use `masquerade` mode so that nvram.dat could use the fixed IP, however VSIM boot ftp client doesn't support PASV mode, and active mode can't go through NAT
    - kubevirt VM must use "10.0.2.2/24" as the interface address, otherwise NAT won't work
    - kubevirt allows user to customize the subnet of "10.0.2.0/24" in [PodNetwork section of VMI](https://kubevirt.io/api-reference/master/definitions.html#_v1_podnetwork)
2. use pod net as the managment interface for vsim, and use `masquerade` mode so that nvram.dat could use the fixed IP, and start a ftp server directly in the virt-launch pod, so that there is no NAT between vsim vm and ftp server, however kubevirt doesn't support sidecar container, and also you can't launch daemon using kubevirt hook sidecar directly
    - however using `exec.Command("/bin/sh", "-c", "/bin/nohup /bin/nc -p 2211 -l &")` works 