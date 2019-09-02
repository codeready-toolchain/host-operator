package e2e

import (
	"context"
	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	"github.com/codeready-toolchain/toolchain-common/pkg/condition"
	"github.com/codeready-toolchain/toolchain-common/pkg/test/e2e"
	murtest "github.com/codeready-toolchain/toolchain-common/pkg/test/masteruserrecord"
	framework "github.com/operator-framework/operator-sdk/pkg/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/api/core/v1"
	"testing"
)

func TestMasterUserRecord(t *testing.T) {
	// given
	murList := &toolchainv1alpha1.MasterUserRecordList{}
	ctx, awaitility := e2e.InitializeOperators(t, murList, cluster.Host)
	defer ctx.Cleanup()

	extraMur := createMasterUserRecord(awaitility, ctx, "extrajohn")
	t.Log("extra MasterUserRecord created at start")
	targetCluster := extraMur.Spec.UserAccounts[0].TargetCluster
	mur := murtest.NewMasterUserRecord("johnsmith",
		murtest.MetaNamespace(awaitility.HostNs), murtest.TargetCluster(targetCluster))

	// when
	err := awaitility.Client.Create(context.TODO(), mur, e2e.CleanupOptions(ctx))

	// then
	require.NoError(t, err)
	t.Logf("MasterUserRecord '%s' created", mur.Name)

	verifyResourcesExpectingToBeProvisioned(awaitility, mur)
	verifyResourcesExpectingToBeProvisioned(awaitility, extraMur)

	t.Run("update UserAccount spec when MasterUserRecord spec is modified", func(t *testing.T) {
		// given
		toBeModifiedMur := NewHostAwaitility(awaitility).GetMasterUserRecord(extraMur.Name)
		murtest.ModifyUaInMur(toBeModifiedMur, targetCluster, murtest.NsLimit("advanced"),
			murtest.TierName("admin"), murtest.Namespace("che", "4321"))

		// when
		err := awaitility.Client.Update(context.TODO(), toBeModifiedMur)

		// then
		require.NoError(t, err)
		t.Logf("MasterUserRecord '%s' updated", mur.Name)

		// TODO: verify expected condition when the member operator has a logic that updates NsTemplateSet and its status
		verifyResources(awaitility, toBeModifiedMur, nil, expectingUaConditions(toBeProvisioned()))
		verifyResourcesExpectingToBeProvisioned(awaitility, mur)
	})

	t.Run("update MasterUserRecord status when UserAccount status is modified", func(t *testing.T) {
		// given
		currentMur := NewHostAwaitility(awaitility).GetMasterUserRecord(mur.Name)
		userAccount := NewMemberAwaitility(awaitility).GetUserAccount(mur.Name)
		userAccount.Status.Conditions, _ = condition.AddOrUpdateStatusConditions(
			userAccount.Status.Conditions, coolStatus())

		// when
		err := awaitility.ControllerClient.Status().Update(context.TODO(), userAccount)

		// then
		require.NoError(t, err)
		t.Logf("MasterUserRecord '%s' updated", mur.Name)

		verifyResources(awaitility, currentMur, expectingMurConditions(toBeProvisioned()),
			expectingUaConditions(toBeProvisioned(), coolStatus()))

		extraMur = NewHostAwaitility(awaitility).GetMasterUserRecord(extraMur.Name)
		// TODO: verify expected condition when the member operator has a logic that updates NsTemplateSet and its status
		verifyResources(awaitility, extraMur, nil, expectingUaConditions(toBeProvisioned()))
	})

	t.Run("delete MasterUserRecord and expect UserAccount to be deleted", func(t *testing.T) {
		// given
		currentMur := NewHostAwaitility(awaitility).GetMasterUserRecord(mur.Name)

		// when
		err := awaitility.Client.Delete(context.TODO(), currentMur)

		// then
		require.NoError(t, err)

		t.Logf("MasterUserRecord '%s' updated", mur.Name)

		verifyDeletion(awaitility, currentMur)
		assert.NoError(t, err)

		extraMur = NewHostAwaitility(awaitility).GetMasterUserRecord(extraMur.Name)
		verifyResources(awaitility, extraMur, nil, expectingUaConditions(toBeProvisioned()))
	})
}

