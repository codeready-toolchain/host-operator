package deactivation

import (
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	. "github.com/codeready-toolchain/host-operator/test"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/operator-framework/operator-sdk/pkg/k8sutil"

	"k8s.io/apimachinery/pkg/types"
)

func TestUserSignupToMasterUserRecordMapper(t *testing.T) {
	t.Run("test UserSignupToMasterUserRecordMapper maps correctly", func(t *testing.T) {
		userSignup := &toolchainv1alpha1.UserSignup{
			ObjectMeta: NewUserSignupObjectMeta("", "foo@redhat.com"),
			Spec: toolchainv1alpha1.UserSignupSpec{
				Username: "foo@redhat.com",
			},
			Status: toolchainv1alpha1.UserSignupStatus{
				CompliantUsername: "foo",
			},
		}

		mapper := &UserSignupToMasterUserRecordMapper{}

		// This is required for the mapper to function
		restore := test.SetEnvVarAndRestore(t, k8sutil.WatchNamespaceEnvVar, test.HostOperatorNs)
		defer restore()

		req := mapper.Map(handler.MapObject{
			Object: userSignup,
		})

		require.Len(t, req, 1)
		require.Equal(t, types.NamespacedName{
			Namespace: userSignup.Namespace,
			Name:      "foo",
		}, req[0].NamespacedName)
	})

	t.Run("test UserSignup doesn't have compliant username", func(t *testing.T) {
		userSignup := &toolchainv1alpha1.UserSignup{
			ObjectMeta: NewUserSignupObjectMeta("", "bravo@redhat.com"),
			Spec: toolchainv1alpha1.UserSignupSpec{
				Username: "bravo@redhat.com",
			},
			Status: toolchainv1alpha1.UserSignupStatus{
				CompliantUsername: "",
			},
		}

		mapper := &UserSignupToMasterUserRecordMapper{}

		// This is required for the mapper to function
		restore := test.SetEnvVarAndRestore(t, k8sutil.WatchNamespaceEnvVar, test.HostOperatorNs)
		defer restore()

		req := mapper.Map(handler.MapObject{
			Object: userSignup,
		})

		require.Len(t, req, 0)
	})

}
