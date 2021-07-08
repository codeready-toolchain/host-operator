package usersignup

import (
	"context"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/configuration"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var mapperLog = ctrl.Log.WithName("BannedUserToUserSignupMapper")

func MapBannedUserToUserSignup(cl client.Client) func(object client.Object) []reconcile.Request {
	return func(obj client.Object) []reconcile.Request {
		if bu, ok := obj.(*toolchainv1alpha1.BannedUser); ok {
			// look-up any associated UserSignup using the BannedUser's "toolchain.dev.openshift.com/email-hash" label
			if emailHashLbl, exists := bu.Labels[toolchainv1alpha1.BannedUserEmailHashLabelKey]; exists {

				labels := map[string]string{toolchainv1alpha1.UserSignupUserEmailHashLabelKey: emailHashLbl}
				opts := client.MatchingLabels(labels)
				userSignupList := &toolchainv1alpha1.UserSignupList{}
				if err := cl.List(context.TODO(), userSignupList, opts); err != nil {
					mapperLog.Error(err, "Could not list UserSignup resources with label value", toolchainv1alpha1.UserSignupUserEmailHashLabelKey, emailHashLbl)
					return nil
				}

				req := []reconcile.Request{}

				ns, err := configuration.GetWatchNamespace()
				if err != nil {
					mapperLog.Error(err, "Could not determine watched namespace")
					return nil
				}

				for _, userSignup := range userSignupList.Items {
					req = append(req, reconcile.Request{
						NamespacedName: types.NamespacedName{Namespace: ns, Name: userSignup.Name},
					})
				}

				return req
			}
		}
		// the obj was not a BannedUser or it did not have the required label.
		return []reconcile.Request{}
	}
}
