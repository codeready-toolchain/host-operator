package nstemplatetier

import (
	"fmt"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/hash"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/codeready-toolchain/toolchain-common/pkg/test/nstemplateset"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewNSTemplateTier(tierName string, nsTypes ...string) *toolchainv1alpha1.NSTemplateTier {
	namespaces := make([]toolchainv1alpha1.NSTemplateTierNamespace, len(nsTypes))
	for i, nsType := range nsTypes {
		revision := fmt.Sprintf("123abc%d", i+1)
		namespaces[i] = toolchainv1alpha1.NSTemplateTierNamespace{
			TemplateRef: nstemplateset.NewTierTemplateName(tierName, nsType, revision),
		}
	}

	return &toolchainv1alpha1.NSTemplateTier{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: test.HostOperatorNs,
			Name:      tierName,
		},
		Spec: toolchainv1alpha1.NSTemplateTierSpec{
			Namespaces: namespaces,
			ClusterResources: &toolchainv1alpha1.NSTemplateTierClusterResources{
				TemplateRef: nstemplateset.NewTierTemplateName(tierName, "clusterresources", "654321b"),
			},
			SpaceRoles: map[string]toolchainv1alpha1.NSTemplateTierSpaceRole{
				"admin": {
					TemplateRef: tierName + "-admin-123abc1",
				},
				"viewer": {
					TemplateRef: tierName + "-viewer-123abc2",
				},
			},
		},
	}
}

// PreviousBase1nsTemplates previous templates for the "base1ns" tier
var PreviousBase1nsTemplates = toolchainv1alpha1.NSTemplateTierSpec{
	Namespaces: []toolchainv1alpha1.NSTemplateTierNamespace{
		{
			TemplateRef: "base1ns-code-123456old",
		},
		{
			TemplateRef: "base1ns-dev-123456old",
		},
		{
			TemplateRef: "base1ns-stage-123456old",
		},
	},
	ClusterResources: &toolchainv1alpha1.NSTemplateTierClusterResources{
		TemplateRef: "base1ns-clusterresources-123456old",
	},
	SpaceRoles: map[string]toolchainv1alpha1.NSTemplateTierSpaceRole{
		"admin": {
			TemplateRef: "base1ns-admin-123456old",
		},
		"viewer": {
			TemplateRef: "base1ns-viewer-123456old",
		},
	},
}

// CurrentBase1nsTemplates current templates for the "base1ns" tier
var CurrentBase1nsTemplates = toolchainv1alpha1.NSTemplateTierSpec{
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
}

func NSTemplateTierSpecWithTierName(tierName string) toolchainv1alpha1.NSTemplateTierSpec {
	return toolchainv1alpha1.NSTemplateTierSpec{
		Namespaces: []toolchainv1alpha1.NSTemplateTierNamespace{
			{
				TemplateRef: tierName + "-code-123456new",
			},
			{
				TemplateRef: tierName + "-dev-123456new",
			},
			{
				TemplateRef: tierName + "-stage-123456new",
			},
		},
		ClusterResources: &toolchainv1alpha1.NSTemplateTierClusterResources{
			TemplateRef: tierName + "-clusterresources-123456new",
		},
		SpaceRoles: map[string]toolchainv1alpha1.NSTemplateTierSpaceRole{
			"admin": {
				TemplateRef: tierName + "-admin-123456new",
			},
			"edit": {
				TemplateRef: tierName + "-edit-123456new",
			},
			"viewer": {
				TemplateRef: tierName + "-viewer-123456new",
			},
		},
	}
}

// AppStudioEnvTemplates current templates for the "appstudio-env" tier
var AppStudioEnvTemplates = toolchainv1alpha1.NSTemplateTierSpec{
	Namespaces: []toolchainv1alpha1.NSTemplateTierNamespace{
		{
			TemplateRef: "appstudio-env-88b275d-88b275d",
		},
	},
	ClusterResources: &toolchainv1alpha1.NSTemplateTierClusterResources{
		TemplateRef: "appstudio-env-clusterresources-e0e1f34-e0e1f34",
	},
	SpaceRoles: map[string]toolchainv1alpha1.NSTemplateTierSpaceRole{
		"admin": {
			TemplateRef: "appstudio-env-admin-ba5db27-ba5db27",
		},
		"contributor": {
			TemplateRef: "appstudio-env-contributor-88b275d-88b275d",
		},
		"maintainer": {
			TemplateRef: "appstudio-env-maintainer-0d170ba-0d170ba",
		},
	},
	SpaceRequestConfig: &toolchainv1alpha1.SpaceRequestConfig{
		ServiceAccountName: toolchainv1alpha1.AdminServiceAccountName,
	},
}

