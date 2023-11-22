package spacebindingrequestmigration

import (
	"context"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"

	"k8s.io/apimachinery/pkg/types"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// MapSpaceBindingRequestToSpaceBinding returns an event for the spacebinding that owns.
func MapSpaceBindingRequestToSpaceBinding(cl runtimeclient.Client, watchNamespace string) func(spaceBindingRequest runtimeclient.Object) []reconcile.Request {
	mapperLog := ctrl.Log.WithName("SpaceBindingRequestToSpaceBinding")
	return func(obj runtimeclient.Object) []reconcile.Request {
		spaceBindings := &toolchainv1alpha1.SpaceBindingList{}
		err := cl.List(context.TODO(), spaceBindings,
			runtimeclient.InNamespace(watchNamespace),
			runtimeclient.MatchingLabels{
				toolchainv1alpha1.SpaceBindingRequestLabelKey:          obj.GetName(),
				toolchainv1alpha1.SpaceBindingRequestNamespaceLabelKey: obj.GetNamespace(),
			})
		if err != nil {
			mapperLog.Error(err, "cannot list spacebindings")
			return []reconcile.Request{}
		}
		if len(spaceBindings.Items) == 0 {
			return []reconcile.Request{}
		}

		req := make([]reconcile.Request, len(spaceBindings.Items))
		for i, item := range spaceBindings.Items {
			req[i] = reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: item.Namespace,
					Name:      item.Name,
				},
			}
		}
		return req
	}
}
