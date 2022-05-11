package usertiers

import (
	"bytes"
	"fmt"
	"testing"
	texttemplate "text/template"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	"github.com/codeready-toolchain/host-operator/pkg/templates/assets"
	testusertiers "github.com/codeready-toolchain/host-operator/test/templates/usertiers"

	"github.com/gofrs/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/kubernetes/scheme"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var expectedProdTiers = []string{
	"nodeactivation",
	"deactivate30",
	"deactivate80",
	"deactivate90",
	"deactivate180",
	"deactivate365",
}

var expectedTestTiers = []string{
	"advanced",
	"base",
}

func TestLoadTemplatesByTiers(t *testing.T) {

	logf.SetLogger(zap.New(zap.UseDevMode(true)))

	t.Run("ok", func(t *testing.T) {

		t.Run("with prod assets", func(t *testing.T) {
			// given
			assets := assets.NewAssets(AssetNames, Asset)
			// when
			tmpls, err := loadTemplatesByTiers(assets)
			// then
			require.NoError(t, err)
			require.Len(t, tmpls, len(expectedProdTiers))
			require.NotContains(t, "foo", tmpls) // make sure that the `foo: bar` entry was ignored
			for _, tier := range expectedProdTiers {
				t.Run(tier, func(t *testing.T) {
					t.Run("tier.yaml", func(t *testing.T) {
						_, found := tmpls[tier]
						require.Truef(t, found, "did not find expected tier '%s'", tier)
						require.NotNil(t, tmpls[tier].rawTemplates.userTier)
						assert.NotEmpty(t, tmpls[tier].rawTemplates.userTier.content)
					})
				})
			}
		})

		t.Run("with test assets", func(t *testing.T) {
			// given
			assets := assets.NewAssets(testusertiers.AssetNames, testusertiers.Asset)
			// when
			tmpls, err := loadTemplatesByTiers(assets)
			// then
			require.NoError(t, err)
			require.Len(t, tmpls, 2)
			require.NotContains(t, "foo", tmpls) // make sure that the `foo: bar` entry was ignored

			for _, tier := range expectedTestTiers {
				t.Run(tier, func(t *testing.T) {
					t.Run("tier.yaml", func(t *testing.T) {
						require.NotNil(t, tmpls[tier].rawTemplates.userTier)
						switch tier {
						case "advanced":
							assert.NotEmpty(t, tmpls[tier].rawTemplates.userTier.content)
						case "base":
							assert.NotEmpty(t, tmpls[tier].rawTemplates.userTier.content)
						default:
							require.Fail(t, "found unexpected tier", "tier '%s' found but not handled", tier)
						}

					})
				})
			}
		})
	})

	t.Run("failures", func(t *testing.T) {

		t.Run("missing asset", func(t *testing.T) {
			// given
			fakeAssets := func(name string) ([]byte, error) {
				return nil, fmt.Errorf("an error occurred")
			}
			assets := assets.NewAssets(testusertiers.AssetNames, fakeAssets)
			// when
			_, err := loadTemplatesByTiers(assets)
			// then
			require.Error(t, err)
			assert.Contains(t, err.Error(), "unable to load templates: an error occurred")
		})

		t.Run("invalid name format", func(t *testing.T) {
			// given
			fakeAssetNames := func() []string {
				return []string{`.DS_Store`} // '/advanced/foo.yaml' is not a valid filename
			}
			fakeAssets := func(name string) ([]byte, error) {
				switch name {
				case ".DS_Store":
					return []byte(`foo:bar`), nil // just make sure the asset exists
				default:
					return testusertiers.Asset(name)
				}
			}
			assets := assets.NewAssets(fakeAssetNames, fakeAssets)
			// when
			_, err := loadTemplatesByTiers(assets)
			// then
			require.Error(t, err)
			assert.EqualError(t, err, "unable to load templates: invalid name format for file '.DS_Store'")
		})

		t.Run("invalid filename scope", func(t *testing.T) {
			// given
			fakeAssetNames := func() []string {
				return []string{`advanced/foo.yaml`} // '/advanced/foo.yaml' is not a valid filename
			}
			fakeAssets := func(name string) ([]byte, error) {
				switch name {
				case "advanced/foo.yaml":
					return []byte(`foo:bar`), nil // just make sure the asset exists
				default:
					return testusertiers.Asset(name)
				}
			}
			assets := assets.NewAssets(fakeAssetNames, fakeAssets)
			// when
			_, err := loadTemplatesByTiers(assets)
			// then
			require.Error(t, err)
			assert.Contains(t, err.Error(), "unable to load templates: unknown scope for file 'advanced/foo.yaml'")
		})
	})
}

func TestNewUserTier(t *testing.T) {

	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)

	t.Run("ok", func(t *testing.T) {

		t.Run("with prod assets", func(t *testing.T) {
			// given
			namespace := "host-operator-" + uuid.Must(uuid.NewV4()).String()[:7]
			assets := assets.NewAssets(AssetNames, Asset)
			// when
			tc, err := newUserTierGenerator(s, nil, namespace, assets)
			require.NoError(t, err)
			// then
			require.Len(t, tc.templatesByTier, len(expectedProdTiers))
			for name, tierData := range tc.templatesByTier {
				// tierData, found := tc.templatesByTier[name]
				tierObjs := tierData.objects
				require.Len(t, tierObjs, 1, "expected only 1 UserTier toolchain object")
				tier := runtimeObjectToUserTier(t, s, tierObjs[0])

				// require.True(t, found)
				assert.Equal(t, name, tier.ObjectMeta.Name)
				assert.Equal(t, namespace, tier.ObjectMeta.Namespace)

				switch name {
				case "nodeactivation":
					assert.Equal(t, 0, tier.Spec.DeactivationTimeoutDays)
				case "deactivate30":
					assert.Equal(t, 30, tier.Spec.DeactivationTimeoutDays)
				case "deactivate80":
					assert.Equal(t, 80, tier.Spec.DeactivationTimeoutDays)
				case "deactivate90":
					assert.Equal(t, 90, tier.Spec.DeactivationTimeoutDays)
				case "deactivate180":
					assert.Equal(t, 180, tier.Spec.DeactivationTimeoutDays)
				case "deactivate365":
					assert.Equal(t, 365, tier.Spec.DeactivationTimeoutDays)
				default:
					require.Fail(t, "found unexpected tier", "tier '%s' found but not handled", tier.Name)
				}
			}
		})

		t.Run("with test assets", func(t *testing.T) {
			// given
			namespace := "host-operator-" + uuid.Must(uuid.NewV4()).String()[:7]
			assets := assets.NewAssets(testusertiers.AssetNames, testusertiers.Asset)
			tc, err := newUserTierGenerator(s, nil, namespace, assets)
			require.NoError(t, err)

			for _, tier := range expectedTestTiers {
				t.Run(tier, func(t *testing.T) {
					// given
					objects := tc.templatesByTier[tier].objects
					require.Len(t, objects, 1, "expected only 1 UserTier toolchain object")
					// when
					actual := runtimeObjectToUserTier(t, s, objects[0])

					// then
					deactivationTimeout := 30
					if tier == "advanced" {
						deactivationTimeout = 0
					}
					expected, err := newUserTierFromYAML(s, tier, namespace, deactivationTimeout)
					require.NoError(t, err)
					// here we don't compare objects because the generated UserTier
					// has no specific values for the `TypeMeta`: the `APIVersion: toolchain.dev.openshift.com/v1alpha1`
					// and `Kind: NSTemplateTier` should be set by the client using the registered GVK
					assert.Equal(t, expected.ObjectMeta, actual.ObjectMeta)
					assert.Equal(t, expected.Spec, actual.Spec)
				})
			}
		})
	})
}

