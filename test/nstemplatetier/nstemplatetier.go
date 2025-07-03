package nstemplatetier

import (
	"fmt"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/constants"
	"github.com/codeready-toolchain/toolchain-common/pkg/hash"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/codeready-toolchain/toolchain-common/pkg/test/nstemplateset"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

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
		Status: toolchainv1alpha1.NSTemplateTierStatus{
			Revisions: map[string]string{
				"advanced-dev-123abc1":              "advanced-dev-123abc1-ttr",
				"advanced-stage-123abc2":            "advanced-stage-123abc2-ttr",
				"advanced-clusterresources-654321b": "advanced-clusterresources-654321b-ttr",
				"advanced-admin-123abc1":            "advanced-admin-123abc1-ttr",
				"advanced-viewer-123abc2":           "advanced-viewer-123abc2-ttr",
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

// AppStudioEnvTier returns an "appstudio-env" NSTemplateTier with template refs in the given spec
func AppStudioEnvTier(t *testing.T, spec toolchainv1alpha1.NSTemplateTierSpec, options ...TierOption) *toolchainv1alpha1.NSTemplateTier {
	return Tier(t, "appstudio-env", spec, options...)
}

func Tier(t *testing.T, name string, spec toolchainv1alpha1.NSTemplateTierSpec, options ...TierOption) *toolchainv1alpha1.NSTemplateTier {
	return TierInNamespace(t, name, "toolchain-host-operator", spec, options...)
}

func TierInNamespace(t *testing.T, name, namespace string, spec toolchainv1alpha1.NSTemplateTierSpec, options ...TierOption) *toolchainv1alpha1.NSTemplateTier {
	tier := &toolchainv1alpha1.NSTemplateTier{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
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

func WithStatusRevisions() TierOption {
	return func(tier *toolchainv1alpha1.NSTemplateTier) {
		tier.Status.Revisions = make(map[string]string)
		for _, ns := range tier.Spec.Namespaces {
			tier.Status.Revisions[ns.TemplateRef] = ns.TemplateRef + "-ttr"
		}
		tier.Status.Revisions[tier.Spec.ClusterResources.TemplateRef] = tier.Spec.ClusterResources.TemplateRef + "-ttr"
		for _, sp := range tier.Spec.SpaceRoles {
			tier.Status.Revisions[sp.TemplateRef] = sp.TemplateRef + "-ttr"
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

// WithFinalizer adds the finalizer to the tier
func WithFinalizer() TierOption {
	return func(nt *toolchainv1alpha1.NSTemplateTier) {
		controllerutil.AddFinalizer(nt, toolchainv1alpha1.FinalizerName)
	}
}

// MarkedBundled marks the tier as bundled by adding the appropriate annotation
func MarkedBundled() TierOption {
	return func(tier *toolchainv1alpha1.NSTemplateTier) {
		if tier.Annotations == nil {
			tier.Annotations = map[string]string{}
		}
		tier.Annotations[toolchainv1alpha1.BundledAnnotationKey] = constants.BundledWithHostOperatorAnnotationValue
	}
}

// OtherTier returns an "other" NSTemplateTier
func OtherTier(t *testing.T, options ...TierOption) *toolchainv1alpha1.NSTemplateTier {
	return Tier(t, "other", toolchainv1alpha1.NSTemplateTierSpec{
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
	}, options...)
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
