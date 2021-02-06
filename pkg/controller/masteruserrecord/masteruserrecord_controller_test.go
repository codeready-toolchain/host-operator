package masteruserrecord

import (
	"context"
	"fmt"
	"github.com/gofrs/uuid"
	"testing"
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	"github.com/codeready-toolchain/host-operator/pkg/configuration"
	"github.com/codeready-toolchain/host-operator/pkg/counter"
	. "github.com/codeready-toolchain/host-operator/test"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	murtest "github.com/codeready-toolchain/toolchain-common/pkg/test/masteruserrecord"
	uatest "github.com/codeready-toolchain/toolchain-common/pkg/test/useraccount"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	apierros "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestAddFinalizer(t *testing.T) {
	// given
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	s := apiScheme(t)

	t.Run("ok", func(t *testing.T) {
		// given
		InitializeCounter(t, 1, UserAccountsForCluster(test.MemberClusterName, 1))
		defer counter.Reset()
		mur := murtest.NewMasterUserRecord(t, "john")
		memberClient := test.NewFakeClient(t)
		hostClient := test.NewFakeClient(t, mur)

		cntrl := newController(t, hostClient, s, NewGetMemberCluster(true, v1.ConditionTrue),
			ClusterClient(test.MemberClusterName, memberClient))

		// when
		result, err := cntrl.Reconcile(newMurRequest(mur))

		// then
		require.NoError(t, err)
		require.False(t, result.Requeue)

		murtest.AssertThatMasterUserRecord(t, "john", hostClient).
			HasConditions(toBeNotReady(toolchainv1alpha1.MasterUserRecordProvisioningReason, "")).
			HasFinalizer()
		AssertThatCounterHas(t, 1, UserAccountsForCluster(test.MemberClusterName, 2))
	})

	t.Run("fails because it cannot add finalizer", func(t *testing.T) {
		// given
		defer counter.Reset()
		InitializeCounter(t, 1, UserAccountsForCluster(test.MemberClusterName, 1))
		mur := murtest.NewMasterUserRecord(t, "john")
		hostClient := test.NewFakeClient(t, mur)
		memberClient := test.NewFakeClient(t)
		hostClient.MockUpdate = func(ctx context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
			return fmt.Errorf("unable to add finalizer to MUR %s", mur.Name)
		}

		cntrl := newController(t, hostClient, s, NewGetMemberCluster(true, v1.ConditionTrue),
			ClusterClient(test.MemberClusterName, memberClient))

		// when
		_, err := cntrl.Reconcile(newMurRequest(mur))

		// then
		require.Error(t, err)
		msg := "failed while updating with added finalizer: unable to add finalizer to MUR john"
		assert.Contains(t, err.Error(), msg)

		uatest.AssertThatUserAccount(t, "john", memberClient).DoesNotExist()
		murtest.AssertThatMasterUserRecord(t, "john", hostClient).
			HasConditions(toBeNotReady(toolchainv1alpha1.MasterUserRecordUnableToAddFinalizerReason, "unable to add finalizer to MUR john"))
		AssertThatCounterHas(t, 1, UserAccountsForCluster(test.MemberClusterName, 1))
	})
}

func TestCreateUserAccountSuccessful(t *testing.T) {
	// given
	defer counter.Reset()
	InitializeCounter(t, 1, UserAccountsForCluster(test.MemberClusterName, 1))
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	s := apiScheme(t)
	mur := murtest.NewMasterUserRecord(t, "john")
	require.NoError(t, murtest.Modify(mur, murtest.Finalizer("finalizer.toolchain.dev.openshift.com")))
	memberClient := test.NewFakeClient(t)
	hostClient := test.NewFakeClient(t, mur)

	cntrl := newController(t, hostClient, s, NewGetMemberCluster(true, v1.ConditionTrue),
		ClusterClient(test.MemberClusterName, memberClient))

	// when
	_, err := cntrl.Reconcile(newMurRequest(mur))

	// then
	require.NoError(t, err)
	uatest.AssertThatUserAccount(t, "john", memberClient).
		Exists().
		MatchMasterUserRecord(mur, mur.Spec.UserAccounts[0].Spec)
	murtest.AssertThatMasterUserRecord(t, "john", hostClient).
		HasConditions(toBeNotReady(toolchainv1alpha1.MasterUserRecordProvisioningReason, "")).
		HasFinalizer()
	AssertThatCounterHas(t, 1, UserAccountsForCluster(test.MemberClusterName, 2))
}

func TestCreateMultipleUserAccountsSuccessful(t *testing.T) {
	// given
	defer counter.Reset()
	InitializeCounter(t, 1, UserAccountsForCluster(test.MemberClusterName, 1))
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	s := apiScheme(t)
	mur := murtest.NewMasterUserRecord(t, "john", murtest.AdditionalAccounts("member2-cluster"), murtest.Finalizer("finalizer.toolchain.dev.openshift.com"))
	toolchainStatus := NewToolchainStatus(
		WithMember(test.MemberClusterName, WithRoutes("https://console.member-cluster/", "", ToBeReady())),
		WithMember("member2-cluster", WithRoutes("https://console.member-cluster/", "", ToBeReady())))
	memberClient := test.NewFakeClient(t)
	memberClient2 := test.NewFakeClient(t)
	hostClient := test.NewFakeClient(t, mur, toolchainStatus)

	cntrl := newController(t, hostClient, s, NewGetMemberCluster(true, v1.ConditionTrue),
		ClusterClient(test.MemberClusterName, memberClient), ClusterClient("member2-cluster", memberClient2))

	// when reconciling
	result, err := cntrl.Reconcile(newMurRequest(mur))
	// then
	require.NoError(t, err)
	assert.False(t, result.Requeue)
	uatest.AssertThatUserAccount(t, "john", memberClient).
		Exists().
		MatchMasterUserRecord(mur, mur.Spec.UserAccounts[0].Spec)
	uatest.AssertThatUserAccount(t, "john", memberClient2).
		Exists().
		MatchMasterUserRecord(mur, mur.Spec.UserAccounts[1].Spec)
	murtest.AssertThatMasterUserRecord(t, "john", hostClient).
		HasConditions(toBeNotReady(toolchainv1alpha1.MasterUserRecordProvisioningReason, "")).
		HasFinalizer()
	AssertThatCounterHas(t, 1, UserAccountsForCluster(test.MemberClusterName, 2), UserAccountsForCluster("member2-cluster", 1))
}

