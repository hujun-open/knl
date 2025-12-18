package common

import (
	"bytes"
	"cmp"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/dchest/siphash"
	ncv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	K8SLABELAPPVAL       = `kubenetlab`
	K8SLABELAPPKey       = `app.kubernetes.io/name`
	K8SLABELSETUPKEY     = `lab.kubenetlab.net/name`
	K8SLABELNodeKEY      = `node.kubenetlab.net/name`
	BridgeIndexLabelKey  = "bridge.kubenetlab.net/index"
	KNLROOTName          = `knlroot`
	VMDiskSubFolder      = `vmdisks`
	IMGSubFolder         = `imgs`
	CfgSubFolder         = `cfgs`
	KVirtPodPVCMountRoot = `/var/run/kubevirt-private/vmi-disks/`
	PVCName              = `knl-pvc` //this must be inline with config/default/pvc
)

const (
	macVTAPResourceKey       = `k8s.v1.cni.cncf.io/resourceName`
	macVTAPResourceValPrefix = `macvtap.network.kubevirt.io`
	NADAnnonKey              = `k8s.v1.cni.cncf.io/networks`
)

// SetDefaultStr return inval if it is not "", otherwise return defval
func SetDefaultStr(inval, defval string) string {
	if inval == "" {
		return defval
	}
	return inval
}

// SetDefaultGeneric return inval if it is not nil, otherwise return defVal
func SetDefaultGeneric[T any](inval *T, defVal T) *T {
	if inval != nil {
		return inval
	}
	r := new(T)
	*r = defVal
	return r
}

// FillNilPointers copies pointer fields from src into dst when dst's pointer fields are nil.
// dst must be a pointer to a struct; src must be a struct of the same concrete type.
//
// Notes:
// - Only exported (settable) fields are touched.
// - If src field is a pointer and non-nil, the pointer value is copied (both dst and src will point to the same underlying value).
// - If src field is a non-pointer value assignable to the pointer element type, a new pointer is allocated and its value copied.
func FillNilPointers(dst any, src any) error {

	srcVal := reflect.ValueOf(src)
	if !srcVal.IsValid() {
		return fmt.Errorf("src value is not valid")
	}
	if srcVal.Kind() == reflect.Interface {
		srcVal = srcVal.Elem()
	}
	dstVal := reflect.ValueOf(dst)
	if !dstVal.IsValid() {
		return fmt.Errorf("dst value is not valid")
	}
	if dstVal.Kind() == reflect.Interface {
		dstVal = dstVal.Elem()
	}
	if dstVal.Kind() != reflect.Ptr || dstVal.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("dst must be pointer to struct")
	}

	if srcVal.Kind() == reflect.Pointer {
		srcVal = srcVal.Elem()
	}
	if srcVal.Kind() != reflect.Struct {
		return fmt.Errorf("src must be a struct, but got a %v, %v", srcVal.Type().String(), srcVal.Kind())
	}
	if srcVal.Type() != dstVal.Elem().Type() {
		return fmt.Errorf("src type %s doesn't match dst type %s", srcVal.Type(), dstVal.Elem().Type())
	}

	fillNilPointersValue(dstVal.Elem(), srcVal)
	return nil
}

// recursive helper: dstStruct and srcStruct are reflect.Values of kind Struct (addressable for dst).
func fillNilPointersValue(dstStruct, srcStruct reflect.Value) {
	for i := 0; i < dstStruct.NumField(); i++ {
		dstField := dstStruct.Field(i)
		srcField := srcStruct.Field(i)

		// skip unexported / unsettable fields
		if !dstField.CanSet() {
			continue
		}
		switch dstField.Kind() {
		case reflect.Pointer:
			if dstField.Type().Elem().Kind() == reflect.Struct {
				//if pointer to a struct, go downif src field is not nil
				if !srcField.IsNil() {
					if dstField.IsNil() {
						dstField.Set(reflect.New(dstField.Type().Elem()))
					}
					fillNilPointersValue(dstField.Elem(), srcField.Elem())
				}
			} else {
				//if not pointer to a struct
				if dstField.IsNil() && !srcField.IsNil() {
					dstField.Set(srcField)
				}
			}
		case reflect.Map:
			if dstField.IsNil() && !srcField.IsNil() {
				dstField.Set(srcField)
			}
		}
		// For other possible pointer-like container types (slices/interfaces) we intentionally do nothing
	}
}

