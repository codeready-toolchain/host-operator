package cluster

import (
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/kubefed/pkg/apis/core/v1beta1"
	"sync"
)

var clusterCache = kubeFedClusterClients{clusters: map[string]*FedCluster{}}

type kubeFedClusterClients struct {
	sync.Mutex
	clusters map[string]*FedCluster
}

// ClusterData stores cluster client and previous health check probe results of individual cluster.
type FedCluster struct {
	// clusterKubeClient is the kube client for the cluster.
	Client client.Client

	Name              string
	Type              Type
	OperatorNamespace string

	// clusterStatus is the cluster status as of last sampling.
	ClusterStatus *v1beta1.KubeFedClusterStatus
}

func (c kubeFedClusterClients) addFedCluster(cluster *FedCluster) {
	c.Lock()
	defer c.Unlock()
	c.clusters[cluster.Name] = cluster
}

func (c kubeFedClusterClients) deleteFedCluster(name string) {
	c.Lock()
	defer c.Unlock()
	delete(c.clusters, name)
}

func (c kubeFedClusterClients) getFedCluster(name string) (*FedCluster, bool) {
	c.Lock()
	defer c.Unlock()
	cluster, ok := c.clusters[name]
	return cluster, ok
}

func GetFedCluster(name string) (*FedCluster, bool) {
	return clusterCache.getFedCluster(name)
}

type Type string

const (
	Member Type = "member"
	Host   Type = "host"
)
