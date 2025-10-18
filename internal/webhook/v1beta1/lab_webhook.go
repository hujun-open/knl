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

	knlv1beta1 "kubenetlab.net/knl/api/v1beta1"
	"kubenetlab.net/knl/internal/common"
	"kubenetlab.net/knl/internal/controller"
)

// nolint:unused
// log is for logging in this package.
var lablog = logf.Log.WithName("lab-resource")

// SetupLabWebhookWithManager registers the webhook for Lab in the manager.
func SetupLabWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&knlv1beta1.Lab{}).
		WithValidator(&LabCustomValidator{}).
		WithDefaulter(&LabCustomDefaulter{}).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// +kubebuilder:webhook:path=/mutate-knl-kubenetlab-net-v1beta1-lab,mutating=true,failurePolicy=fail,sideEffects=None,groups=knl.kubenetlab.net,resources=labs,verbs=create;update,versions=v1beta1,name=mlab-v1beta1.kb.io,admissionReviewVersions=v1

// LabCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind Lab when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type LabCustomDefaulter struct {
	// TODO(user): Add more fields as needed for defaulting
}

var _ webhook.CustomDefaulter = &LabCustomDefaulter{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind Lab.
func (d *LabCustomDefaulter) Default(_ context.Context, obj runtime.Object) error {
	lab, ok := obj.(*knlv1beta1.Lab)

	if !ok {
		return fmt.Errorf("expected an Lab object but got %T", obj)
	}
	lablog.Info("Defaulting for Lab", "lab", lab.GetName())

	//if any node that only contains name, create a new empty instance based on node name
	for i := range lab.Spec.NodeList {
		sys, _ := lab.Spec.NodeList[i].GetSystem()
		if sys == nil {
			lablog.Info(fmt.Sprintf("node %v doesn't specify node type", lab.Spec.NodeList[i].Name), "lab", lab.GetName())
			sys = common.GetNewSystemViaName(lab.Spec.NodeList[i].Name)
			if sys == nil {
				return fmt.Errorf("failed to derived node type from name %v", lab.Spec.NodeList[i].Name)
			}
			knlv1beta1.AssignSystem(sys, &(lab.Spec.NodeList[i].OneOfSystem))
		}
	}

	gconf := controller.GCONF.Get()
	lablog.Info(fmt.Sprintf("defaulting lab, got GCONF:%+v", gconf))
	err := knlv1beta1.LoadDef(&lab.Spec, gconf)
	if err != nil {
		return err
	}
	//Todo: add missing node
	//fill node specific Default
	for i := range lab.Spec.NodeList {
		sys, _ := lab.Spec.NodeList[i].GetSystem()
		sys.FillDefaultVal(lab.Spec.NodeList[i].Name)
	}

	return nil
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-knl-kubenetlab-net-v1beta1-lab,mutating=false,failurePolicy=fail,sideEffects=None,groups=knl.kubenetlab.net,resources=labs,verbs=create;update,versions=v1beta1,name=vlab-v1beta1.kb.io,admissionReviewVersions=v1

// LabCustomValidator struct is responsible for validating the Lab resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type LabCustomValidator struct {
	// TODO(user): Add more fields as needed for validation
}

var _ webhook.CustomValidator = &LabCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type Lab.
func (v *LabCustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	lab, ok := obj.(*knlv1beta1.Lab)
	if !ok {
		return nil, fmt.Errorf("expected a Lab object but got %T", obj)
	}
	lablog.Info("Validation for Lab upon creation", "name", lab.GetName())

	// TODO(user): fill in your validation logic upon object creation.

	return nil, lab.Spec.Validate()
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type Lab.
func (v *LabCustomValidator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	lab, ok := newObj.(*knlv1beta1.Lab)
	if !ok {
		return nil, fmt.Errorf("expected a Lab object for the newObj but got %T", newObj)
	}
	lablog.Info("Validation for Lab upon update", "name", lab.GetName())

	// TODO(user): fill in your validation logic upon object update.

	return nil, lab.Spec.Validate()
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type Lab.
func (v *LabCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	lab, ok := obj.(*knlv1beta1.Lab)
	if !ok {
		return nil, fmt.Errorf("expected a Lab object but got %T", obj)
	}
	lablog.Info("Validation for Lab upon deletion", "name", lab.GetName())

	// TODO(user): fill in your validation logic upon object deletion.

	return nil, nil
}
