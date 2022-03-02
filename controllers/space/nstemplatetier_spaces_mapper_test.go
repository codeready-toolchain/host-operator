package space_test

import (
	"context"
	"fmt"
	"testing"

	tierutil "github.com/codeready-toolchain/host-operator/controllers/nstemplatetier/util"
	"github.com/codeready-toolchain/host-operator/controllers/space"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	tiertest "github.com/codeready-toolchain/host-operator/test/nstemplatetier"
	spacetest "github.com/codeready-toolchain/host-operator/test/space"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestMapNSTemplateTierToSpaces(t *testing.T) {

	// given
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	err := apis.AddToScheme(scheme.Scheme)
	require.NoError(t, err)

	nsTmplTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates)
	otherSpace1 := spacetest.NewSpace("other-space-1", spacetest.WithTierNameAndHashLabelFor(nsTmplTier))
	otherSpace2 := spacetest.NewSpace("other-space-2", spacetest.WithTierNameAndHashLabelFor(nsTmplTier))
	otherSpace3 := spacetest.NewSpace("other-space-3", spacetest.WithTierNameAndHashLabelFor(nsTmplTier))

	t.Run("single match", func(t *testing.T) {
		// given
		outdatedSpace := spacetest.NewSpace("oddity",
			spacetest.WithTierName(nsTmplTier.Name),
			spacetest.WithLabel(tierutil.TemplateTierHashLabelKey(nsTmplTier.Name), "outdated"), // label must exist, but with an outdated value compared to the current NSTemplateTier
		)
		hostClient := test.NewFakeClient(t, nsTmplTier, outdatedSpace, otherSpace1, otherSpace2, otherSpace3)
		mapFrom := space.MapNSTemplateTierToSpaces(test.HostOperatorNs, hostClient)
		// when
		result := mapFrom(nsTmplTier)

		// then
		assert.Equal(t, []reconcile.Request{
			{
				NamespacedName: types.NamespacedName{
					Namespace: outdatedSpace.Namespace,
					Name:      outdatedSpace.Name,
				},
			},
		}, result)
	})

	t.Run("multiple matches", func(t *testing.T) {
		// given
		nsTmplTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates)
		outdatedSpace1 := spacetest.NewSpace("oddity1",
			spacetest.WithTierName(nsTmplTier.Name),
			spacetest.WithLabel(tierutil.TemplateTierHashLabelKey(nsTmplTier.Name), "outdated"), // label must exist, but with an outdated value compared to the current NSTemplateTier
		)
		outdatedSpace2 := spacetest.NewSpace("oddity2",
			spacetest.WithTierName(nsTmplTier.Name),
			spacetest.WithLabel(tierutil.TemplateTierHashLabelKey(nsTmplTier.Name), "outdated-too"), // label must exist, but with an outdated value compared to the current NSTemplateTier
		)
		hostClient := test.NewFakeClient(t, nsTmplTier, outdatedSpace1, outdatedSpace2, otherSpace1, otherSpace2, otherSpace3)
		mapFrom := space.MapNSTemplateTierToSpaces(test.HostOperatorNs, hostClient)
		// when
		result := mapFrom(nsTmplTier)

		// then
		assert.ElementsMatch(t, []reconcile.Request{
			{
				NamespacedName: types.NamespacedName{
					Namespace: outdatedSpace1.Namespace,
					Name:      outdatedSpace1.Name,
				},
			},
			{
				NamespacedName: types.NamespacedName{
					Namespace: outdatedSpace2.Namespace,
					Name:      outdatedSpace2.Name,
				},
			},
		}, result)
	})

	t.Run("no match", func(t *testing.T) {
		// given
		nsTmplTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates)
		hostClient := test.NewFakeClient(t, nsTmplTier, otherSpace1, otherSpace2, otherSpace3)
		mapFrom := space.MapNSTemplateTierToSpaces(test.HostOperatorNs, hostClient)
		// when
		result := mapFrom(nsTmplTier)

		// then
		assert.Empty(t, result)
	})

	t.Run("failures", func(t *testing.T) {

		t.Run("when listing NSTemplateTier", func(t *testing.T) {
			// given
			nsTmplTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates)
			hostClient := test.NewFakeClient(t, nsTmplTier)
			hostClient.MockList = func(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
				return fmt.Errorf("mock error")
			}
			mapFrom := space.MapNSTemplateTierToSpaces(test.HostOperatorNs, hostClient)

			// when
			result := mapFrom(nsTmplTier)

			// then
			assert.Empty(t, result)
		})

		t.Run("when processing another type of resource", func(t *testing.T) {
			// given
			hostClient := test.NewFakeClient(t)
			mapFrom := space.MapNSTemplateTierToSpaces(test.HostOperatorNs, hostClient)

			// when
			result := mapFrom(spacetest.NewSpace("oddity")) // wrong type of resource as arg

			// then
			assert.Empty(t, result)
		})
	})
}
