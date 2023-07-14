package test

import (
	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/controllers/toolchainconfig"
	condition2 "github.com/codeready-toolchain/toolchain-common/pkg/condition"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ToolchainStatusOption func(*toolchainv1alpha1.ToolchainStatus)

func NewToolchainStatus(options ...ToolchainStatusOption) *toolchainv1alpha1.ToolchainStatus {
	toolchainStatus := &toolchainv1alpha1.ToolchainStatus{
		ObjectMeta: metav1.ObjectMeta{
			Name:      toolchainconfig.ToolchainStatusName,
			Namespace: test.HostOperatorNs,
		},
	}
	for _, apply := range options {
		apply(toolchainStatus)
	}
	return toolchainStatus
}

func WithHost(options ...HostToolchainStatusOption) ToolchainStatusOption {
	return func(status *toolchainv1alpha1.ToolchainStatus) {
		host := &toolchainv1alpha1.HostOperatorStatus{}
		for _, modify := range options {
			modify(host)
		}
		status.Status.HostOperator = host
	}
}

type HostToolchainStatusOption func(*toolchainv1alpha1.HostOperatorStatus)

func WithRegistrationService(options ...RegistrationServiceToolchainStatusOption) ToolchainStatusOption {
	return func(status *toolchainv1alpha1.ToolchainStatus) {
		regService := &toolchainv1alpha1.HostRegistrationServiceStatus{
			Deployment:                   toolchainv1alpha1.RegistrationServiceDeploymentStatus{},
			RegistrationServiceResources: toolchainv1alpha1.RegistrationServiceResourcesStatus{},
			Health:                       toolchainv1alpha1.RegistrationServiceHealth{},
			RevisionCheck:                toolchainv1alpha1.RevisionCheck{},
		}
		for _, modify := range options {
			modify(regService)
		}

		status.Status.RegistrationService = regService
	}
}

type RegistrationServiceToolchainStatusOption func(status *toolchainv1alpha1.HostRegistrationServiceStatus)

func WithDeploymentCondition(condition toolchainv1alpha1.Condition) RegistrationServiceToolchainStatusOption {
	return func(status *toolchainv1alpha1.HostRegistrationServiceStatus) {
		status.Deployment.Conditions, _ = condition2.AddOrUpdateStatusConditions(status.Deployment.Conditions, condition)
	}
}

func WithHealthCondition(condition toolchainv1alpha1.Condition) RegistrationServiceToolchainStatusOption {
	return func(status *toolchainv1alpha1.HostRegistrationServiceStatus) {
		status.Health.Conditions, _ = condition2.AddOrUpdateStatusConditions(status.Health.Conditions, condition)
	}
}

func WithRevisionCheckCondition(condition toolchainv1alpha1.Condition) RegistrationServiceToolchainStatusOption {
	return func(status *toolchainv1alpha1.HostRegistrationServiceStatus) {
		status.RevisionCheck.Conditions, _ = condition2.AddOrUpdateStatusConditions(status.RevisionCheck.Conditions, condition)
	}
}

func WithMember(name string, options ...MemberToolchainStatusOption) ToolchainStatusOption {
	return func(status *toolchainv1alpha1.ToolchainStatus) {
		member := toolchainv1alpha1.Member{
			APIEndpoint: "http://api.devcluster.openshift.com",
			ClusterName: name,
		}
		for _, modify := range options {
			modify(&member)
		}
		status.Status.Members = append(status.Status.Members, member)
	}
}

type MemberToolchainStatusOption func(*toolchainv1alpha1.Member)

func WithSpaceCount(count int) MemberToolchainStatusOption {
	return func(status *toolchainv1alpha1.Member) {
		status.SpaceCount = count
	}
}

func WithNodeRoleUsage(role string, usage int) MemberToolchainStatusOption {
	return func(status *toolchainv1alpha1.Member) {
		if status.MemberStatus.ResourceUsage.MemoryUsagePerNodeRole == nil {
			status.MemberStatus.ResourceUsage.MemoryUsagePerNodeRole = map[string]int{}
		}
		status.MemberStatus.ResourceUsage.MemoryUsagePerNodeRole[role] = usage
	}
}

func WithRoutes(consoleURL, cheURL string, condition toolchainv1alpha1.Condition) MemberToolchainStatusOption {
	return func(status *toolchainv1alpha1.Member) {
		if status.MemberStatus.Routes == nil {
			status.MemberStatus.Routes = &toolchainv1alpha1.Routes{}
		}
		status.MemberStatus.Routes.ConsoleURL = consoleURL
		status.MemberStatus.Routes.CheDashboardURL = cheURL
		status.MemberStatus.Routes.Conditions = []toolchainv1alpha1.Condition{condition}
	}
}

func WithMetric(key string, metric toolchainv1alpha1.Metric) ToolchainStatusOption {
	return func(status *toolchainv1alpha1.ToolchainStatus) {
		if status.Status.Metrics == nil {
			status.Status.Metrics = map[string]toolchainv1alpha1.Metric{}
		}
		status.Status.Metrics[key] = metric
	}
}

func WithEmptyMetrics() ToolchainStatusOption {
	return func(status *toolchainv1alpha1.ToolchainStatus) {
		if status.Status.Metrics == nil {
			status.Status.Metrics = map[string]toolchainv1alpha1.Metric{}
		}
		status.Status.Metrics = map[string]toolchainv1alpha1.Metric{
			toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey:        {},
			toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey: {},
		}
	}
}

func ToBeReady() toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:   toolchainv1alpha1.ConditionReady,
		Status: corev1.ConditionTrue,
	}
}

func ToBeNotReady() toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:   toolchainv1alpha1.ConditionReady,
		Status: corev1.ConditionFalse,
	}
}
