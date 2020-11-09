package notification

import (
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ToolchainStatusNotificationContext is used to pass toolchain status state to the notification template parser in order to generate
// customised notification content.
type ToolchainStatusNotificationContext struct {
	AdminEmail    string
	NotReadySince string
}

// NewToolchainStatusNotificationContext creates a new ToolchainStatusNotificationContext with the provided values
func NewToolchainStatusNotificationContext(client client.Client, adminEmail, notReadySince string) (*ToolchainStatusNotificationContext, error) {

	// TODO lookup the toolchain status here and extract the timestamp when the not ready condition was set

	notificationCtx := &ToolchainStatusNotificationContext{
		AdminEmail:    adminEmail,
		NotReadySince: notReadySince,
	}

	return notificationCtx, nil
}

func (c *ToolchainStatusNotificationContext) DeliveryEmail() string {
	return c.AdminEmail
}
