package masteruserrecord

import (
	"context"
	"fmt"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	"github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	murtest "github.com/codeready-toolchain/toolchain-common/pkg/test/masteruserrecord"
	uatest "github.com/codeready-toolchain/toolchain-common/pkg/test/useraccount"
	"github.com/go-logr/logr"
	routev1 "github.com/openshift/api/route/v1"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	apierros "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/kubefed/pkg/apis/core/common"
	"sigs.k8s.io/kubefed/pkg/apis/core/v1beta1"
)

type getMemberCluster func(clusters ...clientForCluster) func(name string) (*cluster.FedCluster, bool)

func TestAddFinalizer(t *testing.T) {

	// given
	logf.SetLogger(logf.ZapLogger(true))
	s := apiScheme(t)

	t.Run("ok", func(t *testing.T) {
		mur := murtest.NewMasterUserRecord("john")
		memberClient := test.NewFakeClient(t)
		hostClient := test.NewFakeClient(t, mur)
		cntrl := newController(hostClient, s, newGetMemberCluster(true, v1.ConditionTrue),
			clusterClient(test.MemberClusterName, memberClient))

		// when
		result, err := cntrl.Reconcile(newMurRequest(mur))

		// then
		require.NoError(t, err)
		require.True(t, result.Requeue)

		murtest.AssertThatMasterUserRecord(t, "john", hostClient).
			HasNoConditions().
			HasFinalizer()
	})

	t.Run("fails because it cannot add finalizer", func(t *testing.T) {
		// given
		mur := murtest.NewMasterUserRecord("john")
		hostClient := test.NewFakeClient(t, mur)
		memberClient := test.NewFakeClient(t)
		hostClient.MockUpdate = func(ctx context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
			return fmt.Errorf("unable to add finalizer to MUR %s", mur.Name)
		}
		cntrl := newController(hostClient, s, newGetMemberCluster(true, v1.ConditionTrue),
			clusterClient(test.MemberClusterName, memberClient))

		// when
		_, err := cntrl.Reconcile(newMurRequest(mur))

		// then
		require.Error(t, err)
		msg := "failed while updating with added finalizer: unable to add finalizer to MUR john"
		assert.Contains(t, err.Error(), msg)

		uatest.AssertThatUserAccount(t, "john", memberClient).DoesNotExist()
		murtest.AssertThatMasterUserRecord(t, "john", hostClient).
			HasConditions(toBeNotReady(toolchainv1alpha1.MasterUserRecordUnableToAddFinalizerReason, "unable to add finalizer to MUR john"))
	})
}

func TestCreateUserAccountSuccessful(t *testing.T) {
	// given
	logf.SetLogger(logf.ZapLogger(true))
	s := apiScheme(t)
	mur := murtest.NewMasterUserRecord("john")
	murtest.Modify(mur, murtest.Finalizer("finalizer.toolchain.dev.openshift.com"))
	memberClient := test.NewFakeClient(t)
	hostClient := test.NewFakeClient(t, mur)
	cntrl := newController(hostClient, s, newGetMemberCluster(true, v1.ConditionTrue),
		clusterClient(test.MemberClusterName, memberClient))

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
}

