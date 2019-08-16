package e2e

import (
	"context"
	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/host-operator/test/masteruserrecord"
	murtest "github.com/codeready-toolchain/host-operator/test/masteruserrecord"
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

	err = verifyResources(awaitility, mur, toBeProvisioned())
	assert.NoError(t, err)

	err = verifyResources(awaitility, extraMur, toBeProvisioned())
	assert.NoError(t, err)

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

		err = verifyResources(awaitility, toBeModifiedMur, toBeProvisioned())
		assert.NoError(t, err)
		err = verifyResources(awaitility, mur, toBeProvisioned())
		assert.NoError(t, err)
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

		err = verifyResources(awaitility, currentMur, toBeProvisioned(), coolStatus())
		assert.NoError(t, err)
		extraMur = NewHostAwaitility(awaitility).GetMasterUserRecord(extraMur.Name)
		err = verifyResources(awaitility, extraMur, toBeProvisioned())
		assert.NoError(t, err)
	})
}

func verifyResources(awaitility *e2e.Awaitility, mur *toolchainv1alpha1.MasterUserRecord, uaStatusConds ...toolchainv1alpha1.Condition) error {
	hostAwait := NewHostAwaitility(awaitility)
	memberAwait := NewMemberAwaitility(awaitility)
	if err := hostAwait.WaitForMasterUserRecord(mur.Name); err != nil {
		return err
	}
	murUserAccount := mur.Spec.UserAccounts[0]
	if err := memberAwait.WaitForUserAccount(mur.Name, murUserAccount.Spec, uaStatusConds...); err != nil {
		return err
	}
	userAccount := memberAwait.GetUserAccount(mur.Name)
	uaStatus := toolchainv1alpha1.UserAccountStatusEmbedded{
		TargetCluster:     murUserAccount.TargetCluster,
		UserAccountStatus: userAccount.Status,
	}
	if err := hostAwait.WaitForMurConditions(mur.Name,
		UntilHasStatusCondition(toBeProvisioned()),
		UntilHasUserAccountStatus(uaStatus)); err != nil {
		return err
	}
	return nil
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

	err = verifyResources(awaitility, mur, toBeProvisioned())
	assert.NoError(awaitility.T, err)

	return mur
}
