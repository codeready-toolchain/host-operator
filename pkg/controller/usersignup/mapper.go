package usersignup

import (
	"context"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/operator-framework/operator-sdk/pkg/k8sutil"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type BannedUserToUserSignupMapper struct {
	client client.Client
}

var _ handler.Mapper = BannedUserToUserSignupMapper{}

func (b BannedUserToUserSignupMapper) Map(obj handler.MapObject) []reconcile.Request {
	if bu, ok := obj.Object.(*toolchainv1alpha1.BannedUser); ok {
		// look-up any associated UserSignup using the BannedUser's "toolchain.dev.openshift.com/email-hash" label
		if emailHashLbl, exists := bu.Labels[toolchainv1alpha1.BannedUserEmailHashLabelKey]; exists {

			labels := map[string]string{toolchainv1alpha1.UserSignupUserEmailHashLabelKey: emailHashLbl}
			opts := client.MatchingLabels(labels)
			userSignupList := &toolchainv1alpha1.UserSignupList{}
			if err := b.client.List(context.TODO(), userSignupList, opts); err != nil {
				log.Error(err, "Could not list UserSignup resources with label value", toolchainv1alpha1.UserSignupUserEmailHashLabelKey, emailHashLbl)
				return nil
			}

			req := []reconcile.Request{}

			ns, err := k8sutil.GetWatchNamespace()
			if err != nil {
				log.Error(err, "Could not determine watched namespace")
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
