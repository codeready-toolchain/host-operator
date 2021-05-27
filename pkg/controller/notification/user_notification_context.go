package notification

import (
	"context"
	"errors"
	"fmt"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/configuration"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// UserNotificationContext is used to pass user-specific data to the notification template parser in order to generate
// customised notification content.
type UserNotificationContext struct {
	UserID          string
	FirstName       string
	LastName        string
	UserEmail       string
	CompanyName     string
	RegistrationURL string
}

// NewUserNotificationContext creates a new UserNotificationContext by looking up the UserSignup with the specified userID
// and using it to populate the context fields
func NewUserNotificationContext(client client.Client, userID, namespace string, config *configuration.Config) (*UserNotificationContext, error) {
	if config == nil {
		return nil, errors.New("configuration was not provided")
	}

	// Lookup the UserSignup resource with the specified userID
	instance := &toolchainv1alpha1.UserSignup{}
	err := client.Get(context.TODO(), types.NamespacedName{
		Namespace: namespace,
		Name:      userID,
	}, instance)

	if err != nil {
		return nil, err
	}

	notificationCtx := &UserNotificationContext{
		UserID:      userID,
		FirstName:   instance.Spec.GivenName,
		LastName:    instance.Spec.FamilyName,
		CompanyName: instance.Spec.Company,
	}

	if emailLbl, exists := instance.Annotations[toolchainv1alpha1.UserSignupUserEmailAnnotationKey]; exists {
		notificationCtx.UserEmail = emailLbl
	}

	notificationCtx.RegistrationURL = config.GetRegistrationServiceURL()

	return notificationCtx, nil
}

func (c *UserNotificationContext) DeliveryEmail() string {
	return fmt.Sprintf("%s %s<%s>",
		c.FirstName,
		c.LastName,
		c.UserEmail)
}

func (c *UserNotificationContext) KeysAndValues() []interface{} {
	return []interface{}{
		"UserID", c.UserID,
		"UserEmail", c.UserEmail,
		"DeliveryEmail", c.DeliveryEmail(),
		"FirstName", c.FirstName,
		"LastName", c.LastName,
		"CompanyName", c.CompanyName,
		"RegistrationURL", c.RegistrationURL,
	}
}
