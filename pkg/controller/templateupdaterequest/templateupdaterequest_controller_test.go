package templateupdaterequest

import (
	"testing"
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	turtest "github.com/codeready-toolchain/host-operator/test/templateupdaterequest"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	murtest "github.com/codeready-toolchain/toolchain-common/pkg/test/masteruserrecord"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
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
			turtest.AssertThatTemplateUpdateRequest(t, "user-1", cl).
				HasConditions(toBeUpdating()).
				HasSyncIndexes(map[string]string{
					"cluster1": "1",
				})
		})

		t.Run("when there are many target clusters to update", func(t *testing.T) {
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
			turtest.AssertThatTemplateUpdateRequest(t, "user-1", cl).
				HasConditions(toBeUpdating()).
				HasSyncIndexes(map[string]string{
					"cluster1": "10",
					"cluster2": "20",
				})
		})
	})

	t.Run("controller should not delete the TemplateUpdateRequest", func(t *testing.T) {

		t.Run("when the MasterUserRecord is not up-to-date yet", func(t *testing.T) {
			// given
			initObjs := []runtime.Object{&newNSTemplateTier}
			initObjs = append(initObjs, turtest.NewTemplateUpdateRequest("user-1", newNSTemplateTier,
				turtest.Condition(toBeUpdating()),
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
			require.NoError(t, err)
			require.Equal(t, reconcile.Result{}, res) // no need to requeue, the MUR is watched
			turtest.AssertThatTemplateUpdateRequest(t, "user-1", cl).
				HasConditions(toBeUpdating()).
				HasSyncIndexes(map[string]string{
					"cluster1": "10",
					"cluster2": "20",
				})
		})

		t.Run("when the MasterUserRecord is up-to-date but not in 'ready' state yet", func(t *testing.T) {
			// given
			initObjs := []runtime.Object{&newNSTemplateTier}
			initObjs = append(initObjs, turtest.NewTemplateUpdateRequest("user-1", newNSTemplateTier,
				turtest.Condition(toBeUpdating()),
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
			require.NoError(t, err)
			require.Equal(t, reconcile.Result{}, res) // no need to requeue, the MUR is watched
			turtest.AssertThatTemplateUpdateRequest(t, "user-1", cl).
				HasConditions(toBeUpdating()).
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
				turtest.Condition(toBeUpdating()),
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
			require.NoError(t, err)
			require.Equal(t, reconcile.Result{Requeue: true, RequeueAfter: DeletionTimeout}, res) // now we need to requeue until timeout is done
			turtest.AssertThatTemplateUpdateRequest(t, "user-1", cl).
				HasConditions(toolchainv1alpha1.Condition{
					Type:   toolchainv1alpha1.TemplateUpdateRequestComplete,
					Status: corev1.ConditionTrue,
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
			require.NoError(t, err)
			require.Equal(t, reconcile.Result{}, res)
			turtest.AssertThatTemplateUpdateRequest(t, "user-1", cl).
				HasConditions(toolchainv1alpha1.Condition{
					Type:    toolchainv1alpha1.TemplateUpdateRequestComplete,
					Status:  corev1.ConditionFalse,
					Reason:  toolchainv1alpha1.TemplateUpdateRequestFailedReason,
					Message: `masteruserrecords.toolchain.dev.openshift.com "user-1" not found`,
				})
		})
	})

	t.Run("controller should delete the TemplateUpdateRequest", func(t *testing.T) {

		t.Run("when the timeout occurred", func(t *testing.T) {
			// given
			initObjs := []runtime.Object{&newNSTemplateTier}
			initObjs = append(initObjs, turtest.NewTemplateUpdateRequest("user-1", newNSTemplateTier, turtest.CompleteFor(2*DeletionTimeout)))
			r, req, cl := prepareReconcile(t, "user-1", initObjs...) // there is no associated MasterUserRecord
			// when
			res, err := r.Reconcile(req)
			require.NoError(t, err)
			require.Equal(t, reconcile.Result{}, res)
			turtest.AssertThatTemplateUpdateRequest(t, "user-1", cl).DoesNotExist()
		})

		t.Run("not when the timeout did not occur", func(t *testing.T) {
			// given
			initObjs := []runtime.Object{&newNSTemplateTier}
			initObjs = append(initObjs, turtest.NewTemplateUpdateRequest("user-1", newNSTemplateTier, turtest.CompleteFor(DeletionTimeout-2*time.Second)))
			r, req, cl := prepareReconcile(t, "user-1", initObjs...) // there is no associated MasterUserRecord
			// when
			res, err := r.Reconcile(req)
			require.NoError(t, err)
			assert.True(t, res.Requeue)
			assert.GreaterOrEqual(t, int64(res.RequeueAfter), int64(time.Second))
			turtest.AssertThatTemplateUpdateRequest(t, "user-1", cl).Exists()
		})
	})
}

func prepareReconcile(t *testing.T, name string, initObjs ...runtime.Object) (*ReconcileTemplateUpdateRequest, reconcile.Request, *test.FakeClient) {
	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)
	cl := test.NewFakeClient(t, initObjs...)
	r := &ReconcileTemplateUpdateRequest{
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
