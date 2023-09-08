package masteruserrecord

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	"github.com/codeready-toolchain/host-operator/pkg/cluster"
	"github.com/codeready-toolchain/host-operator/pkg/metrics"
	. "github.com/codeready-toolchain/host-operator/test"
	tiertest "github.com/codeready-toolchain/host-operator/test/nstemplatetier"
	commoncluster "github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	commontest "github.com/codeready-toolchain/toolchain-common/pkg/test"
	murtest "github.com/codeready-toolchain/toolchain-common/pkg/test/masteruserrecord"
	uatest "github.com/codeready-toolchain/toolchain-common/pkg/test/useraccount"
	commonsignup "github.com/codeready-toolchain/toolchain-common/pkg/test/usersignup"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierros "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var testLog = ctrl.Log.WithName("test")

func TestAddFinalizer(t *testing.T) {
	// given
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	s := apiScheme(t)

	t.Run("ok", func(t *testing.T) {
		// given
		mur := murtest.NewMasterUserRecord(t, "john")
		memberClient := commontest.NewFakeClient(t)
		hostClient := commontest.NewFakeClient(t, mur)
		InitializeCounters(t, NewToolchainStatus(
			WithMember(commontest.MemberClusterName),
			WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
				string(metrics.Internal): 1,
			}),
			WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
				"1,internal": 1,
			}),
		))

		cntrl := newController(hostClient, s, ClusterClient(commontest.MemberClusterName, memberClient))

		// when
		result, err := cntrl.Reconcile(context.TODO(), newMurRequest(mur))

		// then
		require.NoError(t, err)
		require.False(t, result.Requeue)

		murtest.AssertThatMasterUserRecord(t, "john", hostClient).
			HasConditions(toBeNotReady(toolchainv1alpha1.MasterUserRecordProvisioningReason, "")).
			HasFinalizer()
		AssertThatCountersAndMetrics(t).
			HaveUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
				"1,internal": 1, // unchanged
			}).
			HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
				string(metrics.Internal): 1, // unchanged
			})
	})

	t.Run("fails because it cannot add finalizer", func(t *testing.T) {
		// given
		mur := murtest.NewMasterUserRecord(t, "john")
		hostClient := commontest.NewFakeClient(t, mur)
		memberClient := commontest.NewFakeClient(t)
		hostClient.MockUpdate = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.UpdateOption) error {
			return fmt.Errorf("unable to add finalizer to MUR %s", mur.Name)
		}
		InitializeCounters(t, NewToolchainStatus(
			WithMember(commontest.MemberClusterName),
			WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
				"1,internal": 1,
			}),
			WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
				string(metrics.Internal): 1,
			})))

		cntrl := newController(hostClient, s, ClusterClient(commontest.MemberClusterName, memberClient))

		// when
		_, err := cntrl.Reconcile(context.TODO(), newMurRequest(mur))

		// then
		require.Error(t, err)
		msg := "failed while updating with added finalizer: unable to add finalizer to MUR john"
		assert.Contains(t, err.Error(), msg)

		uatest.AssertThatUserAccount(t, "john", memberClient).DoesNotExist()
		murtest.AssertThatMasterUserRecord(t, "john", hostClient).
			HasConditions(toBeNotReady(toolchainv1alpha1.MasterUserRecordUnableToAddFinalizerReason, "unable to add finalizer to MUR john"))
		AssertThatCountersAndMetrics(t).
			HaveUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
				"1,internal": 1, // unchanged
			}).
			HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
				string(metrics.Internal): 1, // unchanged
			})
	})
}

func TestCreateUserAccountSuccessful(t *testing.T) {
	// given
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	s := apiScheme(t)
	mur := murtest.NewMasterUserRecord(t, "john")
	mur.Spec.OriginalSub = "original-sub:12345"
	mur.Annotations[toolchainv1alpha1.SSOUserIDAnnotationKey] = "123456"
	mur.Annotations[toolchainv1alpha1.SSOAccountIDAnnotationKey] = "987654"

	require.NoError(t, murtest.Modify(mur, murtest.Finalizer("finalizer.toolchain.dev.openshift.com")))
	memberClient := commontest.NewFakeClient(t)
	hostClient := commontest.NewFakeClient(t, mur)
	InitializeCounters(t, NewToolchainStatus(
		WithMember(commontest.MemberClusterName),
		WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
			"1,internal": 1,
		}),
		WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
			string(metrics.Internal): 1,
		})))

	cntrl := newController(hostClient, s, ClusterClient(commontest.MemberClusterName, memberClient))

	// when
	_, err := cntrl.Reconcile(context.TODO(), newMurRequest(mur))

	// then
	require.NoError(t, err)
	uatest.AssertThatUserAccount(t, "john", memberClient).
		Exists().
		MatchMasterUserRecord(mur).
		HasLabelWithValue(toolchainv1alpha1.TierLabelKey, "deactivate30").
		HasAnnotationWithValue(toolchainv1alpha1.SSOUserIDAnnotationKey, "123456").
		HasAnnotationWithValue(toolchainv1alpha1.SSOAccountIDAnnotationKey, "987654")

	murtest.AssertThatMasterUserRecord(t, "john", hostClient).
		HasConditions(toBeNotReady(toolchainv1alpha1.MasterUserRecordProvisioningReason, "")).
		HasFinalizer()
	AssertThatCountersAndMetrics(t).
		HaveUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
			"1,internal": 1, // unchanged
		}).
		HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
			string(metrics.Internal): 1, // unchanged
		})
}

