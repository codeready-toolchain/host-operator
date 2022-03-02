package space

import (
	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"testing"
)

func TestNSTemplateSetToSpaceMapper(t *testing.T) {
	t.Run("test MapNSTemplateSetToSpace maps correctly", func(t *testing.T) {
		// given
		NSTemplateSet := &toolchainv1alpha1.NSTemplateSet{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "member-operator",
				Name:      "foo",
			},
		}
		// when
		req := MapNSTemplateSetToSpace("host-operator")(NSTemplateSet)

		// then
		require.Len(t, req, 1)
		require.Equal(t, types.NamespacedName{
			Namespace: "host-operator",
			Name:      "foo",
		}, req[0].NamespacedName)
	})

	t.Run("test non-NSTemplateSet doesn't map", func(t *testing.T) {
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
		req := MapNSTemplateSetToSpace("host-operator")(mur)

		// then
		require.Len(t, req, 0)
	})
}
