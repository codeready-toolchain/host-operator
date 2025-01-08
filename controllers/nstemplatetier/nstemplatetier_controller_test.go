package nstemplatetier_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/controllers/nstemplatetier"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	tiertest "github.com/codeready-toolchain/host-operator/test/nstemplatetier"
	"github.com/codeready-toolchain/host-operator/test/tiertemplaterevision"
	"github.com/codeready-toolchain/toolchain-common/pkg/hash"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	templatev1 "github.com/openshift/api/template/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/kubernetes/scheme"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	operatorNamespace = "toolchain-host-operator"
)

func TestReconcile(t *testing.T) {
	// given
	logf.SetLogger(zap.New(zap.UseDevMode(true)))

	t.Run("controller should add entry in tier.status.updates", func(t *testing.T) {

		t.Run("without previous entry", func(t *testing.T) {
			// given
			base1nsTier := tiertest.Base1nsTier(t, tiertest.CurrentBase1nsTemplates)
			r, req, cl := prepareReconcile(t, base1nsTier.Name, base1nsTier)
			// when
			res, err := r.Reconcile(context.TODO(), req)
			// then
			require.NoError(t, err)
			require.Equal(t, reconcile.Result{Requeue: true}, res) // explicit requeue after the adding an entry in `status.updates`
			// check that an entry was added in `status.updates`
			tiertest.AssertThatNSTemplateTier(t, "base1ns", cl).
				HasStatusUpdatesItems(1).
				HasLatestUpdate(toolchainv1alpha1.NSTemplateTierHistory{
					Hash: base1nsTier.Labels["toolchain.dev.openshift.com/base1ns-tier-hash"],
				})
		})

		t.Run("with previous entries", func(t *testing.T) {
			// given
			base1nsTier := tiertest.Base1nsTier(t, tiertest.CurrentBase1nsTemplates, tiertest.WithPreviousUpdates(
				toolchainv1alpha1.NSTemplateTierHistory{
					StartTime: metav1.Now(),
					Hash:      "abc123",
				},
				toolchainv1alpha1.NSTemplateTierHistory{
					StartTime: metav1.Now(),
					Hash:      "def456",
				}))
			r, req, cl := prepareReconcile(t, base1nsTier.Name, base1nsTier)
			// when
			res, err := r.Reconcile(context.TODO(), req)
			// then
			require.NoError(t, err)
			require.Equal(t, reconcile.Result{Requeue: true}, res) // explicit requeue after the adding an entry in `status.updates`
			// check that an entry was added in `status.updates`
			tiertest.AssertThatNSTemplateTier(t, "base1ns", cl).
				HasStatusUpdatesItems(3).
				HasValidPreviousUpdates().
				HasLatestUpdate(toolchainv1alpha1.NSTemplateTierHistory{
					Hash: base1nsTier.Labels["toolchain.dev.openshift.com/base1ns-tier-hash"],
				})
		})
	})

	t.Run("controller should NOT add entry in tier.status.updates", func(t *testing.T) {

		t.Run("last entry exists with matching hash", func(t *testing.T) {
			// given
			base1nsTier := tiertest.Base1nsTier(t, tiertest.CurrentBase1nsTemplates,
				tiertest.WithCurrentUpdate()) // current update already exists
			tierTemplates := initTierTemplates(t, nil, base1nsTier.Name)

			r, req, cl := prepareReconcile(t, base1nsTier.Name, append(tierTemplates, base1nsTier)...)
			// when
			_, err := r.Reconcile(context.TODO(), req)
			// then
			require.NoError(t, err)
			// check that no entry was added in `status.updates`
			tiertest.AssertThatNSTemplateTier(t, "base1ns", cl).
				HasStatusUpdatesItems(1). // same number of entries
				HasLatestUpdate(toolchainv1alpha1.NSTemplateTierHistory{
					Hash: base1nsTier.Labels["toolchain.dev.openshift.com/base1ns-tier-hash"],
				})
		})
	})

	t.Run("failures", func(t *testing.T) {

		t.Run("unable to get NSTemplateTier", func(t *testing.T) {

			t.Run("tier not found", func(t *testing.T) {
				// given
				base1nsTier := tiertest.Base1nsTier(t, tiertest.CurrentBase1nsTemplates)
				r, req, cl := prepareReconcile(t, base1nsTier.Name, base1nsTier)
				cl.MockGet = func(ctx context.Context, key types.NamespacedName, obj runtimeclient.Object, opts ...runtimeclient.GetOption) error {
					if _, ok := obj.(*toolchainv1alpha1.NSTemplateTier); ok {
						return errors.NewNotFound(schema.GroupResource{}, key.Name)
					}
					return cl.Client.Get(ctx, key, obj, opts...)
				}
				// when
				res, err := r.Reconcile(context.TODO(), req)
				// then
				require.NoError(t, err)
				assert.Equal(t, reconcile.Result{}, res) // no explicit requeue
			})

			t.Run("other error", func(t *testing.T) {
				// given
				base1nsTier := tiertest.Base1nsTier(t, tiertest.CurrentBase1nsTemplates)
				r, req, cl := prepareReconcile(t, base1nsTier.Name, base1nsTier)
				cl.MockGet = func(ctx context.Context, key types.NamespacedName, obj runtimeclient.Object, opts ...runtimeclient.GetOption) error {
					if _, ok := obj.(*toolchainv1alpha1.NSTemplateTier); ok {
						return fmt.Errorf("mock error")
					}
					return cl.Client.Get(ctx, key, obj, opts...)
				}
				// when
				res, err := r.Reconcile(context.TODO(), req)
				// then
				require.Error(t, err)
				require.EqualError(t, err, "unable to get the current NSTemplateTier: mock error")
				assert.Equal(t, reconcile.Result{}, res) // no explicit requeue
			})
		})

		t.Run("unable to update NSTemplateTier status", func(t *testing.T) {

			t.Run("when adding new update", func(t *testing.T) {
				// given
				base1nsTier := tiertest.Base1nsTier(t, tiertest.CurrentBase1nsTemplates)
				r, req, cl := prepareReconcile(t, base1nsTier.Name, base1nsTier)
				cl.MockStatusUpdate = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.SubResourceUpdateOption) error {
					if _, ok := obj.(*toolchainv1alpha1.NSTemplateTier); ok {
						return fmt.Errorf("mock error")
					}
					return cl.Client.Status().Update(ctx, obj, opts...)
				}
				// when
				res, err := r.Reconcile(context.TODO(), req)
				// then
				require.EqualError(t, err, "unable to insert a new entry in status.updates after NSTemplateTier changed: mock error")
				assert.Equal(t, reconcile.Result{}, res) // no explicit requeue
			})
		})

	})

	t.Run("revisions management", func(t *testing.T) {
		// given
		base1nsTier := tiertest.Base1nsTier(t, tiertest.CurrentBase1nsTemplates,
			// the tiertemplate revision CR should have a copy of those parameters
			tiertest.WithParameter("DEPLOYMENT_QUOTA", "60"),
		)
		tierHash, err := hash.ComputeHashForNSTemplateTier(base1nsTier)
		require.NoError(t, err)
		// let's set the update so that we can skip the first reconciliation loop when this is updated
		base1nsTier.Status.Updates = []toolchainv1alpha1.NSTemplateTierHistory{
			{
				StartTime: metav1.Now(),
				Hash:      tierHash,
			},
		}
		tierTemplatesRefs := []string{
			"base1ns-admin-123456new", "base1ns-clusterresources-123456new", "base1ns-code-123456new", "base1ns-dev-123456new", "base1ns-edit-123456new", "base1ns-stage-123456new", "base1ns-viewer-123456new",
		}

		// TODO remove this subtest once we completely switch to using TTRs
		t.Run("using tiertemplates as revisions", func(t *testing.T) {
			tierTemplates := initTierTemplates(t, nil, base1nsTier.Name)
			t.Run("add revisions when they are missing", func(t *testing.T) {
				// given
				r, req, cl := prepareReconcile(t, base1nsTier.Name, append(tierTemplates, base1nsTier)...)
				// when
				res, err := r.Reconcile(context.TODO(), req)
				// then
				require.NoError(t, err)
				require.Equal(t, reconcile.Result{Requeue: true, RequeueAfter: 10 * time.Second}, res) // explicit requeue after the adding revisions in `status.revisions`
				// check that revisions field was populated
				tierTemplatesRefs := []string{
					"base1ns-admin-123456new", "base1ns-clusterresources-123456new", "base1ns-code-123456new", "base1ns-dev-123456new", "base1ns-edit-123456new", "base1ns-stage-123456new", "base1ns-viewer-123456new",
				}
				tiertest.AssertThatNSTemplateTier(t, "base1ns", cl).
					HasStatusTierTemplateRevisions(tierTemplatesRefs)
				// check that expected TierTemplateRevision CRs were NOT created when using TierTemplates as revisions
				tiertemplaterevision.AssertThatTTRs(t, cl, base1nsTier.GetNamespace()).DoNotExist()
				t.Run("don't add revisions when they are up to date", func(t *testing.T) {
					// given
					// the NSTemplateTier already has the revisions from previous test

					// when
					res, err = r.Reconcile(context.TODO(), req)
					// then
					require.NoError(t, err)
					require.Equal(t, reconcile.Result{Requeue: false}, res) // no reconcile
					// revisions are the same
					tiertest.AssertThatNSTemplateTier(t, "base1ns", cl).
						HasStatusTierTemplateRevisions(tierTemplatesRefs)
					// no TierTemplateRevision CRs were created
					tiertemplaterevision.AssertThatTTRs(t, cl, base1nsTier.GetNamespace()).DoNotExist()
				})
			})
		})

		t.Run("using TTR as revisions", func(t *testing.T) {
			// initialize tier templates with templateObjects field populated
			// for simplicity we initialize all of them with the same objects
			var crq = unstructured.Unstructured{Object: map[string]interface{}{
				"kind": "ClusterResourceQuota",
				"metadata": map[string]interface{}{
					"name": "for-{{.SPACE_NAME}}-deployments",
				},
				"spec": map[string]interface{}{
					"quota": map[string]interface{}{
						"hard": map[string]interface{}{
							"count/deploymentconfigs.apps": "{{.DEPLOYMENT_QUOTA}}",
							"count/deployments.apps":       "{{.DEPLOYMENT_QUOTA}}",
							"count/pods":                   "600",
						},
					},
					"selector": map[string]interface{}{
						"annotations": map[string]interface{}{},
						"labels": map[string]interface{}{
							"matchLabels": map[string]interface{}{
								"toolchain.dev.openshift.com/space": "'{{.SPACE_NAME}}'",
							},
						},
					},
				},
			}}
			t.Run("add revisions when they are missing ", func(t *testing.T) {
				// given
				tierTemplates := initTierTemplates(t, withTemplateObjects(crq), base1nsTier.Name)
				r, req, cl := prepareReconcile(t, base1nsTier.Name, append(tierTemplates, base1nsTier)...)
				// when
				res, err := r.Reconcile(context.TODO(), req)
				// then
				require.NoError(t, err)
				require.Equal(t, reconcile.Result{Requeue: true, RequeueAfter: 10 * time.Second}, res) // explicit requeue after the adding revisions in `status.revisions`
				// check that revisions field was populated
				tiertest.AssertThatNSTemplateTier(t, "base1ns", cl).
					HasStatusTierTemplateRevisions(tierTemplatesRefs)
				// check that expected TierTemplateRevision CRs were created
				tiertemplaterevision.AssertThatTTRs(t, cl, base1nsTier.GetNamespace()).
					ExistsFor("base1ns", tierTemplatesRefs...).ForEach(func(ttr *toolchainv1alpha1.TierTemplateRevision) {
					// verify the content of the TierTemplate matches the one of the TTR
					templateRef, ok := ttr.GetLabels()[toolchainv1alpha1.TemplateRefLabelKey]
					assert.True(t, ok)
					tierTemplate := toolchainv1alpha1.TierTemplate{}
					err := cl.Get(context.TODO(), types.NamespacedName{Name: templateRef, Namespace: base1nsTier.GetNamespace()}, &tierTemplate)
					require.NoError(t, err)
					assert.Equal(t, tierTemplate.Spec.TemplateObjects, ttr.Spec.TemplateObjects)
					assert.Equal(t, ttr.Spec.Parameters, base1nsTier.Spec.Parameters)
				})
				t.Run("don't add revisions when they are up to date", func(t *testing.T) {
					// given
					// the NSTemplateTier already has the revisions from previous test

					// when
					res, err = r.Reconcile(context.TODO(), req)
					// then
					require.NoError(t, err)
					require.Equal(t, reconcile.Result{Requeue: false}, res) // no reconcile
					// revisions are the same
					tiertest.AssertThatNSTemplateTier(t, "base1ns", cl).
						HasStatusTierTemplateRevisions(tierTemplatesRefs)
					// expected TierTemplateRevision CRs are still there
					ttrs := toolchainv1alpha1.TierTemplateRevisionList{}
					err = cl.List(context.TODO(), &ttrs, runtimeclient.InNamespace(base1nsTier.GetNamespace()))
					require.NoError(t, err)
					require.Len(t, ttrs.Items, len(tierTemplatesRefs)) // it's one TTR per each tiertemplate in the NSTemplateTier
					t.Run("revision field is set but some TierTemplateRevision is missing, status should be updated", func(t *testing.T) {
						// given
						// the NSTemplateTier already has the revisions from previous test
						// we delete the first TTR to make sure the status is updated and the TTR get's recreated
						err = cl.Delete(context.TODO(), &ttrs.Items[0])
						require.NoError(t, err)

						// when
						res, err = r.Reconcile(context.TODO(), req)
						// then
						require.NoError(t, err)
						// revisions are the same
						tiertest.AssertThatNSTemplateTier(t, "base1ns", cl).
							HasStatusTierTemplateRevisions(tierTemplatesRefs)
						// expected TierTemplateRevision CRs are there
						ttrs := toolchainv1alpha1.TierTemplateRevisionList{}
						err = cl.List(context.TODO(), &ttrs, runtimeclient.InNamespace(base1nsTier.GetNamespace()))
						require.NoError(t, err)
						require.Len(t, ttrs.Items, len(tierTemplatesRefs)) // it's one TTR per each tiertemplate in the NSTemplateTier
					})
				})

			})

			t.Run("revision field is set but TierTemplateRevision CRs are missing, they should be created", func(t *testing.T) {
				// given
				// the NSTemplateTier has already the status.revisions field populated
				// but the TierTemplateRevision CRs are missing
				tierTemplates := initTierTemplates(t, withTemplateObjects(crq), base1nsTier.Name)
				base1nsTierWithRevisions := base1nsTier
				base1nsTierWithRevisions.Status.Revisions = map[string]string{
					"base1ns-admin-123456new": "base1ns-admin-123456new-abcd", "base1ns-clusterresources-123456new": "base1ns-clusterresources-123456new-abcd", "base1ns-code-123456new": "base1ns-code-123456new-abcd", "base1ns-dev-123456new": "base1ns-dev-123456new-abcd", "base1ns-edit-123456new": "`base1ns-edit-123456new-abcd", "base1ns-stage-123456new": "base1ns-stage-123456new-abcd", "base1ns-viewer-123456new": "base1ns-viewer-123456new-abcd"}
				r, req, cl := prepareReconcile(t, base1nsTierWithRevisions.Name, append(tierTemplates, base1nsTierWithRevisions)...)
				// when
				// check no TTR is present before reconciling
				tiertemplaterevision.AssertThatTTRs(t, cl, base1nsTierWithRevisions.GetNamespace()).DoNotExist()
				_, err = r.Reconcile(context.TODO(), req)
				// then
				require.NoError(t, err)
				// check that revisions field was populated
				tiertest.AssertThatNSTemplateTier(t, "base1ns", cl).
					HasStatusTierTemplateRevisions(tierTemplatesRefs)
				// check that expected TierTemplateRevision CRs were created
				ttrs := toolchainv1alpha1.TierTemplateRevisionList{}
				err = cl.List(context.TODO(), &ttrs, runtimeclient.InNamespace(base1nsTierWithRevisions.GetNamespace()))
				require.NoError(t, err)
				require.Len(t, ttrs.Items, len(tierTemplatesRefs))

			})

			t.Run("TTR name should stay within 63 chars, so that they can be used as labels", func(t *testing.T) {
				// given
				// the TierTemplateRevision CRs are missing, and their name are based on the tier name.
				// Making the TierName already 63chars long we test that the TTR name stays within 63 chars
				veryLongTierName := "somerandomstringtomakethenamelongerthan63chars12345678912345678"
				tierWithVeryLongName := tiertest.Tier(t, veryLongTierName, tiertest.NSTemplateTierSpecWithTierName(veryLongTierName),
					tiertest.WithParameter("DEPLOYMENT_QUOTA", "60"),
				)
				tierHash, err := hash.ComputeHashForNSTemplateTier(tierWithVeryLongName)
				require.NoError(t, err)
				// let's set the update so that we can skip the first reconciliation loop when this is updated
				tierWithVeryLongName.Status.Updates = []toolchainv1alpha1.NSTemplateTierHistory{
					{
						StartTime: metav1.Now(),
						Hash:      tierHash,
					},
				}
				tierTemplatesWithLongNames := initTierTemplates(t, withTemplateObjects(crq), tierWithVeryLongName.Name)
				r, req, cl := prepareReconcile(t, tierWithVeryLongName.Name, append(tierTemplatesWithLongNames, tierWithVeryLongName)...)
				// when
				// check no TTR is present before reconciling
				tiertemplaterevision.AssertThatTTRs(t, cl, tierWithVeryLongName.GetNamespace()).DoNotExist()
				_, err = r.Reconcile(context.TODO(), req)
				// then
				require.NoError(t, err)
				// check that expected TierTemplateRevision CRs were created
				// with the expected length
				ttrs := toolchainv1alpha1.TierTemplateRevisionList{}
				err = cl.List(context.TODO(), &ttrs, runtimeclient.InNamespace(tierWithVeryLongName.GetNamespace()))
				require.NoError(t, err)
				require.Len(t, ttrs.Items, 7)
				for _, ttr := range ttrs.Items {
					assert.Empty(t, validation.IsDNS1123Label(ttr.GetName()))
				}

			})

			t.Run("errors", func(t *testing.T) {

				t.Run("error when TierTemplate is missing ", func(t *testing.T) {
					// given
					// make sure revisions field is nill before starting the test
					base1nsTier.Status.Revisions = nil
					r, req, cl := prepareReconcile(t, base1nsTier.Name, base1nsTier)
					// when
					_, err = r.Reconcile(context.TODO(), req)
					// then
					// we expect an error caused by the absence of the tiertemplate for the `code` namespace CR
					require.ErrorContains(t, err, "tiertemplates.toolchain.dev.openshift.com \"base1ns-code-123456new\" not found")
					// the revisions field also should remain empty
					tiertest.AssertThatNSTemplateTier(t, "base1ns", cl).
						HasNoStatusTierTemplateRevisions()
					// and the TierTemplateRevision CRs are not created
					tiertemplaterevision.AssertThatTTRs(t, cl, base1nsTier.GetNamespace()).DoNotExist()
				})

			})

		})
	})

}

