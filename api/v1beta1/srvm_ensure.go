package v1beta1

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	k8slan "github.com/hujun-open/k8slan/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"kubenetlab.net/knl/common"
	"kubenetlab.net/knl/dict"
	kvv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	SRVMFBMTU               = 9000
	vSROSIDLabel            = `kubnetlab.net/vSROSSystemID`
	SRVMLicSecretKeyName    = `license`
	KNLSftpCredentialSecret = `knl-sftp`
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
	vmt, _ := ParseSRVMName_New(nodeName)
	//networking
	indexNum := 1
	if vmt == SRVMMAGC {
		indexNum = 2
	}
	indexList, err := GetAvailableBrIndex(ctx, clnt, indexNum)
	if err != nil {
		return common.MakeErr(fmt.Errorf("failed to allocate bridge index, %w", err))
	}
	if !common.IsIntegratedChassis(*srvm.Chassis.Model) { //these are per distributed SR system NAD, only need one per system, so only CPM node creates them
		//check FB NAD
		fbnad := common.NewFBBridgeNetworkDef(lab.Lab.Namespace, lab.Lab.Name,
			common.GetVSROSFBName(lab.Lab.Name, nodeName), nodeName, *srvm.Chassis.Type,
			int(indexList[0]), SRVMFBMTU)
		err := createIfNotExistsOrRemove(ctx, clnt, lab, fbnad, true, forceRemoval)
		if err != nil {
			return common.MakeErr(err)
		}
		if vmt == SRVMMAGC {
			//MAG-c data fabric NAD
			dfnad := common.NewFBBridgeNetworkDef(lab.Lab.Namespace, lab.Lab.Name,
				common.GetMAGCDFName(lab.Lab.Name, nodeName), nodeName, *srvm.Chassis.Type,
				int(indexList[1]), SRVMFBMTU)
			err := createIfNotExistsOrRemove(ctx, clnt, lab, dfnad, true, forceRemoval)
			if err != nil {
				return common.MakeErr(err)
			}
		}
	}
	//per system operation (one time per system)
	if strings.HasPrefix(*srvm.Image, FTPImagePrefix) {
		//check sr release
		imgageSubFolder := strings.TrimPrefix(*srvm.Image, FTPImagePrefix)
		expectedTarget := filepath.Join("/"+common.KNLROOTName, common.IMGSubFolder, imgageSubFolder)
		vmlinkname := filepath.Join(common.KNLROOTName, common.GetFTPSROSImgPath(lab.Lab.Name, nodeName))
		curLinked, err := os.Readlink(vmlinkname)
		if err != nil || curLinked != expectedTarget {
			//create sr release folder
			err = common.ReCreateSymLink(lab.Lab.Name, nodeName, imgageSubFolder)
			if err != nil {
				return common.MakeErr(err)
			}
		}
	}
	//check sr cfg folder
	absPath := common.GetSRConfigFTPSubFolder(lab.Lab.Name, nodeName)
	if _, err := os.Stat(absPath); errors.Is(err, os.ErrNotExist) {
		//create the folder
		err = os.MkdirAll(absPath, 0755)
		if err != nil {
			return common.MakeErr(err)
		}
	}
	var licFullPath string
	for slot := range srvm.Chassis.Cards {
		if common.IsCPM(slot) {
			//get the lic
			licKey := types.NamespacedName{Namespace: MYNAMESPACE, Name: *srvm.License}
			licSec := new(corev1.Secret)
			err = clnt.Get(ctx, licKey, licSec)
			if err != nil {
				return fmt.Errorf("failed to read license secret %v, %w", *srvm.License, err)
			}
			licFolder := filepath.Join("/", common.KNLROOTName, common.LicSubFolder)
			err = os.MkdirAll(licFolder, 0755)
			if err != nil {
				return common.MakeErr(fmt.Errorf("failed to create lic sub folder, %w", err))
			}
			licFullPath = filepath.Join(licFolder, getSRVMLicFileName(lab.Lab.Name, nodeName))
			err = os.WriteFile(licFullPath, licSec.Data[SRVMLicSecretKeyName], 0644)
			if err != nil {
				return common.MakeErr(fmt.Errorf("failed to write license file, %w", err))
			}

			//SRVM CPM DV
			cpmImage := "docker://" + *srvm.Image
			diskSize := srvm.DiskSize
			if strings.HasPrefix(*srvm.Image, FTPImagePrefix) {
				cpmImage = *gconf.SRCPMLoaderImage
				diskSize = &SRCPMVMDiskSize
			}

			dv := common.NewDV(lab.Lab.Namespace, lab.Lab.Name,
				common.GetSRVMDVName(lab.Lab.Name, nodeName, slot),
				cpmImage, gconf.PVCStorageClass, diskSize)
			err = createIfNotExistsOrRemove(ctx, clnt, lab, dv, false, forceRemoval)
			if err != nil {
				return common.MakeErr(err)
			}
		}
		//get sftp credentials
		sftpUser := ""
		sftpPass := ""
		if strings.HasPrefix(*srvm.Image, FTPImagePrefix) {
			secKey := types.NamespacedName{Namespace: MYNAMESPACE, Name: KNLSftpCredentialSecret}
			sftpSec := new(corev1.Secret)
			err = clnt.Get(ctx, secKey, sftpSec)
			if err != nil {
				return common.MakeErr(err)
			}
			sftpUser = string(sftpSec.Data["username"])
			sftpPass = string(sftpSec.Data["password"])

		}
		//VMI
		vmi := srvm.getVMI(lab, nodeName, slot, licFullPath, sftpUser, sftpPass)
		err = createIfNotExistsOrFailedOrRemove(ctx, clnt, lab, vmi, checkVMIfail, true, forceRemoval)
		if err != nil {
			return common.MakeErr(err)
		}
	}
	return nil
}

