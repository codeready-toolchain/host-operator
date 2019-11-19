package masteruserrecord

import (
	"context"
	"fmt"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	murtest "github.com/codeready-toolchain/toolchain-common/pkg/test/masteruserrecord"
	uatest "github.com/codeready-toolchain/toolchain-common/pkg/test/useraccount"

	routev1 "github.com/openshift/api/route/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/kubefed/pkg/apis/core/common"
	"sigs.k8s.io/kubefed/pkg/apis/core/v1beta1"
)

func TestSynchronizeSpec(t *testing.T) {
	// given
	l := logf.ZapLogger(true)
	logf.SetLogger(l)
	mur := murtest.NewMasterUserRecord("john", murtest.StatusCondition(toBeProvisioned()))

	userAccount := uatest.NewUserAccountFromMur(mur)

	murtest.ModifyUaInMur(mur, test.MemberClusterName, murtest.NsLimit("advanced"),
		murtest.TierName("admin"), murtest.Namespace("ide", "54321"))

	memberClient := test.NewFakeClient(t, userAccount, consoleRoute())
	hostClient := test.NewFakeClient(t, mur)

	sync := Synchronizer{
		record:            mur,
		hostClient:        hostClient,
		memberCluster:     newMemberCluster(memberClient),
		memberUserAcc:     userAccount,
		recordSpecUserAcc: mur.Spec.UserAccounts[0],
		log:               l,
	}

	// when
	err := sync.synchronizeSpec()

	// then
	require.NoError(t, err)

	uatest.AssertThatUserAccount(t, "john", memberClient).
		Exists().
		HasSpec(mur.Spec.UserAccounts[0].Spec)

	murtest.AssertThatMasterUserRecord(t, "john", hostClient).
		HasConditions(toBeNotReady(updatingReason, ""))
}

func TestSynchronizeStatus(t *testing.T) {
	// given
	logf.SetLogger(logf.ZapLogger(true))
	apiScheme(t)

	mur := murtest.NewMasterUserRecord("john",
		murtest.StatusCondition(toBeNotReady(provisioningReason, "")))

	userAccount := uatest.NewUserAccountFromMur(mur,
		uatest.StatusCondition(toBeNotReady("Provisioning", "")), uatest.ResourceVersion("123abc"))

	// when and then
	testSyncMurStatusWithUserAccountStatus(t, userAccount, mur, toBeNotReady(provisioningReason, ""))
}

func TestSyncMurStatusWithUserAccountStatusWhenUpdated(t *testing.T) {
	// given
	logf.SetLogger(logf.ZapLogger(true))
	apiScheme(t)

	mur := murtest.NewMasterUserRecord("john",
		murtest.StatusCondition(toBeNotReady(updatingReason, "")))

	userAccount := uatest.NewUserAccountFromMur(mur,
		uatest.StatusCondition(toBeProvisioned()), uatest.ResourceVersion("123abc"))

	mur.Status.UserAccounts = []toolchainv1alpha1.UserAccountStatusEmbedded{{
		SyncIndex:         "111aaa",
		TargetCluster:     test.MemberClusterName,
		UserAccountStatus: userAccount.Status,
	}}

	uatest.Modify(userAccount, uatest.StatusCondition(toBeNotReady("Updating", "")))

	// when and then
	testSyncMurStatusWithUserAccountStatus(t, userAccount, mur, toBeNotReady(updatingReason, ""))
}

func TestSyncMurStatusWithUserAccountStatusWhenCompleted(t *testing.T) {
	// given
	logf.SetLogger(logf.ZapLogger(true))
	apiScheme(t)

	mur := murtest.NewMasterUserRecord("john",
		murtest.StatusCondition(toBeNotReady(provisioningReason, "")))

	userAccount := uatest.NewUserAccountFromMur(mur,
		uatest.StatusCondition(toBeNotReady(provisioningReason, "")), uatest.ResourceVersion("123abc"))

	mur.Status.UserAccounts = []toolchainv1alpha1.UserAccountStatusEmbedded{{
		SyncIndex:         "111aaa",
		TargetCluster:     test.MemberClusterName,
		UserAccountStatus: userAccount.Status,
	}}

	uatest.Modify(userAccount, uatest.StatusCondition(toBeProvisioned()))

	// when and then
	testSyncMurStatusWithUserAccountStatus(t, userAccount, mur, toBeProvisioned())
}

