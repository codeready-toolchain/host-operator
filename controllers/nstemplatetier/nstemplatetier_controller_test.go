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
	"github.com/codeready-toolchain/toolchain-common/pkg/test"

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
					Hash: basicTier.Labels["toolchain.dev.openshift.com/basic-tier-hash"],
				})
		})

		t.Run("with previous entries", func(t *testing.T) {
			// given
			basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates, tiertest.WithPreviousUpdates(
				toolchainv1alpha1.NSTemplateTierHistory{
					StartTime: metav1.Now(),
					Hash:      "abc123",
				},
				toolchainv1alpha1.NSTemplateTierHistory{
					StartTime: metav1.Now(),
					Hash:      "def456",
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
					Hash: basicTier.Labels["toolchain.dev.openshift.com/basic-tier-hash"],
				})
		})
	})

	t.Run("controller should NOT add entry in tier.status.updates", func(t *testing.T) {

		t.Run("last entry exists with matching hash", func(t *testing.T) {
			// given
			basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates,
				tiertest.WithCurrentUpdate()) // current update already exists

			initObjs := []runtime.Object{basicTier}
			r, req, cl := prepareReconcile(t, basicTier.Name, initObjs...)
			// when
			res, err := r.Reconcile(context.TODO(), req)
			// then
			require.NoError(t, err)
			require.Equal(t, reconcile.Result{}, res) // no explicit requeue
			// check that no entry was added in `status.updates`
			tiertest.AssertThatNSTemplateTier(t, "basic", cl).
				HasStatusUpdatesItems(1). // same number of entries
				HasLatestUpdate(toolchainv1alpha1.NSTemplateTierHistory{
					Hash: basicTier.Labels["toolchain.dev.openshift.com/basic-tier-hash"],
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

		t.Run("unable to update NSTemplateTier status", func(t *testing.T) {

			t.Run("when adding new update", func(t *testing.T) {
				// given
				basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates)
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
				assert.EqualError(t, err, "unable to insert a new entry in status.updates after NSTemplateTier changed: mock error")
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
