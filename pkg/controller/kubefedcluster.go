package controller

import (
	"fmt"
	"github.com/codeready-toolchain/toolchain/pkg/controller"
	"k8s.io/klog"
	"os"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/kubefed/pkg/controller/kubefedcluster"
	"sigs.k8s.io/kubefed/pkg/controller/util"
)

const varOperatorNamespace = "OPERATOR_NAMESPACE"

func StartKubeFedClusterControllers(mgr manager.Manager, stopChan <-chan struct{}) error {
	if err := startHealthCheckController(mgr, stopChan); err != nil {
		return err
	}
	if err := controller.StartCachingController(mgr, stopChan); err != nil {
		return err
	}
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
