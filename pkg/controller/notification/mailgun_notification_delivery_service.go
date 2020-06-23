package notification

import (
	"context"
)

type MailgunNotificationDeliveryService struct {
}

// NewMailgunNotificationDeliveryService
func NewMailgunNotificationDeliveryService() NotificationDeliveryService {
	return &MailgunNotificationDeliveryService{}
}

func (s *MailgunNotificationDeliveryService) Send(ctx context.Context, notificationCtx NotificationContext, templateName string) error {
	// TODO implement
	return nil
}
