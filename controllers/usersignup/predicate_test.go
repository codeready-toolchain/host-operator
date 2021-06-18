package usersignup

import (
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	. "github.com/codeready-toolchain/host-operator/test"
	hostconfig "github.com/codeready-toolchain/host-operator/test/config"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	testconfig "github.com/codeready-toolchain/toolchain-common/pkg/test/config"

	uuid "github.com/satori/go.uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

func TestUserSignupChangedPredicate(t *testing.T) {
	// when
	pred := &UserSignupChangedPredicate{}
	userSignupName := uuid.NewV4().String()

	userSignupOld := &toolchainv1alpha1.UserSignup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      userSignupName,
			Namespace: test.HostOperatorNs,
			Annotations: map[string]string{
				toolchainv1alpha1.UserSignupUserEmailAnnotationKey: "foo@redhat.com",
			},
			Labels: map[string]string{
				toolchainv1alpha1.UserSignupUserEmailHashLabelKey: "fd2addbd8d82f0d2dc088fa122377eaa",
			},
			Generation: 1,
		},
		Spec: toolchainv1alpha1.UserSignupSpec{
			Username: "foo@redhat.com",
		},
	}

	userSignupNewNotChanged := &toolchainv1alpha1.UserSignup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      userSignupName,
			Namespace: test.HostOperatorNs,
			Annotations: map[string]string{
				toolchainv1alpha1.UserSignupUserEmailAnnotationKey: "foo@redhat.com",
			},
			Labels: map[string]string{
				toolchainv1alpha1.UserSignupUserEmailHashLabelKey: "fd2addbd8d82f0d2dc088fa122377eaa",
			},
			Generation: 1,
		},
		Spec: toolchainv1alpha1.UserSignupSpec{
			Username: "alice.mayweather.doe@redhat.com",
		},
	}

	userSignupNewChanged := &toolchainv1alpha1.UserSignup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      userSignupName,
			Namespace: test.HostOperatorNs,
			Annotations: map[string]string{
				toolchainv1alpha1.UserSignupUserEmailAnnotationKey: "alice.mayweather.doe@redhat.com",
			},
			Labels: map[string]string{
				toolchainv1alpha1.UserSignupUserEmailHashLabelKey: "747a250430df0c7976bf2363ebb4014a",
			},
			Generation: 2,
		},
		Spec: toolchainv1alpha1.UserSignupSpec{
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

func TestAutomaticApprovalPredicateWhenApprovalIsEnabled(t *testing.T) {
	// given
	cl := test.NewFakeClient(t, hostconfig.NewToolchainConfigWithReset(t, testconfig.AutomaticApproval().Enabled()))
	predicate := OnlyWhenAutomaticApprovalIsEnabled{
		client: cl,
	}
	toolchainStatus := NewToolchainStatus()

	t.Run("update", func(t *testing.T) {
		t.Run("when all fields are set", func(t *testing.T) {
			// given
			updateEvent := event.UpdateEvent{
				MetaOld:   toolchainStatus.GetObjectMeta(),
				ObjectOld: toolchainStatus,
				MetaNew:   toolchainStatus.GetObjectMeta(),
				ObjectNew: toolchainStatus,
			}

			// when
			shouldTriggerReconcile := predicate.Update(updateEvent)

			// then
			assert.True(t, shouldTriggerReconcile)
		})

		t.Run("when MetaOld field is missing", func(t *testing.T) {
			// given
			updateEvent := event.UpdateEvent{
				ObjectOld: toolchainStatus,
				MetaNew:   toolchainStatus.GetObjectMeta(),
				ObjectNew: toolchainStatus,
			}

			// when
			shouldTriggerReconcile := predicate.Update(updateEvent)

			// then
			assert.False(t, shouldTriggerReconcile)
		})

		t.Run("when ObjectOld is missing", func(t *testing.T) {
			// given
			updateEvent := event.UpdateEvent{
				MetaOld:   toolchainStatus.GetObjectMeta(),
				MetaNew:   toolchainStatus.GetObjectMeta(),
				ObjectNew: toolchainStatus,
			}

			// when
			shouldTriggerReconcile := predicate.Update(updateEvent)

			// then
			assert.False(t, shouldTriggerReconcile)
		})

		t.Run("when MetaNew is missing", func(t *testing.T) {
			// given
			updateEvent := event.UpdateEvent{
				MetaOld:   toolchainStatus.GetObjectMeta(),
				ObjectOld: toolchainStatus,
				ObjectNew: toolchainStatus,
			}

			// when
			shouldTriggerReconcile := predicate.Update(updateEvent)

			// then
			assert.False(t, shouldTriggerReconcile)
		})

		t.Run("when ObjectNew is missing", func(t *testing.T) {
			// given
			updateEvent := event.UpdateEvent{
				MetaOld:   toolchainStatus.GetObjectMeta(),
				ObjectOld: toolchainStatus,
				MetaNew:   toolchainStatus.GetObjectMeta(),
			}

			// when
			shouldTriggerReconcile := predicate.Update(updateEvent)

			// then
			assert.False(t, shouldTriggerReconcile)
		})
	})

	t.Run("create", func(t *testing.T) {
		// given
		createEvent := event.CreateEvent{
			Meta:   toolchainStatus.GetObjectMeta(),
			Object: toolchainStatus,
		}

		// when
		shouldTriggerReconcile := predicate.Create(createEvent)

		// then
		assert.False(t, shouldTriggerReconcile)
	})

	t.Run("delete", func(t *testing.T) {
		// given
		deleteEvent := event.DeleteEvent{
			Meta:   toolchainStatus.GetObjectMeta(),
			Object: toolchainStatus,
		}

		// when
		shouldTriggerReconcile := predicate.Delete(deleteEvent)

		// then
		assert.False(t, shouldTriggerReconcile)
	})

	t.Run("generic", func(t *testing.T) {
		// given
		genericEvent := event.GenericEvent{
			Meta:   toolchainStatus.GetObjectMeta(),
			Object: toolchainStatus,
		}

		// when
		shouldTriggerReconcile := predicate.Generic(genericEvent)

		// then
		assert.True(t, shouldTriggerReconcile)
	})
}

func TestAutomaticApprovalPredicateWhenApprovalIsNotEnabled(t *testing.T) {
	// given
	cl := test.NewFakeClient(t, hostconfig.NewToolchainConfigWithReset(t, testconfig.AutomaticApproval().Disabled()))
	predicate := OnlyWhenAutomaticApprovalIsEnabled{
		client: cl,
	}
	toolchainStatus := NewToolchainStatus()

	t.Run("update", func(t *testing.T) {
		// given
		updateEvent := event.UpdateEvent{
			MetaOld:   toolchainStatus.GetObjectMeta(),
			ObjectOld: toolchainStatus,
			MetaNew:   toolchainStatus.GetObjectMeta(),
			ObjectNew: toolchainStatus,
		}

		// when
		shouldTriggerReconcile := predicate.Update(updateEvent)

		// then
		assert.False(t, shouldTriggerReconcile)
	})

	t.Run("generic", func(t *testing.T) {
		// given
		genericEvent := event.GenericEvent{
			Meta:   toolchainStatus.GetObjectMeta(),
			Object: toolchainStatus,
		}

		// when
		shouldTriggerReconcile := predicate.Generic(genericEvent)

		// then
		assert.False(t, shouldTriggerReconcile)
	})
}
