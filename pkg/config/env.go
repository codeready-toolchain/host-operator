package config

import (
	"os"
)

const (
	DefaultOperatorNamespace = "toolchain-host"
)

func GetOperatorNamespace() string {
	ns := os.Getenv(OperatorNamespace)
	if ns != "" {
		return ns
	}

	return DefaultOperatorNamespace
}

func GetIdP() string {
	// TODO get from openshift
	return "rhd"
}