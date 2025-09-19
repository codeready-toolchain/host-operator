package nstemplatetiers

import (
	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
)

// GetNSTemplateTierRefs returns a list with all the refs from the NSTemplateTier
func GetNSTemplateTierRefs(tmplTier *toolchainv1alpha1.NSTemplateTier) []string {
	var refs []string
	ForAllNSTemplateTierRefs(tmplTier, func(ref string) {
		refs = append(refs, ref)
	})
	return refs
}

// ForAllNSTemplateTierRefs iterates over all the tier templates referenced by the ns template tier
// and calls a function for each of them.
func ForAllNSTemplateTierRefs(tmplTier *toolchainv1alpha1.NSTemplateTier, visitor func(string)) {
	for _, ns := range tmplTier.Spec.Namespaces {
		visitor(ns.TemplateRef)
	}
	if tmplTier.Spec.ClusterResources != nil {
		visitor(tmplTier.Spec.ClusterResources.TemplateRef)
	}

	for _, r := range tmplTier.Spec.SpaceRoles {
		visitor(r.TemplateRef)
	}
}
