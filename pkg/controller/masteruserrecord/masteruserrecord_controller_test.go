package masteruserrecord

import (
	"fmt"
	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	"github.com/codeready-toolchain/host-operator/test"
	murtest "github.com/codeready-toolchain/host-operator/test/masteruserrecord"
	uatest "github.com/codeready-toolchain/host-operator/test/useraccount"
	"github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	commontest "github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	apierros "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/kubefed/pkg/apis/core/common"
	"sigs.k8s.io/kubefed/pkg/apis/core/v1beta1"
	"testing"
)

type getMemberCluster func(clusters ...clientForCluster) func(name string) (*cluster.FedCluster, bool)

func TestCreateUserAccountSuccessful(t *testing.T) {
	// given
	logf.SetLogger(logf.ZapLogger(true))
	s := apiScheme(t)
	mur := murtest.NewMasterUserRecord("john")
	memberClient := fake.NewFakeClientWithScheme(s)
	hostClient := fake.NewFakeClientWithScheme(s, mur)
	cntrl := newController(hostClient, s, newGetMemberCluster(true, v1.ConditionTrue),
		clusterClient(test.MemberClusterName, memberClient))

	// when
	_, err := cntrl.Reconcile(newMurRequest(mur))

	// then
	require.NoError(t, err)

	uatest.AssertThatUserAccount(t, "john", memberClient).
		Exists().
		HasSpec(mur.Spec.UserAccounts[0].Spec)
	murtest.AssertThatMasterUserAccount(t, "john", hostClient).
		HasCondition(toBeNotReady(provisioningReason, ""))
}

func TestCreateMultipleUserAccountsSuccessful(t *testing.T) {
	// given
	logf.SetLogger(logf.ZapLogger(true))
	s := apiScheme(t)
	mur := murtest.NewMasterUserRecord("john", murtest.AdditionalAccounts("member2-cluster"))
	memberClient := fake.NewFakeClientWithScheme(s)
	memberClient2 := fake.NewFakeClientWithScheme(s)
	hostClient := fake.NewFakeClientWithScheme(s, mur)
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
	murtest.AssertThatMasterUserAccount(t, "john", hostClient).
		HasCondition(toBeNotReady(provisioningReason, ""))
}

