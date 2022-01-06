package usersignup

import (
	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	tierutil "github.com/codeready-toolchain/host-operator/controllers/nstemplatetier/util"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func migrateOrFixMurIfNecessary(mur *toolchainv1alpha1.MasterUserRecord, nstemplateTier *toolchainv1alpha1.NSTemplateTier, userSignup *toolchainv1alpha1.UserSignup) (bool, error) {
	changed := false

	// TODO remove this after all users migrated to new SSO Provider client that does not modify the original subject
	if mur.Spec.OriginalSub != userSignup.Spec.OriginalSub {
		mur.Spec.OriginalSub = userSignup.Spec.OriginalSub
		changed = true
	}

	for uaIndex, userAccount := range mur.Spec.UserAccounts {
		if userAccount.Spec.NSLimit == "" {
			mur.Spec.UserAccounts[uaIndex].Spec.NSLimit = "default"
			changed = true
		}
		nsTemplateSet := userAccount.Spec.NSTemplateSet
		if nsTemplateSet != nil && nsTemplateSet.TierName == "" {
			mur.Spec.UserAccounts[uaIndex].Spec.NSTemplateSet = NewNSTemplateSetSpec(nstemplateTier)
			changed = true
		}
	}
	// also, ensure that the MUR has a label for each tier in use
	// this label will be needed to select master user record that need to be updated when tier templates changed.
	for _, ua := range mur.Spec.UserAccounts {
		// skip if no NSTemplateSet defined on the UserAccount
		if ua.Spec.NSTemplateSet == nil {
			continue
		}
		tierName := ua.Spec.NSTemplateSet.TierName
		// only set the label if it is missing.
		if _, ok := mur.Labels[tierutil.TemplateTierHashLabelKey(tierName)]; !ok {
			hash, err := tierutil.ComputeHashForNSTemplateSetSpec(*ua.Spec.NSTemplateSet)
			if err != nil {
				return false, err
			}
			if mur.Labels == nil {
				mur.Labels = map[string]string{}
			}
			mur.Labels[tierutil.TemplateTierHashLabelKey(tierName)] = hash
			changed = true
		}
	}
	// TODO: remove this after UserAccount.NStemplateSet has been removed (CRT-1321)
	if len(mur.Spec.UserAccounts) > 0 && mur.Spec.UserAccounts[0].Spec.NSTemplateSet != nil &&
		mur.Spec.TierName != mur.Spec.UserAccounts[0].Spec.NSTemplateSet.TierName {
		mur.Spec.TierName = mur.Spec.UserAccounts[0].Spec.NSTemplateSet.TierName
		changed = true
	}
	// TODO: remove this after UserAccount.NStemplateSet has been removed (CRT-1321)
	if len(mur.Spec.UserAccounts) > 0 && mur.Spec.UserAccounts[0].Spec.NSTemplateSet != nil &&
		mur.Spec.TierName != mur.Spec.UserAccounts[0].Spec.NSTemplateSet.TierName {
		mur.Spec.TierName = mur.Spec.UserAccounts[0].Spec.NSTemplateSet.TierName
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
	hash, err := tierutil.ComputeHashForNSTemplateTier(nstemplateTier)
	if err != nil {
		return nil, err
	}
	labels := map[string]string{
		toolchainv1alpha1.MasterUserRecordOwnerLabelKey:        userSignup.Name,
		tierutil.TemplateTierHashLabelKey(nstemplateTier.Name): hash,
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
			OriginalSub:  userSignup.Spec.OriginalSub,
			TierName:     nstemplateTier.Name,
		},
	}
	return mur, nil

}

// NewNSTemplateSetSpec initializes a NSTemplateSetSpec from the given NSTemplateTier
func NewNSTemplateSetSpec(nstemplateTier *toolchainv1alpha1.NSTemplateTier) *toolchainv1alpha1.NSTemplateSetSpec {
	namespaces := make([]toolchainv1alpha1.NSTemplateSetNamespace, len(nstemplateTier.Spec.Namespaces))
	for i, ns := range nstemplateTier.Spec.Namespaces {
		namespaces[i] = toolchainv1alpha1.NSTemplateSetNamespace(ns)
	}
	var clusterResources *toolchainv1alpha1.NSTemplateSetClusterResources
	if nstemplateTier.Spec.ClusterResources != nil {
		clusterResources = &toolchainv1alpha1.NSTemplateSetClusterResources{
			TemplateRef: nstemplateTier.Spec.ClusterResources.TemplateRef,
		}
	}
	return &toolchainv1alpha1.NSTemplateSetSpec{
		TierName:         nstemplateTier.Name,
		Namespaces:       namespaces,
		ClusterResources: clusterResources,
	}
}
