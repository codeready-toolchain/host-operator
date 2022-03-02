package space

import (
	"context"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/controllers/nstemplatetier"

	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func MapNSTemplateTierToSpaces(namespace string, cl client.Client) func(object client.Object) []reconcile.Request {
	mapperLog := ctrl.Log.WithName("NSTemplateTierToSpaceMapper")
	return func(obj client.Object) []reconcile.Request {
		if tmplTier, ok := obj.(*toolchainv1alpha1.NSTemplateTier); ok {
			matchOutdated, err := nstemplatetier.OutdatedTierSelector(tmplTier)
			if err != nil {
				mapperLog.Error(err, "cannot create outdated tier label selector", "NSTemplateTier", tmplTier)
				return []reconcile.Request{}
			}
			// look-up all spaces associated with the NSTemplateTier
			spaces := &toolchainv1alpha1.SpaceList{}
			if err := cl.List(context.TODO(), spaces, client.InNamespace(namespace), matchOutdated); err != nil {
				mapperLog.Error(err, "cannot list outdated Spaces", "tierName", tmplTier.Name)
				return []reconcile.Request{}
			}
			mapperLog.Info("enqueuing reconcile requests after NSTemplateTier was updated", "name", tmplTier.Name, "request_count", len(spaces.Items))
			requests := make([]reconcile.Request, len(spaces.Items))
			for i, s := range spaces.Items {
				requests[i] = reconcile.Request{NamespacedName: types.NamespacedName{
					Namespace: s.Namespace,
					Name:      s.Name,
				}}
			}
			return requests
		}
		mapperLog.Error(nil, "cannot map to Spaces")
		return []reconcile.Request{}
	}
}
