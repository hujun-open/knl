package v1beta1

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"syscall"

	"github.com/goccy/go-yaml"
	k8slan "github.com/hujun-open/k8slan/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"kubenetlab.net/knl/common"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func init() {
	common.NewSysRegistry[SRL] = func() common.System { return new(SRLinux) }
}

const (
	SRL           common.NodeType = "srl"
	DefaultSRLMem string          = "4Gi"
	BaseMACPrefix string          = "FA:FA"
	EtcPVCSize    string          = "100Mi"
)

// chassis_type-base_mac-cpm-slot-iom-mda
func parseChassisStr(chassis string) (*SRLChassis, error) {
	flist := strings.FieldsFunc(chassis, func(c rune) bool { return c == '-' })
	const expectedFieldNum = 6
	if len(flist) != expectedFieldNum {
		return nil, fmt.Errorf("expect %d fields in SRL chassis %v, but got %d fields", expectedFieldNum, chassis, len(flist))

	}
	r := new(SRLChassis)
	var err error
	r.ChassisConfiguration.ChassisType, err = strconv.Atoi(flist[0])
	if err != nil {
		return nil, fmt.Errorf("invlalid chassis type id %v, %w", flist[0], err)
	}
	r.ChassisConfiguration.CPM, err = strconv.Atoi(flist[2])
	if err != nil {
		return nil, fmt.Errorf("invlalid cpm type id %v, %w", flist[2], err)
	}

	var slotId int
	slotId, err = strconv.Atoi(flist[3])
	if err != nil {
		return nil, fmt.Errorf("invalid slot id %v, %w", flist[3], err)
	}
	r.SlotConfig = make(map[int]slotConfig)
	slotCfg := slotConfig{}
	slotCfg.CardType, err = strconv.Atoi(flist[4])
	if err != nil {
		return nil, fmt.Errorf("invalid IOM id %v, %w", flist[4], err)
	}

	slotCfg.MDAType, err = strconv.Atoi(flist[5])
	if err != nil {
		return nil, fmt.Errorf("invalid MDA id %v, %w", flist[5], err)
	}
	_, err = net.ParseMAC(flist[1])
	if err != nil {
		return nil, fmt.Errorf("invalid base mac %v, %w", flist[1], err)
	}
	r.ChassisConfiguration.BaseMAC = flist[1]
	r.SlotConfig[slotId] = slotCfg
	return r, nil
}

func getBaseMAC(i int) string {
	return fmt.Sprintf("%v:%X:00:00:00", BaseMACPrefix, i)
}

// getSRLChassisViaTypeStrDict retrun chassis configuration via offical product name defined in chassisDict
func getSRLChassisViaTypeStrDict(chassisType string) *SRLChassis {
	chassisType = strings.ToLower(chassisType)
	if nums, ok := chassisDict[chassisType]; ok {
		return &SRLChassis{
			ChassisConfiguration: chassisConfiguration{
				ChassisType: nums[0],
				CPM:         nums[1],
			},
			SlotConfig: map[int]slotConfig{
				1: {
					CardType: nums[2],
					MDAType:  nums[3],
				},
			},
		}
	}
	return nil
}

// getSRLChassisViaTypeStr expect two types of chassisType:
// 1. offical product model name like ixr-6
// 2. a string include all hardware ID: chassis_type-base_mac-cpm-slot-iom-mda
func getSRLChassisViaTypeStr(chassisType string) (*SRLChassis, error) {
	if chassis := getSRLChassisViaTypeStrDict(chassisType); chassis != nil {
		return chassis, nil
	}
	//not a predefined chassis type, use hardware number instead
	return parseChassisStr(chassisType)

}

/*
SRLinux specifies a Nokia SRLinux chassis;
*/
type SRLinux struct {
	//SRLinux container image
	Image *string `json:"image,omitempty"`
	//chassis model
	Chassis *string `json:"chassis,omitempty"`
	//a k8s secret contains the license file with "license" as the key
	LicSecret *string `json:"license,omitempty"`
	//requested memory in k8s resource unit
	// +optional
	// +nullable
	ReqMemory *resource.Quantity `json:"memory,omitempty"`
	//requested cpu in k8s resource unit
	// +optional
	// +nullable
	ReqCPU *resource.Quantity `json:"cpu,omitempty"`
}

func (srl *SRLinux) SetToAppDefVal() {
	srl.Chassis = common.ReturnPointerVal("ixr-d3l")
	srl.ReqMemory = common.ReturnPointerVal(resource.MustParse(DefaultSRLMem))
}

func (srl *SRLinux) FillDefaultVal(nodeName string) {

}

func (srl *SRLinux) GetNodeType(name string) common.NodeType {
	return SRL
}

