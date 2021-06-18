package toolchainconfig

import toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"

type ToolchainConfig struct {
	cfg *toolchainv1alpha1.ToolchainConfigSpec
}

func NewToolchainConfig(cfg *toolchainv1alpha1.ToolchainConfigSpec) ToolchainConfig {
	return ToolchainConfig{
		cfg: cfg,
	}
}

func (c *ToolchainConfig) AutomaticApproval() AutoApprovalConfig {
	return AutoApprovalConfig{c.cfg.Host.AutomaticApproval}
}

func (c *ToolchainConfig) Deactivation() DeactivationConfig {
	return DeactivationConfig{c.cfg.Host.Deactivation}
}

func (c *ToolchainConfig) Metrics() MetricsConfig {
	return MetricsConfig{c.cfg.Host.Metrics}
}

type AutoApprovalConfig struct {
	approval toolchainv1alpha1.AutomaticApprovalConfig
}

func (a AutoApprovalConfig) IsEnabled() bool {
	return getBool(a.approval.Enabled, false)
}

func (a AutoApprovalConfig) ResourceCapacityThresholdDefault() int {
	return getInt(a.approval.ResourceCapacityThreshold.DefaultThreshold, 80)
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
	dctv toolchainv1alpha1.DeactivationConfig
}

func (d DeactivationConfig) DeactivatingNotificationInDays() int {
	return getInt(d.dctv.DeactivatingNotificationDays, 3)
}

type MetricsConfig struct {
	metrics toolchainv1alpha1.MetricsConfig
}

func (d MetricsConfig) ForceSynchronization() bool {
	return getBool(d.metrics.ForceSynchronization, false)
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
