package nstemplatetier

import (
	"context"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"

	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var mapperLog = ctrl.Log.WithName("MapTierTemplateToNSTemplateTier")

// MapTierTemplateToNSTemplateTier maps the TierTemplate to all the NSTemplateTiers that are referecing it.
func MapTierTemplateToNSTemplateTier(cl runtimeclient.Client) func(ctx context.Context, object runtimeclient.Object) []reconcile.Request {
	return func(ctx context.Context, obj runtimeclient.Object) []reconcile.Request {
		logger := mapperLog.WithValues("object-name", obj.GetName(), "object-kind", obj.GetObjectKind())
		nsTmplTierList := &toolchainv1alpha1.NSTemplateTierList{}
		err := cl.List(context.TODO(), nsTmplTierList,
			runtimeclient.InNamespace(obj.GetNamespace()))
		if err != nil {
			logger.Error(err, "unable to list NSTemplateTier")
			return []reconcile.Request{}
		}
		if len(nsTmplTierList.Items) == 0 {
			logger.Info("no NSTemplateTier found for an object")
			return []reconcile.Request{}
		}

		req := make([]reconcile.Request, 0, len(nsTmplTierList.Items))
		for _, item := range nsTmplTierList.Items {
			if len(item.Status.Revisions) == 0 {
				// this tier doesn't use the template
				continue
			}
			_, inUse := item.Status.Revisions[obj.GetName()]
			if !inUse {
				// this tier doesn't use the template
				continue
			}
			// the tier uses this template
			req = append(req, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: item.Namespace,
					Name:      item.Name,
				},
			})
		}
		return req
	}
}
