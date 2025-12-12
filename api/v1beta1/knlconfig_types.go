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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"kubenetlab.net/knl/common"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// KNLConfigSpec defines the desired state of KNLConfig
type KNLConfigSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	// The following markers will use OpenAPI v3 schema to validate the value
	// More info: https://book.kubebuilder.io/reference/markers/crd-validation.html

	// +optional
	SFTPUser *string `json:"fileUser,omitempty"`
	// +optional
	SFTPPassword *string `json:"filePass,omitempty"`
	// +optional
	//SFTPSever must have format as addr/hostname:port
	SFTPSever *string `json:"fileSvr,omitempty"`
	// +optional
	VXLANGrpAddr *string `json:"vxlanGrp,omitempty"`
	// +optional
	VXLANDefaultDev *string `json:"defaultVxlanDev,omitempty"`
	// +optional
	VxDevMap map[string]string `json:"vxlanDevMap,omitempty"`
	// +optional
	LinkMtu *uint `json:"linkMTU,omitempty"`
	// +optional
	PVCStorageClass *string `json:"storageClass,omitempty"`
	// +optional
	//this url supported by kvirt cdi, either http or docker url
	SRCPMLoaderImage *string `json:"srCPMLoaderImage,omitempty"`
	// +optional
	SRIOMLoaderImage *string `json:"srIOMLoaderImage,omitempty"`
	// +optional
	SideCarHookImg *string `json:"sideCarImage,omitempty"`

	// defaultNode specifies default values for types of node
	// +optional
	DefaultNode *OneOfSystem `json:"defaultNode,omitempty"`
}

// this is default knlconfig to use to fill any non-specified field,
// this is the application default, meaning when user didn't specify the corresponding field in KNLconfig
func DefKNLConfig() KNLConfigSpec {
	var r KNLConfigSpec
	common.AssignPointerVal(&r.SFTPSever, "knl-sftp-service.knl-system.svc.cluster.local:22")
	common.AssignPointerVal(&r.SFTPUser, "knlftp")
	common.AssignPointerVal(&r.SFTPPassword, "knlftp")
	common.AssignPointerVal(&r.VXLANGrpAddr, "ff18::100")
	common.AssignPointerVal(&r.LinkMtu, 7000)
	common.AssignPointerVal(&r.PVCStorageClass, "nfs-client")
	common.AssignPointerVal(&r.SRCPMLoaderImage, "http://knl-http.knl-system.svc.cluster.local/cpmload.img")
	//create app default for each node type
	defOne := OneOfSystem{}
	val := reflect.ValueOf(&defOne)
	val = val.Elem()
	for i := 0; i < val.NumField(); i++ {
		newPointerVal := reflect.New(val.Field(i).Type().Elem())
		newPointerVal.Interface().(common.System).SetToAppDefVal()
		val.Field(i).Set(newPointerVal)
	}
	r.DefaultNode = &defOne
	return r
}

const (
	StaticHTTPFileFolder = "/static"
	StaticHTTPSvrPort    = 8880
)

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

// KNLConfig is the Schema for the knlconfigs API
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
		err = common.FillNilPointers(node, defNode.Interface().(common.System))
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

	if *knlcfg.Spec.LinkMtu < 100 || *knlcfg.Spec.LinkMtu > 10000 {
		return fmt.Errorf("invalid linkmtu %d, must be in range of 100..10000", *knlcfg.Spec.LinkMtu)
	}
	if isStrNotSpecfied(knlcfg.Spec.PVCStorageClass) {
		return fmt.Errorf("storage class not specified")
	}
	if isStrNotSpecfied(knlcfg.Spec.SRCPMLoaderImage) {
		return fmt.Errorf("SR CPM loader image not specified")
	}
	if isStrNotSpecfied(knlcfg.Spec.SRIOMLoaderImage) {
		return fmt.Errorf("SR IOM loader image not specified")
	}
	if isStrNotSpecfied(knlcfg.Spec.SideCarHookImg) {
		return fmt.Errorf("sidecar image not specified")
	}
	if knlcfg.Spec.SFTPSever != nil {
		if !common.IsHostPort(*knlcfg.Spec.SFTPSever) {
			return fmt.Errorf("%v must be in format as addr/host:port", *knlcfg.Spec.SFTPSever)
		}
	} else {
		return fmt.Errorf("file server address not specified")
	}
	if isStrNotSpecfied(knlcfg.Spec.SFTPUser) {
		return fmt.Errorf("file server username not specified")
	}
	if isStrNotSpecfied(knlcfg.Spec.SFTPPassword) {
		return fmt.Errorf("file server password not specified")
	}
	return nil
}
