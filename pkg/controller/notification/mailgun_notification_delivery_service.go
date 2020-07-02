package notification

import (
	"context"
	"errors"
	"fmt"
	"github.com/mailgun/mailgun-go/v4"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"
)

type MailgunNotificationDeliveryService struct {
	base        BaseNotificationDeliveryService
	Mailgun     mailgun.Mailgun
	SenderEmail string
}

type MailgunConfiguration interface {
	GetMailgunDomain() string
	GetMailgunAPIKey() string
	GetMailgunSenderEmail() string
}

type MailgunOption interface {
	// ApplyToMailgun applies this configuration to the given mailgun instance.
	ApplyToMailgun(mailgun.Mailgun)
}

// NewMailgunNotificationDeliveryService creates a delivery service that uses the Mailgun API to deliver email notifications
func NewMailgunNotificationDeliveryService(client client.Client, config MailgunConfiguration, templateLoader TemplateLoader,
	opts ...MailgunOption) NotificationDeliveryService {

	mg := mailgun.NewMailgun(config.GetMailgunDomain(), config.GetMailgunAPIKey())

	for _, opt := range opts {
		opt.ApplyToMailgun(mg)
	}

	svc := &MailgunNotificationDeliveryService{
		base:        BaseNotificationDeliveryService{TemplateLoader: templateLoader},
		Mailgun:     mg,
		SenderEmail: config.GetMailgunSenderEmail(),
	}

	return svc
}

func (s *MailgunNotificationDeliveryService) Send(ctx context.Context, notificationCtx *NotificationContext, templateName string) error {

	template, found, err := s.base.TemplateLoader.GetNotificationTemplate(templateName)
	if err != nil {
		return err
	}

	if !found {
		return errors.New(fmt.Sprintf("notification template [%s] not found", templateName))
	}

	sender := s.SenderEmail

	subject, err := s.base.GenerateContent(ctx, notificationCtx, template.Subject)
	if err != nil {
		return err
	}

	body, err := s.base.GenerateContent(ctx, notificationCtx, template.Content)
	if err != nil {
		return err
	}

	recipient := notificationCtx.FullEmailAddress()

	// The message object allows you to add attachments and Bcc recipients
	message := s.Mailgun.NewMessage(sender, subject, body, recipient)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	// Send the message with a 10 second timeout
	_, _, err = s.Mailgun.Send(ctx, message)
	return err
}
