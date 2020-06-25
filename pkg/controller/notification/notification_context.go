package notification

import (
	"context"
	"fmt"
	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NotificationContext
type NotificationContext struct {
	UserID      string
	FirstName   string
	LastName    string
	Email       string
	CompanyName string
}

// NewNotificationContext
func NewNotificationContext(ctx context.Context, client client.Client, userID, namespace string) (*NotificationContext, error) {
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
		notificationCtx.Email = emailLbl
	}

	return notificationCtx, nil
}

func (c *NotificationContext) FullEmailAddress() string {
	return fmt.Sprintf("%s %s<%s>",
		c.FirstName,
		c.LastName,
		c.Email)
}
