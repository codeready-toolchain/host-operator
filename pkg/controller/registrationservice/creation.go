package registrationservice

import (
	"github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/configuration"
	"github.com/codeready-toolchain/toolchain-common/pkg/template"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func CreateOrUpdateResources(client client.Client, s *runtime.Scheme, namespace string, confg *configuration.Registry) error {
	regService := &v1alpha1.RegistrationService{
		ObjectMeta: v1.ObjectMeta{
			Namespace: namespace,
			Name:      "registration-service",
		},
		Spec: v1alpha1.RegistrationServiceSpec{
			Image:       confg.GetRegServiceImage(),
			Environment: confg.GetRegServiceEnvironment(),
			AuthClient: v1alpha1.AuthClient{
				Config:        confg.GetAuthClientConfigAuthRaw(),
				PublicKeysUrl: confg.GetAuthClientPublicKeysURL(),
				LibraryUrl:    confg.GetAuthClientLibraryURL(),
			},
		}}
	processor := template.NewProcessor(client, s)
	_, err := processor.ApplySingle(regService, false, nil)
	return err
}
