package usersignup

import (
	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/controller/nstemplatetier"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func migrateOrFixMurIfNecessary(mur *toolchainv1alpha1.MasterUserRecord, nstemplateTier *toolchainv1alpha1.NSTemplateTier, userSignup *toolchainv1alpha1.UserSignup) (bool, error) {
	changed := false
	for uaIndex, userAccount := range mur.Spec.UserAccounts {
		if userAccount.Spec.NSLimit == "" {
			mur.Spec.UserAccounts[uaIndex].Spec.NSLimit = "default"
			changed = true
		}
		nsTemplateSet := userAccount.Spec.NSTemplateSet
		if nsTemplateSet.TierName == "" {
			mur.Spec.UserAccounts[uaIndex].Spec.NSTemplateSet = NewNSTemplateSetSpec(nstemplateTier)
			changed = true
		}
	}
	// also, ensure that the MUR has a label for each tier in use
	// this label will be needed to select master user record that need to be updated when tier templates changed.
	for _, ua := range mur.Spec.UserAccounts {
		tierName := ua.Spec.NSTemplateSet.TierName
		// only set the label if it is missing.
		if _, ok := mur.Labels[nstemplatetier.TemplateTierHashLabelKey(tierName)]; !ok {
			hash, err := nstemplatetier.ComputeHashForNSTemplateSetSpec(ua.Spec.NSTemplateSet)
			if err != nil {
				return false, err
			}
			if mur.Labels == nil {
				mur.Labels = map[string]string{}
			}
			mur.Labels[nstemplatetier.TemplateTierHashLabelKey(tierName)] = hash
			changed = true
		}
	}
	// migration for CRT-1075: add an annotation with the email address (same as for associated UserSignup resource)
	if mur.Annotations == nil || mur.Annotations[toolchainv1alpha1.MasterUserRecordEmailAnnotationKey] == "" {
		if mur.Annotations == nil {
			mur.Annotations = map[string]string{}
		}
		mur.Annotations[toolchainv1alpha1.MasterUserRecordEmailAnnotationKey] = userSignup.Annotations[toolchainv1alpha1.UserSignupUserEmailAnnotationKey]
		changed = true
	}
	return changed, nil
}

func newMasterUserRecord(userSignup *toolchainv1alpha1.UserSignup, targetCluster string, nstemplateTier *toolchainv1alpha1.NSTemplateTier, compliantUserName string) (*toolchainv1alpha1.MasterUserRecord, error) {
	userAccounts := []toolchainv1alpha1.UserAccountEmbedded{
		{
			TargetCluster: targetCluster,
			Spec: toolchainv1alpha1.UserAccountSpecEmbedded{
				UserAccountSpecBase: toolchainv1alpha1.UserAccountSpecBase{
					NSLimit:       "default",
					NSTemplateSet: NewNSTemplateSetSpec(nstemplateTier),
				},
			},
		},
	}
	hash, err := nstemplatetier.ComputeHashForNSTemplateTier(nstemplateTier)
	if err != nil {
		return nil, err
	}
	labels := map[string]string{
		toolchainv1alpha1.MasterUserRecordOwnerLabelKey:              userSignup.Name,
		nstemplatetier.TemplateTierHashLabelKey(nstemplateTier.Name): hash,
	}
	annotations := map[string]string{
		toolchainv1alpha1.MasterUserRecordEmailAnnotationKey: userSignup.Annotations[toolchainv1alpha1.UserSignupUserEmailAnnotationKey],
	}

	mur := &toolchainv1alpha1.MasterUserRecord{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   userSignup.Namespace,
			Name:        compliantUserName,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: toolchainv1alpha1.MasterUserRecordSpec{
			UserAccounts: userAccounts,
			UserID:       userSignup.Spec.Userid,
		},
	}
	return mur, nil

}

// NewNSTemplateSetSpec initializes a NSTemplateSetSpec from the given NSTemplateTier
func NewNSTemplateSetSpec(nstemplateTier *toolchainv1alpha1.NSTemplateTier) toolchainv1alpha1.NSTemplateSetSpec {
	namespaces := make([]toolchainv1alpha1.NSTemplateSetNamespace, len(nstemplateTier.Spec.Namespaces))
	for i, ns := range nstemplateTier.Spec.Namespaces {
		namespaces[i] = toolchainv1alpha1.NSTemplateSetNamespace{
			TemplateRef: ns.TemplateRef,
		}
	}
	var clusterResources *toolchainv1alpha1.NSTemplateSetClusterResources
	if nstemplateTier.Spec.ClusterResources != nil {
		clusterResources = &toolchainv1alpha1.NSTemplateSetClusterResources{
			TemplateRef: nstemplateTier.Spec.ClusterResources.TemplateRef,
		}
	}
	return toolchainv1alpha1.NSTemplateSetSpec{
		TierName:         nstemplateTier.Name,
		Namespaces:       namespaces,
		ClusterResources: clusterResources,
	}
}
