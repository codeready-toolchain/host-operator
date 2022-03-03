package usersignup

import (
	"context"
	"fmt"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/configuration"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func MapBannedUserToUserSignup(cl client.Client) func(object client.Object) []reconcile.Request {
	var logger = ctrl.Log.WithName("BannedUserToUserSignupMapper")
	return func(obj client.Object) []reconcile.Request {
		if bu, ok := obj.(*toolchainv1alpha1.BannedUser); ok {
			// look-up any associated UserSignup using the BannedUser's "toolchain.dev.openshift.com/email-hash" label
			if emailHashLbl, exists := bu.Labels[toolchainv1alpha1.BannedUserEmailHashLabelKey]; exists {

				labels := map[string]string{toolchainv1alpha1.UserSignupUserEmailHashLabelKey: emailHashLbl}
				opts := client.MatchingLabels(labels)
				userSignupList := &toolchainv1alpha1.UserSignupList{}
				if err := cl.List(context.TODO(), userSignupList, opts); err != nil {
					logger.Error(err, "Could not list UserSignup resources with label value", toolchainv1alpha1.UserSignupUserEmailHashLabelKey, emailHashLbl)
					return nil
				}

				req := []reconcile.Request{}

				ns, err := configuration.GetWatchNamespace()
				if err != nil {
					logger.Error(err, "Could not determine watched namespace")
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

func MapObjectWithCreatorLabelToUserSignup(cl client.Client) func(object client.Object) []reconcile.Request {
	var logger = ctrl.Log.WithName("CreatorToUserSignupMapper")
	return func(obj client.Object) []reconcile.Request {
		if obj.GetLabels() != nil {
			// look-up the associated UserSignup using the object's "toolchain.dev.openshift.com/creator" label
			if userSignupName, exists := obj.GetLabels()[toolchainv1alpha1.SpaceCreatorLabelKey]; exists {
				ns, err := configuration.GetWatchNamespace()
				if err != nil {
					logger.Error(err, "Could not determine watched namespace")
					return nil
				}

				userSignup := &toolchainv1alpha1.UserSignup{}
				if err := cl.Get(context.TODO(), types.NamespacedName{Namespace: ns, Name: userSignupName}, userSignup); err != nil {
					logger.Error(err, "Could not list UserSignup resources with label value", toolchainv1alpha1.SpaceCreatorLabelKey, userSignupName)
					return nil
				}

				return []reconcile.Request{{types.NamespacedName{Namespace: ns, Name: userSignup.Name}}}
			}
		}
		// the obj has no creator label so it cannot be mapped
		logger.Error(fmt.Errorf("object is missing creator label"), "object cannot be mapped")
		return nil
	}
}
