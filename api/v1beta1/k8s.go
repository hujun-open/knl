package v1beta1

import (
	"context"
	"fmt"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"kubenetlab.net/knl/common"
	kvv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// if remove is false, createIfNotExistsOrRemove creates target if it is not already exists in the lab's ns,
// otherwise remove the target
// if needOwner is false, skip set and checking owner; this is needed for certain type obj doesn't set owner like PVC
func createIfNotExistsOrRemove(ctx context.Context,
	clnt client.Client, lab *ParsedLab,
	target client.Object, needOwner, remove bool) error {
	var err error
	if needOwner {
		err = lab.SetOwnerFunc(target)
		if err != nil {
			return common.MakeErr(err)
		}
	}
	err = clnt.Get(ctx,
		types.NamespacedName{Namespace: lab.Lab.Namespace, Name: target.GetName()},
		target,
	)
	if err != nil {
		if apierrors.IsNotFound(err) {
			if remove {
				return nil
			}
			//not found, create it
			err = clnt.Create(ctx, target)
			if err != nil {
				return common.MakeErr(err)
			}
		} else {
			return common.MakeErr(err)
		}
	} else {
		if remove {
			if err = clnt.Delete(ctx, target); err != nil {
				return common.MakeErr(err)
			}
			return nil
		} else {
			if needOwner {
				if err = IsOwnedbyLab(target, lab); err != nil {
					return common.MakeErr(err)
				}
			}
		}
	}
	return nil
}

// return nil if the k8s obj is owned by lab
func IsOwnedbyLab(obj metav1.Object, lab *ParsedLab) error {
	objkind := obj.(k8sruntime.Object).GetObjectKind().GroupVersionKind().Kind
	owners := obj.GetOwnerReferences()
	if len(owners) == 0 {
		return fmt.Errorf("%v %v already exists, and it doesn't have owner", objkind, obj.GetName())
	}
	ownerNames := []string{}
	for _, owner := range owners {
		ownerNames = append(ownerNames, owner.Name)
		if owner.UID == lab.Lab.UID {
			return nil
		}
	}
	return fmt.Errorf("%v %v already exists and is owned by %v", objkind, obj.GetName(), strings.Join(ownerNames, ","))
}

type checkFailFunc func(client.Object) error

func createIfNotExistsOrFailedOrRemove(ctx context.Context,
	clnt client.Client, lab *ParsedLab,
	target client.Object, chkFunc checkFailFunc,
	needOwner, remove bool) error {
	var err error
	if needOwner {
		err = lab.SetOwnerFunc(target)
		if err != nil {
			return common.MakeErr(err)
		}
	}
	err = clnt.Get(ctx,
		types.NamespacedName{Namespace: lab.Lab.Namespace, Name: target.GetName()},
		target,
	)
	if err != nil {
		if apierrors.IsNotFound(err) {
			if remove {
				return nil
			}
			//not found, create it
			err = clnt.Create(ctx, target)
			if err != nil {
				return common.MakeErr(err)
			}
		} else {
			return common.MakeErr(err)
		}
	} else {
		if remove {
			if err = clnt.Delete(ctx, target); err != nil {
				return common.MakeErr(err)
			}
			return nil
		} else {

			if needOwner {
				if err = IsOwnedbyLab(target, lab); err != nil {
					return common.MakeErr(err)
				}
			}
		}

	}
	if err = chkFunc(target); err != nil {
		//target failed, recreate
		err := clnt.Delete(ctx, target, client.PropagationPolicy(metav1.DeletePropagationBackground))
		if err != nil {
			return err
		}
		common.WaitForObjGone(ctx, clnt, lab.Lab.Namespace, target)

		err = clnt.Create(ctx, target)
		if err != nil {
			return common.MakeErr(err)
		}
	}
	return nil
}

func checkVMIfail(obj client.Object) error {
	if vmi, ok := obj.(*kvv1.VirtualMachineInstance); ok {
		switch vmi.Status.Phase {
		case kvv1.Failed, kvv1.Succeeded:
			return fmt.Errorf("VMI %v is in phase %v", vmi.Name, vmi.Status.Phase)
		}
		return nil

	} else {
		return fmt.Errorf("object is not a vmi")
	}
}
