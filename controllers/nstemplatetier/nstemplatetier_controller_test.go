package nstemplatetier_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/controllers/nstemplatetier"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	tiertest "github.com/codeready-toolchain/host-operator/test/nstemplatetier"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
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

func TestReconcile(t *testing.T) {

	// given
	logf.SetLogger(zap.New(zap.UseDevMode(true)))

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
			r, req, cl := prepareReconcile(t, base1nsTier.Name, initObjs...)
			// when
			res, err := r.Reconcile(context.TODO(), req)
			// then
			require.NoError(t, err)
			require.Equal(t, reconcile.Result{}, res) // no explicit requeue
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
				assert.EqualError(t, err, "unable to get the current NSTemplateTier: mock error")
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
