package nstemplatetier

import (
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	tierutil "github.com/codeready-toolchain/host-operator/pkg/controller/nstemplatetier"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

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

// WithPreviousUpdates adds the given entries in the `status.updates`
func WithPreviousUpdates(entries ...toolchainv1alpha1.NSTemplateTierHistory) TierOption {
	return func(tier *toolchainv1alpha1.NSTemplateTier) {
		tier.Status.Updates = entries
	}
}

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

// WithCurrentUpdateInProgress appends an "in-progress" entry in the `status.updates`
func WithCurrentUpdateInProgress() TierOption {
	return func(tier *toolchainv1alpha1.NSTemplateTier) {
		hash, _ := tierutil.ComputeHashForNSTemplateTier(tier)
		tier.Status.Updates = []toolchainv1alpha1.NSTemplateTierHistory{
			{
				StartTime:      metav1.Now(),
				Hash:           hash,
				Failures:       2,
				FailedAccounts: []string{"failed1", "failed2"},
			},
		}
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
