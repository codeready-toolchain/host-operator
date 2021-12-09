package nstemplatetier

import (
	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	tierutil "github.com/codeready-toolchain/host-operator/controllers/nstemplatetier/util"

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// outdatedSelector creates a label selector to find MasterUserRecords or Spaces which are not up-to-date with
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
func outdatedTierSelector(tier *toolchainv1alpha1.NSTemplateTier) (client.MatchingLabelsSelector, error) {
	// compute the hash of the `.spec.namespaces[].templateRef` + `.spec.clusteResource.TemplateRef`
	hash, err := tierutil.ComputeHashForNSTemplateTier(tier)
	if err != nil {
		return client.MatchingLabelsSelector{}, err
	}
	selector := labels.NewSelector()
	tierLabel, err := labels.NewRequirement(tierutil.TemplateTierHashLabelKey(tier.Name), selection.Exists, []string{})
	if err != nil {
		return client.MatchingLabelsSelector{}, err
	}
	selector = selector.Add(*tierLabel)
	templateHashLabel, err := labels.NewRequirement(tierutil.TemplateTierHashLabelKey(tier.Name), selection.NotEquals, []string{hash})
	if err != nil {
		return client.MatchingLabelsSelector{}, err
	}
	selector = selector.Add(*templateHashLabel)
	return client.MatchingLabelsSelector{
		Selector: selector,
	}, nil
}
