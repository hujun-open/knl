package v1beta1

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"path/filepath"

	"github.com/kubevirt/macvtap-cni/pkg/deviceplugin"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"kubenetlab.net/knl/common"
	kvv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	SRVMFBMTU           = 9000
	vSROSIDLabel        = `kubnetlab.net/vSROSSystemID`
	vSROSSysinfoAnno    = `smbios.vm.kubevirt.io/vSROSSysinfo`
	sftpSVRAnnontation  = "kubenetlab.net/sftpsvr"
	sftpUserAnnontation = "kubenetlab.net/sftpuser"
	sftpPassAnnontation = "kubenetlab.net/sftppasswd"
)

var (
	SRCPMVMDiskSize = resource.MustParse("64Mi")
)

func (srvm *SRVM) Ensure(ctx context.Context, nodeName string, clnt client.Client, forceRemoval bool) error {
	gconf := GCONF.Get()
	val := ctx.Value(ParsedLabKey)
	if val == nil {
		return common.MakeErr(fmt.Errorf("failed to get parsed lab obj from context"))
	}
	var lab *ParsedLab
	var ok bool
	if lab, ok = val.(*ParsedLab); !ok {
		return common.MakeErr(fmt.Errorf("context stored value is not a ParsedLabSpec"))
	}
	vmt, vmid, cardid, _ := ParseSRVMName(nodeName)
	_, isCPM, err := ParseCardID(cardid)
	if err != nil {
		return common.MakeErr(err)
	}
	//networking
	if isCPM && vmt != VSRI { //these are per distributed SR system NAD, only need one per system, so only CPM node creates them

		//check FB NAD
		fbnad := common.NewFBBridgeNetworkDef(lab.Lab.Namespace, lab.Lab.Name,
			common.GetVSROSFBName(VSIM, vmid),
			SRVMFBMTU,
		)
		err := createIfNotExistsOrRemove(ctx, clnt, lab, fbnad, true, forceRemoval)
		if err != nil {
			return common.MakeErr(err)
		}
		if vmt == MAGC {
			//MAG-c data fabric NAD
			dfnad := common.NewFBBridgeNetworkDef(lab.Lab.Namespace, lab.Lab.Name,
				common.GetMAGCDFName(vmid),
				SRVMFBMTU,
			)
			err := createIfNotExistsOrRemove(ctx, clnt, lab, dfnad, true, forceRemoval)
			if err != nil {
				return common.MakeErr(err)
			}
		}
	} else {
		//IOM VM, check link NAD and macvtapcfg
		//macvatpcfg
		cfgList := []deviceplugin.MacvtapConfig{}
		if linkList, ok := lab.ConnectorMap[nodeName]; ok {
			for _, linkName := range linkList {
				var gwPrefix *netip.Prefix = nil
				if lab.Lab.Spec.LinkList[linkName].GWAddr != nil {
					p := netip.MustParsePrefix(*lab.Lab.Spec.LinkList[linkName].GWAddr)
					gwPrefix = &p
				}
				cfgList = append(cfgList, deviceplugin.MacvtapConfig{
					Name:        common.GetMACVTAPResName(lab.Lab.Name, nodeName, linkName),
					LowerDevice: common.GetMACVTAPResName(lab.Lab.Name, nodeName, linkName),
					Desc:        fmt.Sprintf("%v-%v-%v", lab.Lab.Name, nodeName, linkName),
					Mode:        "passthru",
					Capacity:    1,
					MacVethBR: &deviceplugin.MacVethBRConfig{
						BridgeAddr:           gwPrefix,
						Bridge:               common.GetMACVTAPBrName(lab.Lab.Name, linkName),
						VethBR:               common.GetMACVTAPVethBrName(lab.Lab.Name, nodeName, linkName),
						VxLANIfName:          common.GetMACVTAPVXLANIfName(lab.Lab.Name, nodeName),
						VxLANMutilcastPrefix: netip.MustParsePrefix(*gconf.VXLANGrpPrefix),
						MTU:                  int(*gconf.LinkMtu),
					},
				})

				_, c := lab.Lab.getLinkandConnector(nodeName, linkName)
				var mac *net.HardwareAddr = nil
				if c != nil {
					if c.Mac != nil {
						pmac, err := net.ParseMAC(*c.Mac)
						if err == nil {
							mac = &pmac
						}
					}
				}
				linkNAD := common.NewPortMACVTAPNAD(
					lab.Lab.Namespace,
					lab.Lab.Name,
					common.GetLinkMACVTAPNADName(lab.Lab.Name, nodeName, linkName),
					common.GetMACVTAPResName(lab.Lab.Name, nodeName, linkName),
					uint16(*gconf.LinkMtu),
					mac,
				)
				err = createIfNotExistsOrRemove(ctx, clnt, lab, linkNAD, true, forceRemoval)
				if err != nil {
					return common.MakeErr(err)
				}

			}
		}
		err := UpdateMACVTAPDPCfg(clnt, cfgList, !forceRemoval)
		if err != nil {
			return common.MakeErr(err)
		}

	}
	//per system operation (one time per system)
	if isCPM {
		//check sr release
		expectedTarget := filepath.Join("/"+common.KNLROOTName, common.IMGSubFolder, *srvm.Image)
		vmlinkname := filepath.Join(common.KNLROOTName, common.GetFTPSROSImgPath(vmid))
		curLinked, err := os.Readlink(vmlinkname)
		if err != nil || curLinked != expectedTarget {
			//create sr release folder
			err = common.ReCreateSymLink(vmid, *srvm.Image)
			if err != nil {
				return common.MakeErr(err)
			}
		}
		//check sr cfg folder
		ftpPath := common.GetSRConfigFTPSubFolder(lab.Lab.Name, vmid)
		absPath := filepath.Join("/"+common.KNLROOTName, ftpPath)
		if _, err := os.Stat(absPath); errors.Is(err, os.ErrNotExist) {
			//create the folder
			err = os.MkdirAll(absPath, 0660)
			if err != nil {
				return common.MakeErr(err)
			}
		}
	}
	//SRVM CPM DV
	if isCPM {
		dv := common.NewDV(lab.Lab.Namespace, lab.Lab.Name,
			common.GetDVName(lab.Lab.Name, nodeName),
			*gconf.SRCPMLoaderImage, gconf.PVCStorageClass, &SRCPMVMDiskSize)
		err = createIfNotExistsOrRemove(ctx, clnt, lab, dv, true, forceRemoval)
		if err != nil {
			return common.MakeErr(err)
		}
	}

	//VMI
	vmi := srvm.getVMI(lab, nodeName)
	err = createIfNotExistsOrFailedOrRemove(ctx, clnt, lab, vmi, checkVMIfail, true, forceRemoval)
	if err != nil {
		return common.MakeErr(err)
	}
	return nil
}