func TestCreateUserAccountWhenItWasPreviouslyDeleted(t *testing.T) {
	// given
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	s := apiScheme(t)
	mur := murtest.NewMasterUserRecord(t, "john")
	mur.Status.UserAccounts = []toolchainv1alpha1.UserAccountStatusEmbedded{
		{
			Cluster: toolchainv1alpha1.Cluster{
				Name: commontest.MemberClusterName,
			},
		},
	}
	require.NoError(t, murtest.Modify(mur, murtest.Finalizer("finalizer.toolchain.dev.openshift.com")))
	memberClient := commontest.NewFakeClient(t)
	hostClient := commontest.NewFakeClient(t, mur)
	InitializeCounters(t, NewToolchainStatus(
		WithMember(commontest.MemberClusterName),
		WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
			"1,internal": 1,
		}),
		WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
			string(metrics.Internal): 1,
		})))

	cntrl := newController(hostClient, s, ClusterClient(commontest.MemberClusterName, memberClient))

	// when
	_, err := cntrl.Reconcile(context.TODO(), newMurRequest(mur))

	// then
	require.NoError(t, err)
	uatest.AssertThatUserAccount(t, "john", memberClient).
		Exists().
		MatchMasterUserRecord(mur).
		HasLabelWithValue(toolchainv1alpha1.TierLabelKey, "deactivate30")
	murtest.AssertThatMasterUserRecord(t, "john", hostClient).
		HasConditions(toBeNotReady(toolchainv1alpha1.MasterUserRecordProvisioningReason, "")).
		HasFinalizer()
	AssertThatCountersAndMetrics(t).
		HaveUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
			"1,internal": 1, // unchanged
		}).
		HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
			string(metrics.Internal): 1, // unchanged
		})

}

func TestCreateMultipleUserAccountsSuccessful(t *testing.T) {
	// given
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	s := apiScheme(t)
	mur := murtest.NewMasterUserRecord(t, "john", murtest.AdditionalAccounts(commontest.Member2ClusterName), murtest.Finalizer("finalizer.toolchain.dev.openshift.com"))
	toolchainStatus := NewToolchainStatus(
		WithMember(commontest.MemberClusterName, WithRoutes("https://console.member-cluster/", "", ToBeReady())),
		WithMember(commontest.Member2ClusterName, WithRoutes("https://console.member-cluster/", "", ToBeReady())),
		WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
			"1,internal": 1,
		}),
		WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
			string(metrics.Internal): 1,
		}),
	)
	memberClient := commontest.NewFakeClient(t)
	memberClient2 := commontest.NewFakeClient(t)
	hostClient := commontest.NewFakeClient(t, mur, toolchainStatus)
	InitializeCounters(t, toolchainStatus)

	cntrl := newController(hostClient, s, ClusterClient(commontest.MemberClusterName, memberClient), ClusterClient(commontest.Member2ClusterName, memberClient2))

	// when reconciling
	result, err := cntrl.Reconcile(context.TODO(), newMurRequest(mur))
	// then
	require.NoError(t, err)
	assert.False(t, result.Requeue)
	uatest.AssertThatUserAccount(t, "john", memberClient).
		Exists().
		MatchMasterUserRecord(mur).
		HasLabelWithValue(toolchainv1alpha1.TierLabelKey, "deactivate30")
	uatest.AssertThatUserAccount(t, "john", memberClient2).
		Exists().
		MatchMasterUserRecord(mur).
		HasLabelWithValue(toolchainv1alpha1.TierLabelKey, "deactivate30")
	murtest.AssertThatMasterUserRecord(t, "john", hostClient).
		HasConditions(toBeNotReady(toolchainv1alpha1.MasterUserRecordProvisioningReason, "")).
		HasFinalizer()
	AssertThatCountersAndMetrics(t).
		HaveUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
			"1,internal": 1, // unchanged
		}).
		HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
			string(metrics.Internal): 1, // unchanged
		})
}

func TestRequeueWhenUserAccountDeleted(t *testing.T) {
	// given
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	s := apiScheme(t)
	mur := murtest.NewMasterUserRecord(t, "john", murtest.AdditionalAccounts(commontest.Member2ClusterName), murtest.Finalizer("finalizer.toolchain.dev.openshift.com"))
	userAccount1 := uatest.NewUserAccountFromMur(mur)
	userAccount3 := uatest.NewUserAccountFromMur(mur)
	toolchainStatus := NewToolchainStatus(
		WithMember(commontest.MemberClusterName, WithRoutes("https://console.member-cluster/", "", ToBeReady())),
		WithMember(commontest.Member2ClusterName, WithRoutes("https://console.member2-cluster/", "", ToBeReady())),
		WithMember("member3-cluster", WithRoutes("https://console.member3-cluster/", "", ToBeReady())),
		WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
			"1,internal": 1,
		}),
		WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
			string(metrics.Internal): 1,
		}))
	memberClient1 := commontest.NewFakeClient(t, userAccount1)
	memberClient3 := commontest.NewFakeClient(t, userAccount3)
	hostClient := commontest.NewFakeClient(t, mur, toolchainStatus)

	t.Run("when userAccount is accidentally being deleted then don't change the counter", func(t *testing.T) {
		// given
		InitializeCounters(t, toolchainStatus)
		userAccount2 := uatest.NewUserAccountFromMur(mur, uatest.DeletedUa())
		memberClient2 := commontest.NewFakeClient(t, userAccount2)
		cntrl := newController(hostClient, s, ClusterClient(commontest.MemberClusterName, memberClient1),
			ClusterClient(commontest.Member2ClusterName, memberClient2),
			ClusterClient("member3-cluster", memberClient3))

		// when
		_, err := cntrl.Reconcile(context.TODO(), newMurRequest(mur))

		// then
		require.NoError(t, err)
		AssertThatCountersAndMetrics(t).
			HaveUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
				"1,internal": 1, // unchanged
			}).
			HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
				string(metrics.Internal): 1, // unchanged
			})
	})
}

