package notification

type NotificationContext interface {
	DeliveryEmail() string
}
