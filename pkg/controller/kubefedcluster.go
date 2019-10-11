package controller

import (
	"github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	"github.com/codeready-toolchain/toolchain-common/pkg/controller"
	"github.com/operator-framework/operator-sdk/pkg/k8sutil"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/kubefed/pkg/controller/kubefedcluster"
	"sigs.k8s.io/kubefed/pkg/controller/util"
)

func StartKubeFedClusterControllers(mgr manager.Manager, stopChan <-chan struct{}) error {
	if err := cluster.EnsureKubeFedClusterCrd(mgr.GetScheme(), mgr.GetClient()); err != nil {
		return err
	}

	if err := startHealthCheckController(mgr, stopChan); err != nil {
		return err
	}
	namespace, err := k8sutil.GetWatchNamespace()
	if err != nil {
		return err
	}
	if err := controller.StartCachingController(mgr, namespace, stopChan); err != nil {
		return err
	}
	return nil
}

func startHealthCheckController(mgr manager.Manager, stopChan <-chan struct{}) error {
	namespace, err := k8sutil.GetWatchNamespace()
	if err != nil {
		return err
	}
	controllerConfig := &util.ControllerConfig{
		KubeConfig:              mgr.GetConfig(),
		ClusterAvailableDelay:   util.DefaultClusterAvailableDelay,
		ClusterUnavailableDelay: util.DefaultClusterUnavailableDelay,
		KubeFedNamespaces: util.KubeFedNamespaces{
			KubeFedNamespace: namespace,
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
