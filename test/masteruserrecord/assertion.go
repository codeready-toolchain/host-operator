package murtest

import (
	"context"
	"fmt"
	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/host-operator/test"
	commonttest "github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"testing"
)

type MurAssertion struct {
	masterUserRecord *toolchainv1alpha1.MasterUserRecord
	client           client.Client
	namespacedName   types.NamespacedName
	t                *testing.T
}

func (a *MurAssertion) loadUaAssertion() error {
	if a.masterUserRecord != nil {
		return nil
	}
	mur := &toolchainv1alpha1.MasterUserRecord{}
	err := a.client.Get(context.TODO(), a.namespacedName, mur)
	a.masterUserRecord = mur
	return err
}

func AssertThatMasterUserAccount(t *testing.T, name string, client client.Client) *MurAssertion {
	return &MurAssertion{
		client:         client,
		namespacedName: test.NamespacedName(test.HostOperatorNs, name),
		t:              t,
	}
}

func (a *MurAssertion) HasConditions(expected ...toolchainv1alpha1.Condition) *MurAssertion {
	err := a.loadUaAssertion()
	require.NoError(a.t, err)
	commonttest.AssertConditionsMatch(a.t, a.masterUserRecord.Status.Conditions, expected...)
	return a
}

func (a *MurAssertion) HasStatusUserAccounts(targetClusters ...string) *MurAssertion {
	err := a.loadUaAssertion()
	require.NoError(a.t, err)
	require.Len(a.t, a.masterUserRecord.Status.UserAccounts, len(targetClusters))
	for _, cluster := range targetClusters {
		a.hasUserAccount(cluster)
	}
	return a
}

func (a *MurAssertion) hasUserAccount(targetCluster string) *toolchainv1alpha1.UserAccountStatusEmbedded {
	for _, ua := range a.masterUserRecord.Status.UserAccounts {
		if ua.TargetCluster == targetCluster {
			return &ua
		}
	}
	assert.Fail(a.t, fmt.Sprintf("user account status record for the target cluster %s was not found", targetCluster))
	return nil
}

func (a *MurAssertion) AllUserAccountsHaveStatusSyncIndex(syncIndex string) *MurAssertion {
	err := a.loadUaAssertion()
	require.NoError(a.t, err)
	for _, ua := range a.masterUserRecord.Status.UserAccounts {
		assert.Equal(a.t, syncIndex, ua.SyncIndex)
	}
	return a
}

func (a *MurAssertion) AllUserAccountsHaveCondition(expected toolchainv1alpha1.Condition) *MurAssertion {
	err := a.loadUaAssertion()
	require.NoError(a.t, err)
	for _, ua := range a.masterUserRecord.Status.UserAccounts {
		commonttest.AssertConditionsMatch(a.t, ua.Conditions, expected)
	}
	return a
}