func TestCreateMultipleUserAccountsSuccessful(t *testing.T) {
	// given
	logf.SetLogger(logf.ZapLogger(true))
	s := apiScheme(t)
	mur := murtest.NewMasterUserRecord("john", murtest.AdditionalAccounts("member2-cluster"), murtest.Finalizer("finalizer.toolchain.dev.openshift.com"))
	memberClient := test.NewFakeClient(t)
	memberClient2 := test.NewFakeClient(t)
	hostClient := test.NewFakeClient(t, mur)
	memberClient.MockGet = func(ctx context.Context, key types.NamespacedName, obj runtime.Object) error {
		if _, ok := obj.(*routev1.Route); ok {
			obj = &routev1.Route{
				Spec: routev1.RouteSpec{
					Host: "host",
					Path: "console",
				},
			}
			return nil
		}
		return memberClient.Client.Get(ctx, key, obj)
	}
	memberClient2.MockGet = func(ctx context.Context, key types.NamespacedName, obj runtime.Object) error {
		if _, ok := obj.(*routev1.Route); ok {
			obj = &routev1.Route{
				Spec: routev1.RouteSpec{
					Host: "host",
					Path: "console",
				},
			}
			return nil
		}
		return memberClient2.Client.Get(ctx, key, obj)
	}
	cntrl := newController(hostClient, s, newGetMemberCluster(true, v1.ConditionTrue),
		clusterClient(test.MemberClusterName, memberClient), clusterClient("member2-cluster", memberClient2))

	// when reconciling
	result, err := cntrl.Reconcile(newMurRequest(mur))
	// then
	require.NoError(t, err)
	assert.True(t, result.Requeue)
	uatest.AssertThatUserAccount(t, "john", memberClient).
		Exists().
		MatchMasterUserRecord(mur, mur.Spec.UserAccounts[0].Spec)

	// when reconciling again
	_, err = cntrl.Reconcile(newMurRequest(mur))
	// then
	require.NoError(t, err)
	assert.True(t, result.Requeue)
	uatest.AssertThatUserAccount(t, "john", memberClient2).
		Exists().
		MatchMasterUserRecord(mur, mur.Spec.UserAccounts[1].Spec)
	murtest.AssertThatMasterUserRecord(t, "john", hostClient).
		HasConditions(toBeNotReady(toolchainv1alpha1.MasterUserRecordProvisioningReason, "")).
		HasFinalizer()
}

