package deactivation

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/gofrs/uuid"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	"github.com/codeready-toolchain/host-operator/pkg/metrics"
	. "github.com/codeready-toolchain/host-operator/test"
	testusertier "github.com/codeready-toolchain/host-operator/test/usertier"
	"github.com/codeready-toolchain/toolchain-common/pkg/condition"
	commonconfig "github.com/codeready-toolchain/toolchain-common/pkg/configuration"
	"github.com/codeready-toolchain/toolchain-common/pkg/hash"
	"github.com/codeready-toolchain/toolchain-common/pkg/states"
	commontest "github.com/codeready-toolchain/toolchain-common/pkg/test"
	testconfig "github.com/codeready-toolchain/toolchain-common/pkg/test/config"
	murtest "github.com/codeready-toolchain/toolchain-common/pkg/test/masteruserrecord"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	operatorNamespace = "toolchain-host-operator"

	// UserTiers
	expectedDeactivationTimeoutDeactivate30Tier = 30
	expectedDeactivationTimeoutDeactivate90Tier = 90

	preDeactivationNotificationDays = 3
)

func TestReconcile(t *testing.T) {
	config := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.Deactivation().DeactivatingNotificationDays(3))

	// given
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	username := "test-user"

	// UserTiers
	userTier30 := testusertier.NewUserTier("deactivate30", 30)
	userTier90 := testusertier.NewUserTier("deactivate90", 90)
	userTierNoDeactivation := testusertier.NewUserTier("nodeactivation", 0)

	userSignupFoobar := userSignupWithEmail(username, "foo@bar.com")
	userSignupRedhat := userSignupWithEmail(username, "foo@redhat.com")

	// Set the deactivating status
	states.SetDeactivating(userSignupFoobar, true)

	t.Run("controller should not deactivate user", func(t *testing.T) {
		// the time since the mur was provisioned is within the deactivation timeout period for the 'deactivate30' tier
		t.Run("usersignup should not be deactivated - deactivate30 (30 days)", func(t *testing.T) {
			// given
			murProvisionedTime := metav1.Now()
			mur := murtest.NewMasterUserRecord(t, username, murtest.TierName(userTier30.Name), murtest.Account("cluster1"), murtest.ProvisionedMur(&murProvisionedTime), murtest.UserIDFromUserSignup(userSignupFoobar))
			mur.Labels[toolchainv1alpha1.MasterUserRecordOwnerLabelKey] = userSignupFoobar.Name
			r, req, cl := prepareReconcile(t, mur.Name, userTier30, mur, userSignupFoobar, config)
			// when
			timeSinceProvisioned := time.Since(murProvisionedTime.Time)
			res, err := r.Reconcile(context.TODO(), req)
			// then
			require.NoError(t, err)
			expectedTime := (time.Duration((expectedDeactivationTimeoutDeactivate30Tier-preDeactivationNotificationDays)*24) * time.Hour) - timeSinceProvisioned
			actualTime := res.RequeueAfter
			diff := expectedTime - actualTime
			require.Truef(t, diff > 0 && diff < 2*time.Second, "expectedTime: '%v' is not within 2 seconds of actualTime: '%v' diff: '%v'", expectedTime, actualTime, diff)
			assertThatUserSignupDeactivated(t, cl, username, false)
		})

		// the time since the mur was provisioned is within the deactivation timeout period for the 'deactivate90' tier
		t.Run("usersignup should not be deactivated - other tier (90 days)", func(t *testing.T) {
			// given
			murProvisionedTime := metav1.Now()
			mur := murtest.NewMasterUserRecord(t, username, murtest.TierName(userTier90.Name), murtest.Account("cluster1"), murtest.ProvisionedMur(&murProvisionedTime), murtest.UserIDFromUserSignup(userSignupFoobar))
			mur.Labels[toolchainv1alpha1.MasterUserRecordOwnerLabelKey] = userSignupFoobar.Name
			r, req, cl := prepareReconcile(t, mur.Name, userTier90, mur, userSignupFoobar, config)
			// when
			timeSinceProvisioned := time.Since(murProvisionedTime.Time)
			res, err := r.Reconcile(context.TODO(), req)
			// then
			require.NoError(t, err)
			expectedTime := (time.Duration((expectedDeactivationTimeoutDeactivate90Tier-preDeactivationNotificationDays)*24) * time.Hour) - timeSinceProvisioned
			actualTime := res.RequeueAfter
			diff := expectedTime - actualTime
			require.Truef(t, diff > 0 && diff < 2*time.Second, "expectedTime: '%v' is not within 2 seconds of actualTime: '%v' diff: '%v'", expectedTime, actualTime, diff)
			assertThatUserSignupDeactivated(t, cl, username, false)
		})

		// the tier does not have a deactivationTimeoutDays set so the time since the mur was provisioned is irrelevant, the user should not be deactivated
		t.Run("usersignup should not be deactivated - no deactivation tier", func(t *testing.T) {
			// given
			murProvisionedTime := metav1.Now()
			mur := murtest.NewMasterUserRecord(t, username, murtest.TierName(userTierNoDeactivation.Name), murtest.Account("cluster1"), murtest.ProvisionedMur(&murProvisionedTime), murtest.UserIDFromUserSignup(userSignupFoobar))
			mur.Labels[toolchainv1alpha1.MasterUserRecordOwnerLabelKey] = userSignupFoobar.Name
			r, req, cl := prepareReconcile(t, mur.Name, userTierNoDeactivation, mur, userSignupFoobar, config)
			// when
			res, err := r.Reconcile(context.TODO(), req)
			// then
			require.NoError(t, err)
			require.False(t, res.Requeue, "requeue should not be set")
			require.Equal(t, time.Duration(0), res.RequeueAfter, "requeueAfter should not be set")
			assertThatUserSignupDeactivated(t, cl, username, false)
		})

		// a mur that has not been provisioned yet
		t.Run("mur without provisioned time", func(t *testing.T) {
			// given
			mur := murtest.NewMasterUserRecord(t, username, murtest.TierName(userTier30.Name), murtest.Account("cluster1"), murtest.UserIDFromUserSignup(userSignupFoobar))
			r, req, cl := prepareReconcile(t, mur.Name, userTier30, mur, userSignupFoobar, config)
			// when
			res, err := r.Reconcile(context.TODO(), req)
			// then
			require.NoError(t, err)
			require.False(t, res.Requeue, "requeue should not be set")
			require.Equal(t, time.Duration(0), res.RequeueAfter, "requeueAfter should not be set")
			assertThatUserSignupDeactivated(t, cl, username, false)
		})

		// a user that belongs to the deactivation domain excluded list
		t.Run("user deactivation excluded", func(t *testing.T) {
			// given
			config := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.Deactivation().DeactivatingNotificationDays(3))
			restore := commontest.SetEnvVarAndRestore(t, "HOST_OPERATOR_DEACTIVATION_DOMAINS_EXCLUDED", "@redhat.com")
			defer restore()
			murProvisionedTime := &metav1.Time{Time: time.Now().Add(-time.Duration(expectedDeactivationTimeoutDeactivate30Tier*24) * time.Hour)}
			mur := murtest.NewMasterUserRecord(t, username, murtest.TierName(userTier30.Name), murtest.Account("cluster1"), murtest.ProvisionedMur(murProvisionedTime), murtest.UserIDFromUserSignup(userSignupRedhat))
			mur.Labels[toolchainv1alpha1.MasterUserRecordOwnerLabelKey] = userSignupRedhat.Name
			r, req, cl := prepareReconcile(t, mur.Name, userTier30, mur, userSignupRedhat, config)
			// when
			res, err := r.Reconcile(context.TODO(), req)
			// then
			require.NoError(t, err)
			require.False(t, res.Requeue, "requeue should not be set")
			require.Equal(t, time.Duration(0), res.RequeueAfter, "requeueAfter should not be set")
			assertThatUserSignupDeactivated(t, cl, username, false)
		})
	})
	// in these tests, the controller should (eventually) deactivate the user
	t.Run("controller should deactivate user", func(t *testing.T) {
		userSignupFoobar := userSignupWithEmail(username, "foo@bar.com")
		t.Run("usersignup should be marked as deactivating - deactivate30 (30 days)", func(t *testing.T) {
			// given
			states.SetDeactivating(userSignupFoobar, false)

			// Set the provisioned time so that we are now 2 days before the expected deactivation time
			murProvisionedTime := &metav1.Time{Time: time.Now().Add(-time.Duration((expectedDeactivationTimeoutDeactivate30Tier-2)*24) * time.Hour)}
			mur := murtest.NewMasterUserRecord(t, username, murtest.TierName(userTier30.Name), murtest.Account("cluster1"),
				murtest.ProvisionedMur(murProvisionedTime), murtest.UserIDFromUserSignup(userSignupFoobar))
			mur.Labels[toolchainv1alpha1.MasterUserRecordOwnerLabelKey] = userSignupFoobar.Name

			r, req, cl := prepareReconcile(t, mur.Name, userTier30, mur, userSignupFoobar, config)
			// when
			res, err := r.Reconcile(context.TODO(), req)
			// then
			require.NoError(t, err)
			require.False(t, res.Requeue)

			// Reload the userSignup
			require.NoError(t, cl.Get(context.TODO(), types.NamespacedName{Name: userSignupFoobar.Name, Namespace: operatorNamespace}, userSignupFoobar))
			require.True(t, states.Deactivating(userSignupFoobar))
			require.False(t, states.Deactivated(userSignupFoobar))

			t.Run("reconciliation should be requeued when notification not yet sent", func(t *testing.T) {
				r, req, cl := prepareReconcile(t, mur.Name, userTier30, mur, userSignupFoobar, config)
				// when
				res, err := r.Reconcile(context.TODO(), req)
				// then
				require.NoError(t, err)
				require.False(t, res.Requeue)

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

					r, req, cl := prepareReconcile(t, mur.Name, userTier30, mur, userSignupFoobar, config)

					// when
					res, err := r.Reconcile(context.TODO(), req)
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

						r, req, cl := prepareReconcile(t, mur.Name, userTier30, mur, userSignupFoobar, config)

						// when
						res, err := r.Reconcile(context.TODO(), req)
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
							res, err := r.Reconcile(context.TODO(), req)
							// then
							require.NoError(t, err)
							require.False(t, res.Requeue, "requeue should not be set")
							require.Equal(t, time.Duration(0), res.RequeueAfter, "requeueAfter should not be set")
						})
					})
				})
			})
		})

		// the time since the mur was provisioned exceeds the deactivation timeout period for the 'other' tier
		t.Run("usersignup should be deactivated - other tier (60 days)", func(t *testing.T) {
			states.SetDeactivating(userSignupFoobar, true)
			// given
			murProvisionedTime := &metav1.Time{Time: time.Now().Add(-time.Duration(expectedDeactivationTimeoutDeactivate90Tier*24) * time.Hour)}
			mur := murtest.NewMasterUserRecord(t, username, murtest.TierName(userTier90.Name), murtest.Account("cluster1"), murtest.ProvisionedMur(murProvisionedTime), murtest.UserIDFromUserSignup(userSignupFoobar))
			mur.Labels[toolchainv1alpha1.MasterUserRecordOwnerLabelKey] = userSignupFoobar.Name
			r, req, cl := prepareReconcile(t, mur.Name, userTier90, mur, userSignupFoobar, config)
			// when
			res, err := r.Reconcile(context.TODO(), req)
			// then
			require.NoError(t, err)
			require.False(t, res.Requeue, "requeue should not be set")
			require.Equal(t, time.Duration(0), res.RequeueAfter, "requeue should not be set")
			assertThatUserSignupDeactivated(t, cl, username, true)
			AssertMetricsCounterEquals(t, 1, metrics.UserSignupAutoDeactivatedTotal)
		})
	})

	t.Run("test usersignup deactivating state reset to false", func(t *testing.T) {
		t.Run("when the provisioned time is after the deactivatingNotificationTimeout", func(t *testing.T) {
			// given
			userSignupFoobar := userSignupWithEmail(username, "foo@bar.com")

			// Set usersignup state as already set to deactivating
			states.SetDeactivating(userSignupFoobar, true)

			// Set the provisioned time so that we were just 2 days from the original expected 30 day deactivation time (28 days)
			murProvisionedTime := &metav1.Time{Time: time.Now().Add(-time.Duration((expectedDeactivationTimeoutDeactivate30Tier-2)*24) * time.Hour)}

			// Now the MasterUserRecord has been promoted to the 90 day tier
			mur := murtest.NewMasterUserRecord(t, username, murtest.TierName(userTier90.Name), murtest.Account("cluster1"),
				murtest.ProvisionedMur(murProvisionedTime), murtest.UserIDFromUserSignup(userSignupFoobar))
			mur.Labels[toolchainv1alpha1.MasterUserRecordOwnerLabelKey] = userSignupFoobar.Name

			r, req, cl := prepareReconcile(t, mur.Name, userTier90, mur, userSignupFoobar, config)

			// when
			res, err := r.Reconcile(context.TODO(), req)

			// then
			require.NoError(t, err)
			// The RequeueAfter should be ~about 59 days...(28 days from the new deactivatingNotificationTimeout = 90-3-28) let's accept if it's within 1 hour of that
			require.WithinDuration(t, time.Now().Add(time.Duration(59*24)*time.Hour), time.Now().Add(res.RequeueAfter), time.Duration(1)*time.Hour)

			// Reload the userSignup
			require.NoError(t, cl.Get(context.TODO(), types.NamespacedName{Name: userSignupFoobar.Name, Namespace: operatorNamespace}, userSignupFoobar))
			require.False(t, states.Deactivating(userSignupFoobar))
			require.False(t, states.Deactivated(userSignupFoobar))
		})

		t.Run("when provisioning state is set but user is moved to a tier without deactivation", func(t *testing.T) {
			// given
			userSignupFoobar := userSignupWithEmail(username, "foo@bar.com")

			// Set usersignup state as already set to deactivating
			states.SetDeactivating(userSignupFoobar, true)

			// Set the provisioned time so that we were just 2 days from the original expected 30 day deactivation time (28 days)
			murProvisionedTime := &metav1.Time{Time: time.Now().Add(-time.Duration((expectedDeactivationTimeoutDeactivate30Tier-2)*24) * time.Hour)}

			// Now the MasterUserRecord has been promoted to the tier without automatic deactivation
			mur := murtest.NewMasterUserRecord(t, username, murtest.TierName(userTierNoDeactivation.Name), murtest.Account("cluster1"),
				murtest.ProvisionedMur(murProvisionedTime), murtest.UserIDFromUserSignup(userSignupFoobar))
			mur.Labels[toolchainv1alpha1.MasterUserRecordOwnerLabelKey] = userSignupFoobar.Name

			r, req, cl := prepareReconcile(t, mur.Name, userTierNoDeactivation, mur, userSignupFoobar, config)

			// when
			res, err := r.Reconcile(context.TODO(), req)

			// then
			require.NoError(t, err)
			require.False(t, res.Requeue) // no requeue since user should not be auto deactivated

			// Reload the userSignup
			require.NoError(t, cl.Get(context.TODO(), types.NamespacedName{Name: userSignupFoobar.Name, Namespace: operatorNamespace}, userSignupFoobar))
			require.False(t, states.Deactivating(userSignupFoobar))
			require.False(t, states.Deactivated(userSignupFoobar))
		})
	})

	t.Run("failures", func(t *testing.T) {
		// cannot find UserTier
		t.Run("unable to get UserTier", func(t *testing.T) {
			// given
			murProvisionedTime := metav1.Now()
			mur := murtest.NewMasterUserRecord(t, username, murtest.TierName(userTier30.Name), murtest.Account("cluster1"), murtest.ProvisionedMur(&murProvisionedTime), murtest.UserIDFromUserSignup(userSignupFoobar))
			mur.Labels[toolchainv1alpha1.MasterUserRecordOwnerLabelKey] = userSignupFoobar.Name
			r, req, cl := prepareReconcile(t, mur.Name, mur, userSignupFoobar, config)
			// when
			_, err := r.Reconcile(context.TODO(), req)
			// then
			require.Error(t, err)
			require.Contains(t, err.Error(), `usertiers.toolchain.dev.openshift.com "deactivate30" not found`)
			assertThatUserSignupDeactivated(t, cl, username, false)
		})

		// cannot get UserSignup
		t.Run("UserSignup get failure", func(t *testing.T) {
			// given
			murProvisionedTime := &metav1.Time{Time: time.Now().Add(-time.Duration(expectedDeactivationTimeoutDeactivate30Tier*24) * time.Hour)}
			mur := murtest.NewMasterUserRecord(t, username, murtest.TierName(userTier30.Name), murtest.Account("cluster1"), murtest.ProvisionedMur(murProvisionedTime), murtest.UserIDFromUserSignup(userSignupFoobar))
			mur.Labels[toolchainv1alpha1.MasterUserRecordOwnerLabelKey] = userSignupFoobar.Name
			r, req, cl := prepareReconcile(t, mur.Name, userTier30, mur, userSignupFoobar, config)
			cl.MockGet = func(ctx context.Context, key runtimeclient.ObjectKey, obj runtimeclient.Object, opts ...runtimeclient.GetOption) error {
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
			res, err := r.Reconcile(context.TODO(), req)
			// then
			require.Error(t, err)
			require.Contains(t, err.Error(), "usersignup get error")
			require.False(t, res.Requeue, "requeue should not be set")
			require.Equal(t, time.Duration(0), res.RequeueAfter, "requeueAfter should not be set")
		})

		// cannot update UserSignup
		t.Run("UserSignup update failure", func(t *testing.T) {
			// given
			userSignupFoobar := userSignupWithEmail(username, "foo@bar.com")
			states.SetDeactivating(userSignupFoobar, true)
			deactivatingCondition := toolchainv1alpha1.Condition{
				Type:               toolchainv1alpha1.UserSignupUserDeactivatingNotificationCreated,
				Status:             corev1.ConditionTrue,
				Reason:             toolchainv1alpha1.UserSignupDeactivatingNotificationCRCreatedReason,
				LastTransitionTime: metav1.Time{Time: time.Now().Add(time.Duration(-3) * time.Hour * 24)},
			}
			userSignupFoobar.Status.Conditions = condition.AddStatusConditions(userSignupFoobar.Status.Conditions, deactivatingCondition)

			murProvisionedTime := &metav1.Time{Time: time.Now().Add(-time.Duration(expectedDeactivationTimeoutDeactivate30Tier*24) * time.Hour)}
			mur := murtest.NewMasterUserRecord(t, username, murtest.TierName(userTier30.Name), murtest.Account("cluster1"), murtest.ProvisionedMur(murProvisionedTime), murtest.UserIDFromUserSignup(userSignupFoobar))
			mur.Labels[toolchainv1alpha1.MasterUserRecordOwnerLabelKey] = userSignupFoobar.Name
			r, req, cl := prepareReconcile(t, mur.Name, userTier30, mur, userSignupFoobar, config)
			cl.MockUpdate = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.UpdateOption) error {
				_, ok := obj.(*toolchainv1alpha1.UserSignup)
				if ok {
					return fmt.Errorf("usersignup update error")
				}
				return nil
			}
			// when
			res, err := r.Reconcile(context.TODO(), req)
			// then
			require.Error(t, err)
			require.Contains(t, err.Error(), "usersignup update error")
			require.False(t, res.Requeue, "requeue should not be set")
			require.Equal(t, time.Duration(0), res.RequeueAfter, "requeueAfter should not be set")
			assertThatUserSignupDeactivated(t, cl, username, false)
		})
	})
}

