package nstemplatetier_test

import (
	"context"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/controllers/nstemplatetier"
	tiertest "github.com/codeready-toolchain/host-operator/test/nstemplatetier"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestMapTierTemplateToNSTemplateTier(t *testing.T) {
	t.Run("should return the only NSTemplateTier referencing the TierTemplate", func(t *testing.T) {
		// given
		base1nsTier := tiertest.Base1nsTier(t, tiertest.CurrentBase1nsTemplates)
		prepareTierWithRevisions(base1nsTier)
		// and we have a tier without the revisions
		otherTier := tiertest.AppStudioEnvTier(t, tiertest.AppStudioEnvTemplates)
		cl := test.NewFakeClient(t, base1nsTier, otherTier)

		// when
		// for the simplicity of the test, we only try to map one tiertemplate
		clusterResourceTierTemplate := createTierTemplate(t, "clusterresources", nil, base1nsTier.Name)
		requests := nstemplatetier.MapTierTemplateToNSTemplateTier(cl)(context.TODO(), clusterResourceTierTemplate)

		// then
		require.Len(t, requests, 1)
		assert.Contains(t, requests, newRequest(base1nsTier.Name))
	})

	t.Run("should return two NSTemplateTier when both are referencing the same TierTemplate", func(t *testing.T) {
		// given
		base1nsTier := tiertest.Base1nsTier(t, tiertest.CurrentBase1nsTemplates)
		prepareTierWithRevisions(base1nsTier)
		// we update the other tier to use the same revisions
		otherTier := tiertest.AppStudioEnvTier(t, tiertest.AppStudioEnvTemplates)
		prepareTierWithRevisions(otherTier)
		cl := test.NewFakeClient(t, base1nsTier, otherTier)

		// when
		// for the simplicity of the test we only try to map one tiertemplate
		clusterResourceTierTemplate := createTierTemplate(t, "clusterresources", nil, base1nsTier.Name)
		requests := nstemplatetier.MapTierTemplateToNSTemplateTier(cl)(context.TODO(), clusterResourceTierTemplate)

		// then
		require.Len(t, requests, 2)
		assert.Contains(t, requests, newRequest(base1nsTier.Name))
		assert.Contains(t, requests, newRequest(otherTier.Name))
	})

	t.Run("should return no NSTemplateTier when they don't use the TierTemplate", func(t *testing.T) {
		// given
		// both tiers are without revisions
		base1nsTier := tiertest.Base1nsTier(t, tiertest.CurrentBase1nsTemplates)
		otherTier := tiertest.AppStudioEnvTier(t, tiertest.AppStudioEnvTemplates)
		prepareTierWithRevisions(otherTier)
		delete(otherTier.Status.Revisions, "base1ns-clusterresources-123456new")
		cl := test.NewFakeClient(t, base1nsTier, otherTier)

		// when
		// for the simplicity of the test, we only try to map one tiertemplate
		clusterResourceTierTemplate := createTierTemplate(t, "clusterresources", nil, base1nsTier.Name)
		requests := nstemplatetier.MapTierTemplateToNSTemplateTier(cl)(context.TODO(), clusterResourceTierTemplate)

		// then
		require.Empty(t, requests)
	})
}

func prepareTierWithRevisions(tier *toolchainv1alpha1.NSTemplateTier) {
	initialRevisions := map[string]string{
		"base1ns-admin-123456new":            "base1ns-admin-123456new-abcd",
		"base1ns-clusterresources-123456new": "base1ns-clusterresources-123456new-abcd",
		"base1ns-code-123456new":             "base1ns-code-123456new-abcd",
		"base1ns-dev-123456new":              "base1ns-dev-123456new-abcd",
		"base1ns-edit-123456new":             "`base1ns-edit-123456new-abcd",
		"base1ns-stage-123456new":            "base1ns-stage-123456new-abcd",
		"base1ns-viewer-123456new":           "base1ns-viewer-123456new-abcd",
	}
	tier.Status.Revisions = initialRevisions
}

func newRequest(name string) reconcile.Request {
	return reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: test.HostOperatorNs,
			Name:      name,
		},
	}
}