// DO NOT Use this function for API defaulting, because it will create new pointer for all nil fields,
// which is against of purpose of nil pointer field (meaning it has no user input)
// NewStructPointerFields create a new value for all pointer top level fields of s
// s must be a pointer to struct
func NewStructPointerFields(s any) error {
	val := reflect.ValueOf(s)
	if val.Kind() != reflect.Pointer {
		return fmt.Errorf("not a pointer")
	}
	val = val.Elem()
	if val.Kind() != reflect.Struct {
		return fmt.Errorf("not a pointer to struct")
	}
	for i := 0; i < val.NumField(); i++ {
		if val.Field(i).Kind() == reflect.Pointer {
			newval := reflect.New(val.Field(i).Type().Elem())
			val.Field(i).Set(newval)
		}
	}
	return nil
}

// ReturnPointerVal return a new pointer type *T, points to a value val
func ReturnPointerVal[T any](val T) *T {
	r := new(T)
	*r = val
	return r
}

var BaseMACAddr = net.HardwareAddr{0x12, 32, 34, 0, 0, 0}

func DeriveMac(basemac net.HardwareAddr, offset int) net.HardwareAddr {
	buf := make([]byte, 2)
	buf = append(buf, basemac...)
	basenum := binary.BigEndian.Uint64(buf)
	basenum += uint64(offset)
	binary.BigEndian.PutUint64(buf, basenum)
	return buf[2:8]
}

func MakeErr(ierr error) error {
	var buf bytes.Buffer
	pc, _, line, _ := runtime.Caller(1)
	logger := log.New(&buf, "", 0)
	logger.Printf("[%s:%d]: %v", runtime.FuncForPC(pc).Name(), line, ierr)
	return errors.New(buf.String())
}

func MakeErrviaStr(errs string) error {
	var buf bytes.Buffer
	pc, _, line, _ := runtime.Caller(1)
	logger := log.New(&buf, "", 0)
	logger.Printf("[%s:%d]: %v", runtime.FuncForPC(pc).Name(), line, errs)
	return errors.New(buf.String())
}

func GetObjMeta(objName, labName, labNS string) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:      objName,
		Namespace: labNS,
		Labels: map[string]string{
			K8SLABELAPPKey:   K8SLABELAPPVAL,
			K8SLABELSETUPKEY: labName,
		},
	}
}

func NewFBBridgeNetworkDef(nsName, labName, brName string, brIndex, mtu int) *ncv1.NetworkAttachmentDefinition {
	const specTempalte = `
	{
		"cniVersion": "0.3.1",
		"name": "%v",
		"type": "bridge",
		"mtu": %d,
		"bridge": "%v",
		"ipam": {}
	}
	`
	r := &ncv1.NetworkAttachmentDefinition{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "k8s.cni.cncf.io/v1",
			Kind:       "NetworkAttachmentDefinition",
		},
		ObjectMeta: GetObjMeta(brName, labName, nsName),
		Spec: ncv1.NetworkAttachmentDefinitionSpec{
			Config: fmt.Sprintf(specTempalte, brName, mtu, fmt.Sprintf("vsrosfb%d", brIndex)),
		},
	}
	r.Labels[BridgeIndexLabelKey] = strconv.Itoa(brIndex)
	return r
}

func NewPortMACVTAPNAD(nsName, labName, nadname, resname string, mtu uint16, mac *net.HardwareAddr) *ncv1.NetworkAttachmentDefinition {
	const specTempalte = `
	{
		"cniVersion": "0.3.1",
		"name": "%v",
		"type": "macvtap",
		"mode": "passthru", 
		"removeparents": true,
		"promiscMode": true,
		"mtu": %d
	}`

	const specWithMACTempalte = `
	{
		"cniVersion": "0.3.1",
		"plugins": [
		  {
			"cniVersion": "0.3.1",
			"name": "%v",
			"type": "macvtap",
			"mode": "passthru", 
			"removeparents": true,
			"promiscMode": true,
			"mtu": %d
		  },
		  {
			"type": "tuning",
			"mac": "%v"
		  }
		]
	  }
	
	
	`

	r := &ncv1.NetworkAttachmentDefinition{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "k8s.cni.cncf.io/v1",
			Kind:       "NetworkAttachmentDefinition",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      nadname,
			Namespace: nsName,
			Labels: map[string]string{
				K8SLABELAPPKey:   K8SLABELAPPVAL,
				K8SLABELSETUPKEY: labName,
			},
			Annotations: map[string]string{
				macVTAPResourceKey: macVTAPResourceValPrefix + "/" + resname,
			},
		},
		Spec: ncv1.NetworkAttachmentDefinitionSpec{
			Config: fmt.Sprintf(specTempalte, nadname, mtu),
		},
	}
	if mac != nil {
		r.Spec.Config = fmt.Sprintf(specWithMACTempalte, nadname, mtu, mac.String())
	}
	return r
}

// GetFTPSROSImgPath returns sros image ftp path for a given vsim, without knlroot prefix
func GetFTPSROSImgPath(labName, chassisName string) string {
	// return filepath.Join(gconf.SROSImgRoot, fmt.Sprintf("vsim%d-os", vsimid))
	return filepath.Join("/"+IMGSubFolder, fmt.Sprintf("%v-%v-os", labName, chassisName))

}

