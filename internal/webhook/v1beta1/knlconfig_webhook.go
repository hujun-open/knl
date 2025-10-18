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

package v1beta1

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"kubenetlab.net/knl/api/v1beta1"
	knlv1beta1 "kubenetlab.net/knl/api/v1beta1"
	"kubenetlab.net/knl/internal/common"
)

// nolint:unused
// log is for logging in this package.
var knlconfiglog = logf.Log.WithName("knlconfig-resource")

// SetupKNLConfigWebhookWithManager registers the webhook for KNLConfig in the manager.
func SetupKNLConfigWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&knlv1beta1.KNLConfig{}).
		WithValidator(&KNLConfigCustomValidator{}).
		WithDefaulter(&KNLConfigCustomDefaulter{}).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// +kubebuilder:webhook:path=/mutate-knl-kubenetlab-net-v1beta1-knlconfig,mutating=true,failurePolicy=fail,sideEffects=None,groups=knl.kubenetlab.net,resources=knlconfigs,verbs=create;update,versions=v1beta1,name=mknlconfig-v1beta1.kb.io,admissionReviewVersions=v1

// KNLConfigCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind KNLConfig when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type KNLConfigCustomDefaulter struct {
	// TODO(user): Add more fields as needed for defaulting
}

var _ webhook.CustomDefaulter = &KNLConfigCustomDefaulter{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind KNLConfig.
func (d *KNLConfigCustomDefaulter) Default(_ context.Context, obj runtime.Object) error {
	knlconfig, ok := obj.(*knlv1beta1.KNLConfig)

	if !ok {
		return fmt.Errorf("expected an KNLConfig object but got %T", obj)
	}
	knlconfiglog.Info("Defaulting for KNLConfig", "name", knlconfig.GetName())
	spec := knlconfig.Spec
	err := common.FillNilPointers(&spec, v1beta1.DefKNLConfig())
	if err != nil {
		return err
	}
	knlconfig.Spec = spec
	return nil
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-knl-kubenetlab-net-v1beta1-knlconfig,mutating=false,failurePolicy=fail,sideEffects=None,groups=knl.kubenetlab.net,resources=knlconfigs,verbs=create;update,versions=v1beta1,name=vknlconfig-v1beta1.kb.io,admissionReviewVersions=v1

// KNLConfigCustomValidator struct is responsible for validating the KNLConfig resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type KNLConfigCustomValidator struct {
	// TODO(user): Add more fields as needed for validation
}

var _ webhook.CustomValidator = &KNLConfigCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type KNLConfig.
func (v *KNLConfigCustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	knlconfig, ok := obj.(*knlv1beta1.KNLConfig)
	if !ok {
		return nil, fmt.Errorf("expected a KNLConfig object but got %T", obj)
	}
	knlconfiglog.Info("Validation for KNLConfig upon creation", "name", knlconfig.GetName())

	// TODO(user): fill in your validation logic upon object creation.

	return nil, knlconfig.Validate()
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type KNLConfig.
func (v *KNLConfigCustomValidator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	knlconfig, ok := newObj.(*knlv1beta1.KNLConfig)
	if !ok {
		return nil, fmt.Errorf("expected a KNLConfig object for the newObj but got %T", newObj)
	}
	knlconfiglog.Info("Validation for KNLConfig upon update", "name", knlconfig.GetName())

	// TODO(user): fill in your validation logic upon object update.

	return nil, knlconfig.Validate()
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type KNLConfig.
func (v *KNLConfigCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	knlconfig, ok := obj.(*knlv1beta1.KNLConfig)
	if !ok {
		return nil, fmt.Errorf("expected a KNLConfig object but got %T", obj)
	}
	knlconfiglog.Info("Validation for KNLConfig upon deletion", "name", knlconfig.GetName())

	// TODO(user): fill in your validation logic upon object deletion.

	return nil, nil
}
