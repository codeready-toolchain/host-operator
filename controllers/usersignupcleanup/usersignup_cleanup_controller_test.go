package usersignupcleanup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

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
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestUserCleanup(t *testing.T) {
	// A creation time three years in the past
	threeYears := time.Duration(time.Hour * 24 * 365 * 3)

	t.Run("test that user cleanup doesn't delete an active UserSignup", func(t *testing.T) {

		userSignup := commonsignup.NewUserSignup(
			commonsignup.CreatedBefore(threeYears),
			commonsignup.WithStateLabel(toolchainv1alpha1.UserSignupStateLabelValueApproved),
			commonsignup.SignupComplete(""),
			commonsignup.ApprovedAutomatically(threeYears),
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
			commonsignup.ApprovedAutomatically(threeYears),
			commonsignup.WithStateLabel(toolchainv1alpha1.UserSignupStateLabelValueApproved),
			commonsignup.DeactivatedWithLastTransitionTime(time.Duration(5*time.Minute)),
			commonsignup.CreatedBefore(threeYears),
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
			commonsignup.ApprovedAutomatically(threeYears),
			commonsignup.DeactivatedWithLastTransitionTime(threeYears),
			commonsignup.CreatedBefore(threeYears),
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
		require.True(t, errors.As(err, &statusErr))
		require.Equal(t, fmt.Sprintf("usersignups.toolchain.dev.openshift.com \"%s\" not found", key.Name), statusErr.Error())
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
		require.True(t, errors.As(err, &statusErr))
		require.Equal(t, fmt.Sprintf("usersignups.toolchain.dev.openshift.com \"%s\" not found", key.Name), statusErr.Error())
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
		assert.Equal(t, float64(0), promtestutil.ToFloat64(metrics.UserSignupDeletedWithInitiatingVerificationTotal))    // unchanged
		assert.Equal(t, float64(1), promtestutil.ToFloat64(metrics.UserSignupDeletedWithoutInitiatingVerificationTotal)) // incremented
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
	})

	t.Run("test that recently reactivated, unverified UserSignup is NOT deleted", func(t *testing.T) {

		userSignup := commonsignup.NewUserSignup(
			commonsignup.CreatedBefore(threeYears),
			commonsignup.ApprovedAutomatically(days(40)),
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
			commonsignup.CreatedBefore(threeYears),
			commonsignup.ApprovedAutomatically(days(730+21)),
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
			commonsignup.CreatedBefore(threeYears),
		)
		states.SetVerificationRequired(userSignup, false)
		states.SetApproved(userSignup, false)

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

	t.Run("test propagation policy", func(t *testing.T) {
		userSignup := commonsignup.NewUserSignup(
			commonsignup.CreatedBefore(threeYears),
			commonsignup.VerificationRequired(days(8)),
		)

		r, req, fakeClient := prepareReconcile(t, userSignup.Name, userSignup)
		deleted := false
		fakeClient.MockDelete = func(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
			deleted = true
			require.Len(t, opts, 1)
			deleteOptions, ok := opts[0].(*client.DeleteOptions)
			require.True(t, ok)
			require.NotNil(t, deleteOptions)
			require.NotNil(t, deleteOptions.PropagationPolicy)
			assert.Equal(t, v1.DeletePropagationForeground, *deleteOptions.PropagationPolicy)
			return nil
		}
		_, err := r.Reconcile(context.TODO(), req)
		require.NoError(t, err)
		assert.True(t, deleted)
	})

}

func expectRequeue(t *testing.T, res reconcile.Result, margin int) {
	// We expect the requeue duration to be approximately equal to the default retention time of 730 days. Let's
	// accept any value here between the range of 364 days and 366 days
	durLower := time.Duration(days(730 - 1 - margin))
	durUpper := time.Duration(days(730 + 1 - margin))

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
