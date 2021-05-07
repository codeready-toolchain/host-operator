package nstemplatetier

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"sort"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// murSelector creates a label selector to find MasterUserRecords which are not up-to-date with
// the templateRefs of the given NSTemplateTier.
//
// (longer explanation)
// newLabelSelector creates a selector to find MasterUserRecords which have a label with key
// `toolchain.dev.openshift.com/<tiername>-tier-hash` but whose value is NOT `<hash>`
//
// In other words, this label selector will be used to list MasterUserRecords which have a user account set to the given `<tier>`
// but with a template version (defined by `<hash>`) which is NOT to the expected value (the one provided by `instance`).
//
// Note: The `hash` value is computed from the TemplateRefs. See `computeTemplateRefsHash()`
func murSelector(tier *toolchainv1alpha1.NSTemplateTier) (client.MatchingLabelsSelector, error) {
	// compute the hash of the `.spec.namespaces[].templateRef` + `.spec.clusteResource.TemplateRef`
	hash, err := ComputeHashForNSTemplateTier(tier)
	if err != nil {
		return client.MatchingLabelsSelector{}, err
	}
	selector := labels.NewSelector()
	tierLabel, err := labels.NewRequirement(TemplateTierHashLabelKey(tier.Name), selection.Exists, []string{})
	if err != nil {
		return client.MatchingLabelsSelector{}, err
	}
	selector = selector.Add(*tierLabel)
	templateHashLabel, err := labels.NewRequirement(TemplateTierHashLabelKey(tier.Name), selection.NotEquals, []string{hash})
	if err != nil {
		return client.MatchingLabelsSelector{}, err
	}
	selector = selector.Add(*templateHashLabel)
	return client.MatchingLabelsSelector{
		Selector: selector,
	}, nil
}

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
