package notification

import (
	"context"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Assertion struct {
	notification   *toolchainv1alpha1.Notification
	client         client.Client
	namespacedName types.NamespacedName
	t              test.T
}

func (a *Assertion) loadNotificationAssertion() error {
	notification := &toolchainv1alpha1.Notification{}
	err := a.client.Get(context.TODO(), a.namespacedName, notification)
	a.notification = notification
	return err
}

func AssertThatNotification(t test.T, name string, client client.Client) *Assertion {
	return &Assertion{
		client:         client,
		namespacedName: test.NamespacedName(test.HostOperatorNs, name),
		t:              t,
	}
}

func (a *Assertion) HasConditions(expected ...toolchainv1alpha1.Condition) *Assertion {
	err := a.loadNotificationAssertion()
	require.NoError(a.t, err)
	test.AssertConditionsMatch(a.t, a.notification.Status.Conditions, expected...)
	return a
}

func AssertNoNotificationsExist(t test.T, cl client.Client) {
	notifications := &toolchainv1alpha1.NotificationList{}
	err := cl.List(context.TODO(), notifications)
	require.NoError(t, err)
	require.Len(t, notifications.Items, 0)
}

type NotificationAssertion func(test.T, toolchainv1alpha1.Notification)

func OnlyOneNotificationExists(t test.T, cl client.Client, userName, notificationType string, assertions ...NotificationAssertion) {
	notifications := &toolchainv1alpha1.NotificationList{}
	labels := map[string]string{
		toolchainv1alpha1.NotificationUserNameLabelKey: userName,
		// NotificationTypeLabelKey is only used for easy lookup for debugging and e2e tests
		toolchainv1alpha1.NotificationTypeLabelKey: notificationType,
	}
	err := cl.List(context.TODO(), notifications, client.MatchingLabels(labels))
	require.NoError(t, err)
	require.Len(t, notifications.Items, 1)

	for _, a := range assertions {
		a(t, notifications.Items[0])
	}
}

func HasContext(key, expected string) NotificationAssertion {
	return func(t test.T, not toolchainv1alpha1.Notification) {
		assert.Equal(t, expected, not.Spec.Context[key])
	}
}
