package tiertemplate

import (
	"context"
	"fmt"
	"testing"
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const tierTemplateName = "test-tier-template"

func TestAddFinalizer(t *testing.T) {
	t.Run("should add finalizer when not present", func(t *testing.T) {
		// given
		tierTemplate := newTierTemplate()
		r, cl := prepareReconcile(t, tierTemplate)

		// when
		_, err := r.Reconcile(context.TODO(), reconcileRequestFor(tierTemplate))

		// then
		require.NoError(t, err)

		// verify finalizer was added
		updatedTierTemplate := &toolchainv1alpha1.TierTemplate{}
		err = cl.Get(context.TODO(), client.ObjectKeyFromObject(tierTemplate), updatedTierTemplate)
		require.NoError(t, err)
		require.Contains(t, updatedTierTemplate.Finalizers, toolchainv1alpha1.FinalizerName)
	})

	t.Run("should not duplicate finalizer when already present", func(t *testing.T) {
		// given
		tierTemplate := newTierTemplate(withFinalizer())
		r, cl := prepareReconcile(t, tierTemplate)

		// when
		_, err := r.Reconcile(context.TODO(), reconcileRequestFor(tierTemplate))

		// then
		require.NoError(t, err)

		// verify finalizer count remains the same
		updatedTierTemplate := &toolchainv1alpha1.TierTemplate{}
		err = cl.Get(context.TODO(), client.ObjectKeyFromObject(tierTemplate), updatedTierTemplate)
		require.NoError(t, err)
		require.Len(t, updatedTierTemplate.Finalizers, 1)
		require.Contains(t, updatedTierTemplate.Finalizers, toolchainv1alpha1.FinalizerName)
	})

	t.Run("should handle not found resource", func(t *testing.T) {
		// given
		r, _ := prepareReconcile(t)
		request := controllerruntime.Request{
			NamespacedName: types.NamespacedName{
				Name:      "non-existent",
				Namespace: test.HostOperatorNs,
			},
		}

		// when
		_, err := r.Reconcile(context.TODO(), request)

		// then
		require.NoError(t, err)
	})

	t.Run("should fail if unable to add the finalizer", func(t *testing.T) {
		// given
		tierTemplate := newTierTemplate()
		r, cl := prepareReconcile(t, tierTemplate)
		request := controllerruntime.Request{
			NamespacedName: types.NamespacedName{
				Name:      tierTemplateName,
				Namespace: test.HostOperatorNs,
			},
		}

		cl.MockUpdate = func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
			return fmt.Errorf("intentional fail to update")
		}

		// when
		_, err := r.Reconcile(context.TODO(), request)

		// then
		require.Error(t, err)
		assert.Equal(t, "failed to add the finalizer: intentional fail to update", err.Error())
	})
}

func TestCleanup(t *testing.T) {
	t.Run("should remove finalizer when TierTemplate is not used", func(t *testing.T) {
		// given
		tierTemplate := newTierTemplate(withFinalizer(), withDeletionTimestamp())
		nsTemplateTier := newNSTemplateTier("other-tier")
		r, cl := prepareReconcile(t, tierTemplate, nsTemplateTier)

		// when
		_, err := r.Reconcile(context.TODO(), reconcileRequestFor(tierTemplate))

		// then
		require.NoError(t, err)

		// verify finalizer was removed
		updatedTierTemplate := &toolchainv1alpha1.TierTemplate{}
		err = cl.Get(context.TODO(), client.ObjectKeyFromObject(tierTemplate), updatedTierTemplate)
		require.True(t, errors.IsNotFound(err))
	})

	t.Run("should not remove finalizer when TierTemplate is used in namespace template", func(t *testing.T) {
		// given
		tierTemplate := newTierTemplate(withFinalizer(), withDeletionTimestamp())
		nsTemplateTier := newNSTemplateTier("basic-tier", withNamespaceTemplate())
		r, _ := prepareReconcile(t, tierTemplate, nsTemplateTier)

		// when
		_, err := r.Reconcile(context.TODO(), reconcileRequestFor(tierTemplate))

		// then
		require.Error(t, err)
		require.Equal(t, "TierTemplate still used by some NSTemplateTier", err.Error())
	})

	t.Run("should not remove finalizer when TierTemplate is used in cluster resources", func(t *testing.T) {
		// given
		tierTemplate := newTierTemplate(withFinalizer(), withDeletionTimestamp())
		nsTemplateTier := newNSTemplateTier("basic-tier", withClusterResourceTemplate())
		r, _ := prepareReconcile(t, tierTemplate, nsTemplateTier)

		// when
		_, err := r.Reconcile(context.TODO(), reconcileRequestFor(tierTemplate))

		// then
		require.Error(t, err)
	})

	t.Run("should not remove finalizer when TierTemplate is used in space roles", func(t *testing.T) {
		// given
		tierTemplate := newTierTemplate(withFinalizer(), withDeletionTimestamp())
		nsTemplateTier := newNSTemplateTier("basic-tier", withSpaceRoleTemplate("admin"))
		r, _ := prepareReconcile(t, tierTemplate, nsTemplateTier)

		// when
		_, err := r.Reconcile(context.TODO(), reconcileRequestFor(tierTemplate))

		// then
		require.Error(t, err)
	})
}

