package changetierrequest

import (
	"testing"

	"github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/operator-framework/operator-sdk/pkg/predicate"
	"github.com/stretchr/testify/assert"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

var (
	predict = onlyGenerationChangedAndIsNotComplete{
		GenerationChangedPredicate: &predicate.GenerationChangedPredicate{},
	}

	readyCond = &v1alpha1.ChangeTierRequest{
		Status: v1alpha1.ChangeTierRequestStatus{
			Conditions: []v1alpha1.Condition{
				{
					Type:   v1alpha1.ConditionReady,
					Status: apiv1.ConditionTrue,
				},
			},
		},
	}
)

func TestPredicateUpdateShouldReturnFalseBecauseOfMissingData(t *testing.T) {
	// given
	updateEvents := []event.UpdateEvent{
		{},
		{MetaNew: &metav1.ObjectMeta{}, MetaOld: &metav1.ObjectMeta{},
			ObjectNew: &v1alpha1.ChangeTierRequest{}},
		{MetaNew: &metav1.ObjectMeta{}, MetaOld: &metav1.ObjectMeta{},
			ObjectOld: &v1alpha1.ChangeTierRequest{}},
		{MetaNew: &metav1.ObjectMeta{}, ObjectOld: &v1alpha1.ChangeTierRequest{},
			ObjectNew: &v1alpha1.ChangeTierRequest{}},
		{ObjectNew: &v1alpha1.ChangeTierRequest{}, MetaOld: &metav1.ObjectMeta{},
			ObjectOld: &v1alpha1.ChangeTierRequest{}}}

	for _, event := range updateEvents {
		// when
		ok := predict.Update(event)

		// then
		assert.False(t, ok)
	}
}

func TestPredicateUpdateWhenGenerationChanged(t *testing.T) {
	// given
	updateEvent := event.UpdateEvent{
		MetaNew:   &metav1.ObjectMeta{Generation: int64(123456789)},
		MetaOld:   &metav1.ObjectMeta{Generation: int64(987654321)},
		ObjectNew: &v1alpha1.ChangeTierRequest{},
		ObjectOld: &v1alpha1.ChangeTierRequest{}}

	t.Run("doesn't have status", func(t *testing.T) {
		// given
		updateEvent.ObjectNew = &v1alpha1.ChangeTierRequest{}

		// when
		ok := predict.Update(updateEvent)

		// then
		assert.True(t, ok)
	})

	t.Run("is not complete", func(t *testing.T) {
		// given
		updateEvent.ObjectNew = newChangeTierRequest("", "", "", notComplete)

		// when
		ok := predict.Update(updateEvent)

		// then
		assert.True(t, ok)
	})

	t.Run("has different condition type", func(t *testing.T) {
		// given
		updateEvent.ObjectNew = readyCond

		// when
		ok := predict.Update(updateEvent)

		// then
		assert.True(t, ok)
	})

	t.Run("is complete", func(t *testing.T) {
		// given
		updateEvent.ObjectNew = newChangeTierRequest("", "", "", complete)

		// when
		ok := predict.Update(updateEvent)

		// then
		assert.False(t, ok)
	})
}

func TestPredicateUpdateWhenGenerationNotChanged(t *testing.T) {
	// given
	updateEvent := event.UpdateEvent{
		MetaNew:   &metav1.ObjectMeta{Generation: int64(123456789)},
		MetaOld:   &metav1.ObjectMeta{Generation: int64(123456789)},
		ObjectNew: &v1alpha1.ChangeTierRequest{}, ObjectOld: &v1alpha1.ChangeTierRequest{}}

	t.Run("doesn't have status", func(t *testing.T) {
		// given
		updateEvent.ObjectNew = &v1alpha1.ChangeTierRequest{}

		// when
		ok := predict.Update(updateEvent)

		// then
		assert.False(t, ok)
	})

	t.Run("is not complete", func(t *testing.T) {
		// given
		updateEvent.ObjectNew = newChangeTierRequest("", "", "", notComplete)

		// when
		ok := predict.Update(updateEvent)

		// then
		assert.False(t, ok)
	})

	t.Run("has different condition type", func(t *testing.T) {
		// given
		updateEvent.ObjectNew = readyCond

		// when
		ok := predict.Update(updateEvent)

		// then
		assert.False(t, ok)
	})

	t.Run("is complete", func(t *testing.T) {
		// given
		updateEvent.ObjectNew = newChangeTierRequest("", "", "", complete)

		// when
		ok := predict.Update(updateEvent)

		// then
		assert.False(t, ok)
	})
}

func TestPredicateCreate(t *testing.T) {
	// given
	createEvent := event.CreateEvent{
		Meta:   &metav1.ObjectMeta{Generation: int64(123456789)},
		Object: &v1alpha1.ChangeTierRequest{}}

	t.Run("doesn't have status", func(t *testing.T) {
		// given
		createEvent.Object = &v1alpha1.ChangeTierRequest{}

		// when
		ok := predict.Create(createEvent)

		// then
		assert.True(t, ok)
	})

	t.Run("is not complete", func(t *testing.T) {
		// given
		createEvent.Object = newChangeTierRequest("", "", "", notComplete)

		// when
		ok := predict.Create(createEvent)

		// then
		assert.True(t, ok)
	})

	t.Run("has different condition type", func(t *testing.T) {
		// given
		createEvent.Object = readyCond

		// when
		ok := predict.Create(createEvent)

		// then
		assert.True(t, ok)
	})

	t.Run("is complete", func(t *testing.T) {
		// given
		createEvent.Object = newChangeTierRequest("", "", "", complete)

		// when
		ok := predict.Create(createEvent)

		// then
		assert.False(t, ok)
	})
}

func TestPredicateDeleteShouldReturnFalse(t *testing.T) {
	// given
	deleteEvent := event.DeleteEvent{
		Meta:   &metav1.ObjectMeta{Generation: int64(123456789)},
		Object: &v1alpha1.ChangeTierRequest{}}

	// when
	ok := predict.Delete(deleteEvent)

	// then
	assert.False(t, ok)
}

func TestPredicateGenericShouldReturnFalse(t *testing.T) {
	// given
	genericEvent := event.GenericEvent{
		Meta:   &metav1.ObjectMeta{Generation: int64(123456789)},
		Object: &v1alpha1.ChangeTierRequest{}}

	t.Run("doesn't have status", func(t *testing.T) {
		// given
		genericEvent.Object = &v1alpha1.ChangeTierRequest{}

		// when
		ok := predict.Generic(genericEvent)

		// then
		assert.True(t, ok)
	})

	t.Run("is not complete", func(t *testing.T) {
		// given
		genericEvent.Object = newChangeTierRequest("", "", "", notComplete)

		// when
		ok := predict.Generic(genericEvent)

		// then
		assert.True(t, ok)
	})

	t.Run("has different condition type", func(t *testing.T) {
		// given
		genericEvent.Object = readyCond

		// when
		ok := predict.Generic(genericEvent)

		// then
		assert.True(t, ok)
	})

	t.Run("is complete", func(t *testing.T) {
		// given
		genericEvent.Object = newChangeTierRequest("", "", "", complete)

		// when
		ok := predict.Generic(genericEvent)

		// then
		assert.False(t, ok)
	})
}
