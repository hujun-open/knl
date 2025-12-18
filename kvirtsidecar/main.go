/*
this is a kubenetlab hook sidecar to add sysinfo to vSROS xml
it put the vSROSSysinfoAnnotation in VM spec, into libvirt sysinfo field
*/

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/pflag"
	"kubenetlab.net/knl/api/v1beta1"
	"kubenetlab.net/knl/common"
	knlv1beta1 "kubenetlab.net/knl/dict"
	vmSchema "kubevirt.io/api/core/v1"
	"libvirt.org/go/libvirtxml"
)

type vmType string

const (
	vmTypeSR  vmType = "sr"
	vmTypeCSR vmType = "csr"
)

func getVMTypeviaNodeType(t string) (vmType, error) {
	vmt := common.NodeType(strings.ToLower(strings.TrimSpace(t)))
	switch vmt {
	case v1beta1.SRVMVSIM, v1beta1.SRVMMAGC, v1beta1.SRVMVSRI:
		return vmTypeSR, nil
		//TODO for csr
	}
	return "", fmt.Errorf("unknown vm type %v", t)
}

var ignorePortAliasPrefixList = []string{"vxlandev", "macvtapbr"}

func onDefineDomain(vmiJSON, domainXML []byte) (string, error) {
	// f, err := os.CreateTemp("", "origdomainxml*")
	// if err == nil {
	// 	f.Write(domainXML)
	// 	f.Close()
	// }

	vmiSpec := vmSchema.VirtualMachineInstance{}
	if err := json.Unmarshal(vmiJSON, &vmiSpec); err != nil {
		return "", err
	}
	newSpec := &libvirtxml.Domain{}
	err := newSpec.Unmarshal(string(domainXML))
	if err != nil {
		return "", fmt.Errorf("failed to unmarsahl using libvirtxml, %w", err)
	}

	annotations := vmiSpec.GetAnnotations()
	var found bool
	var vmts string
	if vmts, found = annotations[knlv1beta1.ChassisTypeAnnotation]; !found {
		return "", fmt.Errorf("failed to find %v annotation", knlv1beta1.ChassisTypeAnnotation)
	}
	vmt, err := getVMTypeviaNodeType(vmts)
	if err != nil {
		return "", fmt.Errorf("failed to get node type from %v, %w", vmts, err)
	}

	if err := json.Unmarshal(vmiJSON, &vmiSpec); err != nil {
		return "", err
	}
	var chassisName, sftpSvrAddr, sftpUser, sftpPass string

	var labName string
	switch vmt {
	case vmTypeSR:

		if _, found = annotations[knlv1beta1.VSROSSysinfoAnno]; !found {
			//if not vsros, return unchanged
			return string(domainXML), nil
		}
		if labName, found = annotations[knlv1beta1.LabNameAnnotation]; !found {
			//doesn't found
			return "", fmt.Errorf("can't find lab name, %v", knlv1beta1.LabNameAnnotation)
		}
		if sftpSvrAddr, found = annotations[knlv1beta1.SftpSVRAnnontation]; !found {
			return "", fmt.Errorf("can't find %v in VMI's annontation", knlv1beta1.SftpSVRAnnontation)
		}
		if chassisName, found = annotations[knlv1beta1.ChassisNameAnnotation]; !found {
			return "", fmt.Errorf("can't find %v in VMI's annontation", knlv1beta1.ChassisNameAnnotation)
		}
		if sftpUser, found = annotations[knlv1beta1.SftpUserAnnontation]; !found {
			return "", fmt.Errorf("can't find %v in VMI's annontation", knlv1beta1.SftpUserAnnontation)
		}
		if sftpPass, found = annotations[knlv1beta1.SftpPassAnnontation]; !found {
			return "", fmt.Errorf("can't find %v in VMI's annontation", knlv1beta1.SftpPassAnnontation)
		}
		//sftpSvrAddr must be addr or hostname + port
		if sftpSvrAddr, found = annotations[knlv1beta1.SftpSVRAnnontation]; !found {
			return "", fmt.Errorf("can't find %v in VMI's annontation", knlv1beta1.SftpSVRAnnontation)
		}

		//generate ftp server config

		cpmbootrom := fmt.Sprintf("%v/i386-boot.tim", common.GetSFTPSROSImgPath(labName, chassisName))
		iombootrom := fmt.Sprintf("%v/i386-iom.tim", common.GetSFTPSROSImgPath(labName, chassisName))
		bofpath := common.GetSFTPSROSImgPath(labName, chassisName)
		licStr := fmt.Sprintf("%v/vsim.lic", common.KNLROOTName)
		if vmts != string(v1beta1.SRVMVSIM) {
			licStr = fmt.Sprintf("%v/vsr.lic", common.KNLROOTName)
		}
		cfgPath := common.GetSRConfigFTPSubFolder(labName, chassisName)
		cfg := fmt.Sprintf(ftpSVRCFGTemplate,
			cpmbootrom, iombootrom, bofpath,
			licStr, cfgPath, sftpUser, sftpPass, sftpSvrAddr)
		err = os.WriteFile(ftpSvrCfgFilePath, []byte(cfg), 0644)
		if err != nil {
			log.Fatal(err)
		}

		//set CPU model
		newSpec.CPU = &libvirtxml.DomainCPU{
			Mode:   "custom",
			Vendor: "Intel",
			Model: &libvirtxml.DomainCPUModel{
				Value: "SandyBridge",
			},
		}

		//change disk to hda
		newSpec.Devices.Disks[0].Target.Dev = "hda"

		//add sysinfo
		sysinfo := annotations[knlv1beta1.VSROSSysinfoAnno]
		//NOTE: can't remove or change UUID in orignal smbios uuid entry, otherwise, kubevirt won't be able to report VMI status as running
		for i := range newSpec.SysInfo[0].SMBIOS.System.Entry {
			if newSpec.SysInfo[0].SMBIOS.System.Entry[i].Name == "product" {
				newSpec.SysInfo[0].SMBIOS.System.Entry[i].Value = sysinfo
			}
		}
		//change machine
		newSpec.OS.Type.Machine = "pc"
		//clear up controller
		newSpec.Devices.Controllers = []libvirtxml.DomainController{}

		//change interface to bridge
		rlist := []libvirtxml.DomainInterface{}
	L1:
		for i := range newSpec.Devices.Interfaces {
			// if i == 0 {
			// 	//skip the 1st pod network interface
			// 	continue
			// }
			origAlias := newSpec.Devices.Interfaces[i].Alias
			//skip empty alias interface, which could be place holder
			if origAlias == nil {
				continue
			} else {
				if origAlias.Name == "" {
					continue
				}
			}
			for _, prefix := range ignorePortAliasPrefixList {
				if strings.HasPrefix(origAlias.Name, "ua-"+prefix) {
					continue L1
				}
			}

			// if strings.HasPrefix(newSpec.Devices.Interfaces[i].Target.Dev, "tap") {

			newIf := newSpec.Devices.Interfaces[i]
			newIf.Model.Type = "virtio" //virtio is required to make vsros work
			rlist = append(rlist, newIf)
		}
		newSpec.Devices.Interfaces = rlist
		//remove balloon
		newSpec.Devices.MemBalloon = nil
		//remove acpi
		newSpec.Features = nil
		//remove video; update: no need to remove video
		// newSpec.Devices.Videos = []libvirtxml.DomainVideo{}

		//remove seclabel
		newSpec.SecLabel = []libvirtxml.DomainSecLabel{{Type: "none"}}
		//clear channel
		// newSpec.Devices.Channels = []libvirtxml.DomainChannel{}
		//add telnet console
		newSpec.Devices.Consoles = []libvirtxml.DomainConsole{
			{
				Alias: &libvirtxml.DomainAlias{
					Name: "console0",
				},
				Protocol: &libvirtxml.DomainChardevProtocol{
					Type: "telnet",
				},
				Source: &libvirtxml.DomainChardevSource{
					TCP: &libvirtxml.DomainChardevSourceTCP{
						Mode:    "bind",
						Host:    "0.0.0.0",
						Service: "2222",
						TLS:     "no",
					},
				},
				Target: &libvirtxml.DomainConsoleTarget{
					Type: "virtio",
					Port: new(uint),
				},
			},
		}

		//clean up disk setting
		diskFile := newSpec.Devices.Disks[0].Source.File.File

		for i := range newSpec.Devices.Disks {
			newSpec.Devices.Disks[i].Source.File = &libvirtxml.DomainDiskSourceFile{
				// File: "/var/run/kubevirt-private/vmi-disks/knlroot/disk.img",
				File: diskFile,
				// File: filepath.Join(pvcFolderPrefix, vmname[8:]+".qcow2"),
				// File: common.GetKVirtPODDiskImgPath(vmname),
			} //this line is for the PVC
			newSpec.Devices.Disks[i].Address = nil
			newSpec.Devices.Disks[i].Model = "virtio"
			newSpec.Devices.Disks[i].Driver.Type = "raw"
			if vmiSpec.Spec.Volumes[0].ContainerDisk != nil {
				newSpec.Devices.Disks[i].Driver.Type = "qcow2"
			}
			newSpec.Devices.Disks[i].Driver.ErrorPolicy = ""
			newSpec.Devices.Disks[i].Driver.Discard = ""
			break //just do the 1st one
		}

	case vmTypeCSR:
		newSpec.Devices.Graphics = []libvirtxml.DomainGraphic{}
		newSpec.Devices.Videos = []libvirtxml.DomainVideo{}

	}
	newrr, err := newSpec.Marshal()
	if err != nil {
		return "", fmt.Errorf("failed to marsahl using libvirtxml, %w", err)
	}

	return newrr, nil

}

