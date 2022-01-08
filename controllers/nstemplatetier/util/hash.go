package util

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"sort"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
)

// TemplateTierHashLabelKey returns the label key to specify the version of the templates of the given tier
func TemplateTierHashLabelKey(tierName string) string {
	return toolchainv1alpha1.LabelKeyPrefix + tierName + "-tier-hash"
}

// ComputeHashForNSTemplateTier computes the hash of the `.spec.namespaces[].templateRef` + `.spec.clusteResource.TemplateRef`
func ComputeHashForNSTemplateTier(tier *toolchainv1alpha1.NSTemplateTier) (string, error) {
	refs := []string{}
	for _, ns := range tier.Spec.Namespaces {
		refs = append(refs, ns.TemplateRef)
	}
	if tier.Spec.ClusterResources != nil {
		refs = append(refs, tier.Spec.ClusterResources.TemplateRef)
	}
	return computeHash(refs)
}

// ComputeHashForNSTemplateSetSpec computes the hash of the `.spec.namespaces[].templateRef` + `.spec.clusteResource.TemplateRef`
func ComputeHashForNSTemplateSetSpec(s toolchainv1alpha1.NSTemplateSetSpec) (string, error) {
	refs := []string{}
	for _, ns := range s.Namespaces {
		refs = append(refs, ns.TemplateRef)
	}
	if s.ClusterResources != nil && s.ClusterResources.TemplateRef != "" { // ignore when ClusterResources only contains a custom template
		refs = append(refs, s.ClusterResources.TemplateRef)
	}
	return computeHash(refs)
}

func TierHashMatches(tmplTier *toolchainv1alpha1.NSTemplateTier, nsTmplSetSpec toolchainv1alpha1.NSTemplateSetSpec) bool {
	tierHash, err := ComputeHashForNSTemplateTier(tmplTier)
	if err != nil {
		return false
	}

	nsTmplSetSpecHash, err := ComputeHashForNSTemplateSetSpec(nsTmplSetSpec)
	if err != nil {
		return false
	}
	return tierHash == nsTmplSetSpecHash
}

type templateRefs struct {
	Refs []string `json:"refs"`
}

func computeHash(refs []string) (string, error) {
	// sort the refs to make sure we have a predictive hash!
	sort.Strings(refs)
	m, err := json.Marshal(templateRefs{Refs: refs}) // embed in a type with JSON tags
	if err != nil {
		return "", err
	}
	md5hash := md5.New()
	// Ignore the error, as this implementation cannot return one
	_, _ = md5hash.Write(m)
	hash := hex.EncodeToString(md5hash.Sum(nil))
	return hash, nil
}
