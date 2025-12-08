## VM naming
- name format for vsim/magc: vmtype-vmid-cardid
- name format for vpc/vsri: vmtype-vmid
- cardid is either a,b or a number


## KNL PVC
it contains following files:
- TIMOS images
- SROS configurations 

this PVC need to be mounted on sftp server container mentioned below

## managment interface, VM image and TIMOS Image
1. set mgmt interface to pod network interface for both cpm and iom vm
2. by default, kubevirt assign 10.0.2.2/24 to the first interface in the VM, which connects to a bridge interface `k6t-eth0` interface in the virt-launcher pod, which has ip `10.0.2.1/24`
3. the idea is to run a ftp server inside kubevirt hook container, that ftp server proxy the ftp request from SRVM to real backend, e.g. using `https://github.com/fclairamb/ftpserver/`, which supports using sftp server as backend
    - the kubevirt hook program launch the ftp server via "nohup" at end of execution 
4. configure SRVM disk `nvram.dat` and `bof.cfg` to use `10.0.2.2/24` as VM's ip and `10.0.2.1` as the ftp server address
  - the CPM disk must have a bof.cfg that contains "primary-image" line
5. the ftp server run in the virt-launcher pod has capbility to translate path, so that the path name in `nvram.dat` and `bof.cfg` could be fixed 
    - example nvram.dat: fei(0,0)10.0.2.1:/i386-boot.tim e=10.0.2.2:ffffff00 h=10.0.2.1 g=10.0.2.1 u=sw pw=sw123 tn=vsim-2-a f=0x8 s=p1=1
    - example primar-image in bof.cfg: `primary-image    ftp://sw:sw123@10.0.2.1/sros/`
    - this requires a ftp sever supports such path translation feautre
    - the implementation looks like following:
        - kubevirt hook get's vm name, derived the actual file/folder path on backend server (e.g. sftp server path for a given vsim)
        - the hook generate the config file that contains translation rule for the ftp server and launch the ftp server
        - note: both cpm and iom vm could use same nvram.dat, because in case of iom-vm,TIMOS boot loader will automatically changes the "/i386-boot.time" to "/i386-iom.tim"
```mermaid
architecture-beta
  group vmpod[vm pod]
    service vmcontainer[vm container] in vmpod
    group sidecar[sidecar container] in vmpod
      service ftpproxy[ftp proxy] in sidecar
    vmcontainer:L --> R:ftpproxy
  group operator[operator pod]
    service sftpsvr[sftp svr container] in operator
  
  ftpproxy:L --> R:sftpsvr
  service knlpvc[KNL PVC]
  knlpvc:B --> T:sftpsvr


```

This way, we achieve following:
- we don't need to assign or derive SRVM's managment address, since is always `10.0.2.2/24`
- SRVM managment interface could be reach via normal k8s network, no need for seperate multus management network and seperate inter-server link for managment
- no need to update the qcow2 image for each vsim, but do need a duplicate copy for each CPM vsim for persistency
    - using CDI could work (tested), need to use RAW format disk image to build the container image like following Dockerfile, it is important to use `kubevirt/container-disk-v1alpha` even it is old 
    ```
    FROM kubevirt/container-disk-v1alpha
    ADD vsimload.img /disk
    ```
    although still the kubevirt hook need to update the disk setting in xml to make it work, the final xml looks like following:
    ```
    <disk type='file' device='disk' model='virtio'>
      <driver name='qemu' type='raw' cache='none'/>
      <source file='/var/run/kubevirt-private/vmi-disks/knlroot/disk.img' index='1'/>
      <backingStore/>
      <target dev='hda' bus='virtio'/>
      <alias name='ua-knlroot'/>
      <address type='pci' domain='0x0000' bus='0x00' slot='0x05' function='0x0'/>
    </disk>

    ```
- for IOM-VM, since there is no persistency requirement, just use a container disk, however this container disk image must use qcow2 format (the the actual image is same as CPM VM, just in qcow2 format) with Dockerfile like following: 
```
FROM scratch
ADD --chown=107:107 iomloader.qcow2 /disk/
```
 still the kubevirt hook need to update the disk setting in xml to make it work, the final xml looks like following:

```
   <disk type='file' device='disk' model='virtio'>
      <driver name='qemu' type='qcow2' cache='none'/>
      <source file='/var/run/kubevirt-ephemeral-disks/disk-data/knlroot/disk.qcow2' index='1'/>
      <backingStore type='file' index='2'>
        <format type='qcow2'/>
        <source file='/var/run/kubevirt/container-disks/disk_0.img'/>
      </backingStore>
      <target dev='hda' bus='virtio'/>
      <alias name='ua-knlroot'/>
      <address type='pci' domain='0x0000' bus='0x00' slot='0x05' function='0x0'/>
    </disk>
```

### summary 
- for CPM VM, it is kubevirt DV per VM
- for IOM VM, container disk
- TIMOS image loaded via ftp proxy


## SROS configuration
also via ftp proxy, in the sftp server with path like `/data/cfgs/<lab-name>/<vsim-name>`, however it requires the folder needs to be auto created by operator, which implies sftp server better to stay in the operator pod as seperator container and also mounts the knl PVC