package v1beta1

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/netip"
	"net/url"
	"os"
	"syscall"

	ignitiontypes "github.com/coreos/ignition/v2/config/v3_5/types"
	k8slan "github.com/hujun-open/k8slan/api/v1beta1"
	"github.com/tredoe/osutil/user/crypt/sha512_crypt"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	kvv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	InitMethod_CLOUDINIT     = "cloud-init"
	InitMethod_IGNITION_NMGR = "ignition"
)

var VMBaseMAC = net.HardwareAddr{2, 0xff, 0, 0, 0, 99}

func init() {
	NewSysRegistry[VM] = func() System { return new(GeneralVM) }
}

// GeneralVM specifies a general kubevirt VM
type GeneralVM struct {
	//requested memory for the VM in k8s resource unit
	ReqMemory *resource.Quantity `json:"memory,omitempty"`
	//requested cpu for the VM in k8s resource unit
	ReqCPU *resource.Quantity `json:"cpu,omitempty"`
	//the VM disk size in k8s resource unit
	DiskSize *resource.Quantity `json:"diskSize,omitempty"`
	//pin the CPU if true
	PinCPU *bool `json:"cpuPin,omitempty"`
	//request hugepage memory if true
	HugePage *bool `json:"hugePage,omitempty"`
	//kubevirt CDI supported URL, either HTTP (http://) or registry source (docker://)
	Image *string `json:"image,omitempty"`
	//intilization method, supports cloud-init or ignition
	InitMethod *string `json:"init,omitempty"`
	//listening port of the VM on the 1st pod interface
	Ports *[]kvv1.Port `json:"ports,omitempty"`
	//username to login into VM, username and password are feed into vm initialization mechinism like cloud-init
	Username *string `json:"user,omitempty"`
	//password to login into VM
	Password *string `json:"passwd,omitempty"`
}

const (
	DefVPCCPU = "2.0"
	DefVPCMem = "4Gi"
)

const (
	VM NodeType = "vm"
)

func (gvm *GeneralVM) SetToAppDefVal() {
	gvm.ReqCPU = ReturnPointerVal(resource.MustParse(DefVPCCPU))
	gvm.ReqMemory = ReturnPointerVal(resource.MustParse(DefVPCMem))
	gvm.PinCPU = ReturnPointerVal(false)
	gvm.HugePage = ReturnPointerVal(false)
	gvm.Username = ReturnPointerVal("lab")
	gvm.Password = ReturnPointerVal("lab123")
	gvm.InitMethod = ReturnPointerVal(string(InitMethod_CLOUDINIT))
	gvm.Ports = ReturnPointerVal([]kvv1.Port{
		{
			Name:     "ssh",
			Protocol: "TCP",
			Port:     22,
		},
	})
}

func (gvm *GeneralVM) FillDefaultVal(nodeName string) {

}

func (gvm *GeneralVM) GetNodeType(name string) NodeType {

	return VM
}

func (gvm *GeneralVM) Validate(lab *LabSpec, nodeName string) error {
	if gvm.Image == nil {
		return fmt.Errorf("image not specified")
	}
	if gvm.DiskSize == nil {
		return fmt.Errorf("diskSize not specified")
	}
	if gvm.ReqCPU == nil {
		return fmt.Errorf("cpu not specified")
	}
	if gvm.ReqMemory == nil {
		return fmt.Errorf("memory not specified")
	}

	return nil
}
func (gvm *GeneralVM) Ensure(ctx context.Context, nodeName string, clnt client.Client, forceRemoval bool) error {
	gconf := GCONF.Get()
	val := ctx.Value(ParsedLabKey)
	if val == nil {
		return MakeErr(fmt.Errorf("failed to get parsed lab obj from context"))
	}
	var lab *ParsedLab
	var ok bool
	if lab, ok = val.(*ParsedLab); !ok {
		return MakeErr(fmt.Errorf("context stored value is not a ParsedLabSpec"))
	}
	//create DV
	dv := NewDV(lab.Lab.Namespace, lab.Lab.Name,
		GetVMPCDVName(lab.Lab.Name, nodeName),
		*gvm.Image, gconf.PVCStorageClass, gvm.DiskSize)
	err := createIfNotExistsOrRemove(ctx, clnt, lab, dv, false, forceRemoval)
	if err != nil {
		return MakeErr(err)
	}
	//create vm
	vmi := gvm.getVMI(lab, nodeName)
	err = createIfNotExistsOrFailedOrRemove(ctx, clnt, lab, vmi, checkVMIfail, true, forceRemoval)
	if err != nil {
		return MakeErr(err)
	}
	return nil
}