func TestRequeueWhenUserAccountDeleted(t *testing.T) {
	// given
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	s := apiScheme(t)
	mur := murtest.NewMasterUserRecord(t, "john", murtest.AdditionalAccounts("member2-cluster"), murtest.Finalizer("finalizer.toolchain.dev.openshift.com"))
	userAccount1 := uatest.NewUserAccountFromMur(mur)
	userAccount3 := uatest.NewUserAccountFromMur(mur)
	toolchainStatus := NewToolchainStatus(
		WithMember(test.MemberClusterName, WithRoutes("https://console.member-cluster/", "", ToBeReady())),
		WithMember("member2-cluster", WithRoutes("https://console.member-cluster/", "", ToBeReady())),
		WithMember("member3-cluster", WithRoutes("https://console.member-cluster/", "", ToBeReady())))
	memberClient1 := test.NewFakeClient(t, userAccount1)
	memberClient3 := test.NewFakeClient(t, userAccount3)
	hostClient := test.NewFakeClient(t, mur, toolchainStatus)

	t.Run("when deletion timestamp is less than 3 seconds old", func(t *testing.T) {
		// given
		defer counter.Reset()
		InitializeCounter(t, 1, UserAccountsForCluster(test.MemberClusterName, 2), UserAccountsForCluster("member2-cluster", 2))
		userAccount2 := uatest.NewUserAccountFromMur(mur, uatest.DeletedUa())
		memberClient2 := test.NewFakeClient(t, userAccount2)
		cntrl := newController(t, hostClient, s, NewGetMemberCluster(true, v1.ConditionTrue),
			ClusterClient(test.MemberClusterName, memberClient1),
			ClusterClient("member2-cluster", memberClient2),
			ClusterClient("member3-cluster", memberClient3))

		// when
		result, err := cntrl.Reconcile(newMurRequest(mur))

		// then
		require.NoError(t, err)
		assert.True(t, result.Requeue)
		assert.Equal(t, 3*time.Second, result.RequeueAfter)
		AssertThatCounterHas(t, 1, UserAccountsForCluster(test.MemberClusterName, 2), UserAccountsForCluster("member2-cluster", 1))
	})

	t.Run("when deletion timestamp is more than 3 seconds old", func(t *testing.T) {
		// given
		defer counter.Reset()
		InitializeCounter(t, 1, UserAccountsForCluster(test.MemberClusterName, 2), UserAccountsForCluster("member2-cluster", 2))
		userAccount2 := uatest.NewUserAccountFromMur(mur, uatest.DeletedUa())
		userAccount2.DeletionTimestamp = &metav1.Time{Time: time.Now().Add(-3 * time.Second)}
		memberClient2 := test.NewFakeClient(t, userAccount2)
		cntrl := newController(t, hostClient, s, NewGetMemberCluster(true, v1.ConditionTrue),
			ClusterClient(test.MemberClusterName, memberClient1),
			ClusterClient("member2-cluster", memberClient2),
			ClusterClient("member3-cluster", memberClient3))

		// when
		result, err := cntrl.Reconcile(newMurRequest(mur))

		// then
		require.NoError(t, err)
		assert.True(t, result.Requeue)
		assert.Equal(t, 3*time.Second, result.RequeueAfter)
		AssertThatCounterHas(t, 1, UserAccountsForCluster(test.MemberClusterName, 2), UserAccountsForCluster("member2-cluster", 2))
	})

	t.Run("when deletion timestamp is in the future", func(t *testing.T) {
		// given
		defer counter.Reset()
		InitializeCounter(t, 1, UserAccountsForCluster(test.MemberClusterName, 2), UserAccountsForCluster("member2-cluster", 2))
		userAccount2 := uatest.NewUserAccountFromMur(mur, uatest.DeletedUa())
		userAccount2.DeletionTimestamp = &metav1.Time{Time: time.Now().Add(2 * time.Second)}
		memberClient2 := test.NewFakeClient(t, userAccount2)
		cntrl := newController(t, hostClient, s, NewGetMemberCluster(true, v1.ConditionTrue),
			ClusterClient(test.MemberClusterName, memberClient1),
			ClusterClient("member2-cluster", memberClient2),
			ClusterClient("member3-cluster", memberClient3))

		// when
		result, err := cntrl.Reconcile(newMurRequest(mur))

		// then
		require.NoError(t, err)
		assert.True(t, result.Requeue)
		assert.Greater(t, int64(result.RequeueAfter), int64(3*time.Second))
		AssertThatCounterHas(t, 1, UserAccountsForCluster(test.MemberClusterName, 2), UserAccountsForCluster("member2-cluster", 1))
	})
}

