package toolchainconfig

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	testconfig "github.com/codeready-toolchain/toolchain-common/pkg/test/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestCache(t *testing.T) {
	// given
	os.Setenv("WATCH_NAMESPACE", test.HostOperatorNs)
	cl := test.NewFakeClient(t)

	// when
	defaultConfig, err := GetConfig(cl)

	// then
	require.NoError(t, err)
	assert.Equal(t, 1000, defaultConfig.AutomaticApproval().MaxNumberOfUsersOverall())
	assert.Empty(t, defaultConfig.AutomaticApproval().MaxNumberOfUsersSpecificPerMemberCluster())

	t.Run("return config that is stored in cache", func(t *testing.T) {
		// given
		config := NewToolchainConfigWithReset(t, testconfig.AutomaticApproval().MaxNumberOfUsers(123, testconfig.PerMemberCluster("member1", 321)))
		cl := test.NewFakeClient(t, config)

		// when
		actual, err := GetConfig(cl)

		// then
		require.NoError(t, err)
		assert.Equal(t, 123, actual.AutomaticApproval().MaxNumberOfUsersOverall())
		assert.Equal(t, config.Spec.Host.AutomaticApproval.MaxNumberOfUsers.SpecificPerMemberCluster, actual.AutomaticApproval().MaxNumberOfUsersSpecificPerMemberCluster())

		t.Run("returns the same when the cache hasn't been updated", func(t *testing.T) {
			// given
			newConfig := NewToolchainConfigWithReset(t, testconfig.AutomaticApproval().MaxNumberOfUsers(666))
			cl := test.NewFakeClient(t, newConfig)

			// when
			actual, err := GetConfig(cl)

			// then
			require.NoError(t, err)
			assert.Equal(t, 123, actual.AutomaticApproval().MaxNumberOfUsersOverall())
			assert.Equal(t, config.Spec.Host.AutomaticApproval.MaxNumberOfUsers.SpecificPerMemberCluster, actual.AutomaticApproval().MaxNumberOfUsersSpecificPerMemberCluster())
		})

		t.Run("returns the new config when the cache was updated", func(t *testing.T) {
			// given
			newConfig := NewToolchainConfigWithReset(t,
				testconfig.AutomaticApproval().MaxNumberOfUsers(666),
				testconfig.Deactivation().DeactivatingNotificationDays(5),
				testconfig.Notifications().Secret().
					Ref("notification-secret").
					MailgunAPIKey("mailgunAPIKey"),
			)
			cl := test.NewFakeClient(t)
			secretData := map[string]map[string]string{
				"notification-secret": {
					"mailgunAPIKey": "abc123",
				},
			}

			// when
			updateConfig(newConfig, secretData)

			// then
			actual, err := GetConfig(cl)
			require.NoError(t, err)
			assert.Equal(t, 666, actual.AutomaticApproval().MaxNumberOfUsersOverall())
			assert.Empty(t, actual.AutomaticApproval().MaxNumberOfUsersSpecificPerMemberCluster())
			assert.Equal(t, 5, actual.Deactivation().DeactivatingNotificationDays())
			assert.Equal(t, "abc123", actual.Notifications().MailgunAPIKey()) // secret value
		})
	})
}

func TestGetConfigFailed(t *testing.T) {
	// given
	t.Run("config not found", func(t *testing.T) {
		config := NewToolchainConfigWithReset(t, testconfig.AutomaticApproval().MaxNumberOfUsers(123, testconfig.PerMemberCluster("member1", 321)))
		cl := test.NewFakeClient(t, config)
		cl.MockGet = func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
			return apierrors.NewNotFound(schema.GroupResource{}, "config")
		}

		// when
		defaultConfig, err := GetConfig(cl)

		// then
		require.NoError(t, err)
		assert.Equal(t, 1000, defaultConfig.AutomaticApproval().MaxNumberOfUsersOverall())
		assert.Empty(t, defaultConfig.AutomaticApproval().MaxNumberOfUsersSpecificPerMemberCluster())

	})

	t.Run("error getting config", func(t *testing.T) {
		config := NewToolchainConfigWithReset(t, testconfig.AutomaticApproval().MaxNumberOfUsers(123, testconfig.PerMemberCluster("member1", 321)))
		cl := test.NewFakeClient(t, config)
		cl.MockGet = func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
			return fmt.Errorf("some error")
		}

		// when
		defaultConfig, err := GetConfig(cl)

		// then
		require.Error(t, err)
		assert.Equal(t, 1000, defaultConfig.AutomaticApproval().MaxNumberOfUsersOverall())
		assert.Empty(t, defaultConfig.AutomaticApproval().MaxNumberOfUsersSpecificPerMemberCluster())
	})

	t.Run("load secrets error", func(t *testing.T) {
		config := NewToolchainConfigWithReset(t, testconfig.AutomaticApproval().MaxNumberOfUsers(123, testconfig.PerMemberCluster("member1", 321)))
		// given
		cl := test.NewFakeClient(t, config)
		cl.MockList = func(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
			return fmt.Errorf("list error")
		}

		// when
		err := loadLatest(cl)

		// then
		require.EqualError(t, err, "list error")
	})
}

