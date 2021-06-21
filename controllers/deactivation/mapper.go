package deactivation

import (
	"context"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/operator-framework/operator-sdk/pkg/k8sutil"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type UserSignupToMasterUserRecordMapper struct {
	client client.Client
}

var _ handler.Mapper = UserSignupToMasterUserRecordMapper{}
var mapperLog = ctrl.Log.WithName("UserSignupToMasterUserRecordMapper")

func (b UserSignupToMasterUserRecordMapper) Map(obj handler.MapObject) []reconcile.Request {
	if userSignup, ok := obj.Object.(*toolchainv1alpha1.UserSignup); ok {
		// look-up any associated MasterUserRecord using the UserSignup's Status.CompliantUsername value
		mur := &toolchainv1alpha1.MasterUserRecord{}

		if err := b.client.Get(context.TODO(), client.ObjectKey{
			Namespace: userSignup.Namespace,
			Name:      userSignup.Status.CompliantUsername,
		}, mur); err != nil {
			mapperLog.Error(err, "Could not find MasterUserRecord resource with name", "name",
				userSignup.Status.CompliantUsername)
			return nil
		}

		req := []reconcile.Request{}

		ns, err := k8sutil.GetWatchNamespace()
		if err != nil {
			mapperLog.Error(err, "Could not determine watched namespace")
			return nil
		}

		req = append(req, reconcile.Request{
			NamespacedName: types.NamespacedName{Namespace: ns, Name: mur.Name},
		})

		return req
	}

	// the obj was not a UserSignup
	return []reconcile.Request{}
}