func TestCreateSynchronizeOrDeleteUserAccountFailed(t *testing.T) {
	// given
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	s := apiScheme(t)
	mur := murtest.NewMasterUserRecord(t, "john", murtest.Finalizer("finalizer.toolchain.dev.openshift.com"))
	hostClient := test.NewFakeClient(t, mur)

	t.Run("when member cluster does not exist and UA hasn't been created yet", func(t *testing.T) {
		// given
		defer counter.Reset()
		InitializeCounter(t, 1, UserAccountsForCluster(test.MemberClusterName, 1))
		memberClient := test.NewFakeClient(t)

		cntrl := newController(t, hostClient, s, NewGetMemberCluster(false, v1.ConditionTrue),
			ClusterClient(test.MemberClusterName, memberClient))

		// when
		_, err := cntrl.Reconcile(newMurRequest(mur))

		// then
		require.Error(t, err)
		msg := "the member cluster member-cluster not found in the registry"
		assert.Contains(t, err.Error(), msg)

		uatest.AssertThatUserAccount(t, "john", memberClient).DoesNotExist()
		murtest.AssertThatMasterUserRecord(t, "john", hostClient).
			HasConditions(toBeNotReady(toolchainv1alpha1.MasterUserRecordTargetClusterNotReadyReason, msg)).
			HasFinalizer()
		AssertThatCounterHas(t, 1, UserAccountsForCluster(test.MemberClusterName, 1))
	})

	t.Run("when member cluster does not exist and UA was already created", func(t *testing.T) {
		// given
		defer counter.Reset()
		InitializeCounter(t, 1, UserAccountsForCluster(test.MemberClusterName, 1))
		memberClient := test.NewFakeClient(t, uatest.NewUserAccountFromMur(mur))

		cntrl := newController(t, hostClient, s, NewGetMemberCluster(false, v1.ConditionTrue),
			ClusterClient(test.MemberClusterName, memberClient))

		// when
		_, err := cntrl.Reconcile(newMurRequest(mur))

		// then
		require.Error(t, err)
		msg := "the member cluster member-cluster not found in the registry"
		assert.Contains(t, err.Error(), msg)
		murtest.AssertThatMasterUserRecord(t, "john", hostClient).
			HasConditions(toBeNotReady(toolchainv1alpha1.MasterUserRecordTargetClusterNotReadyReason, msg)).
			HasFinalizer()
		AssertThatCounterHas(t, 1, UserAccountsForCluster(test.MemberClusterName, 1))
	})

	t.Run("when member cluster is not ready and UA hasn't been created yet", func(t *testing.T) {
		// given
		defer counter.Reset()
		InitializeCounter(t, 1, UserAccountsForCluster(test.MemberClusterName, 1))
		memberClient := test.NewFakeClient(t)

		cntrl := newController(t, hostClient, s, NewGetMemberCluster(true, v1.ConditionFalse),
			ClusterClient(test.MemberClusterName, memberClient))

		// when
		_, err := cntrl.Reconcile(newMurRequest(mur))

		// then
		require.Error(t, err)
		msg := "the member cluster member-cluster is not ready"
		assert.Contains(t, err.Error(), msg)

		uatest.AssertThatUserAccount(t, "john", memberClient).DoesNotExist()
		murtest.AssertThatMasterUserRecord(t, "john", hostClient).
			HasConditions(toBeNotReady(toolchainv1alpha1.MasterUserRecordTargetClusterNotReadyReason, msg)).
			HasFinalizer()
		AssertThatCounterHas(t, 1, UserAccountsForCluster(test.MemberClusterName, 1))
	})

	t.Run("when member cluster is not ready and UA was already created", func(t *testing.T) {
		// given
		defer counter.Reset()
		InitializeCounter(t, 1, UserAccountsForCluster(test.MemberClusterName, 1))
		memberClient := test.NewFakeClient(t, uatest.NewUserAccountFromMur(mur))

		cntrl := newController(t, hostClient, s, NewGetMemberCluster(true, v1.ConditionFalse),
			ClusterClient(test.MemberClusterName, memberClient))

		// when
		_, err := cntrl.Reconcile(newMurRequest(mur))

		// then
		require.Error(t, err)
		msg := "the member cluster member-cluster is not ready"
		assert.Contains(t, err.Error(), msg)

		murtest.AssertThatMasterUserRecord(t, "john", hostClient).
			HasConditions(toBeNotReady(toolchainv1alpha1.MasterUserRecordTargetClusterNotReadyReason, msg)).
			HasFinalizer()
		AssertThatCounterHas(t, 1, UserAccountsForCluster(test.MemberClusterName, 1))
	})

	t.Run("status update of the MasterUserRecord failed", func(t *testing.T) {
		// given
		defer counter.Reset()
		InitializeCounter(t, 1, UserAccountsForCluster(test.MemberClusterName, 1))
		memberClient := test.NewFakeClient(t)

		cntrl := newController(t, hostClient, s, NewGetMemberCluster(true, v1.ConditionTrue),
			ClusterClient(test.MemberClusterName, memberClient))
		statusUpdater := func(logger logr.Logger, mur *toolchainv1alpha1.MasterUserRecord, message string) error {
			return fmt.Errorf("unable to update status")
		}

		// when
		err := cntrl.wrapErrorWithStatusUpdate(log, mur, statusUpdater,
			apierros.NewBadRequest("oopsy woopsy"), "failed to create %s", "user bob")

		// then
		require.Error(t, err)
		assert.Equal(t, "failed to create user bob: oopsy woopsy", err.Error())
		AssertThatCounterHas(t, 1, UserAccountsForCluster(test.MemberClusterName, 1))
	})

	t.Run("creation of the UserAccount failed", func(t *testing.T) {
		// given
		defer counter.Reset()
		InitializeCounter(t, 1, UserAccountsForCluster(test.MemberClusterName, 1))
		memberClient := test.NewFakeClient(t)
		memberClient.MockCreate = func(ctx context.Context, obj runtime.Object, opts ...client.CreateOption) error {
			return fmt.Errorf("unable to create user account %s", mur.Name)
		}

		cntrl := newController(t, hostClient, s, NewGetMemberCluster(true, v1.ConditionTrue),
			ClusterClient(test.MemberClusterName, memberClient))

		// when
		_, err := cntrl.Reconcile(newMurRequest(mur))

		// then
		require.Error(t, err)
		msg := "failed to create UserAccount in the member cluster 'member-cluster'"
		assert.Contains(t, err.Error(), msg)

		uatest.AssertThatUserAccount(t, "john", memberClient).DoesNotExist()
		murtest.AssertThatMasterUserRecord(t, "john", hostClient).
			HasConditions(toBeNotReady(toolchainv1alpha1.MasterUserRecordUnableToCreateUserAccountReason, "unable to create user account john"))
		AssertThatCounterHas(t, 1, UserAccountsForCluster(test.MemberClusterName, 1))
	})

	t.Run("spec synchronization of the UserAccount failed", func(t *testing.T) {
		// given
		defer counter.Reset()
		InitializeCounter(t, 1, UserAccountsForCluster(test.MemberClusterName, 1))
		userAcc := uatest.NewUserAccountFromMur(mur)
		memberClient := test.NewFakeClient(t, userAcc)
		memberClient.MockUpdate = func(ctx context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
			return fmt.Errorf("unable to update user account %s", mur.Name)
		}
		modifiedMur := murtest.NewMasterUserRecord(t, "john", murtest.Finalizer("finalizer.toolchain.dev.openshift.com"))
		murtest.ModifyUaInMur(modifiedMur, test.MemberClusterName, murtest.TierName("admin"))
		hostClient := test.NewFakeClient(t, modifiedMur)

		cntrl := newController(t, hostClient, s, NewGetMemberCluster(true, v1.ConditionTrue),
			ClusterClient(test.MemberClusterName, memberClient))

		// when
		_, err := cntrl.Reconcile(newMurRequest(modifiedMur))

		// then
		require.Error(t, err)
		msg := "unable to update user account john"
		assert.Contains(t, err.Error(), msg)

		uatest.AssertThatUserAccount(t, "john", memberClient).
			Exists().
			HasSpec(userAcc.Spec)
		murtest.AssertThatMasterUserRecord(t, "john", hostClient).
			HasConditions(toBeNotReady(toolchainv1alpha1.MasterUserRecordUnableToSynchronizeUserAccountSpecReason, "unable to update user account john"))
		AssertThatCounterHas(t, 1, UserAccountsForCluster(test.MemberClusterName, 1))
	})

	t.Run("status synchronization between UserAccount and MasterUserRecord failed", func(t *testing.T) {
		// given
		defer counter.Reset()
		InitializeCounter(t, 1, UserAccountsForCluster(test.MemberClusterName, 1))
		updatingCond := toBeNotReady("updating", "")
		provisionedMur := murtest.NewMasterUserRecord(t, "john",
			murtest.Finalizer("finalizer.toolchain.dev.openshift.com"),
			murtest.StatusCondition(updatingCond))
		memberClient := test.NewFakeClient(t, uatest.NewUserAccountFromMur(provisionedMur,
			uatest.StatusCondition(toBeNotReady("somethingFailed", ""))))
		toolchainStatus := NewToolchainStatus(
			WithMember(test.MemberClusterName, WithRoutes("https://console.member-cluster/", "", ToBeReady())))
		hostClient := test.NewFakeClient(t, provisionedMur, toolchainStatus)

		hostClient.MockStatusUpdate = func(ctx context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
			hostClient.MockStatusUpdate = nil // mock only once
			return fmt.Errorf("unable to update MUR %s", provisionedMur.Name)
		}

		cntrl := newController(t, hostClient, s, NewGetMemberCluster(true, v1.ConditionTrue),
			ClusterClient(test.MemberClusterName, memberClient))

		// when
		_, err := cntrl.Reconcile(newMurRequest(provisionedMur))

		// then
		require.Error(t, err)
		msg := "update of the MasterUserRecord failed while synchronizing with UserAccount status from the cluster 'member-cluster'"
		assert.Contains(t, err.Error(), msg)

		uatest.AssertThatUserAccount(t, "john", memberClient).Exists()
		updatingCond.Message = msg + ": unable to update MUR john"
		murtest.AssertThatMasterUserRecord(t, "john", hostClient).
			HasConditions(updatingCond).
			HasStatusUserAccounts()
		AssertThatCounterHas(t, 1, UserAccountsForCluster(test.MemberClusterName, 1))
	})

	t.Run("deletion of MasterUserRecord fails because it cannot remove finalizer", func(t *testing.T) {
		// given
		defer counter.Reset()
		InitializeCounter(t, 1, UserAccountsForCluster(test.MemberClusterName, 2))
		mur := murtest.NewMasterUserRecord(t, "john",
			murtest.Finalizer("finalizer.toolchain.dev.openshift.com"),
			murtest.ToBeDeleted())

		hostClient := test.NewFakeClient(t, mur)
		memberClient := test.NewFakeClient(t, uatest.NewUserAccountFromMur(mur))
		hostClient.MockUpdate = func(ctx context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
			return fmt.Errorf("unable to remove finalizer from MUR %s", mur.Name)
		}

		cntrl := newController(t, hostClient, s, NewGetMemberCluster(true, v1.ConditionTrue),
			ClusterClient(test.MemberClusterName, memberClient))

		// when
		result1, err1 := cntrl.Reconcile(newMurRequest(mur)) // first reconcile will be requeued to wait for UserAccount deletion
		require.NoError(t, err1)
		assert.True(t, result1.Requeue)
		assert.Equal(t, int64(result1.RequeueAfter), int64(10*time.Second))

		result2, err2 := cntrl.Reconcile(newMurRequest(mur)) // second reconcile

		// then
		require.Empty(t, result2)
		require.Error(t, err2)
		msg := "failed to update MasterUserRecord while deleting finalizer"
		assert.Contains(t, err2.Error(), msg)

		uatest.AssertThatUserAccount(t, "john", memberClient).DoesNotExist()
		murtest.AssertThatMasterUserRecord(t, "john", hostClient).
			HasConditions(toBeNotReady(toolchainv1alpha1.MasterUserRecordUnableToRemoveFinalizerReason, "unable to remove finalizer from MUR john"))
		AssertThatCounterHas(t, 1, UserAccountsForCluster(test.MemberClusterName, 1))
	})

	t.Run("deletion of the UserAccount failed", func(t *testing.T) {
		// given
		defer counter.Reset()
		InitializeCounter(t, 1, UserAccountsForCluster(test.MemberClusterName, 1))
		mur := murtest.NewMasterUserRecord(t, "john",
			murtest.Finalizer("finalizer.toolchain.dev.openshift.com"),
			murtest.ToBeDeleted())
		hostClient := test.NewFakeClient(t, mur)

		memberClient := test.NewFakeClient(t, uatest.NewUserAccountFromMur(mur))
		memberClient.MockDelete = func(ctx context.Context, obj runtime.Object, opts ...client.DeleteOption) error {
			return fmt.Errorf("unable to delete user account %s", mur.Name)
		}

		cntrl := newController(t, hostClient, s, NewGetMemberCluster(true, v1.ConditionTrue),
			ClusterClient(test.MemberClusterName, memberClient))

		// when
		_, err := cntrl.Reconcile(newMurRequest(mur))

		// then
		require.Error(t, err)
		msg := "failed to delete UserAccount in the member cluster 'member-cluster'"
		assert.Contains(t, err.Error(), msg)

		uatest.AssertThatUserAccount(t, "john", memberClient).Exists()
		murtest.AssertThatMasterUserRecord(t, "john", hostClient).
			HasConditions(toBeNotReady(toolchainv1alpha1.MasterUserRecordUnableToDeleteUserAccountsReason, "unable to delete user account john")).
			HasFinalizer()
		AssertThatCounterHas(t, 1, UserAccountsForCluster(test.MemberClusterName, 1))
	})
}

