package registrationservice

import (
	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	commonclient "github.com/codeready-toolchain/toolchain-common/pkg/client"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ResourceName is the name used for the registration service resource
const ResourceName = "registration-service"

func CreateOrUpdateResources(client client.Client, s *runtime.Scheme, namespace string) error {
	regService := &toolchainv1alpha1.RegistrationService{
		ObjectMeta: v1.ObjectMeta{
			Namespace: namespace,
			Name:      ResourceName,
		},
		Spec: toolchainv1alpha1.RegistrationServiceSpec{}}
	commonclient := commonclient.NewApplyClient(client, s)
	_, err := commonclient.ApplyObject(regService)
	return err
}
