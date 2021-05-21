package usersignup

import (
	"fmt"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
)

var testLog = ctrl.Log.WithName("test")

func TestUpdateStatus(t *testing.T) {
	// given
	userSignup := &toolchainv1alpha1.UserSignup{}

	statusUpdater := StatusUpdater{Client: test.NewFakeClient(t)}

	t.Run("status updated", func(t *testing.T) {
		updateStatus := func(changeTierRequest *toolchainv1alpha1.UserSignup, message string) error {
			assert.Equal(t, "oopsy woopsy", message)
			return nil
		}

		// test
		err := statusUpdater.wrapErrorWithStatusUpdate(testLog, userSignup, updateStatus, apierrors.NewBadRequest("oopsy woopsy"), "failed to create namespace")

		require.Error(t, err)
		assert.Equal(t, "failed to create namespace: oopsy woopsy", err.Error())
	})

	t.Run("status update failed", func(t *testing.T) {
		updateStatus := func(changeTierRequest *toolchainv1alpha1.UserSignup, message string) error {
			return fmt.Errorf("unable to update status")
		}

		// when
		err := statusUpdater.wrapErrorWithStatusUpdate(testLog, userSignup, updateStatus, apierrors.NewBadRequest("oopsy woopsy"), "failed to create namespace")

		// then
		require.Error(t, err)
		assert.Equal(t, "failed to create namespace: oopsy woopsy", err.Error())
	})
}
