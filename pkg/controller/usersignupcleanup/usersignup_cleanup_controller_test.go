package usersignupcleanup

import (
	"context"
	"fmt"
	"github.com/redhat-cop/operator-utils/pkg/util"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"testing"
	"time"

	"github.com/codeready-toolchain/toolchain-common/pkg/states"

	"github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	"github.com/codeready-toolchain/host-operator/pkg/configuration"
	"github.com/codeready-toolchain/host-operator/pkg/metrics"
	test2 "github.com/codeready-toolchain/host-operator/test"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
)

func TestUserCleanup(t *testing.T) {
	// A creation time three years in the past
	threeYears := time.Duration(time.Hour * 24 * 365 * 3)

	t.Run("test that user cleanup doesn't delete an active UserSignup", func(t *testing.T) {

		userSignup := test2.NewUserSignup(
			test2.CreatedBefore(threeYears),
			test2.WithStateLabel(v1alpha1.UserSignupStateLabelValueApproved),
			test2.SignupComplete(""),
			test2.ApprovedAutomatically(),
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
			test2.DeactivatedWithLastTransitionTime(time.Duration(5*time.Minute)),
			test2.CreatedBefore(threeYears),
			test2.WithStateLabel(v1alpha1.UserSignupStateLabelValueApproved),
			test2.ApprovedAutomatically(),
		)

		r, req, _ := prepareReconcile(t, userSignup.Name, userSignup)

		res, err := r.Reconcile(req)
		require.NoError(t, err)

		// Confirm the UserSignup still exists
		key := test.NamespacedName(test.HostOperatorNs, userSignup.Name)
		require.NoError(t, r.Client.Get(context.Background(), key, userSignup))
		require.NotNil(t, userSignup)
		require.True(t, res.Requeue)

		// We expect the requeue duration to be approximately equal to the default retention time of 180 days. Let's
		// accept any value here between the range of 179 days and 181 days
		durLower := time.Duration(179 * time.Hour * 24)
		durUpper := time.Duration(181 * time.Hour * 24)

		require.Greater(t, res.RequeueAfter, durLower)
		require.Less(t, res.RequeueAfter, durUpper)
	})

	t.Run("test that an old, deactivated UserSignup is deleted", func(t *testing.T) {

		userSignup := test2.NewUserSignup(
			test2.DeactivatedWithLastTransitionTime(threeYears),
			test2.CreatedBefore(threeYears),
			test2.WithStateLabel(v1alpha1.UserSignupStateLabelValueApproved),
			test2.ApprovedAutomatically(),
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

		userSignup := test2.NewUserSignup(
			test2.CreatedBefore(threeYears),
			test2.VerificationRequired(),
		)

		r, req, _ := prepareReconcile(t, userSignup.Name, userSignup)

		_, err := r.Reconcile(req)
		require.NoError(t, err)

		// Confirm the UserSignup has been deleted
		key := test.NamespacedName(test.HostOperatorNs, userSignup.Name)
		err = r.Client.Get(context.Background(), key, userSignup)
		require.Error(t, err)
		require.True(t, errors.IsNotFound(err))
		require.IsType(t, &errors.StatusError{}, err)
		statusErr := err.(*errors.StatusError)
		require.Equal(t, fmt.Sprintf("usersignups.toolchain.dev.openshift.com \"%s\" not found", key.Name), statusErr.Error())
	})

	t.Run("test that an old, verified but unapproved UserSignup is not deleted", func(t *testing.T) {

		userSignup := test2.NewUserSignup(
			test2.CreatedBefore(threeYears),
		)
		states.SetVerificationRequired(userSignup, false)
		userSignup.Spec.Approved = false

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
			test2.VerificationRequired(),
		)

		r, req, fakeClient := prepareReconcile(t, userSignup.Name, userSignup)

		fakeClient.MockDelete = func(ctx context.Context, obj runtime.Object, opts ...client.DeleteOption) error {

			for _, opt := range(opts){
				val := reflect.ValueOf(opt).Elem()
				deletionPropagation := val.FieldByName("PropagationPolicy").Interface().(*v1.DeletionPropagation)
				if *deletionPropagation == "Foreground" {
					if obj, ok := obj.(*v1alpha1.UserSignup); ok {
						util.AddFinalizer(obj, "foregroundDeletion")
						if err := r.Client.Update(context.TODO(), obj); err != nil {
							return err
						}
						return nil
					}
					return fmt.Errorf("object is not of type userSignup")
				}
			}
			return fmt.Errorf("Deletion Propagation policy is not of type Foreground.")
		}

		_, err := r.Reconcile(req)
		require.NoError(t, err)

		// Confirm the UserSignup has finalizer set
		key := test.NamespacedName(test.HostOperatorNs, userSignup.Name)
		require.NoError(t, r.Client.Get(context.Background(), key, userSignup))
		require.Equal(t, userSignup.Finalizers[0], "foregroundDeletion")

		// now remove finalizer, reset MockDelete to nil and call reconcile
		util.RemoveFinalizer(userSignup, "foregroundDeletion")
		require.NoError(t, r.Client.Update(context.TODO(), userSignup))
		fakeClient.MockDelete = nil

		_, err = r.Reconcile(req)
		require.NoError(t, err)

		// now usersignup should be deleted
		require.Error(t, r.Client.Get(context.Background(), key, userSignup))
	})

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
