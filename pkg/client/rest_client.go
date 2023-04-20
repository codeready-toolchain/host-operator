package client

import (
	"context"
	"fmt"

	authv1 "k8s.io/api/authentication/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/utils/pointer"
)

// CreateTokenRequest creates a TokenRequest for a service account using given expiration in seconds.
func CreateTokenRequest(restClient *rest.RESTClient, namespacedName types.NamespacedName, expirationInSeconds int) (string, error) {
	tokenRequest := &authv1.TokenRequest{
		Spec: authv1.TokenRequestSpec{
			ExpirationSeconds: pointer.Int64(int64(expirationInSeconds)),
		},
	}
	result := &authv1.TokenRequest{}
	if err := restClient.Post().
		AbsPath(fmt.Sprintf("api/v1/namespaces/%s/serviceaccounts/%s/token", namespacedName.Namespace, namespacedName.Name)).
		Body(tokenRequest).
		Do(context.TODO()).
		Into(result); err != nil {
		return "", err
	}
	return result.Status.Token, nil
}
