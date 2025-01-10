package spaceprovisionerconfig

import (
	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	. "github.com/codeready-toolchain/toolchain-common/pkg/test/spaceprovisionerconfig"
)

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
