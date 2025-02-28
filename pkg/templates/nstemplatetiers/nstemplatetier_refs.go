package nstemplatetiers

import (
	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
)

// getNSTemplateTierRefs returns a list with all the refs from the NSTemplateTier
func GetNSTemplateTierRefs(tmplTier *toolchainv1alpha1.NSTemplateTier) []string {
	var refs []string
	for _, ns := range tmplTier.Spec.Namespaces {
		refs = append(refs, ns.TemplateRef)
	}
	if tmplTier.Spec.ClusterResources != nil {
		refs = append(refs, tmplTier.Spec.ClusterResources.TemplateRef)
	}

	roles := make([]string, 0, len(tmplTier.Spec.SpaceRoles))
	for r := range tmplTier.Spec.SpaceRoles {
		roles = append(roles, r)
	}
	for _, r := range roles {
		refs = append(refs, tmplTier.Spec.SpaceRoles[r].TemplateRef)
	}
	return refs
}
