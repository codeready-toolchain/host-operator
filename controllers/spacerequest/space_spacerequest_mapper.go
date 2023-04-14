package spacerequest

import (
	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// MapSubSpaceToSpaceRequest checks whether a space was created from a spacerequest, in case it finds the required labels,
// triggers an event for the given spacerequest associated with the space.
func MapSubSpaceToSpaceRequest() func(object client.Object) []reconcile.Request {
	return func(obj client.Object) []reconcile.Request {
		// get eventual spaceRequest name and namespace associated with current Space
		spaceRequestName, spaceRequestExists := obj.GetLabels()[toolchainv1alpha1.SpaceRequestLabelKey]
		spaceRequestNamespace, spaceRequestNamespaceExists := obj.GetLabels()[toolchainv1alpha1.SpaceRequestNamespaceLabelKey]
		if spaceRequestNamespaceExists &&
			spaceRequestExists &&
			spaceRequestName != "" &&
			spaceRequestNamespace != "" {
			// reconcile associated spacerequest
			return []reconcile.Request{
				{
					NamespacedName: types.NamespacedName{
						Namespace: spaceRequestNamespace,
						Name:      spaceRequestName,
					},
				},
			}
		}
		return []reconcile.Request{}
	}
}

// MapSecretToSpaceRequest maps secrets to the SpaceRequest that owns it.
func MapSecretToSpaceRequest() func(object client.Object) []reconcile.Request {
	return func(obj client.Object) []reconcile.Request {
		if secret, ok := obj.(*corev1.Secret); ok {
			spaceRequestName, spaceRequestExists := secret.GetLabels()[toolchainv1alpha1.SpaceRequestLabelKey]
			spaceRequestNamespace, spaceRequestNamespaceExists := secret.GetLabels()[toolchainv1alpha1.SpaceRequestNamespaceLabelKey]
			if spaceRequestNamespaceExists &&
				spaceRequestExists &&
				spaceRequestName != "" &&
				spaceRequestNamespace != "" {
				// reconcile associated spacerequest
				return []reconcile.Request{
					{
						NamespacedName: types.NamespacedName{
							Namespace: spaceRequestNamespace,
							Name:      spaceRequestName,
						},
					},
				}
			}
		}
		// the obj was not a Secret
		return []reconcile.Request{}
	}
}