// GetSFTPSROSImgPath return sros image SFTP path for a given vsim, with knlroot prefix
func GetSFTPSROSImgPath(labName, chassisName string) string {
	return filepath.Join("/"+KNLROOTName, GetFTPSROSImgPath(labName, chassisName))

}

func ReCreateSymLink(labName, chassisName, newtarget string) error {
	// gconf := conf.GCONF
	newTargetPath := filepath.Join("/"+KNLROOTName, IMGSubFolder, newtarget)
	os.MkdirAll(newTargetPath, 0750)
	os.Remove(filepath.Join("/"+KNLROOTName, GetFTPSROSImgPath(labName, chassisName)))
	return os.Symlink(newTargetPath,
		filepath.Join("/"+KNLROOTName, GetFTPSROSImgPath(labName, chassisName)))
}

func WaitForObjGone(ctx context.Context, clnt client.Client, ns string, obj client.Object) {
	wg := new(wait.Group)
	wctx, cancelf := context.WithDeadline(ctx, time.Now().Add(60*time.Second))
	defer cancelf()
	wg.StartWithContext(wctx, func(c context.Context) {
		wait.PollUntilContextCancel(c, time.Second, false, func(cc context.Context) (bool, error) {
			err := clnt.Get(cc, types.NamespacedName{Namespace: ns, Name: obj.GetName()},
				obj,
			)
			if err != nil {
				if apierrors.IsNotFound(err) {
					return true, nil
				} else {
					return false, err
				}
			}
			return false, nil
		})

	})
}

func IsCPM(slot string) bool {
	nslot := strings.ToLower(slot)
	return nslot == "a" || nslot == "b"
}

func GetVSROSFBName(lab, chassis string) string {
	return strings.ToLower(fmt.Sprintf("%v-%v-fb", lab, chassis))
}

func GetMAGCDFName(lab, chassis string) string {
	return fmt.Sprintf("magc-%v-%v-fb", lab, chassis)
}

func GetMACVTAPResName(lab, node, link string) string {
	return GenerateHashedInterfaceName(GetNodeLinkCombName(lab, node, link), "a")
}
func GenerateHashedInterfaceName(linkName, suffix string) string {
	hashNum := siphash.Hash(0x32487, 0xaed2345, []byte(linkName))
	base62str := big.NewInt(int64(hashNum)).Text(62)
	if strings.HasPrefix(base62str, "-") {
		base62str = "N" + base62str[1:]
	}
	rstr := fmt.Sprintf("%s%s", base62str, suffix)
	if len(rstr) > 14 {
		panic(fmt.Sprintf("%v is too long", rstr))
	}
	return rstr
}
func GetNodeLinkCombName(lab, node, link string) string {
	return fmt.Sprintf("%v-%v-%v", lab, node, link)
}
func GetMACVTAPBrName(lab, link string) string {
	return GenerateHashedInterfaceName(lab+link, "")
}
func GetMACVTAPVethBrName(lab, node, link string) string {
	return GenerateHashedInterfaceName(
		GetNodeLinkCombName(lab, node, link), "b")
}
func GetMACVTAPVXLANIfName(lab, link string) string {
	return GenerateHashedInterfaceName(lab+link, "v")
}
func GetLinkMACVTAPNADName(lab, node, link string) string {
	return GetNodeLinkCombName(lab, node, link)
}

// GetFTPSROSCfgPath return ftp path for config folder for a SR node in a lab
func GetSRConfigFTPSubFolder(labname string, chassisName string) string {
	return filepath.Join("/"+KNLROOTName, CfgSubFolder, labname, chassisName)

}
func NewDV(namespace, labName, dvName, nodeImg string, stroageclass *string, disksize *resource.Quantity) *cdiv1.DataVolume {
	// func newDV(lab *ParsedLabSpec, nodeName, nodeImg string) *cdiv1.DataVolume {
	r := new(cdiv1.DataVolume)

	r.ObjectMeta = metav1.ObjectMeta{
		Name:      dvName,
		Namespace: namespace,
		Labels: map[string]string{
			K8SLABELAPPKey:   K8SLABELAPPVAL,
			K8SLABELSETUPKEY: labName,
		},
	}
	r.Spec.PVC = &corev1.PersistentVolumeClaimSpec{
		StorageClassName: stroageclass,
		AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOncePod},
	}
	if disksize != nil {
		r.Spec.PVC.Resources.Requests = make(corev1.ResourceList)
		r.Spec.PVC.Resources.Requests[corev1.ResourceStorage] = *disksize
	}
	r.Spec.Source = &cdiv1.DataVolumeSource{
		Registry: &cdiv1.DataVolumeSourceRegistry{
			URL: &nodeImg,
		},
	}
	if strings.HasPrefix(strings.ToLower(nodeImg), "http") {
		r.Spec.Source = &cdiv1.DataVolumeSource{
			HTTP: &cdiv1.DataVolumeSourceHTTP{
				URL: nodeImg,
			},
		}
	}
	return r
}