func TestModifyUserAccounts(t *testing.T) {
	// given
	defer counter.Reset()
	InitializeCounter(t, 1,
		UserAccountsForCluster(test.MemberClusterName, 1),
		UserAccountsForCluster("member2-cluster", 1),
		UserAccountsForCluster("member3-cluster", 1))
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	s := apiScheme(t)
	mur := murtest.NewMasterUserRecord(t, "john",
		murtest.Finalizer("finalizer.toolchain.dev.openshift.com"),
		murtest.StatusCondition(toBeProvisioned()),
		murtest.AdditionalAccounts("member2-cluster", "member3-cluster"))

	userAccount := uatest.NewUserAccountFromMur(mur)
	userAccount2 := uatest.NewUserAccountFromMur(mur)
	userAccount3 := uatest.NewUserAccountFromMur(mur)

	murtest.ModifyUaInMur(mur, test.MemberClusterName, murtest.NsLimit("advanced"), murtest.TierName("admin"), murtest.Namespace("ide", "54321"))
	murtest.ModifyUaInMur(mur, "member2-cluster", murtest.NsLimit("admin"), murtest.TierName("basic"))

	toolchainStatus := NewToolchainStatus(
		WithMember(test.MemberClusterName, WithRoutes("https://console.member-cluster/", "", ToBeReady())),
		WithMember("member2-cluster", WithRoutes("https://console.member-cluster/", "", ToBeReady())),
		WithMember("member3-cluster", WithRoutes("https://console.member-cluster/", "", ToBeReady())))

	memberClient := test.NewFakeClient(t, userAccount)
	memberClient2 := test.NewFakeClient(t, userAccount2)
	memberClient3 := test.NewFakeClient(t, userAccount3)
	hostClient := test.NewFakeClient(t, mur, toolchainStatus)

	cntrl := newController(t, hostClient, s, NewGetMemberCluster(true, v1.ConditionTrue),
		ClusterClient(test.MemberClusterName, memberClient), ClusterClient("member2-cluster", memberClient2),
		ClusterClient("member3-cluster", memberClient3))

	// when ensuring 1st account
	_, err := cntrl.Reconcile(newMurRequest(mur))
	// then
	require.NoError(t, err)
	uatest.AssertThatUserAccount(t, "john", memberClient).
		Exists().
		MatchMasterUserRecord(mur, mur.Spec.UserAccounts[0].Spec)

	// when ensuring 2nd account
	_, err = cntrl.Reconcile(newMurRequest(mur))
	// then
	require.NoError(t, err)
	uatest.AssertThatUserAccount(t, "john", memberClient2).
		Exists().
		MatchMasterUserRecord(mur, mur.Spec.UserAccounts[1].Spec)

	// when ensuring 3rd account
	_, err = cntrl.Reconcile(newMurRequest(mur))
	// then
	require.NoError(t, err)
	uatest.AssertThatUserAccount(t, "john", memberClient3).
		Exists().
		MatchMasterUserRecord(mur, mur.Spec.UserAccounts[2].Spec)
	murtest.AssertThatMasterUserRecord(t, "john", hostClient).
		HasConditions(toBeNotReady(toolchainv1alpha1.MasterUserRecordUpdatingReason, ""))
	AssertThatCounterHas(t, 1,
		UserAccountsForCluster(test.MemberClusterName, 1),
		UserAccountsForCluster("member2-cluster", 1),
		UserAccountsForCluster("member3-cluster", 1))
}