func TestCreateSynchronizeOrDeleteUserAccountFailed(t *testing.T) {
	// given
	logf.SetLogger(logf.ZapLogger(true))
	s := apiScheme(t)
	mur := murtest.NewMasterUserRecord("john", murtest.Finalizer("finalizer.toolchain.dev.openshift.com"))
	hostClient := test.NewFakeClient(t, mur)

	t.Run("when member cluster does not exist and UA hasn't been created yet", func(t *testing.T) {
		// given
		memberClient := test.NewFakeClient(t)
		cntrl := newController(hostClient, s, newGetMemberCluster(false, v1.ConditionTrue),
			clusterClient(test.MemberClusterName, memberClient))

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
	})

	t.Run("when member cluster does not exist and UA was already created", func(t *testing.T) {
		// given
		memberClient := test.NewFakeClient(t, uatest.NewUserAccountFromMur(mur))
		cntrl := newController(hostClient, s, newGetMemberCluster(false, v1.ConditionTrue),
			clusterClient(test.MemberClusterName, memberClient))

		// when
		_, err := cntrl.Reconcile(newMurRequest(mur))

		// then
		require.Error(t, err)
		msg := "the member cluster member-cluster not found in the registry"
		assert.Contains(t, err.Error(), msg)
		murtest.AssertThatMasterUserRecord(t, "john", hostClient).
			HasConditions(toBeNotReady(toolchainv1alpha1.MasterUserRecordTargetClusterNotReadyReason, msg)).
			HasFinalizer()
	})

	t.Run("when member cluster is not ready and UA hasn't been created yet", func(t *testing.T) {
		// given
		memberClient := test.NewFakeClient(t)
		cntrl := newController(hostClient, s, newGetMemberCluster(true, v1.ConditionFalse),
			clusterClient(test.MemberClusterName, memberClient))

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
	})

	t.Run("when member cluster is not ready and UA was already created", func(t *testing.T) {
		// given
		memberClient := test.NewFakeClient(t, uatest.NewUserAccountFromMur(mur))
		cntrl := newController(hostClient, s, newGetMemberCluster(true, v1.ConditionFalse),
			clusterClient(test.MemberClusterName, memberClient))

		// when
		_, err := cntrl.Reconcile(newMurRequest(mur))

		// then
		require.Error(t, err)
		msg := "the member cluster member-cluster is not ready"
		assert.Contains(t, err.Error(), msg)

		murtest.AssertThatMasterUserRecord(t, "john", hostClient).
			HasConditions(toBeNotReady(toolchainv1alpha1.MasterUserRecordTargetClusterNotReadyReason, msg)).
			HasFinalizer()
	})

	t.Run("status update of the MasterUserRecord failed", func(t *testing.T) {
		// given
		memberClient := test.NewFakeClient(t)
		cntrl := newController(hostClient, s, newGetMemberCluster(true, v1.ConditionTrue),
			clusterClient(test.MemberClusterName, memberClient))
		statusUpdater := func(logger logr.Logger, mur *toolchainv1alpha1.MasterUserRecord, message string) error {
			return fmt.Errorf("unable to update status")
		}

		// when
		err := cntrl.wrapErrorWithStatusUpdate(log, mur, statusUpdater,
			apierros.NewBadRequest("oopsy woopsy"), "failed to create %s", "user bob")

		// then
		require.Error(t, err)
		assert.Equal(t, "failed to create user bob: oopsy woopsy", err.Error())
	})

	t.Run("creation of the UserAccount failed", func(t *testing.T) {
		// given
		memberClient := test.NewFakeClient(t)
		memberClient.MockCreate = func(ctx context.Context, obj runtime.Object, opts ...client.CreateOption) error {
			return fmt.Errorf("unable to create user account %s", mur.Name)
		}
		cntrl := newController(hostClient, s, newGetMemberCluster(true, v1.ConditionTrue),
			clusterClient(test.MemberClusterName, memberClient))

		// when
		_, err := cntrl.Reconcile(newMurRequest(mur))

		// then
		require.Error(t, err)
		msg := "failed to create UserAccount in the member cluster 'member-cluster'"
		assert.Contains(t, err.Error(), msg)

		uatest.AssertThatUserAccount(t, "john", memberClient).DoesNotExist()
		murtest.AssertThatMasterUserRecord(t, "john", hostClient).
			HasConditions(toBeNotReady(toolchainv1alpha1.MasterUserRecordUnableToCreateUserAccountReason, "unable to create user account john"))
	})

	t.Run("spec synchronization of the UserAccount failed", func(t *testing.T) {
		// given
		userAcc := uatest.NewUserAccountFromMur(mur)
		memberClient := test.NewFakeClient(t, userAcc)
		memberClient.MockUpdate = func(ctx context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
			return fmt.Errorf("unable to update user account %s", mur.Name)
		}
		modifiedMur := murtest.NewMasterUserRecord("john", murtest.Finalizer("finalizer.toolchain.dev.openshift.com"))
		murtest.ModifyUaInMur(modifiedMur, test.MemberClusterName, murtest.TierName("admin"))
		hostClient := test.NewFakeClient(t, modifiedMur)
		cntrl := newController(hostClient, s, newGetMemberCluster(true, v1.ConditionTrue),
			clusterClient(test.MemberClusterName, memberClient))

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
	})

	t.Run("status synchronization between UserAccount and MasterUserRecord failed", func(t *testing.T) {
		// given
		updatingCond := toBeNotReady("updating", "")
		provisionedMur := murtest.NewMasterUserRecord("john",
			murtest.Finalizer("finalizer.toolchain.dev.openshift.com"),
			murtest.StatusCondition(updatingCond))
		memberClient := test.NewFakeClient(t, uatest.NewUserAccountFromMur(provisionedMur,
			uatest.StatusCondition(toBeNotReady("somethingFailed", ""))), consoleRoute())

		hostClient := test.NewFakeClient(t, provisionedMur)

		hostClient.MockStatusUpdate = func(ctx context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
			hostClient.MockStatusUpdate = nil // mock only once
			return fmt.Errorf("unable to update MUR %s", provisionedMur.Name)
		}
		cntrl := newController(hostClient, s, newGetMemberCluster(true, v1.ConditionTrue),
			clusterClient(test.MemberClusterName, memberClient))

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
	})

	t.Run("deletion of MasterUserRecord fails because it cannot remove finalizer", func(t *testing.T) {
		// given
		mur := murtest.NewMasterUserRecord("john",
			murtest.Finalizer("finalizer.toolchain.dev.openshift.com"),
			murtest.ToBeDeleted())

		hostClient := test.NewFakeClient(t, mur)
		memberClient := test.NewFakeClient(t, uatest.NewUserAccountFromMur(mur))
		hostClient.MockUpdate = func(ctx context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
			return fmt.Errorf("unable to remove finalizer from MUR %s", mur.Name)
		}
		cntrl := newController(hostClient, s, newGetMemberCluster(true, v1.ConditionTrue),
			clusterClient(test.MemberClusterName, memberClient))

		// when
		_, err := cntrl.Reconcile(newMurRequest(mur))

		// then
		require.Error(t, err)
		msg := "failed to update MasterUserRecord while deleting finalizer"
		assert.Contains(t, err.Error(), msg)

		uatest.AssertThatUserAccount(t, "john", memberClient).DoesNotExist()
		murtest.AssertThatMasterUserRecord(t, "john", hostClient).
			HasConditions(toBeNotReady(toolchainv1alpha1.MasterUserRecordUnableToRemoveFinalizerReason, "unable to remove finalizer from MUR john"))
	})

	t.Run("deletion of the UserAccount failed", func(t *testing.T) {
		// given
		mur := murtest.NewMasterUserRecord("john",
			murtest.Finalizer("finalizer.toolchain.dev.openshift.com"),
			murtest.ToBeDeleted())
		hostClient := test.NewFakeClient(t, mur)

		memberClient := test.NewFakeClient(t, uatest.NewUserAccountFromMur(mur))
		memberClient.MockDelete = func(ctx context.Context, obj runtime.Object, opts ...client.DeleteOption) error {
			return fmt.Errorf("unable to delete user account %s", mur.Name)
		}
		cntrl := newController(hostClient, s, newGetMemberCluster(true, v1.ConditionTrue),
			clusterClient(test.MemberClusterName, memberClient))

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
	})
}

