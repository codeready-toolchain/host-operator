package nstemplatetier

import (
	"context"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"

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
