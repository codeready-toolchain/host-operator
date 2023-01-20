package space

import (
	"context"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var mapperLog = ctrl.Log.WithName("MapToSubSpacesByParentObjectName")

// MapToSubSpacesByParentObjectName maps the sup-spaces to the SpaceBinding of a given parent-space.
// The correct Spaces are listed using the parent-label whose value should equal to the Space name from object's .
func MapToSubSpacesByParentObjectName(cl client.Client) func(object client.Object) []reconcile.Request {
	return func(obj client.Object) []reconcile.Request {
		logger := mapperLog.WithValues("object-name", obj.GetName(), "object-kind", obj.GetObjectKind())

		// initialize request slice to be returned
		var reconcileRequestObj []reconcile.Request
		// temporary variable that will be used to search for eventual sub-spaces
		parentSpaceName := ""

		// get eventual parent-space name associated with current SpaceBinding
		if name, exists := obj.GetLabels()[toolchainv1alpha1.SpaceBindingSpaceLabelKey]; exists && name != "" {
			// save space name in order to check if it has any sub-spaces
			parentSpaceName = name
			// add space name to the reconcile request list
			reconcileRequestObj = []reconcile.Request{
				{
					NamespacedName: types.NamespacedName{
						Namespace: obj.GetNamespace(),
						Name:      name,
					},
				},
			}
		} else {
			logger.Info("SpaceBinding is missing label key: " + toolchainv1alpha1.SpaceBindingSpaceLabelKey)
			return []reconcile.Request{}
		}

		// list sub-spaces of given parent-space
		// by setting the label value that each sub-space should have.
		subSpaces := &toolchainv1alpha1.SpaceList{}
		err := cl.List(context.TODO(), subSpaces,
			client.InNamespace(obj.GetNamespace()),
			client.MatchingLabels{toolchainv1alpha1.ParentSpaceLabelKey: parentSpaceName})
		if err != nil {
			logger.Error(err, "unable to get sub-spaces for an object")
			return []reconcile.Request{}
		}
		if len(subSpaces.Items) == 0 {
			logger.Info("no sub-spaces found for an object")
			return reconcileRequestObj
		}

		// add sub-spaces to the reconcile request together with the parent-space
		for _, item := range subSpaces.Items {
			logger.Info("adding sub-space to reconcile request: " + item.Name)
			reconcileRequestObj = append(reconcileRequestObj, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: item.Namespace,
					Name:      item.Name,
				},
			})
		}
		return reconcileRequestObj
	}
}
