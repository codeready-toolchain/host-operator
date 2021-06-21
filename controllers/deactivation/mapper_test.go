package deactivation

import (
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

		mur := &toolchainv1alpha1.MasterUserRecord{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo",
				Namespace: userSignup.Namespace,
			},
			Spec:   toolchainv1alpha1.MasterUserRecordSpec{},
			Status: toolchainv1alpha1.MasterUserRecordStatus{},
		}

		userSignup2 := &toolchainv1alpha1.UserSignup{
			ObjectMeta: NewUserSignupObjectMeta("", "alice.mayweather.doe@redhat.com"),
			Spec: toolchainv1alpha1.UserSignupSpec{
				Username: "alice.mayweather.doe@redhat.com",
			},
		}

		c := test.NewFakeClient(t, userSignup, userSignup2, mur)

		mapper := &UserSignupToMasterUserRecordMapper{
			client: c,
		}

		// This is required for the mapper to function
		restore := test.SetEnvVarAndRestore(t, k8sutil.WatchNamespaceEnvVar, test.HostOperatorNs)
		defer restore()

		req := mapper.Map(handler.MapObject{
			Object: userSignup,
		})

		require.Len(t, req, 1)
		require.Equal(t, types.NamespacedName{
			Namespace: mur.Namespace,
			Name:      mur.Name,
		}, req[0].NamespacedName)
	})
}
