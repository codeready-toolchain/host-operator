package usersignup

import (
	"testing"

	commonsignup "github.com/codeready-toolchain/toolchain-common/pkg/test/usersignup"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	testusertier "github.com/codeready-toolchain/host-operator/test/usertier"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNewMasterUserRecord(t *testing.T) {
	// given
	userSignup := commonsignup.NewUserSignup()
	userTier := testusertier.NewUserTier("deactivate90", 90)

	// when
	mur := newMasterUserRecord(userSignup, test.MemberClusterName, userTier, "johny")

	// then
	assert.Equal(t, newExpectedMur(userSignup), mur)
}

func TestMigrateMurIfNecessary(t *testing.T) {

	t.Run("no update needed", func(t *testing.T) {

		t.Run("when mur is the same", func(t *testing.T) {
			// given
			userSignup := commonsignup.NewUserSignup()
			userTier := testusertier.NewUserTier("deactivate90", 90)
			mur := newMasterUserRecord(userSignup, test.MemberClusterName, userTier, "johny")

			// when
			changed := migrateOrFixMurIfNecessary(mur, userTier, userSignup)

			// then
			assert.False(t, changed)
			assert.Equal(t, newExpectedMur(userSignup), mur)
		})
	})

	t.Run("update needed", func(t *testing.T) {

		t.Run("when tierName is missing", func(t *testing.T) {
			userSignup := commonsignup.NewUserSignup()
			userTier := testusertier.NewUserTier("deactivate90", 90)
			mur := newMasterUserRecord(userSignup, test.MemberClusterName, userTier, "johny")
			mur.Spec.TierName = "" // tierName not set

			// when
			changed := migrateOrFixMurIfNecessary(mur, userTier, userSignup)

			// then
			assert.True(t, changed)
			assert.Equal(t, newExpectedMur(userSignup), mur)
		})
	})

}

func newExpectedMur(userSignup *toolchainv1alpha1.UserSignup) *toolchainv1alpha1.MasterUserRecord {
	return &toolchainv1alpha1.MasterUserRecord{
		ObjectMeta: v1.ObjectMeta{
			Name:      "johny",
			Namespace: test.HostOperatorNs,
			Labels: map[string]string{
				"toolchain.dev.openshift.com/owner": userSignup.Name,
			},
			Annotations: map[string]string{
				"toolchain.dev.openshift.com/user-email": "foo@redhat.com",
			},
		},
		Spec: toolchainv1alpha1.MasterUserRecordSpec{
			UserID:   userSignup.Spec.Userid,
			Disabled: false,
			TierName: "deactivate90",
			UserAccounts: []toolchainv1alpha1.UserAccountEmbedded{
				{
					TargetCluster: test.MemberClusterName,
				},
			},
		},
	}
}
