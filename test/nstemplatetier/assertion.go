package nstemplatetier

import (
	"context"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"

	"github.com/stretchr/testify/assert"
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

// HasStatusUpdatesItems verifies the number of items in `status.updates`
func (a *Assertion) HasStatusUpdatesItems(expected int) *Assertion {
	err := a.loadResource()
	require.NoError(a.t, err)
	require.Len(a.t, a.tier.Status.Updates, expected)
	return a
}

// HasStatusTierTemplateRevisions verifies revisions for the given TierTemplates are set in the `NSTemplateTier.Status.Revisions`
func (a *Assertion) HasStatusTierTemplateRevisions(revisions []string) *Assertion {
	err := a.loadResource()
	require.NoError(a.t, err)
	// check that each TierTemplate REF has a TierTemplateRevision set
	for _, tierTemplateRef := range revisions {
		require.NotNil(a.t, a.tier.Status.Revisions)
		_, ok := a.tier.Status.Revisions[tierTemplateRef]
		require.True(a.t, ok)
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

// HasValidPreviousUpdates verifies the previous `status.updates`
// in particular, it checks that:
// - `StartTime` is not nil
// - `Hash` is not nil
func (a *Assertion) HasValidPreviousUpdates() *Assertion {
	err := a.loadResource()
	require.NoError(a.t, err)
	require.NotEmpty(a.t, a.tier.Status.Updates)
	for _, h := range a.tier.Status.Updates[:len(a.tier.Status.Updates)-1] {
		assert.NotNil(a.t, h.StartTime)
		assert.NotNil(a.t, h.Hash)
	}
	return a
}

// HasLatestUpdate verifies the latest `status.updates`
func (a *Assertion) HasLatestUpdate(expected toolchainv1alpha1.NSTemplateTierHistory) *Assertion {
	err := a.loadResource()
	require.NoError(a.t, err)
	require.NotEmpty(a.t, a.tier.Status.Updates)
	latest := a.tier.Status.Updates[len(a.tier.Status.Updates)-1]
	assert.False(a.t, latest.StartTime.IsZero())
	assert.Equal(a.t, expected.Hash, latest.Hash)

	return a
}
