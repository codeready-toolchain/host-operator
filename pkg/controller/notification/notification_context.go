package notification

import (
	"context"
	"fmt"
	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/configuration"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NotificationContext is used to pass user-specific data to the notification template parser in order to generate
// customised notification content.
type NotificationContext struct {
	UserID          string
	FirstName       string
	LastName        string
	UserEmail       string
	CompanyName     string
	RegistrationURL string
}

// NewNotificationContext creates a new NotificationContext by looking up the UserSignup with the specified userID
// and using it to populate the context fields
func NewNotificationContext(client client.Client, userID, namespace string, config *configuration.Config) (*NotificationContext, error) {
	// Lookup the UserSignup resource with the specified userID
	instance := &toolchainv1alpha1.UserSignup{}
	err := client.Get(context.TODO(), types.NamespacedName{
		Namespace: namespace,
		Name:      userID,
	}, instance)

	if err != nil {
		return nil, err
	}

	notificationCtx := &NotificationContext{
		UserID:      userID,
		FirstName:   instance.Spec.GivenName,
		LastName:    instance.Spec.FamilyName,
		CompanyName: instance.Spec.Company,
	}

	if emailLbl, exists := instance.Annotations[toolchainv1alpha1.UserSignupUserEmailAnnotationKey]; exists {
		notificationCtx.UserEmail = emailLbl
	}

	if config != nil {
		notificationCtx.RegistrationURL = config.GetRegistrationServiceURL()
	}

	return notificationCtx, nil
}

func (c *NotificationContext) FullEmailAddress() string {
	return fmt.Sprintf("%s %s<%s>",
		c.FirstName,
		c.LastName,
		c.UserEmail)
}
