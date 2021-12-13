package nstemplatetier_test

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/controllers/nstemplatetier"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	tiertest "github.com/codeready-toolchain/host-operator/test/nstemplatetier"
	spacetest "github.com/codeready-toolchain/host-operator/test/space"
	turtest "github.com/codeready-toolchain/host-operator/test/templateupdaterequest"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	murtest "github.com/codeready-toolchain/toolchain-common/pkg/test/masteruserrecord"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	operatorNamespace = "toolchain-host-operator"
	maxPoolSize       = 5 // same as default in config
)

func TestReconcile(t *testing.T) {

	// given
	logf.SetLogger(zap.New(zap.UseDevMode(true)))

	t.Run("controller should add entry in tier.status.updates", func(t *testing.T) {

		t.Run("without previous entry", func(t *testing.T) {
			// given
			basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates)
			initObjs := []runtime.Object{basicTier}
			r, req, cl := prepareReconcile(t, basicTier.Name, initObjs...)
			// when
			res, err := r.Reconcile(context.TODO(), req)
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
			res, err := r.Reconcile(context.TODO(), req)
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

		t.Run("when no TemplateUpdateRequest resource exists at all", func(t *testing.T) {
			// in this test, there are 10 MasterUserRecords but no associated TemplateUpdateRequest
			previousBasicTier := tiertest.BasicTier(t, tiertest.PreviousBasicTemplates)
			spaces := spacetest.NewSpaces(t, 10, "user-%d", spacetest.WithTierNameAndHashLabelFor(t, previousBasicTier))

			t.Run("MasterUserRecords", func(t *testing.T) {
				// TODO: this test should be removed once migration from MUR -> Spaces is completed.

				// given
				previousBasicTier := tiertest.BasicTier(t, tiertest.PreviousBasicTemplates)
				basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates, tiertest.WithCurrentUpdateInProgress())
				initObjs := append(murtest.NewMasterUserRecords(t, 10, "user-%d",
					murtest.Account("cluster1", *previousBasicTier)), basicTier)
				r, req, cl := prepareReconcile(t, basicTier.Name, append(initObjs, spaces...)...)
				// when
				res, err := r.Reconcile(context.TODO(), req)
				// then
				require.NoError(t, err)
				require.Equal(t, reconcile.Result{}, res) // no need to explicit requeue after the creation
				// check that a single TemplateUpdateRequest was created
				turtest.AssertThatTemplateUpdateRequests(t, cl).TotalCount(1)
				turtest.AssertThatTemplateUpdateRequest(t, "user-0", cl).Exists().HasOwnerReference()
			})

			// in this test, there are 10 Spaces but no associated TemplateUpdateRequest
			t.Run("Spaces", func(t *testing.T) {
				// given
				basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates, tiertest.WithCurrentUpdateInProgress())
				initObjs := append(spaces, basicTier)
				r, req, cl := prepareReconcile(t, basicTier.Name, initObjs...)
				// when
				res, err := r.Reconcile(context.TODO(), req)
				// then
				require.NoError(t, err)
				require.Equal(t, reconcile.Result{}, res) // no need to explicit requeue after the creation
				// check that a single TemplateUpdateRequest was created
				turtest.AssertThatTemplateUpdateRequests(t, cl).TotalCount(1)
				turtest.AssertThatTemplateUpdateRequest(t, "user-0", cl).Exists().HasOwnerReference()
			})
		})
		})

		// in this test, there are TemplateUpdateRequest resources but they are associated with the update of another NSTemplateTier
		t.Run("when no TemplateUpdateRequest resource exists for the given NSTemplateTier", func(t *testing.T) {
			t.Run("MasterUserRecords", func(t *testing.T) {
				// TODO: this test should be removed once migration from MUR -> Spaces is completed.

				// given
				previousBasicTier := tiertest.BasicTier(t, tiertest.PreviousBasicTemplates)
				basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates, tiertest.WithCurrentUpdateInProgress())
				initObjs := []runtime.Object{basicTier}
				initObjs = append(initObjs, murtest.NewMasterUserRecords(t, 10, "user-%d", murtest.Account("cluster1", *previousBasicTier))...)
				initObjs = append(initObjs, turtest.NewTemplateUpdateRequests(maxPoolSize, "other-%d", *basicTier, turtest.TierName("other"))...)
				r, req, cl := prepareReconcile(t, basicTier.Name, initObjs...)
				// when
				res, err := r.Reconcile(context.TODO(), req)
				// then
				require.NoError(t, err)
				require.Equal(t, reconcile.Result{}, res) // no need to explicit requeue after the creation
				// check that a single TemplateUpdateRequest was created
				turtest.AssertThatTemplateUpdateRequests(t, cl).TotalCount(maxPoolSize + 1) // 1 resource was created, `MaxPoolSize` already existed
			})
			t.Run("Spaces", func(t *testing.T) {
				// given
				previousBasicTier := tiertest.BasicTier(t, tiertest.PreviousBasicTemplates)
				basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates, tiertest.WithCurrentUpdateInProgress())
				initObjs := []runtime.Object{basicTier}
				initObjs = append(initObjs, spacetest.NewSpaces(t, 10, "user-%d", spacetest.WithTierNameAndHashLabelFor(t, previousBasicTier))...)
				initObjs = append(initObjs, turtest.NewTemplateUpdateRequests(maxPoolSize, "other-%d", *basicTier, turtest.TierName("other"))...)
				r, req, cl := prepareReconcile(t, basicTier.Name, initObjs...)
				// when
				res, err := r.Reconcile(context.TODO(), req)
				// then
				require.NoError(t, err)
				require.Equal(t, reconcile.Result{}, res) // no need to explicit requeue after the creation
				// check that a single TemplateUpdateRequest was created
				turtest.AssertThatTemplateUpdateRequests(t, cl).TotalCount(maxPoolSize + 1) // 1 resource was created, `MaxPoolSize` already existed
			})
		})

		// in this test, the controller can create an extra TemplateUpdateRequest resource
		// because one is being deleted
		t.Run("when maximum number of TemplateUpdateRequest is reached but one is being deleted", func(t *testing.T) {
			t.Run("MasterUserRecords", func(t *testing.T) {
				// TODO: this test should be removed once migration from MUR -> Spaces is completed.

				// given
				previousBasicTier := tiertest.BasicTier(t, tiertest.PreviousBasicTemplates)
				basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates, tiertest.WithCurrentUpdateInProgress())
				initObjs := []runtime.Object{basicTier}
				initObjs = append(initObjs, murtest.NewMasterUserRecords(t, 10, "user-%d", murtest.Account("cluster1", *previousBasicTier))...)
				initObjs = append(initObjs, turtest.NewTemplateUpdateRequests(maxPoolSize, "user-%d", *basicTier, turtest.DeletionTimestamp("user-0"))...)
				r, req, cl := prepareReconcile(t, basicTier.Name, initObjs...)
				// when
				res, err := r.Reconcile(context.TODO(), req)
				// then
				require.NoError(t, err)
				require.Equal(t, reconcile.Result{}, res) // no need to explicit requeue after the creation
				// check that a single TemplateUpdateRequest was created
				turtest.AssertThatTemplateUpdateRequests(t, cl).TotalCount(maxPoolSize + 1) // one more TemplateUpdateRequest
				turtest.AssertThatTemplateUpdateRequest(t, "user-5", cl).Exists().HasMasterUserRecordFieldsSetForTier(basicTier)
			})
			t.Run("Spaces", func(t *testing.T) {
				// given
				previousBasicTier := tiertest.BasicTier(t, tiertest.PreviousBasicTemplates)
				basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates, tiertest.WithCurrentUpdateInProgress())
				initObjs := []runtime.Object{basicTier}
				initObjs = append(initObjs, spacetest.NewSpaces(t, 10, "user-%d", spacetest.WithTierNameAndHashLabelFor(t, previousBasicTier))...)
				initObjs = append(initObjs, turtest.NewTemplateUpdateRequests(maxPoolSize, "user-%d", *basicTier, turtest.DeletionTimestamp("user-0"))...)
				r, req, cl := prepareReconcile(t, basicTier.Name, initObjs...)
				// when
				res, err := r.Reconcile(context.TODO(), req)
				// then
				require.NoError(t, err)
				require.Equal(t, reconcile.Result{}, res) // no need to explicit requeue after the creation
				// check that a single TemplateUpdateRequest was created
				turtest.AssertThatTemplateUpdateRequests(t, cl).TotalCount(maxPoolSize + 1) // one more TemplateUpdateRequest
				turtest.AssertThatTemplateUpdateRequest(t, "user-5", cl).Exists().HasSpaceFieldsSetForTier(basicTier)
			})
			t.Run("Both MasterUserRecords and Spaces", func(t *testing.T) {
				// this test checks that a TemplateUpdateRequest is created for Spaces after MasterUserRecords have been processed
				// TODO: this test should be removed once migration from MUR -> Spaces is completed.

				// given
				previousBasicTier := tiertest.BasicTier(t, tiertest.PreviousBasicTemplates)
				basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates, tiertest.WithCurrentUpdateInProgress())
				initObjs := []runtime.Object{basicTier}
				initObjs = append(initObjs, murtest.NewMasterUserRecords(t, 5, "user-%d", murtest.Account("cluster1", *previousBasicTier))...)
				initObjs = append(initObjs, spacetest.NewSpaces(t, 5, "spaceuser-%d", spacetest.WithTierNameAndHashLabelFor(t, previousBasicTier))...)
				// mark user-0 for deletion so that the next TemplateUpdateRequest can be created, all MasterUserRecords should have a TemplateUpdateRequest
				// so the next should be for a Space
				initObjs = append(initObjs, turtest.NewTemplateUpdateRequests(maxPoolSize, "user-%d", *basicTier, turtest.DeletionTimestamp("user-0"))...)
				r, req, cl := prepareReconcile(t, basicTier.Name, initObjs...)
				// when
				res, err := r.Reconcile(context.TODO(), req)
				// then
				require.NoError(t, err)
				require.Equal(t, reconcile.Result{}, res) // no need to explicit requeue after the creation
				// check that a single TemplateUpdateRequest was created
				turtest.AssertThatTemplateUpdateRequests(t, cl).TotalCount(maxPoolSize + 1) // one more TemplateUpdateRequest
				turtest.AssertThatTemplateUpdateRequest(t, "user-1", cl).Exists().HasMasterUserRecordFieldsSetForTier(basicTier)
				turtest.AssertThatTemplateUpdateRequest(t, "spaceuser-0", cl).Exists().HasSpaceFieldsSetForTier(basicTier)
			})
		})

		// in this test, there are 20 MasterUserRecords on the same tier.
		// The first 10 are already up-to-date, and the other ones which need to be updated
		t.Run("when MasterUserRecord in continued fetch is not up-to-date", func(t *testing.T) {
			t.Run("MasterUserRecords", func(t *testing.T) {
				// TODO: this test should be removed once migration from MUR -> Spaces is completed.

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
				res, err := r.Reconcile(context.TODO(), req)
				// then
				require.NoError(t, err)
				require.Equal(t, reconcile.Result{}, res) // no need to explicit requeue after the creation
				// check that a single TemplateUpdateRequest was created
				turtest.AssertThatTemplateUpdateRequests(t, cl).TotalCount(1) // one TemplateUpdateRequest created
			})
			t.Run("Spaces", func(t *testing.T) {
				// given
				previousBasicTier := tiertest.BasicTier(t, tiertest.PreviousBasicTemplates)
				basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates, tiertest.WithCurrentUpdateInProgress())
				initObjs := []runtime.Object{basicTier}
				initObjs = append(initObjs, spacetest.NewSpaces(t, 10, "new-user-%d", spacetest.WithTierNameAndHashLabelFor(t, basicTier))...)
				initObjs = append(initObjs, spacetest.NewSpaces(t, 10, "old-user-%d", spacetest.WithTierNameAndHashLabelFor(t, previousBasicTier))...)

				r, req, cl := prepareReconcile(t, basicTier.Name, initObjs...)
				// when
				res, err := r.Reconcile(context.TODO(), req)
				// then
				require.NoError(t, err)
				require.Equal(t, reconcile.Result{}, res) // no need to explicit requeue after the creation
				// check that a single TemplateUpdateRequest was created
				turtest.AssertThatTemplateUpdateRequests(t, cl).TotalCount(1) // one TemplateUpdateRequest created
			})
		})
	})

	// in these tests, the controller should NOT create a single TemplateUpdateRequest
	t.Run("controller should not create a TemplateUpdateRequest", func(t *testing.T) {

		// in this test, there is simply no MasterUserRecord or Space to update
		t.Run("when no MasterUserRecord or Space resource exists", func(t *testing.T) {
			// given
			basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates, tiertest.WithCurrentUpdateInProgress())
			r, req, cl := prepareReconcile(t, basicTier.Name, basicTier)
			// when
			res, err := r.Reconcile(context.TODO(), req)
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

		// in this test, all MasterUserRecords are associated with a different tier,
		// so none if them needs to be updated.
		t.Run("when none associated with the updated NSTemplateTier", func(t *testing.T) {
			t.Run("MasterUserRecords", func(t *testing.T) {
				// TODO: this test should be removed once migration from MUR -> Spaces is completed.

				// given
				basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates, tiertest.WithCurrentUpdateInProgress())
				otherTier := tiertest.OtherTier()
				initObjs := []runtime.Object{basicTier}
				initObjs = append(initObjs, murtest.NewMasterUserRecords(t, 10, "user-%d",
					murtest.Account("cluster1", *otherTier))...)
				r, req, cl := prepareReconcile(t, basicTier.Name, initObjs...)
				// when
				res, err := r.Reconcile(context.TODO(), req)
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
			t.Run("Spaces", func(t *testing.T) {
				// given
				basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates, tiertest.WithCurrentUpdateInProgress())
				otherTier := tiertest.OtherTier()
				initObjs := []runtime.Object{basicTier}
				initObjs = append(initObjs, spacetest.NewSpaces(t, 10, "user-%d", spacetest.WithTierNameAndHashLabelFor(t, otherTier))...)
				r, req, cl := prepareReconcile(t, basicTier.Name, initObjs...)
				// when
				res, err := r.Reconcile(context.TODO(), req)
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
		})

		// in this test, all existing MasterUserRecords or Spaces are already up-to-date
		t.Run("when all are up-to-date", func(t *testing.T) {
			t.Run("MasterUserRecords", func(t *testing.T) {
				// TODO: this test should be removed once migration from MUR -> Spaces is completed.

				// given
				basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates, tiertest.WithCurrentUpdateInProgress())
				initObjs := []runtime.Object{basicTier}
				initObjs = append(initObjs, murtest.NewMasterUserRecords(t, 20, "user-%d", murtest.Account("cluster1", *basicTier))...)
				r, req, cl := prepareReconcile(t, basicTier.Name, initObjs...)
				// when
				res, err := r.Reconcile(context.TODO(), req)
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
			t.Run("Spaces", func(t *testing.T) {
				// given
				basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates, tiertest.WithCurrentUpdateInProgress())
				initObjs := []runtime.Object{basicTier}
				initObjs = append(initObjs, spacetest.NewSpaces(t, 20, "user-%d", spacetest.WithTierNameAndHashLabelFor(t, basicTier))...)
				r, req, cl := prepareReconcile(t, basicTier.Name, initObjs...)
				// when
				res, err := r.Reconcile(context.TODO(), req)
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
		})

		// in this test, all MasterUserRecords/Spaces are being updated,
		// i.e., there is already an associated TemplateUpdateRequest
		t.Run("when all are being updated", func(t *testing.T) {
			t.Run("MasterUserRecords", func(t *testing.T) {
				// TODO: this test should be removed once migration from MUR -> Spaces is completed.

				// given
				basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates, tiertest.WithCurrentUpdateInProgress())
				initObjs := []runtime.Object{basicTier}
				initObjs = append(initObjs, murtest.NewMasterUserRecords(t, 20, "user-%d", murtest.Account("cluster1", *basicTier))...)
				initObjs = append(initObjs, turtest.NewTemplateUpdateRequests(maxPoolSize, "user-%d", *basicTier)...)
				r, req, cl := prepareReconcile(t, basicTier.Name, initObjs...)
				// when
				res, err := r.Reconcile(context.TODO(), req)
				// then
				require.NoError(t, err)
				require.Equal(t, reconcile.Result{}, res)
				// check that no TemplateUpdateRequest was created
				turtest.AssertThatTemplateUpdateRequests(t, cl).TotalCount(maxPoolSize) // size unchanged
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
			t.Run("Spaces", func(t *testing.T) {
				// given
				basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates, tiertest.WithCurrentUpdateInProgress())
				initObjs := []runtime.Object{basicTier}
				initObjs = append(initObjs, spacetest.NewSpaces(t, 20, "user-%d", spacetest.WithTierNameAndHashLabelFor(t, basicTier))...)
				initObjs = append(initObjs, turtest.NewTemplateUpdateRequests(maxPoolSize, "user-%d", *basicTier)...)
				r, req, cl := prepareReconcile(t, basicTier.Name, initObjs...)
				// when
				res, err := r.Reconcile(context.TODO(), req)
				// then
				require.NoError(t, err)
				require.Equal(t, reconcile.Result{}, res)
				// check that no TemplateUpdateRequest was created
				turtest.AssertThatTemplateUpdateRequests(t, cl).TotalCount(maxPoolSize) // size unchanged
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
		})

		// in this test, there are a more MasterUserRecords/Spaces to update than `MaxPoolSize` allows, but
		// the max number of current TemplateRequestUpdate resources is reached (`MaxPoolSize`)
		t.Run("when maximum number of active TemplateUpdateRequest resources is reached", func(t *testing.T) {
			t.Run("MasterUserRecords", func(t *testing.T) {
				// TODO: this test should be removed once migration from MUR -> Spaces is completed.

				// given
				previousBasicTier := tiertest.BasicTier(t, tiertest.PreviousBasicTemplates)
				basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates, tiertest.WithCurrentUpdateInProgress())
				initObjs := []runtime.Object{basicTier}
				initObjs = append(initObjs, murtest.NewMasterUserRecords(t, 10, "user-%d", murtest.Account("cluster1", *previousBasicTier))...)
				initObjs = append(initObjs, turtest.NewTemplateUpdateRequests(maxPoolSize, "user-%d", *basicTier)...)
				r, req, cl := prepareReconcile(t, basicTier.Name, initObjs...)
				// when
				res, err := r.Reconcile(context.TODO(), req)
				// then
				require.NoError(t, err)
				require.Equal(t, reconcile.Result{}, res)
				// check that a single TemplateUpdateRequest was created
				turtest.AssertThatTemplateUpdateRequests(t, cl).TotalCount(maxPoolSize) // no increase
				tiertest.AssertThatNSTemplateTier(t, basicTier.Name, cl).
					HasStatusUpdatesItems(1).
					// at this point, all the counters are '0'
					HasLatestUpdate(toolchainv1alpha1.NSTemplateTierHistory{
						Hash:           basicTier.Labels["toolchain.dev.openshift.com/basic-tier-hash"],
						Failures:       2, // default
						FailedAccounts: []string{"failed1", "failed2"},
					})
			})
			t.Run("Spaces", func(t *testing.T) {
				// given
				previousBasicTier := tiertest.BasicTier(t, tiertest.PreviousBasicTemplates)
				basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates, tiertest.WithCurrentUpdateInProgress())
				initObjs := []runtime.Object{basicTier}
				initObjs = append(initObjs, spacetest.NewSpaces(t, 10, "user-%d", spacetest.WithTierNameAndHashLabelFor(t, previousBasicTier))...)
				initObjs = append(initObjs, turtest.NewTemplateUpdateRequests(maxPoolSize, "user-%d", *basicTier)...)
				r, req, cl := prepareReconcile(t, basicTier.Name, initObjs...)
				// when
				res, err := r.Reconcile(context.TODO(), req)
				// then
				require.NoError(t, err)
				require.Equal(t, reconcile.Result{}, res)
				// check that a single TemplateUpdateRequest was created
				turtest.AssertThatTemplateUpdateRequests(t, cl).TotalCount(maxPoolSize) // no increase
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

		// in this test, the controller can't create an extra TemplateUpdateRequest resource yet
		// because one is in a "completed=true/reason=failed" status and has been deleted
		t.Run("when maximum number of TemplateUpdateRequest is reached but one is failed once", func(t *testing.T) {
			t.Run("MasterUserRecords", func(t *testing.T) {
				// TODO: this test should be removed once migration from MUR -> Spaces is completed.

				// given
				previousBasicTier := tiertest.BasicTier(t, tiertest.PreviousBasicTemplates)
				basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates, tiertest.WithCurrentUpdateInProgress())
				initObjs := []runtime.Object{basicTier}
				initObjs = append(initObjs, murtest.NewMasterUserRecords(t, 10, "user-%d", murtest.Account("cluster1", *previousBasicTier))...)
				initObjs = append(initObjs, turtest.NewTemplateUpdateRequests(maxPoolSize, "user-%d", *basicTier, turtest.Failed("user-0"))...)
				r, req, cl := prepareReconcile(t, basicTier.Name, initObjs...)
				// when
				res, err := r.Reconcile(context.TODO(), req)
				// then
				require.NoError(t, err)
				require.Equal(t, reconcile.Result{}, res) // no need to explicit requeue after the creation
				// check that no TemplateUpdateRequest was created
				turtest.AssertThatTemplateUpdateRequests(t, cl).TotalCount(maxPoolSize) // none created and one marked for deletion
				turtest.AssertThatTemplateUpdateRequest(t, "user-0", cl).Exists()
				tiertest.AssertThatNSTemplateTier(t, basicTier.Name, cl).
					HasStatusUpdatesItems(1).
					// at this point, all the counters are '0'
					HasLatestUpdate(toolchainv1alpha1.NSTemplateTierHistory{
						Hash:           basicTier.Labels["toolchain.dev.openshift.com/basic-tier-hash"],
						Failures:       2,                              // not incremented (yet)
						FailedAccounts: []string{"failed1", "failed2"}, // unchanged
					})
			})
			t.Run("Spaces", func(t *testing.T) {
				// given
				previousBasicTier := tiertest.BasicTier(t, tiertest.PreviousBasicTemplates)
				basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates, tiertest.WithCurrentUpdateInProgress())
				initObjs := []runtime.Object{basicTier}
				initObjs = append(initObjs, spacetest.NewSpaces(t, 10, "user-%d", spacetest.WithTierNameAndHashLabelFor(t, previousBasicTier))...)
				initObjs = append(initObjs, turtest.NewTemplateUpdateRequests(maxPoolSize, "user-%d", *basicTier, turtest.Failed("user-0"))...)
				r, req, cl := prepareReconcile(t, basicTier.Name, initObjs...)
				// when
				res, err := r.Reconcile(context.TODO(), req)
				// then
				require.NoError(t, err)
				require.Equal(t, reconcile.Result{}, res) // no need to explicit requeue after the creation
				// check that no TemplateUpdateRequest was created
				turtest.AssertThatTemplateUpdateRequests(t, cl).TotalCount(maxPoolSize) // none created and one marked for deletion
				turtest.AssertThatTemplateUpdateRequest(t, "user-0", cl).Exists()
				tiertest.AssertThatNSTemplateTier(t, basicTier.Name, cl).
					HasStatusUpdatesItems(1).
					// at this point, all the counters are '0'
					HasLatestUpdate(toolchainv1alpha1.NSTemplateTierHistory{
						Hash:           basicTier.Labels["toolchain.dev.openshift.com/basic-tier-hash"],
						Failures:       2,                              // not incremented (yet)
						FailedAccounts: []string{"failed1", "failed2"}, // unchanged
					})
			})
		})
	})

	// in these tests, the controller should NOT create a single TemplateUpdateRequest
	t.Run("controller should delete a TemplateUpdateRequest", func(t *testing.T) {

		// in this test, the controller can create an extra TemplateUpdateRequest resource
		// because one is in a "completed=true" status
		t.Run("when maximum number of TemplateUpdateRequest is reached but one is complete", func(t *testing.T) {
			t.Run("MasterUserRecords", func(t *testing.T) {
				// TODO: this test should be removed once migration from MUR -> Spaces is completed.

				// given
				previousBasicTier := tiertest.BasicTier(t, tiertest.PreviousBasicTemplates)
				basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates, tiertest.WithCurrentUpdateInProgress())
				initObjs := []runtime.Object{basicTier}
				initObjs = append(initObjs, murtest.NewMasterUserRecords(t, 10, "user-%d", murtest.Account("cluster1", *previousBasicTier))...)
				initObjs = append(initObjs, turtest.NewTemplateUpdateRequests(maxPoolSize, "user-%d", *basicTier, turtest.Complete("user-0"))...)
				r, req, cl := prepareReconcile(t, basicTier.Name, initObjs...)
				// when
				res, err := r.Reconcile(context.TODO(), req)
				// then
				require.NoError(t, err)
				require.Equal(t, reconcile.Result{}, res) // no need to explicit requeue after the creation
				// check that no TemplateUpdateRequest was created
				turtest.AssertThatTemplateUpdateRequests(t, cl).TotalCount(maxPoolSize) // none created and one marked for deletion
				turtest.AssertThatTemplateUpdateRequest(t, "user-0", cl).HasDeletionTimestamp()
			})
			t.Run("Spaces", func(t *testing.T) {
				// given
				previousBasicTier := tiertest.BasicTier(t, tiertest.PreviousBasicTemplates)
				basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates, tiertest.WithCurrentUpdateInProgress())
				initObjs := []runtime.Object{basicTier}
				initObjs = append(initObjs, spacetest.NewSpaces(t, 10, "user-%d", spacetest.WithTierNameAndHashLabelFor(t, previousBasicTier))...)
				initObjs = append(initObjs, turtest.NewTemplateUpdateRequests(maxPoolSize, "user-%d", *basicTier, turtest.Complete("user-0"))...)
				r, req, cl := prepareReconcile(t, basicTier.Name, initObjs...)
				// when
				res, err := r.Reconcile(context.TODO(), req)
				// then
				require.NoError(t, err)
				require.Equal(t, reconcile.Result{}, res) // no need to explicit requeue after the creation
				// check that no TemplateUpdateRequest was created
				turtest.AssertThatTemplateUpdateRequests(t, cl).TotalCount(maxPoolSize) // none created and one marked for deletion
				turtest.AssertThatTemplateUpdateRequest(t, "user-0", cl).HasDeletionTimestamp()
			})
		})

		// in this test, the controller can create an extra TemplateUpdateRequest resource
		// because one is in a "completed=true" status
		t.Run("when maximum number of TemplateUpdateRequest is reached but first is deleted and second is complete", func(t *testing.T) {
			t.Run("MasterUserRecords", func(t *testing.T) {
				// TODO: this test should be removed once migration from MUR -> Spaces is completed.

				// given
				previousBasicTier := tiertest.BasicTier(t, tiertest.PreviousBasicTemplates)
				basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates, tiertest.WithCurrentUpdateInProgress())
				initObjs := []runtime.Object{basicTier}
				initObjs = append(initObjs, murtest.NewMasterUserRecords(t, 10, "user-%d", murtest.Account("cluster1", *previousBasicTier))...)
				initObjs = append(initObjs, turtest.NewTemplateUpdateRequests(maxPoolSize, "user-%d", *basicTier, turtest.Complete("user-0"), turtest.Complete("user-1"))...)
				r, req, cl := prepareReconcile(t, basicTier.Name, initObjs...)
				cl.MockDelete = func(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
					if tur, ok := obj.(*toolchainv1alpha1.TemplateUpdateRequest); ok {
						if tur.Name == "user-0" {
							return errors.NewNotFound(schema.GroupResource{}, "user-0")
						}
						t.Logf("setting deletion timestamp on TemplateUpdateRequest '%s'", tur.Name)
						now := metav1.Now()
						tur.SetDeletionTimestamp(&now)
						return cl.Client.Update(ctx, tur)
					}
					return fmt.Errorf("unexpected type of resource to delete: '%T'", obj)
				}

				// when
				res, err := r.Reconcile(context.TODO(), req)
				// then
				require.NoError(t, err)
				require.Equal(t, reconcile.Result{}, res) // no need to explicit requeue after the creation
				// check that no TemplateUpdateRequest was created
				turtest.AssertThatTemplateUpdateRequests(t, cl).TotalCount(maxPoolSize)         // none created and one marked for deletion
				turtest.AssertThatTemplateUpdateRequest(t, "user-1", cl).HasDeletionTimestamp() // "user-1" is the one marked for deletion
			})
			t.Run("Spaces", func(t *testing.T) {
				// given
				previousBasicTier := tiertest.BasicTier(t, tiertest.PreviousBasicTemplates)
				basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates, tiertest.WithCurrentUpdateInProgress())
				initObjs := []runtime.Object{basicTier}
				initObjs = append(initObjs, spacetest.NewSpaces(t, 10, "user-%d", spacetest.WithTierNameAndHashLabelFor(t, previousBasicTier))...)
				initObjs = append(initObjs, turtest.NewTemplateUpdateRequests(maxPoolSize, "user-%d", *basicTier, turtest.Complete("user-0"), turtest.Complete("user-1"))...)
				r, req, cl := prepareReconcile(t, basicTier.Name, initObjs...)
				cl.MockDelete = func(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
					if tur, ok := obj.(*toolchainv1alpha1.TemplateUpdateRequest); ok {
						if tur.Name == "user-0" {
							return errors.NewNotFound(schema.GroupResource{}, "user-0")
						}
						t.Logf("setting deletion timestamp on TemplateUpdateRequest '%s'", tur.Name)
						now := metav1.Now()
						tur.SetDeletionTimestamp(&now)
						return cl.Client.Update(ctx, tur)
					}
					return fmt.Errorf("unexpected type of resource to delete: '%T'", obj)
				}

				// when
				res, err := r.Reconcile(context.TODO(), req)
				// then
				require.NoError(t, err)
				require.Equal(t, reconcile.Result{}, res) // no need to explicit requeue after the creation
				// check that no TemplateUpdateRequest was created
				turtest.AssertThatTemplateUpdateRequests(t, cl).TotalCount(maxPoolSize)         // none created and one marked for deletion
				turtest.AssertThatTemplateUpdateRequest(t, "user-1", cl).HasDeletionTimestamp() // "user-1" is the one marked for deletion
			})
		})

		// in this test, the controller can't create an extra TemplateUpdateRequest resource yet
		// because one is in a "completed=true/reason=failed" status and has been deleted
		t.Run("when maximum number of TemplateUpdateRequest is reached but one is failed too many times", func(t *testing.T) {
			t.Run("MasterUserRecords", func(t *testing.T) {
				// TODO: this test should be removed once migration from MUR -> Spaces is completed.

				// given
				previousBasicTier := tiertest.BasicTier(t, tiertest.PreviousBasicTemplates)
				basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates, tiertest.WithCurrentUpdateInProgress())
				initObjs := []runtime.Object{basicTier}
				initObjs = append(initObjs, murtest.NewMasterUserRecords(t, 10, "user-%d", murtest.Account("cluster1", *previousBasicTier))...)
				initObjs = append(initObjs, turtest.NewTemplateUpdateRequests(maxPoolSize, "user-%d", *basicTier, turtest.Failed("user-0"), turtest.Failed("user-0"))...) // "user-0" failed twice
				r, req, cl := prepareReconcile(t, basicTier.Name, initObjs...)
				// when
				res, err := r.Reconcile(context.TODO(), req)
				// then
				require.NoError(t, err)
				require.Equal(t, reconcile.Result{}, res) // no need to explicit requeue after the creation
				// check that no TemplateUpdateRequest was created
				turtest.AssertThatTemplateUpdateRequests(t, cl).TotalCount(maxPoolSize) // none created and one marked for deletion
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
			t.Run("Spaces", func(t *testing.T) {
				// given
				previousBasicTier := tiertest.BasicTier(t, tiertest.PreviousBasicTemplates)
				basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates, tiertest.WithCurrentUpdateInProgress())
				initObjs := []runtime.Object{basicTier}
				initObjs = append(initObjs, spacetest.NewSpaces(t, 10, "user-%d", spacetest.WithTierNameAndHashLabelFor(t, previousBasicTier))...)
				initObjs = append(initObjs, turtest.NewTemplateUpdateRequests(maxPoolSize, "user-%d", *basicTier, turtest.Failed("user-0"), turtest.Failed("user-0"))...) // "user-0" failed twice
				r, req, cl := prepareReconcile(t, basicTier.Name, initObjs...)
				// when
				res, err := r.Reconcile(context.TODO(), req)
				// then
				require.NoError(t, err)
				require.Equal(t, reconcile.Result{}, res) // no need to explicit requeue after the creation
				// check that no TemplateUpdateRequest was created
				turtest.AssertThatTemplateUpdateRequests(t, cl).TotalCount(maxPoolSize) // none created and one marked for deletion
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
	})

	t.Run("failures", func(t *testing.T) {

		t.Run("unable to get NSTemplateTier", func(t *testing.T) {

			t.Run("tier not found", func(t *testing.T) {
				// given
				basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates)
				initObjs := []runtime.Object{basicTier}
				r, req, cl := prepareReconcile(t, basicTier.Name, initObjs...)
				cl.MockGet = func(ctx context.Context, key types.NamespacedName, obj client.Object) error {
					if _, ok := obj.(*toolchainv1alpha1.NSTemplateTier); ok {
						return errors.NewNotFound(schema.GroupResource{}, key.Name)
					}
					return cl.Client.Get(ctx, key, obj)
				}
				// when
				res, err := r.Reconcile(context.TODO(), req)
				// then
				require.NoError(t, err)
				assert.Equal(t, reconcile.Result{}, res) // no explicit requeue
			})

			t.Run("other error", func(t *testing.T) {
				// given
				basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates)
				initObjs := []runtime.Object{basicTier}
				r, req, cl := prepareReconcile(t, basicTier.Name, initObjs...)
				cl.MockGet = func(ctx context.Context, key types.NamespacedName, obj client.Object) error {
					if _, ok := obj.(*toolchainv1alpha1.NSTemplateTier); ok {
						return fmt.Errorf("mock error")
					}
					return cl.Client.Get(ctx, key, obj)
				}
				// when
				res, err := r.Reconcile(context.TODO(), req)
				// then
				require.Error(t, err)
				assert.EqualError(t, err, "unable to get the current NSTemplateTier: mock error")
				assert.Equal(t, reconcile.Result{}, res) // no explicit requeue
			})
		})

		t.Run("unable to list MasterUserRecords", func(t *testing.T) {
			// TODO: this test should be removed once migration from MUR -> Spaces is completed.

			// given
			previousBasicTier := tiertest.BasicTier(t, tiertest.PreviousBasicTemplates)
			basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates, tiertest.WithCurrentUpdateInProgress())
			initObjs := []runtime.Object{basicTier}
			initObjs = append(initObjs, murtest.NewMasterUserRecords(t, 10, "new-user-%d",
				murtest.Account("cluster1", *basicTier))...)
			initObjs = append(initObjs, murtest.NewMasterUserRecords(t, 10, "old-user-%d",
				murtest.Account("cluster1", *previousBasicTier))...)

			r, req, cl := prepareReconcile(t, basicTier.Name, initObjs...)
			cl.MockList = func(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
				if _, ok := list.(*toolchainv1alpha1.MasterUserRecordList); ok {
					return fmt.Errorf("mock error")
				}
				return cl.Client.List(ctx, list, opts...)
			}
			// when
			res, err := r.Reconcile(context.TODO(), req)
			// then
			require.Error(t, err)
			assert.EqualError(t, err, "unable to ensure TemplateRequestUpdate resource after NSTemplateTier changed: unable to get MasterUserRecords to update: mock error")
			assert.Equal(t, reconcile.Result{}, res) // no explicit requeue
		})

		t.Run("unable to list Spaces", func(t *testing.T) {
			// given
			previousBasicTier := tiertest.BasicTier(t, tiertest.PreviousBasicTemplates)
			basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates, tiertest.WithCurrentUpdateInProgress())
			initObjs := []runtime.Object{basicTier}
			initObjs = append(initObjs, spacetest.NewSpaces(t, 10, "new-user-%d", spacetest.WithTierNameAndHashLabelFor(t, basicTier))...)
			initObjs = append(initObjs, spacetest.NewSpaces(t, 10, "old-user-%d", spacetest.WithTierNameAndHashLabelFor(t, previousBasicTier))...)

			r, req, cl := prepareReconcile(t, basicTier.Name, initObjs...)
			cl.MockList = func(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
				if _, ok := list.(*toolchainv1alpha1.SpaceList); ok {
					return fmt.Errorf("mock error")
				}
				return cl.Client.List(ctx, list, opts...)
			}
			// when
			res, err := r.Reconcile(context.TODO(), req)
			// then
			require.Error(t, err)
			assert.EqualError(t, err, "unable to ensure TemplateRequestUpdate resource after NSTemplateTier changed: unable to get Spaces to update: mock error")
			assert.Equal(t, reconcile.Result{}, res) // no explicit requeue
		})

		t.Run("unable to list TemplateUpdateRequests", func(t *testing.T) {
			t.Run("MasterUserRecords", func(t *testing.T) {
				// TODO: this test should be removed once migration from MUR -> Spaces is completed.

				// given
				previousBasicTier := tiertest.BasicTier(t, tiertest.PreviousBasicTemplates)
				basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates, tiertest.WithCurrentUpdateInProgress())
				initObjs := []runtime.Object{basicTier}
				initObjs = append(initObjs, murtest.NewMasterUserRecords(t, 10, "user-%d",
					murtest.Account("cluster1", *previousBasicTier))...)

				r, req, cl := prepareReconcile(t, basicTier.Name, initObjs...)
				cl.MockList = func(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
					if _, ok := list.(*toolchainv1alpha1.TemplateUpdateRequestList); ok {
						return fmt.Errorf("mock error")
					}
					return cl.Client.List(ctx, list, opts...)
				}
				// when
				res, err := r.Reconcile(context.TODO(), req)
				// then
				require.Error(t, err)
				assert.EqualError(t, err, "unable to ensure TemplateRequestUpdate resource after NSTemplateTier changed: unable to get active TemplateUpdateRequests: mock error")
				assert.Equal(t, reconcile.Result{}, res) // no explicit requeue
			})
			t.Run("Spaces", func(t *testing.T) {
				// given
				previousBasicTier := tiertest.BasicTier(t, tiertest.PreviousBasicTemplates)
				basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates, tiertest.WithCurrentUpdateInProgress())
				initObjs := []runtime.Object{basicTier}
				initObjs = append(initObjs, spacetest.NewSpaces(t, 10, "user-%d", spacetest.WithTierNameAndHashLabelFor(t, previousBasicTier))...)

				r, req, cl := prepareReconcile(t, basicTier.Name, initObjs...)
				cl.MockList = func(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
					if _, ok := list.(*toolchainv1alpha1.TemplateUpdateRequestList); ok {
						return fmt.Errorf("mock error")
					}
					return cl.Client.List(ctx, list, opts...)
				}
				// when
				res, err := r.Reconcile(context.TODO(), req)
				// then
				require.Error(t, err)
				assert.EqualError(t, err, "unable to ensure TemplateRequestUpdate resource after NSTemplateTier changed: unable to get active TemplateUpdateRequests: mock error")
				assert.Equal(t, reconcile.Result{}, res) // no explicit requeue
			})
		})

		t.Run("unable to get TemplateUpdateRequest associated with MasterUserRecord/Space", func(t *testing.T) {
			t.Run("MasterUserRecords", func(t *testing.T) {
				// TODO: this test should be removed once migration from MUR -> Spaces is completed.

				// given
				previousBasicTier := tiertest.BasicTier(t, tiertest.PreviousBasicTemplates)
				basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates, tiertest.WithCurrentUpdateInProgress())
				initObjs := []runtime.Object{basicTier}
				initObjs = append(initObjs, murtest.NewMasterUserRecords(t, 10, "user-%d",
					murtest.Account("cluster1", *previousBasicTier))...)

				r, req, cl := prepareReconcile(t, basicTier.Name, initObjs...)
				cl.MockGet = func(ctx context.Context, key types.NamespacedName, obj client.Object) error {
					if _, ok := obj.(*toolchainv1alpha1.TemplateUpdateRequest); ok {
						return fmt.Errorf("mock error") // must not be a `NotFoundError` in this test
					}
					return cl.Client.Get(ctx, key, obj)
				}
				// when
				res, err := r.Reconcile(context.TODO(), req)
				// then
				require.Error(t, err)
				assert.EqualError(t, err, "unable to ensure TemplateRequestUpdate resource after NSTemplateTier changed: unable to get TemplateUpdateRequest for MasterUserRecord 'user-0': mock error")
				assert.Equal(t, reconcile.Result{}, res) // no explicit requeue
			})
			t.Run("Spaces", func(t *testing.T) {
				// given
				previousBasicTier := tiertest.BasicTier(t, tiertest.PreviousBasicTemplates)
				basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates, tiertest.WithCurrentUpdateInProgress())
				initObjs := []runtime.Object{basicTier}
				initObjs = append(initObjs, spacetest.NewSpaces(t, 10, "user-%d", spacetest.WithTierNameAndHashLabelFor(t, previousBasicTier))...)

				r, req, cl := prepareReconcile(t, basicTier.Name, initObjs...)
				cl.MockGet = func(ctx context.Context, key types.NamespacedName, obj client.Object) error {
					if _, ok := obj.(*toolchainv1alpha1.TemplateUpdateRequest); ok {
						return fmt.Errorf("mock error") // must not be a `NotFoundError` in this test
					}
					return cl.Client.Get(ctx, key, obj)
				}
				// when
				res, err := r.Reconcile(context.TODO(), req)
				// then
				require.Error(t, err)
				assert.EqualError(t, err, "unable to ensure TemplateRequestUpdate resource after NSTemplateTier changed: unable to get TemplateUpdateRequest for Space 'user-0': mock error")
				assert.Equal(t, reconcile.Result{}, res) // no explicit requeue
			})
		})

		t.Run("unable to delete TemplateUpdateRequest associated with MasterUserRecord/Space", func(t *testing.T) {
			t.Run("MasterUserRecords", func(t *testing.T) {
				// TODO: this test should be removed once migration from MUR -> Spaces is completed.

				// given
				previousBasicTier := tiertest.BasicTier(t, tiertest.PreviousBasicTemplates)
				basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates, tiertest.WithCurrentUpdateInProgress())
				initObjs := []runtime.Object{basicTier}
				initObjs = append(initObjs, murtest.NewMasterUserRecords(t, 10, "user-%d",
					murtest.Account("cluster1", *previousBasicTier))...)
				initObjs = append(initObjs, turtest.NewTemplateUpdateRequests(maxPoolSize, "user-%d", *basicTier, turtest.Complete("user-0"))...)

				r, req, cl := prepareReconcile(t, basicTier.Name, initObjs...)
				cl.MockDelete = func(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
					if _, ok := obj.(*toolchainv1alpha1.TemplateUpdateRequest); ok {
						return fmt.Errorf("mock error") // must not be a `NotFoundError` in this test
					}
					return cl.Client.Delete(ctx, obj, opts...)
				}
				// when
				res, err := r.Reconcile(context.TODO(), req)
				// then
				require.Error(t, err)
				assert.EqualError(t, err, "unable to ensure TemplateRequestUpdate resource after NSTemplateTier changed: unable to get active TemplateUpdateRequests: unable to delete the TemplateUpdateRequest resource 'user-0': mock error")
				assert.Equal(t, reconcile.Result{}, res) // no explicit requeue
			})
			t.Run("Spaces", func(t *testing.T) {
				// given
				previousBasicTier := tiertest.BasicTier(t, tiertest.PreviousBasicTemplates)
				basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates, tiertest.WithCurrentUpdateInProgress())
				initObjs := []runtime.Object{basicTier}
				initObjs = append(initObjs, spacetest.NewSpaces(t, 10, "user-%d", spacetest.WithTierNameAndHashLabelFor(t, previousBasicTier))...)
				initObjs = append(initObjs, turtest.NewTemplateUpdateRequests(maxPoolSize, "user-%d", *basicTier, turtest.Complete("user-0"))...)

				r, req, cl := prepareReconcile(t, basicTier.Name, initObjs...)
				cl.MockDelete = func(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
					if _, ok := obj.(*toolchainv1alpha1.TemplateUpdateRequest); ok {
						return fmt.Errorf("mock error") // must not be a `NotFoundError` in this test
					}
					return cl.Client.Delete(ctx, obj, opts...)
				}
				// when
				res, err := r.Reconcile(context.TODO(), req)
				// then
				require.Error(t, err)
				assert.EqualError(t, err, "unable to ensure TemplateRequestUpdate resource after NSTemplateTier changed: unable to get active TemplateUpdateRequests: unable to delete the TemplateUpdateRequest resource 'user-0': mock error")
				assert.Equal(t, reconcile.Result{}, res) // no explicit requeue
			})
		})

		t.Run("unable to update NSTemplateTier status", func(t *testing.T) {

			t.Run("when starting update process", func(t *testing.T) {
				t.Run("MasterUserRecords", func(t *testing.T) {
					// TODO: this test should be removed once migration from MUR -> Spaces is completed.

					// given
					basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates)
					initObjs := []runtime.Object{basicTier}
					initObjs = append(initObjs, murtest.NewMasterUserRecords(t, 10, "new-user-%d",
						murtest.Account("cluster1", *basicTier))...)
					r, req, cl := prepareReconcile(t, basicTier.Name, initObjs...)
					cl.MockStatusUpdate = func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
						if _, ok := obj.(*toolchainv1alpha1.NSTemplateTier); ok {
							return fmt.Errorf("mock error")
						}
						return cl.Client.Status().Update(ctx, obj, opts...)
					}
					// when
					res, err := r.Reconcile(context.TODO(), req)
					// then
					require.Error(t, err)
					assert.EqualError(t, err, "unable to insert a new entry in status.updates after NSTemplateTier changed: mock error")
					assert.Equal(t, reconcile.Result{}, res) // no explicit requeue
				})
				t.Run("Spaces", func(t *testing.T) {
					// given
					basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates)
					initObjs := []runtime.Object{basicTier}
					initObjs = append(initObjs, spacetest.NewSpaces(t, 10, "new-user-%d", spacetest.WithTierNameAndHashLabelFor(t, basicTier))...)
					r, req, cl := prepareReconcile(t, basicTier.Name, initObjs...)
					cl.MockStatusUpdate = func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
						if _, ok := obj.(*toolchainv1alpha1.NSTemplateTier); ok {
							return fmt.Errorf("mock error")
						}
						return cl.Client.Status().Update(ctx, obj, opts...)
					}
					// when
					res, err := r.Reconcile(context.TODO(), req)
					// then
					require.Error(t, err)
					assert.EqualError(t, err, "unable to insert a new entry in status.updates after NSTemplateTier changed: mock error")
					assert.Equal(t, reconcile.Result{}, res) // no explicit requeue
				})
			})

			t.Run("when marking update process as complete", func(t *testing.T) {
				// given
				basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates, tiertest.WithCurrentUpdateInProgress())
				initObjs := []runtime.Object{basicTier}
				r, req, cl := prepareReconcile(t, basicTier.Name, initObjs...)
				cl.MockStatusUpdate = func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
					if _, ok := obj.(*toolchainv1alpha1.NSTemplateTier); ok {
						return fmt.Errorf("mock error")
					}
					return cl.Client.Status().Update(ctx, obj, opts...)
				}
				// when
				res, err := r.Reconcile(context.TODO(), req)
				// then
				require.Error(t, err)
				assert.EqualError(t, err, "unable to mark latest status.update as complete: mock error")
				assert.Equal(t, reconcile.Result{}, res) // no explicit requeue
			})
		})

	})

}

