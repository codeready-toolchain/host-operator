package notification

import (
	"net/http"
	"net/url"
	"testing"

	"gopkg.in/h2non/gock.v1"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"

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
	defer mgs.Stop()

	mockServerOption := NewMailgunAPIBaseOption(mgs.URL())
	invalidServerOption := NewMailgunAPIBaseOption("https://127.0.0.1:60000/v3")

	config := NewNotificationDeliveryServiceFactoryConfig("mg.foo.com", "abcd12345",
		"noreply@foo.com", "", "mailgun")
	notCtx := map[string]string{
		"UserID":      "jsmith123",
		"FirstName":   "John",
		"LastName":    "Smith",
		"UserEmail":   "jsmith@redhat.com",
		"CompanyName": "Red Hat",
	}

	templateLoader := NewMockTemplateLoader(
		&notificationtemplates.NotificationTemplate{
			Subject: "foo",
			Content: "a message sent to {{.ReplyTo}}",
			Name:    "replyto",
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
		err := mgds.Send(&toolchainv1alpha1.Notification{
			Spec: toolchainv1alpha1.NotificationSpec{
				Recipient: "foo@bar.com",
				Subject:   "test",
				Content:   "abc",
				Context:   notCtx,
			},
		})

		// then
		require.NoError(t, err)
	})

	t.Run("test mailgun notification delivery service send fails", func(t *testing.T) {
		// when
		mgds := NewMailgunNotificationDeliveryService(config, templateLoader, invalidServerOption)
		err := mgds.Send(&toolchainv1alpha1.Notification{
			Spec: toolchainv1alpha1.NotificationSpec{
				Subject: "test",
				Content: "abc",
				Context: notCtx,
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
		err := mgds.Send(&toolchainv1alpha1.Notification{
			Spec: toolchainv1alpha1.NotificationSpec{
				Template: "bar",
				Context:  notCtx,
			},
		})

		// then
		require.Error(t, err)
		require.Equal(t, "template not found", err.Error())
	})

	t.Run("test mailgun notification delivery invalid subject template", func(t *testing.T) {
		// when
		mgds := NewMailgunNotificationDeliveryService(config, templateLoader, mockServerOption)
		err := mgds.Send(&toolchainv1alpha1.Notification{
			Spec: toolchainv1alpha1.NotificationSpec{
				Template: "invalid_subject",
				Context:  notCtx,
			},
		})

		// then
		require.Error(t, err)
		require.Equal(t, "template: template:1: function \"invalid_expression\" not defined", err.Error())
	})

	t.Run("test mailgun notification delivery invalid content template", func(t *testing.T) {
		// when
		mgds := NewMailgunNotificationDeliveryService(config, templateLoader, mockServerOption)
		err := mgds.Send(&toolchainv1alpha1.Notification{
			Spec: toolchainv1alpha1.NotificationSpec{
				Template: "invalid_content",
				Context:  notCtx,
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
		err := mgds.Send(&toolchainv1alpha1.Notification{
			Spec: toolchainv1alpha1.NotificationSpec{
				Recipient: "foo@acme.com",
				Subject:   "test",
				Content:   "abc",
				Context:   notCtx,
			},
		})

		// then
		require.NoError(t, err)
	})

	t.Run("test mailgun notification delivery service with template containing reply-to address", func(t *testing.T) {
		// when
		config := NewNotificationDeliveryServiceFactoryConfig("mg.foo.com", "abcd12345",
			"noreply@foo.com", "info@foo.com", "mailgun")

		gock.Intercept()
		gock.New(mgs.URL()).Reply(http.StatusOK).AddHeader("Content-Type", "application/json").
			BodyString("{\"ID\" : \"<123>\", \"Message\" : \"Queued. Thank you.\"}")
		defer gock.Off()

		var formValues url.Values
		obs := func(request *http.Request, mock gock.Mock) {
			err := request.ParseMultipartForm(-1)
			require.NoError(t, err)
			formValues = request.Form
		}
		gock.Observe(obs)

		mgds := NewMailgunNotificationDeliveryService(config, templateLoader, mockServerOption)
		err := mgds.Send(&toolchainv1alpha1.Notification{
			Spec: toolchainv1alpha1.NotificationSpec{
				Recipient: "foo@acme.com",
				Template:  "replyto",
				Context:   notCtx,
			},
		})

		// then
		require.NoError(t, err)
		require.Equal(t, "a message sent to info@foo.com", formValues.Get("html"))

	})
}
