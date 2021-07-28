package notification

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/types"

	"k8s.io/apimachinery/pkg/runtime"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/gofrs/uuid"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type Option = func(notification *toolchainv1alpha1.Notification) error

type Builder interface {
	WithTemplate(template string) Builder
	WithSubjectAndContent(subject, content string) Builder
	WithNotificationType(notificationType string) Builder
	WithControllerReference(owner v1.Object, scheme *runtime.Scheme) Builder
	WithKeysAndValues(keysAndValues map[string]string) Builder
	WithUserContext(userID string) Builder
	Create(recipient string) (*toolchainv1alpha1.Notification, error)
}

func NewNotificationBuilder(client client.Client, namespace string) Builder {
	return &notificationBuilderImpl{
		client:    client,
		namespace: namespace,
		options:   []Option{},
	}
}

type notificationBuilderImpl struct {
	client    client.Client
	namespace string
	options   []Option
}

func (b *notificationBuilderImpl) Create(recipient string) (*toolchainv1alpha1.Notification, error) {

	notification := &toolchainv1alpha1.Notification{
		ObjectMeta: v1.ObjectMeta{
			Namespace: b.namespace,
			Labels:    map[string]string{},
		},
		Spec: toolchainv1alpha1.NotificationSpec{
			Recipient: recipient,
			Context:   make(map[string]string),
		},
	}

	for _, opt := range b.options {
		err := opt(notification)
		if err != nil {
			return nil, err
		}
	}

	generateName(notification)

	return notification, b.client.Create(context.TODO(), notification)
}

func generateName(notification *toolchainv1alpha1.Notification) {
	username, found := notification.Spec.Context["UserName"]
	if found {
		notificationType, found := notification.Labels[toolchainv1alpha1.NotificationTypeLabelKey]
		if found {
			notification.ObjectMeta.GenerateName = fmt.Sprintf("%s-%s-", username, notificationType)
			return
		}
		notification.ObjectMeta.GenerateName = fmt.Sprintf("%s-untyped", username)
		return
	}

	notification.ObjectMeta.GenerateName = fmt.Sprintf("%s-untyped", uuid.Must(uuid.NewV4()).String())
}

func (b *notificationBuilderImpl) WithTemplate(template string) Builder {
	b.options = append(b.options, func(n *toolchainv1alpha1.Notification) error {
		n.Spec.Template = template
		return nil
	})
	return b
}

func (b *notificationBuilderImpl) WithSubjectAndContent(subject, content string) Builder {
	b.options = append(b.options, func(n *toolchainv1alpha1.Notification) error {
		n.Spec.Subject = subject
		n.Spec.Content = content
		return nil
	})
	return b
}

func (b *notificationBuilderImpl) WithNotificationType(notificationType string) Builder {
	b.options = append(b.options, func(n *toolchainv1alpha1.Notification) error {
		n.ObjectMeta.Labels[toolchainv1alpha1.NotificationTypeLabelKey] = notificationType
		return nil
	})
	return b
}

func (b *notificationBuilderImpl) WithControllerReference(owner v1.Object, scheme *runtime.Scheme) Builder {
	b.options = append(b.options, func(n *toolchainv1alpha1.Notification) error {
		return controllerutil.SetControllerReference(owner, n, scheme)
	})
	return b
}

func (b *notificationBuilderImpl) WithKeysAndValues(keysAndValues map[string]string) Builder {
	b.options = append(b.options, func(n *toolchainv1alpha1.Notification) error {
		for k, v := range keysAndValues {
			n.Spec.Context[k] = v
		}
		return nil
	})
	return b
}

func (b *notificationBuilderImpl) WithUserContext(userID string) Builder {
	b.options = append(b.options, func(n *toolchainv1alpha1.Notification) error {
		// Lookup the UserSignup resource with the specified userID
		instance := &toolchainv1alpha1.UserSignup{}
		err := b.client.Get(context.TODO(), types.NamespacedName{
			Namespace: b.namespace,
			Name:      userID,
		}, instance)

		if err != nil {
			return err
		}

		n.Spec.Context["UserID"] = userID
		n.Spec.Context["UserName"] = instance.Status.CompliantUsername
		n.Spec.Context["FirstName"] = instance.Spec.GivenName
		n.Spec.Context["LastName"] = instance.Spec.FamilyName
		n.Spec.Context["CompanyName"] = instance.Spec.Company

		n.ObjectMeta.Labels[toolchainv1alpha1.NotificationUserNameLabelKey] = instance.Status.CompliantUsername

		if emailLbl, exists := instance.Annotations[toolchainv1alpha1.UserSignupUserEmailAnnotationKey]; exists {
			n.Spec.Context["UserEmail"] = emailLbl
		}

		return nil
	})
	return b
}
