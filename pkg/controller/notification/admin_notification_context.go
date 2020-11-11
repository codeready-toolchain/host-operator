package notification

import (
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// AdminNotificationContext is used to pass toolchain status state to the notification template parser in order to generate
// customised notification content.
type AdminNotificationContext struct {
	AdminEmail string
}

// NewAdminNotificationContext creates a new AdminNotificationContext with the provided values
func NewAdminNotificationContext(client client.Client, adminEmail string) (*AdminNotificationContext, error) {

	notificationCtx := &AdminNotificationContext{
		AdminEmail: adminEmail,
	}

	return notificationCtx, nil
}

func (c *AdminNotificationContext) DeliveryEmail() string {
	return c.AdminEmail
}
