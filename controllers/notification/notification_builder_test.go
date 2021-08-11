package notification

import (
	"testing"

	"github.com/gofrs/uuid"

	"github.com/codeready-toolchain/api/api/v1alpha1"
	test2 "github.com/codeready-toolchain/host-operator/test"

	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/stretchr/testify/require"
)

func TestNotificationBuilder(t *testing.T) {
	// given
	client := test.NewFakeClient(t)

	t.Run("test create with no options", func(t *testing.T) {
		// when
		notification, err := NewNotificationBuilder(client, test.HostOperatorNs).Create("foo@acme.com")

		// then
		require.NoError(t, err)
		require.Equal(t, "foo@acme.com", notification.Spec.Recipient)
	})

	t.Run("test notification builder with user context", func(t *testing.T) {
		// when
		userSignup := test2.NewUserSignup()
		userSignup.Spec.GivenName = "John"
		userSignup.Spec.FamilyName = "Smith"
		userSignup.Spec.Company = "ACME Corp"
		userSignup.Status = v1alpha1.UserSignupStatus{
			CompliantUsername: "jsmith",
		}

		notification, err := NewNotificationBuilder(client, test.HostOperatorNs).
			WithUserContext(userSignup).
			Create(userSignup.Annotations[v1alpha1.UserSignupUserEmailAnnotationKey])

		// then
		require.NoError(t, err)
		require.Equal(t, userSignup.Annotations[v1alpha1.UserSignupUserEmailAnnotationKey], notification.Spec.Recipient)
		require.Equal(t, userSignup.Annotations[v1alpha1.UserSignupUserEmailAnnotationKey], notification.Spec.Context["UserEmail"])
		require.Equal(t, userSignup.Spec.GivenName, notification.Spec.Context["FirstName"])
		require.Equal(t, userSignup.Spec.FamilyName, notification.Spec.Context["LastName"])
		require.Equal(t, userSignup.Spec.Company, notification.Spec.Context["CompanyName"])
		require.Equal(t, userSignup.Spec.Userid, notification.Spec.Context["UserID"])
		require.Equal(t, userSignup.Status.CompliantUsername, notification.Spec.Context["UserName"])
	})

	t.Run("test notification builder with hard coded notification name", func(t *testing.T) {
		// when
		name := uuid.Must(uuid.NewV4()).String()
		notification, err := NewNotificationBuilder(client, test.HostOperatorNs).
			WithName(name).
			Create("foo@bar.com")

		// then
		require.NoError(t, err)
		require.Equal(t, name, notification.Name)
	})

	t.Run("test notification builder with template", func(t *testing.T) {
		// when
		notification, err := NewNotificationBuilder(client, test.HostOperatorNs).
			WithTemplate("default").
			Create("foo@bar.com")

		// then
		require.NoError(t, err)
		require.Equal(t, "default", notification.Spec.Template)
	})

	t.Run("test notification builder with subject and content", func(t *testing.T) {
		// when
		notification, err := NewNotificationBuilder(client, test.HostOperatorNs).
			WithSubjectAndContent("This is a test subject", "This is some test content").
			Create("foo@bar.com")

		// then
		require.NoError(t, err)
		require.Equal(t, "This is a test subject", notification.Spec.Subject)
		require.Equal(t, "This is some test content", notification.Spec.Content)
	})

	t.Run("test notification builder with keys and values", func(t *testing.T) {
		// when
		notification, err := NewNotificationBuilder(client, test.HostOperatorNs).
			WithKeysAndValues(map[string]string{"foo": "bar"}).
			Create("foo@bar.com")

		// then
		require.NoError(t, err)
		require.Equal(t, "bar", notification.Spec.Context["foo"])
	})

	t.Run("test notification builder with notification type", func(t *testing.T) {
		// when
		notification, err := NewNotificationBuilder(client, test.HostOperatorNs).
			WithNotificationType("TestNotificationType").
			Create("foo@bar.com")

		// then
		require.NoError(t, err)
		require.Equal(t, "TestNotificationType", notification.Labels[v1alpha1.NotificationTypeLabelKey])
	})
}
