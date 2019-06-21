package controller

import (
	"fmt"
	"github.com/codeready-toolchain/host-operator/pkg/cluster"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"
	"os"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/kubefed/pkg/apis/core/v1beta1"
	"sigs.k8s.io/kubefed/pkg/controller/kubefedcluster"
	"sigs.k8s.io/kubefed/pkg/controller/util"
)

const varOperatorNamespace = "OPERATOR_NAMESPACE"

func StartKubeFedClusterControllers(mgr manager.Manager, stopChan <-chan struct{}) error {
	if err := startHealthCheckController(mgr, stopChan); err != nil {
		return err
	}
	if err := startCachingController(mgr, stopChan); err != nil {
		return err
	}
	return nil
}

func startCachingController(mgr manager.Manager, stopChan <-chan struct{}) error {
	cntrlName := "controller_kubefedcluster_with_cache"
	clusterCacheService := cluster.KubeFedClusterService{
		LocalConfig: mgr.GetConfig(),
		Log:         logf.Log.WithName(cntrlName),
	}

	_, clusterController, err := util.NewGenericInformerWithEventHandler(
		mgr.GetConfig(),
		"",
		&v1beta1.KubeFedCluster{},
		util.NoResyncPeriod,
		&cache.ResourceEventHandlerFuncs{
			DeleteFunc: clusterCacheService.DeleteKubeFedCluster,
			AddFunc:    clusterCacheService.AddKubeFedCluster,
			UpdateFunc: clusterCacheService.UpdateKubeFedCluster,
		},
	)
	if err != nil {
		return err
	}
	logf.Log.Info("Starting Controller", "controller", cntrlName)
	go clusterController.Run(stopChan)
	return nil
}

func startHealthCheckController(mgr manager.Manager, stopChan <-chan struct{}) error {
	ns, found := os.LookupEnv(varOperatorNamespace)
	if !found {
		return fmt.Errorf("%s must be set", varOperatorNamespace)
	}
	controllerConfig := &util.ControllerConfig{
		KubeConfig:              mgr.GetConfig(),
		ClusterAvailableDelay:   util.DefaultClusterAvailableDelay,
		ClusterUnavailableDelay: util.DefaultClusterUnavailableDelay,
		KubeFedNamespaces: util.KubeFedNamespaces{
			KubeFedNamespace: ns,
		},
	}
	clusterHealthCheckConfig := &util.ClusterHealthCheckConfig{
		PeriodSeconds:    util.DefaultClusterHealthCheckPeriod,
		TimeoutSeconds:   util.DefaultClusterHealthCheckTimeout,
		FailureThreshold: util.DefaultClusterHealthCheckFailureThreshold,
		SuccessThreshold: util.DefaultClusterHealthCheckSuccessThreshold,
	}
	klog.InitFlags(nil)
	return kubefedcluster.StartClusterController(controllerConfig, clusterHealthCheckConfig, stopChan)
}