package v1beta1

import (
	"fmt"

	"kubenetlab.net/knl/common"
)

type SRSIMCard struct {
	Type *string `json:"type,omitempty"`
	// mdas and xioms are mutully exclusive
	MDAs *[]string `json:"mdas,omitempty"`
	// key is XIOM slot id, e.g. x1/x2; mdas and xioms are mutully exclusive
	XIOM map[string]XIOM `json:"xioms,omitempty"`
}

type XIOM struct {
	Type *string  `json:"type,omitempty"`
	MDAs []string `json:"mdas,omitempty"`
}

func (card *SRSIMCard) Validate() error {
	if len(card.XIOM) > 0 {
		if card.MDAs != nil {
			if len(*card.MDAs) > 0 {
				return fmt.Errorf("mdas and xioms are mutully exclusive, can't be both specified")
			}
		}

	}
	return nil
}

type SRChassis struct {
	Type  *string               `json:"type,omitempty"`
	Cards map[string]*SRSIMCard `json:"cards,omitempty"` //key is slot id, "A","B" for CPM, number for IOM
	SFM   *string               `json:"sfm,omitempty"`
}

func (chassis *SRChassis) Validate() error {
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
func DefaultChassis() *SRChassis {
	r := &SRChassis{
		Type: common.ReturnPointerVal("SR-7"),
		SFM:  common.ReturnPointerVal("m-sfm5-7"),
	}
	r.Cards = make(map[string]*SRSIMCard)
	r.Cards["A"] = &SRSIMCard{
		Type: common.ReturnPointerVal("cpm5"),
	}
	r.Cards["1"] = &SRSIMCard{
		Type: common.ReturnPointerVal("iom4-e"),
		MDAs: common.GetPointerVal([]string{"me10-10gb-sfp+", "isa2-tunnel"}),
	}
	return r
}

// GetSysinfoStr return a vsim/vsr/mag-c sysinfo string for the specified card
func (chassis *SRChassis) GetSysinfoStr(cardid string) string {
	rs := fmt.Sprintf("chassis=%v sfm=%v card=%v ", *chassis.Type, *chassis.SFM, chassis.Cards[cardid].Type)
	// if len(chassis.Cards[cardid].MDAs) == 2 {

	// }
	return rs
}
