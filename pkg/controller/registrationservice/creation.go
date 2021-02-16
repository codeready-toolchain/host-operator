package registrationservice

import (
	"strings"

	"github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/configuration"
	commonclient "github.com/codeready-toolchain/toolchain-common/pkg/client"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ResourceName is the name used for the registration service resource
const ResourceName = "registration-service"

func CreateOrUpdateResources(client client.Client, s *runtime.Scheme, namespace string, confg *configuration.Config) error {
	envs := map[string]string{}
	for key, value := range confg.GetAllRegistrationServiceParameters() {
		envs[strings.TrimPrefix(key, configuration.RegServiceEnvPrefix+"_")] = value
	}

	regService := &v1alpha1.RegistrationService{
		ObjectMeta: v1.ObjectMeta{
			Namespace: namespace,
			Name:      ResourceName,
		},
		Spec: v1alpha1.RegistrationServiceSpec{
			EnvironmentVariables: envs,
		}}
	commoncl := commonclient.NewApplyClient(client, s)
	_, err := commoncl.ApplyObject(regService, commonclient.ForceUpdate(false))
	return err
}
