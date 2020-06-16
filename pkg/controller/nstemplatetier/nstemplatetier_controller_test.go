package nstemplatetier

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	"github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

const (
	operatorNamespace = "toolchain-host-operator"
)

func TestReconcile(t *testing.T) {

	// given
	logf.SetLogger(logf.ZapLogger(true))
	// a "basic" NSTemplateTier
	oldNSTemplateTier := toolchainv1alpha1.NSTemplateTier{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: operatorNamespace,
			Name:      "basic",
		},
		Spec: toolchainv1alpha1.NSTemplateTierSpec{
			Namespaces: []toolchainv1alpha1.NSTemplateTierNamespace{
				{
					TemplateRef: "basic-code-123456old",
				},
				{
					TemplateRef: "basic-dev-123456old",
				},
				{
					TemplateRef: "basic-stage-123456old",
				},
			},
			ClusterResources: &toolchainv1alpha1.NSTemplateTierClusterResources{
				TemplateRef: "basic-clusterresources-123456a",
			},
		},
	}
	newNSTemplateTier := toolchainv1alpha1.NSTemplateTier{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: operatorNamespace,
			Name:      "basic",
		},
		Spec: toolchainv1alpha1.NSTemplateTierSpec{
			Namespaces: []toolchainv1alpha1.NSTemplateTierNamespace{
				{
					TemplateRef: "basic-code-123456new",
				},
				{
					TemplateRef: "basic-dev-123456new",
				},
				{
					TemplateRef: "basic-stage-123456new",
				},
			},
			ClusterResources: &toolchainv1alpha1.NSTemplateTierClusterResources{
				TemplateRef: "basic-clusterresources-123456a",
			},
		},
	}
	otherNSTemplateTier := toolchainv1alpha1.NSTemplateTier{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: operatorNamespace,
			Name:      "other",
		},
		Spec: toolchainv1alpha1.NSTemplateTierSpec{
			Namespaces: []toolchainv1alpha1.NSTemplateTierNamespace{
				{
					TemplateRef: "other-code-123456old",
				},
				{
					TemplateRef: "other-dev-123456old",
				},
				{
					TemplateRef: "other-stage-123456old",
				},
			},
			ClusterResources: &toolchainv1alpha1.NSTemplateTierClusterResources{
				TemplateRef: "other-clusterresources-123456a",
			},
		},
	}

	t.Run("controller should create a TemplateUpdateRequest", func(t *testing.T) {

		// in this test, there are 10 MasterUserRecords but no associated TemplateUpdateRequest
		t.Run("when no TemplateUpdateRequest resource exists at all", func(t *testing.T) {
			// given
			initObjs := []runtime.Object{&newNSTemplateTier}
			initObjs = append(initObjs, newMasterUserRecords(t, 10, templates(oldNSTemplateTier))...)
			r, req, cl := prepareReconcile(t, newNSTemplateTier.Name, initObjs...)
			// when
			res, err := r.Reconcile(req)
			// then
			require.NoError(t, err)
			require.Equal(t, reconcile.Result{Requeue: true}, res) // expect a requeue to create more TemplateUpdateRequest resources
			// check that a single TemplateUpdateRequest was created
			actualTemplateUpdateRequests := toolchainv1alpha1.TemplateUpdateRequestList{}
			err = cl.List(context.TODO(), &actualTemplateUpdateRequests)
			require.NoError(t, err)
			assert.Len(t, actualTemplateUpdateRequests.Items, 1)
		})

		// in this test, there are TemplateUpdateRequest resources but for associated with the update of another NSTemplateTier
		t.Run("when no TemplateUpdateRequest resource exists for the given NSTemplateTier", func(t *testing.T) {
			// given
			initObjs := []runtime.Object{&newNSTemplateTier}
			initObjs = append(initObjs, newMasterUserRecords(t, 10, templates(oldNSTemplateTier))...)
			initObjs = append(initObjs, newTemplateUpdateRequests(MaxPoolSize, tierName("other"), nameFormat("other-%d"))...)
			r, req, cl := prepareReconcile(t, newNSTemplateTier.Name, initObjs...)
			// when
			res, err := r.Reconcile(req)
			// then
			require.NoError(t, err)
			require.Equal(t, reconcile.Result{Requeue: true}, res) // expect a requeue to create more TemplateUpdateRequest resources
			// check that a single TemplateUpdateRequest was created
			actualTemplateUpdateRequests := toolchainv1alpha1.TemplateUpdateRequestList{}
			err = cl.List(context.TODO(), &actualTemplateUpdateRequests)
			require.NoError(t, err)
			assert.Len(t, actualTemplateUpdateRequests.Items, MaxPoolSize+1) // 1 resource was created, `MaxPoolSize` already existed
		})

		// in this test, the controller can create an extra TemplateUpdateRequest resource
		// because one of the existing ones is being deleted
		t.Run("when maximum number of TemplateUpdateRequest is reached but one is being deleted", func(t *testing.T) {
			// given
			initObjs := []runtime.Object{&newNSTemplateTier}
			initObjs = append(initObjs, newMasterUserRecords(t, 10, templates(oldNSTemplateTier))...)
			initObjs = append(initObjs, newTemplateUpdateRequests(MaxPoolSize, deletionTimestamp(0))...)
			r, req, cl := prepareReconcile(t, newNSTemplateTier.Name, initObjs...)
			// when
			res, err := r.Reconcile(req)
			// then
			require.NoError(t, err)
			require.Equal(t, reconcile.Result{Requeue: true}, res) // expect a requeue to create more TemplateUpdateRequest resources (if possible)
			// check that a single TemplateUpdateRequest was created
			actualTemplateUpdateRequests := toolchainv1alpha1.TemplateUpdateRequestList{}
			err = cl.List(context.TODO(), &actualTemplateUpdateRequests)
			require.NoError(t, err)
			assert.Len(t, actualTemplateUpdateRequests.Items, MaxPoolSize+1) // one more TemplateUpdateRequest
		})

		// in this test, there are 20 MasterUserRecords on the same tier. The first 10 are already up-to-date, and the other ones which need to be updated
		t.Run("when MasterUserRecord in continued fetch is not up-to-date", func(t *testing.T) {
			// given
			initObjs := []runtime.Object{&newNSTemplateTier}
			initObjs = append(initObjs, newMasterUserRecords(t, 10, nameFormat("new-user-%d"), templates(newNSTemplateTier))...)
			initObjs = append(initObjs, newMasterUserRecords(t, 10, nameFormat("old-user-%d"), templates(oldNSTemplateTier))...)
			r, req, cl := prepareReconcile(t, newNSTemplateTier.Name, initObjs...)
			// when
			res, err := r.Reconcile(req)
			// then
			require.NoError(t, err)
			require.Equal(t, reconcile.Result{Requeue: true}, res) // expect a requeue to create more TemplateUpdateRequest resources
			// check that a single TemplateUpdateRequest was created
			actualTemplateUpdateRequests := toolchainv1alpha1.TemplateUpdateRequestList{}
			err = cl.List(context.TODO(), &actualTemplateUpdateRequests)
			require.NoError(t, err)
			assert.Len(t, actualTemplateUpdateRequests.Items, 1) // one TemplateUpdateRequest created
		})
	})

	// in these tests, the controller should NOT create a single TemplateUpdateRequest
	t.Run("controller should not create any TemplateUpdateRequest", func(t *testing.T) {

		// in this test, there is simply no MasterUserRecord to update
		t.Run("when no MasterUserRecord resource exists", func(t *testing.T) {
			// given
			r, req, cl := prepareReconcile(t, newNSTemplateTier.Name, &newNSTemplateTier)
			// when
			res, err := r.Reconcile(req)
			// then
			require.NoError(t, err)
			require.Equal(t, reconcile.Result{}, res)
			// check that no TemplateUpdateRequest was created
			templateUpdateRequests := toolchainv1alpha1.TemplateUpdateRequestList{}
			err = cl.List(context.TODO(), &templateUpdateRequests)
			require.NoError(t, err)
			assert.Empty(t, templateUpdateRequests.Items)
		})

		// in this test, all existing MasterUserRecords are already up-to-date
		t.Run("when all MasterUserRecords are up-to-date", func(t *testing.T) {
			// given
			initObjs := []runtime.Object{&newNSTemplateTier}
			initObjs = append(initObjs, newMasterUserRecords(t, 20, templates(newNSTemplateTier))...)
			r, req, cl := prepareReconcile(t, newNSTemplateTier.Name, initObjs...)
			// when
			res, err := r.Reconcile(req)
			// then
			require.NoError(t, err)
			require.Equal(t, reconcile.Result{}, res)
			// check that no TemplateUpdateRequest was created
			templateUpdateRequests := toolchainv1alpha1.TemplateUpdateRequestList{}
			err = cl.List(context.TODO(), &templateUpdateRequests)
			require.NoError(t, err)
			assert.Empty(t, templateUpdateRequests.Items)
		})

		// in this test, all MasterUserRecords are being updated,
		// i.e., there is already an associated TemplateUpdateRequest
		t.Run("when all MasterUserRecords are being updated", func(t *testing.T) {
			// given
			initObjs := []runtime.Object{&newNSTemplateTier}
			initObjs = append(initObjs, newMasterUserRecords(t, 20, templates(newNSTemplateTier))...)
			initObjs = append(initObjs, newTemplateUpdateRequests(MaxPoolSize)...)
			r, req, cl := prepareReconcile(t, newNSTemplateTier.Name, initObjs...)
			// when
			res, err := r.Reconcile(req)
			// then
			require.NoError(t, err)
			require.Equal(t, reconcile.Result{}, res)
			// check that no TemplateUpdateRequest was created
			templateUpdateRequests := toolchainv1alpha1.TemplateUpdateRequestList{}
			err = cl.List(context.TODO(), &templateUpdateRequests)
			require.NoError(t, err)
			assert.Len(t, templateUpdateRequests.Items, MaxPoolSize) // size unchanged

		})

		// in this test, there are a more MasterUserRecords to update than `MaxPoolSize` allows, but
		// the max number of current TemplateRequestUpdate resources is reached (`MaxPoolSize`)
		t.Run("when maximum number of active TemplateUpdateRequest resources is reached", func(t *testing.T) {
			// given
			initObjs := []runtime.Object{&newNSTemplateTier}
			initObjs = append(initObjs, newMasterUserRecords(t, 10, templates(oldNSTemplateTier))...)
			initObjs = append(initObjs, newTemplateUpdateRequests(MaxPoolSize)...)
			r, req, cl := prepareReconcile(t, newNSTemplateTier.Name, initObjs...)
			// when
			res, err := r.Reconcile(req)
			// then
			require.NoError(t, err)
			require.Equal(t, reconcile.Result{}, res)
			// check that a single TemplateUpdateRequest was created
			actualTemplateUpdateRequests := toolchainv1alpha1.TemplateUpdateRequestList{}
			err = cl.List(context.TODO(), &actualTemplateUpdateRequests)
			require.NoError(t, err)
			assert.Len(t, actualTemplateUpdateRequests.Items, MaxPoolSize) // no increase
		})

		// in this test, all MasterUserRecords are associated with a different tier,
		// so none if them needs to be updated.
		t.Run("when no MasterUserRecord is associated with the updated NSTemplteTier", func(t *testing.T) {
			// given
			initObjs := []runtime.Object{&newNSTemplateTier}
			initObjs = append(initObjs, newMasterUserRecords(t, 10, templates(otherNSTemplateTier))...)
			r, req, cl := prepareReconcile(t, newNSTemplateTier.Name, initObjs...)
			// when
			res, err := r.Reconcile(req)
			// then
			require.NoError(t, err)
			require.Equal(t, reconcile.Result{}, res)
			// check that no TemplateUpdateRequest was created
			actualTemplateUpdateRequests := toolchainv1alpha1.TemplateUpdateRequestList{}
			err = cl.List(context.TODO(), &actualTemplateUpdateRequests)
			require.NoError(t, err)
			assert.Empty(t, actualTemplateUpdateRequests.Items)
		})

	})
}

