package spaceprovisionerconfig

import (
	"context"
	"errors"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestFindingReferencedSpaceProvisionerConfigs(t *testing.T) {
	t.Run("find referenced provisioner configs", func(t *testing.T) {
		// given
		cl := test.NewFakeClient(t, &toolchainv1alpha1.SpaceProvisionerConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "spc0",
				Namespace: test.HostOperatorNs,
			},
			Spec: toolchainv1alpha1.SpaceProvisionerConfigSpec{
				ToolchainCluster: "cluster1",
			},
		}, &toolchainv1alpha1.SpaceProvisionerConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "spc1",
				Namespace: test.HostOperatorNs,
			},
			Spec: toolchainv1alpha1.SpaceProvisionerConfigSpec{
				ToolchainCluster: "cluster2",
			},
		}, &toolchainv1alpha1.SpaceProvisionerConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "spc2",
				Namespace: test.HostOperatorNs,
			},
			Spec: toolchainv1alpha1.SpaceProvisionerConfigSpec{
				ToolchainCluster: "cluster1",
			},
		})

		// when
		reqs, err := findReferencingProvisionerConfigs(context.TODO(), cl, runtimeclient.ObjectKey{Name: "cluster1", Namespace: test.HostOperatorNs})

		// then
		require.NoError(t, err)
		require.Len(t, reqs, 2)
		assert.Equal(t, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: test.HostOperatorNs,
				Name:      "spc0",
			},
		}, reqs[0])
		assert.Equal(t, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: test.HostOperatorNs,
				Name:      "spc2",
			},
		}, reqs[1])
	})
	t.Run("empty references when listing space provisioner configs fails", func(t *testing.T) {
		// given
		cl := test.NewFakeClient(t, &toolchainv1alpha1.SpaceProvisionerConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "spc0",
				Namespace: test.HostOperatorNs,
			},
			Spec: toolchainv1alpha1.SpaceProvisionerConfigSpec{
				ToolchainCluster: "cluster1",
			},
		})
		expectedErr := errors.New("expected list error")
		cl.MockList = func(ctx context.Context, list runtimeclient.ObjectList, opts ...runtimeclient.ListOption) error {
			if _, ok := list.(*toolchainv1alpha1.SpaceProvisionerConfigList); ok {
				return expectedErr
			}
			return cl.Client.List(ctx, list, opts...)
		}

		// when
		reqs, err := findReferencingProvisionerConfigs(context.TODO(), cl, runtimeclient.ObjectKey{Name: "cluster1", Namespace: test.HostOperatorNs})

		// then
		require.ErrorIs(t, err, expectedErr)
		require.Nil(t, reqs)
	})
}

func TestMapToolchainClusterToSpaceProvisionerConfigs(t *testing.T) {
	t.Run("maps existing spcs", func(t *testing.T) {
		// given
		cl := test.NewFakeClient(t, &toolchainv1alpha1.SpaceProvisionerConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "spc0",
				Namespace: test.HostOperatorNs,
			},
			Spec: toolchainv1alpha1.SpaceProvisionerConfigSpec{
				ToolchainCluster: "cluster1",
			},
		}, &toolchainv1alpha1.SpaceProvisionerConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "spc1",
				Namespace: test.HostOperatorNs,
			},
			Spec: toolchainv1alpha1.SpaceProvisionerConfigSpec{
				ToolchainCluster: "cluster2",
			},
		}, &toolchainv1alpha1.SpaceProvisionerConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "spc2",
				Namespace: test.HostOperatorNs,
			},
			Spec: toolchainv1alpha1.SpaceProvisionerConfigSpec{
				ToolchainCluster: "cluster1",
			},
		})

		// when
		reqs := MapToolchainClusterToSpaceProvisionerConfigs(context.TODO(), cl)(&toolchainv1alpha1.ToolchainCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cluster1",
				Namespace: test.HostOperatorNs,
			},
		})

		// then
		require.Len(t, reqs, 2)
		assert.Equal(t, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: test.HostOperatorNs,
				Name:      "spc0",
			},
		}, reqs[0])
		assert.Equal(t, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: test.HostOperatorNs,
				Name:      "spc2",
			},
		}, reqs[1])
	})
	t.Run("interprets errors as empty result", func(t *testing.T) {
		// given
		cl := test.NewFakeClient(t, &toolchainv1alpha1.SpaceProvisionerConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "spc0",
				Namespace: test.HostOperatorNs,
			},
			Spec: toolchainv1alpha1.SpaceProvisionerConfigSpec{
				ToolchainCluster: "cluster1",
			},
		})
		expectedErr := errors.New("expected list error")
		cl.MockList = func(ctx context.Context, list runtimeclient.ObjectList, opts ...runtimeclient.ListOption) error {
			if _, ok := list.(*toolchainv1alpha1.SpaceProvisionerConfigList); ok {
				return expectedErr
			}
			return cl.Client.List(ctx, list, opts...)
		}

		// when
		reqs := MapToolchainClusterToSpaceProvisionerConfigs(context.TODO(), cl)(&toolchainv1alpha1.ToolchainCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cluster1",
				Namespace: test.HostOperatorNs,
			},
		})

		// then
		require.Empty(t, reqs)
	})
}
