package v1beta1

import (
	"context"
	"fmt"
	"net/netip"
	"os"
	"syscall"

	shlex "github.com/carapace-sh/carapace-shlex"
	k8slan "github.com/hujun-open/k8slan/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GeneralPod specifies a general k8s pod
type GeneralPod struct {
	//pod image
	Image *string `json:"image,omitempty"`
	//pod's command
	Command *string `json:"cmd,omitempty"`
	//privileged pod if true
	Privileged *bool `json:"privileged,omitempty"`
	//size of pvc mounted on /root
	PvcSize *resource.Quantity `json:"pvcSize,omitempty"`
	//requested memory in k8s resource unit
	// +optional
	// +nullable
	ReqMemory *resource.Quantity `json:"memory,omitempty"`
	//requested cpu in k8s resource unit
	// +optional
	// +nullable
	ReqCPU *resource.Quantity `json:"cpu,omitempty"`
}

func init() {
	NewSysRegistry[Pod] = func() System { return new(GeneralPod) }
}

const (
	Pod            NodeType = "pod"
	DefRootPVCSize string   = "100Mi"
)

func (gpod *GeneralPod) SetToAppDefVal() {
	gpod.PvcSize = ReturnPointerVal(resource.MustParse(DefRootPVCSize))
}

func (gpod *GeneralPod) FillDefaultVal(nodeName string) {

}

func (gpod *GeneralPod) GetNodeType(name string) NodeType {
	return Pod
}
func (gpod *GeneralPod) Validate(lab *LabSpec, nodeName string) error {
	if gpod.Image == nil {
		return fmt.Errorf("image not specified")
	}
	if gpod.PvcSize == nil {
		return fmt.Errorf("pvcSize not specified")
	}
	if gpod.Command != nil {
		_, err := shlex.Split(*gpod.Command)
		if err != nil {
			return fmt.Errorf("%v is not a valid shell command: %w", *gpod.Command, err)
		}
	}
	return nil
}

func (gpod *GeneralPod) Ensure(ctx context.Context, nodeName string, clnt client.Client, forceRemoval bool) error {
	val := ctx.Value(ParsedLabKey)
	if val == nil {
		return MakeErr(fmt.Errorf("failed to get parsed lab obj from context"))
	}
	var lab *ParsedLab
	var ok bool
	if lab, ok = val.(*ParsedLab); !ok {
		return MakeErr(fmt.Errorf("context stored value is not a ParsedLabSpec"))
	}
	//create PVC
	rootPVC := gpod.getRootPVC(lab.Lab.Namespace, nodeName, lab.Lab.Name, *gpod.PvcSize)
	err := createIfNotExistsOrRemove(ctx, clnt, lab, rootPVC, false, false)
	if err != nil {
		return fmt.Errorf("failed to create etc pvc for pod %v in lab %v, %w", nodeName, lab.Lab.Name, err)
	}
	//create pod
	pod := NewBasePod(lab.Lab.Name, nodeName, lab.Lab.Namespace, *gpod.Image, Pod)
	if gpod.Command != nil {
		tlist, _ := shlex.Split(*gpod.Command)
		pod.Spec.Containers[0].Command = tlist.Strings()
	}
	pod.Spec.Containers[0].SecurityContext = &corev1.SecurityContext{}
	pod.Spec.Containers[0].SecurityContext.Privileged = gpod.Privileged
	pod.Spec.Containers[0].VolumeMounts = []corev1.VolumeMount{
		{
			Name:      "root",
			MountPath: "/root",
		},
	}
	pod.Spec.Volumes = []corev1.Volume{
		{
			Name: "root",
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: rootPVC.Name,
					ReadOnly:  false,
				},
			},
		},
	}
	//generate NAD if the connector address is specified
	if linkSpokeMap, ok := lab.SpokeMap[nodeName]; ok {
		for _, spokes := range linkSpokeMap {
			for _, spokeName := range spokes {
				if lab.SpokeConnectorMap[spokeName].Addrs != nil {
					lanName := Getk8lanName(lab.Lab.Name, lab.SpokeLinkMap[spokeName])
					prefixList := []netip.Prefix{}
					for _, pstr := range lab.SpokeConnectorMap[spokeName].Addrs {
						prefixList = append(prefixList, netip.MustParsePrefix(pstr))
					}
					routeList := []k8slan.Route{}
					for _, rstr := range lab.SpokeConnectorMap[spokeName].Routes {
						route, _ := parseRoute(rstr)
						routeList = append(routeList, *route)
					}

					nad := k8slan.GenNAD(lanName, spokeName, MYNAMESPACE, true, prefixList, routeList)
					err = createIfNotExistsOrRemove(ctx, clnt, lab, nad, true, false)
					if err != nil {
						return fmt.Errorf("failed to create general pod nad %v in lab %v, %w", nodeName, lab.Lab.Name, err)
					}

				}
			}
		}
	}

	//refer to NADs
	netStr := ""
	pod.Spec.Containers[0].Resources.Limits = make(corev1.ResourceList)
	for _, linkName := range GetSortedKeySlice(lab.SpokeMap[nodeName]) {
		spokes := lab.SpokeMap[nodeName][linkName]
		// for _, spokes := range lab.SpokeMap[nodeName] {
		for _, spokeName := range spokes {
			lanName := Getk8lanName(lab.Lab.Name, lab.SpokeLinkMap[spokeName])

			nadName := k8slan.GetDefNADName(lanName, spokeName, true)
			if lab.SpokeConnectorMap[spokeName].Addrs != nil {
				nadName = k8slan.GetAddrNADName(lanName, spokeName)
			}
			if lab.SpokeConnectorMap[spokeName].PortId == nil {
				netStr += fmt.Sprintf("%v,", nadName)
			} else {
				netStr += fmt.Sprintf("%v@%v,", nadName, *lab.SpokeConnectorMap[spokeName].PortId)
			}
			resName := k8slan.GetDPResouceName(lanName, spokeName, true)
			resKey := fmt.Sprintf("%v/%v", K8sLANResKeyPrefix, resName)
			pod.Spec.Containers[0].Resources.Limits[corev1.ResourceName(resKey)] = resource.MustParse("1")
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
		return fmt.Errorf("failed to create general pod %v in lab %v, %w", nodeName, lab.Lab.Name, err)
	}
	return nil

}

func (gpod *GeneralPod) getRootPVC(ns, nodeName, labName string, size resource.Quantity) *corev1.PersistentVolumeClaim {
	gconf := GCONF.Get()
	name := fmt.Sprintf("%v-%v-root", labName, nodeName)
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: GetObjMeta(name, labName, ns, nodeName, Pod),
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOncePod},
			StorageClassName: GetPointerVal(*gconf.PVCStorageClass),
			Resources: corev1.VolumeResourceRequirements{
				Requests: map[corev1.ResourceName]resource.Quantity{
					corev1.ResourceStorage: size,
				},
			},
		},
	}
}

func (gpod *GeneralPod) Shell(ctx context.Context, clnt client.Client, ns, lab, node, username string) {
	envList := []string{fmt.Sprintf("HOME=%v", os.Getenv("HOME"))}
	fmt.Printf("connecting to %v\n", GetPodName(lab, node))
	syscall.Exec("/bin/sh",
		[]string{"sh", "-c",
			fmt.Sprintf("kubectl -n %v exec -it %v -- bash",
				ns, GetPodName(lab, node))},
		envList)

}

func (gpod *GeneralPod) Console(ctx context.Context, clnt client.Client, ns, lab, chassis string) {
	gpod.Shell(ctx, clnt, ns, lab, chassis, "")
}
