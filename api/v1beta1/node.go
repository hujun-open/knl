package v1beta1

import (
	"fmt"
	"reflect"

	"kubenetlab.net/knl/common"
)

type OneOfSystem struct {
	// +optional
	// +nullable
	SRVM *SRVM `json:"srvm,omitempty"`
	// +optional
	// +nullable
	// VMPC *VMPC `json:"vpc,omitempty"`
	// +optional
	// +nullable
	SRL *SRLinux `json:"srl,omitempty"`
	// +optional
	// +nullable
	Pod *GeneralPod `json:"pod,omitempty"`
}

//nullable marker + omitempty in OneOfSystem is important, it allows have a empty node specific in the CR with `{}`

func (onesys *OneOfSystem) validate() error {
	numberOfSpecified := 0
	v := reflect.ValueOf(*onesys)
	for i := 0; i < v.NumField(); i++ {
		fieldValue := v.Field(i)
		// 4. Check if the field is a pointer
		if fieldValue.Kind() == reflect.Ptr {
			// 5. Use IsNil() to check if the pointer value is nil
			if !fieldValue.IsNil() {
				numberOfSpecified++
				if numberOfSpecified > 1 {
					return fmt.Errorf("only one system type is allowed")
				}
			}
		}
	}
	if numberOfSpecified == 0 {
		return fmt.Errorf("none of system type is specified")
	}
	return nil
}

// GetSystem return the the specified node type as System interface, and corresponding field name
func (onesys *OneOfSystem) GetSystem() (common.System, string) {
	v := reflect.ValueOf(*onesys)
	t := reflect.TypeOf(*onesys)
	for i := 0; i < v.NumField(); i++ {
		fieldValue := v.Field(i)
		// 4. Check if the field is a pointer
		if fieldValue.Kind() == reflect.Ptr {
			// 5. Use IsNil() to check if the pointer value is nil
			if !fieldValue.IsNil() {
				return fieldValue.Interface().(common.System), t.Field(i).Name
			}
		}
	}
	return nil, ""
}

func (spec *LabSpec) Validate() error {
	for nodeName := range spec.NodeList {
		if err := spec.NodeList[nodeName].validate(); err != nil {
			return fmt.Errorf("Node %v is invalid, %w", nodeName, err)
		}
	}
	return nil
}
