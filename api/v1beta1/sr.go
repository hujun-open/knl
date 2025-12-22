package v1beta1

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/api/resource"
	"kubenetlab.net/knl/common"
	kvv1 "kubevirt.io/api/core/v1"
)

// SRCard is a CPM or IOM card
type SRCard struct {
	//Card model
	Model *string `json:"type,omitempty"`
	// sysinfo is only used by vsim, mag-c and vsri
	SysInfo *string `json:"sysinfo,omitempty"`
	// list of MDAs that are insert directly into card without XIOM; mdas and xioms are mutully exclusive
	MDAs *[]string `json:"mdas,omitempty"`
	// list of XIOMs; key is XIOM slot id, e.g. x1/x2; mdas and xioms are mutully exclusive
	XIOM map[string]XIOM `json:"xioms,omitempty"`
	// requested memory in k8s resouce unit
	// +optional
	// +nullable
	ReqMemory *resource.Quantity `json:"memory,omitempty"`
	// +optional
	// +nullable
	// requested CPU in k8s resource unit
	ReqCPU *resource.Quantity `json:"cpu,omitempty"`
	//list of listening ports for management interface
	// +optional
	// +nullable
	ListenPorts *[]kvv1.Port `json:"ports,omitempty"`
}

// SR XIOM
type XIOM struct {
	// XIOM model
	Model *string `json:"type,omitempty"`
	// list of MDAs insert into the XIOM
	MDAs []string `json:"mdas,omitempty"`
}

func (card *SRCard) Validate() error {
	if card.Model == nil {
		return fmt.Errorf("card type is not specified")
	}
	//sysinfo is not checked here since srsim doesn't use it
	if len(card.XIOM) > 0 {
		if card.MDAs != nil {
			if len(*card.MDAs) > 0 {
				return fmt.Errorf("mdas and xioms are mutully exclusive, can't be both specified")
			}
		}

	}
	return nil
}

func getIOMVMListenPorts() *[]kvv1.Port {
	r := []kvv1.Port{
		{
			Name:     "dummy",
			Protocol: "TCP",
			Port:     1,
		},
	}
	return &r
}

func getCPMVMListenPorts() *[]kvv1.Port {
	r := []kvv1.Port{
		{
			Name:     "ssh",
			Protocol: "TCP",
			Port:     22,
		},
		{
			Name:     "netconf",
			Protocol: "TCP",
			Port:     830,
		},
		{
			Name:     "gnmi",
			Protocol: "TCP",
			Port:     57400,
		},
		{
			Name:     "radiuscoa",
			Protocol: "UDP",
			Port:     3799,
		},
	}
	return &r
}

// SRChassis is used by srsim, vsim, vsri and magc to specify the chassis configuration.
type SRChassis struct {
	//type of chassis, srsim, vsim vsris or magc
	Type *common.NodeType `json:"type,omitempty"`
	//chassis model
	Model *string `json:"model,omitempty"`
	// a dictionary of CPM and IOM cards,
	//key is slot id, "A","B" for CPM, number for IOM
	Cards map[string]*SRCard `json:"cards,omitempty"`
	//SFM model
	SFM *string `json:"sfm,omitempty"`
}

func (chassis *SRChassis) GetDefaultCPMSlot() string {
	for slot := range chassis.Cards {
		if common.IsCPM(slot) {
			return slot
		}
	}
	return "n/a"
}

func (chassis *SRChassis) Validate() error {
	if chassis.Model == nil {
		return fmt.Errorf("chassis model not specified")
	}
	if chassis.Type == nil {
		return fmt.Errorf("chassis type not specified")
	}
	for slot, card := range chassis.Cards {
		if err := card.Validate(); err != nil {
			return fmt.Errorf("invalid card %v spec: %w", slot, err)
		}
	}
	if _, ok := chassis.Cards["A"]; !ok {
		return fmt.Errorf("slot A not specified")
	}
	return nil
}

// DefaultSIMChassis return default chassis for SRSIM or VSIM
func DefaultSIMChassis(nt common.NodeType) *SRChassis {
	r := &SRChassis{
		Type:  common.ReturnPointerVal(nt),
		Model: common.ReturnPointerVal("SR-7"),
		SFM:   common.ReturnPointerVal("m-sfm5-7"),
	}
	r.Cards = make(map[string]*SRCard)
	r.Cards["A"] = &SRCard{
		Model: common.ReturnPointerVal("cpm5"),
	}
	if nt == SRVMVSIM {
		r.Cards["A"].ReqMemory = common.ReturnPointerVal(resource.MustParse(DefaultVSIMCPMMEM))
		r.Cards["A"].ReqCPU = common.ReturnPointerVal(resource.MustParse(DefaultVSIMCPMCPU))
		r.Cards["A"].ListenPorts = getCPMVMListenPorts()
	} else {
		//srsim
		r.Cards["A"].ReqMemory = common.ReturnPointerVal(resource.MustParse(DefaultSRSIMCONTAINERMEM))
	}
	r.Cards["1"] = &SRCard{
		Model: common.ReturnPointerVal("iom4-e"),
		MDAs:  common.GetPointerVal([]string{"me10-10gb-sfp+", "me-isa2-ms"}),
	}
	if nt == SRVMVSIM {
		r.Cards["1"].ReqMemory = common.ReturnPointerVal(resource.MustParse(DefaultVSIMIOMMEM))
		r.Cards["1"].ReqCPU = common.ReturnPointerVal(resource.MustParse(DefaultVSIMIOMCPU))
		r.Cards["1"].ListenPorts = getIOMVMListenPorts()
	} else {
		//srsim
		r.Cards["1"].ReqMemory = common.ReturnPointerVal(resource.MustParse(DefaultSRSIMCONTAINERMEM))
	}
	return r
}

