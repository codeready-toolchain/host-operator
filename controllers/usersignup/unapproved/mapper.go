package unapproved

import (
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// UserSignupMapper maps any object to an oldest unapproved UserSignup
type UserSignupMapper struct {
	unapprovedCache *cache
}

// NewUserSignupMapper creates an instance of UserSignupMapper that maps any object to an oldest unapproved UserSignup
func NewUserSignupMapper(client client.Client) UserSignupMapper {
	return UserSignupMapper{
		unapprovedCache: &cache{
			client: client,
		},
	}
}

var _ handler.Mapper = UserSignupMapper{}

func (b UserSignupMapper) Map(obj handler.MapObject) []reconcile.Request {
	userSignup := b.unapprovedCache.getOldestPendingApproval(obj.Meta.GetNamespace())
	if userSignup == nil {
		return []reconcile.Request{}
	}
	return []reconcile.Request{{
		NamespacedName: types.NamespacedName{Namespace: userSignup.Namespace, Name: userSignup.Name},
	}}
}
