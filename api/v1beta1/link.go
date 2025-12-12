package v1beta1

import (
	"context"
	"fmt"

	"github.com/bits-and-blooms/bitset"
	k8slan "github.com/hujun-open/k8slan/api/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"kubenetlab.net/knl/common"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Link struct {
	//+required
	Connectors []Connector `json:"nodes"`
	GWAddr     *string     `json:"gwAddr,omitempty"` //a prefix
	MTU        *uint16     `json:"mtu,omitempty"`
}

type Connector struct {
	//+required
	NodeName *string `json:"node"`           //node name, in case of distruited system like vsim/mag-c, it is the name of IOM VM
	PortId   *string `json:"port,omitempty"` //only used by srsim for mda port id
	Addr     *string `json:"addr,omitempty"` //a prefix, used for cloud-init on vmLinux, and podlinux
	Mac      *string `json:"mac,omitempty"`  //used for cloud-init on vmLinux, and podlinux
}

func Getk8lanName(lab, link string) string {
	return lab + "-" + link
}
func Getk8lanBRName(lab, link string) string {
	return "k8slanbr"
}
func Getk8lanVxName(lab, link string, vni int32) string {
	return fmt.Sprintf("klanvx%d", vni)
}

// This function requires controller's MaxConcurrentReconciles == 1, otherwise there is race issue
func GetAvailableVNI(ctx context.Context, clnt client.Client, requiredNum int) (int32, error) {
	const maxVNI = 16777215
	bset := bitset.New(maxVNI)
	bset = bset.Set(0)
	lans := new(k8slan.LANList)
	err := clnt.List(ctx, lans)
	if err != nil {
		return -1, fmt.Errorf("failed to list lans, %w", err)
	}
	for _, lan := range lans.Items {
		bset.Set(uint(*lan.Spec.VNI))
	}

	next, ok := bset.NextClear(0)
	if !ok {
		return -1, fmt.Errorf("no available vni to allocate")
	}
	if next > maxVNI {
		return -1, fmt.Errorf("no available vni to allocate")
	}
	return int32(next), nil
}

const (
	VxLANPort     int32 = 48622
	FinalizerName       = "lab.kubenetlab.net/finalizer"
)

func getSpokeName(vni int32, connectorIndex int) string {
	return fmt.Sprintf("klan%d-%d", vni, connectorIndex)
}

// this creates k8slan CR for all links
// return two maps, first map: 1st key is nodename, 2nd key is LAN name, val is list of spoke name
// 2nd map: key is spokename, value is corrsponding connector
func (plab *ParsedLab) EnsureLinks(ctx context.Context, clnt client.Client) (map[string]map[string][]string, map[string]*Connector, error) {
	gconf := GCONF.Get()
	rmap := make(map[string]map[string][]string)
	spokeConnectorMap := make(map[string]*Connector)
	for linkName, link := range plab.Lab.Spec.LinkList {
		lan := new(k8slan.LAN)
		err := clnt.Get(ctx,
			types.NamespacedName{Namespace: plab.Lab.Namespace, Name: Getk8lanName(plab.Lab.Name, linkName)},
			lan,
		)
		if err != nil {
			if !apierrors.IsNotFound(err) {
				return nil, nil, fmt.Errorf("unexpected error getting existing LAN %v, %w", Getk8lanName(plab.Lab.Name, linkName), err)
			}
			//not found, create new one
			vni, err := GetAvailableVNI(ctx, clnt, len(plab.Lab.Spec.LinkList))
			if err != nil {
				return nil, nil, fmt.Errorf("failed to get %d vni, %w", len(plab.Lab.Spec.LinkList), err)
			}
			lan = &k8slan.LAN{
				ObjectMeta: common.GetObjMeta(Getk8lanName(plab.Lab.Name, linkName), plab.Lab.Name, plab.Lab.Namespace),
				Spec: k8slan.LANSpec{
					NS:           common.GetPointerVal(Getk8lanName(plab.Lab.Name, linkName)),
					BridgeName:   common.GetPointerVal(Getk8lanBRName(plab.Lab.Name, linkName)),
					VxLANName:    common.GetPointerVal(Getk8lanVxName(plab.Lab.Name, linkName, vni)),
					VNI:          common.GetPointerVal(vni),
					DefaultVxDev: *gconf.VXLANDefaultDev,
					VxDevMap:     gconf.VxDevMap,
					VxPort:       common.GetPointerVal(VxLANPort),
					VxLANGrp:     gconf.VXLANGrpAddr,
					SpokeList:    []string{},
				},
			}
			lan.Finalizers = append(lan.Finalizers, FinalizerName)
		}
		for i, c := range link.Connectors {
			spokeName := getSpokeName(*lan.Spec.VNI, i)
			lan.Spec.SpokeList = append(lan.Spec.SpokeList, spokeName)
			if _, ok := rmap[*c.NodeName]; !ok {
				rmap[*c.NodeName] = make(map[string][]string)
			}
			if _, ok := rmap[*c.NodeName][linkName]; !ok {
				rmap[*c.NodeName][linkName] = []string{}
			}
			rmap[*c.NodeName][linkName] = append(rmap[*c.NodeName][linkName], spokeName)
			spokeConnectorMap[spokeName] = &c
		}

		err = createIfNotExistsOrRemove(ctx, clnt, plab, lan, true, false)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create LAN CR for lab %v link %v, %w", plab.Lab.Name, linkName, err)
		}

	}
	return rmap, spokeConnectorMap, nil
}
