package usertiers_test

import (
	"context"
	"testing"

	"github.com/gofrs/uuid"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	"github.com/codeready-toolchain/host-operator/pkg/templates/assets"
	"github.com/codeready-toolchain/host-operator/pkg/templates/usertiers"
	testusertiers "github.com/codeready-toolchain/host-operator/test/templates/usertiers"
	testsupport "github.com/codeready-toolchain/toolchain-common/pkg/test"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func TestCreateOrUpdateResources(t *testing.T) {

	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)
	logf.SetLogger(zap.New(zap.UseDevMode(true)))

	testassets := assets.NewAssets(testusertiers.AssetNames, testusertiers.Asset)

	t.Run("ok", func(t *testing.T) {

		t.Run("create only", func(t *testing.T) {
			// given
			namespace := "host-operator" + uuid.Must(uuid.NewV4()).String()[:7]
			clt := testsupport.NewFakeClient(t)
			// verify that no UserTier resources exist prior to creation
			userTiers := toolchainv1alpha1.UserTierList{}
			err = clt.List(context.TODO(), &userTiers, client.InNamespace(namespace))
			require.NoError(t, err)
			require.Empty(t, userTiers.Items)

			assets := assets.NewAssets(testusertiers.AssetNames, testusertiers.Asset)

			// when
			err := usertiers.CreateOrUpdateResources(s, clt, namespace, assets)

			// then
			require.NoError(t, err)

			// verify that 4 UserTier CRs were created:
			for _, tierName := range []string{"advanced", "base"} {
				tier := toolchainv1alpha1.UserTier{}
				err = clt.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: tierName}, &tier)
				require.NoError(t, err)
				assert.Equal(t, int64(1), tier.ObjectMeta.Generation)

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
			clt := testsupport.NewFakeClient(t)

			// when
			err := usertiers.CreateOrUpdateResources(s, clt, namespace, testassets)
			require.NoError(t, err)

			// when calling CreateOrUpdateResources a second time
			err = usertiers.CreateOrUpdateResources(s, clt, namespace, testassets)

			// then
			require.NoError(t, err)

			// verify that 4 UserTier CRs were created:
			for _, tierName := range []string{"advanced", "base"} {
				tier := toolchainv1alpha1.UserTier{}
				err = clt.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: tierName}, &tier)
				require.NoError(t, err)
				assert.Equal(t, int64(1), tier.ObjectMeta.Generation)

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

		t.Run("failed to read assets", func(t *testing.T) {
			// given
			fakeAssets := assets.NewAssets(testusertiers.AssetNames, func(name string) ([]byte, error) {
				if name == "metadata.yaml" {
					return []byte("base/tier: abcdef"), nil
				}
				// error occurs when fetching the content of the 'tier.yaml' template
				return nil, errors.Errorf("an error")
			})
			clt := testsupport.NewFakeClient(t)
			// when
			err := usertiers.CreateOrUpdateResources(s, clt, namespace, fakeAssets)
			// then
			require.Error(t, err)
			assert.Equal(t, "unable to create UserTier generator: unable to load templates: an error", err.Error()) // error occurred while creating TierTemplate resources
		})

		t.Run("usertiers", func(t *testing.T) {

			t.Run("failed to create usertiers", func(t *testing.T) {
				// given
				clt := testsupport.NewFakeClient(t)
				clt.MockCreate = func(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
					if obj.GetObjectKind().GroupVersionKind().Kind == "UserTier" {
						// simulate a client/server error
						return errors.Errorf("an error")
					}
					return clt.Client.Create(ctx, obj, opts...)
				}
				assets := assets.NewAssets(testusertiers.AssetNames, testusertiers.Asset)
				// when
				err := usertiers.CreateOrUpdateResources(s, clt, namespace, assets)
				// then
				require.Error(t, err)
				assert.Regexp(t, "unable to create UserTiers: unable to create or update the '\\w+' UserTier: unable to create resource of kind: UserTier, version: v1alpha1: an error", err.Error())
			})

			t.Run("failed to update usertiers", func(t *testing.T) {
				// given
				// initialize the client with an existing `advanced` UserTier
				clt := testsupport.NewFakeClient(t, &toolchainv1alpha1.UserTier{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: namespace,
						Name:      "advanced",
					},
				})
				clt.MockUpdate = func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
					if obj.GetObjectKind().GroupVersionKind().Kind == "UserTier" {
						// simulate a client/server error
						return errors.Errorf("an error")
					}
					return clt.Client.Update(ctx, obj, opts...)
				}
				testassets := assets.NewAssets(testusertiers.AssetNames, testusertiers.Asset)
				// when
				err := usertiers.CreateOrUpdateResources(s, clt, namespace, testassets)
				// then
				require.Error(t, err)
				assert.Contains(t, err.Error(), "unable to create UserTiers: unable to create or update the 'advanced' UserTier: unable to create resource of kind: UserTier, version: v1alpha1: unable to update the resource")
			})
		})
	})
}
