package cluster

import (
	commoncluster "github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Cluster struct {
	*commoncluster.Config
	Client client.Client
	Cache  cache.Cache
}
