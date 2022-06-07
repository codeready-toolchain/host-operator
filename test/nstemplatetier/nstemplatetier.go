package nstemplatetier

import (
	"fmt"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	tierutil "github.com/codeready-toolchain/host-operator/controllers/nstemplatetier/util"
	"github.com/codeready-toolchain/host-operator/pkg/templates/nstemplatetiers"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewNSTemplateTier(tierName string, nsTypes ...string) *toolchainv1alpha1.NSTemplateTier {
	namespaces := make([]toolchainv1alpha1.NSTemplateTierNamespace, len(nsTypes))
	for i, nsType := range nsTypes {
		revision := fmt.Sprintf("123abc%d", i+1)
		namespaces[i] = toolchainv1alpha1.NSTemplateTierNamespace{
			TemplateRef: nstemplatetiers.NewTierTemplateName(tierName, nsType, revision),
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
				TemplateRef: nstemplatetiers.NewTierTemplateName(tierName, "clusterresources", "654321b"),
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

// PreviousBasicTemplates previous templates for the "basic" tier
var PreviousBasicTemplates = toolchainv1alpha1.NSTemplateTierSpec{
	DeactivationTimeoutDays: 30,
	Namespaces: []toolchainv1alpha1.NSTemplateTierNamespace{
		{
			TemplateRef: "basic-code-123456old",
		},
		{
			TemplateRef: "basic-dev-123456old",
		},
		{
			TemplateRef: "basic-stage-123456old",
		},
	},
	ClusterResources: &toolchainv1alpha1.NSTemplateTierClusterResources{
		TemplateRef: "basic-clusterresources-123456old",
	},
	SpaceRoles: map[string]toolchainv1alpha1.NSTemplateTierSpaceRole{
		"admin": {
			TemplateRef: "basic-admin-123456old",
		},
		"viewer": {
			TemplateRef: "basic-viewer-123456old",
		},
	},
}

// CurrentBasicTemplates current templates for the "basic" tier
var CurrentBasicTemplates = toolchainv1alpha1.NSTemplateTierSpec{
	DeactivationTimeoutDays: 30,
	Namespaces: []toolchainv1alpha1.NSTemplateTierNamespace{
		{
			TemplateRef: "basic-code-123456new",
		},
		{
			TemplateRef: "basic-dev-123456new",
		},
		{
			TemplateRef: "basic-stage-123456new",
		},
	},
	ClusterResources: &toolchainv1alpha1.NSTemplateTierClusterResources{
		TemplateRef: "basic-clusterresources-123456new",
	},
	SpaceRoles: map[string]toolchainv1alpha1.NSTemplateTierSpaceRole{
		"admin": {
			TemplateRef: "basic-admin-123456new",
		},
		"viewer": {
			TemplateRef: "basic-viewer-123456new",
		},
	},
}

// BaseTier returns a "base" NSTemplateTier with template refs in the given spec
func BaseTier(t *testing.T, spec toolchainv1alpha1.NSTemplateTierSpec, options ...TierOption) *toolchainv1alpha1.NSTemplateTier {
	tier := &toolchainv1alpha1.NSTemplateTier{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "toolchain-host-operator",
			Name:      "base",
		},
		Spec: spec,
	}
	hash, err := tierutil.ComputeHashForNSTemplateTier(tier)
	require.NoError(t, err)
	tier.Labels = map[string]string{
		"toolchain.dev.openshift.com/" + tier.Name + "-tier-hash": hash,
	}
	for _, set := range options {
		set(tier)
	}
	return tier
}

// BasicTier returns a "basic" NSTemplateTier with template refs in the given spec
func BasicTier(t *testing.T, spec toolchainv1alpha1.NSTemplateTierSpec, options ...TierOption) *toolchainv1alpha1.NSTemplateTier {
	tier := &toolchainv1alpha1.NSTemplateTier{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "toolchain-host-operator",
			Name:      "basic",
		},
		Spec: spec,
	}
	hash, err := tierutil.ComputeHashForNSTemplateTier(tier)
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

// WithoutCodeNamespace removes the `code` templates from the tier's specs.
func WithoutCodeNamespace() TierOption {
	return func(tier *toolchainv1alpha1.NSTemplateTier) {
		tier.Spec.Namespaces = []toolchainv1alpha1.NSTemplateTierNamespace{
			{
				TemplateRef: "basic-dev-123456new",
			},
			{
				TemplateRef: "basic-stage-123456new",
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

// WithPreviousUpdates adds the given entries in the `status.updates`
func WithPreviousUpdates(entries ...toolchainv1alpha1.NSTemplateTierHistory) TierOption {
	return func(tier *toolchainv1alpha1.NSTemplateTier) {
		tier.Status.Updates = entries
	}
}

// WithCurrentUpdate appends an entry in the `status.updates` for the current tier
func WithCurrentUpdate() TierOption {
	return func(tier *toolchainv1alpha1.NSTemplateTier) {
		hash, _ := tierutil.ComputeHashForNSTemplateTier(tier)
		if tier.Status.Updates == nil {
			tier.Status.Updates = []toolchainv1alpha1.NSTemplateTierHistory{}
		}
		tier.Status.Updates = append(tier.Status.Updates,
			toolchainv1alpha1.NSTemplateTierHistory{
				StartTime: metav1.Now(),
				Hash:      hash,
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
			DeactivationTimeoutDays: 60,
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
