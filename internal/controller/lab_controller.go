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

	k8slan "github.com/hujun-open/k8slan/api/v1beta1"
	ncv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"kubenetlab.net/knl/api/v1beta1"
	knlv1beta1 "kubenetlab.net/knl/api/v1beta1"
	"kubenetlab.net/knl/common"
	kvv1 "kubevirt.io/api/core/v1"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// LabReconciler reconciles a Lab object
type LabReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=knl.kubenetlab.net,resources=labs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=knl.kubenetlab.net,resources=labs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=knl.kubenetlab.net,resources=labs/finalizers,verbs=update

//+kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;create;update;patch;delete;deletecollection
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch,namespace=knl-system
//+kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete;deletecollection
//+kubebuilder:rbac:groups=k8s.cni.cncf.io,resources=network-attachment-definitions,verbs=get;list;watch;create;update;patch;delete;deletecollection
//+kubebuilder:rbac:groups=kubevirt.io,resources=virtualmachineinstances,verbs=get;list;watch;create;update;patch;delete;deletecollection
//+kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete;deletecollection
//+kubebuilder:rbac:groups=cdi.kubevirt.io,resources=datavolumes,verbs=get;list;watch;create;update;patch;delete;deletecollection
// +kubebuilder:rbac:groups=lan.k8slan.io,resources=lans,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Lab object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.22.1/pkg/reconcile
func (r *LabReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	logger := log.FromContext(ctx)
	logger.Info("reconcile started", "request", req)
	lab := new(v1beta1.Lab)
	if err := r.Get(ctx, req.NamespacedName, lab); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	plab := knlv1beta1.ParseLab(lab, r.Scheme)

	// name of our custom finalizer

	// examine DeletionTimestamp to determine if object is under deletion
	if lab.ObjectMeta.DeletionTimestamp.IsZero() {
		// The object is not being deleted, so if it does not have our finalizer,
		// then let's add the finalizer and update the object. This is equivalent
		// to registering our finalizer.
		if !controllerutil.ContainsFinalizer(lab, knlv1beta1.FinalizerName) {
			controllerutil.AddFinalizer(lab, knlv1beta1.FinalizerName)
			if err := r.Update(ctx, lab); err != nil {
				return ctrl.Result{}, err
			}
		}
	} else {
		// The object is being deleted
		if controllerutil.ContainsFinalizer(lab, knlv1beta1.FinalizerName) {
			// our finalizer is present, so let's handle any external dependency
			if err := r.deleteExternalResources(ctx, lab); err != nil {
				// if fail to delete the external dependency here, return with error
				// so that it can be retried.
				return ctrl.Result{}, err
			}

			// remove our finalizer from the list and update it.
			controllerutil.RemoveFinalizer(lab, knlv1beta1.FinalizerName)
			if err := r.Update(ctx, lab); err != nil {
				return ctrl.Result{}, err
			}
		}

		// Stop reconciliation as the item is being deleted
		return ctrl.Result{}, nil
	}
	//reconcile logic here
	//create k8sLAN CRs
	var err error
	err = plab.EnsureLinks(ctx, r.Client)
	if err != nil {
		logger.Error(err, "failed to create links")
		return ctrl.Result{}, nil
	}
	logger.Info("links ensured", "SpokeMap", fmt.Sprintf("%+v", plab.SpokeMap))
	ensureCTX := context.WithValue(ctx, v1beta1.ParsedLabKey, plab)
	//create nodes
	for nodeName, node := range plab.Lab.Spec.NodeList {
		sys, _ := node.GetSystem()
		err = sys.Ensure(ensureCTX, nodeName, r.Client, false)
		if err != nil {
			logger.Error(err, "failed to ensure node", "node", nodeName)
			return ctrl.Result{}, nil
		}
	}
	return ctrl.Result{}, nil
}

type myObj[B any] interface {
	client.Object
	*B
}

var (
	OwnerKey = ".metadata.controller"
	apiGVStr = knlv1beta1.GroupVersion.String()
)

func extractKey[T client.Object](rawObj client.Object) []string {
	job := rawObj.(T)
	owner := metav1.GetControllerOf(job)
	if owner == nil {
		return nil
	}

	if owner.APIVersion != apiGVStr || owner.Kind != "Lab" {
		return nil
	}

	// ...and if so, return it
	return []string{owner.Name}
}

func registrResource[T any, PT myObj[T]](ctx context.Context, mgr ctrl.Manager) error {
	return mgr.GetFieldIndexer().IndexField(ctx,
		PT(new(T)),
		OwnerKey,
		extractKey[PT])
}

// SetupWithManager sets up the controller with the Manager.
func (r *LabReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := registrResource[corev1.Pod](context.Background(), mgr); err != nil {
		return common.MakeErr(err)
	}
	if err := registrResource[corev1.PersistentVolumeClaim](context.Background(), mgr); err != nil {
		return common.MakeErr(err)
	}
	if err := registrResource[kvv1.VirtualMachineInstance](context.Background(), mgr); err != nil {
		return common.MakeErr(err)
	}
	if err := registrResource[k8slan.LAN](context.Background(), mgr); err != nil {
		return common.MakeErr(err)
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&knlv1beta1.Lab{}).
		Owns(&kvv1.VirtualMachineInstance{}).
		Owns(&corev1.Pod{}).
		Owns(&k8slan.LAN{}).
		Owns(&cdiv1.DataVolume{}).
		Owns(&ncv1.NetworkAttachmentDefinition{}).
		Named("lab").
		Complete(r)
}

func (r *LabReconciler) deleteExternalResources(ctx context.Context, lab *v1beta1.Lab) error {
	logger := log.FromContext(ctx)
	logger.Info("deleting external resource", "lab", lab.Name)
	//removing finalizer on LAN CR
	for link := range lab.Spec.LinkList {
		lan := new(k8slan.LAN)
		lanName := knlv1beta1.Getk8lanName(lab.Name, link)
		err := r.Client.Get(ctx, types.NamespacedName{Namespace: lab.Namespace, Name: lanName}, lan)
		if err != nil {
			if !apierrors.IsNotFound(err) {
				return fmt.Errorf("failed to check finalizer on LAN %v, %w", lanName, err)
			}
			//already gone
		}
		//remove the finalizer
		controllerutil.RemoveFinalizer(lan, knlv1beta1.FinalizerName)
		if err := r.Update(ctx, lan); err != nil {
			return fmt.Errorf("failed to remove finalizer on LAN %v, %w", lanName, err)
		}
	}

	return nil
}
