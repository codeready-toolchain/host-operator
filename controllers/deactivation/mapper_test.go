package deactivation

import (
	"context"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	commontest "github.com/codeready-toolchain/toolchain-common/pkg/test"
	commonsignup "github.com/codeready-toolchain/toolchain-common/pkg/test/usersignup"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestUserSignupToMasterUserRecordMapper(t *testing.T) {
	t.Run("test UserSignupToMasterUserRecordMapper maps correctly", func(t *testing.T) {
		// given
		userSignup := &toolchainv1alpha1.UserSignup{
			ObjectMeta: commonsignup.NewUserSignupObjectMeta("", "foo@redhat.com"),
			Spec: toolchainv1alpha1.UserSignupSpec{
				IdentityClaims: toolchainv1alpha1.IdentityClaimsEmbedded{
					PreferredUsername: "foo@redhat.com",
				},
			},
			Status: toolchainv1alpha1.UserSignupStatus{
				CompliantUsername: "foo",
			},
		}

		// when
		req := MapUserSignupToMasterUserRecord()(context.TODO(), userSignup)

		// then
		require.Len(t, req, 1)
		require.Equal(t, types.NamespacedName{
			Namespace: userSignup.Namespace,
			Name:      "foo",
		}, req[0].NamespacedName)
	})

	t.Run("test UserSignup doesn't have compliant username", func(t *testing.T) {
		// given
		userSignup := &toolchainv1alpha1.UserSignup{
			ObjectMeta: commonsignup.NewUserSignupObjectMeta("", "bravo@redhat.com"),
			Spec: toolchainv1alpha1.UserSignupSpec{
				IdentityClaims: toolchainv1alpha1.IdentityClaimsEmbedded{
					PreferredUsername: "bravo@redhat.com",
				},
			},
			Status: toolchainv1alpha1.UserSignupStatus{
				CompliantUsername: "",
			},
		}

		// when
		req := MapUserSignupToMasterUserRecord()(context.TODO(), userSignup)

		// then
		require.Empty(t, req)
	})

	t.Run("test non-UserSignup doesn't map", func(t *testing.T) {
		// given
		mur := &toolchainv1alpha1.MasterUserRecord{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "echo",
				Namespace:         commontest.HostOperatorNs,
				CreationTimestamp: metav1.Now(),
			},
			Spec: toolchainv1alpha1.MasterUserRecordSpec{
				PropagatedClaims: toolchainv1alpha1.PropagatedClaims{
					UserID: "echo",
				},
			},
			Status: toolchainv1alpha1.MasterUserRecordStatus{},
		}

		// when
		req := MapUserSignupToMasterUserRecord()(context.TODO(), mur)

		// then
		require.Empty(t, req)
	})

}
