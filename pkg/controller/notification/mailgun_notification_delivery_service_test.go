package notification

import (
	"context"
	"github.com/codeready-toolchain/host-operator/pkg/templates/notificationtemplates"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
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

func NewMailguAPIBaseOption(url string) MailgunOption {
	return &MailgunAPIBaseOption{apiBase: url}
}

func TestMailgunNotificationDeliveryService(t *testing.T) {
	// given
	mgs := mailgun.NewMockServer()
	opt := NewMailguAPIBaseOption(mgs.URL())
	client := test.NewFakeClient(t)
	mgConfig := NewMockMailgunConfiguration("mg.foo.com", "abcd12345", "noreply@foo.com")
	notCtx := &NotificationContext{
		UserID:      "jsmith123",
		FirstName:   "John",
		LastName:    "Smith",
		Email:       "jsmith@redhat.com",
		CompanyName: "Red Hat",
	}

	templateLoader := NewMockTemplateLoader(&notificationtemplates.NotificationTemplate{
		Subject: "foo",
		Content: "bar",
		Name:    "test",
	})

	t.Run("test mailgun notification delivery service send", func(t *testing.T) {
		// when
		mgds := NewMailgunNotificationDeliveryService(client, mgConfig, templateLoader, opt)
		err := mgds.Send(context.Background(), notCtx, "test")

		// then
		require.NoError(t, err)
	})
}
