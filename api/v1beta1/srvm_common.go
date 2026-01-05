package v1beta1

import (
	"fmt"
	"strconv"
	"strings"

	"kubenetlab.net/knl/common"
)

// sysinfo detauls
const (
	DefaultVSIMCPMSysinfo = "chassis=SR-12 sfm=m-sfm5-12 card=cpm5"
	DefaultVSIMIOMSysinfo = "chassis=SR-12 card=iom4-e  mda/1=me10-10gb-sfp+  mda/2=me-isa2-ms"
	DefaultSRSIMSysinfo   = "chassis=SR-12 sfm=m-sfm5-12 card=cpm5 slot=A\nslot=1 card=iom4-e  mda/1=me10-10gb-sfp+  mda/2=me-isa2-ms"
	DefaultVSRISysinfo    = "chassis=VSR-I card=cpm-v mda/1=m20-v"
	DefaultMAGCOAMSysinfo = "chassis=VSR card=cpm-v"
	DefaultMAGCLBSysinfo  = "chassis=VSR card=iom-v mda/1=m20-v ofabric/1=2 control-cpu-cores=2"
	DefaultMAGCMGysinfo   = "chassis=VSR card=iom-v-mg mda/1=isa-ms-v mda/2=isa-ms-v ofabric/1=2 control-cpu-cores=4"
)

// img defaults
const (
	DefSRImgFolder   = "SR_R"
	DefMAGCImgFolder = "MAGC_R"
)

// cpu defaults
const (
	DefaultVSIMCPMCPU = "2"
	DefaultVSIMIOMCPU = "2"
	DefaultVSRICPU    = "2"
	DefaultMAGCOAMCPU = "2"
	DefaultMAGCLBCPU  = "4"
	DefaultMAGCMGCPU  = "8"
)

// mem defaults
const (
	DefaultVSIMCPMMEM        = "4Gi"
	DefaultSRSIMCONTAINERMEM = "4Gi"
	DefaultVSIMIOMMEM        = "4Gi"
	DefaultVSRIMEM           = "6Gi"
	DefaultMAGCOAMMEM        = "16Gi"
	DefaultMAGCLBMEM         = "8Gi"
	DefaultMAGCMGMEM         = "32Gi"
)

// name has prefix: <vmtype>-
func ParseSRVMName_New(name string) (vmtype common.NodeType, err error) {
	s := strings.TrimSpace(name)
	slist := strings.Split(s, "-")
	if len(slist) < 2 {
		err = fmt.Errorf("%v is not a valid name", name)
		return
	}
	vmtype = common.NodeType(slist[0])
	switch vmtype {
	case SRVMVSIM, SRVMVSRI, SRVMMAGC:
	default:
		return common.Unknown, fmt.Errorf("unknown SR VM type %v", slist[0])
	}
	return
}

// name format for vsim/magc: vmtype-vmid-cardid
// name format for vpc/vsri: vmtype-vmid
// cardid is either a,b or a number
func ParseSRVMName(name string) (vmtype common.NodeType, vmid int, cardid string, err error) {
	s := strings.TrimSpace(name)
	slist := strings.Split(s, "-")
	if len(slist) < 2 {
		err = fmt.Errorf("%v is not a valid name", name)
		return
	}
	vmtype = common.NodeType(slist[0])
	vmid, err = strconv.Atoi(slist[1])
	switch vmtype {
	case SRVMVSRI:
		cardid = "a"
	case SRVMVSIM, SRVMMAGC:
		if len(slist) < 3 {
			err = fmt.Errorf("%v is not a valid name", name)
			return
		}
		cardid = slist[2]
	}
	return
}

// a or b is the CPM, a  could also be integrated system like sr-1
func ParseCardID(cardid string) (cardnum int, isCPM bool, err error) {
	switch cardid {
	case "a", "b":
		switch cardid {
		case "a":
			return 199, true, nil
		default:
			return 198, true, nil
		}
	default:
		isCPM = false
		cardnum, err = strconv.Atoi(cardid)
		return
	}
}

func GetSRVMCardVMName(lab, chassis, slot string) string {
	return strings.ToLower(fmt.Sprintf("%v-%v-%v", lab, chassis, slot))
}

func getSRVMLicFileName(lab, chassis string) string {
	return strings.ToLower(fmt.Sprintf("%v-%v", lab, chassis))
}

func getFullQualifiedSRVMChassisName(lab, chassis string) string {
	return strings.ToLower(fmt.Sprintf("%v-%v", lab, chassis))
}

func IsSRVM(nodeT common.NodeType) bool {
	switch nodeT {
	case SRVMMAGC, SRVMVSIM, SRVMVSRI:
		return true
	}
	return false
}
