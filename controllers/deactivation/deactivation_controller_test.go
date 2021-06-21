package deactivation

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/controllers/toolchainconfig"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	"github.com/codeready-toolchain/host-operator/pkg/configuration"
	"github.com/codeready-toolchain/host-operator/pkg/metrics"
	. "github.com/codeready-toolchain/host-operator/test"
	tiertest "github.com/codeready-toolchain/host-operator/test/nstemplatetier"
	"github.com/codeready-toolchain/toolchain-common/pkg/states"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	testconfig "github.com/codeready-toolchain/toolchain-common/pkg/test/config"
	murtest "github.com/codeready-toolchain/toolchain-common/pkg/test/masteruserrecord"

	uuid "github.com/satori/go.uuid"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	operatorNamespace = "toolchain-host-operator"

	expectedDeactivationTimeoutBasicTier = 30
	expectedDeactivationTimeoutOtherTier = 60

	preDeactivationNotificationDays = 3
)

func TestReconcile(t *testing.T) {
	config := newToolchainConfigWithReset(t, testconfig.AutomaticApproval().MaxUsersNumber(123,
		testconfig.PerMemberCluster("member1", 321)),
		testconfig.Deactivation().DeactivatingNotificationDays(3))

	// given
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	username := "test-user"

	basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates)
	otherTier := tiertest.OtherTier()
	noDeactivationTier := tiertest.TierWithoutDeactivationTimeout()

	userSignupFoobar := userSignupWithEmail(username, "foo@bar.com")
	userSignupRedhat := userSignupWithEmail(username, "foo@redhat.com")

	// Set the deactivating status
	states.SetDeactivating(userSignupFoobar, true)

	t.Run("controller should not deactivate user", func(t *testing.T) {

		// the time since the mur was provisioned is within the deactivation timeout period for the 'basic' tier
		t.Run("usersignup should not be deactivated - basic tier (30 days)", func(t *testing.T) {
			// given
			murProvisionedTime := metav1.Now()
			mur := murtest.NewMasterUserRecord(t, username, murtest.Account("cluster1", *basicTier), murtest.ProvisionedMur(&murProvisionedTime), murtest.UserIDFromUserSignup(userSignupFoobar))
			mur.Labels[toolchainv1alpha1.MasterUserRecordOwnerLabelKey] = userSignupFoobar.Name
			r, req, cl := prepareReconcile(t, mur.Name, basicTier, mur, userSignupFoobar, config)
			// when
			timeSinceProvisioned := time.Since(murProvisionedTime.Time)
			res, err := r.Reconcile(req)
			// then
			require.NoError(t, err)
			expectedTime := (time.Duration((expectedDeactivationTimeoutBasicTier-preDeactivationNotificationDays)*24) * time.Hour) - timeSinceProvisioned
			actualTime := res.RequeueAfter
			diff := expectedTime - actualTime
			require.Truef(t, diff > 0 && diff < 2*time.Second, "expectedTime: '%v' is not within 2 seconds of actualTime: '%v' diff: '%v'", expectedTime, actualTime, diff)
			assertThatUserSignupDeactivated(t, cl, username, false)
		})

		// the time since the mur was provisioned is within the deactivation timeout period for the 'other' tier
		t.Run("usersignup should not be deactivated - other tier (60 days)", func(t *testing.T) {
			// given
			murProvisionedTime := metav1.Now()
			mur := murtest.NewMasterUserRecord(t, username, murtest.Account("cluster1", *otherTier), murtest.ProvisionedMur(&murProvisionedTime), murtest.UserIDFromUserSignup(userSignupFoobar))
			mur.Labels[toolchainv1alpha1.MasterUserRecordOwnerLabelKey] = userSignupFoobar.Name
			r, req, cl := prepareReconcile(t, mur.Name, otherTier, mur, userSignupFoobar, config)
			// when
			timeSinceProvisioned := time.Since(murProvisionedTime.Time)
			res, err := r.Reconcile(req)
			// then
			require.NoError(t, err)
			expectedTime := (time.Duration((expectedDeactivationTimeoutOtherTier-preDeactivationNotificationDays)*24) * time.Hour) - timeSinceProvisioned
			actualTime := res.RequeueAfter
			diff := expectedTime - actualTime
			require.Truef(t, diff > 0 && diff < 2*time.Second, "expectedTime: '%v' is not within 2 seconds of actualTime: '%v' diff: '%v'", expectedTime, actualTime, diff)
			assertThatUserSignupDeactivated(t, cl, username, false)
		})

		// the tier does not have a deactivationTimeoutDays set so the time since the mur was provisioned is irrelevant, the user should not be deactivated
		t.Run("usersignup should not be deactivated - no deactivation tier", func(t *testing.T) {
			// given
			murProvisionedTime := metav1.Now()
			mur := murtest.NewMasterUserRecord(t, username, murtest.Account("cluster1", *noDeactivationTier), murtest.ProvisionedMur(&murProvisionedTime), murtest.UserIDFromUserSignup(userSignupFoobar))
			mur.Labels[toolchainv1alpha1.MasterUserRecordOwnerLabelKey] = userSignupFoobar.Name
			r, req, cl := prepareReconcile(t, mur.Name, noDeactivationTier, mur, userSignupFoobar, config)
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
			r, req, cl := prepareReconcile(t, mur.Name, basicTier, mur, userSignupFoobar, config)
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
			mur.Labels[toolchainv1alpha1.MasterUserRecordOwnerLabelKey] = userSignupRedhat.Name
			r, req, cl := prepareReconcile(t, mur.Name, basicTier, mur, userSignupRedhat, config)
			// when
			res, err := r.Reconcile(req)
			// then
			require.NoError(t, err)
			require.False(t, res.Requeue, "requeue should not be set")
			require.True(t, res.RequeueAfter == 0, "requeueAfter should not be set")
			assertThatUserSignupDeactivated(t, cl, username, false)
		})
	})
	// in these tests, the controller should (eventually) deactivate the user
	t.Run("controller should deactivate user", func(t *testing.T) {
		t.Run("usersignup should be marked as deactivating - basic tier (30 days)", func(t *testing.T) {
			// given
			states.SetDeactivating(userSignupFoobar, false)

			// Set the provisioned time so that we are now 2 days before the expected deactivation time
			murProvisionedTime := &metav1.Time{Time: time.Now().Add(-time.Duration((expectedDeactivationTimeoutBasicTier-2)*24) * time.Hour)}
			mur := murtest.NewMasterUserRecord(t, username, murtest.Account("cluster1", *basicTier),
				murtest.ProvisionedMur(murProvisionedTime), murtest.UserIDFromUserSignup(userSignupFoobar))
			mur.Labels[toolchainv1alpha1.MasterUserRecordOwnerLabelKey] = userSignupFoobar.Name

			r, req, cl := prepareReconcile(t, mur.Name, basicTier, mur, userSignupFoobar, config)
			// when
			res, err := r.Reconcile(req)
			// then
			require.NoError(t, err)
			require.True(t, res.Requeue)
			require.Equal(t, time.Duration(10)*time.Second, res.RequeueAfter)

			// Reload the userSignup
			require.NoError(t, cl.Get(context.TODO(), types.NamespacedName{Name: userSignupFoobar.Name, Namespace: operatorNamespace}, userSignupFoobar))
			require.True(t, states.Deactivating(userSignupFoobar))
			require.False(t, states.Deactivated(userSignupFoobar))

			t.Run("reconciliation should be requeued when notification not yet sent", func(t *testing.T) {
				r, req, cl := prepareReconcile(t, mur.Name, basicTier, mur, userSignupFoobar, config)
				// when
				res, err := r.Reconcile(req)
				// then
				require.NoError(t, err)
				require.True(t, res.Requeue)
				require.Equal(t, time.Duration(10)*time.Second, res.RequeueAfter)

				// Reload the userSignup
				require.NoError(t, cl.Get(context.TODO(), types.NamespacedName{Name: userSignupFoobar.Name, Namespace: operatorNamespace}, userSignupFoobar))

				// deactivating state should still be true
				require.True(t, states.Deactivating(userSignupFoobar))

				// deactivated state should still be false
				require.False(t, states.Deactivated(userSignupFoobar))

				t.Run("usersignup requeued after deactivating notification created for user", func(t *testing.T) {
					// Set the notification status condition as sent
					userSignupFoobar.Status.Conditions = []toolchainv1alpha1.Condition{
						{
							Type:               toolchainv1alpha1.UserSignupUserDeactivatingNotificationCreated,
							Status:             corev1.ConditionTrue,
							LastTransitionTime: metav1.Time{Time: time.Now()},
							Reason:             toolchainv1alpha1.UserSignupDeactivatingNotificationCRCreatedReason,
						},
					}

					r, req, cl := prepareReconcile(t, mur.Name, basicTier, mur, userSignupFoobar, config)

					// when
					res, err := r.Reconcile(req)
					// then
					require.NoError(t, err)
					require.False(t, res.Requeue)
					// The RequeueAfter should be ~about 3 days... let's accept if it's within 1 hour of that
					require.WithinDuration(t, time.Now().Add(time.Duration(72)*time.Hour), time.Now().Add(res.RequeueAfter), time.Duration(1)*time.Hour)

					// Reload the userSignup
					require.NoError(t, cl.Get(context.TODO(), types.NamespacedName{Name: userSignupFoobar.Name, Namespace: operatorNamespace}, userSignupFoobar))

					// deactivating state should still be true
					require.True(t, states.Deactivating(userSignupFoobar))

					// deactivated state should still be false
					require.False(t, states.Deactivated(userSignupFoobar))

					t.Run("usersignup should be deactivated", func(t *testing.T) {
						// Set the notification status condition as sent, but this time 3 days in the past
						userSignupFoobar.Status.Conditions = []toolchainv1alpha1.Condition{
							{
								Type:               toolchainv1alpha1.UserSignupUserDeactivatingNotificationCreated,
								Status:             corev1.ConditionTrue,
								LastTransitionTime: metav1.Time{Time: time.Now().Add(time.Duration(-3) * time.Hour * 24)},
								Reason:             toolchainv1alpha1.UserSignupDeactivatingNotificationCRCreatedReason,
							},
						}

						r, req, cl := prepareReconcile(t, mur.Name, basicTier, mur, userSignupFoobar, config)

						// when
						res, err := r.Reconcile(req)
						// then
						require.NoError(t, err)
						require.False(t, res.Requeue)

						// Reload the userSignup
						require.NoError(t, cl.Get(context.TODO(), types.NamespacedName{Name: userSignupFoobar.Name, Namespace: operatorNamespace}, userSignupFoobar))

						// deactivating state should now be false
						require.False(t, states.Deactivating(userSignupFoobar))

						// deactivated state should now be true
						require.True(t, states.Deactivated(userSignupFoobar))

						t.Run("usersignup already deactivated", func(t *testing.T) {
							// additional reconciles should find the usersignup is already deactivated
							res, err := r.Reconcile(req)
							// then
							require.NoError(t, err)
							require.False(t, res.Requeue, "requeue should not be set")
							require.True(t, res.RequeueAfter == 0, "requeueAfter should not be set")
						})
					})
				})
			})
		})

		// the time since the mur was provisioned exceeds the deactivation timeout period for the 'other' tier
		t.Run("usersignup should be deactivated - other tier (60 days)", func(t *testing.T) {
			states.SetDeactivating(userSignupFoobar, true)
			// given
			murProvisionedTime := &metav1.Time{Time: time.Now().Add(-time.Duration(expectedDeactivationTimeoutOtherTier*24) * time.Hour)}
			mur := murtest.NewMasterUserRecord(t, username, murtest.Account("cluster1", *otherTier), murtest.ProvisionedMur(murProvisionedTime), murtest.UserIDFromUserSignup(userSignupFoobar))
			mur.Labels[toolchainv1alpha1.MasterUserRecordOwnerLabelKey] = userSignupFoobar.Name
			r, req, cl := prepareReconcile(t, mur.Name, otherTier, mur, userSignupFoobar, config)
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
			mur.Labels[toolchainv1alpha1.MasterUserRecordOwnerLabelKey] = userSignupFoobar.Name
			r, req, cl := prepareReconcile(t, mur.Name, mur, userSignupFoobar, config)
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
			mur.Labels[toolchainv1alpha1.MasterUserRecordOwnerLabelKey] = userSignupFoobar.Name
			r, req, cl := prepareReconcile(t, mur.Name, basicTier, mur, userSignupFoobar, config)
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
			mur.Labels[toolchainv1alpha1.MasterUserRecordOwnerLabelKey] = userSignupFoobar.Name
			r, req, cl := prepareReconcile(t, mur.Name, basicTier, mur, userSignupFoobar, config)
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
	metrics.Reset()
	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)
	cl := test.NewFakeClient(t, initObjs...)
	cfg, err := configuration.LoadConfig(cl)
	require.NoError(t, err)
	r := &Reconciler{
		Client: cl,
		Scheme: s,
		Config: cfg,
		Log:    ctrl.Log.WithName("controllers").WithName("Deactivation"),
	}
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
	us := &toolchainv1alpha1.UserSignup{
		ObjectMeta: newObjectMeta(username, email),
		Spec: toolchainv1alpha1.UserSignupSpec{
			Username:      email,
			TargetCluster: "east",
			Userid:        username,
		},
	}
	states.SetApproved(us, true)

	return us
}

func assertThatUserSignupDeactivated(t *testing.T, cl *test.FakeClient, name string, expected bool) {
	userSignup := &toolchainv1alpha1.UserSignup{}
	err := cl.Get(context.TODO(), types.NamespacedName{Name: name, Namespace: operatorNamespace}, userSignup)
	require.NoError(t, err)
	require.Equal(t, expected, states.Deactivated(userSignup))
}

func newToolchainConfigWithReset(t *testing.T, options ...testconfig.ToolchainConfigOption) *toolchainv1alpha1.ToolchainConfig {
	t.Cleanup(toolchainconfig.Reset)
	return testconfig.NewToolchainConfig(options...)
}
