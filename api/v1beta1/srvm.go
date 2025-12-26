package v1beta1

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
	"syscall"

	"github.com/distribution/reference"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
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
	// ftp URL for the license file, a default URL is used if not specified
	// +optional
	// +nullable
	LicURL *string `json:"license,omitempty"`
}

const (
	DefaultSRVMDiskSize = "1.5Gi"
)

func (srvm *SRVM) setToAppDefVal() {
	srvm.DiskSize = common.ReturnPointerVal(resource.MustParse(DefaultSRVMDiskSize))
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
	if srvm.Chassis.Validate() != nil {
		return
	}
	for slot := range srvm.Chassis.Cards {
		srvm.Chassis.Cards[slot].SysInfo = common.SetDefaultGeneric(srvm.Chassis.Cards[slot].SysInfo, srvm.Chassis.GetDefaultSysinfoStr(slot))
	}
}

func (srvm *SRVM) Validate() error {
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
		// if url, err := url.Parse(*srvm.Image); err != nil {
		// 	return fmt.Errorf("invalid image url %v, %w", *srvm.Image, err)
		// } else {
		// 	switch strings.ToLower(url.Scheme) {
		// 	case "docker":
		// 	default:
		// 		return fmt.Errorf("only support http,https or docker url, %w", err)
		// 	}
		// }
	}
	if srvm.LicURL == nil {
		return fmt.Errorf("license not specified")
	}
	if url, err := url.Parse(*srvm.LicURL); err != nil {
		return fmt.Errorf("%v is not valid url", *srvm.LicURL)
	} else {
		if strings.ToLower(url.Scheme) != "ftp" {
			return fmt.Errorf("%v is not a ftp url", *srvm.LicURL)
		}
	}

	return srvm.Chassis.Validate()
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
