package usersignup

import (
	"context"
	"fmt"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

func TestUpdateStatus(t *testing.T) {
	// given
	userSignup := &toolchainv1alpha1.UserSignup{}

	statusUpdater := StatusUpdater{Client: test.NewFakeClient(t)}

	t.Run("status updated", func(t *testing.T) {
		// given
		updateStatus := func(_ context.Context, _ *toolchainv1alpha1.UserSignup, message string) error {
			assert.Equal(t, "oopsy woopsy", message)
			return nil
		}

		// when
		err := statusUpdater.wrapErrorWithStatusUpdate(context.TODO(), userSignup, updateStatus, apierrors.NewBadRequest("oopsy woopsy"), "failed to create namespace")

		// then
		require.Error(t, err)
		assert.Equal(t, "failed to create namespace: oopsy woopsy", err.Error())
	})

	t.Run("status update failed", func(t *testing.T) {
		// given
		updateStatus := func(_ context.Context, _ *toolchainv1alpha1.UserSignup, _ string) error {
			return fmt.Errorf("unable to update status")
		}

		// when
		err := statusUpdater.wrapErrorWithStatusUpdate(context.TODO(), userSignup, updateStatus, apierrors.NewBadRequest("oopsy woopsy"), "failed to create namespace")

		// then
		require.Error(t, err)
		assert.Equal(t, "failed to create namespace: oopsy woopsy", err.Error())
	})
}
