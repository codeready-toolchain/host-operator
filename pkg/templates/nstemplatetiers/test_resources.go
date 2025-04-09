package nstemplatetiers

import "embed"

//go:embed testtemplates/fakenstemplatetiers/*
var FakeNSTemplateTierFS embed.FS
