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
	ttrs      []toolchainv1alpha1.TierTemplateRevision
	client    runtimeclient.Client
	namespace string
	t         test.T
}

func (a *Assertion) loadResources(labels map[string]string) error {
	ttrs := &toolchainv1alpha1.TierTemplateRevisionList{}
	err := a.client.List(context.TODO(), ttrs, runtimeclient.InNamespace(a.namespace), runtimeclient.MatchingLabels(labels))
	a.ttrs = ttrs.Items
	return err
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
	err := a.loadResources(nil)
	require.NoError(a.t, err)
	require.Empty(a.t, a.ttrs)
	return a
}

// ExistsFor verifies that there are TTRs with given labels
func (a *Assertion) ExistsFor(tierName string, tierTemplateRef ...string) *Assertion {
	for _, templateRef := range tierTemplateRef {
		labels := map[string]string{
			toolchainv1alpha1.TierLabelKey:        tierName,
			toolchainv1alpha1.TemplateRefLabelKey: templateRef,
		}
		err := a.loadResources(labels)
		require.NoError(a.t, err)
		require.Len(a.t, a.ttrs, 1)
	}
	return a
}

func (a *Assertion) ForEach(assertionFunc func(ttr *toolchainv1alpha1.TierTemplateRevision)) *Assertion {
	for i := range a.ttrs {
		assertionFunc(&a.ttrs[i])
	}
	return a
}
