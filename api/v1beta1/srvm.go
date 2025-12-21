package v1beta1

import (
	"context"
	"fmt"
	"log"
	"os"
	"syscall"

	corev1 "k8s.io/api/core/v1"
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

func (gvm *SRVM) setToAppDefVal() {
	gvm.LicURL = common.ReturnPointerVal(fmt.Sprintf("ftp://ftp:ftp@%v/lic", common.FixedFTPProxySvr))
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

func (gvm *SRVM) FillDefaultVal(name string) {
	for slot := range gvm.Chassis.Cards {
		gvm.Chassis.Cards[slot].SysInfo = common.SetDefaultGeneric(gvm.Chassis.Cards[slot].SysInfo, gvm.Chassis.GetDefaultSysinfoStr(slot))
	}
}

func (gvm *SRVM) Validate() error {
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

func (vsim *VSIM) Shell(ctx context.Context, clnt client.Client, ns, lab, chassis, username string) {
	(*SRVM)(vsim).Shell(ctx, clnt, ns, lab, chassis, username)
}

func (vsri *VSRI) Shell(ctx context.Context, clnt client.Client, ns, lab, chassis, username string) {
	(*SRVM)(vsri).Shell(ctx, clnt, ns, lab, chassis, username)
}
func (magc *MAGC) Shell(ctx context.Context, clnt client.Client, ns, lab, chassis, username string) {
	(*SRVM)(magc).Shell(ctx, clnt, ns, lab, chassis, username)
}
func (gvm *SRVM) Shell(ctx context.Context, clnt client.Client, ns, lab, chassis, username string) {
	defCPMVMName := getSRVMCardVMName(lab, chassis, gvm.Chassis.GetDefaultCPMSlot())
	podList := &corev1.PodList{}
	labelSelector := client.MatchingLabels{
		"vm.kubevirt.io/name": defCPMVMName,
	}
	err := clnt.List(ctx, podList, client.InNamespace(ns), labelSelector)
	if err != nil {
		log.Fatalf("failed to list pods: %v", err)
	}
	if len(podList.Items) == 0 {
		log.Fatalf("failed to find vm pod %v", defCPMVMName)

	}
	envList := []string{fmt.Sprintf("HOME=%v", os.Getenv("HOME"))}
	if username == "" {
		username = "admin"
	}
	fmt.Println("connecting to", chassis, "at", podList.Items[0].Status.PodIP, "username", username)
	syscall.Exec("/bin/sh",
		[]string{"sh", "-c",
			fmt.Sprintf("ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null %v@%v", username, podList.Items[0].Status.PodIP)},
		envList)

}
