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

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"kubenetlab.net/knl/api/v1beta1"
	knlv1beta1 "kubenetlab.net/knl/api/v1beta1"
)

// KNLConfigReconciler reconciles a KNLConfig object
type KNLConfigReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=knl.kubenetlab.net,resources=knlconfigs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=knl.kubenetlab.net,resources=knlconfigs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=knl.kubenetlab.net,resources=knlconfigs/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the KNLConfig object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.22.1/pkg/reconcile

const TARGET_KNLCONFIG_NAME = "knlcfg"

func (r *KNLConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	var knlcfg v1beta1.KNLConfig
	if req.NamespacedName.Name != TARGET_KNLCONFIG_NAME || req.NamespacedName.Namespace != knlv1beta1.MYNAMESPACE {
		log.Info(fmt.Sprintf("%v is not target KNLConfig, ignored", req.NamespacedName.String()))
		return ctrl.Result{}, nil
	}
	if err := r.Get(ctx, req.NamespacedName, &knlcfg); err != nil {
		log.Error(err, "unable to fetch KNLConfig")
		// we'll ignore not-found errors, since they can't be fixed by an immediate
		// requeue (we'll need to wait for a new notification), and we can get them
		// on deleted requests.
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if knlcfg.Status.ObservedGeneration == nil {
		knlcfg.Status.ObservedGeneration = new(int64)
		*knlcfg.Status.ObservedGeneration = -1
	}

	newSpec := knlcfg.Spec
	changed := knlv1beta1.GCONF.Set(&newSpec, knlcfg.Generation)
	if changed {
		*knlcfg.Status.ObservedGeneration = knlv1beta1.GCONF.GetGen()
		r.Status().Update(ctx, &knlcfg)
		log.Info(fmt.Sprintf("%v is updated to Generation %d, %+v", req.NamespacedName, knlcfg.Generation, knlv1beta1.GCONF.Get()))
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *KNLConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&knlv1beta1.KNLConfig{}).
		Named("knlconfig").
		Complete(r)
}
