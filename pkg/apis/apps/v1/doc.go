// Package v1 contains API Schema definitions for the apps v1 API group
// +k8s:deepcopy-gen=package
// +groupName=apps.krome.io

package v1

// defaulter-gen --input-dirs krome/pkg/apis/apps/v1 --input-dirs krome/pkg/apis/apps
// -o $GOPATH/src --go-header-file boilerplate.go.txt
// -O zz_generated.defaults
// --extra-peer-dirs= k8s.io/apimachinery/pkg/apis/meta/v1,k8s.io/apimachinery/pkg/conversion,k8s.io/apimachinery/pkg/runtime