func TestModifyUserAccounts(t *testing.T) {
	// given
	logf.SetLogger(logf.ZapLogger(true))
	s := apiScheme(t)
	mur := murtest.NewMasterUserRecord("john",
		murtest.Finalizer("finalizer.toolchain.dev.openshift.com"),
		murtest.StatusCondition(toBeProvisioned()),
		murtest.AdditionalAccounts("member2-cluster", "member3-cluster"))

	userAccount := uatest.NewUserAccountFromMur(mur)
	userAccount2 := uatest.NewUserAccountFromMur(mur)
	userAccount3 := uatest.NewUserAccountFromMur(mur)

	murtest.ModifyUaInMur(mur, test.MemberClusterName, murtest.NsLimit("advanced"), murtest.TierName("admin"), murtest.Namespace("ide", "54321"))
	murtest.ModifyUaInMur(mur, "member2-cluster", murtest.NsLimit("admin"), murtest.TierName("basic"))

	memberClient := test.NewFakeClient(t, userAccount, consoleRoute())
	memberClient2 := test.NewFakeClient(t, userAccount2, consoleRoute())
	memberClient3 := test.NewFakeClient(t, userAccount3, consoleRoute())
	hostClient := test.NewFakeClient(t, mur)

	cntrl := newController(hostClient, s, newGetMemberCluster(true, v1.ConditionTrue),
		clusterClient(test.MemberClusterName, memberClient), clusterClient("member2-cluster", memberClient2),
		clusterClient("member3-cluster", memberClient3))

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
}

