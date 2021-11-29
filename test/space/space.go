package space

import (
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Option func(space *toolchainv1alpha1.Space)

func WithTargetCluster(name string) Option {
	return func(space *toolchainv1alpha1.Space) {
		space.Spec.TargetCluster = name
	}
}

func WithFinalizer() Option {
	return func(space *toolchainv1alpha1.Space) {
		space.Finalizers = append(space.Finalizers, toolchainv1alpha1.FinalizerName)
	}
}

func WithDeletionTimestamp() Option {
	return func(space *toolchainv1alpha1.Space) {
		now := metav1.NewTime(time.Now())
		space.DeletionTimestamp = &now
	}
}

func NewSpace(name, tier string, options ...Option) *toolchainv1alpha1.Space {
	space := &toolchainv1alpha1.Space{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: test.HostOperatorNs,
		},
		Spec: toolchainv1alpha1.SpaceSpec{
			TierName: tier,
		},
	}
	for _, apply := range options {
		apply(space)
	}
	return space
}
