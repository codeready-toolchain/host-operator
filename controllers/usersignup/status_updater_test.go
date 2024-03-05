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

func TestUpdateHomeSpaceStatus(t *testing.T) {
	t.Run("home space status not updated", func(t *testing.T) {
		// given
		userSignup := &toolchainv1alpha1.UserSignup{}

		statusUpdater := StatusUpdater{Client: test.NewFakeClient(t)}

		spaceName := "foo"
		userSignup.Status.HomeSpace = "foo"

		// when
		err := statusUpdater.updateStatusHomeSpace(context.TODO(), userSignup, spaceName)

		// then
		// if the UserSignup would be updated, then the function would return an error because
		// we didn't add the UserSignup to the FakeClient as part of the setup
		// (there is nothing to be updated), thus we expect no error returned
		require.NoError(t, err)
	})

	t.Run("home space status updated", func(t *testing.T) {
		// given
		userSignup := &toolchainv1alpha1.UserSignup{}

		statusUpdater := StatusUpdater{Client: test.NewFakeClient(t)}

		spaceName := "foo"

		// when
		err := statusUpdater.updateStatusHomeSpace(context.TODO(), userSignup, spaceName)

		// then
		require.Error(t, err)
		assert.Equal(t, " \"\" is invalid: metadata.name: Required value: name is required", err.Error())
	})

}
