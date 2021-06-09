package toolchainconfig

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/codeready-toolchain/toolchain-common/pkg/test/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestCache(t *testing.T) {
	// given
	cl := test.NewFakeClient(t)

	// when
	defaultConfig, err := GetConfig(cl, test.HostOperatorNs)

	// then
	require.NoError(t, err)
	assert.Equal(t, 1000, defaultConfig.AutomaticApproval().MaxNumberOfUsersOverall())
	assert.Empty(t, defaultConfig.AutomaticApproval().MaxNumberOfUsersSpecificPerMemberCluster())

	t.Run("return config that is stored in client", func(t *testing.T) {
		// given
		cfg := newToolchainConfigWithReset(t, config.AutomaticApproval().MaxUsersNumber(123, config.PerMemberCluster("member1", 321)))
		cl := test.NewFakeClient(t, cfg)

		// when
		actual, err := GetConfig(cl, test.HostOperatorNs)

		// then
		require.NoError(t, err)
		assert.Equal(t, 123, actual.AutomaticApproval().MaxNumberOfUsersOverall())
		assert.Equal(t, cfg.Spec.Host.AutomaticApproval.MaxNumberOfUsers.SpecificPerMemberCluster, actual.AutomaticApproval().MaxNumberOfUsersSpecificPerMemberCluster())

		t.Run("returns the same when the cache hasn't been updated", func(t *testing.T) {
			// given
			newConfig := newToolchainConfigWithReset(t, config.AutomaticApproval().MaxUsersNumber(666))
			cl := test.NewFakeClient(t, newConfig)

			// when
			actual, err := GetConfig(cl, test.HostOperatorNs)

			// then
			require.NoError(t, err)
			assert.Equal(t, 123, actual.AutomaticApproval().MaxNumberOfUsersOverall())
			assert.Equal(t, cfg.Spec.Host.AutomaticApproval.MaxNumberOfUsers.SpecificPerMemberCluster, actual.AutomaticApproval().MaxNumberOfUsersSpecificPerMemberCluster())
		})

		t.Run("returns the new config when the cache was updated", func(t *testing.T) {
			// given
			newConfig := newToolchainConfigWithReset(t, config.AutomaticApproval().MaxUsersNumber(666), config.Deactivation().DeactivatingNotificationDays(5))
			cl := test.NewFakeClient(t)

			// when
			updateConfig(newConfig)

			// then
			actual, err := GetConfig(cl, test.HostOperatorNs)
			require.NoError(t, err)
			assert.Equal(t, 666, actual.AutomaticApproval().MaxNumberOfUsersOverall())
			assert.Empty(t, actual.AutomaticApproval().MaxNumberOfUsersSpecificPerMemberCluster())
			assert.Equal(t, 5, actual.Deactivation().DeactivatingNotificationInDays())
		})
	})
}

func TestGetConfigFailed(t *testing.T) {
	// given
	t.Run("config not found", func(t *testing.T) {
		cfg := newToolchainConfigWithReset(t, config.AutomaticApproval().MaxUsersNumber(123, config.PerMemberCluster("member1", 321)))
		cl := test.NewFakeClient(t, cfg)
		cl.MockGet = func(ctx context.Context, key client.ObjectKey, obj runtime.Object) error {
			return apierrors.NewNotFound(schema.GroupResource{}, "config")
		}

		// when
		defaultConfig, err := GetConfig(cl, test.HostOperatorNs)

		// then
		require.NoError(t, err)
		assert.Equal(t, 1000, defaultConfig.AutomaticApproval().MaxNumberOfUsersOverall())
		assert.Empty(t, defaultConfig.AutomaticApproval().MaxNumberOfUsersSpecificPerMemberCluster())

	})

	t.Run("error getting config", func(t *testing.T) {
		config := newToolchainConfigWithReset(t, config.AutomaticApproval().MaxUsersNumber(123, config.PerMemberCluster("member1", 321)))
		cl := test.NewFakeClient(t, config)
		cl.MockGet = func(ctx context.Context, key client.ObjectKey, obj runtime.Object) error {
			return fmt.Errorf("some error")
		}

		// when
		defaultConfig, err := GetConfig(cl, test.HostOperatorNs)

		// then
		require.Error(t, err)
		assert.Equal(t, 1000, defaultConfig.AutomaticApproval().MaxNumberOfUsersOverall())
		assert.Empty(t, defaultConfig.AutomaticApproval().MaxNumberOfUsersSpecificPerMemberCluster())

	})
}

func TestMultipleExecutionsInParallel(t *testing.T) {
	// given
	var latch sync.WaitGroup
	latch.Add(1)
	var waitForFinished sync.WaitGroup
	initconfig := newToolchainConfigWithReset(t, config.AutomaticApproval().MaxUsersNumber(1, config.PerMemberCluster("member", 1)))
	cl := test.NewFakeClient(t, initconfig)

	for i := 0; i < 1000; i++ {
		waitForFinished.Add(2)
		go func() {
			defer waitForFinished.Done()
			latch.Wait()

			// when
			config, err := GetConfig(cl, test.HostOperatorNs)

			// then
			require.NoError(t, err)
			assert.NotEmpty(t, config.AutomaticApproval().MaxNumberOfUsersOverall())
			assert.NotEmpty(t, config.AutomaticApproval().MaxNumberOfUsersSpecificPerMemberCluster())
		}()
		go func(i int) {
			defer waitForFinished.Done()
			latch.Wait()
			config := newToolchainConfigWithReset(t, config.AutomaticApproval().MaxUsersNumber(i+1, config.PerMemberCluster(fmt.Sprintf("member%d", i), i)))
			updateConfig(config)
		}(i)
	}

	// when
	latch.Done()
	waitForFinished.Wait()
	config, err := GetConfig(test.NewFakeClient(t), test.HostOperatorNs)

	// then
	require.NoError(t, err)
	assert.NotEmpty(t, config.AutomaticApproval().MaxNumberOfUsersOverall())
	assert.NotEmpty(t, config.AutomaticApproval().MaxNumberOfUsersSpecificPerMemberCluster())
}
