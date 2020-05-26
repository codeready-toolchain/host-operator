package usersignup

import (
	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/templates/nstemplatetiers"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func migrateMurIfNecessary(mur *toolchainv1alpha1.MasterUserRecord, nstemplateTier *toolchainv1alpha1.NSTemplateTier) (bool, *toolchainv1alpha1.MasterUserRecord) {
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
		} else {
			for nsIndex, ns := range nsTemplateSet.Namespaces {
				if ns.TemplateRef == "" {
					ref := nstemplatetiers.NewTierTemplateName(nsTemplateSet.TierName, ns.Type, ns.Revision)
					mur.Spec.UserAccounts[uaIndex].Spec.NSTemplateSet.Namespaces[nsIndex].TemplateRef = ref
					changed = true
				}
			}
			if nsTemplateSet.ClusterResources != nil && nsTemplateSet.ClusterResources.TemplateRef == "" {
				ref := nstemplatetiers.NewTierTemplateName(nsTemplateSet.TierName, toolchainv1alpha1.ClusterResourcesTemplateType, nsTemplateSet.ClusterResources.Revision)
				mur.Spec.UserAccounts[uaIndex].Spec.NSTemplateSet.ClusterResources.TemplateRef = ref
				changed = true
			}
		}
	}
	return changed, mur
}

func newMasterUserRecord(nstemplateTier *toolchainv1alpha1.NSTemplateTier, name, namespace, targetCluster, userSignupName string) *toolchainv1alpha1.MasterUserRecord {
	return newMasterUserRecordWithNsSet(name, namespace, targetCluster, userSignupName, "default", NewNSTemplateSetSpec(nstemplateTier))
}

func NewNSTemplateSetSpec(nstemplateTier *toolchainv1alpha1.NSTemplateTier) toolchainv1alpha1.NSTemplateSetSpec {
	namespaces := make([]toolchainv1alpha1.NSTemplateSetNamespace, len(nstemplateTier.Spec.Namespaces))
	for i, ns := range nstemplateTier.Spec.Namespaces {
		namespaces[i] = toolchainv1alpha1.NSTemplateSetNamespace{
			Type:        ns.Type,
			Revision:    ns.Revision,
			TemplateRef: ns.TemplateRef,
		}
	}
	var clusterResources *toolchainv1alpha1.NSTemplateSetClusterResources
	if nstemplateTier.Spec.ClusterResources != nil {
		clusterResources = &toolchainv1alpha1.NSTemplateSetClusterResources{
			Revision:    nstemplateTier.Spec.ClusterResources.Revision,
			TemplateRef: nstemplateTier.Spec.ClusterResources.TemplateRef,
		}
	}
	return toolchainv1alpha1.NSTemplateSetSpec{
		TierName:         nstemplateTier.Name,
		Namespaces:       namespaces,
		ClusterResources: clusterResources,
	}
}

func newMasterUserRecordWithNsSet(name, namespace, targetCluster, userSignupName, nsLimit string, nsTmplSet toolchainv1alpha1.NSTemplateSetSpec) *toolchainv1alpha1.MasterUserRecord {
	userAccounts := []toolchainv1alpha1.UserAccountEmbedded{
		{
			TargetCluster: targetCluster,
			Spec: toolchainv1alpha1.UserAccountSpecEmbedded{
				UserAccountSpecBase: toolchainv1alpha1.UserAccountSpecBase{
					NSLimit:       nsLimit,
					NSTemplateSet: nsTmplSet,
				},
			},
		},
	}

	labels := map[string]string{toolchainv1alpha1.MasterUserRecordUserIDLabelKey: userSignupName}

	mur := &toolchainv1alpha1.MasterUserRecord{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: toolchainv1alpha1.MasterUserRecordSpec{
			UserAccounts: userAccounts,
			UserID:       userSignupName,
		},
	}
	return mur
}
