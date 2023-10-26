package space

import (
	"fmt"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	spacerequesttest "github.com/codeready-toolchain/host-operator/test/spacerequest"
	"github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	commoncluster "github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	commonsignup "github.com/codeready-toolchain/toolchain-common/pkg/test/usersignup"

	spacetest "github.com/codeready-toolchain/toolchain-common/pkg/test/space"

	"github.com/stretchr/testify/assert"
)

func TestNewSpace(t *testing.T) {
	// given
	userSignup := commonsignup.NewUserSignup()

	// when
	space := NewSpace(userSignup, test.MemberClusterName, "johny", "advanced")

	// then
	expectedSpace := spacetest.NewSpace(test.HostOperatorNs, "johny",
		spacetest.WithTierName("advanced"),
		spacetest.WithSpecTargetCluster("member-cluster"),
		spacetest.WithSpecTargetClusterRoles([]string{cluster.RoleLabel(cluster.Tenant)}),
		spacetest.WithCreatorLabel(userSignup.Name))
	assert.Equal(t, expectedSpace, space)
}

func TestNewSubSpace(t *testing.T) {
	// given
	srClusterRoles := []string{commoncluster.RoleLabel(commoncluster.Tenant)}
	sr := spacerequesttest.NewSpaceRequest("jane", "jane-tenant",
		spacerequesttest.WithTierName("appstudio"),
		spacerequesttest.WithTargetClusterRoles(srClusterRoles))
	parentSpace := spacetest.NewSpace(test.HostOperatorNs, "parentSpace")

	// when
	subSpace := NewSubSpace(sr, parentSpace)

	// then
	expectedSubSpace := spacetest.NewSpace(test.HostOperatorNs, SubSpaceName(parentSpace, sr),
		spacetest.WithSpecParentSpace(parentSpace.GetName()),
		spacetest.WithTierName("appstudio"),
		spacetest.WithSpecTargetClusterRoles([]string{cluster.RoleLabel(cluster.Tenant)}),
		spacetest.WithLabel(toolchainv1alpha1.SpaceRequestLabelKey, sr.GetName()),
		spacetest.WithLabel(toolchainv1alpha1.SpaceRequestNamespaceLabelKey, sr.GetNamespace()),
		spacetest.WithLabel(toolchainv1alpha1.ParentSpaceLabelKey, parentSpace.GetName()),
	)
	assert.Equal(t, expectedSubSpace, subSpace)
}

func TestNewSubSubSpace(t *testing.T) {
	// given
	srClusterRoles := []string{commoncluster.RoleLabel(commoncluster.Tenant)}
	sr := spacerequesttest.NewSpaceRequest("jane", "jane-tenant",
		spacerequesttest.WithTierName("appstudio"),
		spacerequesttest.WithTargetClusterRoles(srClusterRoles))
	parentSpace := spacetest.NewSpace(test.HostOperatorNs, "parentSpace")
	subSpace := NewSubSpace(sr, parentSpace)
	sr2 := spacerequesttest.NewSpaceRequest("jane2", subSpace.GetName()+"-tenant",
		spacerequesttest.WithTierName("appstudio"),
		spacerequesttest.WithTargetClusterRoles(srClusterRoles))

	// when
	subSubSpace := NewSubSpace(sr2, subSpace)

	// then
	expectedSubSubSpace := spacetest.NewSpace(test.HostOperatorNs, fmt.Sprintf("%s-%s", parentSpace.Name, sr2.UID[:5]),
		spacetest.WithSpecParentSpace(subSpace.GetName()),
		spacetest.WithTierName("appstudio"),
		spacetest.WithSpecTargetClusterRoles([]string{cluster.RoleLabel(cluster.Tenant)}),
		spacetest.WithLabel(toolchainv1alpha1.SpaceRequestLabelKey, sr2.GetName()),
		spacetest.WithLabel(toolchainv1alpha1.SpaceRequestNamespaceLabelKey, sr2.GetNamespace()),
		spacetest.WithLabel(toolchainv1alpha1.ParentSpaceLabelKey, subSpace.GetName()),
	)
	assert.Equal(t, expectedSubSubSpace, subSubSpace)

	// also assert that names don't grow in length as we increase nesting
	assert.Equal(t, len(subSpace.Name), len(subSubSpace.Name))
	assert.NotEqual(t, subSpace.Name, subSubSpace.Name)
}