// Base1nsTier returns a "base1ns" NSTemplateTier with template refs in the given spec
func Base1nsTier(t *testing.T, spec toolchainv1alpha1.NSTemplateTierSpec, options ...TierOption) *toolchainv1alpha1.NSTemplateTier {
	return Tier(t, "base1ns", spec, options...)
}

var NSTemplateTierRevision = map[string]string{
	"advanced-dev-123abc1":              "advanced-dev-123abc1",
	"advanced-stage-123abc2":            "advanced-stage-123abc2",
	"advanced-clusterresources-654321b": "advanced-clusterresources-654321b",
	"advanced-admin-123abc1":            "advanced-admin-123abc1",
	"advanced-viewer-123abc2":           "advanced-viewer-123abc2",
}

// AppStudioEnvTier returns an "appstudio-env" NSTemplateTier with template refs in the given spec
func AppStudioEnvTier(t *testing.T, spec toolchainv1alpha1.NSTemplateTierSpec, options ...TierOption) *toolchainv1alpha1.NSTemplateTier {
	return Tier(t, "appstudio-env", spec, options...)
}

func Tier(t *testing.T, name string, spec toolchainv1alpha1.NSTemplateTierSpec, options ...TierOption) *toolchainv1alpha1.NSTemplateTier {
	tier := &toolchainv1alpha1.NSTemplateTier{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "toolchain-host-operator",
			Name:      name,
		},
		Spec: spec,
	}
	hash, err := hash.ComputeHashForNSTemplateTier(tier)
	require.NoError(t, err)
	tier.Labels = map[string]string{
		"toolchain.dev.openshift.com/" + tier.Name + "-tier-hash": hash,
	}
	for _, set := range options {
		set(tier)
	}
	return tier
}

// TierOption an option to configure the NStemplateTier
type TierOption func(*toolchainv1alpha1.NSTemplateTier)

func WithStatusRevisionsBase1ns() TierOption {
	return func(tier *toolchainv1alpha1.NSTemplateTier) {
		tier.Status.Revisions = map[string]string{
			CurrentBase1nsTemplates.Namespaces[0].TemplateRef:        CurrentBase1nsTemplates.Namespaces[0].TemplateRef,
			CurrentBase1nsTemplates.Namespaces[1].TemplateRef:        CurrentBase1nsTemplates.Namespaces[1].TemplateRef,
			CurrentBase1nsTemplates.Namespaces[2].TemplateRef:        CurrentBase1nsTemplates.Namespaces[2].TemplateRef,
			CurrentBase1nsTemplates.ClusterResources.TemplateRef:     CurrentBase1nsTemplates.ClusterResources.TemplateRef,
			CurrentBase1nsTemplates.SpaceRoles["admin"].TemplateRef:  CurrentBase1nsTemplates.SpaceRoles["admin"].TemplateRef,
			CurrentBase1nsTemplates.SpaceRoles["edit"].TemplateRef:   CurrentBase1nsTemplates.SpaceRoles["edit"].TemplateRef,
			CurrentBase1nsTemplates.SpaceRoles["viewer"].TemplateRef: CurrentBase1nsTemplates.SpaceRoles["viewer"].TemplateRef,
		}
	}
}

func WithStatusRevisionsOlderTier() TierOption {
	return func(tier *toolchainv1alpha1.NSTemplateTier) {
		tier.Status.Revisions = map[string]string{
			PreviousBase1nsTemplates.Namespaces[0].TemplateRef:        PreviousBase1nsTemplates.Namespaces[0].TemplateRef,
			PreviousBase1nsTemplates.Namespaces[1].TemplateRef:        PreviousBase1nsTemplates.Namespaces[1].TemplateRef,
			PreviousBase1nsTemplates.Namespaces[2].TemplateRef:        PreviousBase1nsTemplates.Namespaces[2].TemplateRef,
			PreviousBase1nsTemplates.ClusterResources.TemplateRef:     PreviousBase1nsTemplates.ClusterResources.TemplateRef,
			PreviousBase1nsTemplates.SpaceRoles["admin"].TemplateRef:  PreviousBase1nsTemplates.SpaceRoles["admin"].TemplateRef,
			PreviousBase1nsTemplates.SpaceRoles["viewer"].TemplateRef: PreviousBase1nsTemplates.SpaceRoles["viewer"].TemplateRef,
		}
	}
}

