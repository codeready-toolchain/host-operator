package spacecompletion_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/controllers/spacecompletion"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	"github.com/codeready-toolchain/host-operator/pkg/capacity"
	"github.com/codeready-toolchain/host-operator/pkg/counter"
	. "github.com/codeready-toolchain/host-operator/test"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	spacetest "github.com/codeready-toolchain/toolchain-common/pkg/test/space"
	"github.com/codeready-toolchain/toolchain-common/pkg/test/spaceprovisionerconfig"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestCreateSpace(t *testing.T) {
	spc := spaceprovisionerconfig.NewEnabledValidSpaceProvisionerConfigWithTenantRole("member1Spc", test.HostOperatorNs, "member1")

	t.Run("without any field set - then it sets only tierName", func(t *testing.T) {
		// given
		space := spacetest.NewSpace(test.HostOperatorNs, "without-fields",
			spacetest.WithTierName(""))
		r, req, cl := prepareReconcile(t, space, spc)

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
		space := spacetest.NewSpace(test.HostOperatorNs, "without-targetCluster",
			spacetest.WithTierName("advanced"))
		r, req, cl := prepareReconcile(t, space, spc)

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
		space := spacetest.NewSpace(test.HostOperatorNs, "without-tierName",
			spacetest.WithTierName(""),
			spacetest.WithSpecTargetCluster("member2"))
		r, req, cl := prepareReconcile(t, space, spc)

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
			space := spacetest.NewSpace(test.HostOperatorNs, "with-fields",
				spacetest.WithTierName("advanced"),
				spacetest.WithSpecTargetCluster("member2"))
			r, req, cl := prepareReconcile(t, space, spc)

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
			space := spacetest.NewSpace(test.HostOperatorNs, "without-fields",
				spacetest.WithTierName(""),
				spacetest.WithDeletionTimestamp())
			r, req, cl := prepareReconcile(t, space, spc)

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
			space := spacetest.NewSpace(test.HostOperatorNs, "without-members",
				spacetest.WithTierName("advanced"))
			r, req, cl := prepareReconcile(t, space, nil)

			// when
			_, err := r.Reconcile(context.TODO(), req)

			// then
			require.NoError(t, err)
			spacetest.AssertThatSpace(t, test.HostOperatorNs, space.Name, cl).
				HasTier("advanced").
				HasSpecTargetCluster("")
		})

		t.Run("when the space is not there, then just skip it", func(t *testing.T) {
			// given
			space := spacetest.NewSpace(test.HostOperatorNs, "not-found",
				spacetest.WithTierName("advanced"))
			r, req, _ := prepareReconcile(t, space, nil)
			empty := test.NewFakeClient(t)
			empty.MockUpdate = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.UpdateOption) error {
				return fmt.Errorf("shouldn't be called")
			}
			r.Client = empty

			// when
			_, err := r.Reconcile(context.TODO(), req)

			// then
			require.NoError(t, err)
		})

		t.Run("when getting space fails", func(t *testing.T) {
			// given
			space := spacetest.NewSpace(test.HostOperatorNs, "get-fails",
				spacetest.WithTierName("advanced"))
			r, req, cl := prepareReconcile(t, space, nil)
			cl.MockGet = func(ctx context.Context, key runtimeclient.ObjectKey, obj runtimeclient.Object, opts ...runtimeclient.GetOption) error {
				return fmt.Errorf("some error")
			}

			// when
			_, err := r.Reconcile(context.TODO(), req)

			// then
			require.Error(t, err)
			cl.MockGet = nil
			spacetest.AssertThatSpace(t, test.HostOperatorNs, space.Name, cl).
				HasTier("advanced").
				HasSpecTargetCluster("")
		})

		t.Run("when Get ToolchainConfig fails and no field is set", func(t *testing.T) {
			// given
			space := spacetest.NewSpace(test.HostOperatorNs, "oddity",
				spacetest.WithTierName(""))
			r, req, cl := prepareReconcile(t, space, nil)
			cl.MockGet = func(ctx context.Context, key runtimeclient.ObjectKey, obj runtimeclient.Object, opts ...runtimeclient.GetOption) error {
				if key.Name == "config" {
					return fmt.Errorf("some error")
				}
				return cl.Client.Get(ctx, key, obj, opts...)
			}

			// when
			_, err := r.Reconcile(context.TODO(), req)

			// then
			require.Error(t, err)
			spacetest.AssertThatSpace(t, test.HostOperatorNs, space.Name, cl).
				HasTier("").
				HasSpecTargetCluster("")
		})

		t.Run("when listing SpaceProvisionerConfig fails and only targetCluster is missing", func(t *testing.T) {
			// given
			space := spacetest.NewSpace(test.HostOperatorNs, "oddity",
				spacetest.WithTierName("advanced"))
			r, req, cl := prepareReconcile(t, space, nil)
			cl.MockList = func(ctx context.Context, list runtimeclient.ObjectList, opts ...runtimeclient.ListOption) error {
				if _, ok := list.(*toolchainv1alpha1.SpaceProvisionerConfigList); ok {
					return errors.New("some error")
				}
				return cl.Client.List(ctx, list, opts...)
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

func prepareReconcile(t *testing.T, space *toolchainv1alpha1.Space, member1SpaceProvisionerConfig *toolchainv1alpha1.SpaceProvisionerConfig) (*spacecompletion.Reconciler, reconcile.Request, *test.FakeClient) {
	require.NoError(t, os.Setenv("WATCH_NAMESPACE", test.HostOperatorNs))
	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)

	toolchainStatus := NewToolchainStatus(
		WithMember("member1", WithNodeRoleUsage("worker", 68), WithNodeRoleUsage("master", 65)))
	t.Cleanup(counter.Reset)
	InitializeCounters(t, toolchainStatus)

	objs := []runtime.Object{toolchainStatus, space}
	if member1SpaceProvisionerConfig != nil {
		objs = append(objs, member1SpaceProvisionerConfig)
	}
	fakeClient := test.NewFakeClient(t, objs...)

	r := &spacecompletion.Reconciler{
		Client:         fakeClient,
		Namespace:      test.HostOperatorNs,
		ClusterManager: capacity.NewClusterManager(test.HostOperatorNs, fakeClient),
	}
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: test.HostOperatorNs,
			Name:      space.Name,
		},
	}
	return r, req, fakeClient
}
