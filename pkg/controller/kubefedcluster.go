package controller

import (
	"context"
	"fmt"
	"github.com/codeready-toolchain/toolchain-common/pkg/controller"
	"github.com/operator-framework/operator-sdk/pkg/k8sutil"
	errors2 "github.com/pkg/errors"
	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/kubefed/pkg/controller/kubefedcluster"
	"sigs.k8s.io/kubefed/pkg/controller/util"
)

func StartKubeFedClusterControllers(mgr manager.Manager, stopChan <-chan struct{}) error {
	crd := &v1beta1.CustomResourceDefinition{}
	fmt.Println("checking and creating")
	err := mgr.GetClient().Get(context.TODO(), types.NamespacedName{Name: "kubefedclusters.core.kubefed.k8s.io"}, crd)
	if err != nil {
		if errors.IsNotFound(err) {
			codecFactory := serializer.NewCodecFactory(mgr.GetScheme())
			decoder := codecFactory.UniversalDeserializer()
			kubeFedCrd, err := decodeCrd(decoder, kubeFedClusterCrd)
			if err != nil {
				return err
			}
			err = mgr.GetClient().Create(context.TODO(), kubeFedCrd)
			if err != nil && !errors.IsAlreadyExists(err) {
				return err
			}
		}
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

func decodeCrd(decoder runtime.Decoder, tmplContent string) (*v1beta1.CustomResourceDefinition, error) {
	obj, _, err := decoder.Decode([]byte(tmplContent), nil, nil)
	if err != nil {
		return nil, errors2.Wrapf(err, "unable to decode template")
	}
	tmpl, ok := obj.(*v1beta1.CustomResourceDefinition)
	if !ok {
		return nil, fmt.Errorf("unable to convert object type %T to Template, must be a v1.Template", obj)
	}
	return tmpl, nil
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

var kubeFedClusterCrd = `
apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  creationTimestamp: null
  labels:
    controller-tools.k8s.io: "1.0"
  name: kubefedclusters.core.kubefed.k8s.io
spec:
  additionalPrinterColumns:
  - JSONPath: .status.conditions[?(@.type=='Ready')].status
    name: ready
    type: string
  - JSONPath: .metadata.creationTimestamp
    name: age
    type: date
  group: core.kubefed.k8s.io
  names:
    kind: KubeFedCluster
    plural: kubefedclusters
  scope: Namespaced
  subresources:
    status: {}
  validation:
    openAPIV3Schema:
      properties:
        apiVersion:
          description: 'APIVersion defines the versioned schema of this representation
            of an object. Servers should convert recognized schemas to the latest
            internal value, and may reject unrecognized values. More info: https://git.k8s.io/community/contributors/devel/api-conventions.md#resources'
          type: string
        kind:
          description: 'Kind is a string value representing the REST resource this
            object represents. Servers may infer this from the endpoint the client
            submits requests to. Cannot be updated. In CamelCase. More info: https://git.k8s.io/community/contributors/devel/api-conventions.md#types-kinds'
          type: string
        metadata:
          type: object
        spec:
          properties:
            apiEndpoint:
              description: The API endpoint of the member cluster. This can be a hostname,
                hostname:port, IP or IP:port.
              type: string
            caBundle:
              description: CABundle contains the certificate authority information.
              format: byte
              type: string
            secretRef:
              description: Name of the secret containing the token required to access
                the member cluster. The secret needs to exist in the same namespace
                as the control plane and should have a "token" key.
              properties:
                name:
                  description: Name of a secret within the enclosing namespace
                  type: string
              required:
              - name
              type: object
          required:
          - apiEndpoint
          - secretRef
          type: object
        status:
          properties:
            conditions:
              description: Conditions is an array of current cluster conditions.
              items:
                properties:
                  lastProbeTime:
                    description: Last time the condition was checked.
                    format: date-time
                    type: string
                  lastTransitionTime:
                    description: Last time the condition transit from one status to
                      another.
                    format: date-time
                    type: string
                  message:
                    description: Human readable message indicating details about last
                      transition.
                    type: string
                  reason:
                    description: (brief) reason for the condition's last transition.
                    type: string
                  status:
                    description: Status of the condition, one of True, False, Unknown.
                    type: string
                  type:
                    description: Type of cluster condition, Ready or Offline.
                    type: string
                required:
                - type
                - status
                - lastProbeTime
                type: object
              type: array
            region:
              description: Region is the name of the region in which all of the nodes
                in the cluster exist.  e.g. 'us-east1'.
              type: string
            zones:
              description: Zones are the names of availability zones in which the
                nodes of the cluster exist, e.g. 'us-east1-a'.
              items:
                type: string
              type: array
          required:
          - conditions
          type: object
  version: v1beta1
status:
  acceptedNames:
    kind: ""
    plural: ""
  conditions: []
  storedVersions: []
`
