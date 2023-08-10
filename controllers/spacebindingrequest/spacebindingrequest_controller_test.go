package spacebindingrequest_test

import (
	"context"
	"fmt"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/controllers/spacebindingrequest"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	"github.com/codeready-toolchain/host-operator/pkg/cluster"
	. "github.com/codeready-toolchain/host-operator/test"
	spacebindingrequesttest "github.com/codeready-toolchain/host-operator/test/spacebindingrequest"
	commoncluster "github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestCreateSpaceBindingRequest(t *testing.T) {
	// given
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	err := apis.AddToScheme(scheme.Scheme)
	require.NoError(t, err)
	sbr := spacebindingrequesttest.NewSpaceBindingRequest("jane", "jane-tenant",
		spacebindingrequesttest.WithMUR("jane"),
		spacebindingrequesttest.WithSpaceRole("admin"))
	t.Run("success", func(t *testing.T) {

		// given
		member1 := NewMemberClusterWithClient(test.NewFakeClient(t, sbr), "member-1", corev1.ConditionTrue)
		hostClient := test.NewFakeClient(t)
		ctrl := newReconciler(t, hostClient, member1)

		// when
		_, err = ctrl.Reconcile(context.TODO(), requestFor(sbr))

		// then
		require.NoError(t, err)
		// spacebindingrequest exists with expected cluster roles and finalizer
		spacebindingrequesttest.AssertThatSpaceBindingRequest(t, "jane-tenant", sbr.GetName(), member1.Client).
			HasSpecSpaceRole("admin").
			HasSpecMasterUserRecord("jane")
	})

	t.Run("failure", func(t *testing.T) {
		t.Run("unable to find SpaceBindingRequest", func(t *testing.T) {
			// given
			member1 := NewMemberClusterWithClient(test.NewFakeClient(t), "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t)
			ctrl := newReconciler(t, hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sbr))

			// then
			// space binding request should not be there,
			// but it should not return an error by design
			require.NoError(t, err)
		})
		t.Run("unable to get SpaceBindingRequest", func(t *testing.T) {
			member1Client := test.NewFakeClient(t)
			member1Client.MockGet = mockGetSpaceBindingRequestFail(member1Client)
			member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t)
			ctrl := newReconciler(t, hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sbr))

			// then
			// space binding request should not be there
			require.EqualError(t, err, "unable to get the current SpaceBindingRequest: mock error")
		})

	})
}

func newReconciler(t *testing.T, hostCl runtimeclient.Client, memberClusters ...*commoncluster.CachedToolchainCluster) *spacebindingrequest.Reconciler {
	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)

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
	return &spacebindingrequest.Reconciler{
		Client:         hostCl,
		Scheme:         s,
		Namespace:      test.HostOperatorNs,
		MemberClusters: clusters,
	}
}

func requestFor(s *toolchainv1alpha1.SpaceBindingRequest) reconcile.Request {
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
			Namespace: "john-tenant",
			Name:      "unknown",
		},
	}
}

func mockGetSpaceBindingRequestFail(cl runtimeclient.Client) func(ctx context.Context, key runtimeclient.ObjectKey, obj runtimeclient.Object, opts ...runtimeclient.GetOption) error {
	return func(ctx context.Context, key runtimeclient.ObjectKey, obj runtimeclient.Object, opts ...runtimeclient.GetOption) error {
		if _, ok := obj.(*toolchainv1alpha1.SpaceBindingRequest); ok {
			return fmt.Errorf("mock error")
		}
		return cl.Get(ctx, key, obj, opts...)
	}
}
