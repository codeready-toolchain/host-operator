package notification

import (
	"bytes"
	"errors"
	"github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"text/template"

	"github.com/codeready-toolchain/host-operator/pkg/configuration"
	"github.com/codeready-toolchain/host-operator/pkg/templates/notificationtemplates"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

type notificationDeliveryServiceConfig interface {
	GetNotificationDeliveryService() string
}

type TemplateLoader interface {
	GetNotificationTemplate(name string) (*notificationtemplates.NotificationTemplate, bool, error)
}

type DefaultTemplateLoader struct{}

func (l *DefaultTemplateLoader) GetNotificationTemplate(name string) (*notificationtemplates.NotificationTemplate, bool, error) {
	return notificationtemplates.GetNotificationTemplate(name)
}

type NotificationDeliveryService interface {
	Send(notificationCtx NotificationContext, notification *v1alpha1.Notification) error
}

type NotificationDeliveryServiceFactory struct {
	Client client.Client
	Config NotificationDeliveryServiceFactoryConfig
}

type NotificationDeliveryServiceFactoryConfig interface {
	notificationDeliveryServiceConfig
	MailgunConfig
}

func NewNotificationDeliveryServiceFactory(client client.Client, config NotificationDeliveryServiceFactoryConfig) *NotificationDeliveryServiceFactory {
	return &NotificationDeliveryServiceFactory{
		Client: client,
		Config: config,
	}
}

func (f *NotificationDeliveryServiceFactory) CreateNotificationDeliveryService() (NotificationDeliveryService, error) {
	switch f.Config.GetNotificationDeliveryService() {
	case configuration.NotificationDeliveryServiceMailgun:
		return NewMailgunNotificationDeliveryService(f.Config, &DefaultTemplateLoader{}), nil
	}
	return nil, errors.New("invalid notification delivery service configuration")
}

type BaseNotificationDeliveryService struct {
	TemplateLoader TemplateLoader
}

func (s *BaseNotificationDeliveryService) GenerateContent(notificationCtx interface{},
	templateDefinition string) (string, error) {

	tmpl, err := template.New("template").Parse(templateDefinition)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer

	err = tmpl.Execute(&buf, notificationCtx)
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}