func TestCreateSynchronizeOrDeleteUserAccountFailed(t *testing.T) {
	// given
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	s := apiScheme(t)
	mur := murtest.NewMasterUserRecord(t, "john", murtest.Finalizer("finalizer.toolchain.dev.openshift.com"))
	hostClient := commontest.NewFakeClient(t, mur)

	t.Run("when member cluster does not exist and UA hasn't been created yet", func(t *testing.T) {
		// given
		InitializeCounters(t, NewToolchainStatus(
			WithMember(commontest.MemberClusterName),
			WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
				"1,internal": 1,
			}),
			WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
				string(metrics.Internal): 1,
			}),
		))
		memberClient := commontest.NewFakeClient(t)

		cntrl := newController(hostClient, s, ClusterClient("other", memberClient))

		// when
		_, err := cntrl.Reconcile(context.TODO(), newMurRequest(mur))

		// then
		require.Error(t, err)
		msg := "unknown target member cluster 'member-cluster'"
		assert.Contains(t, err.Error(), msg)

		uatest.AssertThatUserAccount(t, "john", memberClient).DoesNotExist()
		murtest.AssertThatMasterUserRecord(t, "john", hostClient).
			HasConditions(toBeNotReady(toolchainv1alpha1.MasterUserRecordTargetClusterNotReadyReason, msg)).
			HasFinalizer()
		AssertThatCountersAndMetrics(t).
			HaveUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
				"1,internal": 1, // unchanged
			}).
			HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
				string(metrics.Internal): 1, // unchanged
			})
	})

	t.Run("when member cluster does not exist and UA was already created", func(t *testing.T) {
		// given
		InitializeCounters(t, NewToolchainStatus(
			WithMember(commontest.MemberClusterName),
			WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
				"1,internal": 1,
			}),
			WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
				string(metrics.Internal): 1,
			})))
		memberClient := commontest.NewFakeClient(t, uatest.NewUserAccountFromMur(mur))

		cntrl := newController(hostClient, s, ClusterClient("other", memberClient))

		// when
		_, err := cntrl.Reconcile(context.TODO(), newMurRequest(mur))

		// then
		require.Error(t, err)
		msg := "unknown target member cluster 'member-cluster'"
		assert.Contains(t, err.Error(), msg)
		murtest.AssertThatMasterUserRecord(t, "john", hostClient).
			HasConditions(toBeNotReady(toolchainv1alpha1.MasterUserRecordTargetClusterNotReadyReason, msg)).
			HasFinalizer()
		AssertThatCountersAndMetrics(t).
			HaveUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
				"1,internal": 1, // unchanged
			}).
			HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
				string(metrics.Internal): 1, // unchanged
			})
	})

	t.Run("status update of the MasterUserRecord failed", func(t *testing.T) {
		// given
		InitializeCounters(t, NewToolchainStatus(
			WithMember(commontest.MemberClusterName),
			WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
				"1,internal": 1,
			}),
			WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
				string(metrics.Internal): 1,
			})))
		memberClient := commontest.NewFakeClient(t)

		cntrl := newController(hostClient, s, ClusterClient(commontest.MemberClusterName, memberClient))
		statusUpdater := func(logger logr.Logger, mur *toolchainv1alpha1.MasterUserRecord, message string) error {
			return fmt.Errorf("unable to update status")
		}

		// when
		err := cntrl.wrapErrorWithStatusUpdate(testLog, mur, statusUpdater,
			apierros.NewBadRequest("oopsy woopsy"), "failed to create %s", "user bob")

		// then
		require.Error(t, err)
		assert.Equal(t, "failed to create user bob: oopsy woopsy", err.Error())
		AssertThatCountersAndMetrics(t).
			HaveUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
				"1,internal": 1, // unchanged
			}).
			HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
				string(metrics.Internal): 1, // unchanged
			})
	})

	t.Run("creation of the UserAccount failed", func(t *testing.T) {
		// given
		InitializeCounters(t, NewToolchainStatus(
			WithMember(commontest.MemberClusterName),
			WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
				"1,internal": 1,
			}),
			WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
				string(metrics.Internal): 1,
			})))
		memberClient := commontest.NewFakeClient(t)
		memberClient.MockCreate = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.CreateOption) error {
			return fmt.Errorf("unable to create user account %s", mur.Name)
		}

		cntrl := newController(hostClient, s, ClusterClient(commontest.MemberClusterName, memberClient))

		// when
		_, err := cntrl.Reconcile(context.TODO(), newMurRequest(mur))

		// then
		require.Error(t, err)
		msg := "failed to create UserAccount in the member cluster 'member-cluster'"
		assert.Contains(t, err.Error(), msg)

		uatest.AssertThatUserAccount(t, "john", memberClient).DoesNotExist()
		murtest.AssertThatMasterUserRecord(t, "john", hostClient).
			HasConditions(toBeNotReady(toolchainv1alpha1.MasterUserRecordUnableToCreateUserAccountReason, "unable to create user account john"))
		AssertThatCountersAndMetrics(t).
			HaveUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
				"1,internal": 1, // unchanged
			}).
			HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
				string(metrics.Internal): 1, // unchanged
			})
	})

	t.Run("spec synchronization of the UserAccount failed", func(t *testing.T) {
		// given
		InitializeCounters(t, NewToolchainStatus(
			WithMember(commontest.MemberClusterName),
			WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
				"1,internal": 1,
			}),
			WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
				string(metrics.Internal): 1,
			})))
		userAcc := uatest.NewUserAccountFromMur(mur)
		memberClient := commontest.NewFakeClient(t, userAcc)
		memberClient.MockUpdate = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.UpdateOption) error {
			if ua, ok := obj.(*toolchainv1alpha1.UserAccount); ok {
				return fmt.Errorf("unable to update user account %s", ua.Name)
			}
			return memberClient.Client.Update(ctx, obj, opts...)
		}
		otherTier := tiertest.OtherTier()
		modifiedMur := murtest.NewMasterUserRecord(t, "john",
			murtest.Finalizer("finalizer.toolchain.dev.openshift.com"),
			murtest.TierName(otherTier.Name),
			murtest.UserID("abc123")) // UserID is different and needs to be synced
		hostClient := commontest.NewFakeClient(t, modifiedMur)

		cntrl := newController(hostClient, s, ClusterClient(commontest.MemberClusterName, memberClient))

		// when
		_, err := cntrl.Reconcile(context.TODO(), newMurRequest(modifiedMur))

		// then
		require.Error(t, err)
		msg := "unable to update user account john"
		assert.Contains(t, err.Error(), msg)

		uatest.AssertThatUserAccount(t, "john", memberClient).
			Exists().
			HasSpec(userAcc.Spec) // UserAccount should be unchanged
		murtest.AssertThatMasterUserRecord(t, "john", hostClient).
			HasConditions(toBeNotReady(toolchainv1alpha1.MasterUserRecordUnableToSynchronizeUserAccountSpecReason, "unable to update user account john"))
		AssertThatCountersAndMetrics(t).
			HaveUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
				"1,internal": 1, // unchanged
			}).
			HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
				string(metrics.Internal): 1, // unchanged
			})
	})

	t.Run("status synchronization between UserAccount and MasterUserRecord failed", func(t *testing.T) {
		// given
		toolchainStatus := NewToolchainStatus(
			WithMember(commontest.MemberClusterName, WithRoutes("https://console.member-cluster/", "", ToBeReady())),
			WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
				"1,internal": 1,
			}),
			WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
				string(metrics.Internal): 1,
			}))
		updatingCond := toBeNotReady("updating", "")
		provisionedMur := murtest.NewMasterUserRecord(t, "john",
			murtest.Finalizer("finalizer.toolchain.dev.openshift.com"),
			murtest.StatusCondition(updatingCond))
		memberClient := commontest.NewFakeClient(t, uatest.NewUserAccountFromMur(provisionedMur,
			uatest.StatusCondition(toBeNotReady("somethingFailed", ""))))
		hostClient := commontest.NewFakeClient(t, provisionedMur, toolchainStatus)
		InitializeCounters(t, toolchainStatus)

		hostClient.MockStatusUpdate = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.UpdateOption) error {
			hostClient.MockStatusUpdate = nil // mock only once
			return fmt.Errorf("unable to update MUR %s", provisionedMur.Name)
		}

		cntrl := newController(hostClient, s, ClusterClient(commontest.MemberClusterName, memberClient))

		// when
		_, err := cntrl.Reconcile(context.TODO(), newMurRequest(provisionedMur))

		// then
		require.Error(t, err)
		msg := "update of the MasterUserRecord failed while synchronizing with UserAccount status from the cluster 'member-cluster'"
		assert.Contains(t, err.Error(), msg)

		uatest.AssertThatUserAccount(t, "john", memberClient).Exists()
		updatingCond.Message = msg + ": unable to update MUR john"
		murtest.AssertThatMasterUserRecord(t, "john", hostClient).
			HasConditions(updatingCond).
			HasStatusUserAccounts()
		AssertThatCountersAndMetrics(t).
			HaveUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
				"1,internal": 1, // unchanged
			}).
			HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
				string(metrics.Internal): 1, // unchanged
			})
	})

	t.Run("deletion of the UserAccount failed", func(t *testing.T) {
		// given
		InitializeCounters(t, NewToolchainStatus(
			WithMember(commontest.MemberClusterName),
			WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
				"1,internal": 1,
			}),
			WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
				string(metrics.Internal): 1,
			})))
		mur := murtest.NewMasterUserRecord(t, "john",
			murtest.Finalizer("finalizer.toolchain.dev.openshift.com"),
			murtest.ToBeDeleted())
		hostClient := commontest.NewFakeClient(t, mur)

		memberClient := commontest.NewFakeClient(t, uatest.NewUserAccountFromMur(mur))
		memberClient.MockDelete = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.DeleteOption) error {
			return fmt.Errorf("unable to delete user account %s", mur.Name)
		}

		cntrl := newController(hostClient, s, ClusterClient(commontest.MemberClusterName, memberClient))

		// when
		_, err := cntrl.Reconcile(context.TODO(), newMurRequest(mur))

		// then
		require.Error(t, err)
		msg := "failed to delete UserAccount in the member cluster 'member-cluster'"
		assert.Contains(t, err.Error(), msg)

		uatest.AssertThatUserAccount(t, "john", memberClient).Exists()
		murtest.AssertThatMasterUserRecord(t, "john", hostClient).
			HasConditions(toBeNotReady(toolchainv1alpha1.MasterUserRecordUnableToDeleteUserAccountsReason, "unable to delete user account john")).
			HasFinalizer()
		AssertThatCountersAndMetrics(t).
			HaveUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
				"1,internal": 1, // unchanged
			}).
			HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
				string(metrics.Internal): 1, // unchanged
			})
	})
}

