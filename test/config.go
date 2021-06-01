package test

import (
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/controllers/toolchainconfig"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
)

func NewToolchainConfigWithReset(t *testing.T, options ...test.ToolchainConfigOption) *toolchainv1alpha1.ToolchainConfig {
	t.Cleanup(toolchainconfig.Reset)
	return test.NewToolchainConfig(options...)
}
