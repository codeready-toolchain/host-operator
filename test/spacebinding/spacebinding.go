package spacebinding

import (
	"fmt"
	"time"

	"github.com/codeready-toolchain/api/api/v1alpha1"
	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Option func(spaceRequest *toolchainv1alpha1.SpaceBinding)

func NewSpaceBinding(mur, space, spaceRole, creator string, options ...Option) *v1alpha1.SpaceBinding {
	sb := &v1alpha1.SpaceBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", mur, space),
			Namespace: test.HostOperatorNs,
			Labels: map[string]string{
				v1alpha1.SpaceCreatorLabelKey:                 creator,
				v1alpha1.SpaceBindingMasterUserRecordLabelKey: mur,
				v1alpha1.SpaceBindingSpaceLabelKey:            space,
			},
		},
		Spec: v1alpha1.SpaceBindingSpec{
			MasterUserRecord: mur,
			Space:            space,
			SpaceRole:        spaceRole,
		},
	}

	for _, apply := range options {
		apply(sb)
	}
	return sb
}

func WithSpaceBindingRequest(sbr *toolchainv1alpha1.SpaceBindingRequest) Option {
	return func(spaceBinding *toolchainv1alpha1.SpaceBinding) {
		spaceBinding.Labels[toolchainv1alpha1.SpaceBindingRequestLabelKey] = sbr.Name
		spaceBinding.Labels[toolchainv1alpha1.SpaceBindingRequestNamespaceLabelKey] = sbr.Namespace
	}
}

func WithDeletionTimestamp() Option {
	return func(spaceBinding *toolchainv1alpha1.SpaceBinding) {
		now := metav1.NewTime(time.Now())
		spaceBinding.DeletionTimestamp = &now
	}
}

func Provisioning() toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:   toolchainv1alpha1.ConditionReady,
		Status: corev1.ConditionFalse,
		Reason: toolchainv1alpha1.SpaceBindingProvisioningReason,
	}
}

func Ready() toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:   toolchainv1alpha1.ConditionReady,
		Status: corev1.ConditionTrue,
		Reason: toolchainv1alpha1.SpaceBindingProvisionedReason,
	}
}

func Terminating() toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:   toolchainv1alpha1.ConditionReady,
		Status: corev1.ConditionFalse,
		Reason: toolchainv1alpha1.SpaceBindingTerminatingReason,
	}
}

func TerminatingFailed(msg string) toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:    toolchainv1alpha1.ConditionReady,
		Status:  corev1.ConditionFalse,
		Reason:  toolchainv1alpha1.SpaceBindingTerminatingFailedReason,
		Message: msg,
	}
}

func ProvisioningFailed(msg string) toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:    toolchainv1alpha1.ConditionReady,
		Status:  corev1.ConditionFalse,
		Reason:  toolchainv1alpha1.SpaceBindingProvisioningFailedReason,
		Message: msg,
	}
}
