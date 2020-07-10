package nstemplatetier_test

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	"github.com/codeready-toolchain/host-operator/pkg/controller/nstemplatetier"
	tiertest "github.com/codeready-toolchain/host-operator/test/nstemplatetier"
	turtest "github.com/codeready-toolchain/host-operator/test/templateupdaterequest"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	murtest "github.com/codeready-toolchain/toolchain-common/pkg/test/masteruserrecord"

	"github.com/stretchr/testify/require"
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

	t.Run("controller should add entry in status.updates", func(t *testing.T) {

		t.Run("without previous entry", func(t *testing.T) {
			// given
			basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates)
			initObjs := []runtime.Object{basicTier}
			r, req, cl := prepareReconcile(t, basicTier.Name, initObjs...)
			// when
			res, err := r.Reconcile(req)
			// then
			require.NoError(t, err)
			require.Equal(t, reconcile.Result{Requeue: true}, res) // explicit requeue after the adding an entry in `status.updates`
			// check that an entry was added in `status.updates`
			tiertest.AssertThatNSTemplateTier(t, "basic", cl).
				HasStatusUpdatesItems(1).
				HasLatestUpdate(toolchainv1alpha1.NSTemplateTierHistory{
					Hash:           basicTier.Labels["toolchain.dev.openshift.com/basic-tier-hash"],
					Failures:       0,
					FailedAccounts: []string{},
				})
			turtest.AssertThatTemplateUpdateRequests(t, cl).TotalCount(0) // no TemplateUpdateRequest created yet
		})

		t.Run("with previous complete entries", func(t *testing.T) {
			// given
			now := metav1.Now()
			basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates, tiertest.WithPreviousUpdates(
				toolchainv1alpha1.NSTemplateTierHistory{
					StartTime:      metav1.Now(),
					CompletionTime: &now,
					Failures:       0,
					FailedAccounts: []string{},
				},
				toolchainv1alpha1.NSTemplateTierHistory{
					StartTime:      metav1.Now(),
					CompletionTime: &now,
					Failures:       2,
					FailedAccounts: []string{"failed1", "failed2"},
				}))
			initObjs := []runtime.Object{basicTier}
			r, req, cl := prepareReconcile(t, basicTier.Name, initObjs...)
			// when
			res, err := r.Reconcile(req)
			// then
			require.NoError(t, err)
			require.Equal(t, reconcile.Result{Requeue: true}, res) // explicit requeue after the adding an entry in `status.updates`
			// check that an entry was added in `status.updates`
			tiertest.AssertThatNSTemplateTier(t, "basic", cl).
				HasStatusUpdatesItems(3).
				HasValidPreviousUpdates().
				HasLatestUpdate(toolchainv1alpha1.NSTemplateTierHistory{
					Hash:           basicTier.Labels["toolchain.dev.openshift.com/basic-tier-hash"],
					Failures:       0,
					FailedAccounts: []string{},
				})
			turtest.AssertThatTemplateUpdateRequests(t, cl).TotalCount(0) // no TemplateUpdateRequest created yet
		})
	})

	t.Run("controller should create a TemplateUpdateRequest", func(t *testing.T) {

		// in this test, there are 10 MasterUserRecords but no associated TemplateUpdateRequest
		t.Run("when no TemplateUpdateRequest resource exists at all", func(t *testing.T) {
			// given
			previousBasicTier := tiertest.BasicTier(t, tiertest.PreviousBasicTemplates)
			basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates, tiertest.WithCurrentUpdateInProgress())
			initObjs := []runtime.Object{basicTier}
			initObjs = append(initObjs, murtest.NewMasterUserRecords(t, 10, "user-%d",
				murtest.Account("cluster1", *previousBasicTier))...)
			r, req, cl := prepareReconcile(t, basicTier.Name, initObjs...)
			// when
			res, err := r.Reconcile(req)
			// then
			require.NoError(t, err)
			require.Equal(t, reconcile.Result{}, res) // no need to explicit requeue after the creation
			// check that a single TemplateUpdateRequest was created
			turtest.AssertThatTemplateUpdateRequests(t, cl).TotalCount(1)
			turtest.AssertThatTemplateUpdateRequest(t, "user-0", cl).Exists().HasOwnerReference()
		})

		// in this test, there are TemplateUpdateRequest resources but for associated with the update of another NSTemplateTier
		t.Run("when no TemplateUpdateRequest resource exists for the given NSTemplateTier", func(t *testing.T) {
			// given
			previousBasicTier := tiertest.BasicTier(t, tiertest.PreviousBasicTemplates)
			basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates, tiertest.WithCurrentUpdateInProgress())
			initObjs := []runtime.Object{basicTier}
			initObjs = append(initObjs, murtest.NewMasterUserRecords(t, 10, "user-%d", murtest.Account("cluster1", *previousBasicTier))...)
			initObjs = append(initObjs, turtest.NewTemplateUpdateRequests(nstemplatetier.MaxPoolSize, "other-%d", *basicTier, turtest.TierName("other"))...)
			r, req, cl := prepareReconcile(t, basicTier.Name, initObjs...)
			// when
			res, err := r.Reconcile(req)
			// then
			require.NoError(t, err)
			require.Equal(t, reconcile.Result{}, res) // no need to explicit requeue after the creation
			// check that a single TemplateUpdateRequest was created
			turtest.AssertThatTemplateUpdateRequests(t, cl).TotalCount(nstemplatetier.MaxPoolSize + 1) // 1 resource was created, `MaxPoolSize` already existed
		})

		// in this test, the controller can create an extra TemplateUpdateRequest resource
		// because one is being deleted
		t.Run("when maximum number of TemplateUpdateRequest is reached but one is being deleted", func(t *testing.T) {
			// given
			previousBasicTier := tiertest.BasicTier(t, tiertest.PreviousBasicTemplates)
			basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates, tiertest.WithCurrentUpdateInProgress())
			initObjs := []runtime.Object{basicTier}
			initObjs = append(initObjs, murtest.NewMasterUserRecords(t, 10, "user-%d", murtest.Account("cluster1", *previousBasicTier))...)
			initObjs = append(initObjs, turtest.NewTemplateUpdateRequests(nstemplatetier.MaxPoolSize, "user-%d", *basicTier, turtest.DeletionTimestamp("user-0"))...)
			r, req, cl := prepareReconcile(t, basicTier.Name, initObjs...)
			// when
			res, err := r.Reconcile(req)
			// then
			require.NoError(t, err)
			require.Equal(t, reconcile.Result{}, res) // no need to explicit requeue after the creation
			// check that a single TemplateUpdateRequest was created
			turtest.AssertThatTemplateUpdateRequests(t, cl).TotalCount(nstemplatetier.MaxPoolSize + 1) // one more TemplateUpdateRequest
		})

		// in this test, there are 20 MasterUserRecords on the same tier.
		// The first 10 are already up-to-date, and the other ones which need to be updated
		t.Run("when MasterUserRecord in continued fetch is not up-to-date", func(t *testing.T) {
			// given
			previousBasicTier := tiertest.BasicTier(t, tiertest.PreviousBasicTemplates)
			basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates, tiertest.WithCurrentUpdateInProgress())
			initObjs := []runtime.Object{basicTier}
			initObjs = append(initObjs, murtest.NewMasterUserRecords(t, 10, "new-user-%d",
				murtest.Account("cluster1", *basicTier))...)
			initObjs = append(initObjs, murtest.NewMasterUserRecords(t, 10, "old-user-%d",
				murtest.Account("cluster1", *previousBasicTier))...)

			r, req, cl := prepareReconcile(t, basicTier.Name, initObjs...)
			// when
			res, err := r.Reconcile(req)
			// then
			require.NoError(t, err)
			require.Equal(t, reconcile.Result{}, res) // no need to explicit requeue after the creation
			// check that a single TemplateUpdateRequest was created
			turtest.AssertThatTemplateUpdateRequests(t, cl).TotalCount(1) // one TemplateUpdateRequest created
		})
	})

	// in these tests, the controller should NOT create a single TemplateUpdateRequest
	t.Run("controller should not create a TemplateUpdateRequest", func(t *testing.T) {

		// in this test, there is simply no MasterUserRecord to update
		t.Run("when no MasterUserRecord resource exists", func(t *testing.T) {
			// given
			basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates, tiertest.WithCurrentUpdateInProgress())
			r, req, cl := prepareReconcile(t, basicTier.Name, basicTier)
			// when
			res, err := r.Reconcile(req)
			// then
			require.NoError(t, err)
			require.Equal(t, reconcile.Result{}, res)
			// check that no TemplateUpdateRequest was created
			turtest.AssertThatTemplateUpdateRequests(t, cl).TotalCount(0)
			// also check that the tier `status.updates` was updated with a new entry
			now := metav1.NewTime(time.Now())
			tiertest.AssertThatNSTemplateTier(t, basicTier.Name, cl).
				HasStatusUpdatesItems(1).
				// at this point, all the counters are '0'
				HasLatestUpdate(toolchainv1alpha1.NSTemplateTierHistory{
					Hash:           basicTier.Labels["toolchain.dev.openshift.com/basic-tier-hash"],
					Failures:       2, // default
					FailedAccounts: []string{"failed1", "failed2"},
					CompletionTime: &now, // expect a completion time since there was no MasterUserRecord to update
				})
		})

		// in this test, all MasterUserRecords are associated with a different tier,
		// so none if them needs to be updated.
		t.Run("when no MasterUserRecord is associated with the updated NSTemplteTier", func(t *testing.T) {
			// given
			basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates, tiertest.WithCurrentUpdateInProgress())
			otherTier := tiertest.OtherTier()
			initObjs := []runtime.Object{basicTier}
			initObjs = append(initObjs, murtest.NewMasterUserRecords(t, 10, "user-%d",
				murtest.Account("cluster1", *otherTier))...)
			r, req, cl := prepareReconcile(t, basicTier.Name, initObjs...)
			// when
			res, err := r.Reconcile(req)
			// then
			require.NoError(t, err)
			require.Equal(t, reconcile.Result{}, res)
			// check that no TemplateUpdateRequest was created
			turtest.AssertThatTemplateUpdateRequests(t, cl).TotalCount(0)
			// also check that the tier `status.updates` was updated with a new entry
			now := metav1.Now()
			tiertest.AssertThatNSTemplateTier(t, basicTier.Name, cl).
				HasStatusUpdatesItems(1).
				// at this point, all the counters are '0'
				HasLatestUpdate(toolchainv1alpha1.NSTemplateTierHistory{
					Hash:           basicTier.Labels["toolchain.dev.openshift.com/basic-tier-hash"],
					Failures:       2, // default
					FailedAccounts: []string{"failed1", "failed2"},
					CompletionTime: &now, // expect a completion time since there was no MasterUserRecord to update
				})
		})

		// in this test, all existing MasterUserRecords are already up-to-date
		t.Run("when all MasterUserRecords are up-to-date", func(t *testing.T) {
			// given
			basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates, tiertest.WithCurrentUpdateInProgress())
			initObjs := []runtime.Object{basicTier}
			initObjs = append(initObjs, murtest.NewMasterUserRecords(t, 20, "user-%d", murtest.Account("cluster1", *basicTier))...)
			r, req, cl := prepareReconcile(t, basicTier.Name, initObjs...)
			// when
			res, err := r.Reconcile(req)
			// then
			require.NoError(t, err)
			require.Equal(t, reconcile.Result{}, res)
			// check that no TemplateUpdateRequest was created
			turtest.AssertThatTemplateUpdateRequests(t, cl).TotalCount(0)
			now := metav1.Now()
			tiertest.AssertThatNSTemplateTier(t, basicTier.Name, cl).
				HasStatusUpdatesItems(1).
				// at this point, all the counters are '0'
				HasLatestUpdate(toolchainv1alpha1.NSTemplateTierHistory{
					Hash:           basicTier.Labels["toolchain.dev.openshift.com/basic-tier-hash"],
					Failures:       2, // default
					FailedAccounts: []string{"failed1", "failed2"},
					CompletionTime: &now, // completion time should be set
				})
		})

		// in this test, all MasterUserRecords are being updated,
		// i.e., there is already an associated TemplateUpdateRequest
		t.Run("when all MasterUserRecords are being updated", func(t *testing.T) {
			// given
			basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates, tiertest.WithCurrentUpdateInProgress())
			initObjs := []runtime.Object{basicTier}
			initObjs = append(initObjs, murtest.NewMasterUserRecords(t, 20, "user-%d", murtest.Account("cluster1", *basicTier))...)
			initObjs = append(initObjs, turtest.NewTemplateUpdateRequests(nstemplatetier.MaxPoolSize, "user-%d", *basicTier)...)
			r, req, cl := prepareReconcile(t, basicTier.Name, initObjs...)
			// when
			res, err := r.Reconcile(req)
			// then
			require.NoError(t, err)
			require.Equal(t, reconcile.Result{}, res)
			// check that no TemplateUpdateRequest was created
			turtest.AssertThatTemplateUpdateRequests(t, cl).TotalCount(nstemplatetier.MaxPoolSize) // size unchanged
			tiertest.AssertThatNSTemplateTier(t, basicTier.Name, cl).
				HasStatusUpdatesItems(1).
				// at this point, all the counters are '0'
				HasLatestUpdate(toolchainv1alpha1.NSTemplateTierHistory{
					Hash:           basicTier.Labels["toolchain.dev.openshift.com/basic-tier-hash"],
					Failures:       2, // default
					FailedAccounts: []string{"failed1", "failed2"},
					CompletionTime: nil, // completion time should NOT be set
				})
		})

		// in this test, there are a more MasterUserRecords to update than `MaxPoolSize` allows, but
		// the max number of current TemplateRequestUpdate resources is reached (`MaxPoolSize`)
		t.Run("when maximum number of active TemplateUpdateRequest resources is reached", func(t *testing.T) {
			// given
			previousBasicTier := tiertest.BasicTier(t, tiertest.PreviousBasicTemplates)
			basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates, tiertest.WithCurrentUpdateInProgress())
			initObjs := []runtime.Object{basicTier}
			initObjs = append(initObjs, murtest.NewMasterUserRecords(t, 10, "user-%d", murtest.Account("cluster1", *previousBasicTier))...)
			initObjs = append(initObjs, turtest.NewTemplateUpdateRequests(nstemplatetier.MaxPoolSize, "user-%d", *basicTier)...)
			r, req, cl := prepareReconcile(t, basicTier.Name, initObjs...)
			// when
			res, err := r.Reconcile(req)
			// then
			require.NoError(t, err)
			require.Equal(t, reconcile.Result{}, res)
			// check that a single TemplateUpdateRequest was created
			turtest.AssertThatTemplateUpdateRequests(t, cl).TotalCount(nstemplatetier.MaxPoolSize) // no increase
			tiertest.AssertThatNSTemplateTier(t, basicTier.Name, cl).
				HasStatusUpdatesItems(1).
				// at this point, all the counters are '0'
				HasLatestUpdate(toolchainv1alpha1.NSTemplateTierHistory{
					Hash:           basicTier.Labels["toolchain.dev.openshift.com/basic-tier-hash"],
					Failures:       2, // default
					FailedAccounts: []string{"failed1", "failed2"},
				})
		})
	})

	// in these tests, the controller should NOT create a single TemplateUpdateRequest
	t.Run("controller should delete a TemplateUpdateRequest", func(t *testing.T) {

		// in this test, the controller can create an extra TemplateUpdateRequest resource
		// because one is in a "completed=true" status
		t.Run("when maximum number of TemplateUpdateRequest is reached but one is complete", func(t *testing.T) {
			// given
			previousBasicTier := tiertest.BasicTier(t, tiertest.PreviousBasicTemplates)
			basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates, tiertest.WithCurrentUpdateInProgress())
			initObjs := []runtime.Object{basicTier}
			initObjs = append(initObjs, murtest.NewMasterUserRecords(t, 10, "user-%d", murtest.Account("cluster1", *previousBasicTier))...)
			initObjs = append(initObjs, turtest.NewTemplateUpdateRequests(nstemplatetier.MaxPoolSize, "user-%d", *basicTier, turtest.Complete("user-0"))...)
			r, req, cl := prepareReconcile(t, basicTier.Name, initObjs...)
			// when
			res, err := r.Reconcile(req)
			// then
			require.NoError(t, err)
			require.Equal(t, reconcile.Result{}, res) // no need to explicit requeue after the creation
			// check that no TemplateUpdateRequest was created
			turtest.AssertThatTemplateUpdateRequests(t, cl).TotalCount(nstemplatetier.MaxPoolSize) // none created and one marked for deletion
			turtest.AssertThatTemplateUpdateRequest(t, "user-0", cl).HasDeletionTimestamp()
		})

		// in this test, the controller can't create an extra TemplateUpdateRequest resource yet
		// because one is in a "completed=true/reason=failed" status and has been deleted
		t.Run("when maximum number of TemplateUpdateRequest is reached but one is failed", func(t *testing.T) {
			// given
			previousBasicTier := tiertest.BasicTier(t, tiertest.PreviousBasicTemplates)
			basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates, tiertest.WithCurrentUpdateInProgress())
			initObjs := []runtime.Object{basicTier}
			initObjs = append(initObjs, murtest.NewMasterUserRecords(t, 10, "user-%d", murtest.Account("cluster1", *previousBasicTier))...)
			initObjs = append(initObjs, turtest.NewTemplateUpdateRequests(nstemplatetier.MaxPoolSize, "user-%d", *basicTier, turtest.Failed("user-0"))...)
			r, req, cl := prepareReconcile(t, basicTier.Name, initObjs...)
			// when
			res, err := r.Reconcile(req)
			// then
			require.NoError(t, err)
			require.Equal(t, reconcile.Result{}, res) // no need to explicit requeue after the creation
			// check that no TemplateUpdateRequest was created
			turtest.AssertThatTemplateUpdateRequests(t, cl).TotalCount(nstemplatetier.MaxPoolSize) // none created and one marked for deletion
			turtest.AssertThatTemplateUpdateRequest(t, "user-0", cl).HasDeletionTimestamp()
			tiertest.AssertThatNSTemplateTier(t, basicTier.Name, cl).
				HasStatusUpdatesItems(1).
				// at this point, all the counters are '0'
				HasLatestUpdate(toolchainv1alpha1.NSTemplateTierHistory{
					Hash:           basicTier.Labels["toolchain.dev.openshift.com/basic-tier-hash"],
					Failures:       3, // incremented
					FailedAccounts: []string{"failed1", "failed2", "user-0"},
				})
		})
	})
}

func prepareReconcile(t *testing.T, name string, initObjs ...runtime.Object) (reconcile.Reconciler, reconcile.Request, *test.FakeClient) {
	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)
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
	cl.MockDelete = func(ctx context.Context, obj runtime.Object, opts ...client.DeleteOption) error {
		if tur, ok := obj.(*toolchainv1alpha1.TemplateUpdateRequest); ok {
			t.Logf("setting deletion timestamp on TemplateUpdateRequest '%s'", tur.Name)
			now := metav1.Now()
			tur.SetDeletionTimestamp(&now)
			return cl.Client.Update(ctx, tur)
		}
		return fmt.Errorf("unexpected type of resource to delete: '%T'", obj)
	}

	r := nstemplatetier.NewReconciler(cl, s)
	return r, reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      name,
			Namespace: operatorNamespace,
		},
	}, cl
}
