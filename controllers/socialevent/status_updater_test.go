package socialevent

import (
	"context"
	"fmt"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	socialeventtest "github.com/codeready-toolchain/host-operator/test/socialevent"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func TestUpdateStatusCondition(t *testing.T) {

	t.Run("status condition created", func(t *testing.T) {
		// given
		event := socialeventtest.NewSocialEvent("base", "base") // with no pre-existing status condition
		c1 := toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.ConditionReady,
			Status: corev1.ConditionTrue,
		}
		hostClient := test.NewFakeClient(t, event)
		statusUpdater := StatusUpdater{Client: hostClient}

		// when
		err := statusUpdater.updateStatusConditions(context.TODO(), event, c1)

		// then
		require.NoError(t, err)
		socialeventtest.AssertThatSocialEvent(t, test.HostOperatorNs, event.Name, hostClient).
			HasConditions(c1) // created
	})

	t.Run("status condition updated", func(t *testing.T) {
		// given
		c1 := toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.ConditionReady,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.SocialEventInvalidUserTierReason,
			Message: "NSTemplateTier 'foo' not found",
		}
		event := socialeventtest.NewSocialEvent("base", "base",
			socialeventtest.WithConditions(c1), // with pre-existing status condition
		)
		c2 := toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.ConditionReady,
			Status: corev1.ConditionTrue,
		}
		hostClient := test.NewFakeClient(t, event)
		statusUpdater := StatusUpdater{Client: hostClient}

		// when
		err := statusUpdater.updateStatusConditions(context.TODO(), event, c2)

		// then
		require.NoError(t, err)
		socialeventtest.AssertThatSocialEvent(t, test.HostOperatorNs, event.Name, hostClient).
			HasConditions(c2) // updated
	})

	t.Run("status condition update failed", func(t *testing.T) {
		// given
		c1 := toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.ConditionReady,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.SocialEventInvalidUserTierReason,
			Message: "NSTemplateTier 'foo' not found",
		}
		event := socialeventtest.NewSocialEvent("base", "base",
			socialeventtest.WithConditions(c1), // with pre-existing status condition
		)
		hostClient := test.NewFakeClient(t, event)
		hostClient.MockStatusUpdate = func(_ context.Context, _ runtimeclient.Object, _ ...runtimeclient.UpdateOption) error {
			return fmt.Errorf("mock error")
		}
		statusUpdater := StatusUpdater{Client: hostClient}
		c2 := toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.ConditionReady,
			Status: corev1.ConditionTrue,
		}

		// when
		err := statusUpdater.updateStatusConditions(context.TODO(), event, c2)

		// then
		require.EqualError(t, err, "mock error")
		socialeventtest.AssertThatSocialEvent(t, test.HostOperatorNs, event.Name, hostClient).
			HasConditions(c1) // unchanged
	})
}
