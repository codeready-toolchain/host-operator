package notification

import (
	"errors"
	"github.com/codeready-toolchain/host-operator/pkg/configuration"
	"github.com/codeready-toolchain/host-operator/pkg/templates/notificationtemplates"
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

type MockMailgunConfiguration struct {
	Domain      string
	APIKey      string
	SenderEmail string
}

func (c *MockMailgunConfiguration) GetMailgunDomain() string {
	return c.Domain
}

func (c *MockMailgunConfiguration) GetMailgunAPIKey() string {
	return c.APIKey
}

func (c *MockMailgunConfiguration) GetMailgunSenderEmail() string {
	return c.SenderEmail
}

func NewMockMailgunConfiguration(domain, apikey, senderEmail string) MailgunConfiguration {
	return &MockMailgunConfiguration{
		Domain:      domain,
		APIKey:      apikey,
		SenderEmail: senderEmail,
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
		factory := NewNotificationDeliveryServiceFactory(client,
			NewMockNotificationDeliveryServiceConfig(configuration.NotificationDeliveryServiceMailgun),
			NewMockMailgunConfiguration("foo.com", "12345", "sender@foo.com"))
		svc, err := factory.CreateNotificationDeliveryService()

		// then
		require.NoError(t, err)
		require.IsType(t, &MailgunNotificationDeliveryService{}, svc)
	})

	t.Run("factory configured with invalid delivery service", func(t *testing.T) {
		// when
		factory := NewNotificationDeliveryServiceFactory(client,
			NewMockNotificationDeliveryServiceConfig("invalid"),
			nil)
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
		def := "{{.FirstName}} {{.LastName}}<{{.Email}}>"
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