func TestModifyUserAccounts(t *testing.T) {
	// given
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	s := apiScheme(t)
	mur := murtest.NewMasterUserRecord(t, "john",
		murtest.Finalizer("finalizer.toolchain.dev.openshift.com"),
		murtest.StatusCondition(toBeProvisioned()),
		murtest.AdditionalAccounts(commontest.Member2ClusterName, "member3-cluster"))

	userAccount := uatest.NewUserAccountFromMur(mur)
	userAccount2 := uatest.NewUserAccountFromMur(mur)
	userAccount3 := uatest.NewUserAccountFromMur(mur)

	err := murtest.Modify(mur, murtest.UserID("abc123"))
	require.NoError(t, err)

	toolchainStatus := NewToolchainStatus(
		WithMember(commontest.MemberClusterName, WithRoutes("https://console.member-cluster/", "", ToBeReady())),
		WithMember(commontest.Member2ClusterName, WithRoutes("https://console.member2-cluster/", "", ToBeReady())),
		WithMember("member3-cluster", WithRoutes("https://console.member3-cluster/", "", ToBeReady())),
		WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
			"1,internal": 1,
		}),
		WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
			string(metrics.Internal): 1,
		}))

	memberClient := commontest.NewFakeClient(t, userAccount)
	memberClient2 := commontest.NewFakeClient(t, userAccount2)
	memberClient3 := commontest.NewFakeClient(t, userAccount3)
	hostClient := commontest.NewFakeClient(t, mur, toolchainStatus)

	InitializeCounters(t, toolchainStatus)

	cntrl := newController(hostClient, s, ClusterClient(commontest.MemberClusterName, memberClient), ClusterClient(commontest.Member2ClusterName, memberClient2),
		ClusterClient("member3-cluster", memberClient3))

	// when ensuring 1st account
	_, err = cntrl.Reconcile(context.TODO(), newMurRequest(mur))
	// then
	require.NoError(t, err)
	uatest.AssertThatUserAccount(t, "john", memberClient).
		Exists().
		MatchMasterUserRecord(mur).
		HasLabelWithValue(toolchainv1alpha1.TierLabelKey, "deactivate30")

	// when ensuring 2nd account
	_, err = cntrl.Reconcile(context.TODO(), newMurRequest(mur))
	// then
	require.NoError(t, err)
	uatest.AssertThatUserAccount(t, "john", memberClient2).
		Exists().
		MatchMasterUserRecord(mur).
		HasLabelWithValue(toolchainv1alpha1.TierLabelKey, "deactivate30")

	// when ensuring 3rd account
	_, err = cntrl.Reconcile(context.TODO(), newMurRequest(mur))
	// then
	require.NoError(t, err)
	uatest.AssertThatUserAccount(t, "john", memberClient3).
		Exists().
		MatchMasterUserRecord(mur).
		HasLabelWithValue(toolchainv1alpha1.TierLabelKey, "deactivate30")
	murtest.AssertThatMasterUserRecord(t, "john", hostClient).
		HasConditions(toBeNotReady(toolchainv1alpha1.MasterUserRecordUpdatingReason, ""))
	AssertThatCountersAndMetrics(t).
		HaveUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
			"1,internal": 1, // unchanged
		}).
		HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
			string(metrics.Internal): 1, // unchanged
		})
}

