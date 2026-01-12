package v1beta1

import (
	"context"
	"fmt"
	"strconv"

	"github.com/bits-and-blooms/bitset"
	k8slan "github.com/hujun-open/k8slan/api/v1beta1"
	ncv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Link defines a layer2 connection between nodes,
// two or more nodes per link are supported.
type Link struct {
	//+required
	//a list of nodes connect to the link's layer2 network
	Connectors []Connector `json:"nodes"`
	GWAddr     *string     `json:"gwAddr,omitempty"` //a prefix
}

func (link *Link) Validate() error {
	if len(link.Connectors) < 2 {
		return fmt.Errorf("the minimal number of nodes per link is 2")
	}
	return nil
}

// Connector specifies a node name and how it connects to the link
type Connector struct {
	//+required
	//name of node connects to the link
	NodeName *string `json:"node"` //node name
	//used by srsim for mda port id, by SRVM for IOM slot id and by SRL for interface id
	PortId *string `json:"port,omitempty"`
	Addr   *string `json:"addr,omitempty"` //a prefix, used for cloud-init on vmLinux, and podlinux
	Mac    *string `json:"mac,omitempty"`  //used for cloud-init on vmLinux, and podlinux
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
func GetAvailableBrIndex(ctx context.Context, clnt client.Client, requiredNum int) ([]uint, error) {
	const maxBridgeIndex = 16777215
	bset := bitset.New(maxBridgeIndex)
	bset = bset.Set(0)
	nads := new(ncv1.NetworkAttachmentDefinitionList)
	req, err := labels.NewRequirement(BridgeIndexLabelKey, selection.Exists, nil)
	if err != nil {
		return nil, err
	}
	selector := labels.NewSelector().Add(*req)
	err = clnt.List(ctx, nads, client.MatchingLabelsSelector{Selector: selector})
	if err != nil {
		return nil, fmt.Errorf("failed to list net-attach-def, %w", err)
	}
	for _, nad := range nads.Items {
		if val, ok := nad.Labels[BridgeIndexLabelKey]; ok {
			i, err := strconv.Atoi(val)
			if err != nil {
				return nil, fmt.Errorf("invalid %v value found in NAD %v: %v, %w", BridgeIndexLabelKey, val, nad.Name, err)
			}
			bset.Set(uint(i))
		}
	}
	rlist := []uint{}
	var next uint = 0
	var ok bool
	for i := 0; i < requiredNum; i++ {
		next, ok = bset.NextClear(next)
		if !ok {
			return nil, fmt.Errorf("no available bridge index to allocate")
		}
		if next > maxBridgeIndex {
			return nil, fmt.Errorf("no available bridge index to allocate")
		}
		rlist = append(rlist, next)
		next += 1
	}
	return rlist, nil
}

// This function requires controller's MaxConcurrentReconciles == 1, otherwise there is race issue
func GetAvailableVNIs(ctx context.Context, clnt client.Client, requiredNum int) ([]int32, error) {
	const maxVNI = 16777215
	bset := bitset.New(maxVNI)
	bset = bset.Set(0)
	lans := new(k8slan.LANList)
	err := clnt.List(ctx, lans)
	if err != nil {
		return nil, fmt.Errorf("failed to list lans, %w", err)
	}
	rlist := make([]int32, requiredNum)
	for _, lan := range lans.Items {
		bset = bset.Set(uint(*lan.Spec.VNI))
	}
	var next uint = 0
	var ok bool
	for i := 0; i < requiredNum; i++ {
		next, ok = bset.NextClear(next)
		if !ok {
			return nil, fmt.Errorf("no available vni to allocate")
		}
		if next > maxVNI {
			return nil, fmt.Errorf("no available vni to allocate")
		}
		rlist[i] = int32(next)
		next += 1
	}
	return rlist, nil
}

const (
	VxLANPort     int32 = 48622
	FinalizerName       = "lab.kubenetlab.net/finalizer"
)

func getSpokeName(vni int32, connectorIndex int) string {
	return fmt.Sprintf("klan%d-%d", vni, connectorIndex)
}

// this creates k8slan CR for all links
// return two maps, first map: 1st key is nodename, 2nd key is link name, val is list of spoke name
// 2nd map: key is spokename, value is corrsponding connector
func (plab *ParsedLab) EnsureLinks(ctx context.Context, clnt client.Client) error {
	gconf := GCONF.Get()
	if plab.SpokeConnectorMap == nil {
		plab.SpokeConnectorMap = make(map[string]*Connector)
	}
	if plab.SpokeMap == nil {
		plab.SpokeMap = make(map[string]map[string][]string)
	}
	if plab.SpokeLinkMap == nil {
		plab.SpokeLinkMap = make(map[string]string)
	}

	vniList, err := GetAvailableVNIs(ctx, clnt, len(plab.Lab.Spec.LinkList))
	if err != nil {
		return fmt.Errorf("failed to get %d vni, %w", len(plab.Lab.Spec.LinkList), err)
	}
	for i, linkName := range GetSortedKeySlice(plab.Lab.Spec.LinkList) {
		// for linkName, link := range plab.Lab.Spec.LinkList {
		link := plab.Lab.Spec.LinkList[linkName]
		lan := new(k8slan.LAN)
		err := clnt.Get(ctx,
			types.NamespacedName{Namespace: plab.Lab.Namespace, Name: Getk8lanName(plab.Lab.Name, linkName)},
			lan,
		)
		if err != nil {
			if !apierrors.IsNotFound(err) {
				return fmt.Errorf("unexpected error getting existing LAN %v, %w", Getk8lanName(plab.Lab.Name, linkName), err)
			}
			//not found, create new one
			vni := vniList[i]
			lan = &k8slan.LAN{
				ObjectMeta: GetObjMeta(Getk8lanName(plab.Lab.Name, linkName), plab.Lab.Name, plab.Lab.Namespace, "", ""),
				Spec: k8slan.LANSpec{
					NS:           GetPointerVal(Getk8lanName(plab.Lab.Name, linkName)),
					BridgeName:   GetPointerVal(Getk8lanBRName(plab.Lab.Name, linkName)),
					VxLANName:    GetPointerVal(Getk8lanVxName(plab.Lab.Name, linkName, vni)),
					VNI:          GetPointerVal(vni),
					DefaultVxDev: *gconf.VXLANDefaultDev,
					VxDevMap:     gconf.VxDevMap,
					VxPort:       GetPointerVal(VxLANPort),
					VxLANGrp:     gconf.VXLANGrpAddr,
					SpokeList:    []string{},
				},
			}
			lan.Finalizers = append(lan.Finalizers, FinalizerName)
		}
		for i, c := range link.Connectors {
			spokeName := getSpokeName(*lan.Spec.VNI, i)
			lan.Spec.SpokeList = append(lan.Spec.SpokeList, spokeName)
			if _, ok := plab.SpokeMap[*c.NodeName]; !ok {
				plab.SpokeMap[*c.NodeName] = make(map[string][]string)
			}
			if _, ok := plab.SpokeMap[*c.NodeName][linkName]; !ok {
				plab.SpokeMap[*c.NodeName][linkName] = []string{}
			}
			plab.SpokeMap[*c.NodeName][linkName] = append(plab.SpokeMap[*c.NodeName][linkName], spokeName)
			plab.SpokeConnectorMap[spokeName] = &c
			plab.SpokeLinkMap[spokeName] = linkName
		}

		err = createIfNotExistsOrRemove(ctx, clnt, plab, lan, true, false)
		if err != nil {
			return fmt.Errorf("failed to create LAN CR for lab %v link %v, %w", plab.Lab.Name, linkName, err)
		}

	}

	return nil
}
