package masteruserrecord

import (
	"fmt"
	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/host-operator/test"
	murtest "github.com/codeready-toolchain/host-operator/test/masteruserrecord"
	uatest "github.com/codeready-toolchain/host-operator/test/useraccount"
	commontest "github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"testing"
)

func TestSynchronizeSpec(t *testing.T) {
	// given
	logf.SetLogger(logf.ZapLogger(true))
	mur := murtest.NewMasterUserRecord("john", murtest.StatusCondition(toBeProvisioned()))

	userAccount := uatest.NewUserAccountFromMur(mur)

	murtest.ModifyUaInMur(mur, test.MemberClusterName, murtest.NsLimit("advanced"),
		murtest.TierName("admin"), murtest.Namespace("ide", "54321"))

	memberClient := commontest.NewFakeClient(t, userAccount)
	hostClient := commontest.NewFakeClient(t, mur)

	sync := Synchronizer{
		record:            mur,
		hostClient:        hostClient,
		memberClient:      memberClient,
		memberUserAcc:     userAccount,
		recordSpecUserAcc: mur.Spec.UserAccounts[0],
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
	s := apiScheme(t)

	mur := murtest.NewMasterUserRecord("john",
		murtest.StatusCondition(toBeNotReady(provisioningReason, "")))

	userAccount := uatest.NewUserAccountFromMur(mur,
		uatest.StatusCondition(toBeNotReady("Provisioning", "")), uatest.ResourceVersion("123abc"))

	// when and then
	testSyncMurStatusWithUserAccountStatus(t, s, userAccount, mur, toBeNotReady(provisioningReason, ""))
}

func TestSyncMurStatusWithUserAccountStatusWhenUpdated(t *testing.T) {
	// given
	logf.SetLogger(logf.ZapLogger(true))
	s := apiScheme(t)

	mur := murtest.NewMasterUserRecord("john",
		murtest.StatusCondition(toBeNotReady(updatingReason, "")))

	userAccount := uatest.NewUserAccountFromMur(mur,
		uatest.StatusCondition(toBeProvisioned()), uatest.ResourceVersion("123abc"))

	mur.Status.UserAccounts = []toolchainv1alpha1.UserAccountStatusEmbedded{{
		SyncIndex:         "111aaa",
		TargetCluster:     test.MemberClusterName,
		UserAccountStatus: userAccount.Status,
	}}

	uatest.ModifyUa(userAccount, uatest.StatusCondition(toBeNotReady("Updating", "")))

	// when and then
	testSyncMurStatusWithUserAccountStatus(t, s, userAccount, mur, toBeNotReady(updatingReason, ""))
}

func TestSyncMurStatusWithUserAccountStatusWhenCompleted(t *testing.T) {
	// given
	logf.SetLogger(logf.ZapLogger(true))
	s := apiScheme(t)

	mur := murtest.NewMasterUserRecord("john",
		murtest.StatusCondition(toBeNotReady(provisioningReason, "")))

	userAccount := uatest.NewUserAccountFromMur(mur,
		uatest.StatusCondition(toBeNotReady(provisioningReason, "")), uatest.ResourceVersion("123abc"))

	mur.Status.UserAccounts = []toolchainv1alpha1.UserAccountStatusEmbedded{{
		SyncIndex:         "111aaa",
		TargetCluster:     test.MemberClusterName,
		UserAccountStatus: userAccount.Status,
	}}

	uatest.ModifyUa(userAccount, uatest.StatusCondition(toBeProvisioned()))

	// when and then
	testSyncMurStatusWithUserAccountStatus(t, s, userAccount, mur, toBeProvisioned())
}

func TestSynchronizeUserAccountFailed(t *testing.T) {
	// given
	logf.SetLogger(logf.ZapLogger(true))

	t.Run("spec synchronization of the UserAccount failed", func(t *testing.T) {
		// given
		mur := murtest.NewMasterUserRecord("john")
		userAcc := uatest.NewUserAccountFromMur(mur)
		memberClient := commontest.NewFakeClient(t, userAcc)
		memberClient.MockUpdate = func(obj runtime.Object) error {
			return fmt.Errorf("unable to update user account %s", mur.Name)
		}
		murtest.ModifyUaInMur(mur, test.MemberClusterName, murtest.TierName("admin"))
		hostClient := commontest.NewFakeClient(t, mur)

		sync := Synchronizer{
			record:            mur,
			hostClient:        hostClient,
			memberClient:      memberClient,
			memberUserAcc:     userAcc,
			recordSpecUserAcc: mur.Spec.UserAccounts[0],
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
		memberClient := commontest.NewFakeClient(t, userAcc)
		hostClient := commontest.NewFakeClient(t, provisionedMur)
		hostClient.MockStatusUpdate = func(obj runtime.Object) error {
			return fmt.Errorf("unable to update MUR %s", provisionedMur.Name)
		}
		sync := Synchronizer{
			record:            provisionedMur,
			hostClient:        hostClient,
			memberClient:      memberClient,
			memberUserAcc:     userAcc,
			recordSpecUserAcc: provisionedMur.Spec.UserAccounts[0],
		}

		// when
		err := sync.synchronizeStatus()

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unable to update MUR john")
	})
}

func testSyncMurStatusWithUserAccountStatus(t *testing.T, s *runtime.Scheme,
	userAccount *toolchainv1alpha1.UserAccount, mur *toolchainv1alpha1.MasterUserRecord, expMurCon toolchainv1alpha1.Condition) {

	condition := userAccount.Status.Conditions[0]
	memberClient := commontest.NewFakeClient(t, userAccount)
	hostClient := commontest.NewFakeClient(t, mur)
	sync := Synchronizer{
		record:            mur,
		hostClient:        hostClient,
		memberClient:      memberClient,
		memberUserAcc:     userAccount,
		recordSpecUserAcc: mur.Spec.UserAccounts[0],
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
