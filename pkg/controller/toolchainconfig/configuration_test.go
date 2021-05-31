package toolchainconfig

import (
	"testing"

	. "github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/stretchr/testify/assert"
)

func TestAutomaticApprovalConfig(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		cfg := newToolchainConfigWithReset(t)
		toolchainCfg := ToolchainConfig{cfg: &cfg.Spec}

		assert.False(t, toolchainCfg.AutomaticApproval().IsEnabled())
		assert.Equal(t, 1000, toolchainCfg.AutomaticApproval().MaxNumberOfUsersOverall())
		assert.Empty(t, toolchainCfg.AutomaticApproval().MaxNumberOfUsersSpecificPerMemberCluster())
		assert.Equal(t, 80, toolchainCfg.AutomaticApproval().ResourceCapacityThresholdDefault())
		assert.Empty(t, toolchainCfg.AutomaticApproval().ResourceCapacityThresholdSpecificPerMemberCluster())
	})
	t.Run("non-default", func(t *testing.T) {
		cfg := newToolchainConfigWithReset(t, AutomaticApproval().Enabled().MaxUsersNumber(123, PerMemberCluster("member1", 321)).ResourceCapThreshold(456, PerMemberCluster("member1", 654)))
		toolchainCfg := ToolchainConfig{cfg: &cfg.Spec}

		assert.True(t, toolchainCfg.AutomaticApproval().IsEnabled())
		assert.Equal(t, 123, toolchainCfg.AutomaticApproval().MaxNumberOfUsersOverall())
		assert.Equal(t, cfg.Spec.Host.AutomaticApproval.MaxNumberOfUsers.SpecificPerMemberCluster, toolchainCfg.AutomaticApproval().MaxNumberOfUsersSpecificPerMemberCluster())
		assert.Equal(t, 456, toolchainCfg.AutomaticApproval().ResourceCapacityThresholdDefault())
		assert.Equal(t, cfg.Spec.Host.AutomaticApproval.ResourceCapacityThreshold.SpecificPerMemberCluster, toolchainCfg.AutomaticApproval().ResourceCapacityThresholdSpecificPerMemberCluster())
	})
}

func TestDeactivationConfig(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		cfg := newToolchainConfigWithReset(t)
		toolchainCfg := ToolchainConfig{cfg: &cfg.Spec}

		assert.Equal(t, 3, toolchainCfg.Deactivation().DeactivatingNotificationInDays())
	})
	t.Run("non-default", func(t *testing.T) {
		cfg := newToolchainConfigWithReset(t, Deactivation().DeactivatingNotificationDays(5))
		toolchainCfg := ToolchainConfig{cfg: &cfg.Spec}

		assert.Equal(t, 5, toolchainCfg.Deactivation().DeactivatingNotificationInDays())
	})
}