func DefaultVSRIChassis() *SRChassis {
	r := &SRChassis{
		Type:  common.ReturnPointerVal(SRVMVSRI),
		Model: common.ReturnPointerVal("VSR-I"),
	}
	r.Cards = make(map[string]*SRCard)
	r.Cards["A"] = &SRCard{
		Model:       common.ReturnPointerVal("cpm-v"),
		MDAs:        common.GetPointerVal([]string{"m20-v", "isa-tunnel-v"}),
		ReqMemory:   common.ReturnPointerVal(resource.MustParse(DefaultVSRIMEM)),
		ReqCPU:      common.ReturnPointerVal(resource.MustParse(DefaultVSRICPU)),
		ListenPorts: getCPMVMListenPorts(),
	}
	return r
}

func DefaultMAGCChassis() *SRChassis {
	r := &SRChassis{
		Type:  common.ReturnPointerVal(SRVMMAGC),
		Model: common.ReturnPointerVal("VSR"),
	}
	r.Cards = make(map[string]*SRCard)
	r.Cards["A"] = &SRCard{
		Model:       common.ReturnPointerVal("cpm-v"),
		ReqMemory:   common.ReturnPointerVal(resource.MustParse(DefaultMAGCOAMMEM)),
		ReqCPU:      common.ReturnPointerVal(resource.MustParse(DefaultMAGCOAMCPU)),
		ListenPorts: getCPMVMListenPorts(),
	}
	r.Cards["1"] = &SRCard{
		Model:       common.ReturnPointerVal("iom-v"),
		MDAs:        common.GetPointerVal([]string{"m20-v"}),
		ReqMemory:   common.ReturnPointerVal(resource.MustParse(DefaultMAGCLBMEM)),
		ReqCPU:      common.ReturnPointerVal(resource.MustParse(DefaultMAGCLBCPU)),
		ListenPorts: getIOMVMListenPorts(),
	}
	r.Cards["2"] = &SRCard{
		Model:       common.ReturnPointerVal("iom-v-mg"),
		MDAs:        common.GetPointerVal([]string{"isa-ms-v", "isa-ms-v"}),
		ReqMemory:   common.ReturnPointerVal(resource.MustParse(DefaultMAGCMGMEM)),
		ReqCPU:      common.ReturnPointerVal(resource.MustParse(DefaultMAGCMGCPU)),
		ListenPorts: getIOMVMListenPorts(),
	}
	return r
}

// GetDefaultSysinfoStr return a default vsim/vsr/mag-c sysinfo string for the specified card
func (chassis *SRChassis) GetDefaultSysinfoStr(cardid string) string {
	var rs string
	if chassis.SFM != nil {
		rs = fmt.Sprintf("chassis=%v sfm=%v card=%v slot=%v ", *chassis.Model, *chassis.SFM, *chassis.Cards[cardid].Model, cardid)
	} else {
		//sfm is optional
		rs = fmt.Sprintf("chassis=%v card=%v slot=%v ", *chassis.Model, *chassis.Cards[cardid].Model, cardid)
	}
	card := chassis.Cards[cardid]
	if len(card.XIOM) > 0 {
		for xiomSlot, xiom := range card.XIOM {
			rs += fmt.Sprintf("xiom/%v=%v ", xiomSlot, xiom.Model)
			for i, mda := range xiom.MDAs {
				rs += fmt.Sprintf("mda/%v/%d=%v ", xiomSlot, i+1, mda)
			}
		}
	}
	if card.MDAs != nil {
		for i, mda := range *card.MDAs {
			rs += fmt.Sprintf("mda/%d=%v ", i+1, mda)

		}
	}
	if *chassis.Type == SRVMMAGC {
		if !common.IsCPM(cardid) {
			rs += "ofabric/1=2 "
			if strings.ToLower(strings.TrimSpace(*chassis.Cards[cardid].Model)) == "iom-v-mg" {
				rs += "control-cpu-cores=4 "
			} else {
				rs += "control-cpu-cores=2 "
			}
		}

	}
	return rs
}

// GetDefaultMDASlot return default IOM slot
func (chassis *SRChassis) GetDefaultMDASlot() string {
	iomList := []int{}
	if common.IsIntegratedChassis(*chassis.Model) {
		for slot := range chassis.Cards {
			return slot
		}
	} else {
		for slot := range chassis.Cards {
			if !common.IsCPM(slot) {
				slotNum, err := strconv.Atoi(strings.TrimSpace(slot))
				if err != nil {
					panic(err)
				}
				if *chassis.Type != SRVMMAGC {
					iomList = append(iomList, slotNum)
				} else {
					if strings.ToLower(*chassis.Cards[slot].Model) == "iom-v" {
						iomList = append(iomList, slotNum)
					}
				}
			}
		}
	}
	sort.Ints(iomList)
	if len(iomList) > 0 {
		return strconv.Itoa(iomList[0])
	}
	return "n/a"
}
