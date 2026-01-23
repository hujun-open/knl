package v1beta1

import (
	"context"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"syscall"

	k8slan "github.com/hujun-open/k8slan/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func init() {
	NewSysRegistry[SRSIM] = func() System { return new(SRSim) }
}

// SRSIM creates a Nokia SR-SIM;
// note: it is important to set `tx-checksum-ip-generic` off in corresponding bridge interface, otherwise IP traffic toward management interface won't work
// in kind, it is docker bridge;
// in general k8s, it is cni0 bridge in each worker;
// "ethtool -K <interface> tx-checksum-ip-generic off"
// see SR-SIM installation guide for details
type SRSim struct {
	//Docker image
	// +optional
	// +nullable
	Image *string `json:"image,omitempty"`
	// specifies the chassis configuration
	// +optional
	// +nullable
	Chassis *SRChassis `json:"chassis,omitempty"`
	//name of k8s secret contains license file with "license" as the key
	// +optional
	// +nullable
	LicSecret *string `json:"license,omitempty"`
}

const (
	SRSIM  NodeType = "srsim"
	CFSize string   = "64Mi"
)

func (srsim *SRSim) SetToAppDefVal() {
	srsim.Chassis = DefaultSIMChassis(SRSIM)
}

func (srsim *SRSim) FillDefaultVal(nodeName string) {
	srsim.Chassis.Type = ReturnPointerVal(SRSIM)
}

func (srsim *SRSim) GetNodeType(name string) NodeType {
	return SRSIM
}

func (srsim *SRSim) Validate(lab *LabSpec, nodeName string) error {
	if srsim.Image == nil {
		return fmt.Errorf("image not specified")
	}
	if srsim.Chassis == nil {
		return fmt.Errorf("chassis not specified")
	}
	if srsim.LicSecret == nil {
		return fmt.Errorf("license secret not specified")
	}
	re := regexp.MustCompile(`^e\d{1,2}((-[a-z]\d{1,2})?-\d{1,2}){1,2}$`)
	for linkName, link := range lab.LinkList {
		for _, c := range link.Connectors {
			if *c.NodeName != nodeName {
				continue
			}
			if c.PortId != nil {
				if !re.MatchString(*c.PortId) {
					return fmt.Errorf("invlid port id %v in link %v", *c.PortId, linkName)
				}
			}
		}
	}
	return srsim.Chassis.Validate()
}

func (srsim *SRSim) getCFPVC(ns, nodeName, labName, slot string, id int) *corev1.PersistentVolumeClaim {
	gconf := GCONF.Get()

	return &corev1.PersistentVolumeClaim{
		ObjectMeta: GetObjMeta(srsim.getCFPVCName(nodeName, labName, slot, id), labName, ns, nodeName, SRSIM),
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOncePod},
			StorageClassName: GetPointerVal(*gconf.PVCStorageClass),
			Resources: corev1.VolumeResourceRequirements{
				Requests: map[corev1.ResourceName]resource.Quantity{
					corev1.ResourceStorage: resource.MustParse(CFSize),
				},
			},
		},
	}
}

func (srsim *SRSim) getCFPVCName(nodeName, labName, slot string, id int) string {
	return strings.ToLower(fmt.Sprintf("%v-%v-cf-%v-%d", labName, nodeName, slot, id))
}

