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

// getSRLChassisViaTypeStr expect two types of chassisType:
// 1. offical product model name like ixr-6
// 2. a string include all hardware ID: chassis_type-base_mac-cpm-slot-iom-mda
func getSRLChassisViaTypeStr(chassisType string) (*SRLChassis, error) {
	switch strings.ToLower(chassisType) {
	case "ixr-h5-32d":
		return &SRLChassis{
			ChassisConfiguration: chassisConfiguration{
				ChassisType: 47,
				CPM:         180,
			},
			SlotConfig: map[int]slotConfig{
				1: {
					CardType: 180,
					MDAType:  106,
				},
			},
		}, nil
	case "ixr-6":
		return &SRLChassis{
			ChassisConfiguration: chassisConfiguration{
				ChassisType: 42,
				CPM:         69,
			},
			SlotConfig: map[int]slotConfig{
				1: {
					CardType: 127,
					MDAType:  36,
				},
			},
		}, nil
	case "ixr-6e":
		return &SRLChassis{
			ChassisConfiguration: chassisConfiguration{
				ChassisType: 68,
				CPM:         184,
			},
			SlotConfig: map[int]slotConfig{
				1: {
					CardType: 182,
					MDAType:  199,
				},
			},
		}, nil
	default:
		return parseChassisStr(chassisType)
	}
}

/*
SRLinux creates a pod for SRL:
- the specified Chassis will creates a configmap and mounted as /tmp/topology.yml
- the specified LicSecrete reference to a secret, mounted as /opt/srlinux/etc/license.key
- create a PVC mount on /etc/opt/srlinux/checkpoint for persistent configuration
*/
type SRLinux struct {
	Image     *string `json:"image,omitempty"`
	Chassis   *string `json:"chassis,omitempty"`
	LicSecret *string `json:"license,omitempty"`
	// +optional
	// +nullable
	ReqMemory *resource.Quantity `json:"memory,omitempty"`
	// +optional
	// +nullable
	ReqCPU *resource.Quantity `json:"cpu,omitempty"`
}

func (gpod *SRLinux) SetToAppDefVal() {
	gpod.Chassis = common.ReturnPointerVal("ixr-h5-32d")
	gpod.ReqMemory = common.ReturnPointerVal(resource.MustParse(DefaultSRLMem))
}

func (gpod *SRLinux) FillDefaultVal(nodeName string) {

}

func (gpod *SRLinux) GetNodeType(name string) common.NodeType {
	return SRL
}

func (gpod *SRLinux) Validate() error {
	if gpod.Image == nil {
		return fmt.Errorf("image not specified")
	}
	if gpod.Chassis == nil {
		return fmt.Errorf("chassis not specified")
	}

	//check chassis config
	_, err := getSRLChassisViaTypeStr(*gpod.Chassis)

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

func (gpod *SRLinux) getEtcPVC(ns, nodeName, labName string) *corev1.PersistentVolumeClaim {
	gconf := GCONF.Get()
	name := fmt.Sprintf("%v-%v-etc", labName, nodeName)
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: common.GetObjMeta(name, labName, ns),
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

func (gpod *SRLinux) getConfigMapFromSRLChassis(ns, nodeName, labName string, nodeIndex int) *corev1.ConfigMap {
	name := fmt.Sprintf("%v-%v-topo", labName, nodeName)

	chassis, err := getSRLChassisViaTypeStr(*gpod.Chassis)
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
		ObjectMeta: common.GetObjMeta(name, labName, ns),
		Data: map[string]string{
			"topology.yml": string(buf),
		},
	}
}

func (gpod *SRLinux) Ensure(ctx context.Context, nodeName string, clnt client.Client, forceRemoval bool) error {
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
	topoCM := gpod.getConfigMapFromSRLChassis(lab.Lab.Namespace, nodeName, lab.Lab.Name, lab.Lab.Spec.GetNodeSortIndex(nodeName))
	err := createIfNotExistsOrRemove(ctx, clnt, lab, topoCM, true, false)
	if err != nil {
		return fmt.Errorf("failed to create topology configmap for SRL %v in lab %v, %w", nodeName, lab.Lab.Name, err)
	}
	//create PVC for etc
	etcPVC := gpod.getEtcPVC(lab.Lab.Namespace, nodeName, lab.Lab.Name)
	err = createIfNotExistsOrRemove(ctx, clnt, lab, etcPVC, false, false)
	if err != nil {
		return fmt.Errorf("failed to create etc pvc for SRL %v in lab %v, %w", nodeName, lab.Lab.Name, err)
	}

	//create pod
	pod := common.NewBasePod(lab.Lab.Name, nodeName, lab.Lab.Namespace, *gpod.Image)
	pod.Spec.Containers[0].Command = []string{"/tini", "--", "fixuid", "-q", "/entrypoint.sh", "sudo", "bash", "/opt/srlinux/bin/sr_linux"}
	pod.Spec.Containers[0].SecurityContext = &corev1.SecurityContext{}
	pod.Spec.Containers[0].SecurityContext.Privileged = common.ReturnPointerVal(true)
	pod.Spec.Containers[0].VolumeMounts = []corev1.VolumeMount{
		{
			Name:      "topo",
			MountPath: "/tmp/topology.yml",
			SubPath:   "topology.yml",
		},
		{ //this is to persistent configuration, can't mount to /etc/opt/srlinux/ directly due to filesystem ACL requirement, will cause linux_mgr to crash
			Name:      "etc",
			MountPath: "/etc/opt/srlinux/checkpoint",
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
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: etcPVC.Name,
					ReadOnly:  false,
				},
			},
		},
	}
	//add lic if specified
	if gpod.LicSecret != nil {
		pod.Spec.Containers[0].VolumeMounts = append(pod.Spec.Containers[0].VolumeMounts, corev1.VolumeMount{
			Name:      "lic",
			MountPath: "/opt/srlinux/etc/license.key",
			SubPath:   "license.key",
		})
		pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{
			Name: "lic",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: *gpod.LicSecret,
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
			nadName := k8slan.GetNADName(spokeName, true)
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
	if gpod.ReqCPU != nil {
		pod.Spec.Containers[0].Resources.Requests[corev1.ResourceCPU] = *gpod.ReqCPU
	}
	if gpod.ReqMemory != nil {
		pod.Spec.Containers[0].Resources.Requests[corev1.ResourceMemory] = *gpod.ReqMemory
	}

	err = createIfNotExistsOrRemove(ctx, clnt, lab, pod, true, false)
	if err != nil {
		return fmt.Errorf("failed to create SRL pod %v in lab %v, %w", nodeName, lab.Lab.Name, err)
	}
	return nil

}

func (srsim *SRLinux) Shell(ctx context.Context, clnt client.Client, ns, lab, chassis, username string) {
	pod := &corev1.Pod{}
	podKey := types.NamespacedName{Namespace: ns, Name: common.GetPodName(lab, chassis)}
	err := clnt.Get(ctx, podKey, pod)
	if err != nil {
		log.Fatalf("failed to list pods: %v", err)
	}
	envList := []string{fmt.Sprintf("HOME=%v", os.Getenv("HOME"))}
	if username == "" {
		username = "admin"
	}
	fmt.Println("connecting to", chassis, "at", pod.Status.PodIP, "username", username)
	syscall.Exec("/bin/sh",
		[]string{"sh", "-c",
			fmt.Sprintf("ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null %v@%v", username, pod.Status.PodIP)},
		envList)

}
