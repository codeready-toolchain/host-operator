package test

import (
	"crypto/md5"
	"encoding/hex"
	"time"

	"github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"

	uuid "github.com/satori/go.uuid"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func WithTargetCluster(targetCluster string) UserSignupModifier {
	return func(userSignup *v1alpha1.UserSignup) {
		userSignup.Spec.TargetCluster = targetCluster
	}
}

func Approved() UserSignupModifier {
	return func(userSignup *v1alpha1.UserSignup) {
		userSignup.Spec.Approved = true
	}
}

func Deactivated() UserSignupModifier {
	return func(userSignup *v1alpha1.UserSignup) {
		userSignup.Spec.Deactivated = true
	}
}

func VerificationRequired() UserSignupModifier {
	return func(userSignup *v1alpha1.UserSignup) {
		userSignup.Spec.VerificationRequired = true
	}
}

func WithUsername(username string) UserSignupModifier {
	return func(userSignup *v1alpha1.UserSignup) {
		userSignup.Spec.Username = username
	}
}

func WithStateLabel(stateValue string) UserSignupModifier {
	return func(userSignup *v1alpha1.UserSignup) {
		userSignup.Labels[v1alpha1.UserSignupStateLabelKey] = stateValue
	}
}

func CreatedBefore(before time.Duration) UserSignupModifier {
	return func(userSignup *v1alpha1.UserSignup) {
		userSignup.ObjectMeta.CreationTimestamp = metav1.Time{Time: time.Now().Add(-before)}
	}
}

type UserSignupModifier func(*v1alpha1.UserSignup)

func NewUserSignup(modifiers ...UserSignupModifier) *v1alpha1.UserSignup {
	signup := &v1alpha1.UserSignup{
		ObjectMeta: NewUserSignupObjectMeta("", "foo@redhat.com"),
		Spec: v1alpha1.UserSignupSpec{
			UserID:   "UserID123",
			Username: "foo@redhat.com",
		},
	}
	for _, modify := range modifiers {
		modify(signup)
	}
	return signup
}

func NewUserSignupObjectMeta(name, email string) metav1.ObjectMeta {
	if name == "" {
		name = uuid.NewV4().String()
	}

	md5hash := md5.New()
	// Ignore the error, as this implementation cannot return one
	_, _ = md5hash.Write([]byte(email))
	emailHash := hex.EncodeToString(md5hash.Sum(nil))

	return metav1.ObjectMeta{
		Name:      name,
		Namespace: test.HostOperatorNs,
		Annotations: map[string]string{
			toolchainv1alpha1.UserSignupUserEmailAnnotationKey: email,
		},
		Labels: map[string]string{
			toolchainv1alpha1.UserSignupUserEmailHashLabelKey: emailHash,
		},
		CreationTimestamp: metav1.Now(),
	}
}
