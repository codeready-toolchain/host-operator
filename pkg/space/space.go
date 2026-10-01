package space

import (
	"fmt"
	"math/rand"
	"strings"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/controllers/toolchainconfig"
	"github.com/codeready-toolchain/toolchain-common/pkg/cluster"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const maxSpaceRequestNamePrefixLength = 8

// NewSpace creates a space CR for a UserSignup object.
func NewSpace(userSignup *toolchainv1alpha1.UserSignup, targetClusterName string, compliantUserName, tier string) *toolchainv1alpha1.Space {
	labels := map[string]string{
		toolchainv1alpha1.SpaceCreatorLabelKey: userSignup.Name,
	}

	space := &toolchainv1alpha1.Space{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: userSignup.Namespace,
			Name:      compliantUserName,
			Labels:    labels,
		},
		Spec: toolchainv1alpha1.SpaceSpec{
			TargetCluster:      targetClusterName,
			TargetClusterRoles: []string{cluster.RoleLabel(cluster.Tenant)}, // by default usersignups should be provisioned to tenant clusters
			TierName:           tier,
		},
	}
	return space
}

// NewSpaceWithFeatureToggles is the same as NewSpace() but also does a feature toggle lottery drawing
// and adds the corresponding feature annotations for features which "won" and should be enabled for the space.
func NewSpaceWithFeatureToggles(userSignup *toolchainv1alpha1.UserSignup, targetClusterName string, compliantUserName, tier string, toggles []toolchainconfig.FeatureToggle) *toolchainv1alpha1.Space {
	s := NewSpace(userSignup, targetClusterName, compliantUserName, tier)
	addFeatureToggles(s, toggles)
	return s
}

// addFeatureToggles does "lottery" drawing for all given feature toggles according to their weights.
// And it adds the corresponding feature annotation to the space for features that won and should be enabled for the space.
func addFeatureToggles(space *toolchainv1alpha1.Space, toggles []toolchainconfig.FeatureToggle) {
	var winners []string
	for _, t := range toggles {
		//the value of weight is not expected to go beyond 100, it won't overflow, hence its okay to ignore the overflow linter error
		weight := int(t.Weight()) // nolint:gosec
		// We generate a random number between 0 and 100. If the number is equal to or lower than the weight
		// then the feature wins.
		// We don't use recommended crypto/rand here because we don't need crypto grade random generator
		// and math/rand with pseudo random numbers, which is much cheaper, is sufficient, so we are disabling the linter
		if weight == 100 || (weight > 0 && rand.Intn(100) <= weight) { // nolint:gosec
			// Winner!
			winners = append(winners, t.Name())
		}
	}
	if len(winners) > 0 {
		if space.Annotations == nil {
			space.Annotations = make(map[string]string)
		}
		space.Annotations[toolchainv1alpha1.FeatureToggleNameAnnotationKey] = strings.Join(winners, ",")
	}
}

// NewSubSpace creates a space CR for a SpaceRequest object.
func NewSubSpace(spaceRequest *toolchainv1alpha1.SpaceRequest, parentSpace *toolchainv1alpha1.Space, subSpaceName string) *toolchainv1alpha1.Space {
	labels := map[string]string{
		toolchainv1alpha1.SpaceRequestLabelKey:          spaceRequest.GetName(),
		toolchainv1alpha1.SpaceRequestNamespaceLabelKey: spaceRequest.GetNamespace(),
		toolchainv1alpha1.ParentSpaceLabelKey:           parentSpace.GetName(),
	}

	space := &toolchainv1alpha1.Space{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: parentSpace.GetNamespace(),
			Name:      subSpaceName,
			Labels:    labels,
		},
		Spec: toolchainv1alpha1.SpaceSpec{
			TargetClusterRoles: spaceRequest.Spec.TargetClusterRoles,
			TierName:           spaceRequest.Spec.TierName,
			ParentSpace:        parentSpace.GetName(),
			DisableInheritance: spaceRequest.Spec.DisableInheritance,
		},
	}

	// in case target cluster roles are not specified
	// let's set target cluster to be same of the parent space
	if len(spaceRequest.Spec.TargetClusterRoles) == 0 {
		space.Spec.TargetCluster = parentSpace.Spec.TargetCluster
	}

	return space
}

// SubSpaceName generates a base name for a subSpace using the root space name
// and the first 8 characters of the SpaceRequest name (trimming trailing dashes and dots).
func SubSpaceName(rootSpaceName, spaceRequestName string) string {
	shortName := strings.ReplaceAll(spaceRequestName, ".", "") // namespace names cannot contain dots, so we need to remove them before truncating
	if len(shortName) > maxSpaceRequestNamePrefixLength {
		shortName = shortName[:maxSpaceRequestNamePrefixLength]
	}
	shortName = strings.TrimRight(shortName, "-") // trailing dashes are not allowed in namespace names
	return fmt.Sprintf("%s-%s", rootSpaceName, shortName)
}
