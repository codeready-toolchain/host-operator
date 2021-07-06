package notification

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	"github.com/codeready-toolchain/host-operator/pkg/templates/notificationtemplates"
	ntest "github.com/codeready-toolchain/host-operator/test/notification"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	testconfig "github.com/codeready-toolchain/toolchain-common/pkg/test/config"
	"github.com/mailgun/mailgun-go/v4"
	events2 "github.com/mailgun/mailgun-go/v4/events"
	"k8s.io/apimachinery/pkg/types"

	"github.com/spf13/cast"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apiv1 "k8s.io/api/core/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type MockDeliveryService struct {
}

func (s *MockDeliveryService) Send(notificationCtx Context, notification *toolchainv1alpha1.Notification) error {
	return errors.New("delivery error")
}

func TestNotificationSuccess(t *testing.T) {
	toolchainConfig := testconfig.NewToolchainConfig(testconfig.Notifications().DurationBeforeNotificationDeletion("10s"))

	// given
	t.Run("will not do anything and return requeue with shorter duration that 10s", func(t *testing.T) {
		// given
		notification := newNotification("jane", "")
		notification.Status.Conditions = []toolchainv1alpha1.Condition{sentCond()}
		ds, _ := mockDeliveryService(defaultTemplateLoader())
		controller, request, cl := newController(t, notification, ds, toolchainConfig)

		// when
		result, err := controller.Reconcile(request)

		// then
		require.NoError(t, err)
		assert.True(t, result.Requeue)
		assert.True(t, result.RequeueAfter < cast.ToDuration("10s"))
		assert.True(t, result.RequeueAfter > cast.ToDuration("1s"))
		ntest.AssertThatNotification(t, notification.Name, cl).
			HasConditions(sentCond())
	})

	t.Run("sent notification deleted when deletion timeout passed", func(t *testing.T) {
		// given
		notification := newNotification("jane", "")
		notification.Status.Conditions = []toolchainv1alpha1.Condition{sentCond()}
		notification.Status.Conditions[0].LastTransitionTime = v1.Time{Time: time.Now().Add(-cast.ToDuration("10s"))}
		ds, _ := mockDeliveryService(defaultTemplateLoader())
		controller, request, cl := newController(t, notification, ds, toolchainConfig)

		// when
		result, err := controller.Reconcile(request)

		// then
		require.NoError(t, err)
		assert.False(t, result.Requeue)
		AssertThatNotificationIsDeleted(t, cl, notification.Name)
	})
}

func TestNotificationSentFailure(t *testing.T) {
	toolchainConfig := testconfig.NewToolchainConfig(testconfig.Notifications().DurationBeforeNotificationDeletion("10s"))

	t.Run("will return an error since it cannot delete the Notification after successfully sending", func(t *testing.T) {
		// given
		notification := newNotification("abc123", "")
		notification.Status.Conditions = []toolchainv1alpha1.Condition{sentCond()}
		notification.Status.Conditions[0].LastTransitionTime = v1.Time{Time: time.Now().Add(-cast.ToDuration("10s"))}

		ds, _ := mockDeliveryService(defaultTemplateLoader())
		controller, request, cl := newController(t, notification, ds, toolchainConfig)
		cl.MockDelete = func(ctx context.Context, obj runtime.Object, opts ...client.DeleteOption) error {
			return fmt.Errorf("error")
		}

		// when
		result, err := controller.Reconcile(request)

		// then
		require.Error(t, err)
		require.False(t, result.Requeue)
		assert.Equal(t, err.Error(), "failed to delete notification: unable to delete Notification object 'notification-name': error")
		ntest.AssertThatNotification(t, notification.Name, cl).
			HasConditions(sentCond(), deletionCond("unable to delete Notification object 'notification-name': error"))
	})
}

