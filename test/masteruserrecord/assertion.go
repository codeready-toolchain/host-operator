package murtest

import (
	"context"
	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/host-operator/test"
	commonttest "github.com/codeready-toolchain/toolchain-common/pkg/test"
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

func (a *MurAssertion) HasCondition(expected toolchainv1alpha1.Condition) *MurAssertion {
	err := a.loadUaAssertion()
	require.NoError(a.t, err)
	commonttest.AssertConditionsMatch(a.t, a.masterUserRecord.Status.Conditions, expected)
	return a
}
