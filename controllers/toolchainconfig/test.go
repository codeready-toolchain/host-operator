package toolchainconfig

import (
	"context"
	"os"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	testconfig "github.com/codeready-toolchain/toolchain-common/pkg/test/config"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewToolchainConfigWithReset(t *testing.T, options ...testconfig.ToolchainConfigOption) *toolchainv1alpha1.ToolchainConfig {
	os.Setenv("WATCH_NAMESPACE", test.HostOperatorNs)
	t.Cleanup(Reset)
	return newToolchainConfig(options...)
}

func UpdateToolchainConfigWithReset(t *testing.T, cl client.Client, options ...testconfig.ToolchainConfigOption) *toolchainv1alpha1.ToolchainConfig {
	currentConfig := &toolchainv1alpha1.ToolchainConfig{}
	err := cl.Get(context.TODO(), types.NamespacedName{Namespace: test.HostOperatorNs, Name: "config"}, currentConfig)
	require.NoError(t, err)
	t.Cleanup(Reset)
	return modifyToolchainConfig(currentConfig, options...)
}

func newToolchainConfig(options ...testconfig.ToolchainConfigOption) *toolchainv1alpha1.ToolchainConfig {
	toolchainConfig := &toolchainv1alpha1.ToolchainConfig{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: test.HostOperatorNs,
			Name:      "config",
		},
	}
	for _, option := range options {
		option.Apply(toolchainConfig)
	}
	return toolchainConfig
}

func modifyToolchainConfig(toolchainConfig *toolchainv1alpha1.ToolchainConfig, options ...testconfig.ToolchainConfigOption) *toolchainv1alpha1.ToolchainConfig {
	for _, option := range options {
		option.Apply(toolchainConfig)
	}
	return toolchainConfig
}
