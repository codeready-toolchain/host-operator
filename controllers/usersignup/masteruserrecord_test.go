package usersignup

import (
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	tierutil "github.com/codeready-toolchain/host-operator/controllers/nstemplatetier/util"
	. "github.com/codeready-toolchain/host-operator/test"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	murtest "github.com/codeready-toolchain/toolchain-common/pkg/test/masteruserrecord"

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

		t.Run("when useraccount NSLimit was set, it should be empty after migration", func(t *testing.T) {
			userSignup := NewUserSignup()
			nsTemplateTier := newNsTemplateTier("advanced", "dev", "stage", "extra")
			mur := newMasterUserRecord(userSignup, test.MemberClusterName, nsTemplateTier, "johny")
			mur.Spec.UserAccounts[0].Spec.NSLimit = "default" // NSLimit is set

			// when
			changed, err := migrateOrFixMurIfNecessary(mur, nsTemplateTier, userSignup)

			// then
			require.NoError(t, err)
			assert.True(t, changed)
			assert.Equal(t, newExpectedMur(nsTemplateTier, userSignup), mur)
		})

		t.Run("when useraccount NSTemplateSet was set, it should be nil after migration", func(t *testing.T) {
			userSignup := NewUserSignup()
			nsTemplateTier := newNsTemplateTier("advanced", "dev", "stage", "extra")
			testNStemplateSet := murtest.DefaultNSTemplateSet()
			mur := newMasterUserRecord(userSignup, test.MemberClusterName, nsTemplateTier, "johny")
			mur.Spec.UserAccounts[0].Spec.NSTemplateSet = &testNStemplateSet.Spec // NSTemplateSet is set

			// when
			changed, err := migrateOrFixMurIfNecessary(mur, nsTemplateTier, userSignup)

			// then
			require.NoError(t, err)
			assert.True(t, changed)
			assert.Equal(t, newExpectedMur(nsTemplateTier, userSignup), mur)
		})

		t.Run("when MUR has tier hash label, it should be removed after migration", func(t *testing.T) {
			userSignup := NewUserSignup()
			nsTemplateTier := newNsTemplateTier("advanced", "dev", "stage", "extra")
			testNStemplateSet := murtest.DefaultNSTemplateSet()
			mur := newMasterUserRecord(userSignup, test.MemberClusterName, nsTemplateTier, "johny")
			mur.Labels = map[string]string{
				"toolchain.dev.openshift.com/owner":                       userSignup.Name,
				tierutil.TemplateTierHashLabelKey(testNStemplateSet.Name): "abc123", // tier hash label set
			}

			// when
			changed, err := migrateOrFixMurIfNecessary(mur, nsTemplateTier, userSignup)

			// then
			require.NoError(t, err)
			assert.True(t, changed)
			assert.Equal(t, newExpectedMur(nsTemplateTier, userSignup), mur)
		})

		t.Run("when tierName is missing", func(t *testing.T) {
			userSignup := NewUserSignup()
			nsTemplateTier := newNsTemplateTier("advanced", "dev", "stage", "extra")
			mur := newMasterUserRecord(userSignup, test.MemberClusterName, nsTemplateTier, "johny")
			mur.Spec.TierName = "" // tierName not set

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
