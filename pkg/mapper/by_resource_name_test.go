package mapper

import (
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestMapNSTemplateSet(t *testing.T) {
	// given
	NSTemplateSet := &toolchainv1alpha1.NSTemplateSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "member-operator",
			Name:      "foo",
		},
	}
	// when
	req := MapByResourceName("host-operator")(NSTemplateSet)

	// then
	require.Len(t, req, 1)
	require.Equal(t, types.NamespacedName{
		Namespace: "host-operator",
		Name:      "foo",
	}, req[0].NamespacedName)
}

func TestMapUserAccount(t *testing.T) {
	// given
	userAccount := &toolchainv1alpha1.UserAccount{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "member-operator",
			Name:      "foo",
		},
	}
	// when
	req := MapByResourceName("host-operator")(userAccount)

	// then
	require.Len(t, req, 1)
	require.Equal(t, types.NamespacedName{
		Namespace: "host-operator",
		Name:      "foo",
	}, req[0].NamespacedName)
}
