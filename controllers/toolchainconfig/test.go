package toolchainconfig

import (
	"os"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	testconfig "github.com/codeready-toolchain/toolchain-common/pkg/test/config"
)

func NewToolchainConfigWithReset(t *testing.T, options ...testconfig.ToolchainConfigOption) *toolchainv1alpha1.ToolchainConfig {
	os.Setenv("WATCH_NAMESPACE", test.HostOperatorNs)
	t.Cleanup(Reset)
	return testconfig.NewToolchainConfig(options...)
}
