package notification

import (
	"errors"
	"testing"

	"github.com/codeready-toolchain/host-operator/pkg/templates/notificationtemplates"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/stretchr/testify/require"
)

type MockNotificationDeliveryServiceFactoryConfig struct {
	Mailgun MockMailgunConfiguration
	Service MockNotificationDeliveryServiceConfig
}

type MockNotificationDeliveryServiceConfig struct {
	service string
}

func (c *MockNotificationDeliveryServiceFactoryConfig) GetNotificationDeliveryService() string {
	return c.Service.service
}

type MockMailgunConfiguration struct {
	Domain       string
	APIKey       string
	SenderEmail  string
	ReplyToEmail string
}

func (c *MockNotificationDeliveryServiceFactoryConfig) GetMailgunDomain() string {
	return c.Mailgun.Domain
}

func (c *MockNotificationDeliveryServiceFactoryConfig) GetMailgunAPIKey() string {
	return c.Mailgun.APIKey
}

func (c *MockNotificationDeliveryServiceFactoryConfig) GetMailgunSenderEmail() string {
	return c.Mailgun.SenderEmail
}

func (c *MockNotificationDeliveryServiceFactoryConfig) GetMailgunReplyToEmail() string {
	return c.Mailgun.ReplyToEmail
}

func NewNotificationDeliveryServiceFactoryConfig(domain, apiKey, senderEmail, replyToEmail, service string) NotificationDeliveryServiceFactoryConfig {
	return &MockNotificationDeliveryServiceFactoryConfig{
		Mailgun: MockMailgunConfiguration{Domain: domain, APIKey: apiKey, SenderEmail: senderEmail},
		Service: MockNotificationDeliveryServiceConfig{service: service},
	}

}

type MockTemplateLoader struct {
	templates map[string]*notificationtemplates.NotificationTemplate
}

func (l *MockTemplateLoader) GetNotificationTemplate(name string) (*notificationtemplates.NotificationTemplate, bool, error) {
	template := l.templates[name]
	if template != nil {
		return template, true, nil
	} else {
		return nil, false, errors.New("Template not found")
	}
}

func NewMockTemplateLoader(templates ...*notificationtemplates.NotificationTemplate) TemplateLoader {
	tmpl := make(map[string]*notificationtemplates.NotificationTemplate)
	for _, template := range templates {
		tmpl[template.Name] = &notificationtemplates.NotificationTemplate{
			Subject: template.Subject,
			Content: template.Content,
			Name:    template.Name,
		}
	}
	return &MockTemplateLoader{tmpl}
}

func TestNotificationDeliveryServiceFactory(t *testing.T) {
	// given
	client := test.NewFakeClient(t)

	t.Run("factory configured with mailgun delivery service", func(t *testing.T) {
		// when
		factory := NewNotificationDeliveryServiceFactory(client, NewNotificationDeliveryServiceFactoryConfig(
			"mg.foo.com", "abcd12345", "noreply@foo.com", "", "mailgun"))
		svc, err := factory.CreateNotificationDeliveryService()

		// then
		require.NoError(t, err)
		require.IsType(t, &MailgunNotificationDeliveryService{}, svc)
	})

	t.Run("factory configured with invalid delivery service", func(t *testing.T) {

		// when
		factory := NewNotificationDeliveryServiceFactory(client,
			NewNotificationDeliveryServiceFactoryConfig("mg.foo.com", "abcd12345",
				"noreply@foo.com", "", ""))
		_, err := factory.CreateNotificationDeliveryService()

		// then
		require.Error(t, err)
		require.Equal(t, "invalid notification delivery service configuration", err.Error())
	})
}

func TestBaseNotificationDeliveryServiceGenerateContent(t *testing.T) {
	// given
	baseService := &BaseNotificationDeliveryService{}

	nCtx := &UserNotificationContext{
		UserID:      "jsmith",
		FirstName:   "John",
		LastName:    "Smith",
		UserEmail:   "jsmith@redhat.com",
		CompanyName: "Red Hat",
	}

	t.Run("test content generation with notification context for user id", func(t *testing.T) {
		// when
		def := "hello, {{.UserID}}"
		content, err := baseService.GenerateContent(nCtx, def)

		// then
		require.NoError(t, err)
		require.Equal(t, "hello, jsmith", content)
	})

	t.Run("test content generation with notification context for firstname lastname", func(t *testing.T) {
		// when
		def := "Dear {{.FirstName}} {{.LastName}},"
		content, err := baseService.GenerateContent(nCtx, def)

		// then
		require.NoError(t, err)
		require.Equal(t, "Dear John Smith,", content)
	})

	t.Run("test content generation with notification context for full email", func(t *testing.T) {
		// when
		def := "{{.FirstName}} {{.LastName}}<{{.UserEmail}}>"
		content, err := baseService.GenerateContent(nCtx, def)

		// then
		require.NoError(t, err)
		require.Equal(t, "John Smith<jsmith@redhat.com>", content)
	})

	t.Run("test content generation with notification context for company name", func(t *testing.T) {
		// when
		def := "Increase developer productivity at {{.CompanyName}} today!"
		content, err := baseService.GenerateContent(nCtx, def)

		// then
		require.NoError(t, err)
		require.Equal(t, "Increase developer productivity at Red Hat today!", content)
	})
}