// initTierTemplates creates the TierTemplates objects for the base1ns tier
func initTierTemplates(t *testing.T, withTemplateObjects []runtime.RawExtension, tierName string) []runtimeclient.Object {
	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)
	clusterResourceTierTemplate := createTierTemplate(t, "clusterresources", withTemplateObjects, tierName)
	codeNsTierTemplate := createTierTemplate(t, "code", withTemplateObjects, tierName)
	devNsTierTemplate := createTierTemplate(t, "dev", withTemplateObjects, tierName)
	stageNsTierTemplate := createTierTemplate(t, "stage", withTemplateObjects, tierName)
	adminRoleTierTemplate := createTierTemplate(t, "admin", withTemplateObjects, tierName)
	viewerRoleTierTemplate := createTierTemplate(t, "viewer", withTemplateObjects, tierName)
	editRoleTierTemplate := createTierTemplate(t, "edit", withTemplateObjects, tierName)
	tierTemplates := []runtimeclient.Object{clusterResourceTierTemplate, codeNsTierTemplate, devNsTierTemplate, stageNsTierTemplate, adminRoleTierTemplate, viewerRoleTierTemplate, editRoleTierTemplate}
	return tierTemplates
}

func prepareReconcile(t *testing.T, name string, initObjs ...runtimeclient.Object) (reconcile.Reconciler, reconcile.Request, *test.FakeClient) {
	os.Setenv("WATCH_NAMESPACE", test.HostOperatorNs)
	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)
	cl := test.NewFakeClient(t, initObjs...)
	r := &nstemplatetier.Reconciler{
		Client: cl,
		Scheme: s,
	}
	return r, reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      name,
			Namespace: operatorNamespace,
		},
	}, cl
}

