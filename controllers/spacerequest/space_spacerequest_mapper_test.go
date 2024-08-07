package spacerequest_test

import (
	"context"
	"testing"

	"github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/controllers/spacerequest"
	spacerequesttest "github.com/codeready-toolchain/host-operator/test/spacerequest"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	spacetest "github.com/codeready-toolchain/toolchain-common/pkg/test/space"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestMapToSpaceRequestByLabel(t *testing.T) {
	// given
	spaceRequest := spacerequesttest.NewSpaceRequest("mySpaceRequest", "jane")
	// following space has a spaceRequest associated
	subSpace := spacetest.NewSpace(test.HostOperatorNs, "subSpace", spacetest.WithLabel(v1alpha1.SpaceRequestLabelKey, spaceRequest.GetName()), spacetest.WithLabel(v1alpha1.SpaceRequestNamespaceLabelKey, spaceRequest.GetNamespace()))
	// following space has no spaceRequest associated
	spacenosr := spacetest.NewSpace(test.HostOperatorNs, "nospacerequest")

	t.Run("should return no space associated with a spacerequest", func(t *testing.T) {
		// when
		requests := spacerequest.MapSubSpaceToSpaceRequest()(context.TODO(), spacenosr)

		// then
		require.Len(t, requests, 0)
	})

	t.Run("should return space associated with spacerequest", func(t *testing.T) {
		// when
		requests := spacerequest.MapSubSpaceToSpaceRequest()(context.TODO(), subSpace)

		// then
		require.Len(t, requests, 1)
		assert.Contains(t, requests, newRequest(spaceRequest.Name, spaceRequest.Namespace))
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
