package tiertemplatecleanup

import (
	"context"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestMapNSTemplateTierToTierTemplates(t *testing.T) {
	t.Run("returns empty slice for non-NSTemplateTier object", func(t *testing.T) {
		// given
		obj := &toolchainv1alpha1.TierTemplate{} // different type

		// when
		reqs := MapNSTemplateTierToTierTemplates(context.TODO(), obj)

		// then
		assert.Empty(t, reqs)
	})

	t.Run("returns requests for all template types", func(t *testing.T) {
		// given
		nstt := &toolchainv1alpha1.NSTemplateTier{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "advanced",
				Namespace: "toolchain-host-operator",
			},
			Spec: toolchainv1alpha1.NSTemplateTierSpec{
				ClusterResources: &toolchainv1alpha1.NSTemplateTierClusterResources{
					TemplateRef: "advanced-clusterresources",
				},
				Namespaces: []toolchainv1alpha1.NSTemplateTierNamespace{
					{TemplateRef: "advanced-dev"},
					{TemplateRef: "advanced-stage"},
					{TemplateRef: "advanced-prod"},
				},
				SpaceRoles: map[string]toolchainv1alpha1.NSTemplateTierSpaceRole{
					"admin":       {TemplateRef: "advanced-admin"},
					"maintainer":  {TemplateRef: "advanced-maintainer"},
					"contributor": {TemplateRef: "advanced-contributor"},
				},
			},
		}

		// when
		reqs := MapNSTemplateTierToTierTemplates(context.TODO(), nstt)

		// then
		require.Len(t, reqs, 7) // 1 cluster + 3 namespaces + 3 space roles

		// Check cluster resources request
		assert.Contains(t, reqs, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      "advanced-clusterresources",
				Namespace: "toolchain-host-operator",
			},
		})

		// Check namespace requests
		assert.Contains(t, reqs, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      "advanced-dev",
				Namespace: "toolchain-host-operator",
			},
		})
		assert.Contains(t, reqs, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      "advanced-stage",
				Namespace: "toolchain-host-operator",
			},
		})
		assert.Contains(t, reqs, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      "advanced-prod",
				Namespace: "toolchain-host-operator",
			},
		})

		// Check space role requests
		assert.Contains(t, reqs, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      "advanced-admin",
				Namespace: "toolchain-host-operator",
			},
		})
		assert.Contains(t, reqs, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      "advanced-maintainer",
				Namespace: "toolchain-host-operator",
			},
		})
		assert.Contains(t, reqs, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      "advanced-contributor",
				Namespace: "toolchain-host-operator",
			},
		})
	})

	t.Run("handles nil cluster resources", func(t *testing.T) {
		// given
		ctx := context.TODO()
		nstt := &toolchainv1alpha1.NSTemplateTier{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "basic",
				Namespace: "toolchain-host-operator",
			},
			Spec: toolchainv1alpha1.NSTemplateTierSpec{
				ClusterResources: nil,
				Namespaces: []toolchainv1alpha1.NSTemplateTierNamespace{
					{TemplateRef: "basic-dev"},
				},
			},
		}

		// when
		reqs := MapNSTemplateTierToTierTemplates(ctx, nstt)

		// then
		require.Len(t, reqs, 1)
		assert.Equal(t, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      "basic-dev",
				Namespace: "toolchain-host-operator",
			},
		}, reqs[0])
	})

	t.Run("handles empty template ref in cluster resources", func(t *testing.T) {
		// given
		ctx := context.TODO()
		nstt := &toolchainv1alpha1.NSTemplateTier{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "basic",
				Namespace: "toolchain-host-operator",
			},
			Spec: toolchainv1alpha1.NSTemplateTierSpec{
				ClusterResources: &toolchainv1alpha1.NSTemplateTierClusterResources{
					TemplateRef: "",
				},
				Namespaces: []toolchainv1alpha1.NSTemplateTierNamespace{
					{TemplateRef: "basic-dev"},
				},
			},
		}

		// when
		reqs := MapNSTemplateTierToTierTemplates(ctx, nstt)

		// then
		require.Len(t, reqs, 1)
		assert.Equal(t, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      "basic-dev",
				Namespace: "toolchain-host-operator",
			},
		}, reqs[0])
	})

	t.Run("uses correct namespace from NSTemplateTier", func(t *testing.T) {
		// given
		customNamespace := "custom-host-operator"
		nstt := &toolchainv1alpha1.NSTemplateTier{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "basic",
				Namespace: customNamespace,
			},
			Spec: toolchainv1alpha1.NSTemplateTierSpec{
				ClusterResources: &toolchainv1alpha1.NSTemplateTierClusterResources{
					TemplateRef: "basic-clusterresources",
				},
				Namespaces: []toolchainv1alpha1.NSTemplateTierNamespace{
					{TemplateRef: "basic-dev"},
				},
				SpaceRoles: map[string]toolchainv1alpha1.NSTemplateTierSpaceRole{
					"admin": {TemplateRef: "basic-admin"},
				},
			},
		}

		// when
		reqs := MapNSTemplateTierToTierTemplates(context.TODO(), nstt)

		// then
		require.Len(t, reqs, 3)
		for _, req := range reqs {
			assert.Equal(t, customNamespace, req.Namespace, "all requests should use the NSTemplateTier's namespace")
		}
	})
}

