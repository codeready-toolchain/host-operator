package nstemplatetier_test

import (
	"fmt"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/controllers/nstemplatetier"
	"github.com/codeready-toolchain/toolchain-common/pkg/hash"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func TestOutdatedTierSelecter(t *testing.T) {

	t.Run("OutdatedTierSelecter gives a matching label", func(t *testing.T) {
		//given
		nsTTier := &toolchainv1alpha1.NSTemplateTier{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: operatorNamespace,
				Name:      "base1ns",
			},
			Spec: toolchainv1alpha1.NSTemplateTierSpec{
				Namespaces: []toolchainv1alpha1.NSTemplateTierNamespace{
					{
						TemplateRef: "base1ns-code-123456old",
					},
					{
						TemplateRef: "base1ns-dev-123456old",
					},
					{
						TemplateRef: "base1ns-stage-123456old",
					},
				},
			},
			Status: toolchainv1alpha1.NSTemplateTierStatus{
				Revisions: map[string]string{
					"base1ns-code-123456old":  "base1ns-code-123456old",
					"base1ns-dev-123456old":   "base1ns-dev-123456old",
					"base1ns-stage-123456old": "base1ns-stage-123456old",
				},
			},
		}
		h := "78195005ba2e1f73a06d4cf6af823e40"
		tierLabel, _ := labels.NewRequirement(hash.TemplateTierHashLabelKey(nsTTier.Name), selection.Exists, []string{})
		templateHashLabel, _ := labels.NewRequirement(hash.TemplateTierHashLabelKey(nsTTier.Name), selection.NotEquals, []string{h})
		s := labels.NewSelector().Add(*tierLabel)
		s = s.Add(*templateHashLabel)
		expectedLabel := runtimeclient.MatchingLabelsSelector{
			Selector: s,
		}
		//when
		matchOutdated, err := nstemplatetier.OutdatedTierSelector(nsTTier)

		require.NoError(t, err)
		require.Equal(t, expectedLabel, matchOutdated)
		fmt.Println("matched:", matchOutdated)
		fmt.Println("expected:", expectedLabel.Selector)

	})
}
