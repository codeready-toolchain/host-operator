package usersignup

import (
	commontier "github.com/codeready-toolchain/toolchain-common/pkg/test/tier"
	"testing"

	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	commonmur "github.com/codeready-toolchain/toolchain-common/pkg/test/masteruserrecord"
	commonsignup "github.com/codeready-toolchain/toolchain-common/pkg/test/usersignup"

	"github.com/stretchr/testify/assert"
)

func TestNewMasterUserRecord(t *testing.T) {
	// given
	userSignup := commonsignup.NewUserSignup()

	// when
	mur := newMasterUserRecord(userSignup, test.MemberClusterName, "deactivate90", "johny")

	// then
	expectedMUR := commonmur.NewMasterUserRecord(t, "johny",
		commonmur.WithOwnerLabel(userSignup.Name),
		commonmur.TierName("deactivate90"))

	expectedMUR.Spec.PropagatedClaims = userSignup.Spec.IdentityClaims.PropagatedClaims

	assert.Equal(t, expectedMUR, mur)
}

func TestNewMasterUserRecordWhenSpaceCreationIsSkipped(t *testing.T) {
	// given
	userSignup := commonsignup.NewUserSignup(commonsignup.WithAnnotation("toolchain.dev.openshift.com/skip-auto-create-space", "true"))

	// when
	mur := newMasterUserRecord(userSignup, test.MemberClusterName, "deactivate90", "johny")

	// then
	expectedMUR := commonmur.NewMasterUserRecord(t, "johny",
		commonmur.WithOwnerLabel(userSignup.Name),
		commonmur.TierName("deactivate90"),
		commonmur.UserID("UserID123"),
		commonmur.WithAnnotation("toolchain.dev.openshift.com/skip-auto-create-space", "true"))

	expectedMUR.Spec.PropagatedClaims = userSignup.Spec.IdentityClaims.PropagatedClaims

	assert.Equal(t, expectedMUR, mur)
}

func TestMigrateMurIfNecessary(t *testing.T) {
	defaultUserTier := commontier.NewUserTier(commontier.WithName("deactivate90"), commontier.WithDeactivationTimeoutDays(90))

	t.Run("no update needed", func(t *testing.T) {

		t.Run("when mur is the same", func(t *testing.T) {
			// given
			userSignup := commonsignup.NewUserSignup()
			mur := newMasterUserRecord(userSignup, test.MemberClusterName, "deactivate90", "johny")

			// when
			changed := migrateOrFixMurIfNecessary(mur, defaultUserTier, userSignup)

			// then
			assert.False(t, changed)
			expectedMUR := commonmur.NewMasterUserRecord(t, "johny",
				commonmur.WithOwnerLabel(userSignup.Name),
				commonmur.TierName("deactivate90"))
			expectedMUR.Spec.PropagatedClaims = userSignup.Spec.IdentityClaims.PropagatedClaims
			assert.Equal(t, expectedMUR, mur)
		})
	})

	t.Run("update needed", func(t *testing.T) {

		t.Run("when tierName is missing", func(t *testing.T) {
			userSignup := commonsignup.NewUserSignup()
			mur := newMasterUserRecord(userSignup, test.MemberClusterName, "", "johny") // tierName not set

			// when
			changed := migrateOrFixMurIfNecessary(mur, defaultUserTier, userSignup)

			// then
			assert.True(t, changed)
			expectedMUR := commonmur.NewMasterUserRecord(t, "johny",
				commonmur.WithOwnerLabel(userSignup.Name),
				commonmur.TierName("deactivate90"))
			expectedMUR.Spec.PropagatedClaims = userSignup.Spec.IdentityClaims.PropagatedClaims
			assert.Equal(t, expectedMUR, mur)
		})

	})

}