func (gvm *GeneralVM) getVMI(lab *ParsedLab, vmname string) *kvv1.VirtualMachineInstance {
	// log := ctrl.Log.WithName("Discover")
	gconf := GCONF.Get()
	r := new(kvv1.VirtualMachineInstance)
	r.ObjectMeta = GetObjMeta(
		GetPodName(lab.Lab.Name, vmname),
		lab.Lab.Name,
		lab.Lab.Namespace,
		vmname,
		VM,
	)
	r.ObjectMeta.Annotations = map[string]string{
		// LabNameAnnotation:       lab.Lab.Name,
		// ChassisTypeAnnotation:   string(VM),
		KvirtSideCarAnnontation: fmt.Sprintf(`[{"image": "%v"}]`, *gconf.SideCarHookImg),
	}
	r.Spec.Domain.CPU = &kvv1.CPU{
		Model: "host-passthrough",
	}
	if *gvm.PinCPU {
		r.Spec.Domain.CPU.DedicatedCPUPlacement = true
		r.Spec.Domain.CPU.IsolateEmulatorThread = true
	}
	r.Spec.Domain.CPU.Cores = uint32(gvm.ReqCPU.AsApproximateFloat64()) //if the cpu is decimal, this round down to the int
	//NOTE: kubevirt currently doesn't support memory balloning, to save memory, see https://kubevirt.io/user-guide/operations/node_overcommit/#overcommit-guest-memory
	//NOTE: user could also set `spec.configuration.developerConfiguration.memoryOvercommit` in kubevirt CR
	r.Spec.Domain.Memory = &kvv1.Memory{
		Guest: gvm.ReqMemory,
	}

	if *gvm.HugePage {
		r.Spec.Domain.Memory.Hugepages = &kvv1.Hugepages{
			PageSize: "1Gi",
		}
	}
	//enable video, no need to remove video
	r.Spec.Domain.Devices.AutoattachGraphicsDevice = new(bool)
	*r.Spec.Domain.Devices.AutoattachGraphicsDevice = true
	//disk
	r.Spec.Volumes = append(r.Spec.Volumes,
		kvv1.Volume{
			Name: "root",
			VolumeSource: kvv1.VolumeSource{
				DataVolume: &kvv1.DataVolumeSource{
					Name: GetVMPCDVName(lab.Lab.Name, vmname),
				},
			},
		},
	)
	r.Spec.Domain.Devices.Disks = append(r.Spec.Domain.Devices.Disks,
		kvv1.Disk{
			Name: "root",
			DiskDevice: kvv1.DiskDevice{
				Disk: &kvv1.DiskTarget{
					Bus: kvv1.DiskBusVirtio,
				},
			},
		})
	//add cloud-init vol
	// NOTE: make sure there is no inconsistent indent in cloud-init template, like mixed tab and spaces
	const initVolName = "cloudinitvol"
	var cloudinitNetowkrCfg = getDefCloudinitNetworkCfg()
	// 	var cloudinitNetworkDataTemplateBase = fmt.Sprintf(`network:
	//   version: 2
	//   ethernets:
	//     firstnic:
	//       match:
	//         macaddress: "%v"
	//       dhcp4: true`, VMBaseMAC)
	// 	const cloudinitNetworkDataTemplateIntf = `
	//     nic%d:
	//       match:
	//         macaddress: "%v"
	//       addresses:
	//         - %v
	//       %v`
	const cloudinitUserDataTemplate = `
#cloud-config
ssh_pwauth: True
users:
  - name: %v
    shell: /bin/bash
    plain_text_passwd: %v
    lock_passwd: false
    sudo: ALL=(ALL) NOPASSWD:ALL`

	initVolIndex := len(r.Spec.Volumes)
	//ignition
	pHash, _ := genPasswdHash(*gvm.Password)
	userShell := "/bin/bash"
	sudoerFile := encodeDataURL(*gvm.Username + " ALL=(ALL) NOPASSWD:ALL")
	ignitionData := ignitiontypes.Config{
		Ignition: ignitiontypes.Ignition{
			Version: "3.2.0", //version is mandatory
		},
		Passwd: ignitiontypes.Passwd{
			Users: []ignitiontypes.PasswdUser{
				{
					Name:         *gvm.Username,
					PasswordHash: &pHash,
					Shell:        &userShell,
					Groups:       []ignitiontypes.Group{"wheel"},
				},
			},
		},
		Storage: ignitiontypes.Storage{
			Files: []ignitiontypes.File{
				{
					Node: ignitiontypes.Node{
						Path: "/etc/sudoers.d/" + *gvm.Username,
					},
					FileEmbedded1: ignitiontypes.FileEmbedded1{
						Contents: ignitiontypes.Resource{
							Source: &sudoerFile,
						},
					},
				},
			},
		},
	}
	nm_file_template := `
[connection]
id=static-%v
type=ethernet

[ethernet]
mac-address=%v
[ipv4]
method=manual
# Format: IP_ADDRESS/CIDR,GATEWAY
%v

[ipv6]
method=manual
# Format: IP_ADDRESS/PREFIX_LENGTH,GATEWAY
%v`
	switch *gvm.InitMethod {
	case InitMethod_IGNITION_NMGR:
		r.Spec.Volumes = append(r.Spec.Volumes,
			kvv1.Volume{
				Name: initVolName,
				VolumeSource: kvv1.VolumeSource{
					CloudInitConfigDrive: &kvv1.CloudInitConfigDriveSource{
						UserData: "",
					},
				},
			})

	case InitMethod_CLOUDINIT:
		r.Spec.Volumes = append(r.Spec.Volumes,
			kvv1.Volume{
				Name: initVolName,
				VolumeSource: kvv1.VolumeSource{
					CloudInitNoCloud: &kvv1.CloudInitNoCloudSource{
						UserData: fmt.Sprintf(cloudinitUserDataTemplate, *gvm.Username, *gvm.Password),
					},
				},
			})
		r.Spec.Domain.Devices.Disks = append(r.Spec.Domain.Devices.Disks,
			kvv1.Disk{
				Name: initVolName,
				DiskDevice: kvv1.DiskDevice{
					Disk: &kvv1.DiskTarget{
						Bus: kvv1.DiskBusVirtio,
					},
				},
			})

	}

	//net
	//add pod interface, this is used for console telnet access
	r.Spec.Networks = append(r.Spec.Networks,
		kvv1.Network{
			Name: "pod-net",
			NetworkSource: kvv1.NetworkSource{
				Pod: &kvv1.PodNetwork{},
			},
		})
	r.Spec.Domain.Devices.Interfaces = append(r.Spec.Domain.Devices.Interfaces,
		kvv1.Interface{
			Name: "pod-net",
			//the port is needed here to prevent all traffic go into VM
			Ports:      *gvm.Ports,
			MacAddress: VMBaseMAC.String(),
			InterfaceBindingMethod: kvv1.InterfaceBindingMethod{
				Masquerade: &kvv1.InterfaceMasquerade{},
			},
		})
	//port links
	for _, linkName := range GetSortedKeySlice(lab.SpokeMap[vmname]) {
		spokes := lab.SpokeMap[vmname][linkName]
		// for _, spokes := range lab.SpokeMap[vmname] {
		for _, spokeName := range spokes {
			lanName := Getk8lanName(lab.Lab.Name, lab.SpokeLinkMap[spokeName])
			nadName := k8slan.GetDefNADName(lanName, spokeName, false)
			r.Spec.Networks = append(r.Spec.Networks,
				kvv1.Network{
					Name: spokeName,
					NetworkSource: kvv1.NetworkSource{
						Multus: &kvv1.MultusNetwork{
							NetworkName: nadName,
						},
					},
				})
			spokeIf := kvv1.Interface{
				Name: spokeName,
				Binding: &kvv1.PluginBinding{
					Name: "macvtap",
				},
			}
			if lab.SpokeConnectorMap[spokeName].Mac != nil {
				spokeIf.MacAddress = *lab.SpokeConnectorMap[spokeName].Mac
			}
			r.Spec.Domain.Devices.Interfaces = append(r.Spec.Domain.Devices.Interfaces, spokeIf)
		}
	}
	//assign link address
	i := 0
	if links, ok := lab.ConnectorMap[vmname]; ok {
		for _, linkname := range links {
			_, c := lab.getLinkandConnector(vmname, linkname)
			if c != nil {
				i++
				if len(c.Addrs) > 0 {

					//add network data for startup config

					switch *gvm.InitMethod {
					case InitMethod_IGNITION_NMGR:
						// caddr := netip.MustParsePrefix(*c.Addrs)
						v4addrstr := ""
						v6addstr := ""
						for i, addrStr := range c.Addrs {
							addr := netip.MustParseAddr(addrStr)
							if addr.Is4() {
								v4addrstr += fmt.Sprintf("address%d=%v\n", i+1, addr.String())
							} else {
								v6addstr += fmt.Sprintf("address%d=%v\n", i+1, addr.String())
							}
						}
						for i, routeStr := range c.Routes {
							route := mustParseRoute(routeStr)
							if route.Via.Is4() {
								v4addrstr += fmt.Sprintf("route%d=%v,%v\n", i+1, route.To.String(), route.Via.String())
							} else {
								v6addstr += fmt.Sprintf("route%d=%v,%v\n", i+1, route.To.String(), route.Via.String())
							}
						}
						connectionFile := encodeDataURL(fmt.Sprintf(nm_file_template, i, *c.Mac, v4addrstr, v6addstr))
						ignitionData.Storage.Files = append(ignitionData.Storage.Files,
							ignitiontypes.File{
								Node: ignitiontypes.Node{
									Path: fmt.Sprintf("/etc/NetworkManager/system-connections/static-%d.nmconnection", i),
								},
								FileEmbedded1: ignitiontypes.FileEmbedded1{
									Contents: ignitiontypes.Resource{
										Source: &connectionFile,
									},
								},
							},
						)
					case InitMethod_CLOUDINIT:
						cloudinitNetowkrCfg.AddConnector(fmt.Sprintf("nic%d", i), c)
						// if r.Spec.Volumes[initVolIndex].CloudInitNoCloud.NetworkData == "" {
						// 	r.Spec.Volumes[initVolIndex].CloudInitNoCloud.NetworkData = cloudinitNetworkDataTemplateBase
						// }

						// r.Spec.Volumes[initVolIndex].CloudInitNoCloud.NetworkData += fmt.Sprintf(
						// 	cloudinitNetworkDataTemplateIntf,
						// 	i,
						// 	*c.Mac,
						// 	*c.Addrs,
						// 	gwStr,
						// )
					}
				}
			}
		}
	}
	switch *gvm.InitMethod {
	case InitMethod_CLOUDINIT:
		r.Spec.Volumes[initVolIndex].CloudInitNoCloud.NetworkData = string(cloudinitNetowkrCfg.Marshal())

	}

	if *gvm.InitMethod == InitMethod_IGNITION_NMGR {
		buf, _ := json.MarshalIndent(ignitionData, "", "  ")
		r.Spec.Volumes[initVolIndex].CloudInitConfigDrive.UserData = string(buf)
	}
	return r
}

