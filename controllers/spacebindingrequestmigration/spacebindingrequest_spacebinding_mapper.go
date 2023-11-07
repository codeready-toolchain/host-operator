package spacebindingrequestmigration

import (
	"context"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	commonconfig "github.com/codeready-toolchain/toolchain-common/pkg/configuration"
	ctrl "sigs.k8s.io/controller-runtime"

	"k8s.io/apimachinery/pkg/types"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// MapSpaceBindingRequestToSpaceBinding returns an event for the spacebinding that owns.
func MapSpaceBindingRequestToSpaceBinding(cl runtimeclient.Client) func(spaceBindingREquest runtimeclient.Object) []reconcile.Request {
	mapperLog := ctrl.Log.WithName("NSTemplateTierToSpaceMapper")
	return func(obj runtimeclient.Object) []reconcile.Request {
		if spaceBindingRequest, ok := obj.(*toolchainv1alpha1.SpaceBindingRequest); ok {
			watchNamespace, err := commonconfig.GetWatchNamespace()
			if err != nil {
				mapperLog.Error(err, "cannot get watch namespace")
				return []reconcile.Request{}
			}
			spaceBindings := &toolchainv1alpha1.SpaceBindingList{}
			err = cl.List(context.TODO(), spaceBindings,
				runtimeclient.InNamespace(watchNamespace),
				runtimeclient.MatchingLabels{
					toolchainv1alpha1.SpaceBindingRequestLabelKey:          spaceBindingRequest.GetName(),
					toolchainv1alpha1.SpaceBindingRequestNamespaceLabelKey: spaceBindingRequest.GetNamespace(),
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
		return []reconcile.Request{}
	}
}
