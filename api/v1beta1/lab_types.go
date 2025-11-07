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
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"kubenetlab.net/knl/common"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// LabSpec defines the desired state of Lab
type LabSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	// The following markers will use OpenAPI v3 schema to validate the value
	// More info: https://book.kubebuilder.io/reference/markers/crd-validation.html

	// nodes lists all nodes in the lab
	// +optional
	// +nullable
	NodeList map[string]*OneOfSystem `json:"nodes"`
	// +optional
	// +nullable
	LinkList map[string]*Link `json:"links"`
}

// LabStatus defines the observed state of Lab.
type LabStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// For Kubernetes API conventions, see:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties

	// conditions represent the current state of the Lab resource.
	// Each condition has a unique type and reflects the status of a specific aspect of the resource.
	//
	// Standard condition types include:
	// - "Available": the resource is fully functional
	// - "Progressing": the resource is being created or updated
	// - "Degraded": the resource failed to reach or maintain its desired state
	//
	// The status of each condition is one of True, False, or Unknown.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Lab is the Schema for the labs API
type Lab struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// spec defines the desired state of Lab
	// +required
	Spec LabSpec `json:"spec"`

	// status defines the observed state of Lab
	// +optional
	Status LabStatus `json:"status,omitempty,omitzero"`
}

// +kubebuilder:object:root=true

// LabList contains a list of Lab
type LabList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Lab `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Lab{}, &LabList{})
}

// AssignSystem assign s to corresponding field of node
func AssignSystem(s common.System, node *OneOfSystem) {
	targetType := reflect.TypeOf(s).Elem() //should be type of node
	targetVal := reflect.ValueOf(s)
	nodeVal := reflect.ValueOf(node)
	nodeType := reflect.TypeOf(node)
	nodeVal = nodeVal.Elem()
	nodeType = nodeType.Elem()
	for i := 0; i < nodeVal.NumField(); i++ {
		if nodeType.Field(i).Type.Elem().String() == targetType.String() {
			nodeVal.Field(i).Set(targetVal)
			break
		}
	}

}

func (lab *Lab) getLinkandConnector(node, linkName string) (*Link, *Connector) {
	if link, ok := lab.Spec.LinkList[linkName]; ok {
		for _, c := range link.Connectors {
			if *c.NodeName == node {
				return link, &c
			}
		}
	}
	return nil, nil
}