func TestSyncMurStatusWithUserAccountStatuses(t *testing.T) {
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	s := apiScheme(t)
	toolchainStatus := NewToolchainStatus(
		WithMember(test.MemberClusterName, WithRoutes("https://console.member-cluster/", "", ToBeReady())),
		WithMember("member2-cluster", WithRoutes("https://console.member-cluster/", "", ToBeReady())),
		WithMember("member3-cluster", WithRoutes("https://console.member-cluster/", "", ToBeReady())))

	t.Run("mur status synced with updated user account statuses", func(t *testing.T) {
		// given
		defer counter.Reset()
		InitializeCounter(t, 1, UserAccountsForCluster(test.MemberClusterName, 1))
		// setup MUR that wil contain UserAccountStatusEmbedded fields for UserAccounts from "member2-cluster" and "member3-cluster" but will miss from test.MemberClusterName
		// then the reconcile should add the misssing UserAccountStatusEmbedded for the missing test.MemberClusterName cluster without updating anything else
		mur := murtest.NewMasterUserRecord(t, "john",
			murtest.Finalizer("finalizer.toolchain.dev.openshift.com"),
			murtest.StatusCondition(toBeNotReady(toolchainv1alpha1.MasterUserRecordProvisioningReason, "")),
			murtest.AdditionalAccounts("member2-cluster", "member3-cluster"))

		userAccount := uatest.NewUserAccountFromMur(mur,
			uatest.StatusCondition(toBeNotReady("Provisioning", "")), uatest.ResourceVersion("123abc"))
		userAccount2 := uatest.NewUserAccountFromMur(mur,
			uatest.StatusCondition(toBeNotReady("Provisioning", "")), uatest.ResourceVersion("123abc"))
		userAccount3 := uatest.NewUserAccountFromMur(mur,
			uatest.StatusCondition(toBeNotReady("Provisioning", "")), uatest.ResourceVersion("123abc"))

		mur.Status.UserAccounts = []toolchainv1alpha1.UserAccountStatusEmbedded{
			{
				SyncIndex: "111aaa",
				Cluster: toolchainv1alpha1.Cluster{
					Name: "member2-cluster",
				},
				UserAccountStatus: userAccount2.Status,
			},
			{
				SyncIndex: "123abc",
				Cluster: toolchainv1alpha1.Cluster{
					Name: "member3-cluster",
				},
				UserAccountStatus: userAccount3.Status,
			},
		}

		memberClient := test.NewFakeClient(t, userAccount)
		memberClient2 := test.NewFakeClient(t, userAccount2)
		memberClient3 := test.NewFakeClient(t, userAccount3)

		hostClient := test.NewFakeClient(t, mur, toolchainStatus)

		cntrl := newController(t, hostClient, s, NewGetMemberCluster(true, v1.ConditionTrue),
			ClusterClient(test.MemberClusterName, memberClient), ClusterClient("member2-cluster", memberClient2),
			ClusterClient("member3-cluster", memberClient3))

		// when
		_, err := cntrl.Reconcile(newMurRequest(mur))

		// then
		require.NoError(t, err)

		uatest.AssertThatUserAccount(t, "john", memberClient).
			Exists().
			MatchMasterUserRecord(mur, mur.Spec.UserAccounts[0].Spec).
			HasConditions(userAccount.Status.Conditions...)
		uatest.AssertThatUserAccount(t, "john", memberClient2).
			Exists().
			MatchMasterUserRecord(mur, mur.Spec.UserAccounts[1].Spec).
			HasConditions(userAccount2.Status.Conditions...)
		uatest.AssertThatUserAccount(t, "john", memberClient3).
			Exists().
			MatchMasterUserRecord(mur, mur.Spec.UserAccounts[2].Spec).
			HasConditions(userAccount3.Status.Conditions...)

		murtest.AssertThatMasterUserRecord(t, "john", hostClient).
			HasConditions(toBeNotReady(toolchainv1alpha1.MasterUserRecordProvisioningReason, "")).
			HasStatusUserAccounts(test.MemberClusterName, "member2-cluster", "member3-cluster").
			AllUserAccountsHaveStatusSyncIndex("123abc").
			AllUserAccountsHaveCondition(userAccount.Status.Conditions[0])
		AssertThatCounterHas(t, 1, UserAccountsForCluster(test.MemberClusterName, 1))
	})

	t.Run("outdated mur status error cleaned", func(t *testing.T) {
		// given
		defer counter.Reset()
		InitializeCounter(t, 1, UserAccountsForCluster(test.MemberClusterName, 1))
		// MUR with ready condition set to false with an error
		// all MUR.Status.UserAccount[] conditions are already in sync with the corresponding UserAccounts and set to Ready
		mur := murtest.NewMasterUserRecord(t, "john",
			murtest.Finalizer("finalizer.toolchain.dev.openshift.com"),
			murtest.StatusCondition(toBeNotReady(toolchainv1alpha1.MasterUserRecordTargetClusterNotReadyReason, "something went wrong")),
			murtest.AdditionalAccounts("member2-cluster"))
		userAccount := uatest.NewUserAccountFromMur(mur, uatest.StatusCondition(toBeProvisioned()), uatest.ResourceVersion("123abc"))
		userAccount2 := uatest.NewUserAccountFromMur(mur, uatest.StatusCondition(toBeProvisioned()), uatest.ResourceVersion("123abc"))
		owner, err := uuid.NewV4()
		require.NoError(t, err)
		mur.Labels[toolchainv1alpha1.MasterUserRecordOwnerLabelKey] = owner.String()
		mur.Status.UserAccounts = []toolchainv1alpha1.UserAccountStatusEmbedded{
			{
				Cluster:           toolchainv1alpha1.Cluster{Name: test.MemberClusterName},
				SyncIndex:         userAccount.ResourceVersion,
				UserAccountStatus: userAccount.Status,
			},
			{
				Cluster:           toolchainv1alpha1.Cluster{Name: "member2-cluster"},
				SyncIndex:         userAccount2.ResourceVersion,
				UserAccountStatus: userAccount2.Status,
			},
		}

		hostClient := test.NewFakeClient(t, mur, toolchainStatus)

		memberClient := test.NewFakeClient(t, userAccount)
		memberClient2 := test.NewFakeClient(t, uatest.NewUserAccountFromMur(mur))

		cntrl := newController(t, hostClient, s, NewGetMemberCluster(true, v1.ConditionTrue),
			ClusterClient(test.MemberClusterName, memberClient),
			ClusterClient("member2-cluster", memberClient2))

		// when
		_, err = cntrl.Reconcile(newMurRequest(mur))

		// then
		// the original error status should be cleaned
		require.NoError(t, err)

		murtest.AssertThatMasterUserRecord(t, "john", hostClient).
			HasConditions(toBeProvisioned(), toBeProvisionedNotificationCreated()).
			HasStatusUserAccounts(test.MemberClusterName, "member2-cluster").
			HasFinalizer()

		// Get the notification resource and verify it
		notifications := &toolchainv1alpha1.NotificationList{}
		err = hostClient.List(context.TODO(), notifications)
		require.NoError(t, err)
		require.Len(t, notifications.Items, 1)
		notification := notifications.Items[0]
		require.NoError(t, err)
		assert.Equal(t, owner.String(), notification.Spec.UserID)
		assert.Equal(t, "userprovisioned", notification.Spec.Template)
		assert.Contains(t, notification.Name, userAccount.Name+"-provisioned-")
		assert.True(t, len(notification.Name) > len(userAccount.Name+"-provisioned-"))
		assert.Equal(t, mur.Namespace, notification.Namespace)
		require.Equal(t, 1, len(notification.OwnerReferences))
		assert.Equal(t, "MasterUserRecord", notification.OwnerReferences[0].Kind)
		assert.Equal(t, mur.Name, notification.OwnerReferences[0].Name)
		AssertThatCounterHas(t, 1, UserAccountsForCluster(test.MemberClusterName, 1))
	})
}

