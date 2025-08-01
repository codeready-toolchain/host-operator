package usertiers_test

import (
	"context"
	"testing"

	"github.com/gofrs/uuid"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	"github.com/codeready-toolchain/host-operator/pkg/templates/usertiers"
	commontest "github.com/codeready-toolchain/toolchain-common/pkg/test"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

const (
	testRoot = "testtemplates/testusertiers"
)

func TestCreateOrUpdateResources(t *testing.T) {
	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)
	logf.SetLogger(zap.New(zap.UseDevMode(true)))

	t.Run("ok", func(t *testing.T) {
		t.Run("create only", func(t *testing.T) {
			// given
			namespace := "host-operator" + uuid.Must(uuid.NewV4()).String()[:7]
			clt := commontest.NewFakeClient(t)
			// verify that no UserTier resources exist prior to creation
			userTiers := toolchainv1alpha1.UserTierList{}
			err = clt.List(context.TODO(), &userTiers, runtimeclient.InNamespace(namespace))
			require.NoError(t, err)
			require.Empty(t, userTiers.Items)

			// when
			err := usertiers.CreateOrUpdateResources(context.TODO(), s, clt, namespace, usertiers.TestUserTierTemplatesFS, testRoot)

			// then
			require.NoError(t, err)

			// verify that 4 UserTier CRs were created:
			for _, tierName := range []string{"advanced", "base"} {
				tier := toolchainv1alpha1.UserTier{}
				err = clt.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: tierName}, &tier)
				require.NoError(t, err)
				assert.Equal(t, int64(1), tier.Generation)

				switch tierName {
				case "base":
					assert.Equal(t, 30, tier.Spec.DeactivationTimeoutDays)
				case "advanced":
					assert.Equal(t, 0, tier.Spec.DeactivationTimeoutDays)
				default:
					require.Fail(t, "found unexpected tier", "tier '%s' found but not handled", tierName)
				}
			}
		})

		t.Run("create then update with same tier templates", func(t *testing.T) {
			// given
			namespace := "host-operator" + uuid.Must(uuid.NewV4()).String()[:7]
			clt := commontest.NewFakeClient(t)

			// when
			err := usertiers.CreateOrUpdateResources(context.TODO(), s, clt, namespace, usertiers.TestUserTierTemplatesFS, testRoot)
			require.NoError(t, err)

			// when calling CreateOrUpdateResources a second time
			err = usertiers.CreateOrUpdateResources(context.TODO(), s, clt, namespace, usertiers.TestUserTierTemplatesFS, testRoot)

			// then
			require.NoError(t, err)

			// verify that 4 UserTier CRs were created:
			for _, tierName := range []string{"advanced", "base"} {
				tier := toolchainv1alpha1.UserTier{}
				err = clt.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: tierName}, &tier)
				require.NoError(t, err)
				assert.Equal(t, int64(1), tier.Generation)

				switch tierName {
				case "advanced":
					assert.Equal(t, 0, tier.Spec.DeactivationTimeoutDays)
				case "base":
					assert.Equal(t, 30, tier.Spec.DeactivationTimeoutDays)
				default:
					require.Fail(t, "found unexpected tier", "tier '%s' found but not handled", tierName)
				}
			}
		})
	})

	t.Run("failures", func(t *testing.T) {
		namespace := "host-operator" + uuid.Must(uuid.NewV4()).String()[:7]

		t.Run("usertiers", func(t *testing.T) {
			t.Run("failed to patch usertiers", func(t *testing.T) {
				// given
				clt := commontest.NewFakeClient(t)
				clt.MockPatch = func(ctx context.Context, obj runtimeclient.Object, patch runtimeclient.Patch, opts ...runtimeclient.PatchOption) error {
					if obj.GetObjectKind().GroupVersionKind().Kind == "UserTier" {
						// simulate a client/server error
						return errors.Errorf("an error")
					}
					return clt.Client.Patch(ctx, obj, patch, opts...)
				}
				// when
				err := usertiers.CreateOrUpdateResources(context.TODO(), s, clt, namespace, usertiers.TestUserTierTemplatesFS, testRoot)
				// then
				require.Error(t, err)
				assert.Regexp(t, "unable to create UserTiers: unable to patch the '\\w+' UserTier: unable to patch 'toolchain\\.dev\\.openshift\\.com\\/v1alpha1, Kind=UserTier' called '\\w+' in namespace '[a-z0-9-]+': an error", err.Error())
			})
		})
	})
}
