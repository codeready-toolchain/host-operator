package cluster

import (
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Cluster struct {
	runtimecluster.Cluster
	OperatorNamespace string
}