func TestDeleteUserAccountViaMasterUserRecordBeingDeleted(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		// given
		defer counter.Reset()
		InitializeCounter(t, 2, UserAccountsForCluster(test.MemberClusterName, 2))
		logf.SetLogger(zap.New(zap.UseDevMode(true)))
		s := apiScheme(t)
		mur := murtest.NewMasterUserRecord(t, "john",
			murtest.ToBeDeleted())
		userAcc := uatest.NewUserAccountFromMur(mur)

		memberClient := test.NewFakeClient(t, userAcc)
		hostClient := test.NewFakeClient(t, mur)

		cntrl := newController(t, hostClient, s, NewGetMemberCluster(true, v1.ConditionTrue),
			ClusterClient(test.MemberClusterName, memberClient))

		// when
		result1, err1 := cntrl.Reconcile(newMurRequest(mur))
		require.NoError(t, err1)
		assert.True(t, result1.Requeue)
		assert.Equal(t, int64(result1.RequeueAfter), int64(10*time.Second))

		result2, err2 := cntrl.Reconcile(newMurRequest(mur))

		// then
		require.Empty(t, result2)
		require.NoError(t, err2)

		uatest.AssertThatUserAccount(t, "john", memberClient).
			DoesNotExist()
		murtest.AssertThatMasterUserRecord(t, "john", hostClient).
			DoesNotHaveFinalizer()
		AssertThatCounterHas(t, 1, UserAccountsForCluster(test.MemberClusterName, 1))
	})

	t.Run("test wait for UserAccount deletion in progress before removing MUR finalizer", func(t *testing.T) {
		// given
		defer counter.Reset()
		InitializeCounter(t, 2, UserAccountsForCluster(test.MemberClusterName, 2))
		logf.SetLogger(zap.New(zap.UseDevMode(true)))
		s := apiScheme(t)
		mur := murtest.NewMasterUserRecord(t, "john-wait-for-ua",
			murtest.ToBeDeleted())
		userAcc := uatest.NewUserAccountFromMur(mur)
		// set deletion timestamp to indicate UserAccount deletion is in progress
		userAcc.DeletionTimestamp = &metav1.Time{Time: time.Now().Add(-5 * time.Second)}

		hostClient := test.NewFakeClient(t, mur)
		memberClient := test.NewFakeClient(t, userAcc)

		cntrl := newController(t, hostClient, s, NewGetMemberCluster(true, v1.ConditionTrue),
			ClusterClient(test.MemberClusterName, memberClient))

		// when
		result1, err := cntrl.Reconcile(newMurRequest(mur))
		require.NoError(t, err)
		assert.True(t, result1.Requeue)
		assert.Equal(t, int64(result1.RequeueAfter), int64(10*time.Second))

		memberClient.Delete(nil, userAcc)
		result2, err2 := cntrl.Reconcile(newMurRequest(mur))

		// then
		require.Empty(t, result2)
		require.NoError(t, err2)
		uatest.AssertThatUserAccount(t, "john-wait-for-ua", memberClient).
			DoesNotExist()
		murtest.AssertThatMasterUserRecord(t, "john-wait-for-ua", hostClient).
			DoesNotHaveFinalizer()
		AssertThatCounterHas(t, 1, UserAccountsForCluster(test.MemberClusterName, 2))
	})

	t.Run("test wait for UserAccount deletion takes too long", func(t *testing.T) {
		// given
		defer counter.Reset()
		InitializeCounter(t, 2, UserAccountsForCluster(test.MemberClusterName, 2))
		logf.SetLogger(zap.New(zap.UseDevMode(true)))
		s := apiScheme(t)
		mur := murtest.NewMasterUserRecord(t, "john-wait-for-ua",
			murtest.ToBeDeleted())
		userAcc := uatest.NewUserAccountFromMur(mur)
		// set deletion timestamp to indicate UserAccount deletion is in progress
		userAcc.DeletionTimestamp = &metav1.Time{Time: time.Now().Add(-60 * time.Second)}

		hostClient := test.NewFakeClient(t, mur)
		memberClient := test.NewFakeClient(t, userAcc)

		cntrl := newController(t, hostClient, s, NewGetMemberCluster(true, v1.ConditionTrue),
			ClusterClient(test.MemberClusterName, memberClient))

		// when
		result, err := cntrl.Reconcile(newMurRequest(mur))

		// then
		require.Empty(t, result)
		require.Error(t, err)
		require.Equal(t, `failed to delete UserAccount in the member cluster 'member-cluster': UserAccount deletion has not completed in over 1 minute`, err.Error())
		uatest.AssertThatUserAccount(t, "john-wait-for-ua", memberClient).
			Exists()
		murtest.AssertThatMasterUserRecord(t, "john-wait-for-ua", hostClient).
			HasFinalizer()
		AssertThatCounterHas(t, 2, UserAccountsForCluster(test.MemberClusterName, 2))
	})
}

