package usersignupcleanup

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	commonconfig "github.com/codeready-toolchain/toolchain-common/pkg/configuration"
	testconfig "github.com/codeready-toolchain/toolchain-common/pkg/test/config"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	"github.com/codeready-toolchain/host-operator/pkg/metrics"
	"github.com/codeready-toolchain/toolchain-common/pkg/states"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	commonsignup "github.com/codeready-toolchain/toolchain-common/pkg/test/usersignup"

	promtestutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	corev1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestUserCleanup(t *testing.T) {
	// A creation time five years in the past
	fiveYears := time.Duration(time.Hour * 24 * 365 * 5)

	// A creation time two years in the past
	twoYears := time.Duration(time.Hour * 24 * 365 * 2)

	// A creation time one year in the past
	oneYear := time.Duration(time.Hour * 24 * 365 * 1)

	t.Run("test that user cleanup doesn't delete an active UserSignup", func(t *testing.T) {

		userSignup := commonsignup.NewUserSignup(
			commonsignup.CreatedBefore(fiveYears),
			commonsignup.WithStateLabel(toolchainv1alpha1.UserSignupStateLabelValueApproved),
			commonsignup.SignupComplete(""),
			commonsignup.ApprovedManuallyAgo(fiveYears),
		)

		r, req, _ := prepareReconcile(t, userSignup.Name, userSignup)

		_, err := r.Reconcile(context.TODO(), req)
		require.NoError(t, err)

		// Confirm the UserSignup still exists
		key := test.NamespacedName(test.HostOperatorNs, userSignup.Name)
		require.NoError(t, r.Client.Get(context.Background(), key, userSignup))
		require.NotNil(t, userSignup)
	})

	t.Run("test that user cleanup doesn't delete a recently deactivated UserSignup", func(t *testing.T) {

		userSignup := commonsignup.NewUserSignup(
			commonsignup.ApprovedManuallyAgo(fiveYears),
			commonsignup.WithStateLabel(toolchainv1alpha1.UserSignupStateLabelValueApproved),
			commonsignup.DeactivatedWithLastTransitionTime(time.Duration(5*time.Minute)),
			commonsignup.CreatedBefore(fiveYears),
		)

		r, req, _ := prepareReconcile(t, userSignup.Name, userSignup)

		res, err := r.Reconcile(context.TODO(), req)
		require.NoError(t, err)

		// Confirm the UserSignup still exists
		key := test.NamespacedName(test.HostOperatorNs, userSignup.Name)
		require.NoError(t, r.Client.Get(context.Background(), key, userSignup))
		require.NotNil(t, userSignup)

		expectRequeue(t, res, 0)
	})

	t.Run("test that an old, deactivated UserSignup is deleted", func(t *testing.T) {

		userSignup := commonsignup.NewUserSignup(
			commonsignup.WithStateLabel(toolchainv1alpha1.UserSignupStateLabelValueApproved),
			commonsignup.ApprovedManuallyAgo(fiveYears),
			commonsignup.DeactivatedWithLastTransitionTime(fiveYears),
			commonsignup.CreatedBefore(fiveYears),
		)

		r, req, _ := prepareReconcile(t, userSignup.Name, userSignup)

		_, err := r.Reconcile(context.TODO(), req)
		require.NoError(t, err)

		// Confirm the UserSignup has been deleted
		key := test.NamespacedName(test.HostOperatorNs, userSignup.Name)
		err = r.Client.Get(context.Background(), key, userSignup)
		require.Error(t, err)
		require.True(t, apierrors.IsNotFound(err))
		statusErr := &apierrors.StatusError{}
		require.ErrorAs(t, err, &statusErr)
		require.Equal(t, fmt.Sprintf("usersignups.toolchain.dev.openshift.com \"%s\" not found", key.Name), statusErr.Error())
		// and verify the metrics
		assert.Equal(t, float64(0), promtestutil.ToFloat64(metrics.UserSignupDeletedWithInitiatingVerificationTotal))    // unchanged
		assert.Equal(t, float64(0), promtestutil.ToFloat64(metrics.UserSignupDeletedWithoutInitiatingVerificationTotal)) // unchanged
	})

	t.Run("test that an old, unverified UserSignup is deleted", func(t *testing.T) {

		userSignup := commonsignup.NewUserSignup(
			commonsignup.CreatedBefore(days(8)),
			commonsignup.VerificationRequired(days(8)),
			commonsignup.WithActivations("0"),
		)

		r, req, _ := prepareReconcile(t, userSignup.Name, userSignup)

		_, err := r.Reconcile(context.TODO(), req)
		require.NoError(t, err)

		// Confirm the UserSignup has been deleted
		key := test.NamespacedName(test.HostOperatorNs, userSignup.Name)
		err = r.Client.Get(context.Background(), key, userSignup)
		require.Error(t, err)
		require.True(t, apierrors.IsNotFound(err))
		require.IsType(t, &apierrors.StatusError{}, err)
		statusErr := &apierrors.StatusError{}
		require.ErrorAs(t, err, &statusErr)
		require.Equal(t, fmt.Sprintf("usersignups.toolchain.dev.openshift.com \"%s\" not found", key.Name), statusErr.Error())
		// and verify the metrics
		assert.Equal(t, float64(0), promtestutil.ToFloat64(metrics.UserSignupDeletedWithInitiatingVerificationTotal))    // unchanged
		assert.Equal(t, float64(1), promtestutil.ToFloat64(metrics.UserSignupDeletedWithoutInitiatingVerificationTotal)) //  incremented

		t.Run("deletion is not initiated twice", func(t *testing.T) {
			alreadyDeletedSignupIgnored(t, userSignup)
		})
	})

	t.Run("without phone verification initiated", func(t *testing.T) {
		// given
		userSignup := commonsignup.NewUserSignup(
			commonsignup.CreatedBefore(days(8)),
			commonsignup.VerificationRequired(days(8)),
		)
		r, req, _ := prepareReconcile(t, userSignup.Name, userSignup)
		// when
		_, err := r.Reconcile(context.TODO(), req)
		require.NoError(t, err)
		// then
		// confirm the UserSignup has been deleted
		key := test.NamespacedName(test.HostOperatorNs, userSignup.Name)
		err = r.Client.Get(context.Background(), key, userSignup)
		require.True(t, apierrors.IsNotFound(err))
		assert.Errorf(t, err, "usersignups.toolchain.dev.openshift.com \"%s\" not found", key.Name)
		// and verify the metrics
		assert.InDelta(t, float64(0), promtestutil.ToFloat64(metrics.UserSignupDeletedWithInitiatingVerificationTotal), 0.01)    // unchanged
		assert.InDelta(t, float64(1), promtestutil.ToFloat64(metrics.UserSignupDeletedWithoutInitiatingVerificationTotal), 0.01) // incremented

		t.Run("deletion is not initiated twice", func(t *testing.T) {
			alreadyDeletedSignupIgnored(t, userSignup)
		})
	})

	t.Run("with phone verification initiated", func(t *testing.T) {
		// given
		userSignup := commonsignup.NewUserSignup(
			commonsignup.CreatedBefore(days(8)),
			commonsignup.VerificationRequired(days(8)),
			commonsignup.WithAnnotation(toolchainv1alpha1.UserSignupVerificationCodeAnnotationKey, "12345"),
		)
		r, req, _ := prepareReconcile(t, userSignup.Name, userSignup)
		// when
		_, err := r.Reconcile(context.TODO(), req)
		require.NoError(t, err)
		// then
		// confirm the UserSignup has been deleted
		key := test.NamespacedName(test.HostOperatorNs, userSignup.Name)
		err = r.Client.Get(context.Background(), key, userSignup)
		require.True(t, apierrors.IsNotFound(err))
		assert.Errorf(t, err, "usersignups.toolchain.dev.openshift.com \"%s\" not found", key.Name)
		// and verify the metrics
		assert.Equal(t, float64(1), promtestutil.ToFloat64(metrics.UserSignupDeletedWithInitiatingVerificationTotal))    // incremented
		assert.Equal(t, float64(0), promtestutil.ToFloat64(metrics.UserSignupDeletedWithoutInitiatingVerificationTotal)) // unchanged

		t.Run("deletion is not initiated twice", func(t *testing.T) {
			alreadyDeletedSignupIgnored(t, userSignup)
		})
	})

	t.Run("test that recently reactivated, unverified UserSignup is NOT deleted", func(t *testing.T) {

		userSignup := commonsignup.NewUserSignup(
			commonsignup.CreatedBefore(fiveYears),
			commonsignup.ApprovedManuallyAgo(days(40)),
			commonsignup.VerificationRequired(days(10)),
			commonsignup.WithActivations("1"),
		)

		r, req, _ := prepareReconcile(t, userSignup.Name, userSignup)

		res, err := r.Reconcile(context.TODO(), req)
		require.NoError(t, err)

		// Confirm the UserSignup has not been deleted
		key := test.NamespacedName(test.HostOperatorNs, userSignup.Name)
		err = r.Client.Get(context.Background(), key, userSignup)
		require.NoError(t, err)
		require.NotNil(t, userSignup)
		require.False(t, res.Requeue)
	})

	t.Run("test that reactivated, unverified UserSignup long time ago is reset", func(t *testing.T) {

		userSignup := commonsignup.NewUserSignup(
			commonsignup.CreatedBefore(fiveYears),
			commonsignup.ApprovedManuallyAgo(days(730+21)),
			commonsignup.VerificationRequired(days(730+1)),
			commonsignup.WithActivations("2"),
		)

		r, req, _ := prepareReconcile(t, userSignup.Name, userSignup)

		_, err := r.Reconcile(context.TODO(), req)
		require.NoError(t, err)

		// Confirm the UserSignup has been deleted
		key := test.NamespacedName(test.HostOperatorNs, userSignup.Name)
		err = r.Client.Get(context.Background(), key, userSignup)
		require.NoError(t, err)
		require.True(t, states.Deactivated(userSignup))
		require.False(t, states.VerificationRequired(userSignup))
	})

	t.Run("test that an old, verified but unapproved UserSignup is not deleted", func(t *testing.T) {

		userSignup := commonsignup.NewUserSignup(
			commonsignup.CreatedBefore(fiveYears),
		)
		states.SetVerificationRequired(userSignup, false)
		states.SetApprovedManually(userSignup, false)

		r, req, _ := prepareReconcile(t, userSignup.Name, userSignup)

		res, err := r.Reconcile(context.TODO(), req)
		require.NoError(t, err)

		// Confirm the UserSignup has not been deleted
		key := test.NamespacedName(test.HostOperatorNs, userSignup.Name)
		err = r.Client.Get(context.Background(), key, userSignup)
		require.NoError(t, err)
		require.NotNil(t, userSignup)
		require.False(t, res.Requeue)
	})

	t.Run("test old deactivated UserSignup cleanup", func(t *testing.T) {
		config := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true),
			testconfig.Deactivation().UserSignupDeactivatedRetentionDays(720))

		tests := map[string]struct {
			userSignup          *toolchainv1alpha1.UserSignup
			banned              bool
			expectedError       string
			expectedToBeDeleted bool
		}{
			"test that a UserSignup older than 2 years, with 1 activation and not banned, is deleted": {
				userSignup: commonsignup.NewUserSignup(
					commonsignup.DeactivatedWithLastTransitionTime(twoYears),
					commonsignup.WithActivations("1")),
				expectedError:       "",
				expectedToBeDeleted: true,
			},
			"test that a UserSignup older than 2 years, with indeterminate activations and not banned, is deleted": {
				userSignup: commonsignup.NewUserSignup(
					commonsignup.DeactivatedWithLastTransitionTime(twoYears),
					commonsignup.WithActivations("unknown")),
				expectedError:       "",
				expectedToBeDeleted: true,
			},
			"test that a UserSignup 1 year old, with 1 activation and not banned, is not deleted": {
				userSignup: commonsignup.NewUserSignup(
					commonsignup.DeactivatedWithLastTransitionTime(oneYear),
					commonsignup.WithActivations("1")),
				expectedError:       "",
				expectedToBeDeleted: false,
			},
			"test that a UserSignup older than 2 years, with 2 activations and not banned, is not deleted": {
				userSignup: commonsignup.NewUserSignup(
					commonsignup.DeactivatedWithLastTransitionTime(twoYears),
					commonsignup.WithActivations("2")),
				expectedError:       "",
				expectedToBeDeleted: false,
			},
			"test that a UserSignup older than 2 years, with 1 activation but has been banned, is not deleted": {
				userSignup: commonsignup.NewUserSignup(
					commonsignup.DeactivatedWithLastTransitionTime(twoYears),
					commonsignup.WithActivations("1")),
				banned:              true,
				expectedError:       "",
				expectedToBeDeleted: false,
			},
			"test that a banned UserSignup with an invalid email hash returns an error and is not deleted": {
				userSignup: commonsignup.NewUserSignup(
					commonsignup.DeactivatedWithLastTransitionTime(twoYears),
					commonsignup.WithActivations("1"),
					commonsignup.WithName("invalid-email-user"),
					commonsignup.WithLabel(toolchainv1alpha1.UserSignupUserEmailHashLabelKey, "INVALID")),
				banned:              true,
				expectedError:       "email hash is invalid for UserSignup [invalid-email-user]",
				expectedToBeDeleted: false,
			},
			"test that a UserSignup without an email address returns an error and is not deleted": {
				userSignup: commonsignup.NewUserSignup(
					commonsignup.DeactivatedWithLastTransitionTime(twoYears),
					commonsignup.WithActivations("1"),
					commonsignup.WithName("without-email-user"),
					commonsignup.WithEmail("")),
				expectedError:       "could not determine email address for UserSignup [without-email-user]",
				expectedToBeDeleted: false,
			},
		}

		for k, tc := range tests {
			t.Run(k, func(t *testing.T) {
				var r *Reconciler
				var req reconcile.Request
				if tc.banned {
					bannedUser := &toolchainv1alpha1.BannedUser{
						ObjectMeta: corev1.ObjectMeta{
							Labels: map[string]string{
								toolchainv1alpha1.BannedUserEmailHashLabelKey: tc.userSignup.Labels[toolchainv1alpha1.UserSignupUserEmailHashLabelKey],
							},
						},
						Spec: toolchainv1alpha1.BannedUserSpec{
							Email: tc.userSignup.Spec.IdentityClaims.Email,
						},
					}
					r, req, _ = prepareReconcile(t, tc.userSignup.Name, tc.userSignup, config, bannedUser)
				} else {
					r, req, _ = prepareReconcile(t, tc.userSignup.Name, tc.userSignup, config)
				}

				_, err := r.Reconcile(context.TODO(), req)
				if tc.expectedError != "" {
					require.Error(t, err)
					require.Equal(t, tc.expectedError, err.Error())
				} else {
					require.NoError(t, err)
				}

				key := test.NamespacedName(test.HostOperatorNs, tc.userSignup.Name)
				err = r.Client.Get(context.Background(), key, tc.userSignup)
				if tc.expectedToBeDeleted {
					// Confirm the UserSignup has been deleted
					require.Error(t, err)
					require.True(t, apierrors.IsNotFound(err))
					require.IsType(t, &apierrors.StatusError{}, err)
					statusErr := &apierrors.StatusError{}
					require.ErrorAs(t, err, &statusErr)
					require.Equal(t, fmt.Sprintf("usersignups.toolchain.dev.openshift.com \"%s\" not found", key.Name), statusErr.Error())
				} else {
					// Confirm the UserSignup has not been deleted
					require.NoError(t, err)
					require.NotNil(t, tc.userSignup)
				}
			})
		}
	})

	t.Run("test propagation policy", func(t *testing.T) {
		userSignup := commonsignup.NewUserSignup(
			commonsignup.CreatedBefore(fiveYears),
			commonsignup.VerificationRequired(days(8)),
		)

		r, req, fakeClient := prepareReconcile(t, userSignup.Name, userSignup)
		deleted := false
		fakeClient.MockDelete = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.DeleteOption) error {
			deleted = true
			require.Len(t, opts, 1)
			deleteOptions, ok := opts[0].(*runtimeclient.DeleteOptions)
			require.True(t, ok)
			require.NotNil(t, deleteOptions)
			require.NotNil(t, deleteOptions.PropagationPolicy)
			assert.Equal(t, corev1.DeletePropagationForeground, *deleteOptions.PropagationPolicy)
			return nil
		}
		_, err := r.Reconcile(context.TODO(), req)
		require.NoError(t, err)
		assert.True(t, deleted)
	})
}

