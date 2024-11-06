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
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	templatev1 "github.com/openshift/api/template/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

var (
	ns test.TemplateObject = `
- apiVersion: v1
  kind: Namespace
  metadata:
    name: ${SPACE_NAME}-NSTYPE
`

	execPodsRole test.TemplateObject = `
- apiVersion: rbac.authorization.k8s.io/v1
  kind: Role
  metadata:
    name: exec-pods
    namespace: ${SPACE_NAME}-NSTYPE
  rules:
  - apiGroups:
    - ""
    resources:
    - pods/exec
    verbs:
    - get
    - list
    - watch
    - create
    - delete
    - update`

	namespace test.TemplateParam = `
- name: NAMESPACE
  required: true`
	spacename test.TemplateParam = `
- name: SPACE_NAME
  value: johnsmith`
	username test.TemplateParam = `
- name: USERNAME
  value: johnsmith`

	advancedCrq test.TemplateObject = `
- apiVersion: quota.openshift.io/v1
  kind: ClusterResourceQuota
  metadata:
    name: for-${SPACE_NAME}
  spec:
    quota:
      hard:
        limits.cpu: 2000m
        limits.memory: 10Gi
    selector:
      annotations:
        openshift.io/requester: ${SPACE_NAME}
    labels: null
  `
	crqFeature1 test.TemplateObject = `
- apiVersion: quota.openshift.io/v1
  kind: ClusterResourceQuota
  metadata:
    name: feature-1-for-${SPACE_NAME}
    annotations:
      toolchain.dev.openshift.com/feature: feature-1
  spec:
    quota:
      hard:
        limits.cpu: 2000m
        limits.memory: 10Gi
    selector:
      annotations:
        openshift.io/requester: ${SPACE_NAME}
    labels: null
  `
	crqFeature2 test.TemplateObject = `
- apiVersion: quota.openshift.io/v1
  kind: ClusterResourceQuota
  metadata:
    name: feature-2-for-${SPACE_NAME}
    annotations:
      toolchain.dev.openshift.com/feature: feature-2
  spec:
    quota:
      hard:
        limits.cpu: 2000m
        limits.memory: 10Gi
    selector:
      annotations:
        openshift.io/requester: ${SPACE_NAME}
    labels: null
  `
	crqFeature3 test.TemplateObject = `
- apiVersion: quota.openshift.io/v1
  kind: ClusterResourceQuota
  metadata:
    name: feature-3-for-${SPACE_NAME}
    annotations:
      toolchain.dev.openshift.com/feature: feature-3
  spec:
    quota:
      hard:
        limits.cpu: 2000m
        limits.memory: 10Gi
    selector:
      annotations:
        openshift.io/requester: ${SPACE_NAME}
    labels: null
  `

	clusterTektonRb test.TemplateObject = `
- apiVersion: rbac.authorization.k8s.io/v1
  kind: ClusterRoleBinding
  metadata:
    name: ${SPACE_NAME}-tekton-view
  roleRef:
    apiGroup: rbac.authorization.k8s.io
    kind: ClusterRole
    name: tekton-view-for-${SPACE_NAME}
  subjects:
    - kind: User
      name: ${USERNAME}
`
	idlerDev test.TemplateObject = `
- apiVersion: toolchain.dev.openshift.com/v1alpha1
  kind: Idler
  metadata:
    name: ${SPACE_NAME}-dev
  spec:
    timeoutSeconds: 28800 # 8 hours
  `
	idlerStage test.TemplateObject = `
- apiVersion: toolchain.dev.openshift.com/v1alpha1
  kind: Idler
  metadata:
    name: ${SPACE_NAME}-stage
  spec:
    timeoutSeconds: 28800 # 8 hours
  `

	spaceAdmin test.TemplateObject = `
- apiVersion: rbac.authorization.k8s.io/v1
  kind: Role
  metadata:
    name: space-admin
    namespace: ${NAMESPACE}
  rules:
    # examples
    - apiGroups:
        - ""
      resources:
        - "configmaps"
        - "secrets"
        - "serviceaccounts"
      verbs:
        - get
        - list
  `
	spaceAdminRb test.TemplateObject = `
- apiVersion: rbac.authorization.k8s.io/v1
  kind: RoleBinding
  metadata:
    name: ${USERNAME}-space-admin
    namespace: ${NAMESPACE}
  roleRef:
    apiGroup: rbac.authorization.k8s.io
    kind: Role
    name: space-admin
  subjects:
    - kind: User
      name: ${USERNAME}
`
	spaceViewer test.TemplateObject = `
- apiVersion: rbac.authorization.k8s.io/v1
  kind: Role
  metadata:
    name: space-viewer
    namespace: ${NAMESPACE}
  rules:
    # examples
    - apiGroups:
        - ""
      resources:
        - "configmaps"
      verbs:
        - get
        - list
  `
	spaceViewerRb test.TemplateObject = `
- apiVersion: rbac.authorization.k8s.io/v1
  kind: RoleBinding
  metadata:
    name: ${USERNAME}-space-viewer
    namespace: ${NAMESPACE}
  roleRef:
    apiGroup: rbac.authorization.k8s.io
    kind: Role
    name: space-viewer
  subjects:
    - kind: User
      name: ${USERNAME}
`
)