func prepareReconcile(t *testing.T, name string, initObjs ...runtime.Object) (reconcile.Reconciler, reconcile.Request, *test.FakeClient) {
	os.Setenv("WATCH_NAMESPACE", test.HostOperatorNs)
	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)
	cl := test.NewFakeClient(t, initObjs...)
	// (partial) support the `limit` and `continue` when listing MasterUserRecords
	// Here, the result's `continue` is the initial `continue` + `limit`
	cl.MockList = func(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
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
		if spaces, ok := list.(*toolchainv1alpha1.SpaceList); ok {
			c := 0
			if spaces.Continue != "" {
				c, err = strconv.Atoi(spaces.Continue)
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
				spaces.Items = spaces.Items[c:]
			}
			if int(listOpts.Limit) < len(spaces.Items) {
				// keep the first items and remove the following ones to fit into the limit
				spaces.Items = spaces.Items[:listOpts.Limit]
			}
			spaces.Continue = strconv.Itoa(c + int(listOpts.Limit))
			return nil
		}
		// default behaviour
		return cl.Client.List(ctx, list, opts...)
	}
	cl.MockDelete = func(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
		if tur, ok := obj.(*toolchainv1alpha1.TemplateUpdateRequest); ok {
			t.Logf("setting deletion timestamp on TemplateUpdateRequest '%s'", tur.Name)
			now := metav1.Now()
			tur.SetDeletionTimestamp(&now)
			return cl.Client.Update(ctx, tur)
		}
		return fmt.Errorf("unexpected type of resource to delete: '%T'", obj)
	}
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
