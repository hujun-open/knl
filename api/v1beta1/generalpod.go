package v1beta1

import (
	"context"
	"fmt"
	"os"
	"strings"
	"syscall"

	k8slan "github.com/hujun-open/k8slan/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"kubenetlab.net/knl/common"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GeneralPod creates a general pod
type GeneralPod struct {
	Image      *string `json:"image,omitempty"`
	Command    *string `json:"cmd,omitempty"`
	Privileged *bool   `json:"privileged,omitempty"`
	// +optional
	// +nullable
	ReqMemory *resource.Quantity `json:"memory,omitempty"`
	// +optional
	// +nullable
	ReqCPU *resource.Quantity `json:"cpu,omitempty"`
}

func init() {
	common.NewSysRegistry[Pod] = func() common.System { return new(GeneralPod) }
}

const (
	Pod         common.NodeType = "pod"
	RootPVCSize string          = "100Mi"
)

func (gpod *GeneralPod) SetToAppDefVal() {

}

func (gpod *GeneralPod) FillDefaultVal(nodeName string) {

}

func (gpod *GeneralPod) GetNodeType(name string) common.NodeType {
	return Pod
}
func (gpod *GeneralPod) Validate() error {
	if gpod.Image == nil {
		return fmt.Errorf("image not specified")
	}
	return nil
}

func (gpod *GeneralPod) Ensure(ctx context.Context, nodeName string, clnt client.Client, forceRemoval bool) error {
	val := ctx.Value(ParsedLabKey)
	if val == nil {
		return common.MakeErr(fmt.Errorf("failed to get parsed lab obj from context"))
	}
	var lab *ParsedLab
	var ok bool
	if lab, ok = val.(*ParsedLab); !ok {
		return common.MakeErr(fmt.Errorf("context stored value is not a ParsedLabSpec"))
	}
	//create PVC
	rootPVC := gpod.getRootPVC(lab.Lab.Namespace, nodeName, lab.Lab.Name)
	err := createIfNotExistsOrRemove(ctx, clnt, lab, rootPVC, false, false)
	if err != nil {
		return fmt.Errorf("failed to create etc pvc for pod %v in lab %v, %w", nodeName, lab.Lab.Name, err)
	}
	//create pod
	pod := common.NewBasePod(lab.Lab.Name, nodeName, lab.Lab.Namespace, *gpod.Image)
	if gpod.Command != nil {
		pod.Spec.Containers[0].Command = strings.Fields(*gpod.Command)
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
	//refer to NADs
	netStr := ""
	pod.Spec.Containers[0].Resources.Limits = make(corev1.ResourceList)
	for _, spokes := range lab.SpokeMap[nodeName] {
		for _, spokeName := range spokes {
			nadName := k8slan.GetNADName(spokeName, true)
			netStr += fmt.Sprintf("%v,", nadName)
			resKey := fmt.Sprintf("%v/%v", K8sLANResKeyPrefix, nadName)
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

func (gpod *GeneralPod) getRootPVC(ns, nodeName, labName string) *corev1.PersistentVolumeClaim {
	gconf := GCONF.Get()
	name := fmt.Sprintf("%v-%v-root", labName, nodeName)
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: common.GetObjMeta(name, labName, ns),
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOncePod},
			StorageClassName: common.GetPointerVal(*gconf.PVCStorageClass),
			Resources: corev1.VolumeResourceRequirements{
				Requests: map[corev1.ResourceName]resource.Quantity{
					corev1.ResourceStorage: resource.MustParse(RootPVCSize),
				},
			},
		},
	}
}

func (gpod *GeneralPod) Shell(ctx context.Context, clnt client.Client, ns, lab, node, username string) {
	envList := []string{fmt.Sprintf("HOME=%v", os.Getenv("HOME"))}
	fmt.Println("connecting to %v", common.GetPodName(lab, node))
	syscall.Exec("/bin/sh",
		[]string{"sh", "-c",
			fmt.Sprintf("kubectl -n %v exec -it %v -- bash",
				ns, common.GetPodName(lab, node))},
		envList)

}