func TestLoadLatest(t *testing.T) {
	t.Run("config found", func(t *testing.T) {
		initconfig := NewToolchainConfigWithReset(t, testconfig.AutomaticApproval().MaxNumberOfUsers(1100))
		// given
		cl := test.NewFakeClient(t, initconfig)

		// when
		err := loadLatest(cl)

		// then
		require.NoError(t, err)
		actual, err := GetConfig(cl)
		require.NoError(t, err)
		assert.Equal(t, 1100, actual.AutomaticApproval().MaxNumberOfUsersOverall())

		t.Run("returns the same when the config hasn't been updated", func(t *testing.T) {
			// when
			err := loadLatest(cl)

			// then
			require.NoError(t, err)
			actual, err = GetConfig(cl)
			require.NoError(t, err)
			assert.Equal(t, 1100, actual.AutomaticApproval().MaxNumberOfUsersOverall())
		})

		t.Run("returns the new value when the config has been updated", func(t *testing.T) {
			// get
			changedConfig := NewToolchainConfigWithReset(t, testconfig.AutomaticApproval().MaxNumberOfUsers(2000))
			err := cl.Update(context.TODO(), changedConfig)
			require.NoError(t, err)

			// when
			err = loadLatest(cl)

			// then
			require.NoError(t, err)
			actual, err = GetConfig(cl)
			require.NoError(t, err)
			assert.Equal(t, 2000, actual.AutomaticApproval().MaxNumberOfUsersOverall())
		})
	})

	t.Run("config not found", func(t *testing.T) {
		// given
		cl := test.NewFakeClient(t)

		// when
		err := loadLatest(cl)

		// then
		require.NoError(t, err)
	})

	t.Run("get config error", func(t *testing.T) {
		initconfig := NewToolchainConfigWithReset(t, testconfig.AutomaticApproval().MaxNumberOfUsers(100))
		// given
		cl := test.NewFakeClient(t, initconfig)
		cl.MockGet = func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
			return fmt.Errorf("get error")
		}

		// when
		err := loadLatest(cl)

		// then
		require.EqualError(t, err, "get error")
	})

	t.Run("load secrets error", func(t *testing.T) {
		initconfig := NewToolchainConfigWithReset(t, testconfig.AutomaticApproval().MaxNumberOfUsers(100))
		// given
		cl := test.NewFakeClient(t, initconfig)
		cl.MockList = func(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
			return fmt.Errorf("list error")
		}

		// when
		err := loadLatest(cl)

		// then
		require.EqualError(t, err, "list error")
	})
}

func TestMultipleExecutionsInParallel(t *testing.T) {
	// given
	var latch sync.WaitGroup
	latch.Add(1)
	var waitForFinished sync.WaitGroup
	initconfig := NewToolchainConfigWithReset(t, testconfig.AutomaticApproval().MaxNumberOfUsers(1, testconfig.PerMemberCluster("member", 1)))
	cl := test.NewFakeClient(t, initconfig)

	for i := 0; i < 1000; i++ {
		waitForFinished.Add(2)
		go func() {
			defer waitForFinished.Done()
			latch.Wait()

			// when
			config, err := GetConfig(cl)

			// then
			require.NoError(t, err)
			assert.NotEmpty(t, config.AutomaticApproval().MaxNumberOfUsersOverall())
			assert.NotEmpty(t, config.AutomaticApproval().MaxNumberOfUsersSpecificPerMemberCluster())
		}()
		go func(i int) {
			defer waitForFinished.Done()
			latch.Wait()
			config := NewToolchainConfigWithReset(t, testconfig.AutomaticApproval().MaxNumberOfUsers(i+1, testconfig.PerMemberCluster(fmt.Sprintf("member%d", i), i)))
			updateConfig(config, map[string]map[string]string{})
		}(i)
	}

	// when
	latch.Done()
	waitForFinished.Wait()
	config, err := GetConfig(test.NewFakeClient(t))

	// then
	require.NoError(t, err)
	assert.NotEmpty(t, config.AutomaticApproval().MaxNumberOfUsersOverall())
	assert.NotEmpty(t, config.AutomaticApproval().MaxNumberOfUsersSpecificPerMemberCluster())
}

func newSecret(name string, data map[string][]byte) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: test.HostOperatorNs,
		},
		Data: data,
	}
}
