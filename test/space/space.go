package space

import (
	"fmt"
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	tierutil "github.com/codeready-toolchain/host-operator/controllers/nstemplatetier/util"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type Option func(space *toolchainv1alpha1.Space)

func WithoutSpecTargetCluster() Option {
	return func(space *toolchainv1alpha1.Space) {
		space.Spec.TargetCluster = ""
	}
}

func WithSpecTargetCluster(name string) Option {
	return func(space *toolchainv1alpha1.Space) {
		space.Spec.TargetCluster = name
	}
}

func WithLabel(key, value string) Option {
	return func(space *toolchainv1alpha1.Space) {
		if space.ObjectMeta.Labels == nil {
			space.ObjectMeta.Labels = map[string]string{}
		}
		space.ObjectMeta.Labels[key] = value
	}
}

func WithTierName(tierName string) Option {
	return func(space *toolchainv1alpha1.Space) {
		space.Spec.TierName = tierName
	}
}

func WithTierHashLabelFor(tier *toolchainv1alpha1.NSTemplateTier) Option {
	return func(space *toolchainv1alpha1.Space) {
		hash, _ := tierutil.ComputeHashForNSTemplateTier(tier) // we can assume the JSON marshalling will always work
		if space.ObjectMeta.Labels == nil {
			space.ObjectMeta.Labels = map[string]string{}
		}
		space.ObjectMeta.Labels[tierutil.TemplateTierHashLabelKey(tier.Name)] = hash
	}
}

func WithTierNameAndHashLabelFor(tier *toolchainv1alpha1.NSTemplateTier) Option {
	return func(space *toolchainv1alpha1.Space) {
		WithTierName(tier.Name)(space)
		WithTierHashLabelFor(tier)(space)
	}
}

func WithStatusTargetCluster(name string) Option {
	return func(space *toolchainv1alpha1.Space) {
		space.Status.TargetCluster = name
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

func WithCondition(c toolchainv1alpha1.Condition) Option {
	return func(space *toolchainv1alpha1.Space) {
		space.Status.Conditions = append(space.Status.Conditions, c)
	}
}

func WithCreatorLabel(creator string) Option {
	return func(space *toolchainv1alpha1.Space) {
		if space.Labels == nil {
			space.Labels = map[string]string{}
		}
		space.Labels[toolchainv1alpha1.SpaceCreatorLabelKey] = creator
	}
}

func WithCreationTimestamp(t time.Time) Option {
	return func(space *toolchainv1alpha1.Space) {
		space.CreationTimestamp = metav1.NewTime(t)
	}
}

func WithStateLabel(stateValue string) Option {
	return func(space *toolchainv1alpha1.Space) {
		if space.Labels == nil {
			space.Labels = map[string]string{}
		}
		space.Labels[toolchainv1alpha1.SpaceStateLabelKey] = stateValue
	}
}

func CreatedBefore(before time.Duration) Option {
	return func(space *toolchainv1alpha1.Space) {
		space.ObjectMeta.CreationTimestamp = metav1.Time{Time: time.Now().Add(-before)}
	}
}

func NewSpace(name string, options ...Option) *toolchainv1alpha1.Space {
	space := &toolchainv1alpha1.Space{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: test.HostOperatorNs,
		},
		Spec: toolchainv1alpha1.SpaceSpec{
			TierName: "basic",
		},
	}
	for _, apply := range options {
		apply(space)
	}
	return space
}

func NewSpaces(size int, nameFmt string, options ...Option) []runtime.Object {
	murs := make([]runtime.Object, size)
	for i := 0; i < size; i++ {
		murs[i] = NewSpace(fmt.Sprintf(nameFmt, i), options...)
	}
	return murs
}