func (srsim *SRSim) Ensure(ctx context.Context, nodeName string, clnt client.Client, forceRemoval bool) error {
	// gconf := GCONF.Get()
	val := ctx.Value(ParsedLabKey)
	if val == nil {
		return MakeErr(fmt.Errorf("failed to get parsed lab obj from context"))
	}
	var lab *ParsedLab
	var ok bool
	if lab, ok = val.(*ParsedLab); !ok {
		return MakeErr(fmt.Errorf("context stored value is not a ParsedLabSpec"))
	}

	//create pod
	pod := NewBasePod(lab.Lab.Name, nodeName, lab.Lab.Namespace, *srsim.Image, SRSIM)
	pod.Spec.Containers = []corev1.Container{}
	for slotid, card := range srsim.Chassis.Cards {
		container := corev1.Container{
			Name:  strings.ToLower("slot-" + slotid),
			Image: *srsim.Image,
			SecurityContext: &corev1.SecurityContext{
				Privileged: ReturnPointerVal(true),
			},
		}
		//license file
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
			Name:      "lic",
			MountPath: "/nokia/license/license.txt",
			SubPath:   "license",
		})
		//envs
		container.Env = []corev1.EnvVar{
			{
				Name:  "NOKIA_SROS_CHASSIS",
				Value: *srsim.Chassis.Model,
			},
			{
				Name:  "NOKIA_SROS_SLOT",
				Value: slotid,
			},
			{
				Name:  "NOKIA_SROS_CARD",
				Value: *card.Model,
			},
		}
		if srsim.Chassis.SFM != nil {
			if strings.TrimSpace(*srsim.Chassis.SFM) != "" {
				container.Env = append(container.Env, corev1.EnvVar{
					Name:  "NOKIA_SROS_SFM",
					Value: *srsim.Chassis.SFM,
				})
			}
		}

		if len(card.XIOM) > 0 {
			for xiomslot, xiom := range card.XIOM {
				container.Env = append(container.Env, corev1.EnvVar{
					Name:  fmt.Sprintf("NOKIA_SROS_XIOM_%v", xiomslot),
					Value: *xiom.Model,
				})
				for k, mda := range xiom.MDAs {
					container.Env = append(container.Env, corev1.EnvVar{
						Name:  fmt.Sprintf("NOKIA_SROS_MDA_%v_%d", xiomslot, k+1),
						Value: mda,
					})
				}

			}
		}
		if card.MDAs != nil {
			for k, mda := range *card.MDAs {
				container.Env = append(container.Env, corev1.EnvVar{
					Name:  fmt.Sprintf("NOKIA_SROS_MDA_%d", k+1),
					Value: mda,
				})
			}
		}

		if IsCPM(slotid) {
			//cpm
			// chassis mac
			if srsim.Chassis.ChassisMAC != nil {
				container.Env = append(container.Env, corev1.EnvVar{
					Name:  "NOKIA_SROS_SYSTEM_BASE_MAC",
					Value: *srsim.Chassis.ChassisMAC,
				})
			}
			//cf cards
			for i := 1; i <= 3; i++ {
				cfPVC := srsim.getCFPVC(lab.Lab.Namespace, nodeName, lab.Lab.Name, slotid, i)
				err := createIfNotExistsOrRemove(ctx, clnt, lab, cfPVC, false, false)
				if err != nil {
					return fmt.Errorf("failed to create cf card %d pvc for SRSIM %v in lab %v, %w", i, nodeName, lab.Lab.Name, err)
				}
				container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
					Name:      cfPVC.Name,
					MountPath: fmt.Sprintf("/cf%d", i),
				})
				pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{
					Name: cfPVC.Name,
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: cfPVC.Name,
							ReadOnly:  false,
						},
					},
				})

			}

		}
		//add resource request
		container.Resources.Requests = make(corev1.ResourceList)
		if card.ReqCPU != nil {
			container.Resources.Requests[corev1.ResourceCPU] = *card.ReqCPU
		}
		if card.ReqMemory != nil {
			container.Resources.Requests[corev1.ResourceMemory] = *card.ReqMemory
		}

		pod.Spec.Containers = append(pod.Spec.Containers, container)
	}
	//volumes
	pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{
		Name: "lic",
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: *srsim.LicSecret,
			},
		},
	})

	//refer to NADs
	netStr := ""
	i := 1
	pod.Spec.Containers[0].Resources.Limits = make(corev1.ResourceList)
	for _, linkName := range GetSortedKeySlice(lab.SpokeMap[nodeName]) {
		spokes := lab.SpokeMap[nodeName][linkName]
		// for _, spokes := range lab.SpokeMap[nodeName] {
		for _, spokeName := range spokes {
			lanName := Getk8lanName(lab.Lab.Name, lab.SpokeLinkMap[spokeName])
			nadName := k8slan.GetDefNADName(lanName, spokeName, true)
			if lab.SpokeConnectorMap[spokeName].PortId != nil {
				netStr += fmt.Sprintf("%v@%v,", nadName, *(lab.SpokeConnectorMap[spokeName].PortId))
			} else {
				//if port is not specified, default to mda 1 of first IOM slot
				netStr += fmt.Sprintf("%v@e%v-1-%d,", nadName, srsim.Chassis.GetDefaultMDASlot(), i)
			}
			resName := k8slan.GetDPResouceName(lanName, spokeName, true)
			resKey := fmt.Sprintf("%v/%v", K8sLANResKeyPrefix, resName)
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
	err := createIfNotExistsOrRemove(ctx, clnt, lab, pod, true, false)
	if err != nil {
		return fmt.Errorf("failed to create SRL pod %v in lab %v, %w", nodeName, lab.Lab.Name, err)
	}
	return nil
}

func (srsim *SRSim) Shell(ctx context.Context, clnt client.Client, ns, lab, chassis, username string) {
	pod := &corev1.Pod{}
	podKey := types.NamespacedName{Namespace: ns, Name: GetPodName(lab, chassis)}
	err := clnt.Get(ctx, podKey, pod)
	if err != nil {
		log.Fatalf("failed to list pods: %v", err)
	}
	if username == "" {
		username = "admin"
	}
	fmt.Println("connecting to", chassis, "at", pod.Status.PodIP, "username", username)
	SysCallSSH(username, pod.Status.PodIP)

}

func (srsim *SRSim) Console(ctx context.Context, clnt client.Client, ns, lab, chassis string) {
	envList := []string{fmt.Sprintf("HOME=%v", os.Getenv("HOME"))}
	fmt.Printf("connecting to %v\n", GetPodName(lab, chassis))
	syscall.Exec("/bin/sh",
		[]string{"sh", "-c",
			fmt.Sprintf("kubectl -n %v exec -it %v -- bash",
				ns, GetPodName(lab, chassis))},
		envList)
}
