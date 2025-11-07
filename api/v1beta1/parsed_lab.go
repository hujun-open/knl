package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:generate=false
// +kubebuilder:object:root=false
type CtxValueKey int

const (
	ParsedLabKey CtxValueKey = iota
)

// +kubebuilder:object:generate=false
// +kubebuilder:object:root=false
type SetOwnerFuncType func(controlled metav1.Object) error

// +kubebuilder:object:generate=false
// +kubebuilder:object:root=false
type ParsedLab struct {
	Lab          *Lab
	ConnectorMap map[string][]string //key is node name, val is a list of link name
	SetOwnerFunc SetOwnerFuncType
}
