package usersignup

import (
	"context"
	"errors"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/configuration"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	commonsignup "github.com/codeready-toolchain/toolchain-common/pkg/test/usersignup"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func TestBannedUserToUserSignupMapper(t *testing.T) {
	// when
	bannedUser := &toolchainv1alpha1.BannedUser{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				toolchainv1alpha1.BannedUserEmailHashLabelKey: "fd2addbd8d82f0d2dc088fa122377eaa",
			},
		},
		Spec: toolchainv1alpha1.BannedUserSpec{
			Email: "foo@redhat.com",
		},
	}

	t.Run("test BannedUserToUserSignupMapper maps correctly", func(t *testing.T) {
		userSignup := &toolchainv1alpha1.UserSignup{
			ObjectMeta: commonsignup.NewUserSignupObjectMeta("", "foo@redhat.com"),
			Spec: toolchainv1alpha1.UserSignupSpec{
				IdentityClaims: toolchainv1alpha1.IdentityClaimsEmbedded{
					PropagatedClaims:  toolchainv1alpha1.PropagatedClaims{},
					PreferredUsername: "foo@redhat.com",
				},
			},
		}

		userSignup2 := &toolchainv1alpha1.UserSignup{
			ObjectMeta: commonsignup.NewUserSignupObjectMeta("", "alice.mayweather.doe@redhat.com"),
			Spec: toolchainv1alpha1.UserSignupSpec{
				IdentityClaims: toolchainv1alpha1.IdentityClaimsEmbedded{
					PropagatedClaims:  toolchainv1alpha1.PropagatedClaims{},
					PreferredUsername: "alice.mayweather.doe@redhat.com",
				},
			},
		}

		c := test.NewFakeClient(t, userSignup, userSignup2)

		// This is required for the mapper to function
		restore := test.SetEnvVarAndRestore(t, configuration.WatchNamespaceEnvVar, test.HostOperatorNs)
		defer restore()

		// when
		req := MapBannedUserToUserSignup(c)(context.TODO(), bannedUser)

		// then
		require.Len(t, req, 1)
		require.Equal(t, types.NamespacedName{
			Namespace: userSignup.Namespace,
			Name:      userSignup.Name,
		}, req[0].NamespacedName)
	})

	t.Run("test BannedUserToUserSignupMapper returns nil when client list fails", func(t *testing.T) {
		c := test.NewFakeClient(t)
		c.MockList = func(ctx context.Context, list runtimeclient.ObjectList, opts ...runtimeclient.ListOption) error {
			return errors.New("err happened")
		}

		// when
		req := MapBannedUserToUserSignup(c)(context.TODO(), bannedUser)

		// then
		require.Nil(t, req)
	})

	t.Run("test BannedUserToUserSignupMapper returns nil when watch namespace not set ", func(t *testing.T) {
		c := test.NewFakeClient(t)
		restore := test.UnsetEnvVarAndRestore(t, "WATCH_NAMESPACE")
		t.Cleanup(restore)

		// when
		req := MapBannedUserToUserSignup(c)(context.TODO(), bannedUser)

		// then
		require.Nil(t, req)
	})
}
