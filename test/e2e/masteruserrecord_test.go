package e2e

import (
	"context"
	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	murtest "github.com/codeready-toolchain/host-operator/test/masteruserrecord"
	"github.com/codeready-toolchain/toolchain-common/pkg/cluster"
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
	t.Logf("user account '%s' created", mur.Name)

	err = verifyResources(awaitility, mur, toBeProvisioned())
	assert.NoError(t, err)

	err = verifyResources(awaitility, extraMur, toBeProvisioned())
	assert.NoError(t, err)
}

func verifyResources(awaitility *e2e.Awaitility, mur *toolchainv1alpha1.MasterUserRecord, murCond toolchainv1alpha1.Condition) error {
	hostAwait := NewHostAwaitility(awaitility)
	memberAwait := NewMemberAwaitility(awaitility)
	if err := hostAwait.waitForMasterUserRecord(mur.Name); err != nil {
		return err
	}
	if err := memberAwait.waitForUserAccount(mur.Name, mur.Spec.UserAccounts[0].Spec); err != nil {
		return err
	}
	if err := hostAwait.waitForMurConditions(mur.Name, murCond); err != nil {
		return err
	}
	return nil
}

func toBeProvisioned() toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:   toolchainv1alpha1.ConditionReady,
		Status: v1.ConditionTrue,
		Reason: "Provisioned",
	}
}

func toBeProvisioning() toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:   toolchainv1alpha1.ConditionReady,
		Status: v1.ConditionFalse,
		Reason: "Provisioning",
	}
}

func createMasterUserRecord(awaitility *e2e.Awaitility, ctx *framework.TestCtx, name string) *toolchainv1alpha1.MasterUserRecord {
	fedCluster, ok, err := awaitility.Host().GetKubeFedCluster(awaitility.MemberNs, cluster.Member, e2e.ReadyKubeFedCluster)
	require.NoError(awaitility.T, err)
	require.True(awaitility.T, ok, "KubeFedCluster should exist")
	mur := murtest.NewMasterUserRecord(name,
		murtest.MetaNamespace(awaitility.HostNs), murtest.TargetCluster(fedCluster.Name))

	err = awaitility.Client.Create(context.TODO(), mur, e2e.CleanupOptions(ctx))
	require.NoError(awaitility.T, err)

	err = verifyResources(awaitility, mur, toBeProvisioned())
	assert.NoError(awaitility.T, err)

	return mur
}
