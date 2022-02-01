package spacecompletion_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/controllers/spacecompletion"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	"github.com/codeready-toolchain/host-operator/pkg/counter"
	. "github.com/codeready-toolchain/host-operator/test"
	spacetest "github.com/codeready-toolchain/host-operator/test/space"
	"github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	"github.com/codeready-toolchain/toolchain-common/pkg/configuration"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestCreateSpace(t *testing.T) {
	member1 := NewMemberCluster(t, "member1", corev1.ConditionTrue)
	getMemberClusters := NewGetMemberClusters(member1)

	t.Run("without any field set - then it sets only tierName", func(t *testing.T) {
		// given
		space := spacetest.NewSpace("without-fields",
			spacetest.WithTierName(""))
		r, req, cl := prepareReconcile(t, space, getMemberClusters)

		// when
		_, err := r.Reconcile(context.TODO(), req)

		// then
		require.NoError(t, err)
		spacetest.AssertThatSpace(t, test.HostOperatorNs, space.Name, cl).
			HasTier("base").
			HasSpecTargetCluster("")
	})

	t.Run("with tierName but without targetCluster - only targetCluster should be set", func(t *testing.T) {
		// given
		space := spacetest.NewSpace("without-targetCluster",
			spacetest.WithTierName("advanced"))
		r, req, cl := prepareReconcile(t, space, getMemberClusters)

		// when
		_, err := r.Reconcile(context.TODO(), req)

		// then
		require.NoError(t, err)
		spacetest.AssertThatSpace(t, test.HostOperatorNs, space.Name, cl).
			HasTier("advanced").
			HasSpecTargetCluster("member1")
	})

	t.Run("with targetCluster but without tierName - only tierName should be set", func(t *testing.T) {
		// given
		space := spacetest.NewSpace("without-tierName",
			spacetest.WithTierName(""),
			spacetest.WithSpecTargetCluster("member2"))
		r, req, cl := prepareReconcile(t, space, getMemberClusters)

		// when
		_, err := r.Reconcile(context.TODO(), req)

		// then
		require.NoError(t, err)
		spacetest.AssertThatSpace(t, test.HostOperatorNs, space.Name, cl).
			HasTier("base").
			HasSpecTargetCluster("member2")
	})

	t.Run("no updates expected", func(t *testing.T) {
		t.Run("with both fields set", func(t *testing.T) {
			// given
			space := spacetest.NewSpace("with-fields",
				spacetest.WithTierName("advanced"),
				spacetest.WithSpecTargetCluster("member2"))
			r, req, cl := prepareReconcile(t, space, getMemberClusters)

			// when
			_, err := r.Reconcile(context.TODO(), req)

			// then
			require.NoError(t, err)
			spacetest.AssertThatSpace(t, test.HostOperatorNs, space.Name, cl).
				HasTier("advanced").
				HasSpecTargetCluster("member2")
		})

		t.Run("when is being deleted, then nothing should be set", func(t *testing.T) {
			// given
			space := spacetest.NewSpace("without-fields",
				spacetest.WithTierName(""),
				spacetest.WithDeletionTimestamp())
			r, req, cl := prepareReconcile(t, space, getMemberClusters)

			// when
			_, err := r.Reconcile(context.TODO(), req)

			// then
			require.NoError(t, err)
			spacetest.AssertThatSpace(t, test.HostOperatorNs, space.Name, cl).
				HasTier("").
				HasSpecTargetCluster("")
		})

		t.Run("when no member cluster available and when tierName is set", func(t *testing.T) {
			// given
			space := spacetest.NewSpace("without-members",
				spacetest.WithTierName("advanced"))
			r, req, cl := prepareReconcile(t, space, NewGetMemberClusters())

			// when
			_, err := r.Reconcile(context.TODO(), req)

			// then
			require.NoError(t, err)
			spacetest.AssertThatSpace(t, test.HostOperatorNs, space.Name, cl).
				HasTier("advanced").
				HasSpecTargetCluster("")
		})

		t.Run("when Get ToolchainConfig fails and no field is set", func(t *testing.T) {
			// given
			space := spacetest.NewSpace("oddity",
				spacetest.WithTierName(""))
			r, req, cl := prepareReconcile(t, space, NewGetMemberClusters())
			cl.MockGet = func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
				if key.Name == "config" {
					return fmt.Errorf("some error")
				}
				return cl.Client.Get(ctx, key, obj)
			}

			// when
			_, err := r.Reconcile(context.TODO(), req)

			// then
			require.Error(t, err)
			spacetest.AssertThatSpace(t, test.HostOperatorNs, space.Name, cl).
				HasTier("").
				HasSpecTargetCluster("")
		})

		t.Run("when Get ToolchainConfig fails and only targetCluster is missing", func(t *testing.T) {
			// given
			space := spacetest.NewSpace("oddity",
				spacetest.WithTierName("advanced"))
			r, req, cl := prepareReconcile(t, space, NewGetMemberClusters())
			cl.MockGet = func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
				if key.Name == "config" {
					return fmt.Errorf("some error")
				}
				return cl.Client.Get(ctx, key, obj)
			}

			// when
			_, err := r.Reconcile(context.TODO(), req)

			// then
			require.Error(t, err)
			spacetest.AssertThatSpace(t, test.HostOperatorNs, space.Name, cl).
				HasTier("advanced").
				HasSpecTargetCluster("")
		})
	})
}

func prepareReconcile(t *testing.T, space *toolchainv1alpha1.Space, getMemberClusters cluster.GetMemberClustersFunc) (*spacecompletion.Reconciler, reconcile.Request, *test.FakeClient) {
	require.NoError(t, os.Setenv("WATCH_NAMESPACE", test.HostOperatorNs))
	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)

	toolchainStatus := NewToolchainStatus(
		WithMember("member1", WithNodeRoleUsage("worker", 68), WithNodeRoleUsage("master", 65)))
	t.Cleanup(counter.Reset)
	InitializeCounters(t, toolchainStatus)

	conf := configuration.NewToolchainConfigObjWithReset(t)

	fakeClient := test.NewFakeClient(t, toolchainStatus, space, conf)

	r := &spacecompletion.Reconciler{
		Client:            fakeClient,
		Namespace:         test.HostOperatorNs,
		GetMemberClusters: getMemberClusters,
	}
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: test.HostOperatorNs,
			Name:      space.Name,
		},
	}
	return r, req, fakeClient
}