func TestSyncMurStatusWithUserAccountStatuses(t *testing.T) {
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	s := apiScheme(t)

	t.Run("mur status synced with updated user account statuses", func(t *testing.T) {
		// given
		// setup MUR that wil contain UserAccountStatusEmbedded fields for UserAccounts from commontest.Member2ClusterName and "member3-cluster" but will miss from commontest.MemberClusterName
		// then the reconcile should add the misssing UserAccountStatusEmbedded for the missing commontest.MemberClusterName cluster without updating anything else
		mur := murtest.NewMasterUserRecord(t, "john",
			murtest.Finalizer("finalizer.toolchain.dev.openshift.com"),
			murtest.StatusCondition(toBeNotReady(toolchainv1alpha1.MasterUserRecordProvisioningReason, "")),
			murtest.AdditionalAccounts(commontest.Member2ClusterName, "member3-cluster"))

		userAccount := uatest.NewUserAccountFromMur(mur,
			uatest.StatusCondition(toBeNotReady("Provisioning", "")), uatest.ResourceVersion("123abc"))
		userAccount2 := uatest.NewUserAccountFromMur(mur,
			uatest.StatusCondition(toBeNotReady("Provisioning", "")), uatest.ResourceVersion("123abc"))
		userAccount3 := uatest.NewUserAccountFromMur(mur,
			uatest.StatusCondition(toBeNotReady("Provisioning", "")), uatest.ResourceVersion("123abc"))

		mur.Status.UserAccounts = []toolchainv1alpha1.UserAccountStatusEmbedded{
			{
				Cluster: toolchainv1alpha1.Cluster{
					Name: commontest.Member2ClusterName,
				},
				UserAccountStatus: userAccount2.Status,
			},
			{
				Cluster: toolchainv1alpha1.Cluster{
					Name: "member3-cluster",
				},
				UserAccountStatus: userAccount3.Status,
			},
		}

		memberClient := commontest.NewFakeClient(t, userAccount)
		memberClient2 := commontest.NewFakeClient(t, userAccount2)
		memberClient3 := commontest.NewFakeClient(t, userAccount3)

		toolchainStatus := NewToolchainStatus(
			WithMember(commontest.MemberClusterName, WithRoutes("https://console.member-cluster/", "", ToBeReady())),
			WithMember(commontest.Member2ClusterName, WithRoutes("https://console.member2-cluster/", "", ToBeReady())),
			WithMember("member3-cluster", WithRoutes("https://console.member3-cluster/", "", ToBeReady())),
			WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
				"1,internal": 1,
			}),
			WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
				string(metrics.Internal): 1,
			}))
		hostClient := commontest.NewFakeClient(t, mur, toolchainStatus)
		InitializeCounters(t, toolchainStatus)

		cntrl := newController(hostClient, s, ClusterClient(commontest.MemberClusterName, memberClient), ClusterClient(commontest.Member2ClusterName, memberClient2),
			ClusterClient("member3-cluster", memberClient3))

		// when
		_, err := cntrl.Reconcile(context.TODO(), newMurRequest(mur))

		// then
		require.NoError(t, err)

		uatest.AssertThatUserAccount(t, "john", memberClient).
			Exists().
			MatchMasterUserRecord(mur).
			HasLabelWithValue(toolchainv1alpha1.TierLabelKey, "deactivate30").
			HasConditions(userAccount.Status.Conditions...)
		uatest.AssertThatUserAccount(t, "john", memberClient2).
			Exists().
			MatchMasterUserRecord(mur).
			HasLabelWithValue(toolchainv1alpha1.TierLabelKey, "deactivate30").
			HasConditions(userAccount2.Status.Conditions...)
		uatest.AssertThatUserAccount(t, "john", memberClient3).
			Exists().
			MatchMasterUserRecord(mur).
			HasLabelWithValue(toolchainv1alpha1.TierLabelKey, "deactivate30").
			HasConditions(userAccount3.Status.Conditions...)

		murtest.AssertThatMasterUserRecord(t, "john", hostClient).
			HasConditions(toBeNotReady(toolchainv1alpha1.MasterUserRecordProvisioningReason, "")).
			HasStatusUserAccounts(commontest.MemberClusterName, commontest.Member2ClusterName, "member3-cluster").
			AllUserAccountsHaveCondition(userAccount.Status.Conditions[0])
		AssertThatCountersAndMetrics(t).
			HaveUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
				"1,internal": 1, // unchanged
			}).
			HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
				string(metrics.Internal): 1, // unchanged
			})
	})

	t.Run("outdated mur status error cleaned", func(t *testing.T) {
		// given
		// A basic userSignup to set as the mur owner
		userSignup := commonsignup.NewUserSignup()
		userSignup.Status = toolchainv1alpha1.UserSignupStatus{
			CompliantUsername: "john",
		}

		// MUR with ready condition set to false with an error
		// all MUR.Status.UserAccount[] conditions are already in sync with the corresponding UserAccounts and set to Ready
		mur := murtest.NewMasterUserRecord(t, "john",
			murtest.Finalizer("finalizer.toolchain.dev.openshift.com"),
			murtest.StatusCondition(toBeNotReady(toolchainv1alpha1.MasterUserRecordTargetClusterNotReadyReason, "something went wrong")),
			murtest.AdditionalAccounts(commontest.Member2ClusterName))
		userAccount := uatest.NewUserAccountFromMur(mur, uatest.StatusCondition(toBeProvisioned()), uatest.ResourceVersion("123abc"))
		userAccount2 := uatest.NewUserAccountFromMur(mur, uatest.StatusCondition(toBeProvisioned()), uatest.ResourceVersion("123abc"))
		mur.Labels[toolchainv1alpha1.MasterUserRecordOwnerLabelKey] = userSignup.Name
		mur.Status.UserAccounts = []toolchainv1alpha1.UserAccountStatusEmbedded{
			{
				Cluster:           toolchainv1alpha1.Cluster{Name: commontest.MemberClusterName},
				UserAccountStatus: userAccount.Status,
			},
			{
				Cluster:           toolchainv1alpha1.Cluster{Name: commontest.Member2ClusterName},
				UserAccountStatus: userAccount2.Status,
			},
		}

		toolchainStatus := NewToolchainStatus(
			WithMember(commontest.MemberClusterName, WithRoutes("https://console.member-cluster/", "", ToBeReady())),
			WithMember(commontest.Member2ClusterName, WithRoutes("https://console.member2-cluster/", "", ToBeReady())),
			WithMember("member3-cluster", WithRoutes("https://console.member3-cluster/", "", ToBeReady())),
			WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
				"1,internal": 1,
			}),
			WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
				string(metrics.Internal): 1,
			}))
		hostClient := commontest.NewFakeClient(t, userSignup, mur, toolchainStatus)
		InitializeCounters(t, toolchainStatus)

		memberClient := commontest.NewFakeClient(t, userAccount)
		memberClient2 := commontest.NewFakeClient(t, uatest.NewUserAccountFromMur(mur))

		cntrl := newController(hostClient, s, ClusterClient(commontest.MemberClusterName, memberClient),
			ClusterClient(commontest.Member2ClusterName, memberClient2))

		// when
		_, err := cntrl.Reconcile(context.TODO(), newMurRequest(mur))

		// then
		// the original error status should be cleaned
		require.NoError(t, err)

		murtest.AssertThatMasterUserRecord(t, "john", hostClient).
			HasConditions(toBeProvisioned(), toBeProvisionedNotificationCreated()).
			HasStatusUserAccounts(commontest.MemberClusterName, commontest.Member2ClusterName).
			HasFinalizer()

		// Get the notification resource and verify it
		notifications := &toolchainv1alpha1.NotificationList{}
		err = hostClient.List(context.TODO(), notifications)
		require.NoError(t, err)
		require.Len(t, notifications.Items, 1)
		notification := notifications.Items[0]
		require.NoError(t, err)
		assert.Equal(t, "userprovisioned", notification.Spec.Template)
		assert.Contains(t, notification.Name, userAccount.Name+"-provisioned-")
		assert.True(t, len(notification.Name) > len(userAccount.Name+"-provisioned-"))
		assert.Equal(t, mur.Namespace, notification.Namespace)
		require.Equal(t, 1, len(notification.OwnerReferences))
		assert.Equal(t, "MasterUserRecord", notification.OwnerReferences[0].Kind)
		assert.Equal(t, mur.Name, notification.OwnerReferences[0].Name)
		AssertThatCountersAndMetrics(t).
			HaveUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
				"1,internal": 1, // unchanged
			}).
			HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
				string(metrics.Internal): 1, // unchanged
			})
	})
}

