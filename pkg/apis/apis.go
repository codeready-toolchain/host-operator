package apis

import (
	api "github.com/codeready-toolchain/api/api/v1alpha1"

	extensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
)

// AddToScheme adds all Resources to the Scheme
func AddToScheme(s *runtime.Scheme) error {
	var AddToSchemes runtime.SchemeBuilder
	addToSchemes := append(AddToSchemes, api.AddToScheme, extensionsv1.AddToScheme)
	return addToSchemes.AddToScheme(s)
}
