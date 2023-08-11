package spacebindingrequest

import (
	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"

	"k8s.io/apimachinery/pkg/types"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// MapSpaceBindingToSpaceBindingRequest checks whether a spacebinding was created from a spacebindingrequest, in case it finds the required labels,
// triggers an event for the given spacebindingrequest associated with the spacebinding.
func MapSpaceBindingToSpaceBindingRequest() func(object runtimeclient.Object) []reconcile.Request {
	return func(obj runtimeclient.Object) []reconcile.Request {
		// get eventual spaceBindingRequest name and namespace associated with current SpaceBinding
		spaceBindingRequestName, spaceBindingRequestExists := obj.GetLabels()[toolchainv1alpha1.SpaceBindingRequestLabelKey]
		spaceBindingRequestNamespace, spaceBindingRequestNamespaceExists := obj.GetLabels()[toolchainv1alpha1.SpaceBindingRequestNamespaceLabelKey]
		if spaceBindingRequestNamespaceExists &&
			spaceBindingRequestExists &&
			spaceBindingRequestName != "" &&
			spaceBindingRequestNamespace != "" {
			// reconcile associated spacebindingrequest
			return []reconcile.Request{
				{
					NamespacedName: types.NamespacedName{
						Namespace: spaceBindingRequestNamespace,
						Name:      spaceBindingRequestName,
					},
				},
			}
		}
		return []reconcile.Request{}
	}
}