func prepareReconcile(t *testing.T, name string, initObjs ...runtime.Object) (*ReconcileNSTemplateTier, reconcile.Request, *test.FakeClient) {
	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret",
			Namespace: "test-namespace",
		},
		Type: v1.SecretTypeOpaque,
		Data: map[string][]byte{
			"token": []byte("mycooltoken"),
		},
	}
	initObjs = append(initObjs, secret)
	cl := test.NewFakeClient(t, initObjs...)
	// (partial) support the `limit` and `continue` when listing MasterUserRecords
	// Here, the result's `continue` is the initial `continue` + `limit`
	cl.MockList = func(ctx context.Context, list runtime.Object, opts ...client.ListOption) error {
		if murs, ok := list.(*toolchainv1alpha1.MasterUserRecordList); ok {
			c := 0
			if murs.Continue != "" {
				c, err = strconv.Atoi(murs.Continue)
				if err != nil {
					return err
				}
			}
			if err := cl.Client.List(ctx, list, opts...); err != nil {
				return err
			}
			listOpts := client.ListOptions{}
			for _, opt := range opts {
				opt.ApplyToList(&listOpts)
			}
			if c > 0 {
				murs.Items = murs.Items[c:]
			}
			if int(listOpts.Limit) < len(murs.Items) {
				// remove first items
				murs.Items = murs.Items[:listOpts.Limit]
			}
			murs.Continue = strconv.Itoa(c + int(listOpts.Limit))
			return nil
		}
		// default behaviour
		return cl.Client.List(ctx, list, opts...)
	}
	r := &ReconcileNSTemplateTier{
		client: cl,
		scheme: s,
		retrieveMemberClusters: func(conditions ...cluster.Condition) []*cluster.FedCluster {
			return []*cluster.FedCluster{
				{
					Name: "cluster1",
				},
				{
					Name: "cluster2",
				},
			}
		},
	}
	return r, reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      name,
			Namespace: operatorNamespace,
		},
	}, cl
}

