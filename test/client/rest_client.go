package client

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/h2non/gock.v1"
	authv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
)

// SetupGockForServiceAccounts registers the /namespaces/<namespace>/serviceaccounts/<serviceaccount>/token endpoint with gock.
// so that token request are intercepted by gock.
func SetupGockForServiceAccounts(t *testing.T, apiEndpoint string, sas ...*corev1.ServiceAccount) {
	for _, sa := range sas {

		expectedToken := "token-secret-for-" + sa.Name
		resultTokenRequest := &authv1.TokenRequest{
			Status: authv1.TokenRequestStatus{
				Token: expectedToken,
			},
		}
		resultTokenRequestStr, err := json.Marshal(resultTokenRequest)
		require.NoError(t, err)
		path := fmt.Sprintf("api/v1/namespaces/%s/serviceaccounts/%s/token", sa.Namespace, sa.Name)
		t.Logf("mocking access to POST %s/%s", apiEndpoint, path)
		gock.New(apiEndpoint).
			Post(path).
			Persist().
			Reply(200).
			BodyString(string(resultTokenRequestStr))
	}
}

// NewRESTClient returns a new kube api rest client.
func NewRESTClient(token, apiEndpoint string) (*rest.RESTClient, error) {
	config := &rest.Config{
		BearerToken: token,
		Host:        apiEndpoint,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true, // nolint: gosec
			},
		},
		// These fields need to be set when using the REST client
		ContentConfig: rest.ContentConfig{
			GroupVersion:         &authv1.SchemeGroupVersion,
			NegotiatedSerializer: scheme.Codecs,
		},
	}
	return rest.RESTClientFor(config)
}
