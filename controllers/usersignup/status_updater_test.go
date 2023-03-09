package usersignup

import (
	"context"
	"fmt"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	commonsignup "github.com/codeready-toolchain/toolchain-common/pkg/test/usersignup"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestUpdateStatus(t *testing.T) {
	// given
	woopsy := func(message string) toolchainv1alpha1.Condition {
		return toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupComplete,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.SpaceProvisioningFailedReason,
			Message: message,
		}
	}
	t.Run("status updated", func(t *testing.T) {
		// given
		userSignup := commonsignup.NewUserSignup()
		cl := test.NewFakeClient(t, userSignup)
		statusUpdater := StatusUpdater{Client: cl}

		// when
		err := statusUpdater.updateStatus(userSignup, woopsy("oopsy woopsy"))

		// then
		require.NoError(t, err)
		err = cl.Get(context.TODO(), client.ObjectKeyFromObject(userSignup), userSignup)
		require.NoError(t, err)
		test.AssertConditionsMatch(t, userSignup.Status.Conditions, toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupComplete,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.SpaceProvisioningFailedReason,
			Message: "oopsy woopsy",
		})
	})

	t.Run("status update failed", func(t *testing.T) {
		userSignup := &toolchainv1alpha1.UserSignup{}
		cl := test.NewFakeClient(t, userSignup)
		cl.MockStatusUpdate = func(_ context.Context, _ client.Object, _ ...client.UpdateOption) error {
			return fmt.Errorf("failed to update status")
		}
		statusUpdater := StatusUpdater{Client: cl}

		// when
		err := statusUpdater.updateStatus(userSignup, woopsy("oopsy woopsy"))

		// then
		require.Error(t, err)
		err = cl.Get(context.TODO(), client.ObjectKeyFromObject(userSignup), userSignup)
		require.NoError(t, err)
		assert.Empty(t, userSignup.Status.Conditions)
	})
}
