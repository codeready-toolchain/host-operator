package usersignup

import (
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/stretchr/testify/assert"
)

func TestNewNsTemplateSetSpec(t *testing.T) {
	t.Run("when clusterResources template is specified", func(t *testing.T) {
		// given
		nsTemplateTier := newNsTemplateTier("advanced", "dev", "stage", "extra")

		// when
		setSpec := NewNSTemplateSetSpec(nsTemplateTier)

		// then
		assert.Equal(t, newExpectedNsTemplateSetSpec(), setSpec)
	})

	t.Run("when clusterResources template is NOT specified", func(t *testing.T) {
		// given
		nsTemplateTier := newNsTemplateTier("advanced", "dev", "stage", "extra")
		nsTemplateTier.Spec.ClusterResources = nil

		// when
		setSpec := NewNSTemplateSetSpec(nsTemplateTier)

		// then
		withoutClusterRes := newExpectedNsTemplateSetSpec()
		withoutClusterRes.ClusterResources = nil
		assert.Equal(t, withoutClusterRes, setSpec)
	})
}

func newExpectedNsTemplateSetSpec() *toolchainv1alpha1.NSTemplateSetSpec {
	return &toolchainv1alpha1.NSTemplateSetSpec{
		TierName: "advanced",
		Namespaces: []toolchainv1alpha1.NSTemplateSetNamespace{
			{
				TemplateRef: "advanced-dev-123abc1",
			},
			{
				TemplateRef: "advanced-stage-123abc2",
			},
			{
				TemplateRef: "advanced-extra-123abc3",
			},
		},
		ClusterResources: &toolchainv1alpha1.NSTemplateSetClusterResources{
			TemplateRef: "advanced-clusterresources-654321b",
		},
	}
}
