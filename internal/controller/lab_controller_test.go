/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/goccy/go-yaml"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	knlv1beta1 "kubenetlab.net/knl/api/v1beta1"
	"kubenetlab.net/knl/common"
	kvv1 "kubevirt.io/api/core/v1"
)

func getTestKNLConfig() (types.NamespacedName, *knlv1beta1.KNLConfig) {
	knlcfgName := "knlcfg"
	return types.NamespacedName{Namespace: testControllerNS, Name: knlcfgName},
		&knlv1beta1.KNLConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      knlcfgName,
				Namespace: testControllerNS,
			},
			// TODO(user): Specify other spec details if needed.
			Spec: knlv1beta1.KNLConfigSpec{
				PVCStorageClass:  common.ReturnPointerVal("standard"),
				SRIOMLoaderImage: common.ReturnPointerVal("localhost/iomload:v1"),
				SideCarHookImg:   common.ReturnPointerVal("localhost/knl2sidecar:v15"),
				DefaultNode: &knlv1beta1.OneOfSystem{
					Pod: &knlv1beta1.GeneralPod{
						Image: common.ReturnPointerVal("podimage"),
					},
				},
			},
		}
}

var _ = Describe("Lab Controller", func() {
	Context("When reconciling a resource", func() {
		const labName = "test-lab"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      labName,
			Namespace: testControllerNS, // TODO(user):Modify as needed
		}
		lab := &knlv1beta1.Lab{}

		BeforeEach(func() {
			By("installing knlconfig")
			cfgKey, cfg := getTestKNLConfig()
			err := k8sClient.Get(ctx, cfgKey, new(knlv1beta1.KNLConfig))
			if err != nil && errors.IsNotFound(err) {
				Expect(k8sClient.Create(ctx, cfg)).To(Succeed())
			}
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &knlv1beta1.Lab{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance Lab")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})

		It("should successfully reconcile a custom resource for Lab", func() {
			By("creating the custom resource for the Kind Lab")
			err := k8sClient.Get(ctx, typeNamespacedName, lab)
			if err != nil && errors.IsNotFound(err) {
				resource := &knlv1beta1.Lab{
					ObjectMeta: metav1.ObjectMeta{
						Name:      labName,
						Namespace: testControllerNS,
					},
					Spec: knlv1beta1.LabSpec{
						LinkList: map[string]*knlv1beta1.Link{
							"link-1": {
								Connectors: []knlv1beta1.Connector{
									{
										NodeName: common.ReturnPointerVal("vsim-1"),
									},
									{
										NodeName: common.ReturnPointerVal("pod-2"),
									},
								},
							},
							"link-3": {
								Connectors: []knlv1beta1.Connector{
									{
										NodeName: common.ReturnPointerVal("vsim-1"),
									},
									{
										NodeName: common.ReturnPointerVal("pod-3"),
									},
								},
							},
							"link-5": {
								Connectors: []knlv1beta1.Connector{
									{
										NodeName: common.ReturnPointerVal("vsim-1"),
									},
									{
										NodeName: common.ReturnPointerVal("pod-3"),
									},
								},
							},
						},
					},
					// TODO(user): Specify other spec details if needed.
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}

			By("checking if the vsim's port order")
			time.Sleep(10 * time.Second)
			vmiKey := types.NamespacedName{Namespace: testControllerNS,
				Name: knlv1beta1.GetSRVMCardVMName(labName, "vsim-1", "1")}
			vmi := new(kvv1.VirtualMachineInstance)
			Expect(k8sClient.Get(ctx, vmiKey, vmi)).To(Succeed())
			buf, _ := yaml.Marshal(vmi)
			fmt.Println(string(buf))

		})
	})
})
