package toolchainconfig_test

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/codeready-toolchain/host-operator/controllers/toolchainconfig"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	. "github.com/codeready-toolchain/toolchain-common/pkg/test"
	testconfig "github.com/codeready-toolchain/toolchain-common/pkg/test/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestCache(t *testing.T) {
	// given
	os.Setenv("WATCH_NAMESPACE", test.HostOperatorNs)
	cl := NewFakeClient(t)

	// when
	defaultConfig, err := toolchainconfig.GetConfig(cl)

	// then
	require.NoError(t, err)
	assert.Equal(t, 1000, defaultConfig.AutomaticApproval().MaxNumberOfUsersOverall())
	assert.Empty(t, defaultConfig.AutomaticApproval().MaxNumberOfUsersSpecificPerMemberCluster())

	t.Run("return config that is stored in client", func(t *testing.T) {
		// given
		config := newToolchainConfigWithReset(t, testconfig.AutomaticApproval().MaxNumberOfUsers(123, testconfig.PerMemberCluster("member1", 321)))
		cl := NewFakeClient(t, config)

		// when
		actual, err := toolchainconfig.GetConfig(cl)

		// then
		require.NoError(t, err)
		assert.Equal(t, 123, actual.AutomaticApproval().MaxNumberOfUsersOverall())
		assert.Equal(t, config.Spec.Host.AutomaticApproval.MaxNumberOfUsers.SpecificPerMemberCluster, actual.AutomaticApproval().MaxNumberOfUsersSpecificPerMemberCluster())

		t.Run("returns the same when the cache hasn't been updated", func(t *testing.T) {
			// given
			newConfig := newToolchainConfigWithReset(t, testconfig.AutomaticApproval().MaxNumberOfUsers(666))
			cl := NewFakeClient(t, newConfig)

			// when
			actual, err := toolchainconfig.GetConfig(cl)

			// then
			require.NoError(t, err)
			assert.Equal(t, 123, actual.AutomaticApproval().MaxNumberOfUsersOverall())
			assert.Equal(t, config.Spec.Host.AutomaticApproval.MaxNumberOfUsers.SpecificPerMemberCluster, actual.AutomaticApproval().MaxNumberOfUsersSpecificPerMemberCluster())
		})

		t.Run("returns the new config when the cache was updated", func(t *testing.T) {
			// given
			newConfig := newToolchainConfigWithReset(t, testconfig.AutomaticApproval().MaxNumberOfUsers(666), testconfig.Deactivation().DeactivatingNotificationDays(5))
			cl := NewFakeClient(t)

			// when
			toolchainconfig.UpdateConfig(newConfig)

			// then
			actual, err := toolchainconfig.GetConfig(cl)
			require.NoError(t, err)
			assert.Equal(t, 666, actual.AutomaticApproval().MaxNumberOfUsersOverall())
			assert.Empty(t, actual.AutomaticApproval().MaxNumberOfUsersSpecificPerMemberCluster())
			assert.Equal(t, 5, actual.Deactivation().DeactivatingNotificationDays())
		})
	})
}

func TestGetConfigFailed(t *testing.T) {
	// given
	t.Run("config not found", func(t *testing.T) {
		config := newToolchainConfigWithReset(t, testconfig.AutomaticApproval().MaxNumberOfUsers(123, testconfig.PerMemberCluster("member1", 321)))
		cl := NewFakeClient(t, config)
		cl.MockGet = func(ctx context.Context, key client.ObjectKey, obj runtime.Object) error {
			return apierrors.NewNotFound(schema.GroupResource{}, "config")
		}

		// when
		defaultConfig, err := toolchainconfig.GetConfig(cl)

		// then
		require.NoError(t, err)
		assert.Equal(t, 1000, defaultConfig.AutomaticApproval().MaxNumberOfUsersOverall())
		assert.Empty(t, defaultConfig.AutomaticApproval().MaxNumberOfUsersSpecificPerMemberCluster())

	})

	t.Run("error getting config", func(t *testing.T) {
		config := newToolchainConfigWithReset(t, testconfig.AutomaticApproval().MaxNumberOfUsers(123, testconfig.PerMemberCluster("member1", 321)))
		cl := NewFakeClient(t, config)
		cl.MockGet = func(ctx context.Context, key client.ObjectKey, obj runtime.Object) error {
			return fmt.Errorf("some error")
		}

		// when
		defaultConfig, err := toolchainconfig.GetConfig(cl)

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
	initconfig := newToolchainConfigWithReset(t, testconfig.AutomaticApproval().MaxNumberOfUsers(1, testconfig.PerMemberCluster("member", 1)))
	cl := NewFakeClient(t, initconfig)

	for i := 0; i < 1000; i++ {
		waitForFinished.Add(2)
		go func() {
			defer waitForFinished.Done()
			latch.Wait()

			// when
			config, err := toolchainconfig.GetConfig(cl)

			// then
			require.NoError(t, err)
			assert.NotEmpty(t, config.AutomaticApproval().MaxNumberOfUsersOverall())
			assert.NotEmpty(t, config.AutomaticApproval().MaxNumberOfUsersSpecificPerMemberCluster())
		}()
		go func(i int) {
			defer waitForFinished.Done()
			latch.Wait()
			config := newToolchainConfigWithReset(t, testconfig.AutomaticApproval().MaxNumberOfUsers(i+1, testconfig.PerMemberCluster(fmt.Sprintf("member%d", i), i)))
			toolchainconfig.UpdateConfig(config)
		}(i)
	}

	// when
	latch.Done()
	waitForFinished.Wait()
	config, err := toolchainconfig.GetConfig(NewFakeClient(t))

	// then
	require.NoError(t, err)
	assert.NotEmpty(t, config.AutomaticApproval().MaxNumberOfUsersOverall())
	assert.NotEmpty(t, config.AutomaticApproval().MaxNumberOfUsersSpecificPerMemberCluster())
}
