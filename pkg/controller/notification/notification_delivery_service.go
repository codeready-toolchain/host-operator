package notification

import (
	"errors"
	"github.com/codeready-toolchain/host-operator/pkg/configuration"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type NotificationDeliveryService interface {
	Deliver() error
}

type NotificationDeliveryServiceFactory struct {
	Client client.Client
	Config *configuration.Config
}

func NewNotificationDeliveryServiceFactory(client client.Client, config *configuration.Config) *NotificationDeliveryServiceFactory {
	return &NotificationDeliveryServiceFactory{Client: client}
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
