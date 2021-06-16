package test

import (
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/controllers/toolchainconfig"
	testconfig "github.com/codeready-toolchain/toolchain-common/pkg/test/config"
)

func NewToolchainConfigWithReset(t *testing.T, options ...testconfig.ToolchainConfigOption) *toolchainv1alpha1.ToolchainConfig {
	t.Cleanup(toolchainconfig.Reset)
	return testconfig.NewToolchainConfig(options...)
}
