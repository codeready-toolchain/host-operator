package apis

import (
	"github.com/codeready-toolchain/api/pkg/apis"

	templatev1 "github.com/openshift/api/template/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// AddToScheme adds all Resources to the Scheme
func AddToScheme(s *runtime.Scheme) error {
	addToSchemes := append(apis.AddToSchemes, templatev1.Install)
	return addToSchemes.AddToScheme(s)
}
