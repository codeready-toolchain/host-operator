package notification

import (
	"bytes"
	"errors"
	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
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

type DeliveryService interface {
	Send(notificationCtx Context, notification *toolchainv1alpha1.Notification) error
}

type DeliveryServiceFactory struct {
	Client client.Client
	Config DeliveryServiceFactoryConfig
}

type DeliveryServiceFactoryConfig interface {
	notificationDeliveryServiceConfig
	MailgunConfig
}

func NewNotificationDeliveryServiceFactory(client client.Client, config DeliveryServiceFactoryConfig) *DeliveryServiceFactory {
	return &DeliveryServiceFactory{
		Client: client,
		Config: config,
	}
}

func (f *DeliveryServiceFactory) CreateNotificationDeliveryService() (DeliveryService, error) {
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