func alreadyDeletedSignupIgnored(t *testing.T, userSignup *toolchainv1alpha1.UserSignup) {
	// Now let's simulate the situation when the signup is already being deleted and reconcile it again.
	nw := corev1.Now()
	userSignup.DeletionTimestamp = &nw
	r, req, _ := prepareReconcile(t, userSignup.Name, userSignup)

	res, err := r.Reconcile(context.TODO(), req)
	require.NoError(t, err)
	require.Empty(t, res)

	// The UserSignup should still be present because signups with a non-empty deletion timestamp are ignored.
	key := test.NamespacedName(test.HostOperatorNs, userSignup.Name)
	err = r.Client.Get(context.Background(), key, userSignup)
	require.NoError(t, err)

	// And verify that the metrics stay unchanged after they were reset to "0" when we prepared the reconcile above.
	assert.Equal(t, float64(0), promtestutil.ToFloat64(metrics.UserSignupDeletedWithInitiatingVerificationTotal))
	assert.Equal(t, float64(0), promtestutil.ToFloat64(metrics.UserSignupDeletedWithoutInitiatingVerificationTotal))
}

func expectRequeue(t *testing.T, res reconcile.Result, margin int) {
	// We expect the requeue duration to be approximately equal to the default retention time of 1460 days. Let's
	// accept any value here between the range of 364 days and 366 days
	durLower := time.Duration(days(1460 - 1 - margin))
	durUpper := time.Duration(days(1460 + 1 - margin))

	require.True(t, res.Requeue)
	require.Greater(t, res.RequeueAfter, durLower)
	require.Less(t, res.RequeueAfter, durUpper)
}

func days(days int) time.Duration {
	return time.Hour * time.Duration(days*24)
}

func prepareReconcile(t *testing.T, name string, initObjs ...runtime.Object) (*Reconciler, reconcile.Request, *test.FakeClient) { // nolint: unparam
	os.Setenv("WATCH_NAMESPACE", test.HostOperatorNs)
	metrics.Reset()

	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)

	fakeClient := test.NewFakeClient(t, initObjs...)

	r := &Reconciler{
		Scheme: s,
		Client: fakeClient,
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
