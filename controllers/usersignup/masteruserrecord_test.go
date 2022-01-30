package usersignup

import (
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	. "github.com/codeready-toolchain/host-operator/test"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNewMasterUserRecord(t *testing.T) {
	// given
	userSignup := NewUserSignup()
	nsTemplateTier := newNsTemplateTier("advanced", "dev", "stage", "extra")

	// when
	mur := newMasterUserRecord(userSignup, test.MemberClusterName, nsTemplateTier, "johny")

	// then
	assert.Equal(t, newExpectedMur(nsTemplateTier, userSignup), mur)
}

func TestMigrateMurIfNecessary(t *testing.T) {

	t.Run("no update needed", func(t *testing.T) {

		t.Run("when mur is the same", func(t *testing.T) {
			// given
			userSignup := NewUserSignup()
			nsTemplateTier := newNsTemplateTier("advanced", "dev", "stage", "extra")
			mur := newMasterUserRecord(userSignup, test.MemberClusterName, nsTemplateTier, "johny")

			// when
			changed, err := migrateOrFixMurIfNecessary(mur, nsTemplateTier, userSignup)

			// then
			require.NoError(t, err)
			assert.False(t, changed)
			assert.Equal(t, newExpectedMur(nsTemplateTier, userSignup), mur)
		})
	})

	t.Run("update needed", func(t *testing.T) {

		t.Run("when tierName is missing", func(t *testing.T) {
			userSignup := NewUserSignup()
			nsTemplateTier := newNsTemplateTier("advanced", "dev", "stage", "extra")
			mur := newMasterUserRecord(userSignup, test.MemberClusterName, nsTemplateTier, "johny")
			mur.Spec.TierName = ""

			// when
			changed, err := migrateOrFixMurIfNecessary(mur, nsTemplateTier, userSignup)

			// then
			require.NoError(t, err)
			assert.True(t, changed)
			assert.Equal(t, newExpectedMur(nsTemplateTier, userSignup), mur)
		})
	})

}

func newExpectedMur(tier *toolchainv1alpha1.NSTemplateTier, userSignup *toolchainv1alpha1.UserSignup) *toolchainv1alpha1.MasterUserRecord {
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
			UserID:        userSignup.Spec.Userid,
			Banned:        false,
			Disabled:      false,
			Deprovisioned: false,
			TierName:      "advanced",
			UserAccounts: []toolchainv1alpha1.UserAccountEmbedded{
				{
					SyncIndex:     "",
					TargetCluster: test.MemberClusterName,
				},
			},
		},
	}
}
