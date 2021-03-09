package usersignupcleanup

import (
	"context"
	"github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	"github.com/codeready-toolchain/host-operator/pkg/configuration"
	"github.com/codeready-toolchain/host-operator/pkg/metrics"
	test2 "github.com/codeready-toolchain/host-operator/test"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"testing"
	"time"

	"k8s.io/client-go/kubernetes/scheme"
)

func TestUserCleanup(t *testing.T) {
	t.Run("test that user cleanup doesn't delete an active UserSignup", func(t *testing.T) {
		userSignup := &v1alpha1.UserSignup{
			ObjectMeta: test2.NewUserSignupObjectMeta("", "abigail.thompson@redhat.com"),
			Spec: v1alpha1.UserSignupSpec{
				UserID:      "User-98887",
				Username:    "abigail.thompson@redhat.com",
				Deactivated: false,
			},
			Status: v1alpha1.UserSignupStatus{
				Conditions: []v1alpha1.Condition{
					{
						Type:   v1alpha1.UserSignupComplete,
						Status: v1.ConditionTrue,
					},
					{
						Type:   v1alpha1.UserSignupApproved,
						Status: v1.ConditionTrue,
						Reason: "ApprovedAutomatically",
					},
				},
				CompliantUsername: "abigail-thompson",
			},
		}
		userSignup.Labels["toolchain.dev.openshift.com/approved"] = "true"
		userSignup.Labels[v1alpha1.UserSignupStateLabelKey] = "approved"

		r, req, _ := prepareReconcile(t, userSignup.Name, userSignup)

		_, err := r.Reconcile(req)
		require.NoError(t, err)

		// Confirm the UserSignup still exists
		key := test.NamespacedName(test.HostOperatorNs, userSignup.Name)
		require.NoError(t, r.client.Get(context.Background(), key, userSignup))
		require.NotNil(t, userSignup)
	})

	t.Run("test that user cleanup doesn't delete a recently deactivated UserSignup", func(t *testing.T) {

		userSignup := &v1alpha1.UserSignup{
			ObjectMeta: test2.NewUserSignupObjectMeta("", "brian.anderson@redhat.com"),
			Spec: v1alpha1.UserSignupSpec{
				UserID:      "User-93321",
				Username:    "brian.anderson@redhat.com",
				Approved:    true,
				Deactivated: true,
			},
			Status: v1alpha1.UserSignupStatus{
				Conditions: []v1alpha1.Condition{
					{
						Type:               v1alpha1.UserSignupComplete,
						Status:             v1.ConditionTrue,
						Reason:             v1alpha1.UserSignupUserDeactivatedReason,
						LastTransitionTime: metav1.Time{Time: time.Now()},
					},
					{
						Type:   v1alpha1.UserSignupApproved,
						Status: v1.ConditionTrue,
						Reason: "ApprovedAutomatically",
					},
				},
				CompliantUsername: "brian.anderson",
			},
		}
		userSignup.Labels["toolchain.dev.openshift.com/approved"] = "true"
		userSignup.Labels[v1alpha1.UserSignupStateLabelKey] = "approved"

		r, req, _ := prepareReconcile(t, userSignup.Name, userSignup)

		res, err := r.Reconcile(req)
		require.NoError(t, err)

		// Confirm the UserSignup still exists
		key := test.NamespacedName(test.HostOperatorNs, userSignup.Name)
		require.NoError(t, r.client.Get(context.Background(), key, userSignup))
		require.NotNil(t, userSignup)
		require.True(t, res.Requeue)

		// We expect the requeue duration to be approximately equal to the default retention time of 90 days. Let's
		// accept any value here between the range of 89 days and 91 days
		durLower := time.Duration(89 * time.Hour * 24)
		durUpper := time.Duration(91 * time.Hour * 24)

		require.Greater(t, res.RequeueAfter, durLower)
		require.Less(t, res.RequeueAfter, durUpper)
	})

}

func prepareReconcile(t *testing.T, name string, initObjs ...runtime.Object) (*ReconcileUserCleanup, reconcile.Request, *test.FakeClient) {
	metrics.Reset()

	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)

	initObjs = append(initObjs)

	fakeClient := test.NewFakeClient(t, initObjs...)
	config, err := configuration.LoadConfig(fakeClient)
	require.NoError(t, err)

	r := &ReconcileUserCleanup{
		scheme:    s,
		crtConfig: config,
		client:    fakeClient,
	}
	return r, newReconcileRequest(name), fakeClient
}

func newReconcileRequest(name string) reconcile.Request {
	return reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      name,
			Namespace: test.HostOperatorNs,
		},
	}
}
