package cluster

import (
	"fmt"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/kubefed/pkg/apis/core/v1beta1"
	genericclient "sigs.k8s.io/kubefed/pkg/client/generic"
	"sigs.k8s.io/kubefed/pkg/controller/util"
)

const (
	labelType      = "type"
	labelNamespace = "namespace"

	defaultHostOperatorNamespace   = "toolchain-host-operator"
	defaultMemberOperatorNamespace = "toolchain-member-operator"
)

type KubeFedClusterService struct {
	LocalConfig *rest.Config
	Log         logr.Logger
}

func (s KubeFedClusterService) AddKubeFedCluster(obj interface{}) {
	cluster, ok := obj.(*v1beta1.KubeFedCluster)
	if !ok {
		err := fmt.Errorf("incorrect type of KubeFedCluster: %+v", obj)
		s.Log.Error(err, "cluster not added")
		return
	}
	log := s.enrichLogger(cluster)
	log.Info("observed a new cluster")

	err := s.addKubeFedCluster(cluster)
	if err != nil {
		log.Error(err, "the new cluster was not added")
	}
}

func (s KubeFedClusterService) addKubeFedCluster(fedCluster *v1beta1.KubeFedCluster) error {
	localClient, err := genericclient.New(s.LocalConfig)
	if err != nil {
		return errors.Wrap(err, "cannot create local client")
	}
	// create the restclient of fedCluster
	clusterConfig, err := util.BuildClusterConfig(fedCluster, localClient, fedCluster.Namespace)
	if err != nil {
		return errors.Wrap(err, "cannot create KubeFedCluster config")
	}
	cl, err := client.New(clusterConfig, client.Options{})
	if err != nil {
		return errors.Wrap(err, "cannot create KubeFedCluster client")
	}

	cluster := &FedCluster{
		Name:              fedCluster.Name,
		Client:            cl,
		ClusterStatus:     &fedCluster.Status,
		Type:              Type(fedCluster.Labels[labelType]),
		OperatorNamespace: fedCluster.Labels[labelNamespace],
	}
	if cluster.OperatorNamespace == "" {
		if cluster.Type == Host {
			cluster.OperatorNamespace = defaultHostOperatorNamespace
		} else {
			cluster.OperatorNamespace = defaultMemberOperatorNamespace
		}
	}

	clusterCache.addFedCluster(cluster)
	return nil
}

func (s KubeFedClusterService) DeleteKubeFedCluster(obj interface{}) {
	cluster, ok := obj.(*v1beta1.KubeFedCluster)
	if !ok {
		err := fmt.Errorf("incorrect type of KubeFedCluster: %+v", obj)
		s.Log.Error(err, "cluster not deleted")
		return
	}
	log := s.enrichLogger(cluster)
	log.Info("observed a deleted cluster")
	clusterCache.deleteFedCluster(cluster.Name)
}

func (s KubeFedClusterService) UpdateKubeFedCluster(_, newObj interface{}) {
	newCluster, ok := newObj.(*v1beta1.KubeFedCluster)
	if !ok {
		err := fmt.Errorf("incorrect type of new KubeFedCluster: %+v", newObj)
		s.Log.Error(err, "cluster not updated")
	}
	log := s.enrichLogger(newCluster)
	log.Info("observed an updated cluster")

	err := s.addKubeFedCluster(newCluster)
	if err != nil {
		log.Error(err, "the cluster was not updated")
	}
}

func (s KubeFedClusterService) enrichLogger(cluster *v1beta1.KubeFedCluster) logr.Logger {
	return s.Log.
		WithValues("Request.Namespace", cluster.Namespace, "Request.Name", cluster.Name)
}
