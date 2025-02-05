package nstemplatetier_test

import (
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/hash"

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
				Name:      "base1ns",
			},
			Spec: toolchainv1alpha1.NSTemplateTierSpec{
				Namespaces: []toolchainv1alpha1.NSTemplateTierNamespace{
					{
						TemplateRef: "base1ns-code-123456old",
					},
					{
						TemplateRef: "base1ns-dev-123456old",
					},
					{
						TemplateRef: "base1ns-stage-123456old",
					},
				},
			},
			Status: toolchainv1alpha1.NSTemplateTierStatus{
				Revisions: map[string]string{
					"base1ns-code-123456old":  "base1ns-code-123456old",
					"base1ns-dev-123456old":   "base1ns-dev-123456old",
					"base1ns-stage-123456old": "base1ns-stage-123456old",
				},
			},
		}
		nsTemplateSet := toolchainv1alpha1.NSTemplateSet{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: operatorNamespace,
				Name:      "foo",
			},
			Spec: toolchainv1alpha1.NSTemplateSetSpec{
				TierName: "base1ns",
				Namespaces: []toolchainv1alpha1.NSTemplateSetNamespace{
					{
						TemplateRef: "base1ns-code-123456old",
					},
					{
						TemplateRef: "base1ns-dev-123456old",
					},
					{
						TemplateRef: "base1ns-stage-123456old",
					},
				},
			},
		}
		// when
		hash1, err1 := hash.ComputeHashForNSTemplateTier(nsTemplateTier)
		hash2, err2 := hash.ComputeHashForNSTemplateSetSpec(nsTemplateSet.Spec)
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
				Name:      "base1ns",
			},
			Spec: toolchainv1alpha1.NSTemplateTierSpec{
				Namespaces: []toolchainv1alpha1.NSTemplateTierNamespace{
					{
						TemplateRef: "base1ns-code-123456old",
					},
					{
						TemplateRef: "base1ns-dev-123456old",
					},
					{
						TemplateRef: "base1ns-stage-123456old",
					},
				},
				ClusterResources: &toolchainv1alpha1.NSTemplateTierClusterResources{
					TemplateRef: "base1ns-clusterresources-123456a",
				},
			},
			Status: toolchainv1alpha1.NSTemplateTierStatus{
				Revisions: map[string]string{
					"base1ns-code-123456old":           "base1ns-code-123456old",
					"base1ns-dev-123456old":            "base1ns-dev-123456old",
					"base1ns-stage-123456old":          "base1ns-stage-123456old",
					"base1ns-clusterresources-123456a": "base1ns-clusterresources-123456a",
				},
			},
		}
		nsTemplateSet := toolchainv1alpha1.NSTemplateSet{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: operatorNamespace,
				Name:      "foo",
			},
			Spec: toolchainv1alpha1.NSTemplateSetSpec{
				TierName: "base1ns",
				Namespaces: []toolchainv1alpha1.NSTemplateSetNamespace{
					{
						TemplateRef: "base1ns-code-123456old",
					},
					{
						TemplateRef: "base1ns-dev-123456old",
					},
					{
						TemplateRef: "base1ns-stage-123456old",
					},
				},
				ClusterResources: &toolchainv1alpha1.NSTemplateSetClusterResources{
					TemplateRef: "base1ns-clusterresources-123456a",
				},
			},
		}
		// when
		hash1, err1 := hash.ComputeHashForNSTemplateTier(nsTemplateTier)
		hash2, err2 := hash.ComputeHashForNSTemplateSetSpec(nsTemplateSet.Spec)
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
				Name:      "base1ns",
			},
			Spec: toolchainv1alpha1.NSTemplateTierSpec{
				Namespaces: []toolchainv1alpha1.NSTemplateTierNamespace{
					{
						TemplateRef: "base1ns-code-123456new",
					},
					{
						TemplateRef: "base1ns-dev-123456new",
					},
					{
						TemplateRef: "base1ns-stage-123456new",
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
				TierName: "base1ns",
				Namespaces: []toolchainv1alpha1.NSTemplateSetNamespace{
					{
						TemplateRef: "base1ns-code-123456old",
					},
					{
						TemplateRef: "base1ns-dev-123456old",
					},
					{
						TemplateRef: "base1ns-stage-123456old",
					},
				},
			},
		}
		// when
		hash1, err1 := hash.ComputeHashForNSTemplateTier(nsTemplateTier)
		hash2, err2 := hash.ComputeHashForNSTemplateSetSpec(nsTemplateSet.Spec)
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
				Name:      "base1ns",
			},
			Spec: toolchainv1alpha1.NSTemplateTierSpec{
				Namespaces: []toolchainv1alpha1.NSTemplateTierNamespace{
					{
						TemplateRef: "base1ns-code-123456old", // same as `nsTemplateSet` var
					},
					{
						TemplateRef: "base1ns-dev-123456old",
					},
					{
						TemplateRef: "base1ns-stage-123456old",
					},
				},
				ClusterResources: &toolchainv1alpha1.NSTemplateTierClusterResources{
					TemplateRef: "base1ns-clusterresources-123456new",
				},
			},
		}
		nsTemplateSet := toolchainv1alpha1.NSTemplateSet{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: operatorNamespace,
				Name:      "foo",
			},
			Spec: toolchainv1alpha1.NSTemplateSetSpec{
				TierName: "base1ns",
				Namespaces: []toolchainv1alpha1.NSTemplateSetNamespace{
					{
						TemplateRef: "base1ns-code-123456old",
					},
					{
						TemplateRef: "base1ns-dev-123456old",
					},
					{
						TemplateRef: "base1ns-stage-123456old",
					},
				},
				ClusterResources: &toolchainv1alpha1.NSTemplateSetClusterResources{
					TemplateRef: "base1ns-clusterresources-123456a",
				},
			},
		}
		// when
		hash1, err1 := hash.ComputeHashForNSTemplateTier(nsTemplateTier)
		hash2, err2 := hash.ComputeHashForNSTemplateSetSpec(nsTemplateSet.Spec)
		// then
		require.NoError(t, err1)
		require.NoError(t, err2)
		assert.NotEqual(t, hash1, hash2)
	})

}