func TestSyncMurStatusWithUserAccountStatuses(t *testing.T) {
	logf.SetLogger(logf.ZapLogger(true))
	s := apiScheme(t)

	t.Run("mur status synced with updated user account statuses", func(t *testing.T) {
		// given
		// setup MUR that wil contain UserAccountStatusEmbedded fields for UserAccounts from "member2-cluster" and "member3-cluster" but will miss from test.MemberClusterName
		// then the reconcile should add the misssing UserAccountStatusEmbedded for the missing test.MemberClusterName cluster without updating anything else
		mur := murtest.NewMasterUserRecord("john",
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

		memberClient := test.NewFakeClient(t, userAccount, consoleRoute())
		memberClient2 := test.NewFakeClient(t, userAccount2, consoleRoute())
		memberClient3 := test.NewFakeClient(t, userAccount3, consoleRoute())

		hostClient := test.NewFakeClient(t, mur)
		cntrl := newController(hostClient, s, newGetMemberCluster(true, v1.ConditionTrue),
			clusterClient(test.MemberClusterName, memberClient), clusterClient("member2-cluster", memberClient2),
			clusterClient("member3-cluster", memberClient3))

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
	})

	t.Run("outdated mur status error cleaned", func(t *testing.T) {
		// given
		// MUR with ready condition set to false with an error
		// all MUR.Status.UserAccount[] conditions are already in sync with the corresponding UserAccounts and set to Ready
		mur := murtest.NewMasterUserRecord("john",
			murtest.Finalizer("finalizer.toolchain.dev.openshift.com"),
			murtest.StatusCondition(toBeNotReady(toolchainv1alpha1.MasterUserRecordTargetClusterNotReadyReason, "something went wrong")),
			murtest.AdditionalAccounts("member2-cluster"))
		userAccount := uatest.NewUserAccountFromMur(mur, uatest.StatusCondition(toBeProvisioned()), uatest.ResourceVersion("123abc"))
		userAccount2 := uatest.NewUserAccountFromMur(mur, uatest.StatusCondition(toBeProvisioned()), uatest.ResourceVersion("123abc"))
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

		hostClient := test.NewFakeClient(t, mur)

		memberClient := test.NewFakeClient(t, userAccount, consoleRoute())
		memberClient2 := test.NewFakeClient(t, uatest.NewUserAccountFromMur(mur), consoleRoute())
		cntrl := newController(hostClient, s, newGetMemberCluster(true, v1.ConditionTrue),
			clusterClient(test.MemberClusterName, memberClient),
			clusterClient("member2-cluster", memberClient2))

		// when
		_, err := cntrl.Reconcile(newMurRequest(mur))

		// then
		// the original error status should be cleaned
		require.NoError(t, err)

		murtest.AssertThatMasterUserRecord(t, "john", hostClient).
			HasConditions(toBeProvisioned()).
			HasStatusUserAccounts(test.MemberClusterName, "member2-cluster").
			HasFinalizer()
	})
}

func TestDeleteUserAccountViaMasterUserRecordBeingDeleted(t *testing.T) {
	// given
	logf.SetLogger(logf.ZapLogger(true))
	s := apiScheme(t)
	mur := murtest.NewMasterUserRecord("john",
		murtest.Finalizer("finalizer.toolchain.dev.openshift.com"),
		murtest.ToBeDeleted())
	userAcc := uatest.NewUserAccountFromMur(mur)

	memberClient := test.NewFakeClient(t, userAcc)
	hostClient := test.NewFakeClient(t, mur)
	cntrl := newController(hostClient, s, newGetMemberCluster(true, v1.ConditionTrue),
		clusterClient(test.MemberClusterName, memberClient))

	// when
	_, err := cntrl.Reconcile(newMurRequest(mur))

	// then
	require.NoError(t, err)

	uatest.AssertThatUserAccount(t, "john", memberClient).
		DoesNotExist()
	murtest.AssertThatMasterUserRecord(t, "john", hostClient).
		DoesNotHaveFinalizer()
}

func TestDeleteMultipleUserAccountsViaMasterUserRecordBeingDeleted(t *testing.T) {
	// given
	logf.SetLogger(logf.ZapLogger(true))
	s := apiScheme(t)
	mur := murtest.NewMasterUserRecord("john",
		murtest.Finalizer("finalizer.toolchain.dev.openshift.com"),
		murtest.ToBeDeleted(), murtest.AdditionalAccounts("member2-cluster"))
	userAcc := uatest.NewUserAccountFromMur(mur)

	memberClient := test.NewFakeClient(t, userAcc)
	memberClient2 := test.NewFakeClient(t, userAcc)
	hostClient := test.NewFakeClient(t, mur)
	cntrl := newController(hostClient, s, newGetMemberCluster(true, v1.ConditionTrue),
		clusterClient(test.MemberClusterName, memberClient), clusterClient("member2-cluster", memberClient2))

	// when
	_, err := cntrl.Reconcile(newMurRequest(mur))

	// then
	require.NoError(t, err)

	uatest.AssertThatUserAccount(t, "john", memberClient).
		DoesNotExist()
	uatest.AssertThatUserAccount(t, "john", memberClient2).
		DoesNotExist()
	murtest.AssertThatMasterUserRecord(t, "john", hostClient).
		DoesNotHaveFinalizer()
}

func TestDisablingMasterUserRecord(t *testing.T) {
	// given
	logf.SetLogger(logf.ZapLogger(true))
	s := apiScheme(t)
	mur := murtest.NewMasterUserRecord("john",
		murtest.Finalizer("finalizer.toolchain.dev.openshift.com"),
		murtest.DisabledMur(true))
	userAccount := uatest.NewUserAccountFromMur(mur, uatest.DisabledUa(false))
	memberClient := test.NewFakeClient(t, userAccount, consoleRoute(), cheRoute(false))
	hostClient := test.NewFakeClient(t, mur)
	cntrl := newController(hostClient, s, newGetMemberCluster(true, v1.ConditionTrue),
		clusterClient(test.MemberClusterName, memberClient))

	// when
	res, err := cntrl.Reconcile(newMurRequest(mur))
	require.NoError(t, err)
	assert.Equal(t, reconcile.Result{Requeue: true}, res) // request a new reconcile loop upon ensuring user accounts...
	// ... so let's reconcile again!
	res, err = cntrl.Reconcile(newMurRequest(mur))
	require.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, res) // no need to reconcile again this time

	userAcc := &toolchainv1alpha1.UserAccount{}
	err = memberClient.Get(context.TODO(), types.NamespacedName{Name: mur.Name, Namespace: "toolchain-member-operator"}, userAcc)
	require.NoError(t, err)

	assert.True(t, userAcc.Spec.Disabled)
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

func newController(hostCl client.Client, s *runtime.Scheme, getMemberCluster getMemberCluster, memberCl ...clientForCluster) ReconcileMasterUserRecord {
	return ReconcileMasterUserRecord{
		client:                hostCl,
		scheme:                s,
		retrieveMemberCluster: getMemberCluster(memberCl...),
	}
}

func newGetMemberCluster(ok bool, status v1.ConditionStatus) getMemberCluster {
	if !ok {
		return func(clusters ...clientForCluster) func(name string) (*cluster.FedCluster, bool) {
			return func(name string) (*cluster.FedCluster, bool) {
				return nil, false
			}
		}
	}
	return func(clusters ...clientForCluster) func(name string) (*cluster.FedCluster, bool) {
		mapping := map[string]client.Client{}
		for _, cluster := range clusters {
			n, cl := cluster()
			mapping[n] = cl
		}
		return func(name string) (*cluster.FedCluster, bool) {
			cl, ok := mapping[name]
			if !ok {
				return nil, false
			}
			return &cluster.FedCluster{
				Client:            cl,
				Type:              cluster.Host,
				OperatorNamespace: test.MemberOperatorNs,
				OwnerClusterName:  test.HostClusterName,
				ClusterStatus: &v1beta1.KubeFedClusterStatus{
					Conditions: []v1beta1.ClusterCondition{{
						Type:   common.ClusterReady,
						Status: status,
					}},
				},
			}, true
		}
	}
}

type clientForCluster func() (string, client.Client)

func clusterClient(name string, cl client.Client) clientForCluster {
	return func() (string, client.Client) {
		return name, cl
	}
}
