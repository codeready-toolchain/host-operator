package space

import (
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestNSTemplateSetToSpaceMapper(t *testing.T) {

	t.Run("success", func(t *testing.T) {
		// given
		nsTmplSet := &toolchainv1alpha1.NSTemplateSet{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: test.MemberOperatorNs,
				Name:      "foo",
			},
		}
		// when
		req := MapNSTemplateSetToSpace(test.HostOperatorNs)(nsTmplSet)

		// then
		require.Len(t, req, 1)
		require.Equal(t, types.NamespacedName{
			Namespace: test.HostOperatorNs,
			Name:      "foo",
		}, req[0].NamespacedName)
	})

	t.Run("failures", func(t *testing.T) {

		t.Run("wrong type", func(t *testing.T) {
			// given
			mur := &toolchainv1alpha1.MasterUserRecord{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "echo",
					Namespace:         test.HostOperatorNs,
					CreationTimestamp: metav1.Now(),
				},
				Spec: toolchainv1alpha1.MasterUserRecordSpec{
					UserID: "echo",
				},
				Status: toolchainv1alpha1.MasterUserRecordStatus{},
			}

			// when
			req := MapNSTemplateSetToSpace(test.HostOperatorNs)(mur)

			// then
			require.Len(t, req, 0)
		})
	})
}
