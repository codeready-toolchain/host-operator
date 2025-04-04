package nstemplatetiers

import "embed"

//go:embed testtemplates/fakenstemplatetiers/*
var FakeNSTemplatTierFS embed.FS
