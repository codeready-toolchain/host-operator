package pending

import (
	"github.com/codeready-toolchain/api/api/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// ObjectsMapper maps any object to an oldest pending object
type ObjectsMapper struct {
	unapprovedCache *cache
}

// NewUserSignupMapper creates an instance of UserSignupMapper that maps any object to an oldest unapproved UserSignup
func NewUserSignupMapper(client client.Client) ObjectsMapper {
	return NewPendingObjectsMapper(client, &v1alpha1.UserSignup{}, getListOfPendingUserSignups)
}

// NewSpaceMapper creates an instance of SpaceMapper that maps any object to an oldest unapproved Space
func NewSpaceMapper(client client.Client) ObjectsMapper {
	return NewPendingObjectsMapper(client, &v1alpha1.Space{}, getListOfPendingSpaces)
}

// NewPendingObjectsMapper creates an instance of ObjectsMapper that maps any object to an oldest pending object
func NewPendingObjectsMapper(client client.Client, objectType client.Object, getListOfPendingObjects GetListOfPendingObjects) ObjectsMapper {
	return ObjectsMapper{
		unapprovedCache: &cache{
			client:                  client,
			objectType:              objectType,
			getListOfPendingObjects: getListOfPendingObjects,
		},
	}
}

func (b ObjectsMapper) MapToOldestUnapproved(obj client.Object) []reconcile.Request {
	pendingObject := b.unapprovedCache.getOldestPendingObject(obj.GetNamespace())
	if pendingObject == nil {
		return []reconcile.Request{}
	}
	return []reconcile.Request{{
		NamespacedName: types.NamespacedName{Namespace: pendingObject.GetNamespace(), Name: pendingObject.GetName()},
	}}
}
