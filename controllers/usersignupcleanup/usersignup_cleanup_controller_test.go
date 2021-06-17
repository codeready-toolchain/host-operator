package usersignupcleanup

import (
	"context"
	"fmt"
	"testing"
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	"github.com/codeready-toolchain/host-operator/pkg/configuration"
	"github.com/codeready-toolchain/host-operator/pkg/metrics"
	test2 "github.com/codeready-toolchain/host-operator/test"
	"github.com/codeready-toolchain/toolchain-common/pkg/states"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"

	promtestutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestUserCleanup(t *testing.T) {
	// A creation time three years in the past
	threeYears := time.Duration(time.Hour * 24 * 365 * 3)

	t.Run("test that user cleanup doesn't delete an active UserSignup", func(t *testing.T) {

		userSignup := test2.NewUserSignup(
			test2.CreatedBefore(threeYears),
			test2.WithStateLabel(toolchainv1alpha1.UserSignupStateLabelValueApproved),
			test2.SignupComplete(""),
			test2.ApprovedAutomatically(threeYears),
		)

		r, req, _ := prepareReconcile(t, userSignup.Name, userSignup)

		_, err := r.Reconcile(req)
		require.NoError(t, err)

		// Confirm the UserSignup still exists
		key := test.NamespacedName(test.HostOperatorNs, userSignup.Name)
		require.NoError(t, r.Client.Get(context.Background(), key, userSignup))
		require.NotNil(t, userSignup)
	})

	t.Run("test that user cleanup doesn't delete a recently deactivated UserSignup", func(t *testing.T) {

		userSignup := test2.NewUserSignup(
			test2.ApprovedAutomatically(threeYears),
			test2.WithStateLabel(toolchainv1alpha1.UserSignupStateLabelValueApproved),
			test2.DeactivatedWithLastTransitionTime(time.Duration(5*time.Minute)),
			test2.CreatedBefore(threeYears),
		)

		r, req, _ := prepareReconcile(t, userSignup.Name, userSignup)

		res, err := r.Reconcile(req)
		require.NoError(t, err)

		// Confirm the UserSignup still exists
		key := test.NamespacedName(test.HostOperatorNs, userSignup.Name)
		require.NoError(t, r.Client.Get(context.Background(), key, userSignup))
		require.NotNil(t, userSignup)

		expectRequeue(t, res, 0)
	})

	t.Run("test that an old, deactivated UserSignup is deleted", func(t *testing.T) {

		userSignup := test2.NewUserSignup(
			test2.WithStateLabel(toolchainv1alpha1.UserSignupStateLabelValueApproved),
			test2.ApprovedAutomatically(threeYears),
			test2.DeactivatedWithLastTransitionTime(threeYears),
			test2.CreatedBefore(threeYears),
		)

		r, req, _ := prepareReconcile(t, userSignup.Name, userSignup)

		_, err := r.Reconcile(req)
		require.NoError(t, err)

		// Confirm the UserSignup has been deleted
		key := test.NamespacedName(test.HostOperatorNs, userSignup.Name)
		err = r.Client.Get(context.Background(), key, userSignup)
		require.Error(t, err)
		require.True(t, errors.IsNotFound(err))
		statusErr := err.(*errors.StatusError)
		require.Equal(t, fmt.Sprintf("usersignups.toolchain.dev.openshift.com \"%s\" not found", key.Name), statusErr.Error())
	})

	t.Run("test that an old, unverified UserSignup is deleted", func(t *testing.T) {

		t.Run("without phone verification initiated", func(t *testing.T) {
			// given
			userSignup := test2.NewUserSignup(
				test2.CreatedBefore(days(8)),
				test2.VerificationRequired(days(8)),
			)
			r, req, _ := prepareReconcile(t, userSignup.Name, userSignup)
			// when
			_, err := r.Reconcile(req)
			require.NoError(t, err)
			// then
			// confirm the UserSignup has been deleted
			key := test.NamespacedName(test.HostOperatorNs, userSignup.Name)
			err = r.Client.Get(context.Background(), key, userSignup)
			require.True(t, errors.IsNotFound(err))
			assert.Errorf(t, err, "usersignups.toolchain.dev.openshift.com \"%s\" not found", key.Name)
			// and verify the metrics
			assert.Equal(t, float64(0), promtestutil.ToFloat64(metrics.UserSignupDeletedWithInitiatingVerificationTotal))    // unchanged
			assert.Equal(t, float64(1), promtestutil.ToFloat64(metrics.UserSignupDeletedWithoutInitiatingVerificationTotal)) // incremented
		})

		t.Run("with phone verification initiated", func(t *testing.T) {
			// given
			userSignup := test2.NewUserSignup(
				test2.CreatedBefore(days(8)),
				test2.VerificationRequired(days(8)),
				test2.WithAnnotation(toolchainv1alpha1.UserSignupVerificationCodeAnnotationKey, "12345"),
			)
			r, req, _ := prepareReconcile(t, userSignup.Name, userSignup)
			// when
			_, err := r.Reconcile(req)
			require.NoError(t, err)
			// then
			// confirm the UserSignup has been deleted
			key := test.NamespacedName(test.HostOperatorNs, userSignup.Name)
			err = r.Client.Get(context.Background(), key, userSignup)
			require.True(t, errors.IsNotFound(err))
			assert.Errorf(t, err, "usersignups.toolchain.dev.openshift.com \"%s\" not found", key.Name)
			// and verify the metrics
			assert.Equal(t, float64(1), promtestutil.ToFloat64(metrics.UserSignupDeletedWithInitiatingVerificationTotal))    // incremented
			assert.Equal(t, float64(0), promtestutil.ToFloat64(metrics.UserSignupDeletedWithoutInitiatingVerificationTotal)) // unchanged
		})
	})

	t.Run("test that recently reactivated, unverified UserSignup is NOT deleted", func(t *testing.T) {

		userSignup := test2.NewUserSignup(
			test2.CreatedBefore(threeYears),
			test2.ApprovedAutomatically(days(40)),
			test2.VerificationRequired(days(10)),
		)

		r, req, _ := prepareReconcile(t, userSignup.Name, userSignup)

		res, err := r.Reconcile(req)
		require.NoError(t, err)

		// Confirm the UserSignup has not been deleted
		key := test.NamespacedName(test.HostOperatorNs, userSignup.Name)
		err = r.Client.Get(context.Background(), key, userSignup)
		require.NoError(t, err)
		require.NotNil(t, userSignup)
		expectRequeue(t, res, 10)
	})

	t.Run("test that reactivated, unverified UserSignup long time ago is deleted", func(t *testing.T) {

		userSignup := test2.NewUserSignup(
			test2.CreatedBefore(threeYears),
			test2.ApprovedAutomatically(days(396)),
			test2.VerificationRequired(days(366)),
		)

		r, req, _ := prepareReconcile(t, userSignup.Name, userSignup)

		_, err := r.Reconcile(req)
		require.NoError(t, err)

		// Confirm the UserSignup has been deleted
		key := test.NamespacedName(test.HostOperatorNs, userSignup.Name)
		err = r.Client.Get(context.Background(), key, userSignup)
		require.Error(t, err)
		require.True(t, errors.IsNotFound(err))
	})

	t.Run("test that an old, verified but unapproved UserSignup is not deleted", func(t *testing.T) {

		userSignup := test2.NewUserSignup(
			test2.CreatedBefore(threeYears),
		)
		states.SetVerificationRequired(userSignup, false)
		states.SetApproved(userSignup, false)

		r, req, _ := prepareReconcile(t, userSignup.Name, userSignup)

		res, err := r.Reconcile(req)
		require.NoError(t, err)

		// Confirm the UserSignup has not been deleted
		key := test.NamespacedName(test.HostOperatorNs, userSignup.Name)
		err = r.Client.Get(context.Background(), key, userSignup)
		require.NoError(t, err)
		require.NotNil(t, userSignup)
		require.False(t, res.Requeue)
	})

	t.Run("test propagation policy", func(t *testing.T) {
		userSignup := test2.NewUserSignup(
			test2.CreatedBefore(threeYears),
			test2.VerificationRequired(days(8)),
		)

		r, req, fakeClient := prepareReconcile(t, userSignup.Name, userSignup)
		deleted := false
		fakeClient.MockDelete = func(ctx context.Context, obj runtime.Object, opts ...client.DeleteOption) error {
			deleted = true
			require.Len(t, opts, 1)
			deleteOptions, ok := opts[0].(*client.DeleteOptions)
			require.True(t, ok)
			require.NotNil(t, deleteOptions)
			require.NotNil(t, deleteOptions.PropagationPolicy)
			assert.Equal(t, v1.DeletePropagationForeground, *deleteOptions.PropagationPolicy)
			return nil
		}
		_, err := r.Reconcile(req)
		require.NoError(t, err)
		assert.True(t, deleted)
	})

}

func expectRequeue(t *testing.T, res reconcile.Result, minusDays int) {
	// We expect the requeue duration to be approximately equal to the default retention time of 365 days. Let's
	// accept any value here between the range of 364 days and 366 days
	durLower := time.Duration(days(364 - minusDays))
	durUpper := time.Duration(days(366 - minusDays))

	require.True(t, res.Requeue)
	require.Greater(t, res.RequeueAfter, durLower)
	require.Less(t, res.RequeueAfter, durUpper)
}

func days(days int) time.Duration {
	return time.Hour * time.Duration(days*24)
}

func prepareReconcile(t *testing.T, name string, initObjs ...runtime.Object) (*Reconciler, reconcile.Request, *test.FakeClient) { // nolint: unparam
	metrics.Reset()

	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)

	fakeClient := test.NewFakeClient(t, initObjs...)
	config, err := configuration.LoadConfig(fakeClient)
	require.NoError(t, err)

	r := &Reconciler{
		Scheme: s,
		Config: config,
		Client: fakeClient,
		Log:    ctrl.Log.WithName("controllers").WithName("UserSignupCleanup"),
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
