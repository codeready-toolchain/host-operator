package controller

import (
	"github.com/codeready-toolchain/host-operator/pkg/controller/masteruserrecord"
)

func init() {
	// AddToManagerFuncs is a list of functions to create controllers and add them to a manager.
	AddToManagerFuncs = append(AddToManagerFuncs, masteruserrecord.Add)
}
