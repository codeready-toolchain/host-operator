package spacerequest

import (
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Option func(spaceRequest *toolchainv1alpha1.SpaceRequest)

func NewSpaceRequest(name, namespace string, options ...Option) *toolchainv1alpha1.SpaceRequest {
	spaceRequest := &toolchainv1alpha1.SpaceRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
	for _, apply := range options {
		apply(spaceRequest)
	}
	return spaceRequest
}

func WithTierName(tierName string) Option {
	return func(spaceRequest *toolchainv1alpha1.SpaceRequest) {
		spaceRequest.Spec.TierName = tierName
	}
}

func WithTargetClusterRoles(targetClusterRoles []string) Option {
	return func(spaceRequest *toolchainv1alpha1.SpaceRequest) {
		spaceRequest.Spec.TargetClusterRoles = targetClusterRoles
	}
}

func WithDeletionTimestamp() Option {
	return func(spaceRequest *toolchainv1alpha1.SpaceRequest) {
		now := metav1.NewTime(time.Now())
		spaceRequest.DeletionTimestamp = &now
	}
}

func WithFinalizer() Option {
	return func(spaceRequest *toolchainv1alpha1.SpaceRequest) {
		spaceRequest.Finalizers = append(spaceRequest.Finalizers, toolchainv1alpha1.FinalizerName)
	}
}

func WithStatusTargetClusterURL(targetClusterURL string) Option {
	return func(spaceRequest *toolchainv1alpha1.SpaceRequest) {
		spaceRequest.Status.TargetClusterURL = targetClusterURL
	}
}
