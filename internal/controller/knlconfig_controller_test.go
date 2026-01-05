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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	knlv1beta1 "kubenetlab.net/knl/api/v1beta1"
	"kubenetlab.net/knl/common"
)

var _ = Describe("KNLConfig Controller", func() {

	Context("When reconciling a resource", func() {
		const resourceName = "test-knlconfig"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: testControllerNS, // TODO(user):Modify as needed
		}
		knlconfig := &knlv1beta1.KNLConfig{}

		BeforeEach(func() {

			By("creating the custom resource for the Kind KNLConfig")
			err := k8sClient.Get(ctx, typeNamespacedName, knlconfig)
			if err != nil && errors.IsNotFound(err) {
				resource := &knlv1beta1.KNLConfig{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: testControllerNS,
					},
					// TODO(user): Specify other spec details if needed.
					Spec: knlv1beta1.KNLConfigSpec{
						PVCStorageClass:  common.ReturnPointerVal("standard"),
						SRIOMLoaderImage: common.ReturnPointerVal("localhost/iomload:v1"),
						SideCarHookImg:   common.ReturnPointerVal("localhost/knl2sidecar:v15"),
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
				// err = k8sClient.Get(ctx, typeNamespacedName, knlconfig)
				// Expect(err).NotTo(HaveOccurred())
				// buf, _ := yaml.Marshal(*knlconfig)
				// fmt.Println(string(buf))
			}
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &knlv1beta1.KNLConfig{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance KNLConfig")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &KNLConfigReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			// TODO(user): Add more specific assertions depending on your controller's reconciliation logic.
			// Example: If you expect a certain status condition after reconciliation, verify it here.
		})
	})
})