func (srl *SRLinux) Validate() error {
	if srl.Image == nil {
		return fmt.Errorf("image not specified")
	}
	if srl.Chassis == nil {
		return fmt.Errorf("chassis not specified")
	}

	//check chassis config
	_, err := getSRLChassisViaTypeStr(*srl.Chassis)

	return err
}

// This is the struct used to marshal into topology.yml
type chassisConfiguration struct {
	ChassisType int    `yaml:"chassis_type"`
	BaseMAC     string `yaml:"base_mac,omitempty"`
	CPM         int    `yaml:"cpm_card_type"`
}

type slotConfig struct {
	CardType int `yaml:"card_type"`
	MDAType  int `yaml:"mda_type"`
}

type SRLChassis struct {
	ChassisConfiguration chassisConfiguration `yaml:"chassis_configuration"`
	SlotConfig           map[int]slotConfig   `yaml:"slot_configuration"`
}

func (srl *SRLinux) getEtcPVC(ns, nodeName, labName string) *corev1.PersistentVolumeClaim {
	gconf := GCONF.Get()
	name := fmt.Sprintf("%v-%v-etc", labName, nodeName)
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: common.GetObjMeta(name, labName, ns, nodeName, SRL),
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOncePod},
			StorageClassName: common.GetPointerVal(*gconf.PVCStorageClass),
			Resources: corev1.VolumeResourceRequirements{
				Requests: map[corev1.ResourceName]resource.Quantity{
					corev1.ResourceStorage: resource.MustParse(EtcPVCSize),
				},
			},
		},
	}
}

func (srl *SRLinux) getConfigMapFromSRLChassis(ns, nodeName, labName string, nodeIndex int) *corev1.ConfigMap {
	name := fmt.Sprintf("%v-%v-topo", labName, nodeName)

	chassis, err := getSRLChassisViaTypeStr(*srl.Chassis)
	if err != nil {
		//given srl.Chassis has been validated, err happened is unexpected, so use panic
		panic(err)
	}
	chassis.ChassisConfiguration.BaseMAC = getBaseMAC(nodeIndex)
	buf, err := yaml.Marshal(chassis)
	if err != nil {
		panic(err)
	}
	return &corev1.ConfigMap{
		ObjectMeta: common.GetObjMeta(name, labName, ns, nodeName, SRL),
		Data: map[string]string{
			"topology.yml": string(buf),
		},
	}
}

