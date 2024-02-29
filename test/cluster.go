package test

import (
	"fmt"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/strings/slices"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type GetMemberClusterFunc func(clusters ...ClientForCluster) func(name string) (*cluster.CachedToolchainCluster, bool)

func NewGetMemberClusters(memberClusters ...*cluster.CachedToolchainCluster) cluster.GetMemberClustersFunc {
	return func(conditions ...cluster.Condition) []*cluster.CachedToolchainCluster {
		clusters := map[string]*cluster.CachedToolchainCluster{}
		for _, memberCluster := range memberClusters {
			clusters[memberCluster.Name] = memberCluster
		}
		filteredClusters := cluster.Filter(cluster.Member, clusters, conditions...)
		var filteredClustersInOrder []*cluster.CachedToolchainCluster
		for _, givenCluster := range memberClusters {
			for _, filteredCluster := range filteredClusters {
				if givenCluster.Name == filteredCluster.Name {
					filteredClustersInOrder = append(filteredClustersInOrder, givenCluster)
					break
				}
			}
		}
		return filteredClustersInOrder
	}
}

type Modifier func(toolchainCluster *cluster.CachedToolchainCluster)

// Deprecated: Use WithPlacementRoles instead
func WithClusterRoleLabel(labelKey string) Modifier {
	return func(cluster *cluster.CachedToolchainCluster) {
		// This is the legacy way of adding the placement roles. Let's not remove it for now...
		cluster.Config.Labels[labelKey] = "" // we don't care about the label value, only the key is used
	}
}

func WithProvisioningDisabled() Modifier {
	return func(cluster *cluster.CachedToolchainCluster) {
		cluster.Provisioning.Enabled = false
	}
}

func WithMaxMemoryPercent(maxMemoryPercent uint) Modifier {
	return func(cluster *cluster.CachedToolchainCluster) {
		cluster.Provisioning.CapacityThresholds.MaxMemoryUtilizationPercent = maxMemoryPercent
	}
}

func WithMaxNumberOfSpaces(maxSpaces uint) Modifier {
	return func(cluster *cluster.CachedToolchainCluster) {
		cluster.Provisioning.CapacityThresholds.MaxNumberOfSpaces = maxSpaces
	}
}

func WithPlacementRoles(placementRoles ...string) Modifier {
	return func(cluster *cluster.CachedToolchainCluster) {
		for _, pr := range placementRoles {
			if !slices.Contains(cluster.Provisioning.PlacementRoles, pr) {
				cluster.Provisioning.PlacementRoles = append(cluster.Provisioning.PlacementRoles, pr)
			}
		}
	}
}

func NewMemberClusterWithTenantRole(t *testing.T, name string, status corev1.ConditionStatus, modifiers ...Modifier) *cluster.CachedToolchainCluster {
	modifiers = append(modifiers, WithClusterRoleLabel(cluster.RoleLabel(cluster.Tenant)))
	return NewMemberClusterWithClient(test.NewFakeClient(t), name, status, modifiers...)
}

func NewMemberClusterWithClient(cl runtimeclient.Client, name string, status corev1.ConditionStatus, modifiers ...Modifier) *cluster.CachedToolchainCluster {
	toolchainCluster, _ := NewGetMemberCluster(true, status, modifiers...)(ClusterClient(name, cl))(name)
	return toolchainCluster
}

// NewMemberClusterWithoutClusterRoles returns a cached toolchaincluster config that doesn't have the default cluster-roles which a member cluster should have
func NewMemberClusterWithoutClusterRoles(t *testing.T, name string, status corev1.ConditionStatus, modifiers ...Modifier) *cluster.CachedToolchainCluster {
	return NewMemberClusterWithClient(test.NewFakeClient(t), name, status, modifiers...)
}

func NewGetMemberCluster(ok bool, status corev1.ConditionStatus, modifiers ...Modifier) GetMemberClusterFunc {
	if !ok {
		return func(clusters ...ClientForCluster) func(name string) (*cluster.CachedToolchainCluster, bool) {
			return func(name string) (*cluster.CachedToolchainCluster, bool) {
				return nil, false
			}
		}
	}
	return func(clusters ...ClientForCluster) func(name string) (*cluster.CachedToolchainCluster, bool) {
		mapping := map[string]runtimeclient.Client{}
		for _, clusterClient := range clusters {
			n, cl := clusterClient()
			mapping[n] = cl
		}
		return func(name string) (*cluster.CachedToolchainCluster, bool) {
			cl, ok := mapping[name]
			if !ok {
				return nil, false
			}
			cachedCluster := &cluster.CachedToolchainCluster{
				Config: &cluster.Config{
					Name:              name,
					APIEndpoint:       fmt.Sprintf("https://api.%s:6433", name),
					Type:              cluster.Member,
					OperatorNamespace: test.MemberOperatorNs,
					OwnerClusterName:  test.HostClusterName,
					Labels:            map[string]string{},
					Provisioning: cluster.ProvisioningConfig{
						Enabled: true,
					},
				},
				Client: cl,
				ClusterStatus: &toolchainv1alpha1.ToolchainClusterStatus{
					Conditions: []toolchainv1alpha1.ToolchainClusterCondition{{
						Type:   toolchainv1alpha1.ToolchainClusterReady,
						Status: status,
					}},
				},
			}

			for _, modify := range modifiers {
				modify(cachedCluster)
			}

			return cachedCluster, true
		}
	}
}

type ClientForCluster func() (string, runtimeclient.Client)

func ClusterClient(name string, cl runtimeclient.Client) ClientForCluster {
	return func() (string, runtimeclient.Client) {
		return name, cl
	}
}
