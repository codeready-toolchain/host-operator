package nstemplatetier_test

import (
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

func TestOutdatedTierSelector(t *testing.T) {
	nsTTier := &toolchainv1alpha1.NSTemplateTier{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: operatorNamespace,
			Name:      "base1ns",
		},
	}
	h := "d7f6dae27c505a44c3b796b4ad22ed84"
	tierLabel, _ := labels.NewRequirement(hash.TemplateTierHashLabelKey(nsTTier.Name), selection.Exists, []string{})
	templateHashLabel, _ := labels.NewRequirement(hash.TemplateTierHashLabelKey(nsTTier.Name), selection.NotEquals, []string{h})
	s := labels.NewSelector().Add(*tierLabel)
	s = s.Add(*templateHashLabel)
	expectedLabel := runtimeclient.MatchingLabelsSelector{
		Selector: s,
	}
	t.Run("OutdatedTierSelector gives a matching label", func(t *testing.T) {
		//given
		Status := toolchainv1alpha1.NSTemplateTierStatus{
			Revisions: map[string]string{
				"base1ns-code-123456old":             "base1ns-code-123456old-cr",
				"base1ns-dev-123456old":              "base1ns-dev-123456old-cr",
				"base1ns-stage-123456old":            "base1ns-stage-123456old-cr",
				"base1ns-clusterresources-123456old": "base1ns-clusterresources-123456old-cr",
			},
		}
		nsTTier.Status = Status

		//when
		matchOutdated, err := nstemplatetier.OutdatedTierSelector(nsTTier)

		require.NoError(t, err)
		require.Equal(t, expectedLabel, matchOutdated)

	})

	t.Run("OutdatedTierSelector does not give a matching label", func(t *testing.T) {
		//given

		statusout := toolchainv1alpha1.NSTemplateTierStatus{
			Revisions: map[string]string{
				"base1ns-code-123456new": "base1ns-code-123456new-cr",
			},
		}
		nsTTier.Status = statusout

		//when
		matchOutdated, err := nstemplatetier.OutdatedTierSelector(nsTTier)
		require.NoError(t, err)
		require.NotEqual(t, expectedLabel, matchOutdated)

	})
}