func TestDeleteUserAccountViaMasterUserRecordBeingDeleted(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		// given
		logf.SetLogger(zap.New(zap.UseDevMode(true)))
		s := apiScheme(t)
		mur := murtest.NewMasterUserRecord(t, "john",
			murtest.ToBeDeleted())
		userAcc := uatest.NewUserAccountFromMur(mur)

		memberClient := commontest.NewFakeClient(t, userAcc)
		hostClient := commontest.NewFakeClient(t, mur)
		InitializeCounters(t, NewToolchainStatus(
			WithMember(commontest.MemberClusterName),
			WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
				"1,internal": 1,
				"1,external": 1,
			}),
			WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
				string(metrics.Internal): 1,
				string(metrics.External): 1,
			})))

		cntrl := newController(hostClient, s, ClusterClient(commontest.MemberClusterName, memberClient))

		// when
		result1, err1 := cntrl.Reconcile(context.TODO(), newMurRequest(mur))
		require.NoError(t, err1)
		assert.True(t, result1.Requeue)
		assert.Equal(t, int64(result1.RequeueAfter), int64(10*time.Second))

		result2, err2 := cntrl.Reconcile(context.TODO(), newMurRequest(mur))

		// then
		require.Empty(t, result2)
		require.NoError(t, err2)

		uatest.AssertThatUserAccount(t, "john", memberClient).
			DoesNotExist()
		murtest.AssertThatMasterUserRecord(t, "john", hostClient).
			DoesNotExist()
		AssertThatCountersAndMetrics(t).
			HaveUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
				"1,internal": 1, // unchanged
				"1,external": 1, // unchanged
			}).
			HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
				string(metrics.Internal): 0, // decremented
				string(metrics.External): 1, // unchanged
			})
	})

	t.Run("test wait for UserAccount deletion in progress before removing MUR finalizer", func(t *testing.T) {
		// given
		logf.SetLogger(zap.New(zap.UseDevMode(true)))
		s := apiScheme(t)
		mur := murtest.NewMasterUserRecord(t, "john-wait-for-ua",
			murtest.ToBeDeleted())
		userAcc := uatest.NewUserAccountFromMur(mur)
		// set deletion timestamp to indicate UserAccount deletion is in progress
		userAcc.DeletionTimestamp = &metav1.Time{Time: time.Now().Add(-5 * time.Second)}

		hostClient := commontest.NewFakeClient(t, mur)
		memberClient := commontest.NewFakeClient(t, userAcc)
		InitializeCounters(t, NewToolchainStatus(
			WithMember(commontest.MemberClusterName),
			WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
				"1,internal": 1,
				"1,external": 1,
			}),
			WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
				string(metrics.Internal): 1,
				string(metrics.External): 1,
			})))

		cntrl := newController(hostClient, s, ClusterClient(commontest.MemberClusterName, memberClient))

		// when
		result1, err := cntrl.Reconcile(context.TODO(), newMurRequest(mur))
		require.NoError(t, err)
		assert.True(t, result1.Requeue)
		assert.Equal(t, int64(result1.RequeueAfter), int64(10*time.Second))

		err = memberClient.Delete(context.TODO(), userAcc)
		require.NoError(t, err)
		result2, err2 := cntrl.Reconcile(context.TODO(), newMurRequest(mur))

		// then
		require.Empty(t, result2)
		require.NoError(t, err2)
		uatest.AssertThatUserAccount(t, "john-wait-for-ua", memberClient).
			DoesNotExist()
		murtest.AssertThatMasterUserRecord(t, "john-wait-for-ua", hostClient).
			DoesNotExist()
		AssertThatCountersAndMetrics(t).
			HaveUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
				"1,internal": 1, // unchanged
				"1,external": 1, // unchanged
			}).
			HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
				string(metrics.Internal): 0, // decremented
				string(metrics.External): 1, // unchanged
			})
	})

	t.Run("test wait for UserAccount deletion takes too long", func(t *testing.T) {
		// given
		logf.SetLogger(zap.New(zap.UseDevMode(true)))
		s := apiScheme(t)

		// A basic userSignup to set as the mur owner
		userSignup := commonsignup.NewUserSignup()
		userSignup.Status = toolchainv1alpha1.UserSignupStatus{
			CompliantUsername: "john-wait-for-ua",
		}

		mur := murtest.NewMasterUserRecord(t, "john-wait-for-ua",
			murtest.ToBeDeleted())
		userAcc := uatest.NewUserAccountFromMur(mur)
		// set deletion timestamp to indicate UserAccount deletion is in progress
		userAcc.DeletionTimestamp = &metav1.Time{Time: time.Now().Add(-60 * time.Second)}

		hostClient := commontest.NewFakeClient(t, mur)
		memberClient := commontest.NewFakeClient(t, userAcc)
		InitializeCounters(t, NewToolchainStatus(
			WithMember(commontest.MemberClusterName),
			WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
				"1,internal": 1,
				"1,external": 1,
			}),
			WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
				string(metrics.Internal): 1,
				string(metrics.External): 1,
			})))

		cntrl := newController(hostClient, s, ClusterClient(commontest.MemberClusterName, memberClient))

		// when
		result, err := cntrl.Reconcile(context.TODO(), newMurRequest(mur))

		// then
		require.Empty(t, result)
		require.Error(t, err)
		require.Equal(t, `failed to delete UserAccount in the member cluster 'member-cluster': UserAccount deletion has not completed in over 1 minute`, err.Error())
		uatest.AssertThatUserAccount(t, "john-wait-for-ua", memberClient).
			Exists()
		murtest.AssertThatMasterUserRecord(t, "john-wait-for-ua", hostClient).
			HasFinalizer()
	})

	t.Run("test UserAccount deletion has foreground propagation policy", func(t *testing.T) {
		// given
		logf.SetLogger(zap.New(zap.UseDevMode(true)))
		s := apiScheme(t)
		mur := murtest.NewMasterUserRecord(t, "john-wait-for-ua",
			murtest.ToBeDeleted())
		userAcc := uatest.NewUserAccountFromMur(mur)

		hostClient := commontest.NewFakeClient(t, mur)
		memberClient := commontest.NewFakeClient(t, userAcc)

		cntrl := newController(hostClient, s, ClusterClient(commontest.MemberClusterName, memberClient))

		deleted := false
		memberClient.MockDelete = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.DeleteOption) error {
			deleted = true
			require.Len(t, opts, 1)
			deleteOptions, ok := opts[0].(*runtimeclient.DeleteOptions)
			require.True(t, ok)
			require.NotNil(t, deleteOptions)
			require.NotNil(t, deleteOptions.PropagationPolicy)
			assert.Equal(t, metav1.DeletePropagationForeground, *deleteOptions.PropagationPolicy)
			return nil
		}
		// when
		_, err := cntrl.Reconcile(context.TODO(), newMurRequest(mur))
		//then
		require.NoError(t, err)
		assert.True(t, deleted)
	})
}

