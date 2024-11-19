package nstemplatetier

import (
	"context"
	"testing"

	"github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestMapTierTemplateRevisionToNSTemplateTier(t *testing.T) {
	// given
	t.Run("should return no nstemplatetier associated with a ttr", func(t *testing.T) {
		// when
		// ttr with missing label
		tiertemplateRevisionNoTmplTier := v1alpha1.TierTemplateRevision{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "testns",
				Name:      "testttr",
				Labels: map[string]string{
					v1alpha1.TemplateRefLabelKey: "testtiertemplate",
				},
			},
		}
		requests := MapTierTemplateRevisionToNSTemplateTier()(context.TODO(), &tiertemplateRevisionNoTmplTier)

		// then
		require.Empty(t, requests)
	})

	t.Run("should return nstemplatetier associated with ttr", func(t *testing.T) {
		// when
		// ttr with nstemplatetier
		tiertemplateRevision := v1alpha1.TierTemplateRevision{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "testns",
				Name:      "testttr",
				Labels: map[string]string{
					v1alpha1.TierLabelKey:        "base1ns", // this is the nstemplatetier associated with the ttr
					v1alpha1.TemplateRefLabelKey: "testtiertemplate",
				},
			},
		}
		requests := MapTierTemplateRevisionToNSTemplateTier()(context.TODO(), &tiertemplateRevision)

		// then
		require.Len(t, requests, 1)
		assert.Contains(t, requests, newRequest("base1ns", "testns"))
	})
}

func newRequest(name, namespace string) reconcile.Request {
	return reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: namespace,
			Name:      name,
		},
	}
}
