package test

import (
	"fmt"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"

	corev1 "k8s.io/api/core/v1"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type GetMemberClusterFunc func(clusters ...ClientForCluster) func(name string) (*cluster.CachedToolchainCluster, bool)

func NewGetMemberClusters(memberClusters ...*cluster.CachedToolchainCluster) cluster.GetClustersFunc {
	return func(conditions ...cluster.Condition) []*cluster.CachedToolchainCluster {
		clusters := map[string]*cluster.CachedToolchainCluster{}
		for _, memberCluster := range memberClusters {
			clusters[memberCluster.Name] = memberCluster
		}
		filteredClusters := cluster.Filter(clusters, conditions...)
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

func WithClusterRoleLabel(labelKey string) Modifier {
	return func(cluster *cluster.CachedToolchainCluster) {
		cluster.Config.Labels[labelKey] = "" // we don't care about the label value, only the key is used
	}
}

func NewMemberClusterWithTenantRole(t *testing.T, name string, status corev1.ConditionStatus) *cluster.CachedToolchainCluster {
	clusterRoleLabelModifier := WithClusterRoleLabel(cluster.RoleLabel(cluster.Tenant))
	return NewMemberClusterWithClient(test.NewFakeClient(t), name, status, clusterRoleLabelModifier)
}

func NewMemberClusterWithClient(cl runtimeclient.Client, name string, status corev1.ConditionStatus, modifiers ...Modifier) *cluster.CachedToolchainCluster {
	toolchainCluster, _ := NewGetMemberCluster(true, status, modifiers...)(ClusterClient(name, cl))(name)
	return toolchainCluster
}

// NewMemberClusterWithoutClusterRoles returns a cached toolchaincluster config that doesn't have the default cluster-roles which a member cluster should have
func NewMemberClusterWithoutClusterRoles(t *testing.T, name string, status corev1.ConditionStatus) *cluster.CachedToolchainCluster {
	return NewMemberClusterWithClient(test.NewFakeClient(t), name, status)
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
					OperatorNamespace: test.MemberOperatorNs,
					OwnerClusterName:  test.HostClusterName,
					Labels:            map[string]string{},
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
