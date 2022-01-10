package templateupdaterequest_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	tierutil "github.com/codeready-toolchain/host-operator/controllers/nstemplatetier/util"
	"github.com/codeready-toolchain/host-operator/controllers/templateupdaterequest"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	tiertest "github.com/codeready-toolchain/host-operator/test/nstemplatetier"
	spacetest "github.com/codeready-toolchain/host-operator/test/space"
	turtest "github.com/codeready-toolchain/host-operator/test/templateupdaterequest"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	murtest "github.com/codeready-toolchain/toolchain-common/pkg/test/masteruserrecord"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
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
)

func TestReconcile(t *testing.T) {

	// given
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	// a "basic" NSTemplateTier

	t.Run("controller should update the MasterUserRecord", func(t *testing.T) {

		t.Run("when there is a single target cluster to update", func(t *testing.T) {

			t.Run("with same namespaces", func(t *testing.T) {
				// given
				previousBasicTier := tiertest.BasicTier(t, tiertest.PreviousBasicTemplates)
				basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates, tiertest.WithCurrentUpdateInProgress())
				initObjs := []runtime.Object{basicTier}
				initObjs = append(initObjs, murtest.NewMasterUserRecord(t, "user-1",
					murtest.Account("cluster1", *previousBasicTier, murtest.SyncIndex("1"))))
				initObjs = append(initObjs, turtest.NewTemplateUpdateRequest("user-1", *basicTier))
				r, req, cl := prepareReconcile(t, initObjs...)
				// when
				res, err := r.Reconcile(context.TODO(), req)
				// then
				require.NoError(t, err)
				require.Equal(t, reconcile.Result{}, res) // no need to requeue, the MUR is watched
				// check that the MasterUserRecord was updated
				murtest.AssertThatMasterUserRecord(t, "user-1", cl).
					HasTier(*basicTier).
					AllUserAccountsHaveTier(*basicTier)
				// check that TemplateUpdateRequest is in "updating" condition
				turtest.AssertThatTemplateUpdateRequest(t, "user-1", cl).
					HasConditions(templateupdaterequest.ToBeUpdating()).
					HasSyncIndexes(map[string]string{
						"cluster1": "1",
					})
			})

			t.Run("with same namespaces after a failure", func(t *testing.T) {
				// given
				previousBasicTier := tiertest.BasicTier(t, tiertest.PreviousBasicTemplates)
				basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates, tiertest.WithCurrentUpdateInProgress())
				initObjs := []runtime.Object{basicTier}
				initObjs = append(initObjs, murtest.NewMasterUserRecord(t, "user-1",
					murtest.Account("cluster1", *previousBasicTier, murtest.SyncIndex("1"))))
				initObjs = append(initObjs, turtest.NewTemplateUpdateRequest("user-1", *basicTier,
					turtest.Condition(templateupdaterequest.ToFailure(fmt.Errorf("mock error"))), // an error occurred during the previous attempt
				))
				r, req, cl := prepareReconcile(t, initObjs...)
				// when
				res, err := r.Reconcile(context.TODO(), req)
				// then
				require.NoError(t, err)
				require.Equal(t, reconcile.Result{}, res) // no need to requeue, the MUR is watched
				// check that the MasterUserRecord was updated
				murtest.AssertThatMasterUserRecord(t, "user-1", cl).
					HasTier(*basicTier).
					AllUserAccountsHaveTier(*basicTier)
				// check that TemplateUpdateRequest is in "updating" condition
				turtest.AssertThatTemplateUpdateRequest(t, "user-1", cl).
					HasConditions(templateupdaterequest.ToBeUpdating()).
					HasSyncIndexes(map[string]string{
						"cluster1": "1",
					})
			})

			t.Run("with less namespaces", func(t *testing.T) {
				// given
				previousBasicTier := tiertest.BasicTier(t, tiertest.PreviousBasicTemplates)
				basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates,
					tiertest.WithCurrentUpdateInProgress(), tiertest.WithoutCodeNamespace())
				initObjs := []runtime.Object{basicTier}
				initObjs = append(initObjs, murtest.NewMasterUserRecord(t, "user-1",
					murtest.Account("cluster1", *previousBasicTier, murtest.SyncIndex("1"))))
				initObjs = append(initObjs, turtest.NewTemplateUpdateRequest("user-1", *basicTier))
				r, req, cl := prepareReconcile(t, initObjs...)
				// when
				res, err := r.Reconcile(context.TODO(), req)
				// then
				require.NoError(t, err)
				require.Equal(t, reconcile.Result{}, res) // no need to requeue, the MUR is watched
				// check that the MasterUserRecord was updated
				murtest.AssertThatMasterUserRecord(t, "user-1", cl).
					HasTier(*basicTier).
					AllUserAccountsHaveTier(*basicTier)
				// check that TemplateUpdateRequest is in "updating" condition
				turtest.AssertThatTemplateUpdateRequest(t, "user-1", cl).
					HasConditions(templateupdaterequest.ToBeUpdating()).
					HasSyncIndexes(map[string]string{
						"cluster1": "1",
					})
			})

			t.Run("when cluster resources do not exist in update then it should be removed", func(t *testing.T) {
				// given
				previousBasicTier := tiertest.BasicTier(t, tiertest.PreviousBasicTemplates)
				basicTier := tiertest.BasicTier(t, tiertest.PreviousBasicTemplates, tiertest.WithoutClusterResources())
				initObjs := []runtime.Object{basicTier}
				mur := murtest.NewMasterUserRecord(t, "user-1",
					murtest.Account("cluster1", *previousBasicTier, murtest.SyncIndex("1")))
				initObjs = append(initObjs, mur)
				initObjs = append(initObjs, turtest.NewTemplateUpdateRequest("user-1", *basicTier))
				r, req, cl := prepareReconcile(t, initObjs...)
				// when
				res, err := r.Reconcile(context.TODO(), req)
				// then
				require.NoError(t, err)
				require.Equal(t, reconcile.Result{}, res) // no need to requeue, the MUR is watched
				// check that the MasterUserRecord was updated
				murtest.AssertThatMasterUserRecord(t, "user-1", cl).
					HasTier(*basicTier).
					AllUserAccountsHaveTier(*basicTier)
				// check that TemplateUpdateRequest is in "updating" condition
				turtest.AssertThatTemplateUpdateRequest(t, "user-1", cl).
					HasConditions(templateupdaterequest.ToBeUpdating()).
					HasSyncIndexes(map[string]string{
						"cluster1": "1",
					})
			})

			t.Run("with undefined NSTemplateSet in UserAccount", func(t *testing.T) {
				// given
				previousBasicTier := tiertest.BasicTier(t, tiertest.PreviousBasicTemplates)
				basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates, tiertest.WithCurrentUpdateInProgress())
				mur := murtest.NewMasterUserRecord(t, "user-1", murtest.Account("cluster1", *previousBasicTier, murtest.SyncIndex("1")))
				// reset NSTemplateSet in MUR's UserAccount
				mur.Spec.UserAccounts[0].Spec.NSTemplateSet = nil
				initObjs := []runtime.Object{basicTier, mur, turtest.NewTemplateUpdateRequest("user-1", *basicTier)}
				r, req, cl := prepareReconcile(t, initObjs...)
				// when
				res, err := r.Reconcile(context.TODO(), req)
				// then
				require.NoError(t, err)
				require.Equal(t, reconcile.Result{}, res) // no need to requeue, the MUR is watched
				// check that the MasterUserRecord was not updated (ie, still no spec.UserAccounts[].NSTemplateSet )
				murtest.AssertThatMasterUserRecord(t, "user-1", cl).
					UserAccountHasNoTier("cluster-1")
				// check that TemplateUpdateRequest is in "updating" condition
				turtest.AssertThatTemplateUpdateRequest(t, "user-1", cl).
					HasConditions(templateupdaterequest.ToBeUpdating()).
					HasNoSyncIndexes() // no sync
			})
		})

		t.Run("when there are many target clusters to update", func(t *testing.T) {

			t.Run("with same namespaces", func(t *testing.T) {
				// given
				previousBasicTier := tiertest.BasicTier(t, tiertest.PreviousBasicTemplates)
				basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates, tiertest.WithCurrentUpdateInProgress())
				otherTier := tiertest.OtherTier()
				initObjs := []runtime.Object{basicTier}
				initObjs = append(initObjs, murtest.NewMasterUserRecord(t, "user-1",
					murtest.Account("cluster1", *previousBasicTier, murtest.SyncIndex("10")),
					murtest.AdditionalAccount("cluster2", *previousBasicTier, murtest.SyncIndex("20")),
					murtest.AdditionalAccount("cluster3", *otherTier, murtest.SyncIndex("100")))) // this account is not affected by the update
				initObjs = append(initObjs, turtest.NewTemplateUpdateRequest("user-1", *basicTier))
				r, req, cl := prepareReconcile(t, initObjs...)
				// when
				res, err := r.Reconcile(context.TODO(), req)
				// then
				require.NoError(t, err)
				require.Equal(t, reconcile.Result{}, res) // no need to requeue, the MUR is watched
				// check that the MasterUserRecord was updated
				murtest.AssertThatMasterUserRecord(t, "user-1", cl).
					UserAccountHasTier("cluster1", *basicTier).
					UserAccountHasTier("cluster2", *basicTier).
					UserAccountHasTier("cluster3", *otherTier)
				// check that TemplateUpdateRequest is in "updating" condition
				turtest.AssertThatTemplateUpdateRequest(t, "user-1", cl).
					HasConditions(templateupdaterequest.ToBeUpdating()).
					HasSyncIndexes(map[string]string{
						"cluster1": "10",
						"cluster2": "20",
					})
			})

		})

	})

	t.Run("controller should not delete the TemplateUpdateRequest", func(t *testing.T) {

		t.Run("when the MasterUserRecord is not up-to-date yet", func(t *testing.T) {
			// given
			previousBasicTier := tiertest.BasicTier(t, tiertest.PreviousBasicTemplates)
			basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates, tiertest.WithCurrentUpdateInProgress())
			otherTier := tiertest.OtherTier()
			initObjs := []runtime.Object{basicTier}
			initObjs = append(initObjs, turtest.NewTemplateUpdateRequest("user-1", *basicTier,
				turtest.Condition(templateupdaterequest.ToBeUpdating()),
				turtest.SyncIndexes{
					"cluster1": "10",
					"cluster2": "20",
				}))
			initObjs = append(initObjs, murtest.NewMasterUserRecord(t, "user-1",
				murtest.Account("cluster1", *previousBasicTier, murtest.SyncIndex("11")),           // here the sync index changed
				murtest.AdditionalAccount("cluster2", *previousBasicTier, murtest.SyncIndex("20")), // here the sync index did not change
				murtest.AdditionalAccount("cluster3", *otherTier, murtest.SyncIndex("100"))))       // this account is not affected by the update
			r, req, cl := prepareReconcile(t, initObjs...)
			// when
			res, err := r.Reconcile(context.TODO(), req)
			// then
			require.NoError(t, err)
			require.Equal(t, reconcile.Result{}, res) // no need to requeue, the MUR is watched
			// check that TemplateUpdateRequest is in "updating" condition
			turtest.AssertThatTemplateUpdateRequest(t, "user-1", cl).
				HasConditions(templateupdaterequest.ToBeUpdating()).
				HasSyncIndexes(map[string]string{
					"cluster1": "10",
					"cluster2": "20",
				})
		})

		t.Run("when the MasterUserRecord is up-to-date but not in 'ready' state yet", func(t *testing.T) {
			// given
			previousBasicTier := tiertest.BasicTier(t, tiertest.PreviousBasicTemplates)
			basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates, tiertest.WithCurrentUpdateInProgress())
			otherTier := tiertest.OtherTier()
			initObjs := []runtime.Object{basicTier}
			initObjs = append(initObjs, turtest.NewTemplateUpdateRequest("user-1", *basicTier,
				turtest.Condition(templateupdaterequest.ToBeUpdating()),
				turtest.SyncIndexes(map[string]string{
					"cluster1": "10",
					"cluster2": "20",
				})))
			initObjs = append(initObjs, murtest.NewMasterUserRecord(t, "user-1",
				murtest.Account("cluster1", *previousBasicTier, murtest.SyncIndex("11")),           // here the sync index changed
				murtest.AdditionalAccount("cluster2", *previousBasicTier, murtest.SyncIndex("21")), // here the sync index changed too
				murtest.AdditionalAccount("cluster3", *otherTier, murtest.SyncIndex("100"))))       // this account is not affected by the update
			r, req, cl := prepareReconcile(t, initObjs...)
			// when
			res, err := r.Reconcile(context.TODO(), req)
			// then
			require.NoError(t, err)
			require.Equal(t, reconcile.Result{}, res) // no need to requeue, the MUR is watched
			// check that TemplateUpdateRequest is in "updating" condition
			turtest.AssertThatTemplateUpdateRequest(t, "user-1", cl).
				HasConditions(templateupdaterequest.ToBeUpdating()).
				HasSyncIndexes(map[string]string{
					"cluster1": "10",
					"cluster2": "20",
				})
		})
	})

	t.Run("controller should mark TemplateUpdateRequest as complete", func(t *testing.T) {

		t.Run("when the MasterUserRecord is up-to-date and in 'ready' state", func(t *testing.T) {
			// given
			previousBasicTier := tiertest.BasicTier(t, tiertest.PreviousBasicTemplates)
			basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates, tiertest.WithCurrentUpdateInProgress())
			otherTier := tiertest.OtherTier()
			initObjs := []runtime.Object{basicTier}
			initObjs = append(initObjs, turtest.NewTemplateUpdateRequest("user-1", *basicTier,
				turtest.Condition(templateupdaterequest.ToBeUpdating()),
				turtest.SyncIndexes(map[string]string{
					"cluster1": "10",
					"cluster2": "20",
				})))
			initObjs = append(initObjs, murtest.NewMasterUserRecord(t, "user-1",
				murtest.Account("cluster1", *previousBasicTier, murtest.SyncIndex("11")),           // here the sync index changed
				murtest.AdditionalAccount("cluster2", *previousBasicTier, murtest.SyncIndex("21")), // here the sync index changed too
				murtest.AdditionalAccount("cluster3", *otherTier, murtest.SyncIndex("100")),        // account with another tier
				murtest.StatusCondition(toolchainv1alpha1.Condition{ // master user record is "ready"
					Type:   toolchainv1alpha1.ConditionReady,
					Status: corev1.ConditionTrue,
					Reason: toolchainv1alpha1.MasterUserRecordProvisionedReason,
				}),
			)) // this account is not affected by the update
			r, req, cl := prepareReconcile(t, initObjs...)
			// when
			res, err := r.Reconcile(context.TODO(), req)
			// then
			require.NoError(t, err)
			require.Equal(t, reconcile.Result{}, res) // NSTemplateTier controller will reconcile when the TemplateUpdateRequest is updated
			// check that TemplateUpdateRequest is in "complete" condition
			turtest.AssertThatTemplateUpdateRequest(t, "user-1", cl).
				HasConditions(toolchainv1alpha1.Condition{
					Type:   toolchainv1alpha1.TemplateUpdateRequestComplete,
					Status: corev1.ConditionTrue,
					Reason: toolchainv1alpha1.TemplateUpdateRequestUpdatedReason,
				})
		})

		t.Run("when the MasterUserRecord is up-to-date and in 'ready' state after a previous failure", func(t *testing.T) {
			// given
			previousBasicTier := tiertest.BasicTier(t, tiertest.PreviousBasicTemplates)
			basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates, tiertest.WithCurrentUpdateInProgress())
			otherTier := tiertest.OtherTier()
			initObjs := []runtime.Object{basicTier}
			initObjs = append(initObjs, turtest.NewTemplateUpdateRequest("user-1", *basicTier,
				turtest.Condition(templateupdaterequest.ToFailure(fmt.Errorf("mock error"))), // an error occurred during the previous attempt
				turtest.Condition(templateupdaterequest.ToBeUpdating()),
				turtest.SyncIndexes(map[string]string{
					"cluster1": "10",
					"cluster2": "20",
				})))
			initObjs = append(initObjs, murtest.NewMasterUserRecord(t, "user-1",
				murtest.Account("cluster1", *previousBasicTier, murtest.SyncIndex("11")),           // here the sync index changed
				murtest.AdditionalAccount("cluster2", *previousBasicTier, murtest.SyncIndex("21")), // here the sync index changed too
				murtest.AdditionalAccount("cluster3", *otherTier, murtest.SyncIndex("100")),        // account with another tier
				murtest.StatusCondition(toolchainv1alpha1.Condition{ // master user record is "ready"
					Type:   toolchainv1alpha1.ConditionReady,
					Status: corev1.ConditionTrue,
					Reason: toolchainv1alpha1.MasterUserRecordProvisionedReason,
				}),
			)) // this account is not affected by the update
			r, req, cl := prepareReconcile(t, initObjs...)
			// when
			res, err := r.Reconcile(context.TODO(), req)
			// then
			require.NoError(t, err)
			require.Equal(t, reconcile.Result{}, res) // NSTemplateTier controller will reconcile when the TemplateUpdateRequest is updated
			// check that TemplateUpdateRequest is in "complete" condition
			turtest.AssertThatTemplateUpdateRequest(t, "user-1", cl).
				HasConditions(
					// previous failure is not retained in the `status.conditions`
					toolchainv1alpha1.Condition{
						Type:   toolchainv1alpha1.TemplateUpdateRequestComplete,
						Status: corev1.ConditionTrue,
						Reason: toolchainv1alpha1.TemplateUpdateRequestUpdatedReason,
					})
		})

	})

	t.Run("controller should mark TemplateUpdateRequest as failed", func(t *testing.T) {

		t.Run("when the MasterUserRecord was deleted", func(t *testing.T) {
			// given
			basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates, tiertest.WithCurrentUpdateInProgress())
			initObjs := []runtime.Object{basicTier}
			initObjs = append(initObjs, turtest.NewTemplateUpdateRequest("user-1", *basicTier))
			r, req, cl := prepareReconcile(t, initObjs...) // there is no associated MasterUserRecord
			// when
			res, err := r.Reconcile(context.TODO(), req)
			// then
			require.NoError(t, err)
			require.Equal(t, reconcile.Result{}, res)
			// check that TemplateUpdateRequest is in "failed" condition
			turtest.AssertThatTemplateUpdateRequest(t, "user-1", cl).
				HasConditions(toolchainv1alpha1.Condition{
					Type:    toolchainv1alpha1.TemplateUpdateRequestComplete,
					Status:  corev1.ConditionFalse,
					Reason:  toolchainv1alpha1.TemplateUpdateRequestUnableToUpdateReason,
					Message: `masteruserrecords.toolchain.dev.openshift.com "user-1" not found`,
				})
		})

		t.Run("when the MasterUserRecord could not be updated", func(t *testing.T) {

			t.Run("and requeue for another attempt", func(t *testing.T) {
				// given
				previousBasicTier := tiertest.BasicTier(t, tiertest.PreviousBasicTemplates)
				basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates, tiertest.WithCurrentUpdateInProgress())
				initObjs := []runtime.Object{basicTier}
				initObjs = append(initObjs, murtest.NewMasterUserRecord(t, "user-1", murtest.Account("cluster1", *previousBasicTier, murtest.SyncIndex("1"))))
				initObjs = append(initObjs, turtest.NewTemplateUpdateRequest("user-1", *basicTier))
				r, req, cl := prepareReconcile(t, initObjs...)
				cl.MockUpdate = func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
					if _, ok := obj.(*toolchainv1alpha1.MasterUserRecord); ok {
						return fmt.Errorf("mock error")
					}
					return cl.Client.Update(ctx, obj, opts...)
				}
				// when
				res, err := r.Reconcile(context.TODO(), req)
				// then
				require.Error(t, err)
				require.Equal(t, reconcile.Result{Requeue: true, RequeueAfter: 5 * time.Second}, res)
				// check that TemplateUpdateRequest is in "failed" condition
				turtest.AssertThatTemplateUpdateRequest(t, "user-1", cl).
					HasConditions(toolchainv1alpha1.Condition{
						Type:    toolchainv1alpha1.TemplateUpdateRequestComplete,
						Status:  corev1.ConditionFalse,
						Reason:  toolchainv1alpha1.TemplateUpdateRequestUnableToUpdateReason,
						Message: "unable to update the MasterUserRecord associated with the TemplateUpdateRequest: mock error",
					})
			})

			t.Run("and give up", func(t *testing.T) {
				// given
				basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates, tiertest.WithCurrentUpdateInProgress())
				initObjs := []runtime.Object{basicTier}
				initObjs = append(initObjs, murtest.NewMasterUserRecord(t, "user-1"))
				initObjs = append(initObjs, turtest.NewTemplateUpdateRequest("user-1", *basicTier,
					turtest.Condition{ // include a condition for a previous failed attempt to update the MasterUserRecord
						Type:    toolchainv1alpha1.TemplateUpdateRequestComplete,
						Status:  corev1.ConditionFalse,
						Reason:  toolchainv1alpha1.TemplateUpdateRequestUnableToUpdateReason,
						Message: `mock error`,
					}))
				r, req, cl := prepareReconcile(t, initObjs...) // there is no associated MasterUserRecord
				cl.MockUpdate = func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
					if _, ok := obj.(*toolchainv1alpha1.MasterUserRecord); ok {
						return fmt.Errorf("mock error 2")
					}
					return cl.Client.Update(ctx, obj, opts...)
				}
				// when
				res, err := r.Reconcile(context.TODO(), req)
				// then
				require.NoError(t, err)
				require.Equal(t, reconcile.Result{}, res)
				// check that TemplateUpdateRequest is in "failed" condition
				turtest.AssertThatTemplateUpdateRequest(t, "user-1", cl).
					// expect 2 occurrences of the "failure" in the status.conditions
					HasSameConditions(toolchainv1alpha1.Condition{
						Type:    toolchainv1alpha1.TemplateUpdateRequestComplete,
						Status:  corev1.ConditionFalse,
						Reason:  toolchainv1alpha1.TemplateUpdateRequestUnableToUpdateReason,
						Message: `mock error`,
					}, toolchainv1alpha1.Condition{
						Type:    toolchainv1alpha1.TemplateUpdateRequestComplete,
						Status:  corev1.ConditionFalse,
						Reason:  toolchainv1alpha1.TemplateUpdateRequestUnableToUpdateReason,
						Message: `unable to update the MasterUserRecord associated with the TemplateUpdateRequest: mock error 2`,
					})
			})

		})

	})

	t.Run("failures", func(t *testing.T) {

		t.Run("unable to get TemplateUpdateRequest", func(t *testing.T) {

			t.Run("tier not found", func(t *testing.T) {
				// given
				basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates, tiertest.WithCurrentUpdateInProgress())
				r, req, cl := prepareReconcile(t, basicTier, turtest.NewTemplateUpdateRequest("user-1", *basicTier)) // there is no associated MasterUserRecord
				cl.MockGet = func(ctx context.Context, key types.NamespacedName, obj client.Object) error {
					if _, ok := obj.(*toolchainv1alpha1.TemplateUpdateRequest); ok {
						return errors.NewNotFound(schema.GroupResource{}, key.Name)
					}
					return cl.Client.Get(ctx, key, obj)
				}
				// when
				res, err := r.Reconcile(context.TODO(), req)
				// then
				require.NoError(t, err)                  // no error: TemplateUpdateRequest was probably deleted
				assert.Equal(t, reconcile.Result{}, res) // no explicit requeue since there is no TemplateUpdateRequest anyways
			})

			t.Run("other error", func(t *testing.T) {
				// given
				basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates, tiertest.WithCurrentUpdateInProgress())
				r, req, cl := prepareReconcile(t, basicTier, turtest.NewTemplateUpdateRequest("user-1", *basicTier)) // there is no associated MasterUserRecord
				cl.MockGet = func(ctx context.Context, key types.NamespacedName, obj client.Object) error {
					if _, ok := obj.(*toolchainv1alpha1.TemplateUpdateRequest); ok {
						return fmt.Errorf("mock error")
					}
					return cl.Client.Get(ctx, key, obj)
				}
				// when
				res, err := r.Reconcile(context.TODO(), req)
				// then
				require.Error(t, err)
				assert.EqualError(t, err, "unable to get the current TemplateUpdateRequest: mock error")
				assert.Equal(t, reconcile.Result{}, res) // no explicit requeue
			})
		})

		t.Run("unable to get associated MasterUserRecord", func(t *testing.T) {

			t.Run("tier not found", func(t *testing.T) {
				// given
				basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates, tiertest.WithCurrentUpdateInProgress())
				r, req, cl := prepareReconcile(t, basicTier, turtest.NewTemplateUpdateRequest("user-1", *basicTier)) // there is no associated MasterUserRecord
				cl.MockGet = func(ctx context.Context, key types.NamespacedName, obj client.Object) error {
					if _, ok := obj.(*toolchainv1alpha1.MasterUserRecord); ok {
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
				basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates, tiertest.WithCurrentUpdateInProgress())
				r, req, cl := prepareReconcile(t, basicTier, turtest.NewTemplateUpdateRequest("user-1", *basicTier)) // there is no associated MasterUserRecord
				cl.MockGet = func(ctx context.Context, key types.NamespacedName, obj client.Object) error {
					if _, ok := obj.(*toolchainv1alpha1.MasterUserRecord); ok {
						return fmt.Errorf("mock error")
					}
					return cl.Client.Get(ctx, key, obj)
				}
				// when
				res, err := r.Reconcile(context.TODO(), req)
				// then
				require.Error(t, err)
				assert.EqualError(t, err, "unable to get the MasterUserRecord associated with the TemplateUpdateRequest: mock error")
				assert.Equal(t, reconcile.Result{}, res) // no explicit requeue
			})
		})

		t.Run("unable to update the TemplateUpdateRequest status", func(t *testing.T) {
			// given
			previousBasicTier := tiertest.BasicTier(t, tiertest.PreviousBasicTemplates)
			basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates, tiertest.WithCurrentUpdateInProgress())
			mur := murtest.NewMasterUserRecord(t, "user-1", murtest.Account("cluster1", *previousBasicTier, murtest.SyncIndex("1")))
			r, req, cl := prepareReconcile(t, basicTier, turtest.NewTemplateUpdateRequest("user-1", *basicTier), mur)
			cl.MockStatusUpdate = func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
				if _, ok := obj.(*toolchainv1alpha1.TemplateUpdateRequest); ok {
					return fmt.Errorf("mock error")
				}
				return cl.Client.Status().Update(ctx, obj, opts...)
			}
			// when
			res, err := r.Reconcile(context.TODO(), req)
			// then
			require.Error(t, err)
			assert.EqualError(t, err, "unable to update the TemplateUpdateRequest status: mock error")
			assert.Equal(t, reconcile.Result{}, res) // no explicit requeue
		})

		t.Run("unable to update the MasterUserRecord", func(t *testing.T) {
			// given
			previousBasicTier := tiertest.BasicTier(t, tiertest.PreviousBasicTemplates)
			basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates, tiertest.WithCurrentUpdateInProgress())
			mur := murtest.NewMasterUserRecord(t, "user-1", murtest.Account("cluster1", *previousBasicTier, murtest.SyncIndex("1")))
			r, req, cl := prepareReconcile(t, basicTier, turtest.NewTemplateUpdateRequest("user-1", *basicTier), mur)
			cl.MockUpdate = func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
				if _, ok := obj.(*toolchainv1alpha1.MasterUserRecord); ok {
					return fmt.Errorf("mock error")
				}
				return cl.Client.Update(ctx, obj, opts...)
			}

			// when (first attempt)
			res, err := r.Reconcile(context.TODO(), req)
			// then
			require.Error(t, err) // expect an error and an explicit requeue with a delay
			assert.EqualError(t, err, "unable to update the MasterUserRecord associated with the TemplateUpdateRequest: mock error")
			assert.Equal(t, reconcile.Result{Requeue: true, RequeueAfter: 5 * time.Second}, res) // explicit requeue

			// when (second attempt)
			res, err = r.Reconcile(context.TODO(), req)
			// then
			require.NoError(t, err)                  // this time, don't expect an error (but error was logged )
			assert.Equal(t, reconcile.Result{}, res) // no requeue
		})

	})

	t.Run("spaces", func(t *testing.T) {
		t.Run("when space tier hash label still the same", func(t *testing.T) {
			// given
			previousBasicTier := tiertest.BasicTier(t, tiertest.PreviousBasicTemplates)
			basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates, tiertest.WithCurrentUpdateInProgress())
			space := spacetest.NewSpace("user-1", spacetest.WithTierNameAndHashLabelFor(previousBasicTier)) // space's hash matches TUR hash
			previousHash, err := tierutil.ComputeHashForNSTemplateTier(previousBasicTier)
			require.NoError(t, err)
			r, req, cl := prepareReconcile(t, basicTier, space, turtest.NewTemplateUpdateRequest("user-1", *basicTier, turtest.CurrentTierHash(previousHash)))
			// when
			res, err := r.Reconcile(context.TODO(), req)
			// then
			require.NoError(t, err)
			require.Equal(t, reconcile.Result{}, res) // no need to requeue, the Space is watched
			turtest.AssertThatTemplateUpdateRequest(t, "user-1", cl).
				HasConditions(toolchainv1alpha1.Condition{
					Type:   toolchainv1alpha1.TemplateUpdateRequestComplete,
					Status: corev1.ConditionFalse,
					Reason: toolchainv1alpha1.TemplateUpdateRequestUpdatingReason,
				})
		})

		t.Run("when space tier hash label has changed", func(t *testing.T) {
			t.Run("space condition not ready yet", func(t *testing.T) {
				// given
				previousBasicTier := tiertest.BasicTier(t, tiertest.PreviousBasicTemplates)
				basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates, tiertest.WithCurrentUpdateInProgress())
				space := spacetest.NewSpace("user-1", spacetest.WithTierNameAndHashLabelFor(basicTier), spacetest.WithCondition(
					toolchainv1alpha1.Condition{
						Type:   toolchainv1alpha1.TemplateUpdateRequestComplete,
						Status: corev1.ConditionFalse,
						Reason: toolchainv1alpha1.TemplateUpdateRequestUpdatingReason,
					},
				)) // space's hash updated to new tier
				previousHash, err := tierutil.ComputeHashForNSTemplateTier(previousBasicTier)
				require.NoError(t, err)
				r, req, cl := prepareReconcile(t, basicTier, space, turtest.NewTemplateUpdateRequest("user-1", *basicTier, turtest.CurrentTierHash(previousHash)))
				// when
				res, err := r.Reconcile(context.TODO(), req)
				// then
				require.NoError(t, err)
				require.Equal(t, reconcile.Result{}, res) // no need to requeue, the Space is watched
				turtest.AssertThatTemplateUpdateRequest(t, "user-1", cl).
					HasConditions(toolchainv1alpha1.Condition{
						Type:   toolchainv1alpha1.TemplateUpdateRequestComplete,
						Status: corev1.ConditionFalse,
						Reason: toolchainv1alpha1.TemplateUpdateRequestUpdatingReason,
					})
			})
			t.Run("space condition is now ready", func(t *testing.T) {
				// given
				previousBasicTier := tiertest.BasicTier(t, tiertest.PreviousBasicTemplates)
				basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates, tiertest.WithCurrentUpdateInProgress())
				space := spacetest.NewSpace(
					"user-1",
					spacetest.WithTierNameAndHashLabelFor(basicTier), // space's hash updated to new tier and condition is Ready
					spacetest.WithCondition(toolchainv1alpha1.Condition{ // space is ready
						Type:   toolchainv1alpha1.ConditionReady,
						Status: corev1.ConditionTrue,
						Reason: toolchainv1alpha1.SpaceProvisionedReason,
					}),
				)
				previousHash, err := tierutil.ComputeHashForNSTemplateTier(previousBasicTier)
				require.NoError(t, err)
				r, req, cl := prepareReconcile(t, basicTier, space, turtest.NewTemplateUpdateRequest("user-1", *basicTier, turtest.CurrentTierHash(previousHash)))
				// when
				res, err := r.Reconcile(context.TODO(), req)
				// then
				require.NoError(t, err)
				require.Equal(t, reconcile.Result{}, res) // no need to requeue, the Space is watched
				turtest.AssertThatTemplateUpdateRequest(t, "user-1", cl).
					HasConditions(toolchainv1alpha1.Condition{
						Type:   toolchainv1alpha1.TemplateUpdateRequestComplete,
						Status: corev1.ConditionTrue,
						Reason: toolchainv1alpha1.TemplateUpdateRequestUpdatedReason,
					})
			})
		})

		t.Run("errors", func(t *testing.T) {

			t.Run("space not found", func(t *testing.T) {
				// given
				previousBasicTier := tiertest.BasicTier(t, tiertest.PreviousBasicTemplates)
				basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates, tiertest.WithCurrentUpdateInProgress())
				previousHash, err := tierutil.ComputeHashForNSTemplateTier(previousBasicTier)
				require.NoError(t, err)
				r, req, cl := prepareReconcile(t, basicTier, turtest.NewTemplateUpdateRequest("user-1", *basicTier, turtest.CurrentTierHash(previousHash)))
				// when
				res, err := r.Reconcile(context.TODO(), req)
				// then
				require.NoError(t, err)
				require.Equal(t, reconcile.Result{}, res) // no need to requeue, the Space is watched
				// check that TemplateUpdateRequest is in "failed" condition
				turtest.AssertThatTemplateUpdateRequest(t, "user-1", cl).
					HasConditions(toolchainv1alpha1.Condition{
						Type:    toolchainv1alpha1.TemplateUpdateRequestComplete,
						Status:  corev1.ConditionFalse,
						Reason:  toolchainv1alpha1.TemplateUpdateRequestUnableToUpdateReason,
						Message: `spaces.toolchain.dev.openshift.com "user-1" not found`,
					})
			})

			t.Run("get space error", func(t *testing.T) {
				// given
				previousBasicTier := tiertest.BasicTier(t, tiertest.PreviousBasicTemplates)
				basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates, tiertest.WithCurrentUpdateInProgress())
				previousHash, err := tierutil.ComputeHashForNSTemplateTier(previousBasicTier)
				require.NoError(t, err)
				r, req, cl := prepareReconcile(t, basicTier, turtest.NewTemplateUpdateRequest("user-1", *basicTier, turtest.CurrentTierHash(previousHash)))
				cl.MockGet = func(ctx context.Context, key types.NamespacedName, obj client.Object) error {
					if _, ok := obj.(*toolchainv1alpha1.Space); ok {
						return fmt.Errorf("mock error")
					}
					return cl.Client.Get(ctx, key, obj)
				}
				// when
				res, err := r.Reconcile(context.TODO(), req)
				// then
				require.Error(t, err)
				assert.EqualError(t, err, "unable to get the Space associated with the TemplateUpdateRequest: mock error")
				assert.Equal(t, reconcile.Result{}, res) // no need to requeue, the Space is watched
			})

			t.Run("error updating status", func(t *testing.T) {
				// given
				previousBasicTier := tiertest.BasicTier(t, tiertest.PreviousBasicTemplates)
				basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates, tiertest.WithCurrentUpdateInProgress())
				space := spacetest.NewSpace("user-1", spacetest.WithTierNameAndHashLabelFor(previousBasicTier)) // space's hash matches TUR hash
				previousHash, err := tierutil.ComputeHashForNSTemplateTier(previousBasicTier)
				require.NoError(t, err)
				r, req, cl := prepareReconcile(t, basicTier, space, turtest.NewTemplateUpdateRequest("user-1", *basicTier, turtest.CurrentTierHash(previousHash)))
				cl.MockStatusUpdate = func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
					if _, ok := obj.(*toolchainv1alpha1.TemplateUpdateRequest); ok {
						return fmt.Errorf("mock error")
					}
					return cl.Client.Status().Update(ctx, obj, opts...)
				}
				// when
				res, err := r.Reconcile(context.TODO(), req)
				// then
				require.Error(t, err)
				assert.EqualError(t, err, "unable to update the TemplateUpdateRequest status: mock error")
				assert.Equal(t, reconcile.Result{}, res) // no need to requeue, the Space is watched
			})
		})
	})
}

func prepareReconcile(t *testing.T, initObjs ...runtime.Object) (reconcile.Reconciler, reconcile.Request, *test.FakeClient) {
	os.Setenv("WATCH_NAMESPACE", test.HostOperatorNs)
	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)
	cl := test.NewFakeClient(t, initObjs...)
	r := &templateupdaterequest.Reconciler{
		Client: cl,
		Scheme: s,
	}
	return r, reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "user-1",
			Namespace: operatorNamespace,
		},
	}, cl
}
