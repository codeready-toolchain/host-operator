package nstemplatetier

import (
	"context"
	"strconv"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	turtest "github.com/codeready-toolchain/host-operator/test/templateupdaterequest"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	murtest "github.com/codeready-toolchain/toolchain-common/pkg/test/masteruserrecord"

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

func TestComputeHash(t *testing.T) {

	t.Run("should match without cluster resources", func(t *testing.T) {
		// given
		nsTemplateTier := toolchainv1alpha1.NSTemplateTier{
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
			},
		}
		nsTemplateSet := toolchainv1alpha1.NSTemplateSet{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: operatorNamespace,
				Name:      "foo",
			},
			Spec: toolchainv1alpha1.NSTemplateSetSpec{
				TierName: "basic",
				Namespaces: []toolchainv1alpha1.NSTemplateSetNamespace{
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
			},
		}
		// when
		hash1, err1 := ComputeHashForNSTemplateTier(nsTemplateTier)
		hash2, err2 := ComputeHashForNSTemplateSetSpec(nsTemplateSet.Spec)
		// then
		require.NoError(t, err1)
		require.NoError(t, err2)
		assert.Equal(t, hash1, hash2)
	})

	t.Run("should match with cluster resources", func(t *testing.T) {
		// given
		nsTemplateTier := toolchainv1alpha1.NSTemplateTier{
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
		nsTemplateSet := toolchainv1alpha1.NSTemplateSet{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: operatorNamespace,
				Name:      "foo",
			},
			Spec: toolchainv1alpha1.NSTemplateSetSpec{
				TierName: "basic",
				Namespaces: []toolchainv1alpha1.NSTemplateSetNamespace{
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
				ClusterResources: &toolchainv1alpha1.NSTemplateSetClusterResources{
					TemplateRef: "basic-clusterresources-123456a",
				},
			},
		}
		// when
		hash1, err1 := ComputeHashForNSTemplateTier(nsTemplateTier)
		hash2, err2 := ComputeHashForNSTemplateSetSpec(nsTemplateSet.Spec)
		// then
		require.NoError(t, err1)
		require.NoError(t, err2)
		assert.Equal(t, hash1, hash2)
	})

	t.Run("should not match without cluster resources", func(t *testing.T) {
		// given
		nsTemplateTier := toolchainv1alpha1.NSTemplateTier{
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
			},
		}
		nsTemplateSet := toolchainv1alpha1.NSTemplateSet{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: operatorNamespace,
				Name:      "foo",
			},
			Spec: toolchainv1alpha1.NSTemplateSetSpec{
				TierName: "basic",
				Namespaces: []toolchainv1alpha1.NSTemplateSetNamespace{
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
			},
		}
		// when
		hash1, err1 := ComputeHashForNSTemplateTier(nsTemplateTier)
		hash2, err2 := ComputeHashForNSTemplateSetSpec(nsTemplateSet.Spec)
		// then
		require.NoError(t, err1)
		require.NoError(t, err2)
		assert.NotEqual(t, hash1, hash2)
	})

	t.Run("should not match with cluster resources", func(t *testing.T) {
		// given
		nsTemplateTier := toolchainv1alpha1.NSTemplateTier{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: operatorNamespace,
				Name:      "basic",
			},
			Spec: toolchainv1alpha1.NSTemplateTierSpec{
				Namespaces: []toolchainv1alpha1.NSTemplateTierNamespace{
					{
						TemplateRef: "basic-code-123456old", // same as `nsTemplateSet` var
					},
					{
						TemplateRef: "basic-dev-123456old",
					},
					{
						TemplateRef: "basic-stage-123456old",
					},
				},
				ClusterResources: &toolchainv1alpha1.NSTemplateTierClusterResources{
					TemplateRef: "basic-clusterresources-123456new",
				},
			},
		}
		nsTemplateSet := toolchainv1alpha1.NSTemplateSet{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: operatorNamespace,
				Name:      "foo",
			},
			Spec: toolchainv1alpha1.NSTemplateSetSpec{
				TierName: "basic",
				Namespaces: []toolchainv1alpha1.NSTemplateSetNamespace{
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
				ClusterResources: &toolchainv1alpha1.NSTemplateSetClusterResources{
					TemplateRef: "basic-clusterresources-123456a",
				},
			},
		}
		// when
		hash1, err1 := ComputeHashForNSTemplateTier(nsTemplateTier)
		hash2, err2 := ComputeHashForNSTemplateSetSpec(nsTemplateSet.Spec)
		// then
		require.NoError(t, err1)
		require.NoError(t, err2)
		assert.NotEqual(t, hash1, hash2)
	})

}

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
				TemplateRef: "basic-clusterresources-123456old",
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
				TemplateRef: "basic-clusterresources-123456new",
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
					TemplateRef: "other-code-123456a",
				},
				{
					TemplateRef: "other-dev-123456a",
				},
				{
					TemplateRef: "other-stage-123456a",
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
			initObjs = append(initObjs, murtest.NewMasterUserRecords(t, 10, "user-%d", murtest.Account("cluster1", oldNSTemplateTier))...)
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
			assert.NotEmpty(t, actualTemplateUpdateRequests.Items[0].OwnerReferences)
		})

		// in this test, there are TemplateUpdateRequest resources but for associated with the update of another NSTemplateTier
		t.Run("when no TemplateUpdateRequest resource exists for the given NSTemplateTier", func(t *testing.T) {
			// given
			initObjs := []runtime.Object{&newNSTemplateTier}
			initObjs = append(initObjs, murtest.NewMasterUserRecords(t, 10, "user-%d", murtest.Account("cluster1", oldNSTemplateTier))...)
			initObjs = append(initObjs, turtest.NewTemplateUpdateRequests(MaxPoolSize, "other-%d", turtest.TierName("other"))...)
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
		// because one is in a "completed" status
		t.Run("when maximum number of TemplateUpdateRequest is reached but one is complete", func(t *testing.T) {
			// given
			initObjs := []runtime.Object{&newNSTemplateTier}
			initObjs = append(initObjs, murtest.NewMasterUserRecords(t, 10, "user-%d", murtest.Account("cluster1", oldNSTemplateTier))...)
			initObjs = append(initObjs, turtest.NewTemplateUpdateRequests(MaxPoolSize, "user-%d", turtest.Complete("user-0"))...)
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

		// in this test, the controller can create an extra TemplateUpdateRequest resource
		// because one is being deleted
		t.Run("when maximum number of TemplateUpdateRequest is reached but one is being deleted", func(t *testing.T) {
			// given
			initObjs := []runtime.Object{&newNSTemplateTier}
			initObjs = append(initObjs, murtest.NewMasterUserRecords(t, 10, "user-%d", murtest.Account("cluster1", oldNSTemplateTier))...)
			initObjs = append(initObjs, turtest.NewTemplateUpdateRequests(MaxPoolSize, "user-%d", turtest.DeletionTimestamp("user-0"))...)
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
			initObjs = append(initObjs, murtest.NewMasterUserRecords(t, 10, "new-user-%d", murtest.Account("cluster1", newNSTemplateTier))...)
			initObjs = append(initObjs, murtest.NewMasterUserRecords(t, 10, "old-user-%d", murtest.Account("cluster1", oldNSTemplateTier))...)
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
			initObjs = append(initObjs, murtest.NewMasterUserRecords(t, 20, "user-%d", murtest.Account("cluster1", newNSTemplateTier))...)
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
			initObjs = append(initObjs, murtest.NewMasterUserRecords(t, 20, "user-%d", murtest.Account("cluster1", newNSTemplateTier))...)
			initObjs = append(initObjs, turtest.NewTemplateUpdateRequests(MaxPoolSize, "user-%d")...)
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
			initObjs = append(initObjs, murtest.NewMasterUserRecords(t, 10, "user-%d", murtest.Account("cluster1", oldNSTemplateTier))...)
			initObjs = append(initObjs, turtest.NewTemplateUpdateRequests(MaxPoolSize, "user-%d")...)
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
			initObjs = append(initObjs, murtest.NewMasterUserRecords(t, 10, "user-%d", murtest.Account("cluster1", otherNSTemplateTier))...)
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
				// keep the first items and remove the following ones to fit into the limit
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
	}
	return r, reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      name,
			Namespace: operatorNamespace,
		},
	}, cl
}
