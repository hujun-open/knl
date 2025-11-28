package v1beta1

import (
	"k8s.io/apimachinery/pkg/api/resource"
	"kubenetlab.net/knl/common"
)

func init() {
	// common.NewSysRegistry[VPC] = func() common.System { return new(VMPC) }
}

type VMPC struct {
	ReqMemory     *resource.Quantity `json:"memory,omitempty"`
	ReqCPU        *resource.Quantity `json:"cpu,omitempty"`
	DiskSize      *resource.Quantity `json:"diskSize,omitempty"`
	PinCPU        bool               `json:"cpuPin,omitempty"`
	HugePage      bool               `json:"hugePage,omitempty"`
	Image         string             `json:"image,omitempty"`
	DisablePodNet bool               `json:"disablePodNet,omitempty"`
	InitMethod    string             `json:"init,omitempty"`
	Ports         *[]int32           `json:"ports,omitempty"`
}

const (
	DefVPCCPU = "2.0"
	DefVPCMem = "4Gi"
)

const (
	VPC common.NodeType = "vpc"
)

func (vpc *VMPC) SetToAppDefVal() {
	common.AssignPointerVal(&vpc.ReqCPU, resource.MustParse(DefVPCCPU))
	common.AssignPointerVal(&vpc.ReqMemory, resource.MustParse(DefVPCMem))
}

func (vpc *VMPC) FillDefaultVal(nodeName string) {

}

func (vpc *VMPC) GetNodeType(name string) common.NodeType {
	return VPC
}

func (vpc *VMPC) Validate() error {
	return nil
}