func verifyDeletion(awaitility *e2e.Awaitility, mur *toolchainv1alpha1.MasterUserRecord) {
	hostAwait := NewHostAwaitility(awaitility)
	memberAwait := NewMemberAwaitility(awaitility)

	err := memberAwait.WaitForDeletedUserAccount(mur.Name)
	assert.NoError(awaitility.T, err, "UserAccount is not deleted")

	err = hostAwait.WaitForDeletedMasterUserRecord(mur.Name)
	assert.NoError(awaitility.T, err, "MasterUserRecord is not deleted")
}

type murConditionsGetter func() []toolchainv1alpha1.Condition
type uaConditionsGetter func() []toolchainv1alpha1.Condition

func expectingMurConditions(murStatusConds ...toolchainv1alpha1.Condition) murConditionsGetter {
	return func() []toolchainv1alpha1.Condition {
		return murStatusConds
	}
}
func expectingUaConditions(murStatusConds ...toolchainv1alpha1.Condition) uaConditionsGetter {
	return func() []toolchainv1alpha1.Condition {
		return murStatusConds
	}
}
func verifyResourcesExpectingToBeProvisioned(awaitility *e2e.Awaitility, mur *toolchainv1alpha1.MasterUserRecord) {
	verifyResources(awaitility, mur, expectingMurConditions(toBeProvisioned()), expectingUaConditions(toBeProvisioned()))
}

func verifyResources(awaitility *e2e.Awaitility, mur *toolchainv1alpha1.MasterUserRecord,
	expectingMurConds murConditionsGetter, expectingUaCons uaConditionsGetter) {

	hostAwait := NewHostAwaitility(awaitility)
	memberAwait := NewMemberAwaitility(awaitility)
	err := hostAwait.WaitForMasterUserRecord(mur.Name)
	assert.NoError(awaitility.T, err)

	murUserAccount := mur.Spec.UserAccounts[0]
	err = memberAwait.WaitForUserAccount(mur.Name, murUserAccount.Spec, expectingUaCons()...)
	assert.NoError(awaitility.T, err)

	userAccount := memberAwait.GetUserAccount(mur.Name)
	uaStatus := toolchainv1alpha1.UserAccountStatusEmbedded{
		TargetCluster:     murUserAccount.TargetCluster,
		UserAccountStatus: userAccount.Status,
	}

	if expectingMurConds != nil {
		err = hostAwait.WaitForMurConditions(mur.Name,
			UntilHasUserAccountStatus(uaStatus),
			UntilHasStatusCondition(expectingMurConds()...))
	} else {
		err = hostAwait.WaitForMurConditions(mur.Name,
			UntilHasUserAccountStatus(uaStatus))
	}
	assert.NoError(awaitility.T, err)
}

func coolStatus() toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:    toolchainv1alpha1.ConditionType("CoolType"),
		Status:  v1.ConditionTrue,
		Reason:  "EverythingIsGood",
		Message: "because our SaaS is cool",
	}
}

func toBeProvisioned() toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:   toolchainv1alpha1.ConditionReady,
		Status: v1.ConditionTrue,
		Reason: "Provisioned",
	}
}

func createMasterUserRecord(awaitility *e2e.Awaitility, ctx *framework.TestCtx, name string) *toolchainv1alpha1.MasterUserRecord {
	memberCluster, ok, err := awaitility.Host().GetKubeFedCluster(awaitility.MemberNs, cluster.Member, e2e.ReadyKubeFedCluster)
	require.NoError(awaitility.T, err)
	require.True(awaitility.T, ok, "KubeFedCluster should exist")
	mur := murtest.NewMasterUserRecord(name,
		murtest.MetaNamespace(awaitility.HostNs), murtest.TargetCluster(memberCluster.Name))

	err = awaitility.Client.Create(context.TODO(), mur, e2e.CleanupOptions(ctx))
	require.NoError(awaitility.T, err)

	verifyResourcesExpectingToBeProvisioned(awaitility, mur)

	return mur
}
