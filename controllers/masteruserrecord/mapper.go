package masteruserrecord

import (
	"context"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// MapSpaceToMasterUserRecord maps events on Spaces to MasterUserRecords by labels at SpaceBindings
func MapSpaceToMasterUserRecord(cl runtimeclient.Client) func(object runtimeclient.Object) []reconcile.Request {
	var logger = ctrl.Log.WithName("SpaceToMasterUserRecordMapper")
	return func(obj runtimeclient.Object) []reconcile.Request {
		spaceBindings := &toolchainv1alpha1.SpaceBindingList{}
		err := cl.List(context.TODO(), spaceBindings,
			runtimeclient.InNamespace(obj.GetNamespace()),
			runtimeclient.MatchingLabels{toolchainv1alpha1.SpaceBindingSpaceLabelKey: obj.GetName()})
		if err != nil {
			logger.Error(err, "Could not list SpaceBindings for the given Space")
			return nil
		}
		var requests []reconcile.Request
		for _, binding := range spaceBindings.Items {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: obj.GetNamespace(),
					Name:      binding.Labels[toolchainv1alpha1.SpaceBindingMasterUserRecordLabelKey],
				},
			})
		}
		return requests
	}
}
