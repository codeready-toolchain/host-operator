package spacebindingcleanup

import (
	"context"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var mapperLog = ctrl.Log.WithName("MapToSpaceBindingByBoundObjectName")

// MapToSpaceBindingByBoundObjectName maps the bound object (MUR or Space) to the associated SpaceBindings.
// The correct SpaceBindings are listed using the given label whose value should equal to the object's name.
func MapToSpaceBindingByBoundObjectName(cl client.Client, label string) func(object client.Object) []reconcile.Request {
	return func(obj client.Object) []reconcile.Request {
		logger := mapperLog.WithValues("object-name", obj.GetName(), "object-kind", obj.GetObjectKind())
		spaceBindings := &toolchainv1alpha1.SpaceBindingList{}
		err := cl.List(context.TODO(), spaceBindings,
			client.InNamespace(obj.GetNamespace()),
			client.MatchingLabels{label: obj.GetName()})
		if err != nil {
			logger.Error(err, "unable to get SpaceBinding for an object")
			return []reconcile.Request{}
		}
		if len(spaceBindings.Items) == 0 {
			logger.Error(err, "no SpaceBinding found for an object")
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
