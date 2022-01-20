package spacebindingcleanup

import (
	"testing"

	"github.com/codeready-toolchain/host-operator/test/space"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

func TestOnlyDeletionAndGenericPredicate(t *testing.T) {
	// given
	predicate := &OnlyDeleteAndGenericPredicate{}
	sp := space.NewSpace("space")

	t.Run("for create", func(t *testing.T) {
		for _, ev := range []event.CreateEvent{{}, {Object: sp}} {
			// when
			shouldReconcile := predicate.Create(ev)

			// then
			assert.False(t, shouldReconcile)
		}
	})

	t.Run("for update", func(t *testing.T) {
		for _, ev := range []event.UpdateEvent{{}, {ObjectOld: sp, ObjectNew: sp}} {
			// when
			shouldReconcile := predicate.Update(ev)

			// then
			assert.False(t, shouldReconcile)
		}
	})

	t.Run("for delete", func(t *testing.T) {
		for _, ev := range []event.DeleteEvent{{}, {Object: sp}} {
			// when
			shouldReconcile := predicate.Delete(ev)

			// then
			assert.True(t, shouldReconcile)
		}
	})

	t.Run("for generic", func(t *testing.T) {
		for _, ev := range []event.GenericEvent{{}, {Object: sp}} {
			// when
			shouldReconcile := predicate.Generic(ev)

			// then
			assert.True(t, shouldReconcile)
		}
	})
}