func TestDeleteMultipleUserAccountsViaMasterUserRecordBeingDeleted(t *testing.T) {
	// given
	defer counter.Reset()
	InitializeCounter(t, 2,
		UserAccountsForCluster(test.MemberClusterName, 2),
		UserAccountsForCluster("member2-cluster", 2),
		UserAccountsForCluster("member3-cluster", 2))
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	s := apiScheme(t)
	mur := murtest.NewMasterUserRecord(t, "john",
		murtest.Finalizer("finalizer.toolchain.dev.openshift.com"),
		murtest.ToBeDeleted(), murtest.AdditionalAccounts("member2-cluster"))
	userAcc := uatest.NewUserAccountFromMur(mur)

	memberClient := test.NewFakeClient(t, userAcc)
	memberClient2 := test.NewFakeClient(t, userAcc)
	hostClient := test.NewFakeClient(t, mur)

	cntrl := newController(t, hostClient, s, NewGetMemberCluster(true, v1.ConditionTrue),
		ClusterClient(test.MemberClusterName, memberClient), ClusterClient("member2-cluster", memberClient2))

	// when
	result1, err1 := cntrl.Reconcile(newMurRequest(mur)) // first reconcile will wait for first useraccount to be deleted
	require.NoError(t, err1)
	assert.True(t, result1.Requeue)
	assert.Equal(t, int64(result1.RequeueAfter), int64(10*time.Second))

	result2, err2 := cntrl.Reconcile(newMurRequest(mur)) // second reconcile will wait for second useraccount to be deleted
	require.NoError(t, err2)
	assert.True(t, result2.Requeue)
	assert.Equal(t, int64(result2.RequeueAfter), int64(10*time.Second))

	result3, err3 := cntrl.Reconcile(newMurRequest(mur))

	// then
	require.Empty(t, result3)
	require.NoError(t, err3)

	uatest.AssertThatUserAccount(t, "john", memberClient).
		DoesNotExist()
	uatest.AssertThatUserAccount(t, "john", memberClient2).
		DoesNotExist()
	murtest.AssertThatMasterUserRecord(t, "john", hostClient).
		DoesNotHaveFinalizer()
	AssertThatCounterHas(t, 1,
		UserAccountsForCluster(test.MemberClusterName, 1),
		UserAccountsForCluster("member2-cluster", 1),
		UserAccountsForCluster("member3-cluster", 2))
}