func TestReconcile(t *testing.T) {
	// given
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	tierTemplates := initTierTemplates(t)

	t.Run("controller should add entry in tier.status.updates", func(t *testing.T) {

		t.Run("without previous entry", func(t *testing.T) {
			// given
			base1nsTier := tiertest.Base1nsTier(t, tiertest.CurrentBase1nsTemplates)
			initObjs := []runtime.Object{base1nsTier}
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
			initObjs := []runtime.Object{base1nsTier}
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
			// given
			base1nsTier := tiertest.Base1nsTier(t, tiertest.CurrentBase1nsTemplates,
				tiertest.WithCurrentUpdate()) // current update already exists

			initObjs := []runtime.Object{base1nsTier}
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
				initObjs := []runtime.Object{base1nsTier}
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
				initObjs := []runtime.Object{base1nsTier}
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
				initObjs := []runtime.Object{base1nsTier}
				r, req, cl := prepareReconcile(t, base1nsTier.Name, initObjs...)
				cl.MockStatusUpdate = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.UpdateOption) error {
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

		t.Run("add revisions when they are missing ", func(t *testing.T) {
			// given
			base1nsTier := tiertest.Base1nsTier(t, tiertest.CurrentBase1nsTemplates)
			initObjs := []runtime.Object{base1nsTier}
			r, req, cl := prepareReconcile(t, base1nsTier.Name, append(initObjs, tierTemplates...)...)
			// when
			res, err := r.Reconcile(context.TODO(), req)
			// then
			require.NoError(t, err)
			require.Equal(t, reconcile.Result{Requeue: true}, res) // explicit requeue after the adding an entry in `status.updates`

			// when
			res, err = r.Reconcile(context.TODO(), req)
			// then
			require.NoError(t, err)
			require.Equal(t, reconcile.Result{Requeue: true}, res) // explicit requeue after the adding revisions in `status.revisions`
			// check that revisions field was populated
			tierTemplatesRefs := []string{
				"base1ns-admin-123456new", "base1ns-clusterresources-123456new", "base1ns-code-123456new", "base1ns-dev-123456new", "base1ns-edit-123456new", "base1ns-stage-123456new", "base1ns-viewer-123456new",
			}
			tiertest.AssertThatNSTemplateTier(t, "base1ns", cl).
				HasStatusTierTemplateRevision(tierTemplatesRefs)
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
					HasStatusTierTemplateRevision([]string{
						"base1ns-admin-123456new", "base1ns-clusterresources-123456new", "base1ns-code-123456new", "base1ns-dev-123456new", "base1ns-edit-123456new", "base1ns-stage-123456new", "base1ns-viewer-123456new",
					})
				// expected TierTemplateRevision CRs are still there
				ttrs := toolchainv1alpha1.TierTemplateRevisionList{}
				err = cl.List(context.TODO(), &ttrs, runtimeclient.InNamespace(base1nsTier.GetNamespace()))
				require.NoError(t, err)
				require.Len(t, ttrs.Items, 7)
			})

		})

		t.Run("revision field is set but TierTemplateRevision CRs are missing, they should be created", func(t *testing.T) {
			// given
			base1nsTier := tiertest.Base1nsTier(t, tiertest.CurrentBase1nsTemplates)
			// the NSTemplateTier has already the status.revisions field populated
			// but the TierTemplateRevision CRs are missing
			base1nsTier.Status.Revisions = map[string]string{
				"base1ns-admin-123456new": "base1ns-admin-123456new-abcd", "base1ns-clusterresources-123456new": "base1ns-clusterresources-123456new-abcd", "base1ns-code-123456new": "base1ns-code-123456new-abcd", "base1ns-dev-123456new": "base1ns-dev-123456new-abcd", "base1ns-edit-123456new": "`base1ns-edit-123456new-abcd", "base1ns-stage-123456new": "base1ns-stage-123456new-abcd", "base1ns-viewer-123456new": "base1ns-viewer-123456new-abcd"}
			initObjs := []runtime.Object{base1nsTier}
			r, req, cl := prepareReconcile(t, base1nsTier.Name, append(initObjs, tierTemplates...)...)
			// when
			// check no TTR is present before reconciling
			ttrs := toolchainv1alpha1.TierTemplateRevisionList{}
			err := cl.List(context.TODO(), &ttrs, runtimeclient.InNamespace(base1nsTier.GetNamespace()))
			require.NoError(t, err)
			require.Len(t, ttrs.Items, 0)
			res, err := r.Reconcile(context.TODO(), req)
			// then
			require.NoError(t, err)
			require.Equal(t, reconcile.Result{Requeue: true}, res) // explicit requeue after adding an entry in `status.updates`
			// when
			res, err = r.Reconcile(context.TODO(), req)
			// then
			require.NoError(t, err)
			// check that revisions field was populated
			tiertest.AssertThatNSTemplateTier(t, "base1ns", cl).
				HasStatusTierTemplateRevision([]string{
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
				base1nsTier := tiertest.Base1nsTier(t, tiertest.CurrentBase1nsTemplates)
				initObjs := []runtime.Object{base1nsTier}
				r, req, cl := prepareReconcile(t, base1nsTier.Name, initObjs...)
				// when
				res, err := r.Reconcile(context.TODO(), req)
				// then
				require.NoError(t, err)
				require.Equal(t, reconcile.Result{Requeue: true}, res) // explicit requeue after the adding an entry in `status.updates`
				// when
				res, err = r.Reconcile(context.TODO(), req)
				// then
				// we expect an error caused by the absence of the tiertemplate CR
				require.ErrorContains(t, err, "tiertemplates.toolchain.dev.openshift.com \"base1ns-code-123456new\" not found")
				// the revisions field also should remain empty
				tiertest.AssertThatNSTemplateTier(t, "base1ns", cl).
					HasNoStatusTierTemplateRevision()
				// and the TierTemplateRevision CRs are not created
				ttrs := toolchainv1alpha1.TierTemplateRevisionList{}
				err = cl.List(context.TODO(), &ttrs, runtimeclient.InNamespace(base1nsTier.GetNamespace()))
				require.NoError(t, err)
				require.Len(t, ttrs.Items, 0)

			})

		})

	})

}

