package apis

import (
	"github.com/codeready-toolchain/api/pkg/apis"
	"k8s.io/apimachinery/pkg/runtime"
)

// AddToScheme adds all Resources to the Scheme
func AddToScheme(s *runtime.Scheme) error {
	return apis.AddToSchemes.AddToScheme(s)
}
