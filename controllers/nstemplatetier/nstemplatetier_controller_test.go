package nstemplatetier_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/controllers/nstemplatetier"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	tiertest "github.com/codeready-toolchain/host-operator/test/nstemplatetier"
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
	"k8s.io/client-go/kubernetes/scheme"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	operatorNamespace = "toolchain-host-operator"
)

var ns = unstructured.Unstructured{
	Object: map[string]interface{}{
		"kind": "Namespace",
		"metadata": map[string]interface{}{
			"name": "{{.SPACE_NAME}}",
		},
	},
}

func TestReconcile(t *testing.T) {
	// given
	logf.SetLogger(zap.New(zap.UseDevMode(true)))

	t.Run("controller should add entry in tier.status.updates", func(t *testing.T) {

		t.Run("without previous entry", func(t *testing.T) {
			// given
			base1nsTier := tiertest.Base1nsTier(t, tiertest.CurrentBase1nsTemplates)
			initObjs := []runtimeclient.Object{base1nsTier}
			r, req, cl := prepareReconcile(t, base1nsTier.Name, initObjs...)
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
			initObjs := []runtimeclient.Object{base1nsTier}
			r, req, cl := prepareReconcile(t, base1nsTier.Name, initObjs...)
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
			tierTemplates := initTierTemplates(t, nil)
			// given
			base1nsTier := tiertest.Base1nsTier(t, tiertest.CurrentBase1nsTemplates,
				tiertest.WithCurrentUpdate()) // current update already exists

			initObjs := []runtimeclient.Object{base1nsTier}
			r, req, cl := prepareReconcile(t, base1nsTier.Name, append(initObjs, tierTemplates...)...)
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
				initObjs := []runtimeclient.Object{base1nsTier}
				r, req, cl := prepareReconcile(t, base1nsTier.Name, initObjs...)
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
				initObjs := []runtimeclient.Object{base1nsTier}
				r, req, cl := prepareReconcile(t, base1nsTier.Name, initObjs...)
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
				initObjs := []runtimeclient.Object{base1nsTier}
				r, req, cl := prepareReconcile(t, base1nsTier.Name, initObjs...)
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

		base1nsTier := tiertest.Base1nsTier(t, tiertest.CurrentBase1nsTemplates,
			// the tiertemplate content will be evaluated with these parameters, and the output will be stored inside the tiertemplate revision CR's
			tiertest.WithParameter("SPACE_NAME", "janedoe"),
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
		initObjs := []runtimeclient.Object{base1nsTier}

		// TODO remove this subtest once we completely switch to using TTRs
		t.Run("using tiertemplates as revisions", func(t *testing.T) {
			tierTemplates := initTierTemplates(t, nil)
			t.Run("add revisions when they are missing", func(t *testing.T) {
				// given
				r, req, cl := prepareReconcile(t, base1nsTier.Name, append(initObjs, tierTemplates...)...)
				// when
				res, err := r.Reconcile(context.TODO(), req)
				// then
				require.NoError(t, err)
				// check that revisions field was populated
				tierTemplatesRefs := []string{
					"base1ns-admin-123456new", "base1ns-clusterresources-123456new", "base1ns-code-123456new", "base1ns-dev-123456new", "base1ns-edit-123456new", "base1ns-stage-123456new", "base1ns-viewer-123456new",
				}
				tiertest.AssertThatNSTemplateTier(t, "base1ns", cl).
					HasStatusTierTemplateRevisions(tierTemplatesRefs)
				// check that expected TierTemplateRevision CRs were NOT created when using TierTemplates as revisions
				ttrs := toolchainv1alpha1.TierTemplateRevisionList{}
				err = cl.List(context.TODO(), &ttrs, runtimeclient.InNamespace(base1nsTier.GetNamespace()))
				require.NoError(t, err)
				require.Empty(t, ttrs.Items)
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
						HasStatusTierTemplateRevisions([]string{
							"base1ns-admin-123456new", "base1ns-clusterresources-123456new", "base1ns-code-123456new", "base1ns-dev-123456new", "base1ns-edit-123456new", "base1ns-stage-123456new", "base1ns-viewer-123456new",
						})
					// no TierTemplateRevision CRs were created
					ttrs := toolchainv1alpha1.TierTemplateRevisionList{}
					err = cl.List(context.TODO(), &ttrs, runtimeclient.InNamespace(base1nsTier.GetNamespace()))
					require.NoError(t, err)
					require.Empty(t, ttrs.Items)
				})
			})
		})

		t.Run("using TTR as revisions", func(t *testing.T) {
			// initialize tier templates with templateObjects field populated
			// for simplicity we initialize all of them with the same objects
			tierTemplates := initTierTemplates(t, withTemplateObjects(ns))
			t.Run("add revisions when they are missing ", func(t *testing.T) {
				// given
				r, req, cl := prepareReconcile(t, base1nsTier.Name, append(initObjs, tierTemplates...)...)
				// when
				res, err := r.Reconcile(context.TODO(), req)
				// then
				require.NoError(t, err)
				// check that revisions field was populated
				tierTemplatesRefs := []string{
					"base1ns-admin-123456new", "base1ns-clusterresources-123456new", "base1ns-code-123456new", "base1ns-dev-123456new", "base1ns-edit-123456new", "base1ns-stage-123456new", "base1ns-viewer-123456new",
				}
				tiertest.AssertThatNSTemplateTier(t, "base1ns", cl).
					HasStatusTierTemplateRevisions(tierTemplatesRefs)
				// check that expected TierTemplateRevision CRs were created
				for _, ref := range tierTemplatesRefs {
					ttrs := toolchainv1alpha1.TierTemplateRevisionList{}
					// list by exptected labels
					// there should be 1 ttr for each tiertemplateref
					labels := map[string]string{
						toolchainv1alpha1.TemplateRefLabelKey: ref,
						toolchainv1alpha1.TierLabelKey:        "base1ns",
					}
					err = cl.List(context.TODO(), &ttrs, runtimeclient.InNamespace(base1nsTier.GetNamespace()), runtimeclient.MatchingLabels(labels))
					require.NoError(t, err)
					require.Len(t, ttrs.Items, 1)
					// check that owner reference was set
					assert.Equal(t, "TierTemplate", ttrs.Items[0].OwnerReferences[0].Kind)
					assert.Equal(t, ref, ttrs.Items[0].OwnerReferences[0].Name)
					assert.Contains(t, string(ttrs.Items[0].Spec.TemplateObjects[0].Raw), "janedoe")        // the object should have the namespace name replaced
					assert.NotContains(t, string(ttrs.Items[0].Spec.TemplateObjects[0].Raw), ".SPACE_NAME") // the object should NOT contain the variable anymore
				}
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
						HasStatusTierTemplateRevisions([]string{
							"base1ns-admin-123456new", "base1ns-clusterresources-123456new", "base1ns-code-123456new", "base1ns-dev-123456new", "base1ns-edit-123456new", "base1ns-stage-123456new", "base1ns-viewer-123456new",
						})
					// expected TierTemplateRevision CRs are still there
					ttrs := toolchainv1alpha1.TierTemplateRevisionList{}
					err = cl.List(context.TODO(), &ttrs, runtimeclient.InNamespace(base1nsTier.GetNamespace()))
					require.NoError(t, err)
					require.Len(t, ttrs.Items, 7) // it's one TTR per each tiertemplate in the NSTemplateTier
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
							HasStatusTierTemplateRevisions([]string{
								"base1ns-admin-123456new", "base1ns-clusterresources-123456new", "base1ns-code-123456new", "base1ns-dev-123456new", "base1ns-edit-123456new", "base1ns-stage-123456new", "base1ns-viewer-123456new",
							})
						// expected TierTemplateRevision CRs are there
						ttrs := toolchainv1alpha1.TierTemplateRevisionList{}
						err = cl.List(context.TODO(), &ttrs, runtimeclient.InNamespace(base1nsTier.GetNamespace()))
						require.NoError(t, err)
						require.Len(t, ttrs.Items, 7) // it's one TTR per each tiertemplate in the NSTemplateTier
					})
				})

			})

			t.Run("revision field is set but TierTemplateRevision CRs are missing, they should be created", func(t *testing.T) {
				// given
				// the NSTemplateTier has already the status.revisions field populated
				// but the TierTemplateRevision CRs are missing
				base1nsTierWithRevisions := base1nsTier
				base1nsTierWithRevisions.Status.Revisions = map[string]string{
					"base1ns-admin-123456new": "base1ns-admin-123456new-abcd", "base1ns-clusterresources-123456new": "base1ns-clusterresources-123456new-abcd", "base1ns-code-123456new": "base1ns-code-123456new-abcd", "base1ns-dev-123456new": "base1ns-dev-123456new-abcd", "base1ns-edit-123456new": "`base1ns-edit-123456new-abcd", "base1ns-stage-123456new": "base1ns-stage-123456new-abcd", "base1ns-viewer-123456new": "base1ns-viewer-123456new-abcd"}
				initObjs := []runtimeclient.Object{base1nsTier}
				r, req, cl := prepareReconcile(t, base1nsTier.Name, append(initObjs, tierTemplates...)...)
				// when
				// check no TTR is present before reconciling
				ttrs := toolchainv1alpha1.TierTemplateRevisionList{}
				err := cl.List(context.TODO(), &ttrs, runtimeclient.InNamespace(base1nsTier.GetNamespace()))
				require.NoError(t, err)
				require.Empty(t, ttrs.Items)
				_, err = r.Reconcile(context.TODO(), req)
				// then
				require.NoError(t, err)
				// check that revisions field was populated
				tiertest.AssertThatNSTemplateTier(t, "base1ns", cl).
					HasStatusTierTemplateRevisions([]string{
						"base1ns-admin-123456new", "base1ns-clusterresources-123456new", "base1ns-code-123456new", "base1ns-dev-123456new", "base1ns-edit-123456new", "base1ns-stage-123456new", "base1ns-viewer-123456new",
					})
				// check that expected TierTemplateRevision CRs were created
				ttrs = toolchainv1alpha1.TierTemplateRevisionList{}
				err = cl.List(context.TODO(), &ttrs, runtimeclient.InNamespace(base1nsTier.GetNamespace()))
				require.NoError(t, err)
				require.Len(t, ttrs.Items, 7)

			})

			t.Run("errors", func(t *testing.T) {

				t.Run("error when TierTemplate is missing ", func(t *testing.T) {
					// given
					// make sure revisions field is nill before starting the test
					base1nsTier.Status.Revisions = nil
					r, req, cl := prepareReconcile(t, base1nsTier.Name, initObjs...)
					// when
					_, err = r.Reconcile(context.TODO(), req)
					// then
					// we expect an error caused by the absence of the tiertemplate for the `code` namespace CR
					require.ErrorContains(t, err, "tiertemplates.toolchain.dev.openshift.com \"base1ns-code-123456new\" not found")
					// the revisions field also should remain empty
					tiertest.AssertThatNSTemplateTier(t, "base1ns", cl).
						HasNoStatusTierTemplateRevisions()
					// and the TierTemplateRevision CRs are not created
					ttrs := toolchainv1alpha1.TierTemplateRevisionList{}
					err = cl.List(context.TODO(), &ttrs, runtimeclient.InNamespace(base1nsTier.GetNamespace()))
					require.NoError(t, err)
					require.Empty(t, ttrs.Items)
				})

			})

		})
	})

}

// initTierTemplates creates the TierTemplates objects for the base1ns tier
func initTierTemplates(t *testing.T, withTemplateObjects []runtime.RawExtension) []runtimeclient.Object {
	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)
	clusterResourceTierTemplate := createTierTemplate(t, "clusterresources", withTemplateObjects)
	codeNsTierTemplate := createTierTemplate(t, "code", withTemplateObjects)
	devNsTierTemplate := createTierTemplate(t, "dev", withTemplateObjects)
	stageNsTierTemplate := createTierTemplate(t, "stage", withTemplateObjects)
	adminRoleTierTemplate := createTierTemplate(t, "admin", withTemplateObjects)
	viewerRoleTierTemplate := createTierTemplate(t, "viewer", withTemplateObjects)
	editRoleTierTemplate := createTierTemplate(t, "edit", withTemplateObjects)
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

func createTierTemplate(t *testing.T, typeName string, withTemplateObjects []runtime.RawExtension) *toolchainv1alpha1.TierTemplate {
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

	tierName := "base1ns"
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
