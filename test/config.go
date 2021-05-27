package test

import (
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/controller/hostoperatorconfig"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
)

func NewHostOperatorConfigWithReset(t *testing.T, options ...test.HostOperatorConfigOption) *toolchainv1alpha1.HostOperatorConfig {
	t.Cleanup(hostoperatorconfig.Reset)
	return test.NewHostOperatorConfig(options...)
}
