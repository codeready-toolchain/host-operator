package apis

import (
	"github.com/codeready-toolchain/api/pkg/apis"

	extensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
)

// AddToScheme adds all Resources to the Scheme
func AddToScheme(s *runtime.Scheme) error {
	addToSchemes := append(apis.AddToSchemes, extensionsv1.AddToScheme)

	return addToSchemes.AddToScheme(s)
}
