package deactivation

import (
	"context"
	"errors"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"

	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var mapperLog = ctrl.Log.WithName("UserSignupToMasterUserRecordMapper")

func MapUserSignupToMasterUserRecord() func(ctx context.Context, object runtimeclient.Object) []reconcile.Request {
	return func(ctx context.Context, obj runtimeclient.Object) []reconcile.Request {
		if userSignup, ok := obj.(*toolchainv1alpha1.UserSignup); ok {

			if userSignup.Status.CompliantUsername != "" {
				return []reconcile.Request{{
					NamespacedName: types.NamespacedName{Namespace: userSignup.Namespace, Name: userSignup.Status.CompliantUsername},
				}}
			}

			return []reconcile.Request{}
		}

		// the obj was not a UserSignup
		mapperLog.Error(errors.New("not a usersignup"),
			"UserSignupToMasterUserRecordMapper attempted to map an object that wasn't a UserSignup",
			"obj", obj)
		return []reconcile.Request{}
	}
}
