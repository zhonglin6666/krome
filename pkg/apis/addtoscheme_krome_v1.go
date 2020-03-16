package apis

import (
	v1 "github.com/zhonglin6666/krome/pkg/apis/krome/v1"
)

func init() {
	// Register the types with the Scheme so the components can map objects to GroupVersionKinds and back
	AddToSchemes = append(AddToSchemes, v1.SchemeBuilder.AddToScheme)
}
