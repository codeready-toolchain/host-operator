package spaceprovisionerconfig

import (
	"context"
	"os"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/apis"
	"github.com/codeready-toolchain/toolchain-common/pkg/condition"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/kubectl/pkg/scheme"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestSpaceProvisionerConfig(t *testing.T) {
	t.Run("is not valid when non-existing ToolchainCluster is referenced", func(t *testing.T) {
		// given
		spc := &toolchainv1alpha1.SpaceProvisionerConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "spc",
				Namespace: test.HostOperatorNs,
			},
			Spec: toolchainv1alpha1.SpaceProvisionerConfigSpec{
				ToolchainCluster: "non-existent",
			},
		}
		r, req, cl := prepareReconcile(t, spc)

		// when
		_, reconcileErr := r.Reconcile(context.TODO(), req)
		getErr := cl.Get(context.TODO(), runtimeclient.ObjectKey{Name: "spc", Namespace: test.HostOperatorNs}, spc)

		// then
		require.NoError(t, reconcileErr)
		require.NoError(t, getErr)
		assert.True(t, condition.IsFalse(spc.Status.Conditions, toolchainv1alpha1.ConditionReady))
	})

	t.Run("is valid when existing ToolchainCluster is referenced", func(t *testing.T) {
		// given
		spc := &toolchainv1alpha1.SpaceProvisionerConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "spc",
				Namespace: test.HostOperatorNs,
			},
			Spec: toolchainv1alpha1.SpaceProvisionerConfigSpec{
				ToolchainCluster: "cluster1",
			},
		}
		r, req, cl := prepareReconcile(t, spc, &toolchainv1alpha1.ToolchainCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cluster1",
				Namespace: test.HostOperatorNs,
			},
		})

		// when
		_, reconcileErr := r.Reconcile(context.TODO(), req)
		getErr := cl.Get(context.TODO(), runtimeclient.ObjectKey{Name: "spc", Namespace: test.HostOperatorNs}, spc)

		// then
		require.NoError(t, reconcileErr)
		require.NoError(t, getErr)
		assert.True(t, condition.IsTrue(spc.Status.Conditions, toolchainv1alpha1.ConditionReady))
	})
}

func TestEnqueingFilter(t *testing.T) {
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
}

func prepareReconcile(t *testing.T, spc *toolchainv1alpha1.SpaceProvisionerConfig, initObjs ...runtime.Object) (*Reconciler, reconcile.Request, *test.FakeClient) {
	require.NoError(t, os.Setenv("WATCH_NAMESPACE", test.HostOperatorNs))
	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)

	fakeClient := test.NewFakeClient(t, append(initObjs, spc)...)

	r := &Reconciler{
		Client: fakeClient,
	}
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: test.HostOperatorNs,
			Name:      spc.Name,
		},
	}
	return r, req, fakeClient
}
