package v1beta1

import (
	"context"
	"fmt"
	"log"
	"net/netip"
	"os"
	"strings"
	"syscall"

	"github.com/distribution/reference"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func init() {
	newvsimf := func() System { return new(VSIM) }
	NewSysRegistry[SRVMVSIM] = newvsimf
	newvsrif := func() System { return new(VSRI) }
	NewSysRegistry[SRVMVSRI] = newvsrif
	newmagcf := func() System { return new(MAGC) }
	NewSysRegistry[SRVMMAGC] = newmagcf
}

const (
	SRVMVSIM NodeType = "vsim"
	SRVMVSRI NodeType = "vsri"
	SRVMMAGC NodeType = "magc"
)

// VSIM specifies a Nokia vSIM router
type VSIM SRVM

// VSRI specifies a Nokia VSR-I router
type VSRI SRVM

// MAGC specifies a Nokia MAG-c
type MAGC SRVM

const (
	FTPImagePrefix = "filesvr:"
)

// undelying type for VSIM, VSRI, and MAGC
type SRVM struct {
	//specifies chassis configuration
	// +optional
	// +nullable
	Chassis *SRChassis `json:"chassis,omitempty"`
	// one of three types of image loading method:
	// 1.docker image url like "exampleregistry/sros:25.10.1";
	// 2.sub folder name of the SROS/MAGC image when start with "filesvr:", like "filesvr:25.10.1"
	// +optional
	// +nullable
	Image *string `json:"image,omitempty"`
	//Disk size for the CPM, only used when image is a docker image, must >= image size
	// +optional
	// +nullable
	DiskSize *resource.Quantity `json:"diskSize,omitempty"`
	// a k8s secret name contains license with key "license"
	// +optional
	// +nullable
	License *string `json:"license,omitempty"`
	// VM's firmware UUID
	// +optional
	// +nullable
	UUID *string `json:"uuid,omitempty"`
	// if true, allocate dedicate cpu and huge page memory;
	// recommand to set to true in case of vsr and magc
	// +optional
	// +nullable
	Dedicate *bool `json:"dedicate,omitempty"`
}

const (
	DefaultSRVMDiskSize   = "1.5Gi"
	DefaultVSIMLicSecName = "vsimlic"
	DefaultVSRLicSecName  = "vsrlic"
	DefaultMAGCLicSecName = "magclic"
	SRVMConsoleTCPPort    = 2222
)

func (srvm *SRVM) setToAppDefVal() {
	srvm.DiskSize = ReturnPointerVal(resource.MustParse(DefaultSRVMDiskSize))
	srvm.Dedicate = ReturnPointerVal(false)
}

func (vsim *VSIM) SetToAppDefVal() {
	(*SRVM)(vsim).setToAppDefVal()
	vsim.Chassis = DefaultSIMChassis(SRVMVSIM)
	vsim.License = ReturnPointerVal(string(DefaultVSIMLicSecName))
}
func (vsim *VSIM) Validate(lab *LabSpec, nodeName string) error {
	return (*SRVM)(vsim).Validate(lab, nodeName)
}
func (vsim *VSIM) Ensure(ctx context.Context, nodeName string, clnt client.Client, forceRemoval bool) error {
	return (*SRVM)(vsim).Ensure(ctx, nodeName, clnt, forceRemoval)
}

func (vsim *VSIM) FillDefaultVal(name string) {
	vsim.Chassis.Type = ReturnPointerVal(SRVMVSIM)
	(*SRVM)(vsim).FillDefaultVal(name)

}

func (vsri *VSRI) SetToAppDefVal() {
	(*SRVM)(vsri).setToAppDefVal()
	vsri.Chassis = DefaultVSRIChassis()
	vsri.License = ReturnPointerVal(string(DefaultVSRLicSecName))
}

func (vsri *VSRI) Validate(lab *LabSpec, nodeName string) error {
	return (*SRVM)(vsri).Validate(lab, nodeName)
}
func (vsri *VSRI) Ensure(ctx context.Context, nodeName string, clnt client.Client, forceRemoval bool) error {
	return (*SRVM)(vsri).Ensure(ctx, nodeName, clnt, forceRemoval)
}

func (vsri *VSRI) FillDefaultVal(name string) {
	vsri.Chassis.Type = ReturnPointerVal(SRVMVSRI)
	(*SRVM)(vsri).FillDefaultVal(name)

}

func (magc *MAGC) SetToAppDefVal() {
	(*SRVM)(magc).setToAppDefVal()
	magc.Chassis = DefaultMAGCChassis()
	magc.License = ReturnPointerVal(string(DefaultMAGCLicSecName))
}

func (magc *MAGC) Validate(lab *LabSpec, nodeName string) error {
	return (*SRVM)(magc).Validate(lab, nodeName)
}
func (magc *MAGC) Ensure(ctx context.Context, nodeName string, clnt client.Client, forceRemoval bool) error {
	return (*SRVM)(magc).Ensure(ctx, nodeName, clnt, forceRemoval)
}

func (magc *MAGC) FillDefaultVal(name string) {
	magc.Chassis.Type = ReturnPointerVal(SRVMMAGC)
	(*SRVM)(magc).FillDefaultVal(name)

}

func (srvm *SRVM) FillDefaultVal(name string) {
	if srvm.Chassis.Validate() != nil {
		return
	}
	for slot := range srvm.Chassis.Cards {
		srvm.Chassis.Cards[slot].SysInfo = SetDefaultGeneric(srvm.Chassis.Cards[slot].SysInfo, srvm.Chassis.GetDefaultSysinfoStr(slot))
		srvm.Chassis.Cards[slot].FillDefaultVal(*srvm.Chassis.Type, slot)
	}
}

func (srvm *SRVM) Validate(lab *LabSpec, nodeName string) error {
	if srvm.Chassis == nil {
		return fmt.Errorf("chassis not specified")
	}
	if srvm.Image == nil {
		return fmt.Errorf("image not specified")
	}
	if !strings.HasPrefix(*srvm.Image, FTPImagePrefix) {
		if _, err := reference.Parse(*srvm.Image); err != nil {
			return fmt.Errorf("%v is not valid image url, %w", *srvm.Image, err)
		}
	}
	if srvm.License == nil {
		return fmt.Errorf("license not specified")
	}
	if srvm.Dedicate == nil {
		return fmt.Errorf("dedidcate not specified")
	}
	for linkName, link := range lab.LinkList {
		for _, c := range link.Connectors {
			if c.PortId != nil && *c.NodeName == nodeName {
				if _, ok := srvm.Chassis.Cards[*c.PortId]; !ok {
					return fmt.Errorf("port %v of node %v in link %v doesn't exists in its chassis spec", *c.PortId, nodeName, linkName)
				}
			}
		}
	}
	return srvm.Chassis.Validate()
}

func GetSRVMviaSys(nodeName string, sys System) *SRVM {
	switch GetNodeTypeViaName(nodeName) {
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
func (gvm *SRVM) getSRVMCPMPodIP(ctx context.Context, clnt client.Client, ns, lab, chassis string) (netip.Addr, error) {
	defCPMVMName := GetSRVMCardVMName(lab, chassis, gvm.Chassis.GetDefaultCPMSlot())
	podList := &corev1.PodList{}
	labelSelector := client.MatchingLabels{
		"vm.kubevirt.io/name": defCPMVMName,
	}
	err := clnt.List(ctx, podList, client.InNamespace(ns), labelSelector)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("failed to list pods: %v", err)

	}
	if len(podList.Items) == 0 {
		return netip.Addr{}, fmt.Errorf("failed to find vm pod %v", defCPMVMName)
	}
	return netip.MustParseAddr(podList.Items[0].Status.PodIP), nil
}

func (gvm *SRVM) Shell(ctx context.Context, clnt client.Client, ns, lab, chassis, username string) {
	cpmIP, err := gvm.getSRVMCPMPodIP(ctx, clnt, ns, lab, chassis)
	if err != nil {
		log.Fatal(err)
	}
	if username == "" {
		username = "admin"
	}
	fmt.Println("connecting to", chassis, "at", cpmIP, "username", username)
	SysCallSSH(username, cpmIP.String())
}

func (vsim *VSIM) Console(ctx context.Context, clnt client.Client, ns, lab, chassis string) {
	(*SRVM)(vsim).Console(ctx, clnt, ns, lab, chassis)
}

func (vsri *VSRI) Console(ctx context.Context, clnt client.Client, ns, lab, chassis string) {
	(*SRVM)(vsri).Console(ctx, clnt, ns, lab, chassis)
}
func (magc *MAGC) Console(ctx context.Context, clnt client.Client, ns, lab, chassis string) {
	(*SRVM)(magc).Console(ctx, clnt, ns, lab, chassis)
}

func (gvm *SRVM) Console(ctx context.Context, clnt client.Client, ns, lab, chassis string) {
	cpmIP, err := gvm.getSRVMCPMPodIP(ctx, clnt, ns, lab, chassis)
	if err != nil {
		log.Fatal(err)
	}
	envList := []string{fmt.Sprintf("HOME=%v", os.Getenv("HOME"))}
	fmt.Println("connecting to console of", chassis, "at", cpmIP)
	syscall.Exec("/bin/sh",
		[]string{"sh", "-c",
			fmt.Sprintf("telnet %v %d", cpmIP, SRVMConsoleTCPPort)},
		envList)
}
func (magc *MAGC) GetCfg(ctx context.Context, clnt client.Client, ns, lab, chassis, user, pass string) (string, error) {
	cpmIP, err := (*SRVM)(magc).getSRVMCPMPodIP(ctx, clnt, ns, lab, chassis)
	if err != nil {
		return "", fmt.Errorf("failed to get cpm IP for %v in lab %v, %w", chassis, lab, err)
	}
	return srGetCfgviaSSHClassicCLI(cpmIP, user, pass)

}
func (vsri *VSRI) GetCfg(ctx context.Context, clnt client.Client, ns, lab, chassis, user, pass string) (string, error) {
	return (*SRVM)(vsri).GetCfg(ctx, clnt, ns, lab, chassis, user, pass)
}
func (vsim *VSIM) GetCfg(ctx context.Context, clnt client.Client, ns, lab, chassis, user, pass string) (string, error) {
	return (*SRVM)(vsim).GetCfg(ctx, clnt, ns, lab, chassis, user, pass)
}
func (gvm *SRVM) GetCfg(ctx context.Context, clnt client.Client, ns, lab, chassis, user, pass string) (string, error) {
	cpmIP, err := gvm.getSRVMCPMPodIP(ctx, clnt, ns, lab, chassis)
	if err != nil {
		return "", fmt.Errorf("failed to get cpm IP for %v in lab %v, %w", chassis, lab, err)
	}
	return srGetCfgviaGNMI(cpmIP, user, pass)
}

func (magc *MAGC) LoadCfg(ctx context.Context, clnt client.Client, ns, lab, chassis, user, pass, config string) (bool, error) {
	cpmIP, err := (*SRVM)(magc).getSRVMCPMPodIP(ctx, clnt, ns, lab, chassis)
	if err != nil {
		return true, fmt.Errorf("failed to get cpm IP for %v in lab %v, %w", chassis, lab, err)
	}
	return true, srLoadCfgviaSSHClassicCLI(cpmIP, user, pass, config)
}
func (vsri *VSRI) LoadCfg(ctx context.Context, clnt client.Client, ns, lab, chassis, user, pass, config string) (bool, error) {
	return (*SRVM)(vsri).LoadCfg(ctx, clnt, ns, lab, chassis, user, pass, config)
}
func (vsim *VSIM) LoadCfg(ctx context.Context, clnt client.Client, ns, lab, chassis, user, pass, config string) (bool, error) {
	return (*SRVM)(vsim).LoadCfg(ctx, clnt, ns, lab, chassis, user, pass, config)
}
func (gvm *SRVM) LoadCfg(ctx context.Context, clnt client.Client, ns, lab, chassis, user, pass, config string) (bool, error) {
	cpmIP, err := gvm.getSRVMCPMPodIP(ctx, clnt, ns, lab, chassis)
	if err != nil {
		return true, fmt.Errorf("failed to get cpm IP for %v in lab %v, %w", chassis, lab, err)
	}
	return true, srLoadCfgviaGNMI(cpmIP, user, pass, config)
}

func (vsim *VSIM) IsReady(ctx context.Context, clnt client.Client, ns, labName, chassisName string) error {
	return (*SRVM)(vsim).IsReady(ctx, clnt, ns, labName, chassisName)
}

func (vsri *VSRI) IsReady(ctx context.Context, clnt client.Client, ns, labName, chassisName string) error {
	return (*SRVM)(vsri).IsReady(ctx, clnt, ns, labName, chassisName)
}

func (magc *MAGC) IsReady(ctx context.Context, clnt client.Client, ns, labName, chassisName string) error {
	return (*SRVM)(magc).IsReady(ctx, clnt, ns, labName, chassisName)
}
func (gvm *SRVM) IsReady(ctx context.Context, clnt client.Client, ns, labName, chassisName string) error {
	for slot := range gvm.Chassis.Cards {
		vmName := GetSRVMCardVMName(labName, chassisName, slot)
		if err := IsVMIReady(ctx, clnt, ns, vmName); err != nil {
			return err
		}
	}
	return nil
}
