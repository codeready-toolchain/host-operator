package usersignup

import (
	"context"
	"errors"
	"testing"

	. "github.com/codeready-toolchain/host-operator/test"
	"github.com/operator-framework/operator-sdk/pkg/k8sutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	"github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
		userSignup := &v1alpha1.UserSignup{
			ObjectMeta: NewUserSignupObjectMeta("", "foo@redhat.com"),
			Spec: v1alpha1.UserSignupSpec{
				Username: "foo@redhat.com",
			},
		}

		userSignup2 := &v1alpha1.UserSignup{
			ObjectMeta: NewUserSignupObjectMeta("", "alice.mayweather.doe@redhat.com"),
			Spec: v1alpha1.UserSignupSpec{
				Username: "alice.mayweather.doe@redhat.com",
			},
		}

		c := test.NewFakeClient(t, userSignup, userSignup2)

		mapper := &BannedUserToUserSignupMapper{
			client: c,
		}

		// This is required for the mapper to function
		restore := test.SetEnvVarAndRestore(t, k8sutil.WatchNamespaceEnvVar, test.HostOperatorNs)
		defer restore()

		req := mapper.Map(handler.MapObject{
			Object: bannedUser,
		})

		require.Len(t, req, 1)
		require.Equal(t, types.NamespacedName{
			Namespace: userSignup.Namespace,
			Name:      userSignup.Name,
		}, req[0].NamespacedName)
	})

	t.Run("test BannedUserToUserSignupMapper returns nil when Client list fails", func(t *testing.T) {
		c := test.NewFakeClient(t)
		c.MockList = func(ctx context.Context, list runtime.Object, opts ...client.ListOption) error {
			return errors.New("err happened")
		}

		mapper := &BannedUserToUserSignupMapper{
			client: c,
		}
		req := mapper.Map(handler.MapObject{
			Object: bannedUser,
		})

		require.Nil(t, req)
	})

	t.Run("test BannedUserToUserSignupMapper returns nil when watch namespace not set ", func(t *testing.T) {
		c := test.NewFakeClient(t)

		mapper := &BannedUserToUserSignupMapper{
			client: c,
		}
		req := mapper.Map(handler.MapObject{
			Object: bannedUser,
		})

		require.Nil(t, req)
	})
}
