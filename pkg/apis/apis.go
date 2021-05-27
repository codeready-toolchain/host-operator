package apis

import (
	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"

	extensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
)

// AddToScheme adds all Resources to the Scheme
func AddToScheme(s *runtime.Scheme) error {
	var AddToSchemes runtime.SchemeBuilder
	addToSchemes := append(AddToSchemes, toolchainv1alpha1.AddToScheme, extensionsv1.AddToScheme)
	return addToSchemes.AddToScheme(s)
}
