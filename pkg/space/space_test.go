package space

import (
	"fmt"
	"strings"
	"testing"

	"github.com/codeready-toolchain/host-operator/controllers/toolchainconfig"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	spacerequesttest "github.com/codeready-toolchain/host-operator/test/spacerequest"
	commoncluster "github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	spacetest "github.com/codeready-toolchain/toolchain-common/pkg/test/space"
	commonsignup "github.com/codeready-toolchain/toolchain-common/pkg/test/usersignup"

	"github.com/stretchr/testify/assert"
)

func TestNewSpace(t *testing.T) {
	// given
	userSignup := commonsignup.NewUserSignup()

	// when
	space := NewSpace(userSignup, test.MemberClusterName, "johny", "ourtier")

	// then
	expectedSpace := spacetest.NewSpace(test.HostOperatorNs, "johny",
		spacetest.WithTierName("ourtier"),
		spacetest.WithSpecTargetCluster("member-cluster"),
		spacetest.WithSpecTargetClusterRoles([]string{commoncluster.RoleLabel(commoncluster.Tenant)}),
		spacetest.WithCreatorLabel(userSignup.Name))
	assert.Equal(t, expectedSpace, space)
}

func TestNewSpaceWithFeatureToggles(t *testing.T) {
	// given
	userSignup := commonsignup.NewUserSignup()
	weight100 := uint(100)
	weight0 := uint(0)
	weight50 := uint(50)
	featureToggles := []toolchainconfig.FeatureToggle{
		toolchainconfig.NewFeatureToggle(toolchainv1alpha1.FeatureToggle{
			Name:   "my-cool-feature",
			Weight: &weight100,
		}),
		toolchainconfig.NewFeatureToggle(toolchainv1alpha1.FeatureToggle{
			Name:   "my-not-so-cool-feature",
			Weight: &weight0,
		}),
		toolchainconfig.NewFeatureToggle(toolchainv1alpha1.FeatureToggle{
			Name:   "my-so-so-feature",
			Weight: &weight50,
		}),
	}

	// when
	// create space 100 times and verify that the feature with weight 100 is always added
	// the feature with weight 0 never added
	// and the feature with weight 50 is added at least once and not added at least once too
	var myCoolFeatureCount, myNotSoCoolFeatureCount, mySoSoFeatureCount int
	for i := 0; i < 100; i++ {
		space := NewSpaceWithFeatureToggles(userSignup, test.MemberClusterName, "johny", "ourtier", featureToggles)

		// then
		expectedSpace := spacetest.NewSpace(test.HostOperatorNs, "johny",
			spacetest.WithTierName("ourtier"),
			spacetest.WithSpecTargetCluster("member-cluster"),
			spacetest.WithSpecTargetClusterRoles([]string{commoncluster.RoleLabel(commoncluster.Tenant)}),
			spacetest.WithCreatorLabel(userSignup.Name))
		assert.Equal(t, expectedSpace.Spec, space.Spec)
		features := space.GetAnnotations()[toolchainv1alpha1.FeatureToggleNameAnnotationKey]
		for _, feature := range strings.Split(features, ",") {
			switch feature {
			case "my-cool-feature":
				myCoolFeatureCount++
			case "my-not-so-cool-feature":
				myNotSoCoolFeatureCount++
			case "my-so-so-feature":
				mySoSoFeatureCount++
			default:
				assert.Fail(t, "unknown feature", feature)
			}
		}
	}

	// then
	assert.Equal(t, 100, myCoolFeatureCount)
	assert.Equal(t, 0, myNotSoCoolFeatureCount)
	assert.True(t, mySoSoFeatureCount > 0 && mySoSoFeatureCount < 100, "Feature with weight 50 should be present at least once but less than 100 times", mySoSoFeatureCount)
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
		spacetest.WithSpecTargetClusterRoles([]string{commoncluster.RoleLabel(commoncluster.Tenant)}),
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
		spacetest.WithSpecTargetClusterRoles([]string{commoncluster.RoleLabel(commoncluster.Tenant)}),
		spacetest.WithLabel(toolchainv1alpha1.SpaceRequestLabelKey, sr2.GetName()),
		spacetest.WithLabel(toolchainv1alpha1.SpaceRequestNamespaceLabelKey, sr2.GetNamespace()),
		spacetest.WithLabel(toolchainv1alpha1.ParentSpaceLabelKey, subSpace.GetName()),
	)
	assert.Equal(t, expectedSubSubSpace, subSubSpace)

	// also assert that names don't grow in length as we increase nesting
	assert.Len(t, subSubSpace.Name, len(subSpace.Name))
	assert.NotEqual(t, subSpace.Name, subSubSpace.Name)
}
