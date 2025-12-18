package v1beta1

import (
	"context"
	"fmt"

	"kubenetlab.net/knl/common"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func init() {
	newvsimf := func() common.System { return new(VSIM) }
	common.NewSysRegistry[SRVMVSIM] = newvsimf
	newvsrif := func() common.System { return new(VSRI) }
	common.NewSysRegistry[SRVMVSRI] = newvsrif
	newmagcf := func() common.System { return new(MAGC) }
	common.NewSysRegistry[SRVMMAGC] = newmagcf
}

const (
	SRVMVSIM common.NodeType = "vsim"
	SRVMVSRI common.NodeType = "vsri"
	SRVMMAGC common.NodeType = "magc"
)

type VSIM SRVM
type VSRI SRVM

type MAGC SRVM

type SRVM struct {
	// +optional
	// +nullable
	Chassis *SRChassis `json:"chassis,omitempty"` //only contains chassis, sfm, card and mda
	// +optional
	// +nullable
	Image *string `json:"image,omitempty"` //for node type that use ftp, this is the folder name, not full URL
	// +optional
	// +nullable
	LicURL *string `json:"license,omitempty"` //a FTP URL, if not specified, use a fixed URL of SFTP sever in opeartor pod

}

func (srvm *SRVM) setToAppDefVal() {
	srvm.LicURL = common.ReturnPointerVal(fmt.Sprintf("ftp://ftp:ftp@%v/lic", common.FixedFTPProxySvr))
}

func (vsim *VSIM) SetToAppDefVal() {
	(*SRVM)(vsim).setToAppDefVal()
	vsim.Chassis = DefaultSIMChassis(SRVMVSIM)
}
func (vsim *VSIM) Validate() error {
	return (*SRVM)(vsim).Validate()
}
func (vsim *VSIM) Ensure(ctx context.Context, nodeName string, clnt client.Client, forceRemoval bool) error {
	return (*SRVM)(vsim).Ensure(ctx, nodeName, clnt, forceRemoval)
}

func (vsim *VSIM) FillDefaultVal(name string) {
	(*SRVM)(vsim).FillDefaultVal(name)
}

func (vsri *VSRI) SetToAppDefVal() {
	vsri.Chassis = DefaultVSRIChassis()
}

func (vsri *VSRI) Validate() error {
	return (*SRVM)(vsri).Validate()
}
func (vsri *VSRI) Ensure(ctx context.Context, nodeName string, clnt client.Client, forceRemoval bool) error {
	return (*SRVM)(vsri).Ensure(ctx, nodeName, clnt, forceRemoval)
}

func (vsri *VSRI) FillDefaultVal(name string) {
	(*SRVM)(vsri).FillDefaultVal(name)
}

func (magc *MAGC) SetToAppDefVal() {
	magc.Chassis = DefaultMAGCChassis()
}

func (magc *MAGC) Validate() error {
	return (*SRVM)(magc).Validate()
}
func (magc *MAGC) Ensure(ctx context.Context, nodeName string, clnt client.Client, forceRemoval bool) error {
	return (*SRVM)(magc).Ensure(ctx, nodeName, clnt, forceRemoval)
}

func (magc *MAGC) FillDefaultVal(name string) {
	(*SRVM)(magc).FillDefaultVal(name)
}

func (srvm *SRVM) FillDefaultVal(name string) {
	for slot := range srvm.Chassis.Cards {
		srvm.Chassis.Cards[slot].SysInfo = common.SetDefaultGeneric(srvm.Chassis.Cards[slot].SysInfo, srvm.Chassis.GetDefaultSysinfoStr(slot))
	}
}

// func (srvm *SRVM) FillDefaultVal_old(name string) {
// 	nt, _, cardid, err := ParseSRVMName(name)
// 	if err != nil {
// 		return
// 	}
// 	_, isCPM, err := ParseCardID(cardid)
// 	if err != nil {
// 		return
// 	}
// 	//set managment open ports
// 	defaultPorts := new([]kvv1.Port)

// 	if isCPM {

// 		*defaultPorts = append(*defaultPorts, kvv1.Port{
// 			Name:     "ssh",
// 			Protocol: "TCP",
// 			Port:     22,
// 		})
// 		*defaultPorts = append(*defaultPorts, kvv1.Port{
// 			Name:     "netconf",
// 			Protocol: "TCP",
// 			Port:     830,
// 		})
// 		*defaultPorts = append(*defaultPorts, kvv1.Port{
// 			Name:     "gnmi",
// 			Protocol: "TCP",
// 			Port:     57400,
// 		})
// 		*defaultPorts = append(*defaultPorts, kvv1.Port{
// 			Name:     "radiuscoa",
// 			Protocol: "UDP",
// 			Port:     3799,
// 		})
// 	} else {
// 		*defaultPorts = append(*defaultPorts, kvv1.Port{
// 			Name:     "dummy",
// 			Protocol: "TCP",
// 			Port:     1,
// 		})
// 	}
// 	srvm.Ports = common.SetDefaultGeneric(srvm.Ports, *defaultPorts)
// 	//set lic
// 	srvm.LicURL = common.SetDefaultGeneric(srvm.LicURL, fmt.Sprintf("ftp://ftp:ftp@%v/lic", common.FixedFTPProxySvr))
// 	//set srsysinfo
// 	switch nt {
// 	case SRVMVSIM:
// 		if isCPM {
// 			srvm.SRSysinfoStr = common.SetDefaultGeneric(srvm.SRSysinfoStr, DefaultVSIMCPMSysinfo)
// 			srvm.ReqCPU = common.SetDefaultGeneric(srvm.ReqCPU, resource.MustParse(DefaultVSIMCPMCPU))
// 			srvm.ReqMemory = common.SetDefaultGeneric(srvm.ReqMemory, resource.MustParse(DefaultVSIMCPMMEM))
// 		} else {
// 			srvm.SRSysinfoStr = common.SetDefaultGeneric(srvm.SRSysinfoStr, DefaultVSIMIOMSysinfo)
// 			srvm.ReqCPU = common.SetDefaultGeneric(srvm.ReqCPU, resource.MustParse(DefaultVSIMIOMCPU))
// 			srvm.ReqMemory = common.SetDefaultGeneric(srvm.ReqMemory, resource.MustParse(DefaultVSIMIOMMEM))
// 		}
// 	case SRVMVSRI:
// 		srvm.SRSysinfoStr = common.SetDefaultGeneric(srvm.SRSysinfoStr, DefaultSRSIMSysinfo)
// 		srvm.ReqCPU = common.SetDefaultGeneric(srvm.ReqCPU, resource.MustParse(DefaultVSRICPU))
// 		srvm.ReqMemory = common.SetDefaultGeneric(srvm.ReqMemory, resource.MustParse(DefaultVSRIMEM))

// 	case SRVMMAGC:
// 		if isCPM {
// 			srvm.SRSysinfoStr = common.SetDefaultGeneric(srvm.SRSysinfoStr, DefaultMAGCOAMSysinfo)
// 			srvm.ReqCPU = common.SetDefaultGeneric(srvm.ReqCPU, resource.MustParse(DefaultMAGCOAMCPU))
// 			srvm.ReqMemory = common.SetDefaultGeneric(srvm.ReqMemory, resource.MustParse(DefaultMAGCOAMMEM))

// 		} else { //this could be either LB or MG, use LB for default
// 			srvm.SRSysinfoStr = common.SetDefaultGeneric(srvm.SRSysinfoStr, DefaultMAGCLBSysinfo)
// 			srvm.ReqCPU = common.SetDefaultGeneric(srvm.ReqCPU, resource.MustParse(DefaultMAGCLBCPU))
// 			srvm.ReqMemory = common.SetDefaultGeneric(srvm.ReqMemory, resource.MustParse(DefaultMAGCLBMEM))
// 		}

// 	}
// 	//set image
// 	//release folder
// 	switch nt {
// 	case SRVMVSIM, SRVMVSRI:
// 		srvm.Image = common.SetDefaultGeneric(srvm.Image, DefSRImgFolder)
// 	case SRVMMAGC:
// 		srvm.Image = common.SetDefaultGeneric(srvm.Image, DefMAGCImgFolder)

// 	}

// }

// func (srvm *SRVM) SetToAppDefVal() {
// 	srvm.Image = common.ReturnPointerVal("R")
// }

func (srvm *SRVM) Validate() error {
	return nil
}

func GetSRVMviaSys(nodeName string, sys common.System) *SRVM {
	switch common.GetNodeTypeViaName(nodeName) {
	case SRVMMAGC:
		return (*SRVM)(sys.(*MAGC))
	case SRVMVSIM:
		return (*SRVM)(sys.(*VSIM))
	case SRVMVSRI:
		return (*SRVM)(sys.(*VSRI))
	}
	return nil
}
