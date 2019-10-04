package nstemplatetiers_test

import (
	"context"
	"fmt"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	"github.com/codeready-toolchain/host-operator/pkg/templates/nstemplatetiers"
	testnstemplatetiers "github.com/codeready-toolchain/host-operator/test/templates/nstemplatetiers"
	testsupport "github.com/codeready-toolchain/toolchain-common/pkg/test"

	"github.com/pkg/errors"
	uuid "github.com/satori/go.uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
)

func TestCreateOrUpdateResources(t *testing.T) {

	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)

	t.Run("ok", func(t *testing.T) {

		t.Run("create only", func(t *testing.T) {
			// given
			namespace := "host-operator" + uuid.NewV4().String()[:7]
			client := testsupport.NewFakeClient(t)
			// set the 'resourceVersion' when creating the object
			client.MockCreate = func(ctx context.Context, obj runtime.Object) error {
				if obj, ok := obj.(*toolchainv1alpha1.NSTemplateTier); ok {
					if obj.ObjectMeta.ResourceVersion != "" {
						return errors.Errorf("'resourceVersion' field must be specified by the server during object creation")
					}
					obj.ObjectMeta.ResourceVersion = "foo-" + obj.ObjectMeta.Name
				}
				return client.Client.Create(ctx, obj)
			}
			// verify that no "advanced" nor "basic" NSTemplateTier resources exist prior to creation
			for _, tierName := range []string{"advanced", "basic"} {
				tier := toolchainv1alpha1.NSTemplateTier{}
				err = client.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: tierName}, &tier)
				require.Error(t, err)
				assert.Equal(t, fmt.Sprintf("nstemplatetiers.toolchain.dev.openshift.com \"%s\" not found", tierName), err.Error())
			}

			// when
			err := nstemplatetiers.CreateOrUpdateResources(s, client, namespace, testnstemplatetiers.Asset)

			// then
			require.NoError(t, err)
			// verify that 2 NStemplateTier CRs were created: "advanced" and "basic"
			for _, tierName := range []string{"advanced", "basic"} {
				tier := toolchainv1alpha1.NSTemplateTier{}
				err = client.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: tierName}, &tier)
				require.NoError(t, err)
				assert.Equal(t, "foo-"+tier.ObjectMeta.Name, tier.ObjectMeta.ResourceVersion)
			}
		})

		t.Run("create and update", func(t *testing.T) {
			// given
			namespace := "host-operator" + uuid.NewV4().String()[:7]
			client := testsupport.NewFakeClient(t)
			// set the 'resourceVersion' when creating the object
			client.MockCreate = func(ctx context.Context, obj runtime.Object) error {
				if obj, ok := obj.(*toolchainv1alpha1.NSTemplateTier); ok {
					if obj.ObjectMeta.ResourceVersion != "" {
						return errors.Errorf("'resourceVersion' field must be specified by the server during object creation")
					}
					obj.ObjectMeta.ResourceVersion = "foo-" + obj.ObjectMeta.Name
				}
				return client.Client.Create(ctx, obj)
			}
			// check the 'resourceVersion' when updating the object, and change its value
			client.MockUpdate = func(ctx context.Context, obj runtime.Object) error {
				if obj, ok := obj.(*toolchainv1alpha1.NSTemplateTier); ok {
					if obj.ObjectMeta.ResourceVersion != "foo-"+obj.ObjectMeta.Name {
						return errors.Errorf("a matching 'resourceVersion' field must be returned by the client during object update")
					}
					obj.ObjectMeta.ResourceVersion = "bar-" + obj.ObjectMeta.Name
				}
				return client.Client.Update(ctx, obj)
			}
			assets, revisions := generateRevisions("123456")
			err := nstemplatetiers.CreateOrUpdateResources(s, client, namespace, assets)
			require.NoError(t, err)
			// verify that 2 NStemplateTier CRs were created: "advanced" and "basic"
			for _, tierName := range []string{"advanced", "basic"} {
				tier := toolchainv1alpha1.NSTemplateTier{}
				err = client.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: tierName}, &tier)
				require.NoError(t, err)
				// verify that the resourceVersion was set
				assert.Equal(t, "foo-"+tier.ObjectMeta.Name, tier.ObjectMeta.ResourceVersion)
				// verify their namespace revisions
				for _, ns := range tier.Spec.Namespaces {
					assert.Equal(t, revisions[tierName][ns.Type], ns.Revision)
				}
			}
			// override the revisions (but let's keep the templates as-is for the sake of simplicity!)
			assets, revisions = generateRevisions("654321")

			// when calling CreateOrUpdateResources a second time
			err = nstemplatetiers.CreateOrUpdateResources(s, client, namespace, assets)

			// then
			require.NoError(t, err)
			// verify that the 2 NStemplateTier CRs were updated
			for _, tierName := range []string{"advanced", "basic"} {
				tier := toolchainv1alpha1.NSTemplateTier{}
				err = client.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: tierName}, &tier)
				require.NoError(t, err)
				// verify that the resourceVersion was changed
				assert.Equal(t, "bar-"+tier.ObjectMeta.Name, tier.ObjectMeta.ResourceVersion)
				// verify their new namespace revisions
				for _, ns := range tier.Spec.Namespaces {
					assert.Equal(t, revisions[tierName][ns.Type], ns.Revision)
				}
			}
		})
	})

	t.Run("failures", func(t *testing.T) {

		namespace := "host-operator" + uuid.NewV4().String()[:7]
		client := testsupport.NewFakeClient(t)

		t.Run("failed to initialize generator", func(t *testing.T) {
			// given
			assets := func(name string) ([]byte, error) {
				return nil, errors.Errorf("test")
			}
			// when
			err := nstemplatetiers.CreateOrUpdateResources(s, client, namespace, assets)
			// then
			require.Error(t, err)
			assert.Equal(t, "unable to create or update NSTemplateTiers: unable to initialize the nstemplatetierGenerator: test", err.Error())
		})

		t.Run("failed to generate nstemplatetiers", func(t *testing.T) {
			// given
			assets := func(name string) ([]byte, error) {
				if name == "metadata.yaml" {
					return []byte("advanced-code: abcdef"), nil
				}
				return nil, errors.Errorf("test")
			}
			// when
			err := nstemplatetiers.CreateOrUpdateResources(s, client, namespace, assets)
			// then
			require.Error(t, err)
			assert.Equal(t, "unable to create or update NSTemplateTiers: unable to generate all NSTemplateTier manifests: unable to generate 'advanced' NSTemplateTier manifest: test", err.Error())
		})

		t.Run("failed to create nstemplatetiers", func(t *testing.T) {
			// given
			assets := func(name string) ([]byte, error) {
				if name == "metadata.yaml" {
					return []byte("advanced-code: abcdef"), nil
				}
				return testnstemplatetiers.Asset(name)
			}
			client := testsupport.NewFakeClient(t)
			client.MockCreate = func(ctx context.Context, obj runtime.Object) error {
				return errors.Errorf("test")
			}
			// when
			err := nstemplatetiers.CreateOrUpdateResources(s, client, namespace, assets)
			// then
			require.Error(t, err)
			assert.Equal(t, fmt.Sprintf("unable to create the NSTemplateTiers 'advanced' in namespace '%s': test", namespace), err.Error())

		})

		t.Run("failed to update nstemplatetiers", func(t *testing.T) {
			// given
			assets := func(name string) ([]byte, error) {
				if name == "metadata.yaml" {
					return []byte("advanced-code: abcdef"), nil
				}
				return testnstemplatetiers.Asset(name)
			}
			// initialize the client with an existing `advanced` NSTemplatetier, matching the data above
			client := testsupport.NewFakeClient(t, &toolchainv1alpha1.NSTemplateTier{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: namespace,
					Name:      "advanced",
				},
			})
			client.MockUpdate = func(ctx context.Context, obj runtime.Object) error {
				// trigger an error when trying to update the existing `advanced` NSTemplatetier
				return errors.Errorf("test")
			}
			// when
			err := nstemplatetiers.CreateOrUpdateResources(s, client, namespace, assets)
			// then
			require.Error(t, err)
			assert.Equal(t, fmt.Sprintf("unable to update the NSTemplateTiers 'advanced' in namespace '%s': test", namespace), err.Error())
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
basic-stage: %[1]sf`, prefix)), nil
		}
		return testnstemplatetiers.Asset(name)
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
	}
	return assets, revisions
}
