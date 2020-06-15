package notification

import (
	"context"
	"errors"
	"github.com/codeready-toolchain/host-operator/pkg/configuration"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type NotificationDeliveryServiceConfig interface {
	GetNotificationDeliveryService() string
}

type NotificationDeliveryService interface {
	Deliver(ctx context.Context, notificationCtx NotificationContext, templateName string) error
}

type NotificationDeliveryServiceFactory struct {
	Client client.Client
	Config NotificationDeliveryServiceConfig
}

func NewNotificationDeliveryServiceFactory(client client.Client, config NotificationDeliveryServiceConfig) *NotificationDeliveryServiceFactory {
	return &NotificationDeliveryServiceFactory{Client: client, Config: config}
}

func (f *NotificationDeliveryServiceFactory) CreateNotificationDeliveryService() (NotificationDeliveryService, error) {
	switch f.Config.GetNotificationDeliveryService() {
	case configuration.NotificationDeliveryServiceMock:
		return NewMockNotificationDeliveryService(), nil
	case configuration.NotificationDeliveryServiceMailgun:
		return NewMailgunNotificationDeliveryService(), nil
	}
	return nil, errors.New("invalid notification delivery service configuration")
}
