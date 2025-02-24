package nstemplatetiers

import (
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetTierRefs(t *testing.T) {
	// given
	Base1nsTemplates := &toolchainv1alpha1.NSTemplateTier{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "toolchain-host-operator",
			Name:      "base1ns",
		},
		Spec: toolchainv1alpha1.NSTemplateTierSpec{
			Namespaces: []toolchainv1alpha1.NSTemplateTierNamespace{
				{
					TemplateRef: "base1ns-code-123456new",
				},
				{
					TemplateRef: "base1ns-dev-123456new",
				},
				{
					TemplateRef: "base1ns-stage-123456new",
				},
			},
			ClusterResources: &toolchainv1alpha1.NSTemplateTierClusterResources{
				TemplateRef: "base1ns-clusterresources-123456new",
			},
			SpaceRoles: map[string]toolchainv1alpha1.NSTemplateTierSpaceRole{
				"admin": {
					TemplateRef: "base1ns-admin-123456new",
				},
				"edit": {
					TemplateRef: "base1ns-edit-123456new",
				},
				"viewer": {
					TemplateRef: "base1ns-viewer-123456new",
				},
			},
		},
	}
	expectedRefs := []string{"base1ns-code-123456new", "base1ns-dev-123456new", "base1ns-stage-123456new", "base1ns-clusterresources-123456new", "base1ns-admin-123456new", "base1ns-edit-123456new", "base1ns-viewer-123456new"}
	if refs := GetNSTemplateTierRefs(Base1nsTemplates); refs != nil {
		require.Len(t, refs, 7)
		require.Equal(t, expectedRefs, refs)
	}
}
