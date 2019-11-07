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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	apierros "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/kubefed/pkg/apis/core/common"
	"sigs.k8s.io/kubefed/pkg/apis/core/v1beta1"
)

type getMemberCluster func(clusters ...clientForCluster) func(name string) (*cluster.FedCluster, bool)

func TestCreateUserAccountSuccessful(t *testing.T) {
	// given
	logf.SetLogger(logf.ZapLogger(true))
	s := apiScheme(t)
	mur := murtest.NewMasterUserRecord("john")
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
		HasSpec(mur.Spec.UserAccounts[0].Spec)
	murtest.AssertThatMasterUserRecord(t, "john", hostClient).
		HasConditions(toBeNotReady(provisioningReason, "")).
		HasFinalizer()
}

func TestCreateMultipleUserAccountsSuccessful(t *testing.T) {
	// given
	logf.SetLogger(logf.ZapLogger(true))
	s := apiScheme(t)
	mur := murtest.NewMasterUserRecord("john", murtest.AdditionalAccounts("member2-cluster"))
	memberClient := test.NewFakeClient(t)
	memberClient2 := test.NewFakeClient(t)
	hostClient := test.NewFakeClient(t, mur)
	cntrl := newController(hostClient, s, newGetMemberCluster(true, v1.ConditionTrue),
		clusterClient(test.MemberClusterName, memberClient), clusterClient("member2-cluster", memberClient2))

	// when
	_, err := cntrl.Reconcile(newMurRequest(mur))

	// then
	require.NoError(t, err)

	uatest.AssertThatUserAccount(t, "john", memberClient).
		Exists().
		HasSpec(mur.Spec.UserAccounts[0].Spec)
	uatest.AssertThatUserAccount(t, "john", memberClient2).
		Exists().
		HasSpec(mur.Spec.UserAccounts[1].Spec)
	murtest.AssertThatMasterUserRecord(t, "john", hostClient).
		HasConditions(toBeNotReady(provisioningReason, "")).
		HasFinalizer()
}

