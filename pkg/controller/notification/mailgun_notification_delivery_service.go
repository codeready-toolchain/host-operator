package notification

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/mailgun/mailgun-go/v4"
)

type MailgunDeliveryError struct {
	id           string
	response     string
	errorMessage string
}

func (e MailgunDeliveryError) Error() string {
	return fmt.Sprintf("error while delivering notification (ID: %s, Response: %s) - %s", e.id, e.response, e.errorMessage)
}

func NewMailgunDeliveryError(id, response, errorMessage string) error {
	return MailgunDeliveryError{
		id:           id,
		response:     response,
		errorMessage: errorMessage,
	}
}

type MailgunNotificationDeliveryService struct {
	base        BaseNotificationDeliveryService
	Mailgun     mailgun.Mailgun
	SenderEmail string
}

type MailgunConfig interface {
	GetMailgunDomain() string
	GetMailgunAPIKey() string
	GetMailgunSenderEmail() string
}

type MailgunOption interface {
	// ApplyToMailgun applies this configuration to the given mailgun instance.
	ApplyToMailgun(mailgun.Mailgun)
}

// NewMailgunNotificationDeliveryService creates a delivery service that uses the Mailgun API to deliver email notifications
func NewMailgunNotificationDeliveryService(config NotificationDeliveryServiceFactoryConfig, templateLoader TemplateLoader,
	opts ...MailgunOption) NotificationDeliveryService {

	mg := mailgun.NewMailgun(config.GetMailgunDomain(), config.GetMailgunAPIKey())

	for _, opt := range opts {
		opt.ApplyToMailgun(mg)
	}

	return &MailgunNotificationDeliveryService{
		base:        BaseNotificationDeliveryService{TemplateLoader: templateLoader},
		Mailgun:     mg,
		SenderEmail: config.GetMailgunSenderEmail(),
	}
}

func (s *MailgunNotificationDeliveryService) Send(ctx context.Context, notificationCtx *NotificationContext, templateName string) error {

	template, found, err := s.base.TemplateLoader.GetNotificationTemplate(templateName)
	if err != nil {
		return err
	}

	if !found {
		return errors.New(fmt.Sprintf("notification template [%s] not found", templateName))
	}

	subject, err := s.base.GenerateContent(notificationCtx, template.Subject)
	if err != nil {
		return err
	}

	body, err := s.base.GenerateContent(notificationCtx, template.Content)
	if err != nil {
		return err
	}

	// The message object allows you to add attachments and Bcc recipients
	message := s.Mailgun.NewMessage(s.SenderEmail, subject, "", notificationCtx.FullEmailAddress())
	message.SetHtml(body)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	// Send the message with a 10 second timeout
	response, id, err := s.Mailgun.Send(ctx, message)
	if err != nil {
		return NewMailgunDeliveryError(id, response, err.Error())
	}

	return nil
}
