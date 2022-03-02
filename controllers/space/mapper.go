package space

import (
	"errors"
	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)
var mapperLog = ctrl.Log.WithName("NSTemplateSetToSpaceMapper")

func MapNSTemplateSetToSpace(hostNamespace string) func(object client.Object) []reconcile.Request {
	return func(obj client.Object) []reconcile.Request {
		if nsTemplateSet, ok := obj.(*toolchainv1alpha1.NSTemplateSet); ok {
			return []reconcile.Request{{
				NamespacedName: types.NamespacedName{Namespace: hostNamespace, Name: nsTemplateSet.GetName()},
			}}
		}
		// the obj was not a NSTemplateSet
		mapperLog.Error(errors.New("not a NSTemplateSet"),
			"MapNSTemplateSetToSpace attempted to map an object that wasn't a NSTemplateSet",
			"obj", obj)
		return []reconcile.Request{}
	}

}