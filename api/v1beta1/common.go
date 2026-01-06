package v1beta1

import (
	"fmt"
	"strconv"

	ncv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"kubenetlab.net/knl/common"
	"kubenetlab.net/knl/dict"
)

// This function is used for all objects created by operator,
// if the object is not node specific, then nodeName and nodeType is empty
func GetObjMeta(objName, labName, labNS, nodeName string, nodeType NodeType) metav1.ObjectMeta {
	r := metav1.ObjectMeta{
		Name:      objName,
		Namespace: labNS,
		Labels: map[string]string{
			common.K8SLABELAPPKey:   common.K8SLABELAPPVAL,
			common.K8SLABELSETUPKEY: labName,
		},
	}
	if nodeName != "" {
		r.Labels[dict.ChassisNameAnnotation] = nodeName
		r.Labels[dict.ChassisTypeAnnotation] = string(nodeType)
	}
	return r
}

func NewFBBridgeNetworkDef(nsName, labName, brName, nodeName string, nodeType NodeType, brIndex, mtu int) *ncv1.NetworkAttachmentDefinition {
	const specTempalte = `
	{
		"cniVersion": "0.3.1",
		"name": "%v",
		"type": "bridge",
		"mtu": %d,
		"bridge": "%v",
		"ipam": {}
	}
	`
	r := &ncv1.NetworkAttachmentDefinition{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "k8s.cni.cncf.io/v1",
			Kind:       "NetworkAttachmentDefinition",
		},
		ObjectMeta: GetObjMeta(brName, labName, nsName, nodeName, nodeType),
		Spec: ncv1.NetworkAttachmentDefinitionSpec{
			Config: fmt.Sprintf(specTempalte, brName, mtu, fmt.Sprintf("vsrosfb%d", brIndex)),
		},
	}
	r.Labels[common.BridgeIndexLabelKey] = strconv.Itoa(brIndex)
	return r
}

func NewBasePod(labName, nodeName, nameSpace, image string, nodeType NodeType) *corev1.Pod {
	// gconf := conf.GCONF
	r := new(corev1.Pod)
	r.ObjectMeta = GetObjMeta(
		common.GetPodName(labName, nodeName),
		labName,
		nameSpace,
		nodeName,
		nodeType,
	)
	r.ObjectMeta.Labels[common.K8SLABELNodeKEY] = nodeName
	r.Spec.Containers = []corev1.Container{
		{
			Name:  "main",
			Image: image,
		},
	}
	r.ObjectMeta.Annotations = make(map[string]string)
	return r
}
