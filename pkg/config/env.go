package config

import (
	"os"
)

func GetOperatorNamespace() string {
	return os.Getenv(OperatorNamespace)
}

func GetIdP() string {
	// TODO get from openshift
	return "rhd"
}