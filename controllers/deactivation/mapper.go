package deactivation

import (
	"errors"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
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

		req := []reconcile.Request{}

		req = append(req, reconcile.Request{
			NamespacedName: types.NamespacedName{Namespace: userSignup.Namespace, Name: userSignup.Status.CompliantUsername},
		})

		return req
	}

	// the obj was not a UserSignup
	mapperLog.Error(errors.New("not a usersignup"),
		"UserSignupToMasterUserRecordMapper attempted to map an object that wasn't a UserSignup",
		"obj", obj)
	return []reconcile.Request{}
}