type masterUserRecordOption interface {
	applyToMasterUserRecord(int, *toolchainv1alpha1.MasterUserRecord) error
}

type templateUpdateRequestOption interface {
	applyToTemplateUpdateRequest(int, *toolchainv1alpha1.TemplateUpdateRequest)
}

type nameFormat string

var _ masterUserRecordOption = nameFormat("")
var _ templateUpdateRequestOption = nameFormat("")

func (f nameFormat) applyToMasterUserRecord(i int, mur *toolchainv1alpha1.MasterUserRecord) error {
	mur.ObjectMeta.Name = fmt.Sprintf(string(f), i)
	return nil
}

func (f nameFormat) applyToTemplateUpdateRequest(i int, mur *toolchainv1alpha1.TemplateUpdateRequest) {
	mur.ObjectMeta.Name = fmt.Sprintf(string(f), i)
}

type templates toolchainv1alpha1.NSTemplateTier

var _ masterUserRecordOption = templates(toolchainv1alpha1.NSTemplateTier{})

func (t templates) applyToMasterUserRecord(_ int, mur *toolchainv1alpha1.MasterUserRecord) error {
	s := toolchainv1alpha1.NSTemplateSetSpec{}
	s.TierName = t.Name
	s.Namespaces = make([]toolchainv1alpha1.NSTemplateSetNamespace, len(t.Spec.Namespaces))
	for i, ns := range t.Spec.Namespaces {
		s.Namespaces[i].TemplateRef = ns.TemplateRef
	}
	s.ClusterResources = &toolchainv1alpha1.NSTemplateSetClusterResources{}
	s.ClusterResources.TemplateRef = t.Spec.ClusterResources.TemplateRef
	// add the user account
	mur.Spec.UserAccounts = append(mur.Spec.UserAccounts, toolchainv1alpha1.UserAccountEmbedded{
		TargetCluster: "member-cluster",
		Spec: toolchainv1alpha1.UserAccountSpecEmbedded{
			UserAccountSpecBase: toolchainv1alpha1.UserAccountSpecBase{
				NSTemplateSet: s,
			},
		},
	})
	// set the labels for the tier templates in use
	hash, err := ComputeTemplateRefsHash(toolchainv1alpha1.NSTemplateTier(t))
	if err != nil {
		return err
	}
	mur.ObjectMeta.Labels = map[string]string{
		toolchainv1alpha1.LabelKeyPrefix + t.Name + "-tier-hash": hash,
	}
	return nil
}

