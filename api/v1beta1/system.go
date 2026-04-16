package v1beta1

import (
	"context"
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// +kubebuilder:object:generate=false
// +kubebuilder:object:root=false
// System interface is implemented by each node type
type System interface {
	//SetToAppDefVal set calling instance to its app defaults, only used to fill defaults for KNLConfig, not for defaulting lab
	SetToAppDefVal()
	//FillDefaultVal fill default values after defaults in KNLConfig are used
	//This is to have more advance defaulting logic, like defaulting based on nodeName in SRVM case
	FillDefaultVal(nodeName string)
	//Validate is used by validation webhook
	Validate(lab *LabSpec, nodeName string) error
	//Ensure is called by controller to reconcile
	//NOTE: currently forceRemoval is not used
	Ensure(ctx context.Context, nodeName string, clnt client.Client, forceRemoval bool) error
	//Shell is to shell into the system, used by knlcli
	Shell(ctx context.Context, clnt client.Client, ns, lab, chassis, username, passwd string)
	//Console is to login into system's console, not all system types support it
	Console(ctx context.Context, clnt client.Client, ns, lab, chassis string)
	//GetCfg return system's current running configuration like SROS config, not all system support this, emtyp+nil will be returned by non-supporting system
	GetCfg(ctx context.Context, clnt client.Client, ns, lab, chassis, user, pass string) (string, error)
	//LoadCfg loads config (e.g. SROS config) into system, not all system support this, non-supporting system just returns false,nil
	LoadCfg(ctx context.Context, clnt client.Client, ns, lab, chassis, user, pass, config string) (support bool, err error)
	//IsReady return nil if all components of system is ready
	IsReady(ctx context.Context, clnt client.Client, ns, labName, chassisName string) error
}

type NodeType string

const (
	Unknown NodeType = "unknown"
)

func (nt NodeType) MarshalText() (text []byte, err error) {
	return []byte(nt), nil
}

func (nt *NodeType) UnmarshalText(text []byte) error {
	*nt = NodeType(text)
	return nil
}

// NewSysRegistry contains mapping between node type and a new empty node with corresponding type
var NewSysRegistry = make(map[NodeType]func() System)

func GetNewSystemViaType(t NodeType) System {
	if f, ok := NewSysRegistry[t]; ok {
		return f()
	}
	return nil
}

func GetNodeTypeViaName(name string) NodeType {
	if i := strings.Index(name, "-"); i > 0 {
		return NodeType(name[:i])
	}
	return Unknown
}

func GetNewSystemViaName(name string) System {
	return GetNewSystemViaType(GetNodeTypeViaName(name))
}
