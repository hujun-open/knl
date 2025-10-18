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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"kubenetlab.net/knl/internal/common"
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
	FTPUser *string `json:"ftpUser,omitempty"`
	// +optional
	FTPPassword *string `json:"ftpPass,omitempty"`
	// +optional
	FTPSever *string `json:"ftpSvr,omitempty"`
	// defaultNode specifies default values for types of node
	// +optional
	DefaultNode OneOfSystem `json:"defaultNode,omitempty"`
}

// this is default knlconfig to use to fill any non-specified field,
// this is the application default, meaning when user didn't specify the corresponding field in KNLconfig
func DefKNLConfig() KNLConfigSpec {
	var r KNLConfigSpec
	common.AssignPointerVal(&r.FTPUser, "ftp")
	common.AssignPointerVal(&r.FTPPassword, "ftp")
	//create app default for each node type
	defOne := OneOfSystem{}
	val := reflect.ValueOf(&defOne)
	val = val.Elem()
	for i := 0; i < val.NumField(); i++ {
		newPointerVal := reflect.New(val.Field(i).Type().Elem())
		newPointerVal.Interface().(common.System).SetToAppDefVal()
		val.Field(i).Set(newPointerVal)
	}
	r.DefaultNode = defOne
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

// loadDef load non-specified fields of in with def
func LoadDef(in *LabSpec, def KNLConfigSpec) error {
	//node defaults
	defVal := reflect.ValueOf(def.DefaultNode)
	var err error
	for i := range in.NodeList {
		node, fieldName := in.NodeList[i].OneOfSystem.GetSystem()
		if node == nil {
			return fmt.Errorf("node %d's type is not specified", i+1)
		}
		def := defVal.FieldByName(fieldName)
		if !def.IsValid() {
			continue
		}
		if def.IsNil() {
			continue
		}
		err = common.FillNilPointers(node, def.Interface().(common.System))
		if err != nil {
			return err
		}

	}
	return nil
}

func (knlcfg *KNLConfig) Validate() error {
	if knlcfg.Spec.FTPSever != nil {
		if len(*knlcfg.Spec.FTPSever) > 0 {
			_, err := netip.ParseAddr(*knlcfg.Spec.FTPSever)
			if err != nil {
				return err
			}
		}
	}
	return nil
}
