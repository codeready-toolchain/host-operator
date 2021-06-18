package test

import (
	"crypto/md5"
	"encoding/hex"
	"time"

	"github.com/codeready-toolchain/toolchain-common/pkg/condition"
	"github.com/codeready-toolchain/toolchain-common/pkg/states"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"

	uuid "github.com/satori/go.uuid"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func WithTargetCluster(targetCluster string) UserSignupModifier {
	return func(userSignup *toolchainv1alpha1.UserSignup) {
		userSignup.Spec.TargetCluster = targetCluster
	}
}

func Approved() UserSignupModifier {
	return func(userSignup *toolchainv1alpha1.UserSignup) {
		states.SetApproved(userSignup, true)
	}
}

func ApprovedAutomatically(before time.Duration) UserSignupModifier {
	return func(userSignup *toolchainv1alpha1.UserSignup) {
		states.SetApproved(userSignup, true)
		userSignup.Status.Conditions = condition.AddStatusConditions(userSignup.Status.Conditions,
			toolchainv1alpha1.Condition{
				Type:               toolchainv1alpha1.UserSignupApproved,
				Status:             v1.ConditionTrue,
				Reason:             "ApprovedAutomatically",
				LastTransitionTime: metav1.Time{Time: time.Now().Add(-before)},
			})
	}
}

func Deactivated() UserSignupModifier {
	return func(userSignup *toolchainv1alpha1.UserSignup) {
		states.SetDeactivated(userSignup, true)
	}
}

func DeactivatedWithLastTransitionTime(before time.Duration) UserSignupModifier {
	return func(userSignup *toolchainv1alpha1.UserSignup) {
		states.SetDeactivated(userSignup, true)

		deactivatedCondition := toolchainv1alpha1.Condition{
			Type:               toolchainv1alpha1.UserSignupComplete,
			Status:             v1.ConditionTrue,
			Reason:             toolchainv1alpha1.UserSignupUserDeactivatedReason,
			LastTransitionTime: metav1.Time{Time: time.Now().Add(-before)},
		}

		userSignup.Status.Conditions = condition.AddStatusConditions(userSignup.Status.Conditions, deactivatedCondition)
	}
}

func VerificationRequired(before time.Duration) UserSignupModifier {
	return func(userSignup *toolchainv1alpha1.UserSignup) {
		states.SetVerificationRequired(userSignup, true)

		verificationRequired := toolchainv1alpha1.Condition{
			Type:               toolchainv1alpha1.UserSignupComplete,
			Status:             v1.ConditionFalse,
			Reason:             toolchainv1alpha1.UserSignupVerificationRequiredReason,
			LastTransitionTime: metav1.Time{Time: time.Now().Add(-before)},
		}

		userSignup.Status.Conditions = condition.AddStatusConditions(userSignup.Status.Conditions, verificationRequired)

	}
}

func WithUsername(username string) UserSignupModifier {
	return func(userSignup *toolchainv1alpha1.UserSignup) {
		userSignup.Spec.Username = username
	}
}

func WithLabel(key, value string) UserSignupModifier {
	return func(userSignup *toolchainv1alpha1.UserSignup) {
		userSignup.Labels[key] = value
	}
}

func WithStateLabel(stateValue string) UserSignupModifier {
	return func(userSignup *toolchainv1alpha1.UserSignup) {
		userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey] = stateValue
	}
}

func WithEmail(email string) UserSignupModifier {
	return func(userSignup *toolchainv1alpha1.UserSignup) {
		md5hash := md5.New()
		// Ignore the error, as this implementation cannot return one
		_, _ = md5hash.Write([]byte(email))
		emailHash := hex.EncodeToString(md5hash.Sum(nil))
		userSignup.ObjectMeta.Labels[toolchainv1alpha1.UserSignupUserEmailHashLabelKey] = emailHash
		userSignup.ObjectMeta.Annotations[toolchainv1alpha1.UserSignupUserEmailAnnotationKey] = email
	}
}

func SignupComplete(reason string) UserSignupModifier {
	return func(userSignup *toolchainv1alpha1.UserSignup) {
		userSignup.Status.Conditions = condition.AddStatusConditions(userSignup.Status.Conditions,
			toolchainv1alpha1.Condition{
				Type:   toolchainv1alpha1.UserSignupComplete,
				Status: v1.ConditionTrue,
				Reason: reason,
			})
	}
}

func CreatedBefore(before time.Duration) UserSignupModifier {
	return func(userSignup *toolchainv1alpha1.UserSignup) {
		userSignup.ObjectMeta.CreationTimestamp = metav1.Time{Time: time.Now().Add(-before)}
	}
}

func BeingDeleted() UserSignupModifier {
	return func(userSignup *toolchainv1alpha1.UserSignup) {
		userSignup.ObjectMeta.DeletionTimestamp = &metav1.Time{Time: time.Now()}
	}
}

func WithAnnotation(key, value string) UserSignupModifier {
	return func(userSignup *toolchainv1alpha1.UserSignup) {
		userSignup.Annotations[key] = value
	}
}
func WithoutAnnotation(key string) UserSignupModifier {
	return func(userSignup *toolchainv1alpha1.UserSignup) {
		delete(userSignup.Annotations, key)
	}
}

func WithoutAnnotations() UserSignupModifier {
	return func(userSignup *toolchainv1alpha1.UserSignup) {
		userSignup.Annotations = map[string]string{}
	}
}

func WithName(name string) UserSignupModifier {
	return func(userSignup *toolchainv1alpha1.UserSignup) {
		userSignup.Name = name
		userSignup.Spec.Username = name
	}
}

type UserSignupModifier func(*toolchainv1alpha1.UserSignup)

func NewUserSignup(modifiers ...UserSignupModifier) *toolchainv1alpha1.UserSignup {
	signup := &toolchainv1alpha1.UserSignup{
		ObjectMeta: NewUserSignupObjectMeta("", "foo@redhat.com"),
		Spec: toolchainv1alpha1.UserSignupSpec{
			Userid:   "UserID123",
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
