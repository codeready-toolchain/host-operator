package toolchainconfig

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
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
	defaultConfig, err := GetConfig(cl, HostOperatorNs)

	// then
	require.NoError(t, err)
	assert.False(t, defaultConfig.AutomaticApproval().IsEnabled())
	assert.Equal(t, 0, defaultConfig.AutomaticApproval().MaxNumberOfUsersOverall())
	assert.Empty(t, defaultConfig.AutomaticApproval().MaxNumberOfUsersSpecificPerMemberCluster())
	assert.Equal(t, 0, defaultConfig.AutomaticApproval().ResourceCapacityThresholdDefault())
	assert.Empty(t, defaultConfig.AutomaticApproval().ResourceCapacityThresholdSpecificPerMemberCluster())
	assert.Equal(t, 3, defaultConfig.Deactivation().DeactivatingNotificationInDays())

	t.Run("return config that is stored in client", func(t *testing.T) {
		// given
		config := newToolchainConfigWithReset(t, AutomaticApprovalCfg().MaxUsersNumber(123, PerMemberClusterCfg("member1", 321)))
		cl := NewFakeClient(t, config)

		// when
		actual, err := GetConfig(cl, HostOperatorNs)

		// then
		require.NoError(t, err)
		assert.False(t, actual.AutomaticApproval().IsEnabled())
		assert.Equal(t, 123, actual.AutomaticApproval().MaxNumberOfUsersOverall())
		assert.Equal(t, config.Spec.Host.AutomaticApproval.MaxNumberOfUsers.SpecificPerMemberCluster, actual.AutomaticApproval().MaxNumberOfUsersSpecificPerMemberCluster())
		assert.Equal(t, 0, actual.AutomaticApproval().ResourceCapacityThresholdDefault())
		assert.Equal(t, config.Spec.Host.AutomaticApproval.ResourceCapacityThreshold.SpecificPerMemberCluster, actual.AutomaticApproval().ResourceCapacityThresholdSpecificPerMemberCluster())
		assert.Equal(t, 3, actual.Deactivation().DeactivatingNotificationInDays())

		t.Run("returns the same when the cache hasn't been updated", func(t *testing.T) {
			// given
			newConfig := newToolchainConfigWithReset(t, AutomaticApprovalCfg().MaxUsersNumber(666))
			cl := NewFakeClient(t, newConfig)

			// when
			actual, err := GetConfig(cl, HostOperatorNs)

			// then
			require.NoError(t, err)
			assert.False(t, actual.AutomaticApproval().IsEnabled())
			assert.Equal(t, 123, actual.AutomaticApproval().MaxNumberOfUsersOverall())
			assert.Equal(t, config.Spec.Host.AutomaticApproval.MaxNumberOfUsers.SpecificPerMemberCluster, actual.AutomaticApproval().MaxNumberOfUsersSpecificPerMemberCluster())
			assert.Equal(t, 0, actual.AutomaticApproval().ResourceCapacityThresholdDefault())
			assert.Equal(t, config.Spec.Host.AutomaticApproval.ResourceCapacityThreshold.SpecificPerMemberCluster, actual.AutomaticApproval().ResourceCapacityThresholdSpecificPerMemberCluster())
			assert.Equal(t, 3, actual.Deactivation().DeactivatingNotificationInDays())
		})

		t.Run("returns the new config when the cache was updated", func(t *testing.T) {
			// given
			newConfig := newToolchainConfigWithReset(t, AutomaticApprovalCfg().MaxUsersNumber(666), DeactivationCfg().DeactivatingNotificationDays(5))
			cl := NewFakeClient(t)

			// when
			updateConfig(newConfig)

			// then
			actual, err := GetConfig(cl, HostOperatorNs)
			require.NoError(t, err)
			assert.False(t, actual.AutomaticApproval().IsEnabled())
			assert.Equal(t, 666, actual.AutomaticApproval().MaxNumberOfUsersOverall())
			assert.Empty(t, actual.AutomaticApproval().MaxNumberOfUsersSpecificPerMemberCluster())
			assert.Equal(t, 0, actual.AutomaticApproval().ResourceCapacityThresholdDefault())
			assert.Empty(t, actual.AutomaticApproval().ResourceCapacityThresholdSpecificPerMemberCluster())
			assert.Equal(t, 5, actual.Deactivation().DeactivatingNotificationInDays())
		})
	})
}

