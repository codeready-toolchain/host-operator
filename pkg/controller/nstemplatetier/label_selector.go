package nstemplatetier

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"

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
	hash, err := ComputeHashForNSTemplateTier(*tier)
	if err != nil {
		return client.MatchingLabelsSelector{}, err
	}
	selector := labels.NewSelector()
	tierLabel, err := labels.NewRequirement(TemplateTierHashLabelKey(tier.Name), selection.Exists, []string{})
	selector = selector.Add(*tierLabel)
	templateHashLabel, err := labels.NewRequirement(TemplateTierHashLabelKey(tier.Name), selection.NotEquals, []string{hash})
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
func ComputeHashForNSTemplateTier(tier toolchainv1alpha1.NSTemplateTier) (string, error) {
	refs := templateRefs{}
	for _, ns := range tier.Spec.Namespaces {
		refs.Namespaces = append(refs.Namespaces, ns.TemplateRef)
	}
	if tier.Spec.ClusterResources != nil {
		refs.ClusterResources = tier.Spec.ClusterResources.TemplateRef
	}
	return computeHash(refs)
}

// ComputeHashForNSTemplateSetSpec computes the hash of the `.spec.namespaces[].templateRef` + `.spec.clusteResource.TemplateRef`
func ComputeHashForNSTemplateSetSpec(s toolchainv1alpha1.NSTemplateSetSpec) (string, error) {
	refs := templateRefs{}
	for _, ns := range s.Namespaces {
		refs.Namespaces = append(refs.Namespaces, ns.TemplateRef)
	}
	if s.ClusterResources != nil {
		refs.ClusterResources = s.ClusterResources.TemplateRef
	}
	return computeHash(refs)
}

type templateRefs struct {
	Namespaces       []string `json:"namespaces"`
	ClusterResources string   `json:"clusterresource,omitempty"`
}

func computeHash(refs templateRefs) (string, error) {
	m, err := json.Marshal(refs)
	if err != nil {
		return "", err
	}
	md5hash := md5.New()
	// Ignore the error, as this implementation cannot return one
	md5hash.Write(m)
	hash := hex.EncodeToString(md5hash.Sum(nil))
	return hash, nil
}

func templateRefsFor(obj interface{}) []string {
	switch obj := obj.(type) {
	case toolchainv1alpha1.NSTemplateSetSpec:
		templateRefs := []string{}
		for _, ns := range obj.Namespaces {
			templateRefs = append(templateRefs, ns.TemplateRef)
		}
		if obj.ClusterResources != nil {
			templateRefs = append(templateRefs, obj.ClusterResources.TemplateRef)
		}
		return templateRefs
	case toolchainv1alpha1.NSTemplateTierSpec:
		templateRefs := []string{}
		for _, ns := range obj.Namespaces {
			templateRefs = append(templateRefs, ns.TemplateRef)
		}
		if obj.ClusterResources != nil {
			templateRefs = append(templateRefs, obj.ClusterResources.TemplateRef)
		}
		return templateRefs
	default:
		return []string{}
	}
}
