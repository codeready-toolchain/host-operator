package space

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	murtest "github.com/codeready-toolchain/toolchain-common/pkg/test/masteruserrecord"
	"github.com/stretchr/testify/require"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const DefaultNSTemplateTierName = "basic"

func WithTargetCluster(targetCluster string) SpaceModifier {
	return func(space *toolchainv1alpha1.Space) {
		space.Spec.TargetCluster = targetCluster
	}
}

func WithTierNameAndHashLabelFor(t *testing.T, tier toolchainv1alpha1.NSTemplateTier) SpaceModifier {
	return func(space *toolchainv1alpha1.Space) {
		hash, err := computeTemplateRefsHash(tier) // we can assume the JSON marshalling will always work
		require.NoError(t, err)
		space.Spec.TierName = tier.Name
		space.ObjectMeta.Labels = map[string]string{
			templateTierHashLabelKey(tier.Name): hash,
		}
	}
}

// func WithHashLabelForTier(tier string) SpaceModifier {
// 	return func(space *toolchainv1alpha1.Space) {
// 		space.Labels[key] = value
// 	}
// }

type SpaceModifier func(*toolchainv1alpha1.Space)

func NewSpace(t *testing.T, name string, modifiers ...SpaceModifier) *toolchainv1alpha1.Space {
	hash, err := computeTemplateRefsHash(murtest.DefaultNSTemplateTier()) // we can assume the JSON marshalling will always work
	require.NoError(t, err)
	space := &toolchainv1alpha1.Space{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: test.HostOperatorNs,
			Labels: map[string]string{
				templateTierHashLabelKey(DefaultNSTemplateTierName): hash,
			},
		},
		Spec: toolchainv1alpha1.SpaceSpec{
			TargetCluster: "member-1",
			TierName:      "basic",
		},
	}
	for _, modify := range modifiers {
		modify(space)
	}
	return space
}

func NewSpaces(t *testing.T, size int, nameFmt string, modifiers ...SpaceModifier) []runtime.Object {
	murs := make([]runtime.Object, size)
	for i := 0; i < size; i++ {
		murs[i] = NewSpace(t, fmt.Sprintf(nameFmt, i), modifiers...)
	}
	return murs
}

type templateRefs struct {
	Refs []string `json:"refs"`
}

// computeTemplateRefsHash computes the hash of the `.spec.namespaces[].templateRef` + `.spec.clusteResource.TemplateRef`
func computeTemplateRefsHash(tier toolchainv1alpha1.NSTemplateTier) (string, error) {
	refs := []string{}
	for _, ns := range tier.Spec.Namespaces {
		refs = append(refs, ns.TemplateRef)
	}
	if tier.Spec.ClusterResources != nil {
		refs = append(refs, tier.Spec.ClusterResources.TemplateRef)
	}
	sort.Strings(refs)
	m, err := json.Marshal(templateRefs{Refs: refs})
	if err != nil {
		return "", err
	}
	md5hash := md5.New()
	// Ignore the error, as this implementation cannot return one
	_, _ = md5hash.Write(m)
	hash := hex.EncodeToString(md5hash.Sum(nil))
	return hash, nil
}

// TODO move to toolchain-common
// templateTierHashLabel returns the label key to specify the version of the templates of the given tier
func templateTierHashLabelKey(tierName string) string {
	return toolchainv1alpha1.LabelKeyPrefix + tierName + "-tier-hash"
}
