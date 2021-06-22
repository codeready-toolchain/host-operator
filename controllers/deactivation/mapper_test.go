package deactivation

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	. "github.com/codeready-toolchain/host-operator/test"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/operator-framework/operator-sdk/pkg/k8sutil"

	"k8s.io/apimachinery/pkg/types"
)

func TestUserSignupToMasterUserRecordMapper(t *testing.T) {
	// This is required for the mapper to function
	restore := test.SetEnvVarAndRestore(t, k8sutil.WatchNamespaceEnvVar, test.HostOperatorNs)
	defer restore()

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

		req := mapper.Map(handler.MapObject{
			Object: userSignup,
		})

		require.Len(t, req, 0)
	})

	t.Run("test non-UserSignup doesn't map", func(t *testing.T) {

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

		mapper := &UserSignupToMasterUserRecordMapper{}
		req := mapper.Map(handler.MapObject{
			Object: mur,
		})

		require.Len(t, req, 0)
	})

}
