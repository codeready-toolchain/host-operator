package notification

import (
	"bytes"
	"context"
	"errors"
	"github.com/codeready-toolchain/host-operator/pkg/templates/notificationtemplates"

	"github.com/codeready-toolchain/host-operator/pkg/configuration"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"text/template"
)

type NotificationDeliveryServiceConfig interface {
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
	Send(ctx context.Context, notificationCtx *NotificationContext, templateName string) error
}

type NotificationDeliveryServiceFactory struct {
	Client        client.Client
	Config        NotificationDeliveryServiceConfig
	MailgunConfig MailgunConfiguration
}

func NewNotificationDeliveryServiceFactory(client client.Client, config NotificationDeliveryServiceConfig,
	mailgunConfig MailgunConfiguration) *NotificationDeliveryServiceFactory {
	return &NotificationDeliveryServiceFactory{
		Client:        client,
		Config:        config,
		MailgunConfig: mailgunConfig,
	}
}

func (f *NotificationDeliveryServiceFactory) CreateNotificationDeliveryService() (NotificationDeliveryService, error) {
	switch f.Config.GetNotificationDeliveryService() {
	case configuration.NotificationDeliveryServiceMailgun:
		return NewMailgunNotificationDeliveryService(f.MailgunConfig, &DefaultTemplateLoader{}), nil
	}
	return nil, errors.New("invalid notification delivery service configuration")
}

type BaseNotificationDeliveryService struct {
	TemplateLoader TemplateLoader
}

func (s *BaseNotificationDeliveryService) GenerateContent(notificationCtx *NotificationContext,
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
