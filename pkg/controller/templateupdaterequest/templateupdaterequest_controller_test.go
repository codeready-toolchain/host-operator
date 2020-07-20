package templateupdaterequest_test

import (
	"context"
	"fmt"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	"github.com/codeready-toolchain/host-operator/pkg/controller/templateupdaterequest"
	turtest "github.com/codeready-toolchain/host-operator/test/templateupdaterequest"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	murtest "github.com/codeready-toolchain/toolchain-common/pkg/test/masteruserrecord"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
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
				TemplateRef: "basic-clusterresources-123456old",
			},
		},
	}

	newNSTemplateTier := toolchainv1alpha1.NSTemplateTier{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: operatorNamespace,
			Name:      "basic", // must be the same name as `oldNSTemplateTier`
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

	newNSTemplateTier2 := toolchainv1alpha1.NSTemplateTier{ // same as newNSTemplateTier, but without "code" namespace
		ObjectMeta: metav1.ObjectMeta{
			Namespace: operatorNamespace,
			Name:      "basic", // must be the same name as `oldNSTemplateTier`
		},
		Spec: toolchainv1alpha1.NSTemplateTierSpec{
			Namespaces: []toolchainv1alpha1.NSTemplateTierNamespace{
				// no "code-" namespace
				{
					TemplateRef: "basic2-dev-123456new",
				},
				{
					TemplateRef: "basic2-stage-123456new",
				},
			},
			ClusterResources: &toolchainv1alpha1.NSTemplateTierClusterResources{
				TemplateRef: "basic2-clusterresources-123456new",
			},
		},
	}

	newNSTemplateTier3 := toolchainv1alpha1.NSTemplateTier{ // same as newNSTemplateTier, but without ClusterResources
		ObjectMeta: metav1.ObjectMeta{
			Namespace: operatorNamespace,
			Name:      "basic", // must be the same name as `oldNSTemplateTier`
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

	otherNSTemplateTier := toolchainv1alpha1.NSTemplateTier{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: operatorNamespace,
			Name:      "other",
		},
		Spec: toolchainv1alpha1.NSTemplateTierSpec{
			Namespaces: []toolchainv1alpha1.NSTemplateTierNamespace{
				{
					TemplateRef: "other-code-123456new",
				},
				{
					TemplateRef: "other-dev-123456new",
				},
				{
					TemplateRef: "other-stage-123456new",
				},
			},
			ClusterResources: &toolchainv1alpha1.NSTemplateTierClusterResources{
				TemplateRef: "other-clusterresources-123456new",
			},
		},
	}

	t.Run("controller should update the MasterUserRecord", func(t *testing.T) {

		t.Run("when there is a single target cluster to update", func(t *testing.T) {

			t.Run("with same namespaces", func(t *testing.T) {
				// given
				initObjs := []runtime.Object{&newNSTemplateTier}
				initObjs = append(initObjs, murtest.NewMasterUserRecord(t, "user-1",
					murtest.Account("cluster1", oldNSTemplateTier, murtest.SyncIndex("1"))))
				initObjs = append(initObjs, turtest.NewTemplateUpdateRequest("user-1", newNSTemplateTier))
				r, req, cl := prepareReconcile(t, "user-1", initObjs...)
				// when
				res, err := r.Reconcile(req)
				// then
				require.NoError(t, err)
				require.Equal(t, reconcile.Result{}, res) // no need to requeue, the MUR is watched
				// check that the MasterUserRecord was updated
				murtest.AssertThatMasterUserRecord(t, "user-1", cl).
					AllUserAccountsHaveTier(newNSTemplateTier)
				// check that TemplateUpdateRequest is in "updating" condition
				turtest.AssertThatTemplateUpdateRequest(t, "user-1", cl).
					HasConditions(templateupdaterequest.ToBeUpdating()).
					HasSyncIndexes(map[string]string{
						"cluster1": "1",
					})
			})

			t.Run("with less namespaces", func(t *testing.T) {
				// given
				initObjs := []runtime.Object{&newNSTemplateTier2}
				initObjs = append(initObjs, murtest.NewMasterUserRecord(t, "user-1",
					murtest.Account("cluster1", oldNSTemplateTier, murtest.SyncIndex("1"))))
				initObjs = append(initObjs, turtest.NewTemplateUpdateRequest("user-1", newNSTemplateTier2))
				r, req, cl := prepareReconcile(t, "user-1", initObjs...)
				// when
				res, err := r.Reconcile(req)
				// then
				require.NoError(t, err)
				require.Equal(t, reconcile.Result{}, res) // no need to requeue, the MUR is watched
				// check that the MasterUserRecord was updated
				murtest.AssertThatMasterUserRecord(t, "user-1", cl).
					AllUserAccountsHaveTier(newNSTemplateTier2)
				// check that TemplateUpdateRequest is in "updating" condition
				turtest.AssertThatTemplateUpdateRequest(t, "user-1", cl).
					HasConditions(templateupdaterequest.ToBeUpdating()).
					HasSyncIndexes(map[string]string{
						"cluster1": "1",
					})
			})

			t.Run("with custom template on namespace", func(t *testing.T) {
				// given
				initObjs := []runtime.Object{&newNSTemplateTier}
				mur := murtest.NewMasterUserRecord(t, "user-1",
					murtest.Account("cluster1", oldNSTemplateTier, murtest.SyncIndex("1"),
						murtest.CustomNamespaceTemplate("basic-code-123456old", "custom"), // a custom template is defined for the 1st namespace
					))
				initObjs = append(initObjs, mur)
				initObjs = append(initObjs, turtest.NewTemplateUpdateRequest("user-1", newNSTemplateTier))
				r, req, cl := prepareReconcile(t, "user-1", initObjs...)
				// when
				res, err := r.Reconcile(req)
				// then
				require.NoError(t, err)
				require.Equal(t, reconcile.Result{}, res) // no need to requeue, the MUR is watched
				// check that the MasterUserRecord was updated
				murtest.AssertThatMasterUserRecord(t, "user-1", cl).
					AllUserAccountsHaveTier(newNSTemplateTier).
					// check that the custom template for the given cluster/namespace was not lost
					HasCustomNamespaceTemplate("cluster1", "basic-code-123456new", "custom")

				// check that TemplateUpdateRequest is in "updating" condition
				turtest.AssertThatTemplateUpdateRequest(t, "user-1", cl).
					HasConditions(templateupdaterequest.ToBeUpdating()).
					HasSyncIndexes(map[string]string{
						"cluster1": "1",
					})
			})

			t.Run("with custom template when cluster resources exist in update", func(t *testing.T) {
				// given
				initObjs := []runtime.Object{&newNSTemplateTier}
				mur := murtest.NewMasterUserRecord(t, "user-1",
					murtest.Account("cluster1", oldNSTemplateTier, murtest.SyncIndex("1"),
						murtest.CustomClusterResourcesTemplate("custom"), // a custom template is defined for the 1st namespace
					))
				initObjs = append(initObjs, mur)
				initObjs = append(initObjs, turtest.NewTemplateUpdateRequest("user-1", newNSTemplateTier))
				r, req, cl := prepareReconcile(t, "user-1", initObjs...)
				// when
				res, err := r.Reconcile(req)
				// then
				require.NoError(t, err)
				require.Equal(t, reconcile.Result{}, res) // no need to requeue, the MUR is watched
				// check that the MasterUserRecord was updated
				murtest.AssertThatMasterUserRecord(t, "user-1", cl).
					AllUserAccountsHaveTier(newNSTemplateTier).
					// check that the custom template for the given cluster/namespace was not lost
					HasCustomClusterResourcesTemplate("cluster1", "custom")

				// check that TemplateUpdateRequest is in "updating" condition
				turtest.AssertThatTemplateUpdateRequest(t, "user-1", cl).
					HasConditions(templateupdaterequest.ToBeUpdating()).
					HasSyncIndexes(map[string]string{
						"cluster1": "1",
					})
			})

			t.Run("with custom template when cluster resources do not exist in update", func(t *testing.T) {
				// given
				initObjs := []runtime.Object{&newNSTemplateTier3}
				mur := murtest.NewMasterUserRecord(t, "user-1",
					murtest.Account("cluster1", oldNSTemplateTier, murtest.SyncIndex("1"),
						murtest.CustomClusterResourcesTemplate("custom"), // a custom template is defined for the 1st namespace
					))
				initObjs = append(initObjs, mur)
				initObjs = append(initObjs, turtest.NewTemplateUpdateRequest("user-1", newNSTemplateTier3))
				r, req, cl := prepareReconcile(t, "user-1", initObjs...)
				// when
				res, err := r.Reconcile(req)
				// then
				require.NoError(t, err)
				require.Equal(t, reconcile.Result{}, res) // no need to requeue, the MUR is watched
				// check that the MasterUserRecord was updated
				murtest.AssertThatMasterUserRecord(t, "user-1", cl).
					AllUserAccountsHaveTier(newNSTemplateTier3).
					// check that the custom template for the given cluster/namespace was not lost
					HasCustomClusterResourcesTemplate("cluster1", "custom")

				// check that TemplateUpdateRequest is in "updating" condition
				turtest.AssertThatTemplateUpdateRequest(t, "user-1", cl).
					HasConditions(templateupdaterequest.ToBeUpdating()).
					HasSyncIndexes(map[string]string{
						"cluster1": "1",
					})
			})

			t.Run("when cluster resources do not exist in update then it should be removed", func(t *testing.T) {
				// given
				initObjs := []runtime.Object{&newNSTemplateTier3}
				mur := murtest.NewMasterUserRecord(t, "user-1",
					murtest.Account("cluster1", oldNSTemplateTier, murtest.SyncIndex("1")))
				initObjs = append(initObjs, mur)
				initObjs = append(initObjs, turtest.NewTemplateUpdateRequest("user-1", newNSTemplateTier3))
				r, req, cl := prepareReconcile(t, "user-1", initObjs...)
				// when
				res, err := r.Reconcile(req)
				// then
				require.NoError(t, err)
				require.Equal(t, reconcile.Result{}, res) // no need to requeue, the MUR is watched
				// check that the MasterUserRecord was updated
				murtest.AssertThatMasterUserRecord(t, "user-1", cl).
					AllUserAccountsHaveTier(newNSTemplateTier3)

				// check that TemplateUpdateRequest is in "updating" condition
				turtest.AssertThatTemplateUpdateRequest(t, "user-1", cl).
					HasConditions(templateupdaterequest.ToBeUpdating()).
					HasSyncIndexes(map[string]string{
						"cluster1": "1",
					})
			})
		})

		t.Run("when there are many target clusters to update", func(t *testing.T) {

			t.Run("with same namespaces", func(t *testing.T) {
				// given
				initObjs := []runtime.Object{&newNSTemplateTier}
				initObjs = append(initObjs, murtest.NewMasterUserRecord(t, "user-1",
					murtest.Account("cluster1", oldNSTemplateTier, murtest.SyncIndex("10")),
					murtest.AdditionalAccount("cluster2", oldNSTemplateTier, murtest.SyncIndex("20")),
					murtest.AdditionalAccount("cluster3", otherNSTemplateTier, murtest.SyncIndex("100")))) // this account is not affected by the update
				initObjs = append(initObjs, turtest.NewTemplateUpdateRequest("user-1", newNSTemplateTier))
				r, req, cl := prepareReconcile(t, "user-1", initObjs...)
				// when
				res, err := r.Reconcile(req)
				// then
				require.NoError(t, err)
				require.Equal(t, reconcile.Result{}, res) // no need to requeue, the MUR is watched
				// check that the MasterUserRecord was updated
				murtest.AssertThatMasterUserRecord(t, "user-1", cl).
					UserAccountHasTier("cluster1", newNSTemplateTier).
					UserAccountHasTier("cluster2", newNSTemplateTier).
					UserAccountHasTier("cluster3", otherNSTemplateTier)
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
			initObjs := []runtime.Object{&newNSTemplateTier}
			initObjs = append(initObjs, turtest.NewTemplateUpdateRequest("user-1", newNSTemplateTier,
				turtest.Condition(templateupdaterequest.ToBeUpdating()),
				turtest.SyncIndexes{
					"cluster1": "10",
					"cluster2": "20",
				}))
			initObjs = append(initObjs, murtest.NewMasterUserRecord(t, "user-1",
				murtest.Account("cluster1", oldNSTemplateTier, murtest.SyncIndex("11")),               // here the sync index changed
				murtest.AdditionalAccount("cluster2", oldNSTemplateTier, murtest.SyncIndex("20")),     // here the sync index did not change
				murtest.AdditionalAccount("cluster3", otherNSTemplateTier, murtest.SyncIndex("100")))) // this account is not affected by the update
			r, req, cl := prepareReconcile(t, "user-1", initObjs...)
			// when
			res, err := r.Reconcile(req)
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
			initObjs := []runtime.Object{&newNSTemplateTier}
			initObjs = append(initObjs, turtest.NewTemplateUpdateRequest("user-1", newNSTemplateTier,
				turtest.Condition(templateupdaterequest.ToBeUpdating()),
				turtest.SyncIndexes(map[string]string{
					"cluster1": "10",
					"cluster2": "20",
				})))
			initObjs = append(initObjs, murtest.NewMasterUserRecord(t, "user-1",
				murtest.Account("cluster1", oldNSTemplateTier, murtest.SyncIndex("11")),               // here the sync index changed
				murtest.AdditionalAccount("cluster2", oldNSTemplateTier, murtest.SyncIndex("21")),     // here the sync index changed too
				murtest.AdditionalAccount("cluster3", otherNSTemplateTier, murtest.SyncIndex("100")))) // this account is not affected by the update
			r, req, cl := prepareReconcile(t, "user-1", initObjs...)
			// when
			res, err := r.Reconcile(req)
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
			initObjs := []runtime.Object{&newNSTemplateTier}
			initObjs = append(initObjs, turtest.NewTemplateUpdateRequest("user-1", newNSTemplateTier,
				turtest.Condition(templateupdaterequest.ToBeUpdating()),
				turtest.SyncIndexes(map[string]string{
					"cluster1": "10",
					"cluster2": "20",
				})))
			initObjs = append(initObjs, murtest.NewMasterUserRecord(t, "user-1",
				murtest.Account("cluster1", oldNSTemplateTier, murtest.SyncIndex("11")),              // here the sync index changed
				murtest.AdditionalAccount("cluster2", oldNSTemplateTier, murtest.SyncIndex("21")),    // here the sync index changed too
				murtest.AdditionalAccount("cluster3", otherNSTemplateTier, murtest.SyncIndex("100")), // account with another tier
				murtest.StatusCondition(toolchainv1alpha1.Condition{ // master user record is "ready"
					Type:   toolchainv1alpha1.ConditionReady,
					Status: corev1.ConditionTrue,
					Reason: toolchainv1alpha1.MasterUserRecordProvisionedReason,
				}),
			)) // this account is not affected by the update
			r, req, cl := prepareReconcile(t, "user-1", initObjs...)
			// when
			res, err := r.Reconcile(req)
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

	})

	t.Run("controller should mark TemplateUpdateRequest as failed", func(t *testing.T) {

		t.Run("when the MasterUserRecord was deleted", func(t *testing.T) {
			// given
			initObjs := []runtime.Object{&newNSTemplateTier}
			initObjs = append(initObjs, turtest.NewTemplateUpdateRequest("user-1", newNSTemplateTier))
			r, req, cl := prepareReconcile(t, "user-1", initObjs...) // there is no associated MasterUserRecord
			// when
			res, err := r.Reconcile(req)
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
	})

	t.Run("failures", func(t *testing.T) {

		t.Run("unable to get TemplateUpdateRequest", func(t *testing.T) {

			t.Run("tier not found", func(t *testing.T) {
				// given
				initObjs := []runtime.Object{&newNSTemplateTier}
				initObjs = append(initObjs, turtest.NewTemplateUpdateRequest("user-1", newNSTemplateTier))
				r, req, cl := prepareReconcile(t, "user-1", initObjs...) // there is no associated MasterUserRecord
				cl.MockGet = func(ctx context.Context, key types.NamespacedName, obj runtime.Object) error {
					if _, ok := obj.(*toolchainv1alpha1.TemplateUpdateRequest); ok {
						return errors.NewNotFound(schema.GroupResource{}, key.Name)
					}
					return cl.Client.Get(ctx, key, obj)
				}
				// when
				res, err := r.Reconcile(req)
				// then
				require.NoError(t, err)
				assert.Equal(t, reconcile.Result{}, res) // no explicit requeue
			})

			t.Run("other error", func(t *testing.T) {
				// given
				initObjs := []runtime.Object{&newNSTemplateTier}
				initObjs = append(initObjs, turtest.NewTemplateUpdateRequest("user-1", newNSTemplateTier))
				r, req, cl := prepareReconcile(t, "user-1", initObjs...) // there is no associated MasterUserRecord
				cl.MockGet = func(ctx context.Context, key types.NamespacedName, obj runtime.Object) error {
					if _, ok := obj.(*toolchainv1alpha1.TemplateUpdateRequest); ok {
						return fmt.Errorf("mock error!")
					}
					return cl.Client.Get(ctx, key, obj)
				}
				// when
				res, err := r.Reconcile(req)
				// then
				require.Error(t, err)
				assert.EqualError(t, err, "unable to get the current TemplateUpdateRequest: mock error!")
				assert.Equal(t, reconcile.Result{}, res) // no explicit requeue
			})
		})

		t.Run("unable to get associated MasterUserRecord", func(t *testing.T) {

			t.Run("tier not found", func(t *testing.T) {
				// given
				initObjs := []runtime.Object{&newNSTemplateTier}
				initObjs = append(initObjs, turtest.NewTemplateUpdateRequest("user-1", newNSTemplateTier))
				r, req, cl := prepareReconcile(t, "user-1", initObjs...) // there is no associated MasterUserRecord
				cl.MockGet = func(ctx context.Context, key types.NamespacedName, obj runtime.Object) error {
					if _, ok := obj.(*toolchainv1alpha1.MasterUserRecord); ok {
						return errors.NewNotFound(schema.GroupResource{}, key.Name)
					}
					return cl.Client.Get(ctx, key, obj)
				}
				// when
				res, err := r.Reconcile(req)
				// then
				require.NoError(t, err)
				assert.Equal(t, reconcile.Result{}, res) // no explicit requeue
			})

			t.Run("other error", func(t *testing.T) {
				// given
				initObjs := []runtime.Object{&newNSTemplateTier}
				initObjs = append(initObjs, turtest.NewTemplateUpdateRequest("user-1", newNSTemplateTier))
				r, req, cl := prepareReconcile(t, "user-1", initObjs...) // there is no associated MasterUserRecord
				cl.MockGet = func(ctx context.Context, key types.NamespacedName, obj runtime.Object) error {
					if _, ok := obj.(*toolchainv1alpha1.MasterUserRecord); ok {
						return fmt.Errorf("mock error!")
					}
					return cl.Client.Get(ctx, key, obj)
				}
				// when
				res, err := r.Reconcile(req)
				// then
				require.Error(t, err)
				assert.EqualError(t, err, "unable to get the MasterUserRecord associated with the TemplateUpdateRequest: mock error!")
				assert.Equal(t, reconcile.Result{}, res) // no explicit requeue
			})
		})

		t.Run("unable to update the TemplateUpdateRequest status", func(t *testing.T) {
			// given
			initObjs := []runtime.Object{&newNSTemplateTier}
			initObjs = append(initObjs, turtest.NewTemplateUpdateRequest("user-1", newNSTemplateTier))
			mur := murtest.NewMasterUserRecord(t, "user-1", murtest.Account("cluster1", oldNSTemplateTier, murtest.SyncIndex("1")))
			initObjs = append(initObjs, mur)
			r, req, cl := prepareReconcile(t, "user-1", initObjs...)
			cl.MockStatusUpdate = func(ctx context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
				if _, ok := obj.(*toolchainv1alpha1.TemplateUpdateRequest); ok {
					return fmt.Errorf("mock error!")
				}
				return cl.Client.Status().Update(ctx, obj, opts...)
			}
			// when
			res, err := r.Reconcile(req)
			// then
			require.Error(t, err)
			assert.EqualError(t, err, "unable to update the TemplateUpdateRequest status: mock error!")
			assert.Equal(t, reconcile.Result{}, res) // no explicit requeue
		})

		t.Run("unable to update the MasterUserRecord", func(t *testing.T) {
			// given
			initObjs := []runtime.Object{&newNSTemplateTier}
			initObjs = append(initObjs, turtest.NewTemplateUpdateRequest("user-1", newNSTemplateTier))
			mur := murtest.NewMasterUserRecord(t, "user-1", murtest.Account("cluster1", oldNSTemplateTier, murtest.SyncIndex("1")))
			initObjs = append(initObjs, mur)
			r, req, cl := prepareReconcile(t, "user-1", initObjs...)
			cl.MockUpdate = func(ctx context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
				if _, ok := obj.(*toolchainv1alpha1.MasterUserRecord); ok {
					return fmt.Errorf("mock error!")
				}
				return cl.Client.Update(ctx, obj, opts...)
			}
			// when
			res, err := r.Reconcile(req)
			// then
			require.Error(t, err)
			assert.EqualError(t, err, "unable to update the MasterUserRecord associated with the TemplateUpdateRequest: mock error!")
			assert.Equal(t, reconcile.Result{}, res) // no explicit requeue
		})

	})

}

func prepareReconcile(t *testing.T, name string, initObjs ...runtime.Object) (reconcile.Reconciler, reconcile.Request, *test.FakeClient) {
	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)
	cl := test.NewFakeClient(t, initObjs...)
	r := templateupdaterequest.NewReconciler(cl, s)
	return r, reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      name,
			Namespace: operatorNamespace,
		},
	}, cl
}
