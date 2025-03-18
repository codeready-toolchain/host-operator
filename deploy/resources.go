package deploy

import "embed"

//go:embed templates/notificationtemplates/*
var NotificationTemplateFS embed.FS

//go:embed templates/toolchaincluster/*
var ToolchainClusterTemplateFS embed.FS

//go:embed registration-service/*
var RegistrationServiceFS embed.FS
