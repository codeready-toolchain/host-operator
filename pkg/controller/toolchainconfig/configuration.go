package toolchainconfig

import "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"

type ToolchainConfig struct {
	cfg *v1alpha1.ToolchainConfigSpec
}

func (c *ToolchainConfig) AutomaticApproval() AutoApprovalConfig {
	return AutoApprovalConfig{c.cfg.Host.AutomaticApproval}
}

func (c *ToolchainConfig) Deactivation() DeactivationConfig {
	return DeactivationConfig{c.cfg.Host.Deactivation}
}

type AutoApprovalConfig struct {
	approval v1alpha1.AutomaticApproval
}

func (a AutoApprovalConfig) IsEnabled() bool {
	return getBool(a.approval.Enabled, false)
}

func (a AutoApprovalConfig) ResourceCapacityThresholdDefault() int {
	return getInt(a.approval.ResourceCapacityThreshold.DefaultThreshold, 0)
}

func (a AutoApprovalConfig) ResourceCapacityThresholdSpecificPerMemberCluster() map[string]int {
	return a.approval.ResourceCapacityThreshold.SpecificPerMemberCluster
}

func (a AutoApprovalConfig) MaxNumberOfUsersOverall() int {
	return getInt(a.approval.MaxNumberOfUsers.Overall, 1000)
}

func (a AutoApprovalConfig) MaxNumberOfUsersSpecificPerMemberCluster() map[string]int {
	return a.approval.MaxNumberOfUsers.SpecificPerMemberCluster
}

type DeactivationConfig struct {
	dctv v1alpha1.Deactivation
}

func (d DeactivationConfig) DeactivatingNotificationInDays() int {
	return getInt(d.dctv.DeactivatingNotificationDays, 3)
}

func getBool(value *bool, defaultValue bool) bool {
	if value != nil {
		return *value
	}
	return defaultValue
}

func getInt(value *int, defaultValue int) int {
	if value != nil {
		return *value
	}
	return defaultValue
}
