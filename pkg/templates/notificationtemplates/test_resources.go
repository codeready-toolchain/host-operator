package notificationtemplates

import "embed"

//go:embed testTemplates/*
var fakeTemplates embed.FS
