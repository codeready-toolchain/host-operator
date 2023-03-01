package toolchainconfig

type DeliveryServiceFactoryConfig struct {
	ToolchainConfig
}

func (d DeliveryServiceFactoryConfig) GetNotificationDeliveryService() string {
	return d.Notifications().NotificationDeliveryService()
}

func (d DeliveryServiceFactoryConfig) GetMailgunDomain() string {
	return d.Notifications().MailgunDomain()
}

func (d DeliveryServiceFactoryConfig) GetMailgunAPIKey() string {
	return d.Notifications().MailgunAPIKey()
}

func (d DeliveryServiceFactoryConfig) GetMailgunSenderEmail() string {
	return d.Notifications().MailgunSenderEmail()
}

func (d DeliveryServiceFactoryConfig) GetMailgunReplyToEmail() string {
	return d.Notifications().MailgunReplyToEmail()
}
