package test

import (
	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/configuration"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ToolchainStatusOption func(*toolchainv1alpha1.ToolchainStatus)

func NewToolchainStatus(options ...ToolchainStatusOption) *toolchainv1alpha1.ToolchainStatus {
	toolchainStatus := &toolchainv1alpha1.ToolchainStatus{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configuration.ToolchainStatusName,
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

func WithMember(name string, options ...MemberToolchainStatusOption) ToolchainStatusOption {
	return func(status *toolchainv1alpha1.ToolchainStatus) {
		member := toolchainv1alpha1.Member{
			ApiEndpoint: "http://api.devcluster.openshift.com",
			ClusterName: name,
		}
		for _, modify := range options {
			modify(&member)
		}
		status.Status.Members = append(status.Status.Members, member)
	}
}

type MemberToolchainStatusOption func(*toolchainv1alpha1.Member)

func WithUserAccountCount(count int) MemberToolchainStatusOption {
	return func(status *toolchainv1alpha1.Member) {
		status.UserAccountCount = count
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

func ToBeReady() toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:   toolchainv1alpha1.ConditionReady,
		Status: v1.ConditionTrue,
	}
}

func ToBeNotReady() toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:   toolchainv1alpha1.ConditionReady,
		Status: v1.ConditionFalse,
	}
}
