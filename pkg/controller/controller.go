package controller

import (
	"github.com/codeready-toolchain/host-operator/pkg/controller/masteruserrecord"
	"github.com/codeready-toolchain/host-operator/pkg/controller/registrationservice"
	"github.com/codeready-toolchain/host-operator/pkg/controller/usersignup"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// addToManagerFuncs is a list of functions to add all Controllers to the Manager
var addToManagerFuncs []func(manager.Manager) error

func init() {
	addToManagerFuncs = append(addToManagerFuncs, masteruserrecord.Add)
	addToManagerFuncs = append(addToManagerFuncs, registrationservice.Add)
	addToManagerFuncs = append(addToManagerFuncs, usersignup.Add)
}

// AddToManager adds all Controllers to the Manager
func AddToManager(m manager.Manager) error {
	for _, f := range addToManagerFuncs {
		if err := f(m); err != nil {
			return err
		}
	}
	return nil
}
