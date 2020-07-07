package notification

import (
	"context"
	"github.com/codeready-toolchain/host-operator/pkg/templates/notificationtemplates"
	"github.com/mailgun/mailgun-go/v4"
	"github.com/stretchr/testify/require"
	"testing"
)

type MailgunAPIBaseOption struct {
	apiBase string
}

func (o *MailgunAPIBaseOption) ApplyToMailgun(mg mailgun.Mailgun) {
	mg.SetAPIBase(o.apiBase)
}

func NewMailgunAPIBaseOption(url string) MailgunOption {
	return &MailgunAPIBaseOption{apiBase: url}
}

func TestMailgunNotificationDeliveryService(t *testing.T) {
	// given
	mgs := mailgun.NewMockServer()
	mockServerOption := NewMailgunAPIBaseOption(mgs.URL())
	invalidServerOption := NewMailgunAPIBaseOption("https://127.0.0.1:60000/v3")

	mgConfig := NewMockMailgunConfiguration("mg.foo.com", "abcd12345", "noreply@foo.com")
	notCtx := &NotificationContext{
		UserID:      "jsmith123",
		FirstName:   "John",
		LastName:    "Smith",
		Email:       "jsmith@redhat.com",
		CompanyName: "Red Hat",
	}

	templateLoader := NewMockTemplateLoader(
		&notificationtemplates.NotificationTemplate{
			Subject: "foo",
			Content: "bar",
			Name:    "test",
		},
		&notificationtemplates.NotificationTemplate{
			Subject: "Hi there, {{invalid_expression}}",
			Content: "Content",
			Name:    "invalid_subject",
		},
		&notificationtemplates.NotificationTemplate{
			Subject: "Hi",
			Content: "{{invalid_expression}}",
			Name:    "invalid_content",
		})

	t.Run("test mailgun notification delivery service send", func(t *testing.T) {
		// when
		mgds := NewMailgunNotificationDeliveryService(mgConfig, templateLoader, mockServerOption)
		err := mgds.Send(context.Background(), notCtx, "test")

		// then
		require.NoError(t, err)
	})

	t.Run("test mailgun notification delivery service send fails", func(t *testing.T) {
		// when
		mgds := NewMailgunNotificationDeliveryService(mgConfig, templateLoader, invalidServerOption)
		err := mgds.Send(context.Background(), notCtx, "test")

		// then
		require.Error(t, err)
		require.IsType(t, MailgunDeliveryError{}, err)
		require.Equal(t, "error while delivering notification (ID: , Response: ) - while making http request: "+
			"Post https://127.0.0.1:60000/v3/mg.foo.com/messages: dial tcp 127.0.0.1:60000: connect: connection refused", err.Error())
	})

	t.Run("test mailgun notification delivery service invalid template", func(t *testing.T) {
		// when
		mgds := NewMailgunNotificationDeliveryService(mgConfig, templateLoader, mockServerOption)
		err := mgds.Send(context.Background(), notCtx, "bar")

		// then
		require.Error(t, err)
		require.Equal(t, "Template not found", err.Error())
	})

	t.Run("test mailgun notification delivery invalid subject template", func(t *testing.T) {
		// when
		mgds := NewMailgunNotificationDeliveryService(mgConfig, templateLoader, mockServerOption)
		err := mgds.Send(context.Background(), notCtx, "invalid_subject")

		// then
		require.Error(t, err)
		require.Equal(t, "template: template:1: function \"invalid_expression\" not defined", err.Error())
	})

	t.Run("test mailgun notification delivery invalid content template", func(t *testing.T) {
		// when
		mgds := NewMailgunNotificationDeliveryService(mgConfig, templateLoader, mockServerOption)
		err := mgds.Send(context.Background(), notCtx, "invalid_content")

		// then
		require.Error(t, err)
		require.Equal(t, "template: template:1: function \"invalid_expression\" not defined", err.Error())
	})
}
