package deploy

import "embed"

//go:embed templates/notificationtemplates/*
var NotificationTemplateFS embed.FS
