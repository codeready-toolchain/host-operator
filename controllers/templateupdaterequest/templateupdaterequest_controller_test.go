package templateupdaterequest_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	tierutil "github.com/codeready-toolchain/host-operator/controllers/nstemplatetier/util"
	"github.com/codeready-toolchain/host-operator/controllers/templateupdaterequest"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	tiertest "github.com/codeready-toolchain/host-operator/test/nstemplatetier"
	spacetest "github.com/codeready-toolchain/host-operator/test/space"
	turtest "github.com/codeready-toolchain/host-operator/test/templateupdaterequest"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"

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

		t.Run("unable to update the TemplateUpdateRequest status", func(t *testing.T) {
			// given
			previousBasicTier := tiertest.BasicTier(t, tiertest.PreviousBasicTemplates)
			basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates, tiertest.WithCurrentUpdateInProgress())
			space := spacetest.NewSpace("user-1", spacetest.WithTierNameAndHashLabelFor(previousBasicTier)) // space's hash matches TUR hash
			r, req, cl := prepareReconcile(t, basicTier, turtest.NewTemplateUpdateRequest("user-1", *basicTier), space)
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
