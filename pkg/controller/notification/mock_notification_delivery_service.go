package notification

import (
	"context"
)

type MockNotificationDeliveryService struct {
}

// NewMockNotificationDeliveryService creates a new mock notification delivery service, used for testing
func NewMockNotificationDeliveryService() NotificationDeliveryService {
	return &MockNotificationDeliveryService{}
}

func (s *MockNotificationDeliveryService) Send(ctx context.Context, notificationCtx *NotificationContext, templateName string) error {
	// TODO implement
	return nil
}