func newTierTemplate(options ...tierTemplateOption) *toolchainv1alpha1.TierTemplate {
	tt := &toolchainv1alpha1.TierTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      tierTemplateName,
			Namespace: test.HostOperatorNs,
		},
	}
	for _, apply := range options {
		apply(tt)
	}
	return tt
}

type tierTemplateOption func(*toolchainv1alpha1.TierTemplate)

func withFinalizer() tierTemplateOption {
	return func(tt *toolchainv1alpha1.TierTemplate) {
		tt.Finalizers = []string{toolchainv1alpha1.FinalizerName}
	}
}

func withDeletionTimestamp() tierTemplateOption {
	return func(tt *toolchainv1alpha1.TierTemplate) {
		tt.DeletionTimestamp = &metav1.Time{Time: time.Now()}
	}
}

func newNSTemplateTier(name string, options ...nsTemplateTierOption) *toolchainv1alpha1.NSTemplateTier {
	nstt := &toolchainv1alpha1.NSTemplateTier{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: test.HostOperatorNs,
		},
	}
	for _, apply := range options {
		apply(nstt)
	}
	return nstt
}

type nsTemplateTierOption func(*toolchainv1alpha1.NSTemplateTier)

func withClusterResourceTemplate() nsTemplateTierOption {
	return func(nstt *toolchainv1alpha1.NSTemplateTier) {
		nstt.Spec.ClusterResources = &toolchainv1alpha1.NSTemplateTierClusterResources{
			TemplateRef: tierTemplateName,
		}
	}
}

func withSpaceRoleTemplate(role string) nsTemplateTierOption {
	return func(nstt *toolchainv1alpha1.NSTemplateTier) {
		if nstt.Spec.SpaceRoles == nil {
			nstt.Spec.SpaceRoles = make(map[string]toolchainv1alpha1.NSTemplateTierSpaceRole)
		}
		nstt.Spec.SpaceRoles[role] = toolchainv1alpha1.NSTemplateTierSpaceRole{
			TemplateRef: tierTemplateName,
		}
	}
}

func withNamespaceTemplate() nsTemplateTierOption {
	return func(nstt *toolchainv1alpha1.NSTemplateTier) {
		nstt.Spec.Namespaces = []toolchainv1alpha1.NSTemplateTierNamespace{
			{TemplateRef: tierTemplateName},
		}
	}
}

func prepareReconcile(t *testing.T, initObjs ...client.Object) (*Reconciler, *test.FakeClient) {
	s := runtime.NewScheme()
	err := apis.AddToScheme(s)
	require.NoError(t, err)
	cl := test.NewFakeClient(t, initObjs...)
	r := &Reconciler{
		Client: cl,
	}
	return r, cl
}

func reconcileRequestFor(obj client.Object) controllerruntime.Request {
	return controllerruntime.Request{
		NamespacedName: types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()},
	}
}
