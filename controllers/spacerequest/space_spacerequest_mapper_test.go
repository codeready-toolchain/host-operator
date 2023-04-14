package spacerequest_test

import (
	"testing"

	"github.com/codeready-toolchain/api/api/v1alpha1"
	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/controllers/spacerequest"
	"github.com/codeready-toolchain/host-operator/test/space"
	spacerequesttest "github.com/codeready-toolchain/host-operator/test/spacerequest"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestMapToSpaceRequestByLabel(t *testing.T) {
	// given
	spaceRequest := spacerequesttest.NewSpaceRequest("mySpaceRequest", "jane")
	// following space has a spaceRequest associated
	subSpace := space.NewSpace("subSpace", space.WithLabel(v1alpha1.SpaceRequestLabelKey, spaceRequest.GetName()), space.WithLabel(v1alpha1.SpaceRequestNamespaceLabelKey, spaceRequest.GetNamespace()))
	// following space has no spaceRequest associated
	spacenosr := space.NewSpace("nospacerequest")

	t.Run("should return no space associated with a spacerequest", func(t *testing.T) {
		// when
		requests := spacerequest.MapSubSpaceToSpaceRequest()(spacenosr)

		// then
		require.Len(t, requests, 0)
	})

	t.Run("should return space associated with spacerequest", func(t *testing.T) {
		// when
		requests := spacerequest.MapSubSpaceToSpaceRequest()(subSpace)

		// then
		require.Len(t, requests, 1)
		assert.Contains(t, requests, newRequest(spaceRequest.Name, spaceRequest.Namespace))
	})
}

func TestMapSecretToSpaceRequest(t *testing.T) {
	// when
	secretData := map[string][]byte{
		"token": []byte("abc123"),
	}

	t.Run("test secret maps correctly", func(t *testing.T) {
		//given
		spaceRequest := spacerequesttest.NewSpaceRequest("mySpaceRequest", "jane")
		secret := test.CreateSecret("test-secret", "mynamespace-env", secretData)
		secret.Labels = map[string]string{
			toolchainv1alpha1.SpaceRequestLabelKey:                     spaceRequest.GetName(),
			toolchainv1alpha1.SpaceRequestNamespaceLabelKey:            spaceRequest.GetNamespace(),
			toolchainv1alpha1.SpaceRequestProvisionedNamespaceLabelKey: "mynamespace-env",
		}

		// when
		req := spacerequest.MapSecretToSpaceRequest()(secret)

		// then
		require.Len(t, req, 1)
		require.Equal(t, types.NamespacedName{
			Namespace: spaceRequest.Namespace,
			Name:      spaceRequest.Name,
		}, req[0].NamespacedName)
	})

	t.Run("a non-secret resource is not mapped", func(t *testing.T) {
		// given
		pod := &corev1.Pod{}

		// when
		req := spacerequest.MapSecretToSpaceRequest()(pod)

		// then
		require.Len(t, req, 0)
	})
}

func newRequest(name, namespace string) reconcile.Request {
	return reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: namespace,
			Name:      name,
		},
	}
}
