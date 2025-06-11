package nstemplatetier

import (
	"context"
	"fmt"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/hash"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/finalizer"
)

type nsTemplateTierUsedFinalizer struct {
	runtimeclient.Client
}

// Finalize implements finalizer.Finalizer.
func (n *nsTemplateTierUsedFinalizer) Finalize(ctx context.Context, tier runtimeclient.Object) (finalizer.Result, error) {
	list := &toolchainv1alpha1.SpaceList{}
	if err := n.List(ctx, list,
		runtimeclient.InNamespace(tier.GetNamespace()),
		runtimeclient.HasLabels{hash.TemplateTierHashLabelKey(tier.GetName())},
		runtimeclient.Limit(1)); err != nil {
		return finalizer.Result{}, err
	}

	if len(list.Items) > 0 {
		return finalizer.Result{}, fmt.Errorf("the tier is still used by some spaces")
	}

	return finalizer.Result{}, nil
}

var _ finalizer.Finalizer = (*nsTemplateTierUsedFinalizer)(nil)
