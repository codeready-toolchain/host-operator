package spacerequest_test

import (
	"context"
	"os"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/controllers/spacerequest"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	"github.com/codeready-toolchain/host-operator/pkg/cluster"
	. "github.com/codeready-toolchain/host-operator/test"
	spacerequesttest "github.com/codeready-toolchain/host-operator/test/spacerequest"
	commoncluster "github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestCreateSpaceRequest(t *testing.T) {

	// given
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	err := apis.AddToScheme(scheme.Scheme)
	require.NoError(t, err)
	t.Run("success", func(t *testing.T) {
		// given
		sr := spacerequesttest.NewSpaceRequest("jane", "jane-tenant", spacerequesttest.WithTierName("basic"), spacerequesttest.WithTargetClusterRoles([]string{commoncluster.RoleLabel(commoncluster.Tenant)}))
		member1 := NewMemberClusterWithTenantRole(t, "member-1", corev1.ConditionTrue)
		member2 := NewMemberClusterWithTenantRole(t, "member-2", corev1.ConditionTrue)
		ctrl := newReconciler(member1, member2)

		// when
		res, err := ctrl.Reconcile(context.TODO(), requestFor(sr))

		// then
		require.NoError(t, err)
		assert.False(t, res.Requeue)
	})

	t.Run("failure", func(t *testing.T) {

	})
}

func TestDeleteSpaceRequest(t *testing.T) {
	// TODO implement deletion testcase
}

func TestUpdateSpaceRequestTier(t *testing.T) {
	// TODO implement spacerequest tier field update
}

func TestUpdateSpaceRequestTargetClusterRoles(t *testing.T) {
	// TODO implement
}

func newReconciler(memberClusters ...*commoncluster.CachedToolchainCluster) *spacerequest.Reconciler {
	os.Setenv("WATCH_NAMESPACE", test.HostOperatorNs)
	clusters := map[string]cluster.Cluster{}
	for _, c := range memberClusters {
		clusters[c.Name] = cluster.Cluster{
			Config: &commoncluster.Config{
				Type:              commoncluster.Member,
				OperatorNamespace: c.OperatorNamespace,
				OwnerClusterName:  test.MemberClusterName,
			},
			Client: c.Client,
		}
	}
	return &spacerequest.Reconciler{
		Namespace:      test.HostOperatorNs,
		MemberClusters: clusters,
	}
}

func requestFor(s *toolchainv1alpha1.SpaceRequest) reconcile.Request {
	if s != nil {
		return reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: s.Namespace,
				Name:      s.Name,
			},
		}
	}
	return reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: test.HostOperatorNs,
			Name:      "unknown",
		},
	}
}
