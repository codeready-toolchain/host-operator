package registrationservice

import (
	templatev1 "github.com/openshift/api/template/v1"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/kubernetes/scheme"
)

// ResourceName is the name used for the registration service resource
const ResourceName = "registration-service"

// ProxyRouteName is the name used for the Proxy route resource
const ProxyRouteName = "api"

func GetDeploymentTemplate() (*templatev1.Template, error) {
	deployment, err := Asset("registration-service.yaml")
	if err != nil {
		return nil, err
	}
	decoder := serializer.NewCodecFactory(scheme.Scheme).UniversalDeserializer()
	deploymentTemplate := &templatev1.Template{}
	_, _, err = decoder.Decode([]byte(deployment), nil, deploymentTemplate)
	return deploymentTemplate, err
}
