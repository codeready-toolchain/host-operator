package spaceprovisionerconfig

import (
	"context"

	"github.com/charmbracelet/log"
	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/capacity"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	. "github.com/codeready-toolchain/toolchain-common/pkg/test/spaceprovisionerconfig"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GetSpaceCountFromSpaceProvisionerConfigs is a function that returns the space count as it is stored in the SpaceProvisionerConfig
// objects in the provided namespace. The returned function can be used as an argument to capacity.NewClusterManager().
func GetSpaceCountFromSpaceProvisionerConfigs(cl client.Client, namespace string) capacity.SpaceCountGetter {
	return func(ctx context.Context, clusterName string) (int, bool) {
		l := &toolchainv1alpha1.SpaceProvisionerConfigList{}
		if err := cl.List(ctx, l, client.InNamespace(namespace)); err != nil {
			log.FromContext(ctx).Error(err, "failed to list the SpaceProvisionerConfig objects while computing figuring out the space count stored in them")
			return 0, false
		}

		for _, spc := range l.Items {
			if spc.Spec.ToolchainCluster == clusterName {
				if spc.Status.ConsumedCapacity == nil {
					return 0, false
				}
				return spc.Status.ConsumedCapacity.SpaceCount, true
			}
		}
		return 0, false
	}
}

func NewEnabledValidTenantSPC(referencedToolchainCluster string, opts ...CreateOption) *toolchainv1alpha1.SpaceProvisionerConfig {
	return NewSpaceProvisionerConfig(referencedToolchainCluster+"Spc", test.HostOperatorNs,
		append(opts,
			Enabled(true),
			WithReadyConditionValid(),
			ReferencingToolchainCluster(referencedToolchainCluster),
			WithPlacementRoles(PlacementRole("tenant")))...,
	)
}

func NewEnabledValidSPC(referencedToolchainCluster string, opts ...CreateOption) *toolchainv1alpha1.SpaceProvisionerConfig {
	return NewSpaceProvisionerConfig(referencedToolchainCluster+"Spc", test.HostOperatorNs,
		append(opts,
			Enabled(true),
			WithReadyConditionValid(),
			ReferencingToolchainCluster(referencedToolchainCluster))...,
	)
}

func NewEnabledTenantSPC(referencedToolchainCluster string, opts ...CreateOption) *toolchainv1alpha1.SpaceProvisionerConfig {
	return NewSpaceProvisionerConfig(referencedToolchainCluster+"Spc", test.HostOperatorNs,
		append(opts,
			Enabled(true),
			ReferencingToolchainCluster(referencedToolchainCluster),
			WithPlacementRoles(PlacementRole("tenant")))...,
	)
}
