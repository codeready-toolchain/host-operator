package hostoperatorconfig

import (
	"context"
	"fmt"
	"sync"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	. "github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestGetConfig(t *testing.T) {
	// given
	cl := NewFakeClient(t)

	// when
	configSpec, err := GetConfig(cl, HostOperatorNs)

	// then
	require.NoError(t, err)
	assert.Equal(t, toolchainv1alpha1.HostConfig{}, configSpec)

	t.Run("return config that is stored in client", func(t *testing.T) {
		// given
		config := newHostOperatorConfigWithReset(t, AutomaticApproval().MaxUsersNumber(123, PerMemberCluster("member1", 321)))
		cl := NewFakeClient(t, config)

		// when
		configSpec, err := GetConfig(cl, HostOperatorNs)

		// then
		require.NoError(t, err)
		assert.Equal(t, config.Spec, configSpec)

		t.Run("returns the same when the cache hasn't been updated", func(t *testing.T) {
			// given
			newConfig := newHostOperatorConfigWithReset(t, AutomaticApproval().MaxUsersNumber(666))
			cl := NewFakeClient(t, newConfig)

			// when
			configSpec, err := GetConfig(cl, HostOperatorNs)

			// then
			require.NoError(t, err)
			assert.Equal(t, config.Spec, configSpec)
		})

		t.Run("returns the new config when the cache was updated", func(t *testing.T) {
			// given
			newConfig := newHostOperatorConfigWithReset(t, AutomaticApproval().MaxUsersNumber(666))
			cl := NewFakeClient(t)

			// when
			updateConfig(newConfig)

			// then
			configSpec, err := GetConfig(cl, HostOperatorNs)
			require.NoError(t, err)
			assert.Equal(t, newConfig.Spec, configSpec)
		})
	})
}

func TestGetConfigErrored(t *testing.T) {
	// given
	config := newHostOperatorConfigWithReset(t, AutomaticApproval().MaxUsersNumber(123, PerMemberCluster("member1", 321)))
	cl := NewFakeClient(t, config)
	cl.MockGet = func(ctx context.Context, key client.ObjectKey, obj runtime.Object) error {
		return fmt.Errorf("some error")
	}

	// when
	configSpec, err := GetConfig(cl, HostOperatorNs)

	// then
	require.Error(t, err)
	assert.Equal(t, toolchainv1alpha1.HostConfig{}, configSpec)
}

func TestMultipleExecutionsInParallel(t *testing.T) {
	// given
	var latch sync.WaitGroup
	latch.Add(1)
	var waitForFinished sync.WaitGroup
	config := newHostOperatorConfigWithReset(t, AutomaticApproval().MaxUsersNumber(1, PerMemberCluster("member", 1)))
	cl := NewFakeClient(t, config)

	for i := 0; i < 1000; i++ {
		waitForFinished.Add(2)
		go func() {
			defer waitForFinished.Done()
			latch.Wait()

			// when
			configSpec, err := GetConfig(cl, HostOperatorNs)

			// then
			require.NoError(t, err)
			assert.NotEmpty(t, configSpec.AutomaticApproval.MaxNumberOfUsers)
			assert.NotEmpty(t, configSpec.AutomaticApproval.MaxNumberOfUsers.SpecificPerMemberCluster)
		}()
		go func(i int) {
			defer waitForFinished.Done()
			latch.Wait()
			config := newHostOperatorConfigWithReset(t, AutomaticApproval().MaxUsersNumber(i+1, PerMemberCluster(fmt.Sprintf("member%d", i), i)))
			updateConfig(config)
		}(i)
	}

	// when
	latch.Done()
	waitForFinished.Wait()
	configSpec, err := GetConfig(NewFakeClient(t), HostOperatorNs)

	// then
	require.NoError(t, err)
	assert.NotEmpty(t, configSpec.AutomaticApproval.MaxNumberOfUsers)
	assert.NotEmpty(t, configSpec.AutomaticApproval.MaxNumberOfUsers.SpecificPerMemberCluster)
}

func newHostOperatorConfigWithReset(t *testing.T, options ...HostConfigOption) *toolchainv1alpha1.HostOperatorConfig {
	t.Cleanup(Reset)
	return NewHostOperatorConfig(options...)
}
