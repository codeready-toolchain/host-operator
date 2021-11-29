package nstemplateset

import (
	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	clusterResourcesTemplateRef = "basic-clusterresources-abcde00"
	devTemplateRef              = "basic-dev-abcde11"
	codeTemplateRef             = "basic-code-abcde21"
)

type Option func(*toolchainv1alpha1.NSTemplateSet)

func WithReadyCondition() Option {
	return func(nstmplSet *toolchainv1alpha1.NSTemplateSet) {
		nstmplSet.Status.Conditions = []toolchainv1alpha1.Condition{
			{
				Type:   toolchainv1alpha1.ConditionReady,
				Status: corev1.ConditionTrue,
			},
		}
	}
}

func WithNotReadyCondition(reason, message string) Option {
	return func(nstmplSet *toolchainv1alpha1.NSTemplateSet) {
		nstmplSet.Status.Conditions = []toolchainv1alpha1.Condition{
			{
				Type:    toolchainv1alpha1.ConditionReady,
				Status:  corev1.ConditionFalse,
				Reason:  reason,
				Message: message,
			},
		}
	}
}

func NewNSTemplateSet(name string, options ...Option) *toolchainv1alpha1.NSTemplateSet {
	nstmplSet := &toolchainv1alpha1.NSTemplateSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: test.MemberOperatorNs,
			Name:      name,
		},
		Spec: toolchainv1alpha1.NSTemplateSetSpec{
			TierName: "basic",
			ClusterResources: &toolchainv1alpha1.NSTemplateSetClusterResources{
				TemplateRef: clusterResourcesTemplateRef,
			},
			Namespaces: []toolchainv1alpha1.NSTemplateSetNamespace{
				{
					TemplateRef: devTemplateRef,
				},
				{
					TemplateRef: codeTemplateRef,
				},
			},
		},
		Status: toolchainv1alpha1.NSTemplateSetStatus{},
	}
	for _, apply := range options {
		apply(nstmplSet)
	}
	return nstmplSet
}
