package nstemplatetiers_test

import (
	"context"
	"fmt"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	"github.com/codeready-toolchain/host-operator/pkg/templates/nstemplatetiers"
	testclusterresources "github.com/codeready-toolchain/host-operator/test/templates/nstemplatetiers/clusterresources"
	testnamespaces "github.com/codeready-toolchain/host-operator/test/templates/nstemplatetiers/namespaces"
	testsupport "github.com/codeready-toolchain/toolchain-common/pkg/test"

	"github.com/pkg/errors"
	uuid "github.com/satori/go.uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestCreateOrUpdateResources(t *testing.T) {

	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)

	t.Run("ok", func(t *testing.T) {

		t.Run("create only", func(t *testing.T) {
			// given
			namespace := "host-operator" + uuid.NewV4().String()[:7]
			clt := testsupport.NewFakeClient(t)
			// set the 'generation' when creating the object
			clt.MockCreate = func(ctx context.Context, obj runtime.Object, opts ...client.CreateOption) error {
				if obj, ok := obj.(*toolchainv1alpha1.NSTemplateTier); ok {
					if obj.ObjectMeta.Generation != 0 {
						return errors.Errorf("'generation' field will be specified by the server during object creation: %v", obj.ObjectMeta.Generation)
					}
					obj.ObjectMeta.Generation = obj.ObjectMeta.Generation + 1
				}
				return clt.Client.Create(ctx, obj)
			}
			// verify that no "advanced" nor "basic" nor "team" NSTemplateTier resources exist prior to creation
			for _, tierName := range []string{"advanced", "basic", "team"} {
				tier := toolchainv1alpha1.NSTemplateTier{}
				err = clt.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: tierName}, &tier)
				require.Error(t, err)
				assert.IsType(t, metav1.StatusReasonNotFound, apierrors.ReasonForError(err))
			}

			// when
			err := nstemplatetiers.CreateOrUpdateResources(s, clt, namespace, testnamespaces.Asset, testclusterresources.Asset)

			// then
			require.NoError(t, err)
			// verify that 3 NSTemplateTier CRs were created: "advanced", "basic", and "team"
			for _, tierName := range []string{"advanced", "basic", "team"} {
				tier := toolchainv1alpha1.NSTemplateTier{}
				err = clt.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: tierName}, &tier)
				require.NoError(t, err)
				assert.Equal(t, int64(1), tier.ObjectMeta.Generation)
				if tier.Name == "team" {
					assert.Nil(t, tier.Spec.ClusterResources) // "team" tier should not have cluster resources set
				} else {
					assert.NotNil(t, tier.Spec.ClusterResources)
				}
			}
		})

		t.Run("create and update", func(t *testing.T) {
			// given
			namespace := "host-operator" + uuid.NewV4().String()[:7]
			clt := testsupport.NewFakeClient(t)
			// set the 'generation' when creating the object
			clt.MockCreate = func(ctx context.Context, obj runtime.Object, opts ...client.CreateOption) error {
				if tier, ok := obj.(*toolchainv1alpha1.NSTemplateTier); ok {
					if tier.ObjectMeta.Generation != 0 {
						return errors.Errorf("'generation' field will be specified by the server during object creation: %v", tier.ObjectMeta.Generation)
					}
					tier.ObjectMeta.Generation = tier.ObjectMeta.Generation + 1
					tier.ObjectMeta.ResourceVersion = "foo" // set by the server
					err := clt.Client.Create(ctx, obj)
					if err != nil && apierrors.IsAlreadyExists(err) {
						// prevent the fake client to return the actual ResourceVersion,
						// as it seems like the real client/server don't not do it in the e2e tests
						// see client.MockUpdate just underneath for the use-case
						tier.ResourceVersion = ""
					}
					return err
				}
				return errors.Errorf("did not expect to create a resource of type %T", obj)
			}
			// check the 'generation' when updating the object, and increment its value
			clt.MockUpdate = func(ctx context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
				if obj, ok := obj.(*toolchainv1alpha1.NSTemplateTier); ok {
					obj.ObjectMeta.Generation = obj.ObjectMeta.Generation + 1
					if obj.ObjectMeta.ResourceVersion != "foo" {
						// here, we expect that the caller will have set the ResourceVersion based on the existing object
						return errors.Errorf("'ResourceVersion ' field must be specified during object update")
					}
				}
				return clt.Client.Update(ctx, obj)
			}
			fakeAssets, revisions := generateRevisions("123456")
			err := nstemplatetiers.CreateOrUpdateResources(s, clt, namespace, fakeAssets, testclusterresources.Asset)
			require.NoError(t, err)
			// verify that 3 NSTemplateTier CRs were created: "advanced", "basic", and "team"
			for _, tierName := range []string{"advanced", "basic", "team"} {
				tier := toolchainv1alpha1.NSTemplateTier{}
				err = clt.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: tierName}, &tier)
				require.NoError(t, err)
				// verify that the ResourceVersion was set by the server
				assert.Equal(t, "foo", tier.ObjectMeta.ResourceVersion)
				// verify that the generation was set to 1
				assert.Equal(t, int64(1), tier.ObjectMeta.Generation)
				// verify their namespace revisions
				for _, ns := range tier.Spec.Namespaces {
					assert.Equal(t, revisions[tierName][ns.Type], ns.Revision)
				}
			}
			// override the revisions (but let's keep the templates as-is for the sake of simplicity!)
			fakeAssets, revisions = generateRevisions("654321")

			// when calling CreateOrUpdateResources a second time
			err = nstemplatetiers.CreateOrUpdateResources(s, clt, namespace, fakeAssets, testclusterresources.Asset)

			// then
			require.NoError(t, err)
			// verify that the 3 NStemplateTier CRs were updated
			for _, tierName := range []string{"advanced", "basic", "team"} {
				tier := toolchainv1alpha1.NSTemplateTier{}
				err = clt.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: tierName}, &tier)
				require.NoError(t, err)
				// verify that the generation was increased
				assert.Equal(t, int64(2), tier.ObjectMeta.Generation)
				// verify their new namespace revisions
				for _, ns := range tier.Spec.Namespaces {
					assert.Equal(t, revisions[tierName][ns.Type], ns.Revision)
				}
			}
		})
	})

	t.Run("failures", func(t *testing.T) {

		namespace := "host-operator" + uuid.NewV4().String()[:7]
		clt := testsupport.NewFakeClient(t)

		t.Run("failed to generate nstemplatetiers", func(t *testing.T) {

			t.Run("error with namespace assets", func(t *testing.T) {
				// given
				fakeAssets := func(name string) ([]byte, error) {
					if name == "metadata.yaml" {
						return []byte("advanced-code: abcdef"), nil
					}
					// error occurs when fetching the content of the 'advanced-code.yaml' template
					return nil, errors.Errorf("an error")
				}
				// when
				err := nstemplatetiers.CreateOrUpdateResources(s, clt, namespace, fakeAssets, testclusterresources.Asset)
				// then
				require.Error(t, err)
				assert.Equal(t, "unable to create or update NSTemplateTiers: unable to load namespace templates: an error", err.Error())
			})

			t.Run("error with cluster resource quota assets", func(t *testing.T) {
				// given
				fakeAssets := func(name string) ([]byte, error) {
					if name == "metadata.yaml" {
						return []byte("advanced: abcdef"), nil
					}
					// error occurs when fetching the content of the 'advanced.yaml' template
					return nil, errors.Errorf("an error")
				}
				// when
				err := nstemplatetiers.CreateOrUpdateResources(s, clt, namespace, testnamespaces.Asset, fakeAssets)
				// then
				require.Error(t, err)
				assert.Equal(t, "unable to create or update NSTemplateTiers: unable to load cluster resource quota templates: an error", err.Error())
			})
		})

		t.Run("failed to create nstemplatetiers", func(t *testing.T) {
			// given
			clt := testsupport.NewFakeClient(t)
			clt.MockCreate = func(ctx context.Context, obj runtime.Object, opts ...client.CreateOption) error {
				// simulate a client/server error
				return errors.Errorf("an error")
			}
			// when
			err := nstemplatetiers.CreateOrUpdateResources(s, clt, namespace, testnamespaces.Asset, testclusterresources.Asset)
			// then
			require.Error(t, err)
			assert.Equal(t, fmt.Sprintf("unable to create the 'advanced' NSTemplateTiers in namespace '%s': an error", namespace), err.Error())
		})

		t.Run("failed to update nstemplatetiers", func(t *testing.T) {
			// given
			// initialize the client with an existing `advanced` NSTemplatetier
			clt := testsupport.NewFakeClient(t, &toolchainv1alpha1.NSTemplateTier{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: namespace,
					Name:      "advanced",
				},
			})
			clt.MockUpdate = func(ctx context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
				// trigger an error when trying to update the existing `advanced` NSTemplatetier
				return errors.Errorf("an error")
			}
			// when
			err := nstemplatetiers.CreateOrUpdateResources(s, clt, namespace, testnamespaces.Asset, testclusterresources.Asset)
			// then
			require.Error(t, err)
			assert.Equal(t, fmt.Sprintf("unable to update the 'advanced' NSTemplateTiers in namespace '%s': an error", namespace), err.Error())
		})

	})
}

func generateRevisions(prefix string) (func(name string) ([]byte, error), map[string]map[string]string) {
	assets := func(name string) ([]byte, error) {
		if name == "metadata.yaml" {
			return []byte(fmt.Sprintf(`advanced-code: %[1]sa
advanced-dev: %[1]sb
advanced-stage: %[1]sc
basic-code: %[1]sd
basic-dev: %[1]se
basic-stage: %[1]sf
team-dev: %[1]sg
team-stage: %[1]sh
`, prefix)), nil
		}
		return testnamespaces.Asset(name)
	}
	revisions := map[string]map[string]string{
		"advanced": {
			"code":  fmt.Sprintf("%sa", prefix),
			"dev":   fmt.Sprintf("%sb", prefix),
			"stage": fmt.Sprintf("%sc", prefix),
		},
		"basic": {
			"code":  fmt.Sprintf("%sd", prefix),
			"dev":   fmt.Sprintf("%se", prefix),
			"stage": fmt.Sprintf("%sf", prefix),
		},
		"team": {
			"dev":   fmt.Sprintf("%sg", prefix),
			"stage": fmt.Sprintf("%sh", prefix),
		},
	}
	return assets, revisions
}
