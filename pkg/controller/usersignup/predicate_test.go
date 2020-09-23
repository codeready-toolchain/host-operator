package usersignup

import (
	"testing"

	"github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	uuid "github.com/satori/go.uuid"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

func TestUserSignupChangedPredicate(t *testing.T) {
	// when
	pred := &UserSignupChangedPredicate{}
	userSignupName := uuid.NewV4().String()

	userSignupOld := &v1alpha1.UserSignup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      userSignupName,
			Namespace: operatorNamespace,
			Annotations: map[string]string{
				toolchainv1alpha1.UserSignupUserEmailAnnotationKey: "foo@redhat.com",
			},
			Labels: map[string]string{
				toolchainv1alpha1.UserSignupUserEmailHashLabelKey: "fd2addbd8d82f0d2dc088fa122377eaa",
			},
			Generation: 1,
		},
		Spec: v1alpha1.UserSignupSpec{
			Username: "foo@redhat.com",
		},
	}

	userSignupNewNotChanged := &v1alpha1.UserSignup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      userSignupName,
			Namespace: operatorNamespace,
			Annotations: map[string]string{
				toolchainv1alpha1.UserSignupUserEmailAnnotationKey: "foo@redhat.com",
			},
			Labels: map[string]string{
				toolchainv1alpha1.UserSignupUserEmailHashLabelKey: "fd2addbd8d82f0d2dc088fa122377eaa",
			},
			Generation: 1,
		},
		Spec: v1alpha1.UserSignupSpec{
			Username: "alice.mayweather.doe@redhat.com",
		},
	}

	userSignupNewChanged := &v1alpha1.UserSignup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      userSignupName,
			Namespace: operatorNamespace,
			Annotations: map[string]string{
				toolchainv1alpha1.UserSignupUserEmailAnnotationKey: "alice.mayweather.doe@redhat.com",
			},
			Labels: map[string]string{
				toolchainv1alpha1.UserSignupUserEmailHashLabelKey: "747a250430df0c7976bf2363ebb4014a",
			},
			Generation: 2,
		},
		Spec: v1alpha1.UserSignupSpec{
			Username: "alice.mayweather.doe@redhat.com",
		},
	}

	t.Run("test UserSignupChangedPredicate returns false when MetaOld not set", func(t *testing.T) {
		e := event.UpdateEvent{
			MetaOld:   nil,
			ObjectOld: userSignupOld,
			MetaNew:   userSignupNewNotChanged.ObjectMeta.GetObjectMeta(),
			ObjectNew: userSignupNewNotChanged,
		}
		require.False(t, pred.Update(e))
	})
	t.Run("test UserSignupChangedPredicate returns false when ObjectOld not set", func(t *testing.T) {
		e := event.UpdateEvent{
			MetaOld:   userSignupOld.ObjectMeta.GetObjectMeta(),
			ObjectOld: nil,
			MetaNew:   userSignupNewNotChanged.ObjectMeta.GetObjectMeta(),
			ObjectNew: userSignupNewNotChanged,
		}
		require.False(t, pred.Update(e))
	})
	t.Run("test UserSignupChangedPredicate returns false when ObjectNew not set", func(t *testing.T) {
		e := event.UpdateEvent{
			MetaOld:   userSignupOld.ObjectMeta.GetObjectMeta(),
			ObjectOld: userSignupOld,
			MetaNew:   userSignupNewNotChanged.ObjectMeta.GetObjectMeta(),
			ObjectNew: nil,
		}
		require.False(t, pred.Update(e))
	})
	t.Run("test UserSignupChangedPredicate returns false when MetaNew not set", func(t *testing.T) {
		e := event.UpdateEvent{
			MetaOld:   userSignupOld.ObjectMeta.GetObjectMeta(),
			ObjectOld: userSignupOld,
			MetaNew:   nil,
			ObjectNew: userSignupNewNotChanged,
		}
		require.False(t, pred.Update(e))
	})
	t.Run("test UserSignupChangedPredicate returns false when generation unchanged and annoations unchanged", func(t *testing.T) {
		e := event.UpdateEvent{
			MetaOld:   userSignupOld.ObjectMeta.GetObjectMeta(),
			ObjectOld: userSignupOld,
			MetaNew:   userSignupNewNotChanged.ObjectMeta.GetObjectMeta(),
			ObjectNew: userSignupNewNotChanged,
		}
		require.False(t, pred.Update(e))
	})
	t.Run("test UserSignupChangedPredicate returns true when generation changed", func(t *testing.T) {
		e := event.UpdateEvent{
			MetaOld:   userSignupOld.ObjectMeta.GetObjectMeta(),
			ObjectOld: userSignupOld,
			MetaNew:   userSignupNewChanged.ObjectMeta.GetObjectMeta(),
			ObjectNew: userSignupNewChanged,
		}
		require.True(t, pred.Update(e))
	})
}