// newUserTierFromYAML generates toolchainv1alpha1.UserTier using a golang template which is applied to the given tier.
func newUserTierFromYAML(s *runtime.Scheme, tier, namespace string, deactivationTimeout int) (*toolchainv1alpha1.UserTier, error) {
	expectedTmpl, err := texttemplate.New("template").Parse(`
{{ $tier := .Tier}}
kind: UserTier
apiVersion: toolchain.dev.openshift.com/v1alpha1
metadata:
  namespace: {{ .Namespace }}
  name: {{ .Tier }}
spec:
  deactivationTimeoutDays: {{ .DeactivationTimeout }} 
`)
	if err != nil {
		return nil, err
	}
	buf := bytes.NewBuffer(nil)
	err = expectedTmpl.Execute(buf, struct {
		Tier                string
		Namespace           string
		DeactivationTimeout int
	}{
		Tier:                tier,
		Namespace:           namespace,
		DeactivationTimeout: deactivationTimeout,
	})
	if err != nil {
		return nil, err
	}
	result := &toolchainv1alpha1.UserTier{}
	codecFactory := serializer.NewCodecFactory(s)
	_, _, err = codecFactory.UniversalDeserializer().Decode(buf.Bytes(), nil, result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func runtimeObjectToUserTier(t *testing.T, s *runtime.Scheme, tierObj runtime.Object) *toolchainv1alpha1.UserTier {
	tier := &toolchainv1alpha1.UserTier{}
	err := s.Convert(tierObj, tier, nil)
	require.NoError(t, err)
	return tier
}
