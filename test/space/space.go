package space

import (
	"fmt"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/controllers/nstemplatetier/util"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/stretchr/testify/require"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const DefaultNSTemplateTierName = "basic"

func WithTargetCluster(targetCluster string) SpaceModifier {
	return func(space *toolchainv1alpha1.Space) {
		space.Spec.TargetCluster = targetCluster
	}
}

func WithTierNameAndHashLabelFor(t *testing.T, tier *toolchainv1alpha1.NSTemplateTier) SpaceModifier {
	return func(space *toolchainv1alpha1.Space) {
		hash, err := util.ComputeHashForNSTemplateTier(tier) // we can assume the JSON marshalling will always work
		require.NoError(t, err)
		space.Spec.TierName = tier.Name
		space.ObjectMeta.Labels = map[string]string{
			util.TemplateTierHashLabelKey(tier.Name): hash,
		}
	}
}

type SpaceModifier func(*toolchainv1alpha1.Space)

func NewSpace(t *testing.T, name string, modifiers ...SpaceModifier) *toolchainv1alpha1.Space {
	hash, err := util.ComputeHashForNSTemplateTier(DefaultNSTemplateTier()) // we can assume the JSON marshalling will always work
	require.NoError(t, err)
	space := &toolchainv1alpha1.Space{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: test.HostOperatorNs,
			Labels: map[string]string{
				util.TemplateTierHashLabelKey(DefaultNSTemplateTierName): hash,
			},
		},
		Spec: toolchainv1alpha1.SpaceSpec{
			TargetCluster: "member-1",
			TierName:      "basic",
		},
	}
	for _, modify := range modifiers {
		modify(space)
	}
	return space
}

func NewSpaces(t *testing.T, size int, nameFmt string, modifiers ...SpaceModifier) []runtime.Object {
	murs := make([]runtime.Object, size)
	for i := 0; i < size; i++ {
		murs[i] = NewSpace(t, fmt.Sprintf(nameFmt, i), modifiers...)
	}
	return murs
}

// DefaultNSTemplateTier the default NSTemplateTier used to initialize the MasterUserRecord
func DefaultNSTemplateTier() *toolchainv1alpha1.NSTemplateTier {
	return &toolchainv1alpha1.NSTemplateTier{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: test.HostOperatorNs,
			Name:      DefaultNSTemplateTierName,
		},
		Spec: toolchainv1alpha1.NSTemplateTierSpec{
			Namespaces: []toolchainv1alpha1.NSTemplateTierNamespace{
				{
					TemplateRef: "basic-dev-123abc",
				},
				{
					TemplateRef: "basic-code-123abc",
				},
				{
					TemplateRef: "basic-stage-123abc",
				},
			},
			ClusterResources: &toolchainv1alpha1.NSTemplateTierClusterResources{
				TemplateRef: "basic-clusterresources-654321a",
			},
		},
	}
}
