package nstemplatetier_test

import (
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/controllers/nstemplatetier"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestComputeHash(t *testing.T) {

	t.Run("should match without cluster resources", func(t *testing.T) {
		// given
		nsTemplateTier := &toolchainv1alpha1.NSTemplateTier{
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
			},
		}
		nsTemplateSet := toolchainv1alpha1.NSTemplateSet{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: operatorNamespace,
				Name:      "foo",
			},
			Spec: toolchainv1alpha1.NSTemplateSetSpec{
				TierName: "basic",
				Namespaces: []toolchainv1alpha1.NSTemplateSetNamespace{
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
			},
		}
		// when
		hash1, err1 := nstemplatetier.ComputeHashForNSTemplateTier(nsTemplateTier)
		hash2, err2 := nstemplatetier.ComputeHashForNSTemplateSetSpec(nsTemplateSet.Spec)
		// then
		require.NoError(t, err1)
		require.NoError(t, err2)
		assert.Equal(t, hash1, hash2)
	})

	t.Run("should match with cluster resources", func(t *testing.T) {
		// given
		nsTemplateTier := &toolchainv1alpha1.NSTemplateTier{
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
					TemplateRef: "basic-clusterresources-123456a",
				},
			},
		}
		nsTemplateSet := toolchainv1alpha1.NSTemplateSet{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: operatorNamespace,
				Name:      "foo",
			},
			Spec: toolchainv1alpha1.NSTemplateSetSpec{
				TierName: "basic",
				Namespaces: []toolchainv1alpha1.NSTemplateSetNamespace{
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
				ClusterResources: &toolchainv1alpha1.NSTemplateSetClusterResources{
					TemplateRef: "basic-clusterresources-123456a",
				},
			},
		}
		// when
		hash1, err1 := nstemplatetier.ComputeHashForNSTemplateTier(nsTemplateTier)
		hash2, err2 := nstemplatetier.ComputeHashForNSTemplateSetSpec(nsTemplateSet.Spec)
		// then
		require.NoError(t, err1)
		require.NoError(t, err2)
		assert.Equal(t, hash1, hash2)
	})

	t.Run("should not match without cluster resources", func(t *testing.T) {
		// given
		nsTemplateTier := &toolchainv1alpha1.NSTemplateTier{
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
			},
		}
		nsTemplateSet := toolchainv1alpha1.NSTemplateSet{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: operatorNamespace,
				Name:      "foo",
			},
			Spec: toolchainv1alpha1.NSTemplateSetSpec{
				TierName: "basic",
				Namespaces: []toolchainv1alpha1.NSTemplateSetNamespace{
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
			},
		}
		// when
		hash1, err1 := nstemplatetier.ComputeHashForNSTemplateTier(nsTemplateTier)
		hash2, err2 := nstemplatetier.ComputeHashForNSTemplateSetSpec(nsTemplateSet.Spec)
		// then
		require.NoError(t, err1)
		require.NoError(t, err2)
		assert.NotEqual(t, hash1, hash2)
	})

	t.Run("should not match with cluster resources", func(t *testing.T) {
		// given
		nsTemplateTier := &toolchainv1alpha1.NSTemplateTier{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: operatorNamespace,
				Name:      "basic",
			},
			Spec: toolchainv1alpha1.NSTemplateTierSpec{
				Namespaces: []toolchainv1alpha1.NSTemplateTierNamespace{
					{
						TemplateRef: "basic-code-123456old", // same as `nsTemplateSet` var
					},
					{
						TemplateRef: "basic-dev-123456old",
					},
					{
						TemplateRef: "basic-stage-123456old",
					},
				},
				ClusterResources: &toolchainv1alpha1.NSTemplateTierClusterResources{
					TemplateRef: "basic-clusterresources-123456new",
				},
			},
		}
		nsTemplateSet := toolchainv1alpha1.NSTemplateSet{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: operatorNamespace,
				Name:      "foo",
			},
			Spec: toolchainv1alpha1.NSTemplateSetSpec{
				TierName: "basic",
				Namespaces: []toolchainv1alpha1.NSTemplateSetNamespace{
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
				ClusterResources: &toolchainv1alpha1.NSTemplateSetClusterResources{
					TemplateRef: "basic-clusterresources-123456a",
				},
			},
		}
		// when
		hash1, err1 := nstemplatetier.ComputeHashForNSTemplateTier(nsTemplateTier)
		hash2, err2 := nstemplatetier.ComputeHashForNSTemplateSetSpec(nsTemplateSet.Spec)
		// then
		require.NoError(t, err1)
		require.NoError(t, err2)
		assert.NotEqual(t, hash1, hash2)
	})

}
