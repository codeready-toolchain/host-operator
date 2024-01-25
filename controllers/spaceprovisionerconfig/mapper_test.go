package spaceprovisionerconfig

import (
	"context"
	"errors"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	testSpc "github.com/codeready-toolchain/toolchain-common/pkg/test/spaceprovisionerconfig"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func requestFromObject(obj runtimeclient.Object) reconcile.Request {
	return reconcile.Request{
		NamespacedName: runtimeclient.ObjectKeyFromObject(obj),
	}
}

func TestFindingReferencingSpaceProvisionerConfigs(t *testing.T) {
	type testCase struct {
		ToolchainClusterName string
		ExpectedRequests     []reconcile.Request
	}

	spc0 := testSpc.NewSpaceProvisionerConfig("spc0", test.HostOperatorNs, testSpc.ReferencingToolchainCluster("cluster1"))
	spc1 := testSpc.NewSpaceProvisionerConfig("spc1", test.HostOperatorNs, testSpc.ReferencingToolchainCluster("cluster2"))
	spc2 := testSpc.NewSpaceProvisionerConfig("spc2", test.HostOperatorNs, testSpc.ReferencingToolchainCluster("cluster1"))

	tests := map[string]testCase{
		"find 1 referencing SpaceProvisionerConfig in many": {
			ToolchainClusterName: "cluster2",
			ExpectedRequests: []reconcile.Request{
				requestFromObject(spc1),
			},
		},
		"find more referencing SpaceProvisionerConfigs in many": {
			ToolchainClusterName: "cluster1",
			ExpectedRequests: []reconcile.Request{
				requestFromObject(spc0),
				requestFromObject(spc2),
			},
		},
		"find no referencing SpaceProvisionerConfigs in many": {
			ToolchainClusterName: "cluster3",
			ExpectedRequests:     []reconcile.Request{},
		},
	}

	for name, tst := range tests {
		t.Run(name, func(t *testing.T) {
			// given
			cl := test.NewFakeClient(t, spc0, spc1, spc2)

			// when
			reqs, err := findReferencingProvisionerConfigs(context.TODO(), cl, runtimeclient.ObjectKey{Name: tst.ToolchainClusterName, Namespace: test.HostOperatorNs})

			// then
			require.NoError(t, err)
			require.Equal(t, reqs, tst.ExpectedRequests)
		})
	}

	t.Run("empty references when listing space provisioner configs fails", func(t *testing.T) {
		// given
		cl := test.NewFakeClient(t, testSpc.NewSpaceProvisionerConfig("spc0", test.HostOperatorNs, testSpc.ReferencingToolchainCluster("cluster1")))
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
		// this pretty much duplicates the test of finding the referencing space provisioner configs but does it from the POV
		// of the higher level function. Therefore, we don't need to replicate all the positive cases here, just one should
		//

		// given
		spc0 := testSpc.NewSpaceProvisionerConfig("spc0", test.HostOperatorNs, testSpc.ReferencingToolchainCluster("cluster1"))
		spc1 := testSpc.NewSpaceProvisionerConfig("spc1", test.HostOperatorNs, testSpc.ReferencingToolchainCluster("cluster2"))
		spc2 := testSpc.NewSpaceProvisionerConfig("spc2", test.HostOperatorNs, testSpc.ReferencingToolchainCluster("cluster1"))
		cl := test.NewFakeClient(t, spc0, spc1, spc2)

		// when
		reqs := MapToolchainClusterToSpaceProvisionerConfigs(context.TODO(), cl)(&toolchainv1alpha1.ToolchainCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cluster1",
				Namespace: test.HostOperatorNs,
			},
		})

		// then
		require.Equal(t, reqs, []reconcile.Request{
			requestFromObject(spc0),
			requestFromObject(spc2),
		})
	})

	t.Run("interprets errors as empty result", func(t *testing.T) {
		// given
		cl := test.NewFakeClient(t, testSpc.NewSpaceProvisionerConfig("spc0", test.HostOperatorNs, testSpc.ReferencingToolchainCluster("cluster1")))
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