func TestSynchronizeUserAccountFailed(t *testing.T) {
	// given
	l := logf.ZapLogger(true)
	logf.SetLogger(l)

	t.Run("spec synchronization of the UserAccount failed", func(t *testing.T) {
		// given
		mur := murtest.NewMasterUserRecord("john")
		userAcc := uatest.NewUserAccountFromMur(mur)
		memberClient := test.NewFakeClient(t, userAcc, consoleRoute())
		memberClient.MockUpdate = func(ctx context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
			return fmt.Errorf("unable to update user account %s", mur.Name)
		}
		murtest.ModifyUaInMur(mur, test.MemberClusterName, murtest.TierName("admin"))
		hostClient := test.NewFakeClient(t, mur)

		sync := Synchronizer{
			record:            mur,
			hostClient:        hostClient,
			memberCluster:     newMemberCluster(memberClient),
			memberUserAcc:     userAcc,
			recordSpecUserAcc: mur.Spec.UserAccounts[0],
			log:               l,
		}

		// when
		err := sync.synchronizeSpec()

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unable to update user account john")
	})

	t.Run("status synchronization of the UserAccount & MasterUserRecord failed", func(t *testing.T) {
		// given
		provisionedMur := murtest.NewMasterUserRecord("john",
			murtest.StatusCondition(toBeProvisioned()))
		userAcc := uatest.NewUserAccountFromMur(provisionedMur,
			uatest.StatusCondition(toBeNotReady("somethingFailed", "")))
		memberClient := test.NewFakeClient(t, userAcc, consoleRoute())
		hostClient := test.NewFakeClient(t, provisionedMur)
		hostClient.MockStatusUpdate = func(ctx context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
			return fmt.Errorf("unable to update MUR %s", provisionedMur.Name)
		}
		sync := Synchronizer{
			record:            provisionedMur,
			hostClient:        hostClient,
			memberCluster:     newMemberCluster(memberClient),
			memberUserAcc:     userAcc,
			recordSpecUserAcc: provisionedMur.Spec.UserAccounts[0],
			log:               l,
		}

		t.Run("with empty set of UserAccounts statuses", func(t *testing.T) {
			// when
			err := sync.synchronizeStatus()

			// then
			require.Error(t, err)
			assert.Contains(t, err.Error(), "unable to update MUR john")
			assert.Len(t, provisionedMur.Status.UserAccounts, 0)
		})

		t.Run("when the UserAccount was added", func(t *testing.T) {
			//given
			additionalUserAcc := toolchainv1alpha1.UserAccountStatusEmbedded{
				TargetCluster: "some-other",
			}
			provisionedMur.Status.UserAccounts = []toolchainv1alpha1.UserAccountStatusEmbedded{additionalUserAcc}

			// when
			err := sync.synchronizeStatus()

			// then
			require.Error(t, err)
			assert.Contains(t, err.Error(), "unable to update MUR john")
			assert.Len(t, provisionedMur.Status.UserAccounts, 1)
			assert.Contains(t, provisionedMur.Status.UserAccounts, additionalUserAcc)
		})

		t.Run("when the UserAccount was modified", func(t *testing.T) {
			//given
			toBeModified := toolchainv1alpha1.UserAccountStatusEmbedded{
				TargetCluster: test.MemberClusterName,
				SyncIndex:     "somethingCrazy",
			}
			provisionedMur.Status.UserAccounts = []toolchainv1alpha1.UserAccountStatusEmbedded{toBeModified}

			// when
			err := sync.synchronizeStatus()

			// then
			require.Error(t, err)
			assert.Contains(t, err.Error(), "unable to update MUR john")
			assert.Len(t, provisionedMur.Status.UserAccounts, 1)
			assert.Contains(t, provisionedMur.Status.UserAccounts, toBeModified)
		})
	})
}

func testSyncMurStatusWithUserAccountStatus(t *testing.T, userAccount *toolchainv1alpha1.UserAccount, mur *toolchainv1alpha1.MasterUserRecord, expMurCon toolchainv1alpha1.Condition) {
	l := logf.ZapLogger(true)
	condition := userAccount.Status.Conditions[0]

	memberClient := test.NewFakeClient(t, userAccount, consoleRoute())
	hostClient := test.NewFakeClient(t, mur)
	sync := Synchronizer{
		record:            mur,
		hostClient:        hostClient,
		memberCluster:     newMemberCluster(memberClient),
		memberUserAcc:     userAccount,
		recordSpecUserAcc: mur.Spec.UserAccounts[0],
		log:               l,
	}

	// when
	err := sync.synchronizeStatus()

	// then
	require.NoError(t, err)

	uatest.AssertThatUserAccount(t, "john", memberClient).
		Exists().
		HasSpec(mur.Spec.UserAccounts[0].Spec).
		HasConditions(condition)
	murtest.AssertThatMasterUserRecord(t, "john", hostClient).
		HasConditions(expMurCon).
		HasStatusUserAccounts(test.MemberClusterName).
		AllUserAccountsHaveStatusSyncIndex("123abc").
		AllUserAccountsHaveCondition(condition)
}

func newMemberCluster(cl client.Client) *cluster.FedCluster {
	return &cluster.FedCluster{
		Name:              test.MemberClusterName,
		APIEndpoint:       fmt.Sprintf("https://api.%s:6433", test.MemberClusterName),
		Client:            cl,
		Type:              cluster.Member,
		OperatorNamespace: test.HostOperatorNs,
		OwnerClusterName:  test.HostClusterName,
		ClusterStatus: &v1beta1.KubeFedClusterStatus{
			Conditions: []v1beta1.ClusterCondition{{
				Type:   common.ClusterReady,
				Status: v1.ConditionTrue,
			}},
		},
	}
}

func consoleRoute() *routev1.Route {
	return &routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "console",
			Namespace: "openshift-console",
		},
		Spec: routev1.RouteSpec{Host: fmt.Sprintf("console.%s", test.MemberClusterName)},
	}
}
