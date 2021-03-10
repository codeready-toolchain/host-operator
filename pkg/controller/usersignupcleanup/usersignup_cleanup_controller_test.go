package usersignupcleanup

import (
	"context"
	"fmt"
	"github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	"github.com/codeready-toolchain/host-operator/pkg/configuration"
	"github.com/codeready-toolchain/host-operator/pkg/metrics"
	test2 "github.com/codeready-toolchain/host-operator/test"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"testing"
	"time"

	"k8s.io/client-go/kubernetes/scheme"
)

func TestUserCleanup(t *testing.T) {
	// A creation time three years in the past
	old := time.Now().AddDate(-3, 0, 0)

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
		userSignup.ObjectMeta.CreationTimestamp = metav1.Time{Time: old}
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

	t.Run("test that an old, deactivated UserSignup is deleted", func(t *testing.T) {

		userSignup := &v1alpha1.UserSignup{
			ObjectMeta: test2.NewUserSignupObjectMeta("", "jessica.lansbury@redhat.com"),
			Spec: v1alpha1.UserSignupSpec{
				UserID:      "User-91923",
				Username:    "jessica.lansbury@redhat.com",
				Deactivated: true,
			},
			Status: v1alpha1.UserSignupStatus{
				Conditions: []v1alpha1.Condition{
					{
						Type:            v1alpha1.UserSignupComplete,
						Status:          v1.ConditionTrue,
						Reason:          v1alpha1.UserSignupUserDeactivatedReason,
						LastUpdatedTime: &metav1.Time{Time: old},
					},
					{
						Type:   v1alpha1.UserSignupApproved,
						Status: v1.ConditionTrue,
						Reason: "ApprovedAutomatically",
					},
				},
				CompliantUsername: "jessica-lansbury",
			},
		}
		userSignup.ObjectMeta.CreationTimestamp = metav1.Time{Time: old}
		userSignup.Labels["toolchain.dev.openshift.com/approved"] = "true"
		userSignup.Labels[v1alpha1.UserSignupStateLabelKey] = v1alpha1.UserSignupStateLabelValueDeactivated

		r, req, _ := prepareReconcile(t, userSignup.Name, userSignup)

		_, err := r.Reconcile(req)
		require.NoError(t, err)

		// Confirm the UserSignup has been deleted
		key := test.NamespacedName(test.HostOperatorNs, userSignup.Name)
		err = r.client.Get(context.Background(), key, userSignup)
		require.Error(t, err)
		require.True(t, errors.IsNotFound(err))
		statusErr := err.(*errors.StatusError)
		require.Equal(t, fmt.Sprintf("usersignups.toolchain.dev.openshift.com \"%s\" not found", key.Name), statusErr.Error())
	})

	t.Run("test that an old, unverified UserSignup is deleted", func(t *testing.T) {

		userSignup := &v1alpha1.UserSignup{
			ObjectMeta: test2.NewUserSignupObjectMeta("", "brandon.elder@redhat.com"),
			Spec: v1alpha1.UserSignupSpec{
				UserID:               "User-83922",
				Username:             "brandon.elder@redhat.com",
				VerificationRequired: true,
			},
			Status: v1alpha1.UserSignupStatus{
				Conditions: []v1alpha1.Condition{
					{
						Type:            v1alpha1.UserSignupComplete,
						Status:          v1.ConditionFalse,
						Reason:          v1alpha1.UserSignupVerificationRequiredReason,
						LastUpdatedTime: &metav1.Time{Time: old},
					},
				},
				CompliantUsername: "brandon-elder",
			},
		}
		userSignup.ObjectMeta.CreationTimestamp = metav1.Time{Time: old}
		userSignup.Labels[v1alpha1.UserSignupStateLabelKey] = v1alpha1.UserSignupStateLabelValuePending

		r, req, _ := prepareReconcile(t, userSignup.Name, userSignup)

		_, err := r.Reconcile(req)
		require.NoError(t, err)

		// Confirm the UserSignup has been deleted
		key := test.NamespacedName(test.HostOperatorNs, userSignup.Name)
		err = r.client.Get(context.Background(), key, userSignup)
		require.Error(t, err)
		require.True(t, errors.IsNotFound(err))
		require.IsType(t, &errors.StatusError{}, err)
		statusErr := err.(*errors.StatusError)
		require.Equal(t, fmt.Sprintf("usersignups.toolchain.dev.openshift.com \"%s\" not found", key.Name), statusErr.Error())
	})

	t.Run("test that an old, verified but unapproved UserSignup is not deleted", func(t *testing.T) {

		userSignup := &v1alpha1.UserSignup{
			ObjectMeta: test2.NewUserSignupObjectMeta("", "charles.xavier@xmen.com"),
			Spec: v1alpha1.UserSignupSpec{
				UserID:               "User-73211",
				Username:             "charles.xavier@xmen.com",
				VerificationRequired: false,
				Approved:             false,
			},
			Status: v1alpha1.UserSignupStatus{
				Conditions: []v1alpha1.Condition{
					{
						Type:            v1alpha1.UserSignupComplete,
						Status:          v1.ConditionFalse,
						Reason:          v1alpha1.UserSignupVerificationRequiredReason,
						LastUpdatedTime: &metav1.Time{Time: old},
					},
				},
				CompliantUsername: "charles-xavier",
			},
		}
		userSignup.ObjectMeta.CreationTimestamp = metav1.Time{Time: old}
		userSignup.Labels[v1alpha1.UserSignupStateLabelKey] = v1alpha1.UserSignupStateLabelValuePending

		r, req, _ := prepareReconcile(t, userSignup.Name, userSignup)

		res, err := r.Reconcile(req)
		require.NoError(t, err)

		// Confirm the UserSignup has not been deleted
		key := test.NamespacedName(test.HostOperatorNs, userSignup.Name)
		err = r.client.Get(context.Background(), key, userSignup)
		require.NoError(t, err)
		require.NotNil(t, userSignup)
		require.False(t, res.Requeue)
	})

}

func prepareReconcile(t *testing.T, name string, initObjs ...runtime.Object) (*ReconcileUserSignupCleanup, reconcile.Request, *test.FakeClient) {
	metrics.Reset()

	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)

	initObjs = append(initObjs)

	fakeClient := test.NewFakeClient(t, initObjs...)
	config, err := configuration.LoadConfig(fakeClient)
	require.NoError(t, err)

	r := &ReconcileUserSignupCleanup{
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
