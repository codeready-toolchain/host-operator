package nstemplatetierrevisioncleanup_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/controllers/nstemplatetierrevisioncleanup"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	tiertest "github.com/codeready-toolchain/host-operator/test/nstemplatetier"
	"github.com/codeready-toolchain/host-operator/test/tiertemplaterevision"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	spacetest "github.com/codeready-toolchain/toolchain-common/pkg/test/space"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/kubectl/pkg/scheme"
	controllerruntime "sigs.k8s.io/controller-runtime"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestTTRDeletionReconcile(t *testing.T) {
	oldCreationTime := metav1.NewTime(time.Now().Add(-time.Minute))
	nsTemplateTier := tiertest.Base1nsTier(t, tiertest.CurrentBase1nsTemplates, tiertest.WithStatusRevisions())
	ttrName := nsTemplateTier.Spec.ClusterResources.TemplateRef + "-ttrcr"
	ttr := newTTR(*nsTemplateTier, ttrName, oldCreationTime)
	s := newSpace(nsTemplateTier)
	t.Run("TTR Deleted Successfully", func(t *testing.T) {
		//given
		r, req, cl := prepareReconcile(t, ttr.Name, ttr, s, nsTemplateTier)
		//when
		res, err := r.Reconcile(context.TODO(), req)
		//then
		require.NoError(t, err)
		require.Equal(t, controllerruntime.Result{}, res)
		tiertemplaterevision.AssertThatTTRs(t, cl, nsTemplateTier.GetNamespace()).DoNotExist()
	})

	t.Run("TTR has deletion time Stamp", func(t *testing.T) {
		//given
		ttr.DeletionTimestamp = &metav1.Time{
			Time: time.Now(),
		}
		controllerutil.AddFinalizer(ttr, toolchainv1alpha1.FinalizerName)
		r, req, cl := prepareReconcile(t, ttr.Name, ttr, s, nsTemplateTier)
		//when
		res, err := r.Reconcile(context.TODO(), req)
		//then
		require.NoError(t, err)
		require.Equal(t, controllerruntime.Result{}, res)
		tiertemplaterevision.AssertThatTTRs(t, cl, nsTemplateTier.GetNamespace()).ExistFor(nsTemplateTier.Name)
	})

	t.Run("the creation timestamp is less than 30 sec", func(t *testing.T) {
		// given
		ttr := newTTR(*nsTemplateTier, ttrName, metav1.NewTime(time.Now().Add(-29*time.Second)))
		r, req, cl := prepareReconcile(t, ttr.Name, ttr, s, nsTemplateTier)

		// when
		res, err := r.Reconcile(context.TODO(), req)

		// then
		require.NoError(t, err)
		require.LessOrEqual(t, res.RequeueAfter, time.Second)
		tiertemplaterevision.AssertThatTTRs(t, cl, nsTemplateTier.GetNamespace()).ExistFor(nsTemplateTier.Name)

	})
	t.Run("ttr is still being referenced in status.revisions", func(t *testing.T) {
		// given
		ttr := newTTR(*nsTemplateTier, nsTemplateTier.Spec.ClusterResources.TemplateRef+"-ttr", oldCreationTime)
		r, req, cl := prepareReconcile(t, ttr.Name, ttr, s, nsTemplateTier)

		// when
		res, err := r.Reconcile(context.TODO(), req)

		// then
		require.NoError(t, err)
		require.Equal(t, controllerruntime.Result{}, res)
		tiertemplaterevision.AssertThatTTRs(t, cl, nsTemplateTier.GetNamespace()).ExistFor(nsTemplateTier.Name)

	})

	t.Run("spaces are still being updated", func(t *testing.T) {
		// given
		nsTemplateTier := tiertest.Base1nsTier(t, tiertest.CurrentBase1nsTemplates, tiertest.WithStatusRevisions())
		nsTemplateTier.Status.Revisions = map[string]string{
			"base1ns-code-123456new":             "base1ns-code-123456new-ttr",
			"base1ns-clusterresources-123456new": "base1ns-clusterresources-123456new-ttr",
		}
		ttr := newTTR(*nsTemplateTier, ttrName, oldCreationTime)
		r, req, cl := prepareReconcile(t, ttr.Name, ttr, s, nsTemplateTier)

		// when
		res, err := r.Reconcile(context.TODO(), req)

		// then
		require.NoError(t, err)
		require.Equal(t, controllerruntime.Result{}, res)
		tiertemplaterevision.AssertThatTTRs(t, cl, nsTemplateTier.GetNamespace()).ExistFor(nsTemplateTier.Name)
	})

	t.Run("Failure", func(t *testing.T) {
		deleteErr := "unable to delete the current Tier Template Revision base1ns-clusterresources-123456new-ttrcr: some error cannot delete"
		nsTemplateTier := tiertest.Base1nsTier(t, tiertest.CurrentBase1nsTemplates, tiertest.WithStatusRevisions())
		ttr := newTTR(*nsTemplateTier, ttrName, oldCreationTime)
		s := newSpace(nsTemplateTier)
		t.Run("Error while deleting the TTR", func(t *testing.T) {
			// given
			ttr := newTTR(*nsTemplateTier, ttrName, oldCreationTime)
			s := newSpace(nsTemplateTier)
			r, req, cl := prepareReconcile(t, ttr.Name, ttr, s, nsTemplateTier)
			cl.MockDelete = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.DeleteOption) error {
				return fmt.Errorf("some error cannot delete")
			}
			// when
			res, err := r.Reconcile(context.TODO(), req)

			// then
			require.EqualError(t, err, deleteErr)
			require.Equal(t, controllerruntime.Result{}, res)
			tiertemplaterevision.AssertThatTTRs(t, cl, nsTemplateTier.GetNamespace()).ExistFor(nsTemplateTier.Name)
		})

		t.Run("Is Not Found Error-already deleted, while deleting the TTR", func(t *testing.T) {
			// given
			ttr := newTTR(*nsTemplateTier, ttrName, oldCreationTime)
			s := newSpace(nsTemplateTier)
			r, req, cl := prepareReconcile(t, ttr.Name, ttr, s, nsTemplateTier)
			cl.MockDelete = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.DeleteOption) error {
				return errors.NewNotFound(schema.GroupResource{}, ttr.Name)
			}

			// when
			res, err := r.Reconcile(context.TODO(), req)

			// then
			require.NoError(t, err)
			require.Equal(t, controllerruntime.Result{}, res)
		})
		t.Run("error while getting revision", func(t *testing.T) {
			r, req, cl := prepareReconcile(t, ttr.Name, ttr, s, nsTemplateTier)
			cl.MockGet = func(ctx context.Context, key runtimeclient.ObjectKey, obj runtimeclient.Object, opts ...runtimeclient.GetOption) error {
				return fmt.Errorf("some error cannot get")
			}
			// when
			res, err := r.Reconcile(context.TODO(), req)

			// then
			require.EqualError(t, err, "unable to get the current TierTemplateRevision: some error cannot get")
			require.Equal(t, controllerruntime.Result{}, res)
			tiertemplaterevision.AssertThatTTRs(t, cl, nsTemplateTier.GetNamespace()).ExistFor(nsTemplateTier.Name)
		})

		t.Run("error while getting NSTemplate Tier", func(t *testing.T) {
			r, req, cl := prepareReconcile(t, ttr.Name, ttr, nsTemplateTier)

			cl.MockGet = func(ctx context.Context, key types.NamespacedName, obj runtimeclient.Object, opts ...runtimeclient.GetOption) error {
				if _, ok := obj.(*toolchainv1alpha1.NSTemplateTier); ok {
					return fmt.Errorf("mock error")
				}
				return cl.Client.Get(ctx, key, obj, opts...)
			}
			//when
			_, err := r.Reconcile(context.TODO(), req)
			//then
			require.EqualError(t, err, "unable to get the current NSTemplateTier: mock error")
			tiertemplaterevision.AssertThatTTRs(t, cl, nsTemplateTier.GetNamespace()).ExistFor(nsTemplateTier.Name)

		})
		t.Run("error while listing outdated spaces", func(t *testing.T) {
			r, req, cl := prepareReconcile(t, ttr.Name, ttr, s, nsTemplateTier)
			cl.MockList = func(ctx context.Context, list runtimeclient.ObjectList, opts ...runtimeclient.ListOption) error {
				return fmt.Errorf("some error")
			}
			// when
			res, err := r.Reconcile(context.TODO(), req)

			// then
			require.EqualError(t, err, "some error")
			require.Equal(t, controllerruntime.Result{}, res)
			tiertemplaterevision.AssertThatTTRs(t, cl, nsTemplateTier.GetNamespace()).ExistFor(nsTemplateTier.Name)
		})

		t.Run("revision not found", func(t *testing.T) {
			r, req, _ := prepareReconcile(t, "base")
			//when
			res, err := r.Reconcile(context.TODO(), req)
			//then
			require.NoError(t, err)
			require.Equal(t, controllerruntime.Result{}, res)

		})
		t.Run("NSTemplate Tier not found - ttr gets deleted", func(t *testing.T) {
			//given
			r, req, cl := prepareReconcile(t, ttr.Name, ttr)
			//when
			res, err := r.Reconcile(context.TODO(), req)
			//then
			require.NoError(t, err)
			require.Equal(t, controllerruntime.Result{}, res)
			tiertemplaterevision.AssertThatTTRs(t, cl, nsTemplateTier.GetNamespace()).DoNotExist()

		})

		t.Run("NSTemplate Tier not found, but error deleting ttr", func(t *testing.T) {
			r, req, cl := prepareReconcile(t, ttr.Name, ttr)
			cl.MockDelete = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.DeleteOption) error {
				return fmt.Errorf("some error cannot delete")
			}
			//when
			res, err := r.Reconcile(context.TODO(), req)
			//then
			require.EqualError(t, err, deleteErr)
			require.Equal(t, controllerruntime.Result{}, res)
			tiertemplaterevision.AssertThatTTRs(t, cl, nsTemplateTier.GetNamespace()).ExistFor(nsTemplateTier.Name)
		})
		t.Run("tier label not found - ttr gets deleted", func(t *testing.T) {
			ttr := newTTR(*nsTemplateTier, ttrName, oldCreationTime)
			ttr.Labels = map[string]string{}
			r, req, cl := prepareReconcile(t, ttr.Name, ttr)
			//when
			res, err := r.Reconcile(context.TODO(), req)
			//then
			require.NoError(t, err)
			require.Equal(t, controllerruntime.Result{}, res)
			tiertemplaterevision.AssertThatTTRs(t, cl, nsTemplateTier.GetNamespace()).DoNotExist()
		})

		t.Run("tier label not found but error deleting ttr", func(t *testing.T) {
			ttr := newTTR(*nsTemplateTier, ttrName, oldCreationTime)
			ttr.Labels = map[string]string{}
			r, req, cl := prepareReconcile(t, ttr.Name, ttr)
			cl.MockDelete = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.DeleteOption) error {
				return fmt.Errorf("some error cannot delete")
			}
			//when
			res, err := r.Reconcile(context.TODO(), req)
			//then
			require.EqualError(t, err, deleteErr)
			require.Equal(t, controllerruntime.Result{}, res)
			tiertemplaterevision.AssertThatTTRs(t, cl, nsTemplateTier.GetNamespace()).ExistFor(nsTemplateTier.Name)
		})

	})

}

func prepareReconcile(t *testing.T, name string, initObjs ...runtimeclient.Object) (*nstemplatetierrevisioncleanup.Reconciler, reconcile.Request, *test.FakeClient) {
	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)
	cl := test.NewFakeClient(t, initObjs...)
	r := &nstemplatetierrevisioncleanup.Reconciler{
		Client: cl,
	}
	return r, reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      name,
			Namespace: test.HostOperatorNs,
		},
	}, cl
}
func newTTR(nsTTier toolchainv1alpha1.NSTemplateTier, name string, crtime metav1.Time) *toolchainv1alpha1.TierTemplateRevision {
	labels := map[string]string{
		toolchainv1alpha1.TierLabelKey: nsTTier.Name,
	}
	ttr := &toolchainv1alpha1.TierTemplateRevision{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:         test.HostOperatorNs,
			Name:              name,
			Labels:            labels,
			CreationTimestamp: crtime,
		},
	}
	return ttr
}

func newSpace(nsTTier *toolchainv1alpha1.NSTemplateTier) *toolchainv1alpha1.Space {
	testSpace := spacetest.NewSpace(test.HostOperatorNs, "oddity1",
		spacetest.WithTierNameAndHashLabelFor(nsTTier))
	return testSpace
}
