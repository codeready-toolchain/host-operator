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

func (s *MailgunNotificationDeliveryService) Deliver(ctx context.Context, notificationCtx NotificationContext, templateName string) error {
	// TODO implement
	return nil
}
