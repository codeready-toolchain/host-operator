package toolchainconfig

import "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"

type Config struct {
	toolchainconfig *v1alpha1.ToolchainConfigSpec
}

func (c *Config) AutomaticApproval() *autoApproval {
	return &autoApproval{c.toolchainconfig.Host.AutomaticApproval}
}

func (c *Config) Deactivation() *deactivation {
	return &deactivation{c.toolchainconfig.Host.Deactivation}
}

type autoApproval struct {
	approval v1alpha1.AutomaticApprovalCfg
}

func (a *autoApproval) IsEnabled() bool {
	return getBool(a.approval.Enabled, false)
}

func (a *autoApproval) ResourceCapacityThresholdDefault() int {
	return getInt(a.approval.ResourceCapacityThreshold.DefaultThreshold, 0)
}

func (a *autoApproval) ResourceCapacityThresholdSpecificPerMemberCluster() map[string]int {
	return a.approval.ResourceCapacityThreshold.SpecificPerMemberCluster
}

func (a *autoApproval) MaxNumberOfUsersOverall() int {
	return getInt(a.approval.MaxNumberOfUsers.Overall, 0)
}

func (a *autoApproval) MaxNumberOfUsersSpecificPerMemberCluster() map[string]int {
	return a.approval.MaxNumberOfUsers.SpecificPerMemberCluster
}

type deactivation struct {
	dctv v1alpha1.DeactivationCfg
}

func (d *deactivation) DeactivatingNotificationInDays() int {
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
