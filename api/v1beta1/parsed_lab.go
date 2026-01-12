package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
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
	Lab               *Lab
	ConnectorMap      map[string][]string //key is node name, val is a list of link name
	SetOwnerFunc      SetOwnerFuncType
	SpokeMap          map[string]map[string][]string //1st key is nodename, 2nd key is link name, val is list of spoke name
	SpokeConnectorMap map[string]*Connector          //key is the spokename, spokename is per connector
	SpokeLinkMap      map[string]string              //key is the spokename, value is link name
}

func ParseLab(lab *Lab, sch *runtime.Scheme) *ParsedLab {
	r := new(ParsedLab)
	r.Lab = lab
	r.ConnectorMap = make(map[string][]string)
	for linkName, link := range r.Lab.Spec.LinkList {
		for _, c := range link.Connectors {
			if _, ok := r.ConnectorMap[*c.NodeName]; ok {
				r.ConnectorMap[*c.NodeName] = append(r.ConnectorMap[*c.NodeName], linkName)
			} else {
				r.ConnectorMap[*c.NodeName] = []string{linkName}
			}
		}
	}
	r.SetOwnerFunc = func(controlled metav1.Object) error {
		return ctrl.SetControllerReference(lab, controlled, sch)
	}
	return r
}

func (lab *ParsedLab) getLinkandConnector(node, linkName string) (*Link, *Connector) {
	if link, ok := lab.Lab.Spec.LinkList[linkName]; ok {
		for _, c := range link.Connectors {
			if *c.NodeName == node {
				return link, &c
			}
		}
	}
	return nil, nil
}
