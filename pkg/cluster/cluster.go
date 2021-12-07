package cluster

import (
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Cluster struct {
	OperatorNamespace string
	Client            client.Client
	Cache             cache.Cache
}