func TestNotificationDelivery(t *testing.T) {
	// given
	ds, mockServer := mockDeliveryService(defaultTemplateLoader())

	mg := mailgun.NewMailgun("crt-test.com", "123")
	mg.SetAPIBase(mockServer.URL())

	t.Run("test notification delivery ok", func(t *testing.T) {
		// given
		userSignup := &toolchainv1alpha1.UserSignup{
			ObjectMeta: newObjectMeta("abc123", "foo@redhat.com"),
			Spec: toolchainv1alpha1.UserSignupSpec{
				Username:   "foo@redhat.com",
				GivenName:  "Foo",
				FamilyName: "Bar",
				Company:    "Red Hat",
			},
		}
		notification := newNotification("abc123", "test")
		controller, request, client := newController(t, notification, ds, userSignup)

		// when
		result, err := controller.Reconcile(request)

		// then
		require.NoError(t, err)
		require.True(t, result.Requeue)

		// Load the reconciled notification
		key := types.NamespacedName{
			Namespace: operatorNamespace,
			Name:      notification.Name,
		}
		instance := &toolchainv1alpha1.Notification{}
		err = client.Get(context.TODO(), key, instance)
		require.NoError(t, err)

		test.AssertConditionsMatch(t, instance.Status.Conditions,
			toolchainv1alpha1.Condition{
				Type:   toolchainv1alpha1.NotificationSent,
				Status: corev1.ConditionTrue,
				Reason: toolchainv1alpha1.NotificationSentReason,
			},
		)

		iter := mg.ListEvents(&mailgun.ListEventOptions{Limit: 1})
		var events []mailgun.Event
		require.True(t, iter.First(context.Background(), &events))
		require.True(t, iter.Last(context.Background(), &events))
		require.Len(t, events, 1)
		e := events[0]
		require.IsType(t, &events2.Accepted{}, e)
		accepted := e.(*events2.Accepted)
		require.Equal(t, "Foo Bar<foo@redhat.com>", accepted.Recipient)
		require.Equal(t, "redhat.com", accepted.RecipientDomain)
		require.Equal(t, "foo", accepted.Message.Headers.Subject)
		require.Equal(t, "noreply@foo.com", accepted.Message.Headers.From)
	})

	t.Run("test admin notification delivery ok", func(t *testing.T) {
		// given
		notification := newAdminNotification("sandbox-admin@developers.redhat.com", "Alert",
			"Something bad happened")
		controller, request, client := newController(t, notification, ds)

		// when
		result, err := controller.Reconcile(request)

		// then
		require.NoError(t, err)
		require.True(t, result.Requeue)

		// Load the reconciled notification
		key := types.NamespacedName{
			Namespace: operatorNamespace,
			Name:      notification.Name,
		}
		instance := &toolchainv1alpha1.Notification{}
		err = client.Get(context.TODO(), key, instance)
		require.NoError(t, err)

		test.AssertConditionsMatch(t, instance.Status.Conditions,
			toolchainv1alpha1.Condition{
				Type:   toolchainv1alpha1.NotificationSent,
				Status: corev1.ConditionTrue,
				Reason: toolchainv1alpha1.NotificationSentReason,
			},
		)

		iter := mg.ListEvents(&mailgun.ListEventOptions{Limit: 1})
		var events []mailgun.Event
		require.True(t, iter.First(context.Background(), &events))
		require.True(t, iter.Last(context.Background(), &events))
		require.Len(t, events, 1)
		e := events[0]
		require.IsType(t, &events2.Accepted{}, e)
		accepted := e.(*events2.Accepted)
		require.Equal(t, "sandbox-admin@developers.redhat.com", accepted.Recipient)
		require.Equal(t, "developers.redhat.com", accepted.RecipientDomain)
		require.Equal(t, "Alert", accepted.Message.Headers.Subject)
		require.Equal(t, "noreply@foo.com", accepted.Message.Headers.From)
	})

	t.Run("test notification with environment e2e", func(t *testing.T) {

		// given
		toolchainConfig := testconfig.NewToolchainConfig(testconfig.Environment(testconfig.E2E))
		userSignup := &toolchainv1alpha1.UserSignup{
			ObjectMeta: newObjectMeta("abc123", "jane@redhat.com"),
			Spec: toolchainv1alpha1.UserSignupSpec{
				Username:   "jane@redhat.com",
				GivenName:  "jane",
				FamilyName: "doe",
				Company:    "Red Hat",
			},
		}
		notification := newNotification("abc123", "test")
		// pass in nil for deliveryService since send won't be used (sending skipped)
		controller, request, client := newController(t, notification, nil, userSignup, toolchainConfig)

		// when
		result, err := controller.Reconcile(request)

		// then
		require.NoError(t, err)
		require.True(t, result.Requeue)

		// Load the reconciled notification
		key := types.NamespacedName{
			Namespace: operatorNamespace,
			Name:      notification.Name,
		}
		instance := &toolchainv1alpha1.Notification{}
		err = client.Get(context.TODO(), key, instance)
		require.NoError(t, err)

		ntest.AssertThatNotification(t, instance.Name, client).
			HasConditions(sentCond())
	})

	t.Run("test notification delivery fails for invalid user ID", func(t *testing.T) {
		// given
		notification := newNotification("abc123", "test")
		toolchainConfig := testconfig.NewToolchainConfig(testconfig.Environment(testconfig.E2E))
		controller, request, client := newController(t, notification, ds, toolchainConfig)

		// when
		result, err := controller.Reconcile(request)

		// then
		require.Error(t, err)
		require.False(t, result.Requeue)
		require.Equal(t, "failed to create notification context: usersignups.toolchain.dev.openshift.com \"abc123\" not found", err.Error())

		// Load the reconciled notification
		key := types.NamespacedName{
			Namespace: operatorNamespace,
			Name:      notification.Name,
		}
		instance := &toolchainv1alpha1.Notification{}
		err = client.Get(context.TODO(), key, instance)
		require.NoError(t, err)

		ntest.AssertThatNotification(t, instance.Name, client).
			HasConditions(contextErrorCond("usersignups.toolchain.dev.openshift.com \"abc123\" not found"))

	})

	t.Run("test notification delivery fails for delivery service failure", func(t *testing.T) {
		// given
		userSignup := &toolchainv1alpha1.UserSignup{
			ObjectMeta: newObjectMeta("abc123", "foo@redhat.com"),
			Spec: toolchainv1alpha1.UserSignupSpec{
				Username:   "foo@redhat.com",
				GivenName:  "Foo",
				FamilyName: "Bar",
				Company:    "Red Hat",
			},
		}
		mds := &MockDeliveryService{}
		notification := newNotification("abc123", "test")
		controller, request, client := newController(t, notification, mds, userSignup)

		// when
		result, err := controller.Reconcile(request)

		// then
		require.Error(t, err)
		require.False(t, result.Requeue)
		require.Equal(t, "failed to send notification: delivery error", err.Error())

		// Load the reconciled notification
		key := types.NamespacedName{
			Namespace: operatorNamespace,
			Name:      notification.Name,
		}
		instance := &toolchainv1alpha1.Notification{}
		err = client.Get(context.TODO(), key, instance)
		require.NoError(t, err)

		ntest.AssertThatNotification(t, instance.Name, client).
			HasConditions(deliveryErrorCond("delivery error"))
	})
}

