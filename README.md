# <a href="https://hujun-open.github.io/knldoc/"><img src="logo.png" width="200"> </a>
KubeNetLab (KNL) is a k8s operator creates and manages virtual network topology consists of VNF/CNF for learning and testing purposes. 

### [Check here](https://hujun-open.github.io/knldoc/) for installation & usage guide.


KNL has following key features:

1. Running on top of k8s with capability of to scale out to entire k8s cluster. e.g. a single lab topology could span across multiple servers.
2. Topology is defined via KNL `Lab` CR, standard k8s tool like `kubectl` or `k9s` could be used to create/manage labs, while KNL cli tool `knlcli` provides more user friendly CLI operations.
3. Supports both CNF and VNF, following are node types supported today:
    - Nokia vSIM
    - Nokia SR-SIM
    - Nokia VSR-I
    - Nokia MAG-c
    - Nokia SRLinux
    - General Virtual Machine
    - General pod

Follow YAML defines a simple example-lab contains a Nokia SR-SIM and a Nokia vSIM, with default chassis configuration, connects to each other via a virtual link `link1`.

```yaml
apiVersion: knl.kubenetlab.net/v1beta1
kind: Lab
metadata:
    name: example-lab
    namespace: knl-system
spec:           
  links:
    link1: 
      nodes:
      - node: srsim-1
      - node: vsim-1
```


Use KNL command line tool `knlcli` to check the created Lab, the two lab nodes(`srsim-1` and `vsim-1`) run on two different servers (`worker-1` and `worker-2`), `link1` spans across these two servers.
```bash
user@svr-1:~$ knlcli show
example-lab:
   Node      Type    Chassis   Pods                                       Worker/PodIP
   srsim-1   SRSIM   SR-7      example-lab-srsim-1                        worker-1/10.244.2.92
   vsim-1    VSIM    SR-7      virt-launcher-example-lab-vsim-1-1-cjfn6   worker-2/10.244.0.95
                               virt-launcher-example-lab-vsim-1-a-lvhqd   worker-2/10.244.0.115
   Link      Nodes
   link1     srsim-1
             vsim-1

```
  