func (srl *SRLinux) Ensure(ctx context.Context, nodeName string, clnt client.Client, forceRemoval bool) error {
	// gconf := GCONF.Get()
	val := ctx.Value(ParsedLabKey)
	if val == nil {
		return common.MakeErr(fmt.Errorf("failed to get parsed lab obj from context"))
	}
	var lab *ParsedLab
	var ok bool
	if lab, ok = val.(*ParsedLab); !ok {
		return common.MakeErr(fmt.Errorf("context stored value is not a ParsedLabSpec"))
	}
	//create configmap
	topoCM := srl.getConfigMapFromSRLChassis(lab.Lab.Namespace, nodeName, lab.Lab.Name, lab.Lab.Spec.GetNodeSortIndex(nodeName))
	err := createIfNotExistsOrRemove(ctx, clnt, lab, topoCM, true, false)
	if err != nil {
		return fmt.Errorf("failed to create topology configmap for SRL %v in lab %v, %w", nodeName, lab.Lab.Name, err)
	}
	//create PVC for etc
	etcPVC := srl.getEtcPVC(lab.Lab.Namespace, nodeName, lab.Lab.Name)
	err = createIfNotExistsOrRemove(ctx, clnt, lab, etcPVC, false, false)
	if err != nil {
		return fmt.Errorf("failed to create etc pvc for SRL %v in lab %v, %w", nodeName, lab.Lab.Name, err)
	}

	//create pod
	pod := common.NewBasePod(lab.Lab.Name, nodeName, lab.Lab.Namespace, *srl.Image, SRL)
	//create init-container to sync file from pvc to emptydir

	initContainer := corev1.Container{
		Name:    "sync-srl-etc",
		Image:   *srl.Image,
		Command: []string{"sh", "-c", "rm -rf /etc/opt/srlinux/*; rsync -rptgoD /persis-etc/ /etc/opt/srlinux/"},
		SecurityContext: &corev1.SecurityContext{
			Privileged: common.ReturnPointerVal(true),
			RunAsUser:  common.ReturnPointerVal(int64(0)),
			RunAsGroup: common.ReturnPointerVal(int64(0)),
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "persis-etc",
				MountPath: "/persis-etc/",
			},
			{
				Name:      "etc",
				MountPath: "/etc/opt/srlinux/",
			},
		},
	}
	pod.Spec.InitContainers = append(pod.Spec.InitContainers, initContainer)
	pod.Spec.Containers[0].Command = []string{"/tini", "--", "fixuid", "-q", "/entrypoint.sh", "sudo", "bash", "/opt/srlinux/bin/sr_linux"}
	pod.Spec.Containers[0].SecurityContext = &corev1.SecurityContext{
		Privileged: common.ReturnPointerVal(true),
	}
	pod.Spec.Containers[0].VolumeMounts = []corev1.VolumeMount{
		{
			Name:      "topo",
			MountPath: "/tmp/topology.yml",
			SubPath:   "topology.yml",
		},
		{ //this is emptyDir volume
			Name:      "etc",
			MountPath: "/etc/opt/srlinux/",
		},
		{ //this is a pvc, use to store persitent file to/from emptyDir volume
			Name:      "persis-etc",
			MountPath: "/persis-etc/",
		},
	}
	//add pre-stop hook to sync files back to pvc
	pod.Spec.Containers[0].Lifecycle = &corev1.Lifecycle{
		PreStop: &corev1.LifecycleHandler{
			Exec: &corev1.ExecAction{
				Command: []string{"sh", "-c", "rm -rf /persis-etc/*;rsync -rptgoD /etc/opt/srlinux/ /persis-etc/"},
			},
		},
	}
	pod.Spec.Volumes = []corev1.Volume{
		{
			Name: "topo",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: topoCM.Name,
					},
				},
			},
		},
		{
			Name: "etc",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{
					SizeLimit: common.ReturnPointerVal(resource.MustParse(EtcPVCSize)),
				},
			},
		},
		{
			Name: "persis-etc",
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: etcPVC.Name,
					ReadOnly:  false,
				},
			},
		},
	}
	//add lic if specified
	if srl.LicSecret != nil {
		pod.Spec.Containers[0].VolumeMounts = append(pod.Spec.Containers[0].VolumeMounts, corev1.VolumeMount{
			Name:      "lic",
			MountPath: "/opt/srlinux/etc/license.key",
			SubPath:   "license",
		})
		pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{
			Name: "lic",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: *srl.LicSecret,
				},
			},
		})

	}
	//refer to NADs
	netStr := ""
	i := 1
	pod.Spec.Containers[0].Resources.Limits = make(corev1.ResourceList)
	for _, spokes := range lab.SpokeMap[nodeName] {
		for _, spokeName := range spokes {
			lanName := Getk8lanName(lab.Lab.Name, lab.SpokeLinkMap[spokeName])
			nadName := k8slan.GetNADName(lanName, spokeName, true)
			netStr += fmt.Sprintf("%v@e1-%d,", nadName, i)
			resKey := fmt.Sprintf("%v/%v", K8sLANResKeyPrefix, nadName)
			pod.Spec.Containers[0].Resources.Limits[corev1.ResourceName(resKey)] = resource.MustParse("1")
			i += 1
		}

	}
	if netStr != "" {
		netStr = netStr[:len(netStr)-1]
		pod.Annotations = map[string]string{
			MultusAnnoKey: netStr,
		}
	}
	//add resource request
	pod.Spec.Containers[0].Resources.Requests = make(corev1.ResourceList)
	if srl.ReqCPU != nil {
		pod.Spec.Containers[0].Resources.Requests[corev1.ResourceCPU] = *srl.ReqCPU
	}
	if srl.ReqMemory != nil {
		pod.Spec.Containers[0].Resources.Requests[corev1.ResourceMemory] = *srl.ReqMemory
	}

	err = createIfNotExistsOrRemove(ctx, clnt, lab, pod, true, false)
	if err != nil {
		return fmt.Errorf("failed to create SRL pod %v in lab %v, %w", nodeName, lab.Lab.Name, err)
	}
	return nil

}

func (srl *SRLinux) Shell(ctx context.Context, clnt client.Client, ns, lab, chassis, username string) {
	pod := &corev1.Pod{}
	podKey := types.NamespacedName{Namespace: ns, Name: common.GetPodName(lab, chassis)}
	err := clnt.Get(ctx, podKey, pod)
	if err != nil {
		log.Fatalf("failed to list pods: %v", err)
	}
	if username == "" {
		username = "admin"
	}
	fmt.Println("connecting to", chassis, "at", pod.Status.PodIP, "username", username)
	common.SysCallSSH(username, pod.Status.PodIP)
}

func (srl *SRLinux) Console(ctx context.Context, clnt client.Client, ns, lab, chassis string) {
	envList := []string{fmt.Sprintf("HOME=%v", os.Getenv("HOME"))}
	fmt.Printf("connecting to %v\n", common.GetPodName(lab, chassis))
	syscall.Exec("/bin/sh",
		[]string{"sh", "-c",
			fmt.Sprintf("kubectl -n %v exec -it %v -- bash",
				ns, common.GetPodName(lab, chassis))},
		envList)
}
