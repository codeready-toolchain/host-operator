package test

import (
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

func NewMemberCluster(t *testing.T, name string, status v1.ConditionStatus) *cluster.CachedToolchainCluster {
	return NewMemberClusterWithClient(test.NewFakeClient(t), name, status)
}

func NewMemberClusterWithClient(cl client.Client, name string, status v1.ConditionStatus) *cluster.CachedToolchainCluster {
	toolchainCluster, _ := NewGetMemberCluster(true, status)(ClusterClient(name, cl))(name)
	return toolchainCluster
}

func NewGetMemberCluster(ok bool, status v1.ConditionStatus) GetMemberClusterFunc {
	if !ok {
		return func(clusters ...ClientForCluster) func(name string) (*cluster.CachedToolchainCluster, bool) {
			return func(name string) (*cluster.CachedToolchainCluster, bool) {
				return nil, false
			}
		}
	}
	return func(clusters ...ClientForCluster) func(name string) (*cluster.CachedToolchainCluster, bool) {
		mapping := map[string]client.Client{}
		for _, clusterClient := range clusters {
			n, cl := clusterClient()
			mapping[n] = cl
		}
		return func(name string) (*cluster.CachedToolchainCluster, bool) {
			cl, ok := mapping[name]
			if !ok {
				return nil, false
			}
			return &cluster.CachedToolchainCluster{
				Client:            cl,
				Name:              name,
				Type:              cluster.Member,
				OperatorNamespace: test.MemberOperatorNs,
				OwnerClusterName:  test.HostClusterName,
				ClusterStatus: &toolchainv1alpha1.ToolchainClusterStatus{
					Conditions: []toolchainv1alpha1.ToolchainClusterCondition{{
						Type:   toolchainv1alpha1.ToolchainClusterReady,
						Status: status,
					}},
				},
			}, true
		}
	}
}

type ClientForCluster func() (string, client.Client)

func ClusterClient(name string, cl client.Client) ClientForCluster {
	return func() (string, client.Client) {
		return name, cl
	}
}
