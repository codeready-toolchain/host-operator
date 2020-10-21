package deactivation

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	"github.com/codeready-toolchain/host-operator/pkg/configuration"
	"github.com/codeready-toolchain/host-operator/pkg/metrics"
	. "github.com/codeready-toolchain/host-operator/test"
	tiertest "github.com/codeready-toolchain/host-operator/test/nstemplatetier"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	murtest "github.com/codeready-toolchain/toolchain-common/pkg/test/masteruserrecord"
	uuid "github.com/satori/go.uuid"
	"github.com/stretchr/testify/require"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	operatorNamespace = "toolchain-host-operator"

	expectedDeactivationTimeoutBasicTier = 30
	expectedDeactivationTimeoutOtherTier = 60
)

func TestReconcile(t *testing.T) {

	// given
	logf.SetLogger(zap.Logger(true))
	username := "test-user"

	basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates)
	otherTier := tiertest.OtherTier()
	noDeactivationTier := tiertest.TierWithoutDeactivationTimeout()

	userSignupFoobar := userSignupWithEmail(username, "foo@bar.com")
	userSignupRedhat := userSignupWithEmail(username, "foo@redhat.com")

	t.Run("controller should not deactivate user", func(t *testing.T) {

		// the time since the mur was provisioned is within the deactivation timeout period for the 'basic' tier
		t.Run("usersignup should not be deactivated - basic tier (30 days)", func(t *testing.T) {
			// given
			murProvisionedTime := metav1.Now()
			mur := murtest.NewMasterUserRecord(t, username, murtest.Account("cluster1", *basicTier), murtest.ProvisionedMur(&murProvisionedTime), murtest.UserIDFromUserSignup(userSignupFoobar))
			r, req, cl := prepareReconcile(t, mur.Name, basicTier, mur, userSignupFoobar)
			// when
			timeSinceProvisioned := time.Since(murProvisionedTime.Time)
			res, err := r.Reconcile(req)
			// then
			require.NoError(t, err)
			expectedTime := (time.Duration(expectedDeactivationTimeoutBasicTier*24) * time.Hour) - timeSinceProvisioned
			actualTime := res.RequeueAfter
			diff := expectedTime - actualTime
			require.Truef(t, diff > 0 && diff < time.Second, "expectedTime: '%v' is not within 1 second of actualTime: '%v' diff: '%v'", expectedTime, actualTime, diff)
			assertThatUserSignupDeactivated(t, cl, username, false)
		})

		// the time since the mur was provisioned is within the deactivation timeout period for the 'other' tier
		t.Run("usersignup should not be deactivated - other tier (60 days)", func(t *testing.T) {
			// given
			murProvisionedTime := metav1.Now()
			mur := murtest.NewMasterUserRecord(t, username, murtest.Account("cluster1", *otherTier), murtest.ProvisionedMur(&murProvisionedTime), murtest.UserIDFromUserSignup(userSignupFoobar))
			r, req, cl := prepareReconcile(t, mur.Name, otherTier, mur, userSignupFoobar)
			// when
			timeSinceProvisioned := time.Since(murProvisionedTime.Time)
			res, err := r.Reconcile(req)
			// then
			require.NoError(t, err)
			expectedTime := (time.Duration(expectedDeactivationTimeoutOtherTier*24) * time.Hour) - timeSinceProvisioned
			actualTime := res.RequeueAfter
			diff := expectedTime - actualTime
			require.Truef(t, diff > 0 && diff < time.Second, "expectedTime: '%v' is not within 1 second of actualTime: '%v' diff: '%v'", expectedTime, actualTime, diff)
			assertThatUserSignupDeactivated(t, cl, username, false)
		})

		// the tier does not have a deactivationTimeoutDays set so the time since the mur was provisioned is irrelevant, the user should not be deactivated
		t.Run("usersignup should not be deactivated - no deactivation tier", func(t *testing.T) {
			// given
			murProvisionedTime := metav1.Now()
			mur := murtest.NewMasterUserRecord(t, username, murtest.Account("cluster1", *noDeactivationTier), murtest.ProvisionedMur(&murProvisionedTime), murtest.UserIDFromUserSignup(userSignupFoobar))
			r, req, cl := prepareReconcile(t, mur.Name, noDeactivationTier, mur, userSignupFoobar)
			// when
			res, err := r.Reconcile(req)
			// then
			require.NoError(t, err)
			require.False(t, res.Requeue, "requeue should not be set")
			require.True(t, res.RequeueAfter == 0, "requeueAfter should not be set")
			assertThatUserSignupDeactivated(t, cl, username, false)
		})

		// a mur that has not been provisioned yet
		t.Run("mur without provisioned time", func(t *testing.T) {
			// given
			mur := murtest.NewMasterUserRecord(t, username, murtest.Account("cluster1", *basicTier), murtest.UserIDFromUserSignup(userSignupFoobar))
			r, req, cl := prepareReconcile(t, mur.Name, basicTier, mur, userSignupFoobar)
			// when
			res, err := r.Reconcile(req)
			// then
			require.NoError(t, err)
			require.False(t, res.Requeue, "requeue should not be set")
			require.True(t, res.RequeueAfter == 0, "requeueAfter should not be set")
			assertThatUserSignupDeactivated(t, cl, username, false)
		})

		// a user that belongs to the deactivation domain excluded list
		t.Run("user deactivation excluded", func(t *testing.T) {
			// given
			restore := test.SetEnvVarAndRestore(t, "HOST_OPERATOR_DEACTIVATION_DOMAINS_EXCLUDED", "@redhat.com")
			defer restore()
			murProvisionedTime := &metav1.Time{Time: time.Now().Add(-time.Duration(expectedDeactivationTimeoutBasicTier*24) * time.Hour)}
			mur := murtest.NewMasterUserRecord(t, username, murtest.Account("cluster1", *basicTier), murtest.ProvisionedMur(murProvisionedTime), murtest.UserIDFromUserSignup(userSignupRedhat))
			r, req, cl := prepareReconcile(t, mur.Name, basicTier, mur, userSignupRedhat)
			// when
			res, err := r.Reconcile(req)
			// then
			require.NoError(t, err)
			require.False(t, res.Requeue, "requeue should not be set")
			require.True(t, res.RequeueAfter == 0, "requeueAfter should not be set")
			assertThatUserSignupDeactivated(t, cl, username, false)
		})
	})

	// in these tests, the controller should deactivate the user
	t.Run("controller should deactivate user", func(t *testing.T) {
		// the time since the mur was provisioned exceeds the deactivation timeout period for the 'basic' tier
		t.Run("usersignup should be deactivated - basic tier (30 days)", func(t *testing.T) {
			// given
			defer metrics.Reset()
			murProvisionedTime := &metav1.Time{Time: time.Now().Add(-time.Duration(expectedDeactivationTimeoutBasicTier*24) * time.Hour)}
			mur := murtest.NewMasterUserRecord(t, username, murtest.Account("cluster1", *basicTier), murtest.ProvisionedMur(murProvisionedTime), murtest.UserIDFromUserSignup(userSignupFoobar))
			r, req, cl := prepareReconcile(t, mur.Name, basicTier, mur, userSignupFoobar)
			// when
			res, err := r.Reconcile(req)
			// then
			require.NoError(t, err)
			require.False(t, res.Requeue, "requeue should not be set")
			require.True(t, res.RequeueAfter == 0, "requeueAfter should not be set")
			assertThatUserSignupDeactivated(t, cl, username, true)
			AssertMetricsCounterEquals(t, 1, metrics.UserSignupAutoDeactivatedTotal)
		})

		// the time since the mur was provisioned exceeds the deactivation timeout period for the 'other' tier
		t.Run("usersignup should be deactivated - other tier (60 days)", func(t *testing.T) {
			// given
			defer metrics.Reset()
			murProvisionedTime := &metav1.Time{Time: time.Now().Add(-time.Duration(expectedDeactivationTimeoutOtherTier*24) * time.Hour)}
			mur := murtest.NewMasterUserRecord(t, username, murtest.Account("cluster1", *otherTier), murtest.ProvisionedMur(murProvisionedTime), murtest.UserIDFromUserSignup(userSignupFoobar))
			r, req, cl := prepareReconcile(t, mur.Name, otherTier, mur, userSignupFoobar)
			// when
			res, err := r.Reconcile(req)
			// then
			require.NoError(t, err)
			require.False(t, res.Requeue, "requeue should not be set")
			require.True(t, res.RequeueAfter == 0, "requeue should not be set")
			assertThatUserSignupDeactivated(t, cl, username, true)
			AssertMetricsCounterEquals(t, 1, metrics.UserSignupAutoDeactivatedTotal)
		})

	})

	t.Run("failures", func(t *testing.T) {

		// cannot find NSTemplateTier
		t.Run("unable to get NSTemplateTier", func(t *testing.T) {
			// given
			murProvisionedTime := metav1.Now()
			mur := murtest.NewMasterUserRecord(t, username, murtest.Account("cluster1", *basicTier), murtest.ProvisionedMur(&murProvisionedTime), murtest.UserIDFromUserSignup(userSignupFoobar))
			r, req, cl := prepareReconcile(t, mur.Name, mur, userSignupFoobar)
			// when
			_, err := r.Reconcile(req)
			// then
			require.Error(t, err)
			require.Contains(t, err.Error(), `nstemplatetiers.toolchain.dev.openshift.com "basic" not found`)
			assertThatUserSignupDeactivated(t, cl, username, false)
		})

		// cannot get UserSignup
		t.Run("UserSignup get failure", func(t *testing.T) {
			// given
			murProvisionedTime := &metav1.Time{Time: time.Now().Add(-time.Duration(expectedDeactivationTimeoutBasicTier*24) * time.Hour)}
			mur := murtest.NewMasterUserRecord(t, username, murtest.Account("cluster1", *basicTier), murtest.ProvisionedMur(murProvisionedTime), murtest.UserIDFromUserSignup(userSignupFoobar))
			r, req, cl := prepareReconcile(t, mur.Name, basicTier, mur, userSignupFoobar)
			cl.MockGet = func(ctx context.Context, key client.ObjectKey, obj runtime.Object) error {
				_, ok := obj.(*toolchainv1alpha1.UserSignup)
				if ok {
					return fmt.Errorf("usersignup get error")
				}
				rmur, ok := obj.(*toolchainv1alpha1.MasterUserRecord)
				if ok {
					mur.DeepCopyInto(rmur)
					return nil
				}
				return nil
			}
			// when
			res, err := r.Reconcile(req)
			// then
			require.Error(t, err)
			require.Contains(t, err.Error(), "usersignup get error")
			require.False(t, res.Requeue, "requeue should not be set")
			require.True(t, res.RequeueAfter == 0, "requeueAfter should not be set")
		})

		// cannot update UserSignup
		t.Run("UserSignup update failure", func(t *testing.T) {
			// given
			murProvisionedTime := &metav1.Time{Time: time.Now().Add(-time.Duration(expectedDeactivationTimeoutBasicTier*24) * time.Hour)}
			mur := murtest.NewMasterUserRecord(t, username, murtest.Account("cluster1", *basicTier), murtest.ProvisionedMur(murProvisionedTime), murtest.UserIDFromUserSignup(userSignupFoobar))
			r, req, cl := prepareReconcile(t, mur.Name, basicTier, mur, userSignupFoobar)
			cl.MockUpdate = func(ctx context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
				_, ok := obj.(*toolchainv1alpha1.UserSignup)
				if ok {
					return fmt.Errorf("usersignup update error")
				}
				return nil
			}
			// when
			res, err := r.Reconcile(req)
			// then
			require.Error(t, err)
			require.Contains(t, err.Error(), "usersignup update error")
			require.False(t, res.Requeue, "requeue should not be set")
			require.True(t, res.RequeueAfter == 0, "requeueAfter should not be set")
			assertThatUserSignupDeactivated(t, cl, username, false)
		})
	})

}