func (srvm *SRVM) getVMI(lab *ParsedLab, vmname string) *kvv1.VirtualMachineInstance {
	gconf := GCONF.Get()
	vmt, vmid, cardid, _ := ParseSRVMName(vmname)
	_, isCPM, _ := ParseCardID(cardid)
	r := new(kvv1.VirtualMachineInstance)
	r.ObjectMeta = common.GetObjMeta(
		vmname,
		lab.Lab.Name,
		lab.Lab.Namespace,
	)
	r.ObjectMeta.Labels[vSROSIDLabel] = fmt.Sprintf("%d", vmid)
	//add sysinfo for SR like node
	cfgURL := common.GetSRConfigFTPSubFolder(lab.Lab.Name, vmid)
	r.ObjectMeta.Annotations = map[string]string{
		sftpSVRAnnontation:               *gconf.SFTPSever,
		sftpPassAnnontation:              *gconf.SFTPPassword,
		sftpUserAnnontation:              *gconf.SFTPUser,
		"hooks.kubevirt.io/hookSidecars": fmt.Sprintf(`[{"image": "%v"}]`, gconf.SideCarHookImg),
		vSROSSysinfoAnno: common.GenSysinfo(*srvm.SRSysinfoStr,
			cardid, cfgURL, *srvm.LicURL),
	}

	//can't set pc here will be rejected by adminssion webhook

	r.Spec.Domain.CPU = &kvv1.CPU{
		Model: "host-passthrough",
	}

	//check if need pin CPU
	// if common.IsResourcePinNeededViaSysinfo(node.SRSysinfoStr) {
	dedicated := false
	switch vmt {
	case VSRI, MAGC:
		dedicated = true
	}
	if dedicated {
		r.Spec.Domain.CPU.DedicatedCPUPlacement = true
		r.Spec.Domain.CPU.IsolateEmulatorThread = true
	}

	//enable video, no need to remove video
	r.Spec.Domain.Devices.AutoattachGraphicsDevice = new(bool)
	*r.Spec.Domain.Devices.AutoattachGraphicsDevice = true

	//cpu & memory
	r.Spec.Domain.CPU.Cores = uint32(srvm.ReqCPU.AsApproximateFloat64()) //if the cpu is decimal, this round down to the int
	//NOTE: kubevirt currently doesn't support memory balloning, to save memory, see https://kubevirt.io/user-guide/operations/node_overcommit/#overcommit-guest-memory
	r.Spec.Domain.Memory = &kvv1.Memory{
		Guest: srvm.ReqMemory,
	}
	//check if hugepage is needed
	if dedicated {
		r.Spec.Domain.Memory.Hugepages = &kvv1.Hugepages{
			PageSize: "1Gi",
		}
	}

	//for vsros node, disable auto console, this is needed since default unix socket console doesn't work for vsim
	switch vmt {
	case MAGC, VSIM, VSRI:
		r.Spec.Domain.Devices.AutoattachSerialConsole = new(bool)
		*r.Spec.Domain.Devices.AutoattachSerialConsole = false

	}

	//disk
	//add mainvol
	if isCPM {
		r.Spec.Volumes = append(r.Spec.Volumes,
			kvv1.Volume{
				Name: common.KNLROOTName,
				VolumeSource: kvv1.VolumeSource{
					DataVolume: &kvv1.DataVolumeSource{
						Name: common.GetDVName(lab.Lab.Name, vmname),
					},
					// //note: vsim only support qcow2, not RAW, so using hostdisk and hook to change it to qcow2
					// PersistentVolumeClaim: &kvv1.PersistentVolumeClaimVolumeSource{
					// 	PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
					// 		ClaimName: common.PVCName,
					// 	},
				},
			},
		)
	} else {
		//IOM
		r.Spec.Volumes = append(r.Spec.Volumes,
			kvv1.Volume{
				Name: common.KNLROOTName,
				VolumeSource: kvv1.VolumeSource{
					ContainerDisk: &kvv1.ContainerDiskSource{
						Image: *gconf.SRIOMLoaderImage,
					},
					// //note: vsim only support qcow2, not RAW, so using hostdisk and hook to change it to qcow2
					// PersistentVolumeClaim: &kvv1.PersistentVolumeClaimVolumeSource{
					// 	PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
					// 		ClaimName: common.PVCName,
					// 	},
				},
			},
		)
	}
	r.Spec.Domain.Devices.Disks = append(r.Spec.Domain.Devices.Disks,
		kvv1.Disk{
			Name: common.KNLROOTName,
			DiskDevice: kvv1.DiskDevice{
				Disk: &kvv1.DiskTarget{
					Bus: kvv1.DiskBusVirtio,
				},
			},
		})

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
			Ports: *srvm.Ports,
			InterfaceBindingMethod: kvv1.InterfaceBindingMethod{
				Masquerade: &kvv1.InterfaceMasquerade{},
			},
		})
	//fabric
	switch vmt {
	case VSIM, MAGC:
		if !common.IsIntegratedChassisViaSysinfo(*srvm.SRSysinfoStr) {
			//add fabric only if it is not integrated chassis
			r.Spec.Networks = append(r.Spec.Networks,
				kvv1.Network{
					Name: "fb-net",
					NetworkSource: kvv1.NetworkSource{
						Multus: &kvv1.MultusNetwork{
							NetworkName: common.GetVSROSFBName(vmt, vmid),
						},
					},
				})
			r.Spec.Domain.Devices.Interfaces = append(r.Spec.Domain.Devices.Interfaces,
				kvv1.Interface{
					Name: "fb-net",
					InterfaceBindingMethod: kvv1.InterfaceBindingMethod{
						Bridge: &kvv1.InterfaceBridge{},
					},
				},
			)
		}
		//add data fabric for mag-c
		if vmt == MAGC && !isCPM {
			r.Spec.Networks = append(r.Spec.Networks,
				kvv1.Network{
					Name: "df-net",
					NetworkSource: kvv1.NetworkSource{
						Multus: &kvv1.MultusNetwork{
							NetworkName: common.GetMAGCDFName(vmid),
						},
					},
				})
			r.Spec.Domain.Devices.Interfaces = append(r.Spec.Domain.Devices.Interfaces,
				kvv1.Interface{
					Name: "df-net",
					InterfaceBindingMethod: kvv1.InterfaceBindingMethod{
						Bridge: &kvv1.InterfaceBridge{},
					},
				},
			)

		}

	}

	//port links
	if links, ok := lab.ConnectorMap[vmname]; ok {
		for _, linkname := range links {
			r.Spec.Networks = append(r.Spec.Networks,
				kvv1.Network{
					Name: linkname,
					NetworkSource: kvv1.NetworkSource{
						Multus: &kvv1.MultusNetwork{
							NetworkName: common.GetLinkMACVTAPNADName(lab.Lab.Name, vmname, linkname),
						},
					},
				})
			r.Spec.Domain.Devices.Interfaces = append(r.Spec.Domain.Devices.Interfaces,
				kvv1.Interface{
					Name: linkname,
					Binding: &kvv1.PluginBinding{
						Name: "macvtap",
					},
				},
			)
		}
	}
	//define inter-pod affinity for distributed VSROS VM like vsime and magc, so that all vms of given system are on same node
	r.Spec.Affinity = &corev1.Affinity{
		PodAffinity: &corev1.PodAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
				{
					LabelSelector: &metav1.LabelSelector{
						MatchExpressions: []metav1.LabelSelectorRequirement{
							{
								Key:      vSROSIDLabel,
								Operator: metav1.LabelSelectorOpIn,
								Values:   []string{fmt.Sprintf("%d", vmid)},
							},
						},
					},
					TopologyKey: "kubernetes.io/hostname",
				},
			},
		},
	}

	return r
}
