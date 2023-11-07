package spacebindingrequestmigration_test

import (
	"testing"

	"github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/controllers/spacebindingrequestmigration"
	sb "github.com/codeready-toolchain/host-operator/test/spacebinding"
	spacebindingrequesttest "github.com/codeready-toolchain/host-operator/test/spacebindingrequest"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestMapSpaceBindingRequestToSpaceBinding(t *testing.T) {
	// given
	restore := test.SetEnvVarAndRestore(t, "WATCH_NAMESPACE", test.HostOperatorNs)
	defer restore()
	spaceBindingRequest := spacebindingrequesttest.NewSpaceBindingRequest("mySpaceBindingRequest", "jane")
	// following spaceBinding has a spaceBindingRequest associated
	spaceBinding := sb.NewSpaceBinding("jane", "jane", "admin", "signupAdmin")
	spaceBinding.Labels[v1alpha1.SpaceBindingRequestLabelKey] = spaceBindingRequest.Name
	spaceBinding.Labels[v1alpha1.SpaceBindingRequestNamespaceLabelKey] = spaceBindingRequest.Namespace

	cl := test.NewFakeClient(t, spaceBinding)

	t.Run("should return SpaceBinding requests for SpaceBindingRequest", func(t *testing.T) {
		// when
		requests := spacebindingrequestmigration.MapSpaceBindingRequestToSpaceBinding(cl)(spaceBindingRequest)

		// then
		require.Len(t, requests, 1)
		assert.Contains(t, requests, newRequest(spaceBinding.Name))
	})

}

func newRequest(name string) reconcile.Request {
	return reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: test.HostOperatorNs,
			Name:      name,
		},
	}
}
