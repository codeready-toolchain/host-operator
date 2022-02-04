package usersignup

import (
	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func newSpace(userSignup *toolchainv1alpha1.UserSignup, targetCluster targetCluster, compliantUserName, tier string) *toolchainv1alpha1.Space {
	labels := map[string]string{
		toolchainv1alpha1.SpaceCreatorLabelKey: compliantUserName,
	}

	space := &toolchainv1alpha1.Space{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: userSignup.Namespace,
			Name:      compliantUserName,
			Labels:    labels,
		},
		Spec: toolchainv1alpha1.SpaceSpec{
			TargetCluster: targetCluster.getClusterName(),
			TierName:      tier,
		},
	}
	return space
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
