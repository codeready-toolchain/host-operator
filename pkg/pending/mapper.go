package pending

import (
	"context"

	"github.com/codeready-toolchain/api/api/v1alpha1"

	"k8s.io/apimachinery/pkg/types"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// ObjectsMapper maps any object to an oldest pending object
type ObjectsMapper struct {
	unapprovedCache *cache
}

// NewUserSignupMapper creates an instance of UserSignupMapper that maps any object to an oldest unapproved UserSignup
func NewUserSignupMapper(client runtimeclient.Client) ObjectsMapper {
	return NewPendingObjectsMapper(client, &v1alpha1.UserSignup{}, listPendingUserSignups)
}

// NewSpaceMapper creates an instance of SpaceMapper that maps any object to an oldest unapproved Space
func NewSpaceMapper(client runtimeclient.Client) ObjectsMapper {
	return NewPendingObjectsMapper(client, &v1alpha1.Space{}, listPendingSpaces)
}

// NewPendingObjectsMapper creates an instance of ObjectsMapper that maps any object to an oldest pending object
func NewPendingObjectsMapper(client runtimeclient.Client, objectType runtimeclient.Object, listPendingObjects ListPendingObjects) ObjectsMapper {
	return ObjectsMapper{
		unapprovedCache: &cache{
			client:             client,
			objectType:         objectType,
			listPendingObjects: listPendingObjects,
		},
	}
}

func (b ObjectsMapper) BuildMapToOldestPending(ctx context.Context) handler.MapFunc {
	return func(obj runtimeclient.Object) []reconcile.Request {
		return b.MapToOldestPending(ctx, obj)
	}
}

func (b ObjectsMapper) MapToOldestPending(ctx context.Context, obj runtimeclient.Object) []reconcile.Request {
	pendingObject := b.unapprovedCache.getOldestPendingObject(ctx, obj.GetNamespace())
	if pendingObject == nil {
		return []reconcile.Request{}
	}
	return []reconcile.Request{{
		NamespacedName: types.NamespacedName{Namespace: pendingObject.GetNamespace(), Name: pendingObject.GetName()},
	}}
}
