package spacebindingrequest_test

import (
	"testing"

	"github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/controllers/spacebindingrequest"
	spacebindingtest "github.com/codeready-toolchain/host-operator/test/spacebinding"
	sbrtestcommon "github.com/codeready-toolchain/toolchain-common/pkg/test/spacebindingrequest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestMapSpaceBindingToSpaceBindingRequestByLabel(t *testing.T) {
	// given
	spaceBindingRequest := sbrtestcommon.NewSpaceBindingRequest("mySpaceBindingRequest", "jane")
	// following spaceBinding has a spaceBindingRequest associated
	sb := spacebindingtest.NewSpaceBinding("jane", "jane", "admin", "signupAdmin")
	sb.Labels[v1alpha1.SpaceBindingRequestLabelKey] = spaceBindingRequest.Name
	sb.Labels[v1alpha1.SpaceBindingRequestNamespaceLabelKey] = spaceBindingRequest.Namespace
	// following spaceBinding has no spaceBindingRequest associated
	sbnosbr := spacebindingtest.NewSpaceBinding("john", "john", "admin", "signupAdmin")

	t.Run("should return no spacebinding associated with a spacebindingrequest", func(t *testing.T) {
		// when
		requests := spacebindingrequest.MapSpaceBindingToSpaceBindingRequest()(sbnosbr)

		// then
		require.Empty(t, requests)
	})

	t.Run("should return spacebinding associated with spacebindingrequest", func(t *testing.T) {
		// when
		requests := spacebindingrequest.MapSpaceBindingToSpaceBindingRequest()(sb)

		// then
		require.Len(t, requests, 1)
		assert.Contains(t, requests, newRequest(spaceBindingRequest.Name, spaceBindingRequest.Namespace))
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
