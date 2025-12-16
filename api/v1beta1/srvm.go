package v1beta1

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/resource"
	"kubenetlab.net/knl/common"
	kvv1 "kubevirt.io/api/core/v1"
)

func init() {
	newf := func() common.System { return new(SRVM) }
	common.NewSysRegistry[VSIM] = newf
	common.NewSysRegistry[VSRI] = newf
	common.NewSysRegistry[MAGC] = newf

}

const (
	VSIM common.NodeType = "vsim"
	VSRI common.NodeType = "vsri"
	MAGC common.NodeType = "magc"
)

type SRVM struct {
	// +optional
	// +nullable
	SRSysinfoStr *string `json:"sysinfo,omitempty"` //only contains chassis, sfm, card and mda
	// +optional
	// +nullable
	ReqMemory *resource.Quantity `json:"memory,omitempty"`
	// +optional
	// +nullable
	ReqCPU *resource.Quantity `json:"cpu,omitempty"`
	// +optional
	// +nullable
	Image *string `json:"image,omitempty"` //for node type that use ftp, this is the folder name, not full URL
	// +optional
	// +nullable
	LicURL *string `json:"license,omitempty"` //a FTP URL, if not specified, use a fixed URL of SFTP sever in opeartor pod
	// +optional
	// +nullable
	Ports *[]kvv1.Port `json:"ports,omitempty"` //list of open port for management interface
}

func (srvm *SRVM) FillDefaultVal(name string) {
	nt, _, cardid, err := ParseSRVMName(name)
	if err != nil {
		return
	}
	_, isCPM, err := ParseCardID(cardid)
	if err != nil {
		return
	}
	//set managment open ports
	defaultPorts := new([]kvv1.Port)

	if isCPM {

		*defaultPorts = append(*defaultPorts, kvv1.Port{
			Name:     "ssh",
			Protocol: "TCP",
			Port:     22,
		})
		*defaultPorts = append(*defaultPorts, kvv1.Port{
			Name:     "netconf",
			Protocol: "TCP",
			Port:     830,
		})
		*defaultPorts = append(*defaultPorts, kvv1.Port{
			Name:     "gnmi",
			Protocol: "TCP",
			Port:     57400,
		})
		*defaultPorts = append(*defaultPorts, kvv1.Port{
			Name:     "radiuscoa",
			Protocol: "UDP",
			Port:     3799,
		})
	} else {
		*defaultPorts = append(*defaultPorts, kvv1.Port{
			Name:     "dummy",
			Protocol: "TCP",
			Port:     1,
		})
	}
	srvm.Ports = common.SetDefaultGeneric(srvm.Ports, *defaultPorts)
	//set lic
	srvm.LicURL = common.SetDefaultGeneric(srvm.LicURL, fmt.Sprintf("ftp://ftp:ftp@%v/lic", common.FixedFTPProxySvr))
	//set srsysinfo
	switch nt {
	case VSIM:
		if isCPM {
			srvm.SRSysinfoStr = common.SetDefaultGeneric(srvm.SRSysinfoStr, DefaultVSIMCPMSysinfo)
			srvm.ReqCPU = common.SetDefaultGeneric(srvm.ReqCPU, resource.MustParse(DefaultVSIMCPMCPU))
			srvm.ReqMemory = common.SetDefaultGeneric(srvm.ReqMemory, resource.MustParse(DefaultVSIMCPMMEM))
		} else {
			srvm.SRSysinfoStr = common.SetDefaultGeneric(srvm.SRSysinfoStr, DefaultVSIMIOMSysinfo)
			srvm.ReqCPU = common.SetDefaultGeneric(srvm.ReqCPU, resource.MustParse(DefaultVSIMIOMCPU))
			srvm.ReqMemory = common.SetDefaultGeneric(srvm.ReqMemory, resource.MustParse(DefaultVSIMIOMMEM))
		}
	case VSRI:
		srvm.SRSysinfoStr = common.SetDefaultGeneric(srvm.SRSysinfoStr, DefaultSRSIMSysinfo)
		srvm.ReqCPU = common.SetDefaultGeneric(srvm.ReqCPU, resource.MustParse(DefaultVSRICPU))
		srvm.ReqMemory = common.SetDefaultGeneric(srvm.ReqMemory, resource.MustParse(DefaultVSRIMEM))

	case MAGC:
		if isCPM {
			srvm.SRSysinfoStr = common.SetDefaultGeneric(srvm.SRSysinfoStr, DefaultMAGCOAMSysinfo)
			srvm.ReqCPU = common.SetDefaultGeneric(srvm.ReqCPU, resource.MustParse(DefaultMAGCOAMCPU))
			srvm.ReqMemory = common.SetDefaultGeneric(srvm.ReqMemory, resource.MustParse(DefaultMAGCOAMMEM))

		} else { //this could be either LB or MG, use LB for default
			srvm.SRSysinfoStr = common.SetDefaultGeneric(srvm.SRSysinfoStr, DefaultMAGCLBSysinfo)
			srvm.ReqCPU = common.SetDefaultGeneric(srvm.ReqCPU, resource.MustParse(DefaultMAGCLBCPU))
			srvm.ReqMemory = common.SetDefaultGeneric(srvm.ReqMemory, resource.MustParse(DefaultMAGCLBMEM))
		}

	}
	//set image
	//release folder
	switch nt {
	case VSIM, VSRI:
		srvm.Image = common.SetDefaultGeneric(srvm.Image, DefSRImgFolder)
	case MAGC:
		srvm.Image = common.SetDefaultGeneric(srvm.Image, DefMAGCImgFolder)

	}

}
func (srvm *SRVM) SetToAppDefVal() {
	srvm.Image = common.ReturnPointerVal("R")
}

func (srvm *SRVM) Validate() error {
	return nil
}

func (srvm *SRVM) GetNodeType(name string) common.NodeType {
	nt, _, _, err := ParseSRVMName(name)
	if err != nil {
		return common.Unknown
	}
	return nt
}
