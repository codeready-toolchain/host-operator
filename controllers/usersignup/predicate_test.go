package usersignup

import (
	"testing"

	"github.com/gofrs/uuid"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	. "github.com/codeready-toolchain/host-operator/test"

	commonconfig "github.com/codeready-toolchain/toolchain-common/pkg/configuration"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	testconfig "github.com/codeready-toolchain/toolchain-common/pkg/test/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

func TestUserSignupChangedPredicate(t *testing.T) {
	// when
	pred := &UserSignupChangedPredicate{}
	userSignupName := uuid.Must(uuid.NewV4()).String()

	userSignupOld := &toolchainv1alpha1.UserSignup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      userSignupName,
			Namespace: test.HostOperatorNs,
			Annotations: map[string]string{
				toolchainv1alpha1.UserSignupUserEmailAnnotationKey: "jane.doe@redhat.com",
			},
			Labels: map[string]string{
				toolchainv1alpha1.UserSignupUserEmailHashLabelKey: "fd2addbd8d82f0d2dc088fa122377eaa",
			},
			Generation: 1,
		},
		Spec: toolchainv1alpha1.UserSignupSpec{
			Username: "jane.doe@redhat.com",
		},
	}

	userSignupUnchanged := &toolchainv1alpha1.UserSignup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      userSignupName,
			Namespace: test.HostOperatorNs,
			Annotations: map[string]string{
				toolchainv1alpha1.UserSignupUserEmailAnnotationKey: "jane.doe@redhat.com",
				migrationInProgressAnnotationName:                  "true",
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

	userSignupWithGenerationChanged := &toolchainv1alpha1.UserSignup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      userSignupName,
			Namespace: test.HostOperatorNs,
			Annotations: map[string]string{
				toolchainv1alpha1.UserSignupUserEmailAnnotationKey: "jane.doe@redhat.com",
				migrationInProgressAnnotationName:                  "true",
			},
			Labels: map[string]string{
				toolchainv1alpha1.UserSignupUserEmailHashLabelKey: "fd2addbd8d82f0d2dc088fa122377eaa",
			},
			Generation: 2,
		},
		Spec: toolchainv1alpha1.UserSignupSpec{
			Username: "alice.mayweather.doe@redhat.com",
		},
	}

	userSignupWithEmailHashLabelChanged := &toolchainv1alpha1.UserSignup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      userSignupName,
			Namespace: test.HostOperatorNs,
			Annotations: map[string]string{
				toolchainv1alpha1.UserSignupUserEmailAnnotationKey: "jane.doe@redhat.com",
				migrationInProgressAnnotationName:                  "true",
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

	userSignupWithEmailAnnotationChanged := &toolchainv1alpha1.UserSignup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      userSignupName,
			Namespace: test.HostOperatorNs,
			Annotations: map[string]string{
				toolchainv1alpha1.UserSignupUserEmailAnnotationKey: "alice.mayweather.doe@redhat.com",
				migrationInProgressAnnotationName:                  "true",
			},
			Labels: map[string]string{
				toolchainv1alpha1.UserSignupUserEmailHashLabelKey: "fd2addbd8d82f0d2dc088fa122377eaa",
			},
			Generation: 2,
		},
		Spec: toolchainv1alpha1.UserSignupSpec{
			Username: "alice.mayweather.doe@redhat.com",
		},
	}

	userSignupWithMigrationInProgressAnnotationRemoved := &toolchainv1alpha1.UserSignup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      userSignupName,
			Namespace: test.HostOperatorNs,
			Annotations: map[string]string{
				toolchainv1alpha1.UserSignupUserEmailAnnotationKey: "jane.doe@redhat.com",
			},
			Labels: map[string]string{
				toolchainv1alpha1.UserSignupUserEmailHashLabelKey: "fd2addbd8d82f0d2dc088fa122377eaa",
			},
			Generation: 2,
		},
		Spec: toolchainv1alpha1.UserSignupSpec{
			Username: "alice.mayweather.doe@redhat.com",
		},
	}

	t.Run("test UserSignupChangedPredicate returns false when ObjectOld not set", func(t *testing.T) {
		e := event.UpdateEvent{
			ObjectOld: nil,
			ObjectNew: userSignupUnchanged,
		}
		require.False(t, pred.Update(e))
	})

	t.Run("test UserSignupChangedPredicate returns false when ObjectNew not set", func(t *testing.T) {
		e := event.UpdateEvent{
			ObjectOld: userSignupOld,
			ObjectNew: nil,
		}
		require.False(t, pred.Update(e))
	})

	t.Run("test UserSignupChangedPredicate returns false when resource unchanged", func(t *testing.T) {
		e := event.UpdateEvent{
			ObjectOld: userSignupOld,
			ObjectNew: userSignupUnchanged,
		}
		require.False(t, pred.Update(e))
	})

	t.Run("test UserSignupChangedPredicate returns true when generation changed", func(t *testing.T) {
		e := event.UpdateEvent{
			ObjectOld: userSignupOld,
			ObjectNew: userSignupWithGenerationChanged,
		}
		require.True(t, pred.Update(e))
	})

	t.Run("test UserSignupChangedPredicate returns true when email-hash label changed", func(t *testing.T) {
		e := event.UpdateEvent{
			ObjectOld: userSignupOld,
			ObjectNew: userSignupWithEmailHashLabelChanged,
		}
		require.True(t, pred.Update(e))
	})

	t.Run("test UserSignupChangedPredicate returns true when email annotation changed", func(t *testing.T) {
		e := event.UpdateEvent{
			ObjectOld: userSignupOld,
			ObjectNew: userSignupWithEmailAnnotationChanged,
		}
		require.True(t, pred.Update(e))
	})

	t.Run("test UserSignupChangedPredicate returns true when migration-in-progress annotation removed", func(t *testing.T) {
		e := event.UpdateEvent{
			ObjectOld: userSignupOld,
			ObjectNew: userSignupWithMigrationInProgressAnnotationRemoved,
		}
		require.True(t, pred.Update(e))
	})
}

func TestAutomaticApprovalPredicateWhenApprovalIsEnabled(t *testing.T) {
	// given
	cl := test.NewFakeClient(t, commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true)))
	predicate := OnlyWhenAutomaticApprovalIsEnabled{
		client: cl,
	}
	toolchainStatus := NewToolchainStatus()

	t.Run("update", func(t *testing.T) {
		t.Run("when all fields are set", func(t *testing.T) {
			// given
			updateEvent := event.UpdateEvent{
				ObjectOld: toolchainStatus,
				ObjectNew: toolchainStatus,
			}

			// when
			shouldTriggerReconcile := predicate.Update(updateEvent)

			// then
			assert.True(t, shouldTriggerReconcile)
		})

		t.Run("when ObjectOld is missing", func(t *testing.T) {
			// given
			updateEvent := event.UpdateEvent{
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
				ObjectOld: toolchainStatus,
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
	cl := test.NewFakeClient(t, commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(false)))
	predicate := OnlyWhenAutomaticApprovalIsEnabled{
		client: cl,
	}
	toolchainStatus := NewToolchainStatus()

	t.Run("update", func(t *testing.T) {
		// given
		updateEvent := event.UpdateEvent{
			ObjectOld: toolchainStatus,
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
			Object: toolchainStatus,
		}

		// when
		shouldTriggerReconcile := predicate.Generic(genericEvent)

		// then
		assert.False(t, shouldTriggerReconcile)
	})
}
