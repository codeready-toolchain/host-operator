package notification

import (
	"context"
	"github.com/codeready-toolchain/host-operator/pkg/configuration"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/stretchr/testify/require"
	"testing"
)

type MockNotificationDeliveryServiceConfig struct {
	service string
}

func (c *MockNotificationDeliveryServiceConfig) GetNotificationDeliveryService() string {
	return c.service
}

func NewMockNotificationDeliveryServiceConfig(service string) NotificationDeliveryServiceConfig {
	return &MockNotificationDeliveryServiceConfig{service: service}
}

func TestNotificationDeliveryServiceFactory(t *testing.T) {
	// given
	client := test.NewFakeClient(t)

	t.Run("factory configured with mock delivery service", func(t *testing.T) {
		// when
		factory := NewNotificationDeliveryServiceFactory(client,
			NewMockNotificationDeliveryServiceConfig(configuration.NotificationDeliveryServiceMock))
		svc, err := factory.CreateNotificationDeliveryService()

		// then
		require.NoError(t, err)
		require.IsType(t, &MockNotificationDeliveryService{}, svc)
	})

	t.Run("factory configured with mailgun delivery service", func(t *testing.T) {
		// when
		factory := NewNotificationDeliveryServiceFactory(client,
			NewMockNotificationDeliveryServiceConfig(configuration.NotificationDeliveryServiceMailgun))
		svc, err := factory.CreateNotificationDeliveryService()

		// then
		require.NoError(t, err)
		require.IsType(t, &MailgunNotificationDeliveryService{}, svc)
	})

	t.Run("factory configured with invalid delivery service", func(t *testing.T) {
		// when
		factory := NewNotificationDeliveryServiceFactory(client,
			NewMockNotificationDeliveryServiceConfig("invalid"))
		_, err := factory.CreateNotificationDeliveryService()

		// then
		require.Error(t, err)
		require.Equal(t, "invalid notification delivery service configuration", err.Error())
	})
}

func TestBaseNotificationDeliveryServiceGenerateContent(t *testing.T) {
	// given
	baseService := &BaseNotificationDeliveryService{}

	nCtx := &NotificationContext{
		UserID:      "jsmith",
		FirstName:   "John",
		LastName:    "Smith",
		Email:       "jsmith@redhat.com",
		CompanyName: "Red Hat",
	}

	t.Run("test content generation with notification context for user id", func(t *testing.T) {
		// when
		def := "hello, {{.UserID}}"
		content, err := baseService.GenerateContent(context.Background(), nCtx, def)

		// then
		require.NoError(t, err)
		require.Equal(t, "hello, jsmith", content)
	})

	t.Run("test content generation with notification context for firstname lastname", func(t *testing.T) {
		// when
		def := "Dear {{.FirstName}} {{.LastName}},"
		content, err := baseService.GenerateContent(context.Background(), nCtx, def)

		// then
		require.NoError(t, err)
		require.Equal(t, "Dear John Smith,", content)
	})

	t.Run("test content generation with notification context for full email", func(t *testing.T) {
		// when
		def := "{{.FirstName}} {{.LastName}}<{{.Email}}>"
		content, err := baseService.GenerateContent(context.Background(), nCtx, def)

		// then
		require.NoError(t, err)
		require.Equal(t, "John Smith<jsmith@redhat.com>", content)
	})

	t.Run("test content generation with notification context for company name", func(t *testing.T) {
		// when
		def := "Increase developer productivity at {{.CompanyName}} today!"
		content, err := baseService.GenerateContent(context.Background(), nCtx, def)

		// then
		require.NoError(t, err)
		require.Equal(t, "Increase developer productivity at Red Hat today!", content)
	})
}
