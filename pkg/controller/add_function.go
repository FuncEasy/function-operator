package controller

import (
	"github.com/FuncEasy/function-operator/pkg/controller/function"
)

func init() {
	// AddToManagerFuncs is a list of functions to create controllers and add them to a manager.
	AddToManagerFuncs = append(AddToManagerFuncs, function.Add)
}
