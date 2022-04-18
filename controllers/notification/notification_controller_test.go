package notification

import (
	"context"
	"crypto/md5" // nolint:gosec
	"encoding/hex"
	"errors"
	"fmt"
	"testing"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
	notify "github.com/codeready-toolchain/toolchain-common/pkg/notification"
	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	"github.com/codeready-toolchain/host-operator/pkg/templates/notificationtemplates"
	ntest "github.com/codeready-toolchain/host-operator/test/notification"
	commonconfig "github.com/codeready-toolchain/toolchain-common/pkg/configuration"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	testconfig "github.com/codeready-toolchain/toolchain-common/pkg/test/config"
	"github.com/gofrs/uuid"
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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type MockDeliveryService struct {
}

func (s *MockDeliveryService) Send(notification *toolchainv1alpha1.Notification) error {
	return errors.New("delivery error")
}

func TestNotificationSuccess(t *testing.T) {
	toolchainConfig := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.Notifications().DurationBeforeNotificationDeletion("10s"))

	// given
	t.Run("will not do anything and return requeue with shorter duration that 10s", func(t *testing.T) {
		// given
		ds, _ := mockDeliveryService(defaultTemplateLoader())
		controller, cl := newController(t, ds, toolchainConfig)

		notification, err := notify.NewNotificationBuilder(cl, test.HostOperatorNs).Create("jane@acme.com")
		require.NoError(t, err)
		notification.Status.Conditions = []toolchainv1alpha1.Condition{sentCond()}
		require.NoError(t, cl.Update(context.TODO(), notification))

		// when
		result, err := reconcileNotification(controller, notification)

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
		ds, _ := mockDeliveryService(defaultTemplateLoader())
		controller, cl := newController(t, ds, toolchainConfig)

		notification, err := notify.NewNotificationBuilder(cl, test.HostOperatorNs).Create("jane@acme.com")
		require.NoError(t, err)
		notification.Status.Conditions = []toolchainv1alpha1.Condition{sentCond()}
		notification.Status.Conditions[0].LastTransitionTime = v1.Time{Time: time.Now().Add(-cast.ToDuration("10s"))}
		require.NoError(t, cl.Update(context.TODO(), notification))

		// when
		result, err := reconcileNotification(controller, notification)

		// then
		require.NoError(t, err)
		assert.False(t, result.Requeue)
		AssertThatNotificationIsDeleted(t, cl, notification.Name)
	})
}