const (
	ftpSVRCFGTemplate = `{
"version": 1,
"listen_address": ":21",
"path_map": {"/i386-boot.tim": "%v","/i386-iom.tim": "%v", "/sros": "%v","/lic": "%v","/cfg": "%v"},
"accesses": [
{
	"user": "ftp",
	"pass": "ftp",
	"fs": "sftp",
	"params": {
	"username": "%v",
	"password": "%v",
	"hostname": "%v"
	}
}
],
"passive_transfer_port_range": {
"start": 2122,
"end": 2130
}
}`
	ftpSvrCfgFilePath = "/tmp/ftpsvrcfg.json"
)

func main() {
	var vmiJSON, domainXML string
	pflag.StringVar(&vmiJSON, "vmi", "", "VMI to change in JSON format")
	pflag.StringVar(&domainXML, "domain", "", "Domain spec in XML format")
	pflag.Parse()
	logf, err := os.CreateTemp("", "knlhook*")
	if err == nil {
		defer logf.Close()
		log.SetOutput(logf)
	}

	// logger := log.New(os.Stderr, "knlhook", log.Ldate)
	if vmiJSON == "" || domainXML == "" {
		log.Printf("Bad input vmi=%d, domain=%d", len(vmiJSON), len(domainXML))
		os.Exit(1)
	}
	log.Print("orig xml", domainXML)
	rdomainXML, err := onDefineDomain([]byte(vmiJSON), []byte(domainXML))
	if err != nil {
		log.Print(err)
		os.Exit(1)
	}
	log.Print("result xml", rdomainXML)
	//start daemon
	cmd := exec.Command("/bin/sh", "-c", fmt.Sprintf("/usr/bin/ftpserver -conf %v &", ftpSvrCfgFilePath))
	log.Printf("Running command %v and waiting for it to finish...", cmd.String())
	err = cmd.Run()
	if err != nil {
		log.Printf("command failed with %v", err)
	} else {
		log.Printf("damemon launched")
	}

	//do NOT remove line below this is needed to return result xml
	fmt.Print(rdomainXML)
}
