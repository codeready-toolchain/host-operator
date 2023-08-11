package spacebindingrequest

import (
	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/gofrs/uuid"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

type Option func(spaceRequest *toolchainv1alpha1.SpaceBindingRequest)

func NewSpaceBindingRequest(name, namespace string, options ...Option) *toolchainv1alpha1.SpaceBindingRequest {
	spaceBindingRequest := &toolchainv1alpha1.SpaceBindingRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			UID:       types.UID(uuid.Must(uuid.NewV4()).String()),
		},
	}
	for _, apply := range options {
		apply(spaceBindingRequest)
	}
	return spaceBindingRequest
}

func WithMUR(mur string) Option {
	return func(spaceBindingRequest *toolchainv1alpha1.SpaceBindingRequest) {
		spaceBindingRequest.Spec.MasterUserRecord = mur
	}
}

func WithSpaceRole(spaceRole string) Option {
	return func(spaceBindingRequest *toolchainv1alpha1.SpaceBindingRequest) {
		spaceBindingRequest.Spec.SpaceRole = spaceRole
	}
}