func TestNotificationSentFailure(t *testing.T) {
	toolchainConfig := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.Notifications().DurationBeforeNotificationDeletion("10s"))

	t.Run("will return an error since it cannot delete the Notification after successfully sending", func(t *testing.T) {
		// given
		ds, _ := mockDeliveryService(defaultTemplateLoader())
		controller, cl := newController(t, ds, toolchainConfig)
		cl.MockDelete = func(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
			return fmt.Errorf("error")
		}

		notification, err := notify.NewNotificationBuilder(cl, test.HostOperatorNs).
			WithSubjectAndContent("test", "test content").
			Create("abc123@acme.com")
		require.NoError(t, err)
		notification.Status.Conditions = []toolchainv1alpha1.Condition{sentCond()}
		notification.Status.Conditions[0].LastTransitionTime = v1.Time{Time: time.Now().Add(-cast.ToDuration("10s"))}
		require.NoError(t, cl.Update(context.TODO(), notification))

		// when
		result, err := reconcileNotification(controller, notification)

		// then
		require.Error(t, err)
		require.False(t, result.Requeue)
		assert.Equal(t, err.Error(), fmt.Sprintf("failed to delete notification: unable to delete Notification object '%s': error", notification.Name))
		ntest.AssertThatNotification(t, notification.Name, cl).
			HasConditions(sentCond(), deletionCond(fmt.Sprintf("unable to delete Notification object '%s': error", notification.Name)))
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
				Userid:     "foo",
				GivenName:  "Foo",
				FamilyName: "Bar",
				Company:    "Red Hat",
			},
		}
		controller, client := newController(t, ds, userSignup)

		notification, err := notify.NewNotificationBuilder(client, test.HostOperatorNs).
			WithUserContext(userSignup).
			WithSubjectAndContent("foo", "test content").
			Create("foo@redhat.com")
		require.NoError(t, err)

		// when
		result, err := reconcileNotification(controller, notification)

		// then
		require.NoError(t, err)
		require.True(t, result.Requeue)

		// Load the reconciled notification
		key := types.NamespacedName{
			Namespace: test.HostOperatorNs,
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
		require.Equal(t, "foo@redhat.com", accepted.Recipient)
		require.Equal(t, "redhat.com", accepted.RecipientDomain)
		require.Equal(t, "foo", accepted.Message.Headers.Subject)
		require.Equal(t, "noreply@foo.com", accepted.Message.Headers.From)
	})

	t.Run("test admin notification delivery ok", func(t *testing.T) {
		// given
		controller, client := newController(t, ds)

		notification, err := notify.NewNotificationBuilder(client, test.HostOperatorNs).
			WithSubjectAndContent("Alert", "Something bad happened").
			Create("sandbox-admin@developers.redhat.com")
		require.NoError(t, err)

		// when
		result, err := reconcileNotification(controller, notification)

		// then
		require.NoError(t, err)
		require.True(t, result.Requeue)

		// Load the reconciled notification
		key := types.NamespacedName{
			Namespace: test.HostOperatorNs,
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
		toolchainConfig := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.Environment(testconfig.E2E))
		userSignup := &toolchainv1alpha1.UserSignup{
			ObjectMeta: newObjectMeta("abc123", "jane@redhat.com"),
			Spec: toolchainv1alpha1.UserSignupSpec{
				Username:   "jane@redhat.com",
				GivenName:  "jane",
				FamilyName: "doe",
				Company:    "Red Hat",
			},
		}
		// pass in nil for deliveryService since send won't be used (sending skipped)
		controller, client := newController(t, nil, userSignup, toolchainConfig)

		notification, err := notify.NewNotificationBuilder(client, test.HostOperatorNs).
			Create("jane@redhat.com")
		require.NoError(t, err)

		// when
		result, err := reconcileNotification(controller, notification)

		// then
		require.NoError(t, err)
		require.True(t, result.Requeue)

		// Load the reconciled notification
		key := types.NamespacedName{
			Namespace: test.HostOperatorNs,
			Name:      notification.Name,
		}
		instance := &toolchainv1alpha1.Notification{}
		err = client.Get(context.TODO(), key, instance)
		require.NoError(t, err)

		ntest.AssertThatNotification(t, instance.Name, client).
			HasConditions(sentCond())
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
		controller, client := newController(t, mds, userSignup)

		notification, err := notify.NewNotificationBuilder(client, test.HostOperatorNs).
			Create("foo@redhat.com")
		require.NoError(t, err)

		// when
		result, err := reconcileNotification(controller, notification)

		// then
		require.Error(t, err)
		require.False(t, result.Requeue)
		require.Equal(t, "failed to send notification: delivery error", err.Error())

		// Load the reconciled notification
		key := types.NamespacedName{
			Namespace: test.HostOperatorNs,
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

func deletionCond(msg string) toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:               toolchainv1alpha1.NotificationDeletionError,
		Status:             apiv1.ConditionTrue,
		Reason:             toolchainv1alpha1.NotificationDeletionErrorReason,
		Message:            msg,
		LastTransitionTime: v1.Time{Time: time.Now()},
	}
}

func newController(t *testing.T, deliveryService DeliveryService,
	initObjs ...runtime.Object) (*Reconciler, *test.FakeClient) {
	restore := test.SetEnvVarAndRestore(t, commonconfig.WatchNamespaceEnvVar, test.HostOperatorNs)
	t.Cleanup(restore)

	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)
	cl := test.NewFakeClient(t, initObjs...)

	controller := &Reconciler{
		Client:          cl,
		Scheme:          s,
		deliveryService: deliveryService,
	}

	return controller, cl
}

func reconcileNotification(reconciler *Reconciler, notification *toolchainv1alpha1.Notification) (ctrl.Result, error) {
	return reconciler.Reconcile(context.TODO(), reconcile.Request{
		NamespacedName: test.NamespacedName(test.HostOperatorNs, notification.Name),
	})
}

func newObjectMeta(name, email string) v1.ObjectMeta {
	if name == "" {
		name = uuid.Must(uuid.NewV4()).String()
	}

	md5hash := md5.New() // nolint:gosec
	// Ignore the error, as this implementation cannot return one
	_, _ = md5hash.Write([]byte(email))
	emailHash := hex.EncodeToString(md5hash.Sum(nil))

	return v1.ObjectMeta{
		Name:      name,
		Namespace: test.HostOperatorNs,
		Annotations: map[string]string{
			toolchainv1alpha1.UserSignupUserEmailAnnotationKey: email,
		},
		Labels: map[string]string{
			toolchainv1alpha1.UserSignupUserEmailHashLabelKey: emailHash,
		},
	}
}
