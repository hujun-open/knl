package common

import (
	"context"
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// System interface is implemented by each node type
type System interface {
	//SetToAppDefVal set calling instance to its app defaults, only used to fill defaults for KNLConfig, not for defaulting lab
	SetToAppDefVal()
	//FillDefaultVal fill default values after defaults in KNLConfig are used
	//This is to have more advance defaulting logic, like defaulting based on nodeName in SRVM case
	FillDefaultVal(nodeName string)
	//Validate is used by validation webhook
	Validate() error
	//Ensure is called by controller to reconcile
	Ensure(ctx context.Context, nodeName string, clnt client.Client, forceRemoval bool) error
	//Shell is to shell into the system, used by knlcli
	Shell(ctx context.Context, clnt client.Client, ns, lab, chassis, username string)
	//Console is to login into system's console, not all system types support it
	Console(ctx context.Context, clnt client.Client, ns, lab, chassis string)
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
var NewSysRegistry map[NodeType]func() System

func init() {
	NewSysRegistry = make(map[NodeType]func() System)
}

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