func TestGetConfigErrored(t *testing.T) {
	// given
	config := newToolchainConfigWithReset(t, AutomaticApprovalCfg().MaxUsersNumber(123, PerMemberClusterCfg("member1", 321)))
	cl := NewFakeClient(t, config)
	cl.MockGet = func(ctx context.Context, key client.ObjectKey, obj runtime.Object) error {
		return fmt.Errorf("some error")
	}

	// when
	defaultConfig, err := GetConfig(cl, HostOperatorNs)

	// then
	require.Error(t, err)
	assert.False(t, defaultConfig.AutomaticApproval().IsEnabled())
	assert.Equal(t, 0, defaultConfig.AutomaticApproval().MaxNumberOfUsersOverall())
	assert.Empty(t, defaultConfig.AutomaticApproval().MaxNumberOfUsersSpecificPerMemberCluster())
	assert.Equal(t, 0, defaultConfig.AutomaticApproval().ResourceCapacityThresholdDefault())
	assert.Empty(t, defaultConfig.AutomaticApproval().ResourceCapacityThresholdSpecificPerMemberCluster())
	assert.Equal(t, 3, defaultConfig.Deactivation().DeactivatingNotificationInDays())
}

func TestMultipleExecutionsInParallel(t *testing.T) {
	// given
	var latch sync.WaitGroup
	latch.Add(1)
	var waitForFinished sync.WaitGroup
	initconfig := newToolchainConfigWithReset(t, AutomaticApprovalCfg().MaxUsersNumber(1, PerMemberClusterCfg("member", 1)))
	cl := NewFakeClient(t, initconfig)

	for i := 0; i < 1000; i++ {
		waitForFinished.Add(2)
		go func() {
			defer waitForFinished.Done()
			latch.Wait()

			// when
			config, err := GetConfig(cl, HostOperatorNs)

			// then
			require.NoError(t, err)
			assert.NotEmpty(t, config.AutomaticApproval().MaxNumberOfUsersOverall())
			assert.NotEmpty(t, config.AutomaticApproval().MaxNumberOfUsersSpecificPerMemberCluster())
		}()
		go func(i int) {
			defer waitForFinished.Done()
			latch.Wait()
			config := newToolchainConfigWithReset(t, AutomaticApprovalCfg().MaxUsersNumber(i+1, PerMemberClusterCfg(fmt.Sprintf("member%d", i), i)))
			updateConfig(config)
		}(i)
	}

	// when
	latch.Done()
	waitForFinished.Wait()
	config, err := GetConfig(NewFakeClient(t), HostOperatorNs)

	// then
	require.NoError(t, err)
	assert.NotEmpty(t, config.AutomaticApproval().MaxNumberOfUsersOverall())
	assert.NotEmpty(t, config.AutomaticApproval().MaxNumberOfUsersSpecificPerMemberCluster())
}

func newToolchainConfigWithReset(t *testing.T, options ...ToolchainConfigOption) *v1alpha1.ToolchainConfig {
	t.Cleanup(Reset)
	return NewToolchainConfig(options...)
}

func matchesDefaultConfig(t *testing.T, actual ToolchainConfig) {
	assert.False(t, actual.AutomaticApproval().IsEnabled())
	assert.Equal(t, 0, actual.AutomaticApproval().MaxNumberOfUsersOverall())
	assert.Empty(t, actual.AutomaticApproval().MaxNumberOfUsersSpecificPerMemberCluster())
	assert.Equal(t, 0, actual.AutomaticApproval().ResourceCapacityThresholdDefault())
	assert.Empty(t, actual.AutomaticApproval().ResourceCapacityThresholdSpecificPerMemberCluster())
	assert.Equal(t, 3, actual.Deactivation().DeactivatingNotificationInDays())
}