func defaultTemplateLoader() TemplateLoader {
	templateLoader := NewMockTemplateLoader(
		&notificationtemplates.NotificationTemplate{
			Subject: "foo",
			Content: "bar",
			Name:    "test",
		})

	return templateLoader
}

func mockDeliveryService(templateLoader TemplateLoader) (DeliveryService, mailgun.MockServer) {
	mgs := mailgun.NewMockServer()
	mockServerOption := NewMailgunAPIBaseOption(mgs.URL())

	config := NewNotificationDeliveryServiceFactoryConfig("mg.foo.com", "abcd12345", "noreply@foo.com", "", "mailgun")

	mgds := NewMailgunNotificationDeliveryService(config, templateLoader, mockServerOption)
	return mgds, mgs
}

func AssertThatNotificationIsDeleted(t *testing.T, cl client.Client, name string) {
	notification := &toolchainv1alpha1.Notification{}
	err := cl.Get(context.TODO(), test.NamespacedName(test.HostOperatorNs, name), notification)
	require.Error(t, err)
	assert.IsType(t, v1.StatusReasonNotFound, apierrors.ReasonForError(err))
}

func sentCond() toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:               toolchainv1alpha1.NotificationSent,
		Status:             apiv1.ConditionTrue,
		Reason:             "Sent",
		LastTransitionTime: v1.Time{Time: time.Now()},
	}
}

func deliveryErrorCond(msg string) toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:    toolchainv1alpha1.NotificationSent,
		Status:  corev1.ConditionFalse,
		Reason:  toolchainv1alpha1.NotificationDeliveryErrorReason,
		Message: msg,
	}
}

func contextErrorCond(msg string) toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:    toolchainv1alpha1.NotificationSent,
		Status:  corev1.ConditionFalse,
		Reason:  toolchainv1alpha1.NotificationContextErrorReason,
		Message: msg,
	}
}

func deletionCond(msg string) toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:               toolchainv1alpha1.NotificationDeletionError,
		Status:             apiv1.ConditionTrue,
		Reason:             toolchainv1alpha1.NotificationDeletionErrorReason,
		Message:            msg,
		LastTransitionTime: v1.Time{Time: time.Now()},
	}
}

func newNotification(userID, template string) *toolchainv1alpha1.Notification {
	notification := &toolchainv1alpha1.Notification{
		ObjectMeta: v1.ObjectMeta{
			Namespace: test.HostOperatorNs,
			Name:      "notification-name",
		},
		Spec: toolchainv1alpha1.NotificationSpec{
			UserID:   userID,
			Template: template,
		},
	}
	return notification
}

func newAdminNotification(recipient, subject, content string) *toolchainv1alpha1.Notification {
	return &toolchainv1alpha1.Notification{
		ObjectMeta: v1.ObjectMeta{
			Namespace: test.HostOperatorNs,
			Name:      "notification-name",
		},
		Spec: toolchainv1alpha1.NotificationSpec{
			Recipient: recipient,
			Subject:   subject,
			Content:   content,
		},
	}
}

func newController(t *testing.T, notification *toolchainv1alpha1.Notification, deliveryService DeliveryService,
	initObjs ...runtime.Object) (*Reconciler, reconcile.Request, *test.FakeClient) {

	os.Setenv("WATCH_NAMESPACE", test.HostOperatorNs)
	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)
	cl := test.NewFakeClient(t, append(initObjs, notification)...)

	controller := &Reconciler{
		Client:          cl,
		Scheme:          s,
		deliveryService: deliveryService,
		Log:             ctrl.Log.WithName("controllers").WithName("Notification"),
	}
	request := reconcile.Request{
		NamespacedName: test.NamespacedName(test.HostOperatorNs, notification.Name),
	}

	return controller, request, cl
}