// initTierTemplates creates the TierTemplates objects for the base1ns tier
func initTierTemplates(t *testing.T) []runtime.Object {
	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)
	clusterResourcesContent := test.CreateTemplate(test.WithObjects(advancedCrq, crqFeature1, crqFeature2, crqFeature3, clusterTektonRb, idlerDev, idlerStage), test.WithParams(spacename, username))
	nsContent := test.CreateTemplate(test.WithObjects(ns, execPodsRole), test.WithParams(spacename))
	adminRoleContent := test.CreateTemplate(test.WithObjects(spaceAdmin, spaceAdminRb), test.WithParams(namespace, username))
	viewerRoleContent := test.CreateTemplate(test.WithObjects(spaceViewer, spaceViewerRb), test.WithParams(namespace, username))
	codecFactory := serializer.NewCodecFactory(s)
	decoder := codecFactory.UniversalDeserializer()
	clusterResourceTierTemplate, err := createTierTemplate(decoder, clusterResourcesContent, "base1ns", "clusterresources", "123456new")
	require.NoError(t, err)
	codeNsTierTemplate, err := createTierTemplate(decoder, nsContent, "base1ns", "code", "123456new")
	require.NoError(t, err)
	devNsTierTemplate, err := createTierTemplate(decoder, nsContent, "base1ns", "dev", "123456new")
	require.NoError(t, err)
	stageNsTierTemplate, err := createTierTemplate(decoder, nsContent, "base1ns", "stage", "123456new")
	require.NoError(t, err)
	adminRoleTierTemplate, err := createTierTemplate(decoder, adminRoleContent, "base1ns", "admin", "123456new")
	require.NoError(t, err)
	viewerRoleTierTemplate, err := createTierTemplate(decoder, viewerRoleContent, "base1ns", "viewer", "123456new")
	require.NoError(t, err)
	editRoleTierTemplate, err := createTierTemplate(decoder, adminRoleContent, "base1ns", "edit", "123456new")
	require.NoError(t, err)
	tierTemplates := []runtime.Object{clusterResourceTierTemplate, codeNsTierTemplate, devNsTierTemplate, stageNsTierTemplate, adminRoleTierTemplate, viewerRoleTierTemplate, editRoleTierTemplate}
	return tierTemplates
}

func prepareReconcile(t *testing.T, name string, initObjs ...runtime.Object) (reconcile.Reconciler, reconcile.Request, *test.FakeClient) {
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

func createTierTemplate(decoder runtime.Decoder, tmplContent string, tierName, typeName, revision string) (*toolchainv1alpha1.TierTemplate, error) {
	tmplContent = strings.ReplaceAll(tmplContent, "NSTYPE", typeName)
	tmpl := templatev1.Template{}
	_, _, err := decoder.Decode([]byte(tmplContent), nil, &tmpl)
	if err != nil {
		return nil, err
	}
	return &toolchainv1alpha1.TierTemplate{
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
	}, nil
}
