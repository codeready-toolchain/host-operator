package notification

type Context interface {
	DeliveryEmail() string
}

// AdminNotificationContext is used to generate a notification for sending to an admin mailing list
type AdminNotificationContext struct {
	AdminEmail string
}

// NewAdminNotificationContext creates a new AdminNotificationContext with the provided values
func NewAdminNotificationContext(adminEmail string) *AdminNotificationContext {

	notificationCtx := &AdminNotificationContext{
		AdminEmail: adminEmail,
	}

	return notificationCtx
}

func (c *AdminNotificationContext) DeliveryEmail() string {
	return c.AdminEmail
}