func TestDisablingMasterUserRecord(t *testing.T) {
	// given
	defer counter.Reset()
	InitializeCounter(t, 1, UserAccountsForCluster(test.MemberClusterName, 1))
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	s := apiScheme(t)
	mur := murtest.NewMasterUserRecord(t, "john",
		murtest.Finalizer("finalizer.toolchain.dev.openshift.com"),
		murtest.DisabledMur(true))
	userAccount := uatest.NewUserAccountFromMur(mur, uatest.DisabledUa(false))
	memberClient := test.NewFakeClient(t, userAccount)
	toolchainStatus := NewToolchainStatus(
		WithMember(test.MemberClusterName, WithRoutes("https://console.member-cluster/", "", ToBeReady())))
	hostClient := test.NewFakeClient(t, mur, toolchainStatus)

	cntrl := newController(t, hostClient, s, NewGetMemberCluster(true, v1.ConditionTrue),
		ClusterClient(test.MemberClusterName, memberClient))

	// when
	res, err := cntrl.Reconcile(newMurRequest(mur))
	require.NoError(t, err)
	assert.Equal(t, reconcile.Result{Requeue: false}, res)
	userAcc := &toolchainv1alpha1.UserAccount{}
	err = memberClient.Get(context.TODO(), types.NamespacedName{Name: mur.Name, Namespace: "toolchain-member-operator"}, userAcc)
	require.NoError(t, err)
	assert.True(t, userAcc.Spec.Disabled)
	AssertThatCounterHas(t, 1, UserAccountsForCluster(test.MemberClusterName, 1))
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

func newController(t *testing.T, hostCl client.Client, s *runtime.Scheme, getMemberCluster GetMemberClusterFunc, memberCl ...ClientForCluster) ReconcileMasterUserRecord {
	config, err := configuration.LoadConfig(hostCl)
	require.NoError(t, err)
	return ReconcileMasterUserRecord{
		client:                hostCl,
		scheme:                s,
		retrieveMemberCluster: getMemberCluster(memberCl...),
		config:                config,
	}
}
