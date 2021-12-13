package apis

import (
	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"

	routev1 "github.com/openshift/api/route/v1"
	templatev1 "github.com/openshift/api/template/v1"
	extensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// AddToScheme adds all Resources to the Scheme
func AddToScheme(s *runtime.Scheme) error {
	addToSchemes := append(runtime.SchemeBuilder{}, toolchainv1alpha1.AddToScheme, extensionsv1.AddToScheme, templatev1.Install, routev1.Install)
	return addToSchemes.AddToScheme(s)
}
