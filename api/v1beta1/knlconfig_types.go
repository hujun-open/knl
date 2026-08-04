/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1beta1

import (
	"fmt"
	"net/netip"
	"reflect"
	"strings"

	"github.com/distribution/reference"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// KNLConfigSpec specifies KNL operator's configuration
type KNLConfigSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	// The following markers will use OpenAPI v3 schema to validate the value
	// More info: https://book.kubebuilder.io/reference/markers/crd-validation.html

	//SFTPSever address, must have format as addr/hostname:port
	// +optional
	SFTPSever *string `json:"fileSvr,omitempty"`
	//multicast address used by VxLAN tunnel between k8s workers
	// +optional
	VXLANGrpAddr *string `json:"vxlanGrp,omitempty"`
	//default VxLAN device name, used if not specified in vxlanDevMap
	// +optional
	VXLANDefaultDev *string `json:"defaultVxlanDev,omitempty"`
	//a map between k8s worker name and its interface name used as VxLAN device
	// +optional
	VxDevMap map[string]string `json:"vxlanDevMap,omitempty"`
	//name of k8s storageclass used to create PVCs
	// +optional
	PVCStorageClass *string `json:"storageClass,omitempty"`
	//CPM loader container image, used by vsim, vsri and magc
	// +optional
	SRCPMLoaderImage *string `json:"srCPMLoaderImage,omitempty"`
	//IOM loader container image, used by vsim and magc
	// +optional
	SRIOMLoaderImage *string `json:"srIOMLoaderImage,omitempty"`
	//Kubevirt sidecar hook image, used by vsim, vsri and magc
	// +optional
	SideCarHookImg *string `json:"sideCarImage,omitempty"`
	// defaultNode specifies default values for types of node
	// +optional
	DefaultNode *OneOfSystem `json:"defaultNode,omitempty"`
}

// this is default knlconfig to use to fill any non-specified field,
// this is the application default, meaning when user didn't specify the corresponding field in KNLconfig
func DefKNLConfig() KNLConfigSpec {
	r := KNLConfigSpec{
		SFTPSever:      ReturnPointerVal("knl-sftp-service.knl-system.svc.cluster.local:22"),
		VXLANGrpAddr:   ReturnPointerVal("ff18::100"),
		SideCarHookImg: ReturnPointerVal("ghcr.io/hujun-open/knl/knlsidecar:latest"),
	}
	//create app default for each node type
	defOne := OneOfSystem{}
	val := reflect.ValueOf(&defOne)
	val = val.Elem()
	for i := 0; i < val.NumField(); i++ {
		newPointerVal := reflect.New(val.Field(i).Type().Elem())
		newPointerVal.Interface().(System).SetToAppDefVal()
		val.Field(i).Set(newPointerVal)
	}
	r.DefaultNode = &defOne
	return r
}

// KNLConfigStatus defines the observed state of KNLConfig.
type KNLConfigStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// For Kubernetes API conventions, see:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties

	// +optional
	ObservedGeneration *int64 `json:"observedGeneration"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// KNLConfig is the Schema for the configuration of KNL operator
type KNLConfig struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// spec defines the desired state of KNLConfig
	// +required
	Spec KNLConfigSpec `json:"spec"`

	// status defines the observed state of KNLConfig
	// +optional
	Status KNLConfigStatus `json:"status,omitempty,omitzero"`
}

// +kubebuilder:object:root=true

// KNLConfigList contains a list of KNLConfig
type KNLConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KNLConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KNLConfig{}, &KNLConfigList{})
}

// loadDef load non-specified fields of in with def, using DefaultNode in KNLConfigSpec
func LoadDef(in *LabSpec, def KNLConfigSpec) error {
	//node defaults
	defVal := reflect.ValueOf(def.DefaultNode).Elem()
	var err error
	for nodeName := range in.NodeList {
		nodesys := in.NodeList[nodeName]
		node, fieldName := nodesys.GetSystem()
		if node == nil {
			return fmt.Errorf("node %v's type is not specified", nodeName)
		}
		defNode := defVal.FieldByName(fieldName)
		if !defNode.IsValid() {
			continue
		}
		if defNode.IsNil() {
			continue
		}
		err = FillNilPointers(node, defNode.Interface().(System))
		if err != nil {
			return err
		}

	}
	return nil
}

func isStrNotSpecfied(str *string) bool {
	if str == nil {
		return true
	}
	if strings.TrimSpace(*str) == "" {
		return true
	}
	return false
}

func (knlcfg *KNLConfig) Validate() error {

	if addr, err := netip.ParseAddr(*knlcfg.Spec.VXLANGrpAddr); err != nil {
		return fmt.Errorf("%v is not valid VxLAN Group IP addr", *knlcfg.Spec.VXLANGrpAddr)
	} else {
		if addr.Is4() || !addr.IsMulticast() {
			return fmt.Errorf("VxLAN Group IP addr %v is not a IPv6 multicast address", *knlcfg.Spec.VXLANGrpAddr)
		}
	}
	// if p, err := netip.ParsePrefix(*knlcfg.Spec.VXLANGrpPrefix); err != nil {
	// 	return fmt.Errorf("%v is not valid VxLAN Group IP prefix", *knlcfg.Spec.VXLANGrpPrefix)
	// } else {
	// 	if !p.Addr().IsMulticast() {
	// 		return fmt.Errorf("%v is not a multicast address prefix", *knlcfg.Spec.VXLANGrpPrefix)
	// 	}
	// }

	if isStrNotSpecfied(knlcfg.Spec.PVCStorageClass) {
		return fmt.Errorf("storage class not specified")
	}
	if isStrNotSpecfied(knlcfg.Spec.VXLANDefaultDev) && len(knlcfg.Spec.VxDevMap) == 0 {
		return fmt.Errorf("vxlan dev not specified")
	}
	// if isStrNotSpecfied(knlcfg.Spec.SRCPMLoaderImage) {
	// 	return fmt.Errorf("SR CPM loader image not specified")
	// }
	if knlcfg.Spec.SRCPMLoaderImage != nil {
		if _, err := reference.Parse(*knlcfg.Spec.SRCPMLoaderImage); err != nil {
			return fmt.Errorf("%v is not valid container image url: %w", *knlcfg.Spec.SRCPMLoaderImage, err)
		}
	}
	// if isStrNotSpecfied(knlcfg.Spec.SRIOMLoaderImage) {
	// 	return fmt.Errorf("SR IOM loader image not specified")
	// }
	if knlcfg.Spec.SRIOMLoaderImage != nil {
		if _, err := reference.Parse(*knlcfg.Spec.SRIOMLoaderImage); err != nil {
			return fmt.Errorf("%v is not valid container image url: %w", *knlcfg.Spec.SRIOMLoaderImage, err)
		}
	}
	if isStrNotSpecfied(knlcfg.Spec.SideCarHookImg) {
		return fmt.Errorf("sidecar image not specified")
	}
	if _, err := reference.Parse(*knlcfg.Spec.SideCarHookImg); err != nil {
		return fmt.Errorf("%v is not valid container image url: %w", *knlcfg.Spec.SideCarHookImg, err)
	}

	if knlcfg.Spec.SFTPSever != nil {
		if !IsHostPort(*knlcfg.Spec.SFTPSever) {
			return fmt.Errorf("%v must be in format as addr/host:port", *knlcfg.Spec.SFTPSever)
		}
	} else {
		return fmt.Errorf("file server address not specified")
	}
	return nil
}
