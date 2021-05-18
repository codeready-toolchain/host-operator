package toolchainconfig

import "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"

type ToolchainConfig struct {
	cfg *v1alpha1.ToolchainConfigSpec
}

func (c *ToolchainConfig) AutomaticApproval() AutoApprovalCfg {
	return AutoApprovalCfg{c.cfg.Host.AutomaticApproval}
}

func (c *ToolchainConfig) Deactivation() DeactivationCfg {
	return DeactivationCfg{c.cfg.Host.Deactivation}
}

type AutoApprovalCfg struct {
	approval v1alpha1.AutomaticApprovalCfg
}

func (a AutoApprovalCfg) IsEnabled() bool {
	return getBool(a.approval.Enabled, false)
}

func (a AutoApprovalCfg) ResourceCapacityThresholdDefault() int {
	return getInt(a.approval.ResourceCapacityThreshold.DefaultThreshold, 0)
}

func (a AutoApprovalCfg) ResourceCapacityThresholdSpecificPerMemberCluster() map[string]int {
	return a.approval.ResourceCapacityThreshold.SpecificPerMemberCluster
}

func (a AutoApprovalCfg) MaxNumberOfUsersOverall() int {
	return getInt(a.approval.MaxNumberOfUsers.Overall, 0)
}

func (a AutoApprovalCfg) MaxNumberOfUsersSpecificPerMemberCluster() map[string]int {
	return a.approval.MaxNumberOfUsers.SpecificPerMemberCluster
}

type DeactivationCfg struct {
	dctv v1alpha1.DeactivationCfg
}

func (d DeactivationCfg) DeactivatingNotificationInDays() int {
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
