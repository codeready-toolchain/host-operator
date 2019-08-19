package e2e

import (
	"context"
	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/host-operator/test/masteruserrecord"
	"github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	"github.com/codeready-toolchain/toolchain-common/pkg/condition"
	"github.com/codeready-toolchain/toolchain-common/pkg/test/e2e"
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

	verifyResources(awaitility, mur, toBeProvisioned())
	verifyResources(awaitility, extraMur, toBeProvisioned())

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

		verifyResources(awaitility, toBeModifiedMur, toBeProvisioned())
		verifyResources(awaitility, mur, toBeProvisioned())
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

		verifyResources(awaitility, currentMur, toBeProvisioned(), coolStatus())

		extraMur = NewHostAwaitility(awaitility).GetMasterUserRecord(extraMur.Name)
		verifyResources(awaitility, extraMur, toBeProvisioned())
	})
}

func verifyResources(awaitility *e2e.Awaitility, mur *toolchainv1alpha1.MasterUserRecord, uaStatusConds ...toolchainv1alpha1.Condition) {
	hostAwait := NewHostAwaitility(awaitility)
	memberAwait := NewMemberAwaitility(awaitility)
	err := hostAwait.WaitForMasterUserRecord(mur.Name)
	assert.NoError(awaitility.T, err)

	murUserAccount := mur.Spec.UserAccounts[0]
	err = memberAwait.WaitForUserAccount(mur.Name, murUserAccount.Spec, uaStatusConds...)
	assert.NoError(awaitility.T, err)

	userAccount := memberAwait.GetUserAccount(mur.Name)
	uaStatus := toolchainv1alpha1.UserAccountStatusEmbedded{
		TargetCluster:     murUserAccount.TargetCluster,
		UserAccountStatus: userAccount.Status,
	}

	err = hostAwait.WaitForMurConditions(mur.Name,
		UntilHasUserAccountStatus(uaStatus),
		UntilHasStatusCondition(toBeProvisioned()))
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

	verifyResources(awaitility, mur, toBeProvisioned())

	return mur
}
