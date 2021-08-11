package notification

import (
	"context"
	"fmt"
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"

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
	base         BaseNotificationDeliveryService
	Mailgun      mailgun.Mailgun
	SenderEmail  string
	ReplyToEmail string
}

type MailgunConfig interface {
	GetMailgunDomain() string
	GetMailgunAPIKey() string
	GetMailgunSenderEmail() string
	GetMailgunReplyToEmail() string
}

type MailgunOption interface {
	// ApplyToMailgun applies this configuration to the given mailgun instance.
	ApplyToMailgun(mailgun.Mailgun)
}

// NewMailgunNotificationDeliveryService creates a delivery service that uses the Mailgun API to deliver email notifications
func NewMailgunNotificationDeliveryService(config DeliveryServiceFactoryConfig, templateLoader TemplateLoader,
	opts ...MailgunOption) DeliveryService {

	mg := mailgun.NewMailgun(config.GetMailgunDomain(), config.GetMailgunAPIKey())

	for _, opt := range opts {
		opt.ApplyToMailgun(mg)
	}

	return &MailgunNotificationDeliveryService{
		base:         BaseNotificationDeliveryService{TemplateLoader: templateLoader},
		Mailgun:      mg,
		SenderEmail:  config.GetMailgunSenderEmail(),
		ReplyToEmail: config.GetMailgunReplyToEmail(),
	}
}

func (s *MailgunNotificationDeliveryService) Send(notification *toolchainv1alpha1.Notification) error {

	var subject, body string

	if notification.Spec.Template != "" {
		template, found, err := s.base.TemplateLoader.GetNotificationTemplate(notification.Spec.Template)
		if err != nil {
			return err
		}

		if !found {
			return fmt.Errorf("notification template [%s] not found", notification.Spec.Template)
		}

		subject, err = s.base.GenerateContent(notification.Spec.Context, template.Subject)
		if err != nil {
			return err
		}

		body, err = s.base.GenerateContent(notification.Spec.Context, template.Content)
		if err != nil {
			return err
		}
	} else {
		// If there is no template specified then simply use the subject and content provided by the notification
		subject = notification.Spec.Subject
		body = notification.Spec.Content
	}

	if subject == "" && body == "" {
		return fmt.Errorf("no subject or body specified for notification")
	}

	// The message object allows you to add attachments and Bcc recipients
	message := s.Mailgun.NewMessage(s.SenderEmail, subject, "", notification.Spec.Recipient)

	if s.ReplyToEmail != "" {
		message.SetReplyTo(s.ReplyToEmail)
	}

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
