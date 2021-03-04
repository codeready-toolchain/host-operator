package notification

import (
	"github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"testing"

	"github.com/codeready-toolchain/host-operator/pkg/templates/notificationtemplates"

	"github.com/mailgun/mailgun-go/v4"
	"github.com/stretchr/testify/require"
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

	config := NewNotificationDeliveryServiceFactoryConfig("mg.foo.com", "abcd12345",
		"noreply@foo.com", "", "mailgun")
	notCtx := &UserNotificationContext{
		UserID:      "jsmith123",
		FirstName:   "John",
		LastName:    "Smith",
		UserEmail:   "jsmith@redhat.com",
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
		mgds := NewMailgunNotificationDeliveryService(config, templateLoader, mockServerOption)
		err := mgds.Send(notCtx, &v1alpha1.Notification{
			Spec: v1alpha1.NotificationSpec{
				Subject: "test",
				Content: "abc",
			},
		})

		// then
		require.NoError(t, err)
	})

	t.Run("test mailgun notification delivery service send fails", func(t *testing.T) {
		// when
		mgds := NewMailgunNotificationDeliveryService(config, templateLoader, invalidServerOption)
		err := mgds.Send(notCtx, &v1alpha1.Notification{
			Spec: v1alpha1.NotificationSpec{
				Subject: "test",
				Content: "abc",
			},
		})

		// then
		require.Error(t, err)
		require.IsType(t, MailgunDeliveryError{}, err)
		require.Contains(t, err.Error(), "error while delivering notification (ID: , Response: ) - while making http request: Post ")
		require.Contains(t, err.Error(), "https://127.0.0.1:60000/v3/mg.foo.com/messages")
		require.Contains(t, err.Error(), ": dial tcp 127.0.0.1:60000: connect: connection refused")
	})

	t.Run("test mailgun notification delivery service invalid template", func(t *testing.T) {
		// when
		mgds := NewMailgunNotificationDeliveryService(config, templateLoader, mockServerOption)
		err := mgds.Send(notCtx, &v1alpha1.Notification{
			Spec: v1alpha1.NotificationSpec{
				Template: "bar",
			},
		})

		// then
		require.Error(t, err)
		require.Equal(t, "Template not found", err.Error())
	})

	t.Run("test mailgun notification delivery invalid subject template", func(t *testing.T) {
		// when
		mgds := NewMailgunNotificationDeliveryService(config, templateLoader, mockServerOption)
		err := mgds.Send(notCtx, &v1alpha1.Notification{
			Spec: v1alpha1.NotificationSpec{
				Template: "invalid_subject",
			},
		})

		// then
		require.Error(t, err)
		require.Equal(t, "template: template:1: function \"invalid_expression\" not defined", err.Error())
	})

	t.Run("test mailgun notification delivery invalid content template", func(t *testing.T) {
		// when
		mgds := NewMailgunNotificationDeliveryService(config, templateLoader, mockServerOption)
		err := mgds.Send(notCtx, &v1alpha1.Notification{
			Spec: v1alpha1.NotificationSpec{
				Template: "invalid_content",
			},
		})

		// then
		require.Error(t, err)
		require.Equal(t, "template: template:1: function \"invalid_expression\" not defined", err.Error())
	})

	t.Run("test mailgun notification delivery service send with reply-to address", func(t *testing.T) {
		// when
		config := NewNotificationDeliveryServiceFactoryConfig("mg.foo.com", "abcd12345",
			"noreply@foo.com", "info@foo.com", "mailgun")

		mgds := NewMailgunNotificationDeliveryService(config, templateLoader, mockServerOption)
		err := mgds.Send(notCtx, &v1alpha1.Notification{
			Spec: v1alpha1.NotificationSpec{
				Subject: "test",
				Content: "abc",
			},
		})

		// then
		require.NoError(t, err)

	})
}