// this functon generate hash for passwd, used for linux passwd hash provsion
func genPasswdHash(passwd string) (string, error) {
	c := sha512_crypt.New()
	return c.Generate([]byte(passwd), []byte(""))
}

// this encode msg in data URL according to rfc2397, this is what ignition requires: https://coreos.github.io/ignition/examples/#create-files-on-the-root-filesystem
func encodeDataURL(msg string) string {
	return fmt.Sprintf("data:,%v", url.PathEscape(msg))
}

func (gvm *GeneralVM) Shell(ctx context.Context, clnt client.Client, ns, lab, chassis, username string) {
	podList := &corev1.PodList{}
	labelSelector := client.MatchingLabels{
		"vm.kubevirt.io/name": GetPodName(lab, chassis),
	}
	err := clnt.List(ctx, podList, client.InNamespace(ns), labelSelector)
	if err != nil {
		log.Fatalf("failed to list pods: %v", err)
	}
	if len(podList.Items) == 0 {
		log.Fatalf("failed to find vm pod %v", GetPodName(lab, chassis))

	}
	fmt.Println("connecting to", chassis, "at", podList.Items[0].Status.PodIP, "username", *gvm.Username)
	SysCallSSH(*gvm.Username, podList.Items[0].Status.PodIP)
}

func (gvm *GeneralVM) Console(ctx context.Context, clnt client.Client, ns, lab, chassis string) {

	podList := &corev1.PodList{}
	labelSelector := client.MatchingLabels{
		"vm.kubevirt.io/name": GetPodName(lab, chassis),
	}
	err := clnt.List(ctx, podList, client.InNamespace(ns), labelSelector)
	if err != nil {
		log.Fatalf("failed to list pods: %v", err)
	}
	if len(podList.Items) == 0 {
		log.Fatalf("failed to find vm pod %v", GetPodName(lab, chassis))

	}
	envList := []string{fmt.Sprintf("HOME=%v", os.Getenv("HOME"))}
	fmt.Println("connecting to console of", chassis, "at", podList.Items[0].Status.PodIP)
	syscall.Exec("/bin/sh",
		[]string{"sh", "-c",
			fmt.Sprintf("telnet %v %d", podList.Items[0].Status.PodIP, SRVMConsoleTCPPort)},
		envList)
}