func createTierTemplate(t *testing.T, typeName string, withTemplateObjects []runtime.RawExtension, tierName string) *toolchainv1alpha1.TierTemplate {
	var (
		ns test.TemplateObject = `
- apiVersion: v1
  kind: Namespace
  metadata:
    name: ${SPACE_NAME}
`
		spacename test.TemplateParam = `
- name: SPACE_NAME
  value: johnsmith`
	)
	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)
	codecFactory := serializer.NewCodecFactory(s)
	decoder := codecFactory.UniversalDeserializer()
	tmpl := templatev1.Template{}
	_, _, err = decoder.Decode([]byte(test.CreateTemplate(test.WithObjects(ns), test.WithParams(spacename))), nil, &tmpl)
	require.NoError(t, err)

	revision := "123456new"
	// we can set the template field to something empty as it is not relevant for the tests
	tt := &toolchainv1alpha1.TierTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      strings.ToLower(fmt.Sprintf("%s-%s-%s", tierName, typeName, revision)),
			Namespace: test.HostOperatorNs,
		},
		Spec: toolchainv1alpha1.TierTemplateSpec{
			TierName: tierName,
			Type:     typeName,
			Revision: revision,
			Template: tmpl,
		},
	}

	// just copy the raw objects to the templateObjects field
	// TODO this will be removed once we switch on using templateObjects only in the TierTemplates
	if withTemplateObjects != nil {
		tt.Spec.TemplateObjects = withTemplateObjects
	}

	return tt
}

func withTemplateObjects(templates ...unstructured.Unstructured) []runtime.RawExtension {
	var templateObjects []runtime.RawExtension
	for i := range templates {
		templateObjects = append(templateObjects, runtime.RawExtension{Object: &templates[i]})
	}
	return templateObjects
}