func TestDeleteMultipleUserAccountsViaMasterUserRecordBeingDeleted(t *testing.T) {
	// given
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	s := apiScheme(t)
	mur := murtest.NewMasterUserRecord(t, "john",
		murtest.Finalizer("finalizer.toolchain.dev.openshift.com"),
		murtest.ToBeDeleted(), murtest.AdditionalAccounts(commontest.Member2ClusterName))
	userAcc := uatest.NewUserAccountFromMur(mur)

	memberClient := commontest.NewFakeClient(t, userAcc)
	memberClient2 := commontest.NewFakeClient(t, userAcc)
	hostClient := commontest.NewFakeClient(t, mur)
	InitializeCounters(t, NewToolchainStatus(
		WithMember(commontest.MemberClusterName),
		WithMember(commontest.Member2ClusterName),
		WithMember("member3-cluster"),
		WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
			"1,internal": 1,
			"1,external": 1,
		}),
		WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
			string(metrics.Internal): 1,
			string(metrics.External): 1,
		})))

	cntrl := newController(hostClient, s, ClusterClient(commontest.MemberClusterName, memberClient), ClusterClient(commontest.Member2ClusterName, memberClient2))

	// when
	result1, err1 := cntrl.Reconcile(context.TODO(), newMurRequest(mur)) // first reconcile will wait for first useraccount to be deleted
	require.NoError(t, err1)
	assert.True(t, result1.Requeue)
	assert.Equal(t, int64(result1.RequeueAfter), int64(10*time.Second))

	result2, err2 := cntrl.Reconcile(context.TODO(), newMurRequest(mur)) // second reconcile will wait for second useraccount to be deleted
	require.NoError(t, err2)
	assert.True(t, result2.Requeue)
	assert.Equal(t, int64(result2.RequeueAfter), int64(10*time.Second))

	result3, err3 := cntrl.Reconcile(context.TODO(), newMurRequest(mur))

	// then
	require.Empty(t, result3)
	require.NoError(t, err3)

	uatest.AssertThatUserAccount(t, "john", memberClient).
		DoesNotExist()
	uatest.AssertThatUserAccount(t, "john", memberClient2).
		DoesNotExist()
	murtest.AssertThatMasterUserRecord(t, "john", hostClient).
		DoesNotExist()
	AssertThatCountersAndMetrics(t).
		HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
			string(metrics.Internal): 0, // decremented
			string(metrics.External): 1, // unchanged
		})
}