func prepareReconcile(t *testing.T, name string, initObjs ...runtime.Object) (reconcile.Reconciler, reconcile.Request, *commontest.FakeClient) {
	os.Setenv("WATCH_NAMESPACE", commontest.HostOperatorNs)
	metrics.Reset()
	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)
	cl := commontest.NewFakeClient(t, initObjs...)
	require.NoError(t, err)
	r := &Reconciler{
		Client: cl,
		Scheme: s,
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
		name = uuid.Must(uuid.NewV4()).String()
	}
	emailHash := hash.EncodeString(email)
	return metav1.ObjectMeta{
		Name:      name,
		Namespace: operatorNamespace,
		Labels: map[string]string{
			toolchainv1alpha1.UserSignupUserEmailHashLabelKey: emailHash,
		},
	}
}

func userSignupWithEmail(username, email string) *toolchainv1alpha1.UserSignup {
	us := &toolchainv1alpha1.UserSignup{
		ObjectMeta: newObjectMeta(username, email),
		Spec: toolchainv1alpha1.UserSignupSpec{
			TargetCluster: "east",
			IdentityClaims: toolchainv1alpha1.IdentityClaimsEmbedded{
				PropagatedClaims: toolchainv1alpha1.PropagatedClaims{
					Sub:   username,
					Email: email,
				},
				PreferredUsername: email,
			},
		},
	}
	states.SetApprovedManually(us, true)

	return us
}

func assertThatUserSignupDeactivated(t *testing.T, cl *commontest.FakeClient, name string, expected bool) {
	userSignup := &toolchainv1alpha1.UserSignup{}
	err := cl.Get(context.TODO(), types.NamespacedName{Name: name, Namespace: operatorNamespace}, userSignup)
	require.NoError(t, err)
	require.Equal(t, expected, states.Deactivated(userSignup))
}
