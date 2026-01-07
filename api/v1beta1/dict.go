package v1beta1

const (
	VSROSSysinfoAnno         = `smbios.vm.kubevirt.io/vSROSSysinfo`
	SftpSVRAnnontation       = "kubenetlab.net/sftpsvr"
	SftpUserAnnontation      = "kubenetlab.net/sftpuser"
	SftpPassAnnontation      = "kubenetlab.net/sftppasswd"
	LabNameAnnotation        = "lab.kubenetlab.net/name"
	ChassisNameAnnotation    = "chassis.kubenetlab.net/name"
	ChassisTypeAnnotation    = "chassis.kubenetlab.net/type"
	FTPPathMapAnnotation     = "kubenetlab.net/ftppathmap" //json of map[string]string
	KvirtSideCarAnnontation  = "hooks.kubevirt.io/hookSidecars"
	K8SLABELAPPVAL           = `kubenetlab`
	K8SLABELAPPKey           = `app.kubernetes.io/name`
	K8SLABELSETUPKEY         = `lab.kubenetlab.net/name`
	K8SLABELNodeKEY          = `node.kubenetlab.net/name`
	BridgeIndexLabelKey      = "bridge.kubenetlab.net/index"
	KNLROOTName              = `knlroot`
	VMDiskSubFolder          = `vmdisks`
	IMGSubFolder             = `imgs`
	LicSubFolder             = `lic`
	CfgSubFolder             = `cfgs`
	KVirtPodPVCMountRoot     = `/var/run/kubevirt-private/vmi-disks/`
	PVCName                  = `knl-pvc` //this must be inline with config/default/pvc
	macVTAPResourceKey       = `k8s.v1.cni.cncf.io/resourceName`
	macVTAPResourceValPrefix = `macvtap.network.kubevirt.io`
	NADAnnonKey              = `k8s.v1.cni.cncf.io/networks`
)
