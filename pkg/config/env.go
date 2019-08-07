package config

import (
	"github.com/operator-framework/operator-sdk/pkg/k8sutil"
	"os"
)

const (
	DefaultOperatorNamespace = "toolchain-host-operator"
)

func GetOperatorNamespace() string {
	ns := os.Getenv(OperatorNamespace)
	if ns != "" {
		return ns
	}

	ns, err := k8sutil.GetOperatorNamespace()
	if err != nil {
		return DefaultOperatorNamespace
	}

	return ns
}

func GetIdP() string {
	// TODO get from openshift
	return "rhd"
}