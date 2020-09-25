package test

import (
	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/configuration"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ToolchainStatusOption func(*toolchainv1alpha1.ToolchainStatus)

func WithMember(name string, options ...MemberToolchainStatusOption) ToolchainStatusOption {
	return func(status *toolchainv1alpha1.ToolchainStatus) {
		member := toolchainv1alpha1.Member{
			ClusterName: name,
		}
		for _, modify := range options {
			modify(&member)
		}
		status.Status.Members = append(status.Status.Members, member)
	}
}

type MemberToolchainStatusOption func(*toolchainv1alpha1.Member)

func WithNodeRoleUsage(role string, usage int) MemberToolchainStatusOption {
	return func(status *toolchainv1alpha1.Member) {
		if status.MemberStatus.ResourceUsage.MemoryUsagePerNodeRole == nil {
			status.MemberStatus.ResourceUsage.MemoryUsagePerNodeRole = map[string]int{}
		}
		status.MemberStatus.ResourceUsage.MemoryUsagePerNodeRole[role] = usage
	}
}

func NewToolchainStatus(options ...func(*toolchainv1alpha1.ToolchainStatus)) *toolchainv1alpha1.ToolchainStatus {
	toolchainStatus := &toolchainv1alpha1.ToolchainStatus{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configuration.DefaultToolchainStatusName,
			Namespace: test.HostOperatorNs,
		},
	}
	for _, modify := range options {
		modify(toolchainStatus)
	}
	return toolchainStatus
}
