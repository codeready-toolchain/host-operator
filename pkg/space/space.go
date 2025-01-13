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
func NewSubSpace(spaceRequest *toolchainv1alpha1.SpaceRequest, parentSpace *toolchainv1alpha1.Space) *toolchainv1alpha1.Space {
	labels := map[string]string{
		toolchainv1alpha1.SpaceRequestLabelKey:          spaceRequest.GetName(),
		toolchainv1alpha1.SpaceRequestNamespaceLabelKey: spaceRequest.GetNamespace(),
		toolchainv1alpha1.ParentSpaceLabelKey:           parentSpace.GetName(),
	}

	space := &toolchainv1alpha1.Space{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: parentSpace.GetNamespace(),
			Name:      SubSpaceName(parentSpace, spaceRequest),
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

// SubSpaceName generates a name for a subSpace based on parentSpace name and spacerequest UID.
func SubSpaceName(parentSpace *toolchainv1alpha1.Space, spacerequest *toolchainv1alpha1.SpaceRequest) string {
	parentSpaceName := parentSpace.GetName()

	// if the parent space is itself a subspace, then we need to strip its UID
	// suffix to prevent length limitations from kicking in
	if parentSpace.Spec.ParentSpace != "" && len(parentSpaceName) > 6 {
		// take off 6 characters to include the dash.  for example,
		// "parentspace-12345" becomes "parentspace" when we strip the
		// 5-character UID suffix and the "-"
		parentSpaceName = string(parentSpaceName[:len(parentSpaceName)-6])
	}

	// only get the first 5 chars from the spacerequest's UID so we can keep
	// the name within length limits.
	return fmt.Sprintf("%v-%v", parentSpaceName, string(spacerequest.UID[:5]))
}
