package notification

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	"github.com/codeready-toolchain/host-operator/pkg/configuration"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"

	"github.com/spf13/cast"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apiv1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestNotificationSuccess(t *testing.T) {
	// given
	restore := test.SetEnvVarAndRestore(t, "HOST_OPERATOR_DURATION_BEFORE_NOTIFICATION_DELETION", "10s")
	defer restore()

	t.Run("will not do anything and return requeue with shorter duration that 10s", func(t *testing.T) {
		// given
		notification := newNotification("jane", "")
		notification.Status.Conditions = []v1alpha1.Condition{toBeSent()}
		controller, request, cl := newController(t, notification)

		// when
		result, err := controller.Reconcile(request)

		// then
		require.NoError(t, err)
		assert.True(t, result.Requeue)
		assert.True(t, result.RequeueAfter < cast.ToDuration("10s"))
		assert.True(t, result.RequeueAfter > cast.ToDuration("1s"))
		AssertThatNotificationHasCondition(t, cl, notification.Name, toBeSent())
	})

	t.Run("sent notification deleted when deletion timeout passed", func(t *testing.T) {
		// given
		notification := newNotification("jane", "")
		notification.Status.Conditions = []v1alpha1.Condition{toBeSent()}
		notification.Status.Conditions[0].LastTransitionTime = v1.Time{Time: time.Now().Add(-cast.ToDuration("10s"))}
		controller, request, cl := newController(t, notification)

		// when
		result, err := controller.Reconcile(request)

		// then
		require.NoError(t, err)
		assert.False(t, result.Requeue)
		AssertThatNotificationIsDeleted(t, cl, notification.Name)
	})
}

func TestNotificationSentFailure(t *testing.T) {
	restore := test.SetEnvVarAndRestore(t, "HOST_OPERATOR_DURATION_BEFORE_NOTIFICATION_DELETION", "10s")
	defer restore()

	t.Run("will return an error since it cannot delete the Notification after successfully sending", func(t *testing.T) {
		// given
		notification := newNotification("abc123", "")
		notification.Status.Conditions = []v1alpha1.Condition{toBeSent()}
		notification.Status.Conditions[0].LastTransitionTime = v1.Time{Time: time.Now().Add(-cast.ToDuration("10s"))}

		controller, request, cl := newController(t, notification)
		cl.MockDelete = func(ctx context.Context, obj runtime.Object, opts ...client.DeleteOption) error {
			return fmt.Errorf("error")
		}

		// when
		_, err := controller.Reconcile(request)

		// then
		require.Error(t, err)
		assert.Equal(t, err.Error(), "failed to delete notification: unable to delete Notification object 'notification-name': error")

		AssertThatNotificationHasCondition(t, cl, notification.Name, toBeSent(), toBeDeletionError("unable to delete Notification object 'notification-name': error"))
	})
}

func AssertThatNotificationIsDeleted(t *testing.T, cl client.Client, name string) {
	notification := &v1alpha1.Notification{}
	err := cl.Get(context.TODO(), test.NamespacedName(test.HostOperatorNs, name), notification)
	require.Error(t, err)
	assert.IsType(t, v1.StatusReasonNotFound, apierrors.ReasonForError(err))
}

func AssertThatNotificationHasCondition(t *testing.T, cl client.Client, name string, condition ...v1alpha1.Condition) {
	notification := &v1alpha1.Notification{}
	err := cl.Get(context.TODO(), test.NamespacedName(test.HostOperatorNs, name), notification)
	require.NoError(t, err)
	test.AssertConditionsMatch(t, notification.Status.Conditions, condition...)
}

func toBeSent() v1alpha1.Condition {
	return v1alpha1.Condition{
		Type:               v1alpha1.NotificationSent,
		Status:             apiv1.ConditionTrue,
		Reason:             "Sent",
		LastTransitionTime: v1.Time{Time: time.Now()},
	}
}

func toBeDeletionError(msg string) v1alpha1.Condition {
	return v1alpha1.Condition{
		Type:               v1alpha1.NotificationDeletionError,
		Status:             apiv1.ConditionTrue,
		Reason:             v1alpha1.NotificationDeletionErrorReason,
		Message:            msg,
		LastTransitionTime: v1.Time{Time: time.Now()},
	}
}

type notificationOption func(*v1alpha1.Notification)

func newNotification(userID, template string, options ...notificationOption) *v1alpha1.Notification {
	notification := &v1alpha1.Notification{
		ObjectMeta: v1.ObjectMeta{
			Namespace: test.HostOperatorNs,
			Name:      "notification-name",
		},
		Spec: v1alpha1.NotificationSpec{
			UserID:   userID,
			Template: template,
		},
	}
	for _, set := range options {
		set(notification)
	}
	return notification
}

func newController(t *testing.T, notification *v1alpha1.Notification, initObjs ...runtime.Object) (*ReconcileNotification, reconcile.Request, *test.FakeClient) {
	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)
	cl := test.NewFakeClient(t, append(initObjs, notification)...)

	config := configuration.LoadConfig(cl)
	controller := &ReconcileNotification{
		client: cl,
		scheme: s,
		config: config,
	}
	request := reconcile.Request{
		NamespacedName: test.NamespacedName(test.HostOperatorNs, notification.Name),
	}

	return controller, request, cl
}
