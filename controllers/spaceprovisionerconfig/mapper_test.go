package spaceprovisionerconfig

import (
	"context"
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

func TestToolchainClusterToSpaceProvisionerConfigMapper(t *testing.T) {
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