func GetSRVMDVName(lab, node, slot string) string {
	return strings.ToLower(fmt.Sprintf("%v-%v-%v", lab, node, slot))
}
func GetVMPCDVName(lab, node string) string {
	return strings.ToLower(fmt.Sprintf("%v-%v", lab, node))
}

const (
	FixedSRVMMgmtAddrPrefixStr = "10.0.2.2/24"
	FixedFTPProxyUser          = "ftp"
	FixedFTPProxyPass          = "ftp"
	FixedFTPProxySvr           = "10.0.2.1"
)

// chassis is sysinfo string specified in API that only contains chassis,card, mda, sfm
func GenSysinfo(baseSysinfo string, cfgURL, licURL string) string {
	return fmt.Sprintf("TIMOS: %v address=%v@active primary-config=%v license-file=%v static-route=0.0.0.0/0@%v",
		baseSysinfo, FixedSRVMMgmtAddrPrefixStr, cfgURL, licURL, FixedFTPProxySvr)
}

func IsIntegratedChassis(chassisModel string) bool {
	switch strings.ToLower(chassisModel) {
	case "sr-1", "sr-1s", "vsr-i", "sr-1se":
		return true
	default:
		switch {
		case strings.HasPrefix(strings.ToLower(chassisModel), "sr-1x-"):
			return true
		case strings.HasPrefix(strings.ToLower(chassisModel), "sr-1-"):
			return true
		}
	}
	return false
}

func IsIntegratedChassisViaSysinfo(sysinfo string) bool {
	return IsIntegratedChassis(GetChassisFromSysinfoStr(sysinfo))
}
func GetChassisFromSysinfoStr(sysinfostr string) string {
	flist := strings.Fields(strings.TrimSpace(sysinfostr))
	for _, f := range flist {
		f = strings.TrimSpace(f)
		if rs, ok := strings.CutPrefix(f, "chassis="); ok {
			return rs
		}
	}
	return ""
}

// Regex for a basic FQDN check based on RFCs 952 and 1123,
// allowing letters, numbers, and hyphens in labels, but not starting/ending with a hyphen.
// It also ensures a top-level domain (TLD) exists. This is a common, though not exhaustive, validation.
const fqdnRegex = `^(([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9\-]*[a-zA-Z0-9])\.)*([A-Za-z0-9]|[A-Za-z0-9][A-Za-z0-9\-]*[A-Za-z0-9])$`

var fqdnPattern = regexp.MustCompile(fqdnRegex)

// IsValidFQDN performs a basic FQDN validation using a regular expression.
func IsValidFQDN(s string) bool {
	// FQDN length constraint: max 253 characters (excluding a trailing dot, if present)
	if len(s) > 253 {
		return false
	}

	// A domain name label (part between dots) cannot exceed 63 characters.
	// The regex implicitly handles other RFC rules like allowed characters.

	// FQDN regex match
	return fqdnPattern.MatchString(s)
}

// IsIPOrFQDN checks if the input string is a valid IP address or a valid FQDN.
func IsIPOrFQDN(host string) bool {
	// 1. Check for IP Address (IPv4 or IPv6)
	if net.ParseIP(host) != nil {
		return true
	}

	// 2. Check for FQDN (if not an IP)
	return IsValidFQDN(host)

}

// IsHostPort check if inputs is addr:port or fqdn:port
func IsHostPort(inputs string) bool {
	host, _, err := net.SplitHostPort(inputs)
	if err != nil {
		return false
	}
	return IsIPOrFQDN(host)
}

func GetPodName(lab, node string) string {
	return lab + "-" + node
}
func NewBasePod(labName, nodeName, nameSpace, image string) *corev1.Pod {
	// gconf := conf.GCONF
	r := new(corev1.Pod)
	r.ObjectMeta = GetObjMeta(
		GetPodName(labName, nodeName),
		labName,
		nameSpace,
	)
	r.ObjectMeta.Labels[K8SLABELNodeKEY] = nodeName
	r.Spec.Containers = []corev1.Container{
		{
			Name:  "main",
			Image: image,
		},
	}
	r.ObjectMeta.Annotations = make(map[string]string)
	return r
}

func GetPointerVal[T any](v T) *T {
	r := new(T)
	*r = v
	return r

}

func GetSortedKeySlice[K cmp.Ordered, V any](m map[K]V) []K {
	keys := make([]K, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
