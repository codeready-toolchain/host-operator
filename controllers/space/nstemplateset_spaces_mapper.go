package space

import (
	"errors"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func MapNSTemplateSetToSpace(hostNamespace string) func(object client.Object) []reconcile.Request {
	mapperLog := ctrl.Log.WithName("NSTemplateSetToSpaceMapper")
	return func(obj client.Object) []reconcile.Request {
		if nsTemplateSet, ok := obj.(*toolchainv1alpha1.NSTemplateSet); ok {
			return []reconcile.Request{{
				NamespacedName: types.NamespacedName{
					Namespace: hostNamespace,
					Name:      nsTemplateSet.GetName(),
				},
			}}
		}
		// the obj was not a NSTemplateSet
		mapperLog.Error(errors.New("not a NSTemplateSet"),
			"MapNSTemplateSetToSpace attempted to map an object that was not a NSTemplateSet",
			"obj", obj)
		return []reconcile.Request{}
	}

}

func MapSpaceBindingToSpace() func(object client.Object) []reconcile.Request {
	mapperLog := ctrl.Log.WithName("SpaceBindingToSpaceMapper")
	return func(obj client.Object) []reconcile.Request {
		if sb, ok := obj.(*toolchainv1alpha1.SpaceBinding); ok {
			if spaceName, found := sb.Labels[toolchainv1alpha1.SpaceBindingSpaceLabelKey]; found {
				return []reconcile.Request{{
					NamespacedName: types.NamespacedName{
						Namespace: sb.Namespace,
						Name:      spaceName,
					},
				}}
			}
			// the obj was not a SpaceBinding
			mapperLog.Error(errors.New("not a SpaceBinding"),
				"MapSpaceBindingToSpace attempted to map a SpaceBinding without any '"+toolchainv1alpha1.SpaceBindingSpaceLabelKey+"' label",
				"obj", obj)
		}
		// the obj was not a SpaceBinding
		mapperLog.Error(errors.New("not a SpaceBinding"),
			"MapSpaceBindingToSpace attempted to map an object that was not a SpaceBinding",
			"obj", obj)
		return []reconcile.Request{}
	}

}
