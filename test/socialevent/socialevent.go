package socialevent

import (
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NewSocialEvent returns a new SocialEvent with the given name and tiername,
// starting: now
// ending: 1hr later
// max attendees: 10
func NewSocialEvent(name, tierName string, options ...Option) *toolchainv1alpha1.SocialEvent {
	se := &toolchainv1alpha1.SocialEvent{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: test.HostOperatorNs,
			Name:      name,
		},
		Spec: toolchainv1alpha1.SocialEventSpec{
			Tier:         tierName,
			StartTime:    metav1.Now(),
			EndTime:      metav1.NewTime(time.Now().Add(1 * time.Hour)),
			MaxAttendees: 10,
		},
	}
	for _, apply := range options {
		apply(se)
	}
	return se
}

type Option func(*toolchainv1alpha1.SocialEvent)

func WithConditions(c ...toolchainv1alpha1.Condition) Option {
	return func(se *toolchainv1alpha1.SocialEvent) {
		se.Status.Conditions = c
	}
}