func TestCreateSynchronizeOrDeleteUserAccountFailed(t *testing.T) {
	// given
	logf.SetLogger(logf.ZapLogger(true))
	s := apiScheme(t)
	mur := murtest.NewMasterUserRecord("john")
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
			HasConditions(toBeNotReady(targetClusterNotReadyReason, msg)).
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
			HasConditions(toBeNotReady(targetClusterNotReadyReason, msg)).
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
			HasConditions(toBeNotReady(targetClusterNotReadyReason, msg)).
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
			HasConditions(toBeNotReady(targetClusterNotReadyReason, msg)).
			HasFinalizer()
	})

	t.Run("status update of the MasterUserRecord failed", func(t *testing.T) {
		// given
		memberClient := test.NewFakeClient(t)
		cntrl := newController(hostClient, s, newGetMemberCluster(true, v1.ConditionTrue),
			clusterClient(test.MemberClusterName, memberClient))
		statusUpdater := func(mur *toolchainv1alpha1.MasterUserRecord, message string) error {
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
			HasConditions(toBeNotReady(unableToCreateUserAccountReason, "unable to create user account john"))
	})

	t.Run("spec synchronization of the UserAccount failed", func(t *testing.T) {
		// given
		userAcc := uatest.NewUserAccountFromMur(mur)
		memberClient := test.NewFakeClient(t, userAcc)
		memberClient.MockUpdate = func(ctx context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
			return fmt.Errorf("unable to update user account %s", mur.Name)
		}
		modifiedMur := murtest.NewMasterUserRecord("john")
		murtest.ModifyUaInMur(modifiedMur, test.MemberClusterName, murtest.TierName("admin"))
		hostClient := test.NewFakeClient(t, modifiedMur)
		cntrl := newController(hostClient, s, newGetMemberCluster(true, v1.ConditionTrue),
			clusterClient(test.MemberClusterName, memberClient))

		// when
		_, err := cntrl.Reconcile(newMurRequest(modifiedMur))

		// then
		require.Error(t, err)
		msg := "update of the UserAccount.spec in the cluster 'member-cluster' failed"
		assert.Contains(t, err.Error(), msg)

		uatest.AssertThatUserAccount(t, "john", memberClient).
			Exists().
			HasSpec(userAcc.Spec)
		murtest.AssertThatMasterUserRecord(t, "john", hostClient).
			HasConditions(toBeNotReady(unableToSynchronizeUserAccountSpecReason, "unable to update user account john"))
	})

	t.Run("status synchronization of the UserAccount & MasterUserRecord failed", func(t *testing.T) {
		// given
		updatingCond := toBeNotReady("updating", "")
		provisionedMur := murtest.NewMasterUserRecord("john",
			murtest.StatusCondition(updatingCond))
		memberClient := test.NewFakeClient(t, uatest.NewUserAccountFromMur(provisionedMur,
			uatest.StatusCondition(toBeNotReady("somethingFailed", ""))))

		hostClient := test.NewFakeClient(t, provisionedMur)
		// mock only once
		hostClient.MockStatusUpdate = func(ctx context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
			hostClient.MockStatusUpdate = nil
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

	t.Run("creation of the UserAccount fails because it cannot add finalizer", func(t *testing.T) {
		// given
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
			HasConditions(toBeNotReady(unableToAddFinalizerReason, "unable to add finalizer to MUR john"))
	})

	t.Run("deletion of MasterUserRecord fails because it cannot remove finalizer", func(t *testing.T) {
		// given
		mur := murtest.NewMasterUserRecord("john", murtest.ToBeDeleted())

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
			HasConditions(toBeNotReady(unableToRemoveFinalizerReason, "unable to remove finalizer from MUR john"))
	})

	t.Run("deletion of the UserAccount failed", func(t *testing.T) {
		// given
		mur := murtest.NewMasterUserRecord("john", murtest.ToBeDeleted())
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
			HasConditions(toBeNotReady(unableToDeleteUserAccountsReason, "unable to delete user account john")).
			HasFinalizer()
	})
}

func TestModifyUserAccounts(t *testing.T) {
	// given
	logf.SetLogger(logf.ZapLogger(true))
	s := apiScheme(t)
	mur := murtest.NewMasterUserRecord("john", murtest.StatusCondition(toBeProvisioned()),
		murtest.AdditionalAccounts("member2-cluster", "member3-cluster"))

	userAccount := uatest.NewUserAccountFromMur(mur)
	userAccount2 := uatest.NewUserAccountFromMur(mur)
	userAccount3 := uatest.NewUserAccountFromMur(mur)

	murtest.ModifyUaInMur(mur, test.MemberClusterName, murtest.NsLimit("advanced"), murtest.TierName("admin"), murtest.Namespace("ide", "54321"))
	murtest.ModifyUaInMur(mur, "member2-cluster", murtest.NsLimit("admin"), murtest.TierName("basic"))

	memberClient := test.NewFakeClient(t, userAccount)
	memberClient2 := test.NewFakeClient(t, userAccount2)
	memberClient3 := test.NewFakeClient(t, userAccount3)
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
		HasSpec(mur.Spec.UserAccounts[0].Spec)
	uatest.AssertThatUserAccount(t, "john", memberClient2).
		Exists().
		HasSpec(mur.Spec.UserAccounts[1].Spec)
	uatest.AssertThatUserAccount(t, "john", memberClient3).
		Exists().
		HasSpec(mur.Spec.UserAccounts[2].Spec)
	murtest.AssertThatMasterUserRecord(t, "john", hostClient).
		HasConditions(toBeNotReady(updatingReason, ""))
}

func TestSyncMurStatusWithUserAccountStatuses(t *testing.T) {
	// given
	logf.SetLogger(logf.ZapLogger(true))
	s := apiScheme(t)

	mur := murtest.NewMasterUserRecord("john",
		murtest.StatusCondition(toBeNotReady(provisioningReason, "")),
		murtest.AdditionalAccounts("member2-cluster", "member3-cluster"))

	userAccount := uatest.NewUserAccountFromMur(mur,
		uatest.StatusCondition(toBeNotReady("Provisioning", "")), uatest.ResourceVersion("123abc"))
	mur.Status.UserAccounts = []toolchainv1alpha1.UserAccountStatusEmbedded{}

	userAccount2 := uatest.NewUserAccountFromMur(mur,
		uatest.StatusCondition(toBeNotReady("Provisioning", "")), uatest.ResourceVersion("123abc"))
	mur.Status.UserAccounts = append(mur.Status.UserAccounts, toolchainv1alpha1.UserAccountStatusEmbedded{
		SyncIndex:         "111aaa",
		TargetCluster:     "member2-cluster",
		UserAccountStatus: userAccount.Status,
	})

	userAccount3 := uatest.NewUserAccountFromMur(mur,
		uatest.StatusCondition(toBeNotReady("Provisioning", "")), uatest.ResourceVersion("123abc"))
	mur.Status.UserAccounts = append(mur.Status.UserAccounts, toolchainv1alpha1.UserAccountStatusEmbedded{
		SyncIndex:         "123abc",
		TargetCluster:     "member3-cluster",
		UserAccountStatus: userAccount.Status,
	})

	memberClient := test.NewFakeClient(t, userAccount)
	memberClient2 := test.NewFakeClient(t, userAccount2)
	memberClient3 := test.NewFakeClient(t, userAccount3)

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
		HasSpec(mur.Spec.UserAccounts[0].Spec).
		HasConditions(userAccount.Status.Conditions...)
	uatest.AssertThatUserAccount(t, "john", memberClient2).
		Exists().
		HasSpec(mur.Spec.UserAccounts[1].Spec).
		HasConditions(userAccount2.Status.Conditions...)
	uatest.AssertThatUserAccount(t, "john", memberClient3).
		Exists().
		HasSpec(mur.Spec.UserAccounts[2].Spec).
		HasConditions(userAccount3.Status.Conditions...)

	murtest.AssertThatMasterUserRecord(t, "john", hostClient).
		HasConditions(toBeNotReady(provisioningReason, "")).
		HasStatusUserAccounts(test.MemberClusterName, "member2-cluster", "member3-cluster").
		AllUserAccountsHaveStatusSyncIndex("123abc").
		AllUserAccountsHaveCondition(userAccount.Status.Conditions[0])
}

func TestDeleteUserAccountViaMasterUserRecordBeingDeleted(t *testing.T) {
	// given
	logf.SetLogger(logf.ZapLogger(true))
	s := apiScheme(t)
	mur := murtest.NewMasterUserRecord("john", murtest.ToBeDeleted())
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
