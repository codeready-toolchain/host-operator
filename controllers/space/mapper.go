package space

import (
	"context"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var mapperLog = ctrl.Log.WithName("MapSpaceBindingToParentAndSubSpaces")

// MapSpaceBindingToParentAndSubSpaces maps the SpaceBinding of a given Space to it's subSpaces (if any).
// It enables SpaceBinding inheritance from the parentSpace to the eventual subSpaces.
//
// The logic works as following:
// - an event is triggered on a SpaceBinding object (something has changed)
// - from the SpaceBinding labels we get the name of the Space object (a.k.a parentSpace)
// - with the name of the Space (parentSpace) we search for eventual subSpaces
// - in order to reflect the changes on SpaceBinding we trigger a `reconcile.Request` for the Space (parentSpace) and all it's subSpaces (if any)
func MapSpaceBindingToParentAndSubSpaces(cl runtimeclient.Client) func(object runtimeclient.Object) []reconcile.Request {
	return func(obj runtimeclient.Object) []reconcile.Request {
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

		subSpaces := toolchainv1alpha1.SpaceList{}
		subSpaces, err := listSubSpaces(obj.GetNamespace(), cl, subSpaces, parentSpaceName)
		if err != nil {
			logger.Error(err, "unable to get sub-spaces for an object")
			return []reconcile.Request{}
		}
		if len(subSpaces.Items) == 0 {
			return reconcileRequestObj
		}

		// add sub-spaces to the reconcile request together with the parent-space
		for _, item := range subSpaces.Items {
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

// listSubSpaces start from a parentSpace name and recursively searches for subspaces.
func listSubSpaces(ns string, cl runtimeclient.Client, subSpaces toolchainv1alpha1.SpaceList, parentSpaceName string) (toolchainv1alpha1.SpaceList, error) {
	subSpacesFound := &toolchainv1alpha1.SpaceList{}
	err := cl.List(context.TODO(), subSpacesFound,
		runtimeclient.InNamespace(ns),
		runtimeclient.MatchingLabels{toolchainv1alpha1.ParentSpaceLabelKey: parentSpaceName})
	if err != nil {
		return subSpaces, err
	}

	// merge found spaces into existing ones
	subSpaces.Items = append(subSpaces.Items, subSpacesFound.Items...)

	// keep searching for subspaces
	for _, space := range subSpacesFound.Items {
		return listSubSpaces(space.Namespace, cl, subSpaces, space.Name)
	}
	return subSpaces, nil
}
