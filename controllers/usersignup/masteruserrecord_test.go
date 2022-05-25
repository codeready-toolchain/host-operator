package usersignup

import (
	"fmt"
	"testing"

	commonmur "github.com/codeready-toolchain/toolchain-common/pkg/test/masteruserrecord"
	commonsignup "github.com/codeready-toolchain/toolchain-common/pkg/test/usersignup"

	testusertier "github.com/codeready-toolchain/host-operator/test/usertier"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"

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
		commonmur.TierName("deactivate90"),
		commonmur.UserID("UserID123"),
		commonmur.WithAnnotation("toolchain.dev.openshift.com/user-email", "foo@redhat.com"))
	assert.Equal(t, expectedMUR, mur)
}

type userTierMigrationCase struct {
	originalTierName string
	expectedTierName string
}

func TestMigrateMurIfNecessary(t *testing.T) {

	t.Run("no update needed", func(t *testing.T) {

		t.Run("when mur is the same", func(t *testing.T) {
			// given
			userSignup := commonsignup.NewUserSignup()
			defaultUserTier := testusertier.NewUserTier("deactivate90", 90)
			mur := newMasterUserRecord(userSignup, test.MemberClusterName, "deactivate90", "johny")

			// when
			changed := migrateOrFixMurIfNecessary(mur, defaultUserTier, userSignup)

			// then
			assert.False(t, changed)
			expectedMUR := commonmur.NewMasterUserRecord(t, "johny",
				commonmur.WithOwnerLabel(userSignup.Name),
				commonmur.TierName("deactivate90"),
				commonmur.UserID("UserID123"),
				commonmur.WithAnnotation("toolchain.dev.openshift.com/user-email", "foo@redhat.com"))
			assert.Equal(t, expectedMUR, mur)
		})
	})

	t.Run("update needed", func(t *testing.T) {

		t.Run("when tierName is missing", func(t *testing.T) {
			userSignup := commonsignup.NewUserSignup()
			defaultUserTier := testusertier.NewUserTier("deactivate90", 90)
			mur := newMasterUserRecord(userSignup, test.MemberClusterName, "", "johny") // tierName not set

			// when
			changed := migrateOrFixMurIfNecessary(mur, defaultUserTier, userSignup)

			// then
			assert.True(t, changed)
			expectedMUR := commonmur.NewMasterUserRecord(t, "johny",
				commonmur.WithOwnerLabel(userSignup.Name),
				commonmur.TierName("deactivate90"),
				commonmur.UserID("UserID123"),
				commonmur.WithAnnotation("toolchain.dev.openshift.com/user-email", "foo@redhat.com"))
			assert.Equal(t, expectedMUR, mur)
		})

		t.Run("UserTier migration tests", func(t *testing.T) {

			testcases := []userTierMigrationCase{
				{
					originalTierName: "advanced",
					expectedTierName: "nodeactivation",
				},
				{
					originalTierName: "appstudio",
					expectedTierName: "deactivate30",
				},
				{
					originalTierName: "base",
					expectedTierName: "deactivate30",
				},
				{
					originalTierName: "base1ns",
					expectedTierName: "deactivate30",
				},
				{
					originalTierName: "basedeactivationdisabled",
					expectedTierName: "nodeactivation",
				},
				{
					originalTierName: "baseextended",
					expectedTierName: "deactivate180",
				},
				{
					originalTierName: "baseextendedidling",
					expectedTierName: "deactivate30",
				},
				{
					originalTierName: "baselarge",
					expectedTierName: "deactivate90",
				},
				{
					originalTierName: "hackathon",
					expectedTierName: "deactivate80",
				},
				{
					originalTierName: "test",
					expectedTierName: "deactivate30",
				},
			}

			for _, tc := range testcases {
				t.Run(fmt.Sprintf("when tierName is %s", tc.originalTierName), func(t *testing.T) {
					userSignup := commonsignup.NewUserSignup()
					defaultUserTier := testusertier.NewUserTier("deactivatetest", 30)
					mur := newMasterUserRecord(userSignup, test.MemberClusterName, tc.originalTierName, "johny")

					// when
					changed := migrateOrFixMurIfNecessary(mur, defaultUserTier, userSignup)

					// then
					assert.True(t, changed)
					expectedMUR := commonmur.NewMasterUserRecord(t, "johny",
						commonmur.WithOwnerLabel(userSignup.Name),
						commonmur.TierName(tc.expectedTierName),
						commonmur.UserID("UserID123"),
						commonmur.WithAnnotation("toolchain.dev.openshift.com/user-email", "foo@redhat.com"))
					assert.Equal(t, expectedMUR, mur)
				})
			}
		})
	})

}