func newMasterUserRecords(t *testing.T, size int, options ...masterUserRecordOption) []runtime.Object {
	murs := make([]runtime.Object, size)
	for i := 0; i < size; i++ {
		mur := &toolchainv1alpha1.MasterUserRecord{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: operatorNamespace,
				Name:      fmt.Sprintf("user-%d", i),
				Labels:    map[string]string{},
			},
			Spec: toolchainv1alpha1.MasterUserRecordSpec{
				UserAccounts: []toolchainv1alpha1.UserAccountEmbedded{},
			},
		}
		for _, opt := range options {
			err := opt.applyToMasterUserRecord(i, mur)
			require.NoError(t, err)
		}
		murs[i] = mur
	}
	return murs
}

type deletionTimestamp int

var _ templateUpdateRequestOption = deletionTimestamp(0)

func (d deletionTimestamp) applyToTemplateUpdateRequest(i int, r *toolchainv1alpha1.TemplateUpdateRequest) {
	if i == int(d) {
		deletionTS := metav1.NewTime(time.Now())
		r.DeletionTimestamp = &deletionTS
	}
}

type tierName string

var _ templateUpdateRequestOption = tierName("")

func (t tierName) applyToTemplateUpdateRequest(_ int, r *toolchainv1alpha1.TemplateUpdateRequest) {
	r.Spec.TierName = string(t)
	r.Labels = map[string]string{
		toolchainv1alpha1.NSTemplateTierNameLabelKey: string(t),
	}
}

func newTemplateUpdateRequests(size int, options ...templateUpdateRequestOption) []runtime.Object {
	templateUpdateRequests := make([]runtime.Object, size)
	for i := 0; i < size; i++ {
		r := &toolchainv1alpha1.TemplateUpdateRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("user-%d", i),
				Namespace: operatorNamespace,
				Labels: map[string]string{
					toolchainv1alpha1.NSTemplateTierNameLabelKey: "basic",
				},
			},
		}
		for _, opt := range options {
			opt.applyToTemplateUpdateRequest(i, r)
		}
		templateUpdateRequests[i] = r
	}
	return templateUpdateRequests
}
