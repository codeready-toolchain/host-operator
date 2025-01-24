package tiertemplaterevision

import (
	"context"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"

	"github.com/stretchr/testify/require"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// Assertion an assertion helper for an TierTemplateRevision
type Assertion struct {
	client    runtimeclient.Client
	namespace string
	t         test.T
}

func (a *Assertion) loadResources(labels map[string]string) ([]toolchainv1alpha1.TierTemplateRevision, error) {
	ttrs := &toolchainv1alpha1.TierTemplateRevisionList{}
	err := a.client.List(context.TODO(), ttrs, runtimeclient.InNamespace(a.namespace), runtimeclient.MatchingLabels(labels))
	return ttrs.Items, err
}

// AssertThatTTRs helper func to begin with the assertions on an TierTemplateRevisions
func AssertThatTTRs(t test.T, client runtimeclient.Client, namespace string) *Assertion {
	return &Assertion{
		client:    client,
		namespace: namespace,
		t:         t,
	}
}

// DoNotExist verifies that there is no TTR in the given namespace
func (a *Assertion) DoNotExist() *Assertion {
	ttrs, err := a.loadResources(nil)
	require.NoError(a.t, err)
	require.Empty(a.t, ttrs)
	return a
}

// ExistFor verifies that there are TTRs with given labels
func (a *Assertion) ExistFor(tierName string, tierTemplateRef ...string) *Assertion {
	for _, templateRef := range tierTemplateRef {
		labels := map[string]string{
			toolchainv1alpha1.TierLabelKey:        tierName,
			toolchainv1alpha1.TemplateRefLabelKey: templateRef,
		}
		ttrs, err := a.loadResources(labels)
		require.NoError(a.t, err)
		require.Len(a.t, ttrs, 1)
	}
	return a
}

func (a *Assertion) ForEach(assertionFunc func(ttr *toolchainv1alpha1.TierTemplateRevision)) *Assertion {
	ttrs, err := a.loadResources(nil)
	require.NoError(a.t, err)
	for i := range ttrs {
		assertionFunc(&ttrs[i])
	}
	return a
}

func (a *Assertion) NumberOfPresentCRs(expected int) *Assertion {
	ttrs, err := a.loadResources(nil)
	require.NoError(a.t, err)
	require.Len(a.t, ttrs, expected)
	return a
}
