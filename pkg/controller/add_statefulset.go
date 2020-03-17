package controller

import (
	"github.com/zhonglin6666/krome/pkg/controller/statefulset"
)

func init() {
	// AddToManagerFuncs is a list of functions to create controllers and add them to a manager.
	AddToManagerFuncs = append(AddToManagerFuncs, statefulset.Add)
}
