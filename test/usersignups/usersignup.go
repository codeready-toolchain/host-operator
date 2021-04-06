package usersignup

import (
	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Option func(*toolchainv1alpha1.UserSignup)

func WithAnnotation(key, value string) Option {
	return func(usersignup *toolchainv1alpha1.UserSignup) {
		usersignup.Annotations[key] = value
	}
}

func NewUserSignup(name string, options ...Option) *toolchainv1alpha1.UserSignup {
	usersignup := &toolchainv1alpha1.UserSignup{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: test.HostOperatorNs,
			Name:      name,
			Annotations: map[string]string{
				toolchainv1alpha1.UserSignupActivationCounterAnnotationKey: "1",
			},
			Finalizers: []string{"finalizer.toolchain.dev.openshift.com"},
		},
	}
	for _, apply := range options {
		apply(usersignup)
	}
	return usersignup
}