func prepareReconcile(t *testing.T, name string, initObjs ...runtime.Object) (reconcile.Reconciler, reconcile.Request, *test.FakeClient) {
	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)
	cl := test.NewFakeClient(t, initObjs...)
	cfg, err := configuration.LoadConfig(cl)
	require.NoError(t, err)
	r := &ReconcileDeactivation{client: cl, scheme: s, config: cfg}
	return r, reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      name,
			Namespace: operatorNamespace,
		},
	}, cl
}

func newObjectMeta(name, email string) metav1.ObjectMeta {
	if name == "" {
		name = uuid.NewV4().String()
	}

	md5hash := md5.New()
	// Ignore the error, as this implementation cannot return one
	_, _ = md5hash.Write([]byte(email))
	emailHash := hex.EncodeToString(md5hash.Sum(nil))

	return metav1.ObjectMeta{
		Name:      name,
		Namespace: operatorNamespace,
		Annotations: map[string]string{
			toolchainv1alpha1.UserSignupUserEmailAnnotationKey: email,
		},
		Labels: map[string]string{
			toolchainv1alpha1.UserSignupUserEmailHashLabelKey: emailHash,
		},
	}
}

func userSignupWithEmail(username, email string) *toolchainv1alpha1.UserSignup {
	return &toolchainv1alpha1.UserSignup{
		ObjectMeta: newObjectMeta(username, email),
		Spec: toolchainv1alpha1.UserSignupSpec{
			Username:      email,
			Approved:      true,
			TargetCluster: "east",
		},
	}
}

func assertThatUserSignupDeactivated(t *testing.T, cl *test.FakeClient, name string, expected bool) {
	userSignup := &toolchainv1alpha1.UserSignup{}
	err := cl.Get(context.TODO(), types.NamespacedName{Name: name, Namespace: operatorNamespace}, userSignup)
	require.NoError(t, err)
	require.Equal(t, expected, userSignup.Spec.Deactivated)
}