func TestDisablingMasterUserRecord(t *testing.T) {
	// given
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	s := apiScheme(t)
	mur := murtest.NewMasterUserRecord(t, "john",
		murtest.Finalizer("finalizer.toolchain.dev.openshift.com"),
		murtest.DisabledMur(true))
	userAccount := uatest.NewUserAccountFromMur(mur, uatest.DisabledUa(false))
	memberClient := commontest.NewFakeClient(t, userAccount)
	toolchainStatus := NewToolchainStatus(
		WithMember(commontest.MemberClusterName, WithRoutes("https://console.member-cluster/", "", ToBeReady())),
		WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
			"1,internal": 1,
		}),
		WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
			string(metrics.Internal): 1,
		}))
	hostClient := commontest.NewFakeClient(t, mur, toolchainStatus)
	InitializeCounters(t, toolchainStatus)

	cntrl := newController(hostClient, s, ClusterClient(commontest.MemberClusterName, memberClient))

	// when
	res, err := cntrl.Reconcile(context.TODO(), newMurRequest(mur))
	require.NoError(t, err)
	assert.Equal(t, reconcile.Result{Requeue: false}, res)
	userAcc := &toolchainv1alpha1.UserAccount{}
	err = memberClient.Get(context.TODO(), types.NamespacedName{Name: mur.Name, Namespace: "toolchain-member-operator"}, userAcc)
	require.NoError(t, err)
	assert.True(t, userAcc.Spec.Disabled)
	AssertThatCountersAndMetrics(t).
		HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{string(metrics.Internal): 1}) // unchanged
}

func newMurRequest(mur *toolchainv1alpha1.MasterUserRecord) reconcile.Request {
	return reconcile.Request{
		NamespacedName: namespacedName(mur.ObjectMeta.Namespace, mur.ObjectMeta.Name),
	}
}

func apiScheme(t *testing.T) *runtime.Scheme {
	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)
	return s
}

func newController(hostCl runtimeclient.Client, s *runtime.Scheme, memberCl ...ClientForCluster) Reconciler {
	os.Setenv("WATCH_NAMESPACE", commontest.HostOperatorNs)
	r := Reconciler{
		Client:         hostCl,
		Scheme:         s,
		MemberClusters: map[string]cluster.Cluster{},
	}
	for _, c := range memberCl {
		name, cl := c()
		NewMemberClusterWithClient(cl, name, corev1.ConditionTrue)
		r.MemberClusters[name] = cluster.Cluster{
			Config: &commoncluster.Config{
				Type:              commoncluster.Member,
				OperatorNamespace: commontest.MemberOperatorNs,
				OwnerClusterName:  commontest.MemberClusterName,
			},
			Client: cl,
		}
	}
	return r
}
