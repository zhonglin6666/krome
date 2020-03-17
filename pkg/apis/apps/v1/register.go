// NOTE: Boilerplate only.  Ignore this file.

// Package v1 contains API Schema definitions for the apps v1 API group
// +k8s:deepcopy-gen=package,register
// +groupName=apps.krome.io
package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	// SchemeGroupVersion is group version used to register these objects
	SchemeGroupVersion = schema.GroupVersion{Group: "apps.krome.io", Version: "v1"}

	// SchemeBuilder is used to add go types to the GroupVersionKind scheme
	// SchemeBuilder = &scheme.Builder{GroupVersion: SchemeGroupVersion}
	SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)

	// AddToScheme is a global function that registers this API group & version to a scheme
	AddToScheme = SchemeBuilder.AddToScheme
)

func Resource(resource string) schema.GroupResource {
	return SchemeGroupVersion.WithResource(resource).GroupResource()
}

func Kind(kind string) schema.GroupKind {
	return SchemeGroupVersion.WithKind(kind).GroupKind()
}

func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(
		SchemeGroupVersion,
		&Statefulset{},
		&StatefulsetList{},
	)

	// register the type in the scheme
	metav1.AddToGroupVersion(scheme, SchemeGroupVersion)
	return nil
}
