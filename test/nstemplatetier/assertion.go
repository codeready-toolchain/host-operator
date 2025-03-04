package nstemplatetier

import (
	"context"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/types"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// Assertion an assertion helper for an NSTemplateTier
type Assertion struct {
	tier           *toolchainv1alpha1.NSTemplateTier
	client         runtimeclient.Client
	namespacedName types.NamespacedName
	t              test.T
}

func (a *Assertion) Tier() *toolchainv1alpha1.NSTemplateTier {
	return a.tier
}

func (a *Assertion) loadResource() error {
	tier := &toolchainv1alpha1.NSTemplateTier{}
	err := a.client.Get(context.TODO(), a.namespacedName, tier)
	a.tier = tier
	return err
}

// AssertThatNSTemplateTier helper func to begin with the assertions on an NSTemplateTier
func AssertThatNSTemplateTier(t test.T, name string, client runtimeclient.Client) *Assertion {
	return &Assertion{
		client:         client,
		namespacedName: test.NamespacedName(test.HostOperatorNs, name),
		t:              t,
	}
}

// HasStatusTierTemplateRevisions verifies revisions for the given TierTemplates are set in the `NSTemplateTier.Status.Revisions`
func (a *Assertion) HasStatusTierTemplateRevisions(revisions []string) *Assertion {
	err := a.loadResource()
	require.NoError(a.t, err)
	require.Len(a.t, a.tier.Status.Revisions, len(revisions))
	// check that each TierTemplate REF has a TierTemplateRevision set
	for _, tierTemplateRef := range revisions {
		require.NotNil(a.t, a.tier.Status.Revisions)
		value, ok := a.tier.Status.Revisions[tierTemplateRef]
		require.True(a.t, ok)
		require.NotEmpty(a.t, value)
	}
	return a
}

// HasNoStatusTierTemplateRevisions verifies revisions are not set for in the `NSTemplateTier.Status.Revisions`
func (a *Assertion) HasNoStatusTierTemplateRevisions() *Assertion {
	err := a.loadResource()
	require.NoError(a.t, err)
	require.Nil(a.t, a.tier.Status.Revisions)
	return a
}

func (a *Assertion) HasConditions(expected ...toolchainv1alpha1.Condition) *Assertion {
	err := a.loadResource()
	require.NoError(a.t, err)
	test.AssertConditionsMatch(a.t, a.tier.Status.Conditions, expected...)
	return a
}
