package nstemplatetier

import (
	"context"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// MapTierTemplateRevisionToNSTemplateTier returns a reconcile request for the NSTemplateTier it is part of.
func MapTierTemplateRevisionToNSTemplateTier() func(ctx context.Context, object runtimeclient.Object) []reconcile.Request {
	mapperLog := ctrl.Log.WithName("TierTemplateRevisionToNSTemplatetier")
	return func(ctx context.Context, obj runtimeclient.Object) []reconcile.Request {
		if ttr, ok := obj.(*toolchainv1alpha1.TierTemplateRevision); ok {
			if tierName, exists := ttr.GetLabels()[toolchainv1alpha1.TierLabelKey]; exists && tierName != "" {
				// add tiername to the reconcile request list
				return []reconcile.Request{
					{
						NamespacedName: types.NamespacedName{
							Namespace: ttr.GetNamespace(),
							Name:      tierName,
						},
					},
				}
			} else {
				mapperLog.Info("TierTemplateRevision is missing label key: " + toolchainv1alpha1.TierLabelKey)
				return []reconcile.Request{}
			}
		}
		return []reconcile.Request{}
	}
}
