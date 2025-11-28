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

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"kubenetlab.net/knl/api/v1beta1"
	knlv1beta1 "kubenetlab.net/knl/api/v1beta1"
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
	ensureCTX := context.WithValue(ctx, v1beta1.ParsedLabKey, plab)

	// name of our custom finalizer
	myFinalizerName := "lab.kubenetlab.net/finalizer"

	// examine DeletionTimestamp to determine if object is under deletion
	if lab.ObjectMeta.DeletionTimestamp.IsZero() {
		// The object is not being deleted, so if it does not have our finalizer,
		// then let's add the finalizer and update the object. This is equivalent
		// to registering our finalizer.
		if !controllerutil.ContainsFinalizer(lab, myFinalizerName) {
			controllerutil.AddFinalizer(lab, myFinalizerName)
			if err := r.Update(ctx, lab); err != nil {
				return ctrl.Result{}, err
			}
		}
	} else {
		// The object is being deleted
		if controllerutil.ContainsFinalizer(lab, myFinalizerName) {
			// our finalizer is present, so let's handle any external dependency
			if err := r.deleteExternalResources(ctx, lab); err != nil {
				// if fail to delete the external dependency here, return with error
				// so that it can be retried.
				return ctrl.Result{}, err
			}

			// remove our finalizer from the list and update it.
			controllerutil.RemoveFinalizer(lab, myFinalizerName)
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
	plab.SpokeMap, err = plab.EnsureLinks(ensureCTX, r.Client)
	if err != nil {
		logger.Error(err, "failed to create links")
		return ctrl.Result{}, nil
	}
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

// SetupWithManager sets up the controller with the Manager.
func (r *LabReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&knlv1beta1.Lab{}).
		Named("lab").
		Complete(r)
}

func (r *LabReconciler) deleteExternalResources(ctx context.Context, lab *v1beta1.Lab) error {
	//delete MACvtap configmap entries of lab
	logger := log.FromContext(ctx)
	logger.Info("deleting external resource", "lab", lab.Name)
	return nil
}
