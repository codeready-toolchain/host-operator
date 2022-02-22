package space_test

import (
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
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestMapNSTemplateTierToSpaces(t *testing.T) {

	// given
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	err := apis.AddToScheme(scheme.Scheme)
	require.NoError(t, err)

	t.Run("single match", func(t *testing.T) {
		// given
		nsTmplTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates)
		outdatedSpace := spacetest.NewSpace("oddity",
			spacetest.WithTierName(nsTmplTier.Name),
			spacetest.WithLabel(tierutil.TemplateTierHashLabelKey(nsTmplTier.Name), "outdated"), // label must exist, but with an outdated value compared to the current NSTemplateTier
		)
		hostClient := test.NewFakeClient(t, nsTmplTier, outdatedSpace)
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

	t.Run("multiple matchs", func(t *testing.T) {
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
		uptodateSpace := spacetest.NewSpace("oddity3",
			spacetest.WithTierNameAndHashLabelFor(nsTmplTier),
		)
		hostClient := test.NewFakeClient(t, nsTmplTier, outdatedSpace1, outdatedSpace2, uptodateSpace)
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
		uptodateSpace := spacetest.NewSpace("oddity3",
			spacetest.WithTierNameAndHashLabelFor(nsTmplTier),
		)
		hostClient := test.NewFakeClient(t, nsTmplTier, uptodateSpace)
		mapFrom := space.MapNSTemplateTierToSpaces(test.HostOperatorNs, hostClient)
		// when
		result := mapFrom(nsTmplTier)

		// then
		assert.Empty(t, result)
	})

}
