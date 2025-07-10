package nstemplatetier

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

func TestIsBeingDeletedPredicate(t *testing.T) {
	pred := IsBeingDeletedPredicate{}
	t.Run("doesn't react on create", func(t *testing.T) {
		assert.False(t, pred.Create(event.CreateEvent{}))
	})
	t.Run("doesn't react on delete", func(t *testing.T) {
		assert.False(t, pred.Delete(event.DeleteEvent{}))
	})
	t.Run("doesn't react on unrelated update", func(t *testing.T) {
		assert.False(t, pred.Update(event.UpdateEvent{}))
	})
	t.Run("doesn't react on unrelated generic event", func(t *testing.T) {
		assert.False(t, pred.Generic(event.GenericEvent{}))
	})
	t.Run("reacts on update of deletion timestamp", func(t *testing.T) {
		assert.True(t, pred.Update(event.UpdateEvent{
			ObjectNew: &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &metav1.Time{Time: time.Now()}}},
		}))
	})
	t.Run("reacts on update of deletion timestamp through generic event", func(t *testing.T) {
		assert.True(t, pred.Generic(event.GenericEvent{
			Object: &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &metav1.Time{Time: time.Now()}}},
		}))
	})
}
