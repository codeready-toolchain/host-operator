package notification

import (
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