func TestCreateOrSynchronizeUserAccountFailed(t *testing.T) {
	// given
	logf.SetLogger(logf.ZapLogger(true))
	s := apiScheme(t)
	mur := murtest.NewMasterUserRecord("john")
	hostClient := fake.NewFakeClientWithScheme(s, mur)

	t.Run("when member cluster does not exist and UA hasn't been created yet", func(t *testing.T) {
		// given
		memberClient := fake.NewFakeClientWithScheme(s)
		cntrl := newController(hostClient, s, newGetMemberCluster(false, v1.ConditionTrue),
			clusterClient(test.MemberClusterName, memberClient))

		// when
		_, err := cntrl.Reconcile(newMurRequest(mur))

		// then
		require.Error(t, err)
		msg := "the member cluster member-cluster not found in the registry"
		assert.Contains(t, err.Error(), msg)

		uatest.AssertThatUserAccount(t, "john", memberClient).DoesNotExist()
		murtest.AssertThatMasterUserAccount(t, "john", hostClient).
			HasCondition(toBeNotReady(targetClusterNotReadyReason, msg))
	})

	t.Run("when member cluster does not exist and UA was already created", func(t *testing.T) {
		// given
		memberClient := fake.NewFakeClientWithScheme(s, uatest.NewUserAccountFromMur(mur))
		cntrl := newController(hostClient, s, newGetMemberCluster(false, v1.ConditionTrue),
			clusterClient(test.MemberClusterName, memberClient))

		// when
		_, err := cntrl.Reconcile(newMurRequest(mur))

		// then
		require.Error(t, err)
		msg := "the member cluster member-cluster not found in the registry"
		assert.Contains(t, err.Error(), msg)

		murtest.AssertThatMasterUserAccount(t, "john", hostClient).
			HasCondition(toBeNotReady(targetClusterNotReadyReason, msg))
	})

	t.Run("when member cluster is not ready and UA hasn't been created yet", func(t *testing.T) {
		// given
		memberClient := fake.NewFakeClientWithScheme(s)
		cntrl := newController(hostClient, s, newGetMemberCluster(true, v1.ConditionFalse),
			clusterClient(test.MemberClusterName, memberClient))

		// when
		_, err := cntrl.Reconcile(newMurRequest(mur))

		// then
		require.Error(t, err)
		msg := "the member cluster member-cluster is not ready"
		assert.Contains(t, err.Error(), msg)

		uatest.AssertThatUserAccount(t, "john", memberClient).DoesNotExist()
		murtest.AssertThatMasterUserAccount(t, "john", hostClient).
			HasCondition(toBeNotReady(targetClusterNotReadyReason, msg))
	})

	t.Run("when member cluster is not ready and UA was already created", func(t *testing.T) {
		// given
		memberClient := fake.NewFakeClientWithScheme(s, uatest.NewUserAccountFromMur(mur))
		cntrl := newController(hostClient, s, newGetMemberCluster(true, v1.ConditionFalse),
			clusterClient(test.MemberClusterName, memberClient))

		// when
		_, err := cntrl.Reconcile(newMurRequest(mur))

		// then
		require.Error(t, err)
		msg := "the member cluster member-cluster is not ready"
		assert.Contains(t, err.Error(), msg)

		murtest.AssertThatMasterUserAccount(t, "john", hostClient).
			HasCondition(toBeNotReady(targetClusterNotReadyReason, msg))
	})

	t.Run("status update of the MasterUserRecord failed", func(t *testing.T) {
		// given
		memberClient := fake.NewFakeClientWithScheme(s)
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
		memberClient := commontest.NewFakeClient(t)
		memberClient.MockCreate = func(obj runtime.Object) error {
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
		murtest.AssertThatMasterUserAccount(t, "john", hostClient).
			HasCondition(toBeNotReady(unableToCreateUserAccountReason, "unable to create user account john"))
	})

	t.Run("spec synchronization of the UserAccount failed", func(t *testing.T) {
		// given
		userAcc := uatest.NewUserAccountFromMur(mur)
		memberClient := commontest.NewFakeClient(t, userAcc)
		memberClient.MockUpdate = func(obj runtime.Object) error {
			return fmt.Errorf("unable to update user account %s", mur.Name)
		}
		modifiedMur := murtest.NewMasterUserRecord("john")
		murtest.ModifyUaInMur(modifiedMur, test.MemberClusterName, murtest.TierName("admin"))
		hostClient := fake.NewFakeClientWithScheme(s, modifiedMur)
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
		murtest.AssertThatMasterUserAccount(t, "john", hostClient).
			HasCondition(toBeNotReady(unableToSynchronizeUserAccountSpecReason, "unable to update user account john"))
	})

	t.Run("status synchronization of the UserAccount & MasterUserRecord failed", func(t *testing.T) {
		// given
		provisionedMur := murtest.NewMasterUserRecord("john",
			murtest.StatusCondition(toBeProvisioned()))
		memberClient := commontest.NewFakeClient(t, uatest.NewUserAccountFromMur(provisionedMur,
			uatest.StatusCondition(toBeNotReady("somethingFailed", ""))))
		hostClient := commontest.NewFakeClient(t, provisionedMur)
		hostClient.MockStatusUpdate = func(obj runtime.Object) error {
			return fmt.Errorf("unable to update MUR %s", provisionedMur.Name)
		}
		cntrl := newController(hostClient, s, newGetMemberCluster(true, v1.ConditionTrue),
			clusterClient(test.MemberClusterName, memberClient))

		// when
		_, err := cntrl.Reconcile(newMurRequest(provisionedMur))

		// then
		require.Error(t, err)
		msg := "update of the UserAccount.status in the cluster 'member-cluster' failed"
		assert.Contains(t, err.Error(), msg)

		uatest.AssertThatUserAccount(t, "john", memberClient).Exists()
		murtest.AssertThatMasterUserAccount(t, "john", hostClient).
			HasCondition(toBeProvisioned()).
			HasStatusUserAccounts()
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

	memberClient := fake.NewFakeClientWithScheme(s, userAccount)
	memberClient2 := fake.NewFakeClientWithScheme(s, userAccount2)
	memberClient3 := fake.NewFakeClientWithScheme(s, userAccount3)
	hostClient := fake.NewFakeClientWithScheme(s, mur)

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
	murtest.AssertThatMasterUserAccount(t, "john", hostClient).
		HasCondition(toBeNotReady(updatingReason, ""))
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

	memberClient := fake.NewFakeClientWithScheme(s, userAccount)
	memberClient2 := fake.NewFakeClientWithScheme(s, userAccount2)
	memberClient3 := fake.NewFakeClientWithScheme(s, userAccount3)

	hostClient := fake.NewFakeClientWithScheme(s, mur)
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
		HasCondition(userAccount.Status.Conditions[0])
	uatest.AssertThatUserAccount(t, "john", memberClient2).
		Exists().
		HasSpec(mur.Spec.UserAccounts[1].Spec).
		HasCondition(userAccount2.Status.Conditions[0])
	uatest.AssertThatUserAccount(t, "john", memberClient3).
		Exists().
		HasSpec(mur.Spec.UserAccounts[2].Spec).
		HasCondition(userAccount3.Status.Conditions[0])

	murtest.AssertThatMasterUserAccount(t, "john", hostClient).
		HasCondition(toBeNotReady(updatingReason, "")).
		HasStatusUserAccounts(test.MemberClusterName, "member2-cluster", "member3-cluster").
		HasAllStatusSyncIndex("123abc").
		HasAllUserAccountCondition(userAccount.Status.Conditions[0])
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
