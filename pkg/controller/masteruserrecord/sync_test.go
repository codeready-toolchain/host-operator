package masteruserrecord

import (
	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/host-operator/test"
	"github.com/codeready-toolchain/host-operator/test/masteruserrecord"
	uatest "github.com/codeready-toolchain/host-operator/test/useraccount"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"testing"
)

func TestSynchronizeSpec(t *testing.T) {
	// given
	logf.SetLogger(logf.ZapLogger(true))
	s := apiScheme(t)
	mur := murtest.NewMasterUserRecord("john", murtest.StatusCondition(toBeProvisioned()))

	userAccount := uatest.NewUserAccountFromMur(mur)

	murtest.ModifyUaInMur(mur, test.MemberClusterName, murtest.NsLimit("advanced"),
		murtest.TierName("admin"), murtest.Namespace("ide", "54321"))

	memberClient := fake.NewFakeClientWithScheme(s, userAccount)
	hostClient := fake.NewFakeClientWithScheme(s, mur)

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

	murtest.AssertThatMasterUserAccount(t, "john", hostClient).
		HasCondition(toBeNotReady(updatingReason, ""))
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

func testSyncMurStatusWithUserAccountStatus(t *testing.T, s *runtime.Scheme,
	userAccount *toolchainv1alpha1.UserAccount, mur *toolchainv1alpha1.MasterUserRecord, expMurCon toolchainv1alpha1.Condition) {

	condition := userAccount.Status.Conditions[0]
	memberClient := fake.NewFakeClientWithScheme(s, userAccount)
	hostClient := fake.NewFakeClientWithScheme(s, mur)
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
		HasCondition(condition)
	murtest.AssertThatMasterUserAccount(t, "john", hostClient).
		HasCondition(expMurCon).
		HasStatusUserAccounts(test.MemberClusterName).
		HasStatusSyncIndex("123abc").
		HasUserAccountCondition(condition)
}