func WithStatusRevisionsOtherTier() TierOption {
	return func(tier *toolchainv1alpha1.NSTemplateTier) {
		tier.Status.Revisions = map[string]string{
			OtherTier().Spec.Namespaces[0].TemplateRef:    OtherTier().Spec.Namespaces[0].TemplateRef,
			OtherTier().Spec.Namespaces[1].TemplateRef:    OtherTier().Spec.Namespaces[1].TemplateRef,
			OtherTier().Spec.Namespaces[2].TemplateRef:    OtherTier().Spec.Namespaces[2].TemplateRef,
			OtherTier().Spec.ClusterResources.TemplateRef: OtherTier().Spec.ClusterResources.TemplateRef,
		}
	}
}

// WithoutCodeNamespace removes the `code` templates from the tier's specs.
func WithoutCodeNamespace() TierOption {
	return func(tier *toolchainv1alpha1.NSTemplateTier) {
		tier.Spec.Namespaces = []toolchainv1alpha1.NSTemplateTierNamespace{
			{
				TemplateRef: "base1ns-dev-123456new",
			},
			{
				TemplateRef: "base1ns-stage-123456new",
			},
		}
	}
}

// WithoutClusterResources removes the `clusterResources` templates from the tier's specs.
func WithoutClusterResources() TierOption {
	return func(tier *toolchainv1alpha1.NSTemplateTier) {
		tier.Spec.ClusterResources = nil
	}
}

// WithParameter appends a parameter to the parameter's list
func WithParameter(name, value string) TierOption {
	return func(tier *toolchainv1alpha1.NSTemplateTier) {
		if tier.Spec.Parameters == nil {
			tier.Spec.Parameters = []toolchainv1alpha1.Parameter{}
		}
		tier.Spec.Parameters = append(tier.Spec.Parameters,
			toolchainv1alpha1.Parameter{
				Name:  name,
				Value: value,
			},
		)
	}
}

// OtherTier returns an "other" NSTemplateTier
func OtherTier() *toolchainv1alpha1.NSTemplateTier {
	return &toolchainv1alpha1.NSTemplateTier{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "toolchain-host-operator",
			Name:      "other",
		},
		Spec: toolchainv1alpha1.NSTemplateTierSpec{
			Namespaces: []toolchainv1alpha1.NSTemplateTierNamespace{
				{
					TemplateRef: "other-code-123456a",
				},
				{
					TemplateRef: "other-dev-123456a",
				},
				{
					TemplateRef: "other-stage-123456a",
				},
			},
			ClusterResources: &toolchainv1alpha1.NSTemplateTierClusterResources{
				TemplateRef: "other-clusterresources-123456a",
			},
		},
		Status: toolchainv1alpha1.NSTemplateTierStatus{
			Revisions: map[string]string{
				"other-code-123456a":             "other-code-123456a",
				"other-dev-123456a":              "other-dev-123456a",
				"other-stage-123456a":            "other-stage-123456a",
				"other-clusterresources-123456a": "other-clusterresources-123456a",
			},
		},
	}
}

func OtherTierWithoutRevision() *toolchainv1alpha1.NSTemplateTier {
	return &toolchainv1alpha1.NSTemplateTier{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "toolchain-host-operator",
			Name:      "other",
		},
		Spec: toolchainv1alpha1.NSTemplateTierSpec{
			Namespaces: []toolchainv1alpha1.NSTemplateTierNamespace{
				{
					TemplateRef: "other-code-123456a",
				},
				{
					TemplateRef: "other-dev-123456a",
				},
				{
					TemplateRef: "other-stage-123456a",
				},
			},
			ClusterResources: &toolchainv1alpha1.NSTemplateTierClusterResources{
				TemplateRef: "other-clusterresources-123456a",
			},
		},
	}
}

// TierWithoutDeactivationTimeout returns a NSTemplateTier with no deactivation timeout set
func TierWithoutDeactivationTimeout() *toolchainv1alpha1.NSTemplateTier {
	return &toolchainv1alpha1.NSTemplateTier{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "toolchain-host-operator",
			Name:      "no-deactivation",
		},
		Spec: toolchainv1alpha1.NSTemplateTierSpec{
			Namespaces: []toolchainv1alpha1.NSTemplateTierNamespace{
				{
					TemplateRef: "no-deactivation-code-123456a",
				},
				{
					TemplateRef: "no-deactivation-dev-123456a",
				},
				{
					TemplateRef: "no-deactivation-stage-123456a",
				},
			},
			ClusterResources: &toolchainv1alpha1.NSTemplateTierClusterResources{
				TemplateRef: "no-deactivation-clusterresources-123456a",
			},
		},
	}
}