func (srvm *SRVM) getVMI(lab *ParsedLab, chassisName, cardslot, licPath, sftpuser, sftppass string) *kvv1.VirtualMachineInstance {
	vmt, _ := ParseSRVMName_New(chassisName)
	gconf := GCONF.Get()
	isCPM := common.IsCPM(cardslot)
	r := new(kvv1.VirtualMachineInstance)
	r.ObjectMeta = common.GetObjMeta(
		getSRVMCardVMName(lab.Lab.Name, chassisName, cardslot),
		lab.Lab.Name,
		lab.Lab.Namespace,
		chassisName,
		*srvm.Chassis.Type,
	)

	r.ObjectMeta.Labels[vSROSIDLabel] = getFullQualifiedSRVMChassisName(lab.Lab.Name, chassisName)
	//add sysinfo for SR like node
	cfgURL := fmt.Sprintf("ftp://ftp:ftp@%v/cfg/config.cfg", common.FixedFTPProxySvr)
	//add ftp Path Map
	ftpPathMap := map[string]string{
		"/i386-boot.tim": fmt.Sprintf("%v/i386-boot.tim", common.GetSFTPSROSImgPath(lab.Lab.Name, chassisName)),
		"/i386-iom.tim":  fmt.Sprintf("%v/i386-iom.tim", common.GetSFTPSROSImgPath(lab.Lab.Name, chassisName)),
		"/sros":          common.GetSFTPSROSImgPath(lab.Lab.Name, chassisName),
		"/cfg":           common.GetSRConfigFTPSubFolder(lab.Lab.Name, chassisName),
		"/lic":           licPath,
	}
	pathMapBuf, err := json.Marshal(ftpPathMap)
	if err != nil {
		panic(err)
	}
	fixedLicLocalFTPURL := fmt.Sprintf("ftp://ftp:ftp@%v/lic", common.FixedFTPProxySvr)

	r.ObjectMeta.Annotations = map[string]string{
		dict.SftpSVRAnnontation:  *gconf.SFTPSever,
		dict.SftpPassAnnontation: sftppass,
		dict.SftpUserAnnontation: sftpuser,
		// dict.LabNameAnnotation:       lab.Lab.Name,
		// dict.ChassisNameAnnotation:   chassisName,
		// dict.ChassisTypeAnnotation:   string(*srvm.Chassis.Type),
		dict.FTPPathMapAnnotation:    string(pathMapBuf),
		dict.KvirtSideCarAnnontation: fmt.Sprintf(`[{"image": "%v"}]`, *gconf.SideCarHookImg),
		dict.VSROSSysinfoAnno:        common.GenSysinfo(*srvm.Chassis.Cards[cardslot].SysInfo, cfgURL, fixedLicLocalFTPURL),
	}

	//can't set pc here will be rejected by adminssion webhook

	r.Spec.Domain.CPU = &kvv1.CPU{
		Model: "host-passthrough",
	}
	//add UUID if specified
	if srvm.UUID != nil {
		r.Spec.Domain.Firmware = &kvv1.Firmware{
			UUID: types.UID(*srvm.UUID),
		}
	}

	//check if need pin CPU
	// if common.IsResourcePinNeededViaSysinfo(node.SRSysinfoStr) {
	dedicated := false

	switch vmt {
	case SRVMVSRI, SRVMMAGC:
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
	r.Spec.Domain.CPU.Cores = uint32(srvm.Chassis.Cards[cardslot].ReqCPU.AsApproximateFloat64()) //if the cpu is decimal, this round down to the int
	//NOTE: kubevirt currently doesn't support memory balloning, to save memory, see https://kubevirt.io/user-guide/operations/node_overcommit/#overcommit-guest-memory
	//NOTE: user could also set `spec.configuration.developerConfiguration.memoryOvercommit` in kubevirt CR
	r.Spec.Domain.Memory = &kvv1.Memory{
		Guest: srvm.Chassis.Cards[cardslot].ReqMemory,
	}
	//check if hugepage is needed
	if dedicated {
		r.Spec.Domain.Memory.Hugepages = &kvv1.Hugepages{
			PageSize: "1Gi",
		}
	}

	//for vsros node, disable auto console, this is needed since default unix socket console doesn't work for vsim
	switch vmt {
	case SRVMMAGC, SRVMVSIM, SRVMVSRI:
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
						Name: common.GetSRVMDVName(lab.Lab.Name, chassisName, cardslot),
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
		// iomImage := strings.TrimPrefix(*srvm.Image, "docker://")
		iomImage := *srvm.Image
		if strings.HasPrefix(*srvm.Image, FTPImagePrefix) {
			iomImage = *gconf.SRIOMLoaderImage
		}
		r.Spec.Volumes = append(r.Spec.Volumes,
			kvv1.Volume{
				Name: common.KNLROOTName,
				VolumeSource: kvv1.VolumeSource{
					ContainerDisk: &kvv1.ContainerDiskSource{
						Image: iomImage,
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
			Ports: *srvm.Chassis.Cards[cardslot].ListenPorts,
			InterfaceBindingMethod: kvv1.InterfaceBindingMethod{
				Masquerade: &kvv1.InterfaceMasquerade{},
			},
		})
	//fabric
	switch vmt {
	case SRVMVSIM, SRVMMAGC:
		if !common.IsIntegratedChassisViaSysinfo(*srvm.Chassis.Cards[cardslot].SysInfo) {
			//add fabric only if it is not integrated chassis
			r.Spec.Networks = append(r.Spec.Networks,
				kvv1.Network{
					Name: "fb-net",
					NetworkSource: kvv1.NetworkSource{
						Multus: &kvv1.MultusNetwork{
							NetworkName: common.GetVSROSFBName(lab.Lab.Name, chassisName),
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
		if vmt == SRVMMAGC && !isCPM {
			r.Spec.Networks = append(r.Spec.Networks,
				kvv1.Network{
					Name: "df-net",
					NetworkSource: kvv1.NetworkSource{
						Multus: &kvv1.MultusNetwork{
							NetworkName: common.GetMAGCDFName(lab.Lab.Name, chassisName),
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
	for _, spokes := range lab.SpokeMap[chassisName] {
		for _, spokeName := range spokes {
			if *lab.SpokeConnectorMap[spokeName].PortId != cardslot {
				continue
			}
			lanName := Getk8lanName(lab.Lab.Name, lab.SpokeLinkMap[spokeName])
			nadName := k8slan.GetNADName(lanName, spokeName, false)
			r.Spec.Networks = append(r.Spec.Networks,
				kvv1.Network{
					Name: spokeName,
					NetworkSource: kvv1.NetworkSource{
						Multus: &kvv1.MultusNetwork{
							NetworkName: nadName,
						},
					},
				})
			r.Spec.Domain.Devices.Interfaces = append(r.Spec.Domain.Devices.Interfaces,
				kvv1.Interface{
					Name: spokeName,
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
								Values:   []string{getFullQualifiedSRVMChassisName(lab.Lab.Name, chassisName)},
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
