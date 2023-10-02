package masteruserrecord

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	commonsignup "github.com/codeready-toolchain/toolchain-common/pkg/test/usersignup"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/cluster"
	. "github.com/codeready-toolchain/host-operator/test"
	. "github.com/codeready-toolchain/host-operator/test/notification"
	testusertier "github.com/codeready-toolchain/host-operator/test/usertier"
	commoncluster "github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	murtest "github.com/codeready-toolchain/toolchain-common/pkg/test/masteruserrecord"
	uatest "github.com/codeready-toolchain/toolchain-common/pkg/test/useraccount"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var readyToolchainStatus = NewToolchainStatus(
	WithMember(test.MemberClusterName, WithRoutes("https://console.member-cluster/", "http://che-toolchain-che.member-cluster/", ToBeReady())))

func TestIsSynchronized(t *testing.T) {

	t.Run("synchronized", func(t *testing.T) {
		// given
		record, memberUserAcc := setupSynchronizerItems()
		s := Synchronizer{
			memberUserAcc: &memberUserAcc,
			record:        &record,
		}
		// when/then
		assert.True(t, s.isSynchronized())
	})

	t.Run("not synchronized", func(t *testing.T) {

		t.Run("missing label", func(t *testing.T) {
			// given
			record, memberUserAcc := setupSynchronizerItems()
			delete(memberUserAcc.Labels, toolchainv1alpha1.TierLabelKey)
			s := Synchronizer{
				memberUserAcc: &memberUserAcc,
				record:        &record,
			}
			// when/then
			assert.False(t, s.isSynchronized())
		})

		t.Run("label does not match", func(t *testing.T) {
			// given
			record, memberUserAcc := setupSynchronizerItems()
			memberUserAcc.Labels[toolchainv1alpha1.TierLabelKey] = "foo"
			s := Synchronizer{
				memberUserAcc: &memberUserAcc,
				record:        &record,
			}
			// when/then
			assert.False(t, s.isSynchronized())
		})

		t.Run("missing email annotation", func(t *testing.T) {
			// given
			record, memberUserAcc := setupSynchronizerItems()
			delete(memberUserAcc.Annotations, toolchainv1alpha1.UserEmailAnnotationKey)
			s := Synchronizer{
				memberUserAcc: &memberUserAcc,
				record:        &record,
			}
			// when/then
			assert.False(t, s.isSynchronized())
		})

		t.Run("email annotation does not match", func(t *testing.T) {
			// given
			record, memberUserAcc := setupSynchronizerItems()
			memberUserAcc.Annotations[toolchainv1alpha1.UserEmailAnnotationKey] = "foo"
			s := Synchronizer{
				memberUserAcc: &memberUserAcc,
				record:        &record,
			}
			// when/then
			assert.False(t, s.isSynchronized())
		})

		t.Run("different disable", func(t *testing.T) {
			// given
			record, memberUserAcc := setupSynchronizerItems()
			record.Spec.Disabled = true
			s := Synchronizer{
				memberUserAcc: &memberUserAcc,
				record:        &record,
			}
			// when/then
			assert.False(t, s.isSynchronized())
		})

		t.Run("different userID", func(t *testing.T) {
			// given
			record, memberUserAcc := setupSynchronizerItems()
			record.Spec.UserID = "bar"
			s := Synchronizer{
				memberUserAcc: &memberUserAcc,
				record:        &record,
			}
			// when/then
			assert.False(t, s.isSynchronized())
		})
	})
}

func setupSynchronizerItems() (toolchainv1alpha1.MasterUserRecord, toolchainv1alpha1.UserAccount) {
	memberUserAcc := toolchainv1alpha1.UserAccount{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				toolchainv1alpha1.TierLabelKey: "base1ns",
			},
			Annotations: map[string]string{
				toolchainv1alpha1.UserEmailAnnotationKey: "foo@bar.com",
			},
		},
		Spec: toolchainv1alpha1.UserAccountSpec{
			UserID:   "foo",
			Disabled: false,
		},
	}
	record := toolchainv1alpha1.MasterUserRecord{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				toolchainv1alpha1.UserEmailAnnotationKey: "foo@bar.com",
			},
		},
		Spec: toolchainv1alpha1.MasterUserRecordSpec{
			UserID:   "foo",
			Disabled: false,
			TierName: "base1ns",
		},
	}
	return record, memberUserAcc
}

func TestSynchronizeSpec(t *testing.T) {
	// given
	apiScheme(t)
	otherTier := testusertier.NewUserTier("other", 90)
	mur := murtest.NewMasterUserRecord(t, "john", murtest.StatusCondition(toBeProvisioned()), murtest.TierName(otherTier.Name))

	userAccount := uatest.NewUserAccountFromMur(mur)

	err := murtest.Modify(mur, murtest.UserID("abc123"))
	require.NoError(t, err)

	hostClient := test.NewFakeClient(t, mur)
	sync, memberClient := prepareSynchronizer(t, userAccount, mur, hostClient)

	// when
	createdOrUpdated, err := sync.synchronizeSpec()

	// then
	require.NoError(t, err)
	assert.True(t, createdOrUpdated)
	uatest.AssertThatUserAccount(t, "john", memberClient).
		Exists().
		MatchMasterUserRecord(mur)

	murtest.AssertThatMasterUserRecord(t, "john", hostClient).
		HasTier(*otherTier).
		HasConditions(toBeNotReady(toolchainv1alpha1.MasterUserRecordUpdatingReason, ""))
}

func TestSynchronizeAnnotation(t *testing.T) {
	// given
	apiScheme(t)
	mur := murtest.NewMasterUserRecord(t, "john", murtest.StatusCondition(toBeProvisioned()))

	userAccount := uatest.NewUserAccountFromMur(mur)
	userAccount.Annotations = nil

	hostClient := test.NewFakeClient(t, mur)
	sync, memberClient := prepareSynchronizer(t, userAccount, mur, hostClient)

	// when
	createdOrUpdated, err := sync.synchronizeSpec()

	// then
	require.NoError(t, err)
	assert.True(t, createdOrUpdated)
	uatest.AssertThatUserAccount(t, "john", memberClient).
		Exists().
		MatchMasterUserRecord(mur)

	murtest.AssertThatMasterUserRecord(t, "john", hostClient).
		HasConditions(toBeNotReady(toolchainv1alpha1.MasterUserRecordUpdatingReason, ""))
}

func TestSynchronizeStatus(t *testing.T) {
	// given
	apiScheme(t)
	mur := murtest.NewMasterUserRecord(t, "john",
		murtest.StatusCondition(toBeNotReady(toolchainv1alpha1.MasterUserRecordProvisioningReason, "")))

	userAccount := uatest.NewUserAccountFromMur(mur,
		uatest.StatusCondition(toBeNotReady("Provisioning", "")), uatest.ResourceVersion("123abc"))

	t.Run("successful", func(t *testing.T) {
		// given
		hostClient := test.NewFakeClient(t, mur, readyToolchainStatus)
		sync, memberClient := prepareSynchronizer(t, userAccount, mur, hostClient)

		// when
		err := sync.synchronizeStatus()

		// then
		require.NoError(t, err)
		require.Nil(t, sync.record.Status.ProvisionedTime)
		verifySyncMurStatusWithUserAccountStatus(t, memberClient, hostClient, userAccount, mur, toBeNotReady(toolchainv1alpha1.MasterUserRecordProvisioningReason, ""))
	})

	t.Run("failed on the host side", func(t *testing.T) {
		// given
		hostClient := test.NewFakeClient(t, mur, readyToolchainStatus)
		hostClient.MockStatusUpdate = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.UpdateOption) error {
			return fmt.Errorf("some error")
		}
		sync, _ := prepareSynchronizer(t, userAccount, mur, hostClient)

		// when
		err := sync.synchronizeStatus()

		// then
		require.Error(t, err)
		assert.Equal(t, "some error", err.Error())
	})
}

func TestSyncMurStatusWithUserAccountStatusWhenUpdated(t *testing.T) {
	// given
	apiScheme(t)
	userSignup := commonsignup.NewUserSignup()
	userSignup.Status = toolchainv1alpha1.UserSignupStatus{
		CompliantUsername: "john",
	}
	mur := murtest.NewMasterUserRecord(t, "john",
		murtest.StatusCondition(toBeNotReady(toolchainv1alpha1.MasterUserRecordUpdatingReason, "")),
		murtest.WithOwnerLabel(userSignup.Name))

	userAccount := uatest.NewUserAccountFromMur(mur,
		uatest.StatusCondition(toBeProvisioned()), uatest.ResourceVersion("123abc"))

	mur.Status.UserAccounts = []toolchainv1alpha1.UserAccountStatusEmbedded{{
		Cluster: toolchainv1alpha1.Cluster{
			Name: test.MemberClusterName,
		},
		UserAccountStatus: userAccount.Status,
	}}

	t.Run("successful", func(t *testing.T) {
		// given
		updatingUserAccount := userAccount.DeepCopy()
		uatest.Modify(updatingUserAccount, uatest.StatusCondition(toBeNotReady("Updating", "")))
		hostClient := test.NewFakeClient(t, mur, readyToolchainStatus)
		sync, memberClient := prepareSynchronizer(t, updatingUserAccount, mur, hostClient)

		// when
		err := sync.synchronizeStatus()

		// then
		require.NoError(t, err)
		require.Nil(t, sync.record.Status.ProvisionedTime)
		verifySyncMurStatusWithUserAccountStatus(t, memberClient, hostClient, updatingUserAccount, mur, toBeNotReady(toolchainv1alpha1.MasterUserRecordUpdatingReason, ""))
	})

	t.Run("successful and ready", func(t *testing.T) {
		// given

		hostClient := test.NewFakeClient(t, userSignup, mur, readyToolchainStatus)
		sync, memberClient := prepareSynchronizer(t, userAccount, mur, hostClient)

		// when
		preSyncTime := metav1.Now()
		err := sync.synchronizeStatus()

		// then
		require.NoError(t, err)
		require.True(t, preSyncTime.Time.Before(sync.record.Status.ProvisionedTime.Time), "the timestamp just before syncing should be before the ProvisionedTime")
		verifySyncMurStatusWithUserAccountStatus(t, memberClient, hostClient, userAccount, mur, toBeProvisioned(), toBeProvisionedNotificationCreated())
		OnlyOneNotificationExists(t, hostClient, mur.Name, toolchainv1alpha1.NotificationTypeProvisioned, HasContext("RegistrationURL", "https://registration.crt-placeholder.com"))
	})

	t.Run("failed on the host side", func(t *testing.T) {
		// given
		hostClient := test.NewFakeClient(t, mur)
		hostClient.MockStatusUpdate = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.UpdateOption) error {
			return fmt.Errorf("some error")
		}
		sync, _ := prepareSynchronizer(t, userAccount, mur, hostClient)

		// when
		err := sync.synchronizeStatus()

		// then
		require.Error(t, err)
	})
}

func TestSyncMurStatusWithUserAccountStatusWhenDisabled(t *testing.T) {
	// given
	apiScheme(t)
	mur := murtest.NewMasterUserRecord(t, "john",
		murtest.StatusCondition(toBeNotReady(toolchainv1alpha1.MasterUserRecordProvisioningReason, "")))

	userAccount := uatest.NewUserAccountFromMur(mur,
		uatest.StatusCondition(toBeNotReady(toolchainv1alpha1.MasterUserRecordProvisioningReason, "")), uatest.ResourceVersion("123abc"))

	mur.Status.UserAccounts = []toolchainv1alpha1.UserAccountStatusEmbedded{{
		Cluster: toolchainv1alpha1.Cluster{
			Name: test.MemberClusterName,
		},
		UserAccountStatus: userAccount.Status,
	}}

	uatest.Modify(userAccount, uatest.StatusCondition(toBeDisabled()))

	t.Run("successful", func(t *testing.T) {
		// given
		hostClient := test.NewFakeClient(t, mur, readyToolchainStatus)
		sync, memberClient := prepareSynchronizer(t, userAccount, mur, hostClient)

		// when
		err := sync.synchronizeStatus()

		// then
		require.NoError(t, err)
		require.Nil(t, sync.record.Status.ProvisionedTime)
		verifySyncMurStatusWithUserAccountStatus(t, memberClient, hostClient, userAccount, mur, toBeDisabled())
	})

	t.Run("failed on the host side", func(t *testing.T) {
		// given
		hostClient := test.NewFakeClient(t, mur.DeepCopy())
		hostClient.MockStatusUpdate = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.UpdateOption) error {
			return fmt.Errorf("some error")
		}
		sync, _ := prepareSynchronizer(t, userAccount, mur, hostClient)

		// when
		err := sync.synchronizeStatus()

		// then
		require.Error(t, err)
	})
}

func TestAlignReadiness(t *testing.T) {
	// given
	s := apiScheme(t)
	os.Setenv("WATCH_NAMESPACE", test.HostOperatorNs)

	// A basic userSignup to set as the mur owner
	userSignup := commonsignup.NewUserSignup()
	userSignup.Status = toolchainv1alpha1.UserSignupStatus{
		CompliantUsername: "john",
	}

	preparedMUR := murtest.NewMasterUserRecord(t, "john",
		murtest.StatusCondition(toBeNotReady(toolchainv1alpha1.MasterUserRecordProvisioningReason, "")),
		murtest.StatusUserAccount(test.MemberClusterName, toBeProvisioned()),
		murtest.WithOwnerLabel(userSignup.Name))

	dummyNotification := &toolchainv1alpha1.Notification{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("dummy-%s", toolchainv1alpha1.NotificationTypeProvisioned),
			Namespace: test.HostOperatorNs,
			Labels: map[string]string{
				toolchainv1alpha1.NotificationUserNameLabelKey: "dummy",
				toolchainv1alpha1.NotificationTypeLabelKey:     toolchainv1alpha1.NotificationTypeProvisioned,
			},
		},
	}

	log := zap.New(zap.UseDevMode(true))

	t.Run("ready propagated and notification created", func(t *testing.T) {
		// given
		mur := preparedMUR.DeepCopy()
		hostClient := test.NewFakeClient(t, userSignup, mur, readyToolchainStatus, dummyNotification)

		// when
		preAlignTime := metav1.Now()
		ready, err := alignReadiness(log, s, hostClient, mur)

		// then
		require.NoError(t, err)
		assert.True(t, ready)
		require.True(t, preAlignTime.Time.Before(mur.Status.ProvisionedTime.Time), "the timestamp just before syncing should be before the ProvisionedTime")
		murtest.AssertThatMasterUserRecord(t, "john", test.NewFakeClient(t, mur)).
			HasConditions(toBeProvisioned(), toBeProvisionedNotificationCreated()).
			HasStatusUserAccounts(test.MemberClusterName).
			AllUserAccountsHaveCondition(toBeProvisioned())
		OnlyOneNotificationExists(t, hostClient, mur.Name, toolchainv1alpha1.NotificationTypeProvisioned, HasContext("RegistrationURL", "https://registration.crt-placeholder.com"))
	})

	t.Run("ready propagated and notification created with two accounts", func(t *testing.T) {
		// given
		mur := murtest.NewMasterUserRecord(t, "john",
			murtest.StatusCondition(toBeNotReady(toolchainv1alpha1.MasterUserRecordProvisioningReason, "")),
			murtest.StatusUserAccount(test.MemberClusterName, toBeProvisioned()),
			murtest.StatusUserAccount(test.Member2ClusterName, toBeProvisioned()),
			murtest.WithOwnerLabel(userSignup.Name))
		hostClient := test.NewFakeClient(t, userSignup, mur, readyToolchainStatus, dummyNotification)

		// when
		preAlignTime := metav1.Now()
		ready, err := alignReadiness(log, s, hostClient, mur)

		// then
		require.NoError(t, err)
		assert.True(t, ready)
		require.True(t, preAlignTime.Time.Before(mur.Status.ProvisionedTime.Time), "the timestamp just before syncing should be before the ProvisionedTime")
		murtest.AssertThatMasterUserRecord(t, "john", test.NewFakeClient(t, mur)).
			HasConditions(toBeProvisioned(), toBeProvisionedNotificationCreated()).
			HasStatusUserAccounts(test.MemberClusterName, test.Member2ClusterName).
			AllUserAccountsHaveCondition(toBeProvisioned())
		OnlyOneNotificationExists(t, hostClient, mur.Name, toolchainv1alpha1.NotificationTypeProvisioned, HasContext("RegistrationURL", "https://registration.crt-placeholder.com"))
	})

	t.Run("UserAccount not ready", func(t *testing.T) {
		// given
		mur := murtest.NewMasterUserRecord(t, "john",
			murtest.StatusCondition(toBeNotReady(toolchainv1alpha1.MasterUserRecordProvisioningReason, "")),
			murtest.StatusUserAccount(test.MemberClusterName, toBeNotReady(toolchainv1alpha1.UserAccountProvisioningReason, "")),
			murtest.WithOwnerLabel(userSignup.Name))

		hostClient := test.NewFakeClient(t, userSignup, mur, readyToolchainStatus)

		// when
		ready, err := alignReadiness(log, s, hostClient, mur)

		// then
		require.NoError(t, err)
		assert.False(t, ready)
		require.Empty(t, mur.Status.ProvisionedTime)
		murtest.AssertThatMasterUserRecord(t, "john", test.NewFakeClient(t, mur)).
			HasConditions(toBeNotReady(toolchainv1alpha1.MasterUserRecordProvisioningReason, "")).
			HasStatusUserAccounts(test.MemberClusterName).
			AllUserAccountsHaveCondition(toBeNotReady(toolchainv1alpha1.UserAccountProvisioningReason, ""))
		AssertNoNotificationsExist(t, hostClient)
	})

	t.Run("two UserAccounts, one not ready", func(t *testing.T) {
		// given
		mur := murtest.NewMasterUserRecord(t, "john",
			murtest.StatusCondition(toBeNotReady(toolchainv1alpha1.MasterUserRecordProvisioningReason, "")),
			murtest.StatusUserAccount(test.MemberClusterName, toBeProvisioned()),
			murtest.StatusUserAccount(test.Member2ClusterName, toBeNotReady(toolchainv1alpha1.UserAccountProvisioningReason, "")),
			murtest.WithOwnerLabel(userSignup.Name))

		hostClient := test.NewFakeClient(t, userSignup, mur, readyToolchainStatus)

		// when
		ready, err := alignReadiness(log, s, hostClient, mur)

		// then
		require.NoError(t, err)
		assert.False(t, ready)
		require.Empty(t, mur.Status.ProvisionedTime)
		murtest.AssertThatMasterUserRecord(t, "john", test.NewFakeClient(t, mur)).
			HasConditions(toBeNotReady(toolchainv1alpha1.MasterUserRecordProvisioningReason, "")).
			HasStatusUserAccounts(test.MemberClusterName, test.Member2ClusterName).
			HasStatusUserAccountsWithCondition(test.MemberClusterName, toBeProvisioned()).
			HasStatusUserAccountsWithCondition(test.Member2ClusterName, toBeNotReady(toolchainv1alpha1.UserAccountProvisioningReason, ""))
		AssertNoNotificationsExist(t, hostClient)
	})

	t.Run("no UserAccount in status, space creation not skipped, no pre-existing MUR ready condition", func(t *testing.T) {
		// given
		mur := murtest.NewMasterUserRecord(t, "john",
			murtest.WithOwnerLabel(userSignup.Name))

		hostClient := test.NewFakeClient(t, userSignup, mur, readyToolchainStatus)

		// when
		ready, err := alignReadiness(log, s, hostClient, mur)

		// then
		require.NoError(t, err)
		assert.False(t, ready)
		require.Empty(t, mur.Status.ProvisionedTime)
		murtest.AssertThatMasterUserRecord(t, "john", test.NewFakeClient(t, mur)).
			HasConditions().
			HasStatusUserAccounts()
		AssertNoNotificationsExist(t, hostClient)
	})

	t.Run("no UserAccount in status, space creation not skipped, with pre-existing MUR ready condition", func(t *testing.T) {
		// given
		mur := murtest.NewMasterUserRecord(t, "john",
			murtest.StatusCondition(toBeNotReady(toolchainv1alpha1.MasterUserRecordProvisioningReason, "")),
			murtest.WithOwnerLabel(userSignup.Name))
		provisionTime := metav1.NewTime(time.Now().Add(-time.Hour))
		mur.Status.ProvisionedTime = &provisionTime
		hostClient := test.NewFakeClient(t, userSignup, mur, readyToolchainStatus)

		// when
		ready, err := alignReadiness(log, s, hostClient, mur)

		// then
		require.NoError(t, err)
		assert.True(t, ready)
		murtest.AssertThatMasterUserRecord(t, "john", test.NewFakeClient(t, mur)).
			HasStatusUserAccounts().
			HasConditions(toBeProvisioned(), toBeProvisionedNotificationCreated())
		OnlyOneNotificationExists(t, hostClient, mur.Name, toolchainv1alpha1.NotificationTypeProvisioned, HasContext("RegistrationURL", "https://registration.crt-placeholder.com"))
	})

	t.Run("no UserAccount in status, space creation is skipped", func(t *testing.T) {
		// given
		murWithoutCondition := murtest.NewMasterUserRecord(t, "john",
			murtest.WithOwnerLabel(userSignup.Name),
			murtest.WithAnnotation("toolchain.dev.openshift.com/skip-auto-create-space", "true"))
		murWithCondition := murtest.NewMasterUserRecord(t, "john",
			murtest.StatusCondition(toBeNotReady(toolchainv1alpha1.MasterUserRecordProvisioningReason, "")),
			murtest.WithOwnerLabel(userSignup.Name),
			murtest.WithAnnotation("toolchain.dev.openshift.com/skip-auto-create-space", "true"))

		for _, mur := range []*toolchainv1alpha1.MasterUserRecord{murWithoutCondition, murWithCondition} {
			hostClient := test.NewFakeClient(t, userSignup, mur, readyToolchainStatus)

			// when
			preAlignTime := metav1.Now()
			ready, err := alignReadiness(log, s, hostClient, mur)

			// then
			require.NoError(t, err)
			assert.True(t, ready)
			require.True(t, preAlignTime.Time.Before(mur.Status.ProvisionedTime.Time), "the timestamp just before syncing should be before the ProvisionedTime")
			murtest.AssertThatMasterUserRecord(t, "john", test.NewFakeClient(t, mur)).
				HasConditions(toBeProvisioned(), toBeProvisionedNotificationCreated()).
				HasStatusUserAccounts().
				AllUserAccountsHaveCondition(toBeProvisioned())
			OnlyOneNotificationExists(t, hostClient, mur.Name, toolchainv1alpha1.NotificationTypeProvisioned, HasContext("RegistrationURL", "https://registration.crt-placeholder.com"))
		}

	})

	t.Run("ProvisionedTime should not be updated when synced more than once", func(t *testing.T) {
		// given
		mur := preparedMUR.DeepCopy()
		hostClient := test.NewFakeClient(t, userSignup, mur, readyToolchainStatus, dummyNotification)
		provisionTime := metav1.NewTime(time.Now().Add(-time.Hour))
		mur.Status.ProvisionedTime = &provisionTime

		// when
		preAlignTime := metav1.Now()
		ready, err := alignReadiness(log, s, hostClient, mur)

		// then
		require.NoError(t, err)
		assert.True(t, ready)
		require.True(t, preAlignTime.Time.After(mur.Status.ProvisionedTime.Time), "the timestamp just before syncing should be after the ProvisionedTime because this is simulating the case where the record was already provisioned before")
		assert.Equal(t, provisionTime.Time, mur.Status.ProvisionedTime.Time) // timestamp should be the same
		murtest.AssertThatMasterUserRecord(t, "john", test.NewFakeClient(t, mur)).
			HasConditions(toBeProvisioned(), toBeProvisionedNotificationCreated()).
			HasStatusUserAccounts(test.MemberClusterName).
			AllUserAccountsHaveCondition(toBeProvisioned())
		OnlyOneNotificationExists(t, hostClient, mur.Name, toolchainv1alpha1.NotificationTypeProvisioned, HasContext("RegistrationURL", "https://registration.crt-placeholder.com"))
	})

	t.Run("When notification was already created, but the update of status failed before, which means that the condition is not set", func(t *testing.T) {
		// given
		mur := preparedMUR.DeepCopy()
		notification := &toolchainv1alpha1.Notification{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-%s", mur.Name, toolchainv1alpha1.NotificationTypeProvisioned),
				Namespace: test.HostOperatorNs,
				Labels: map[string]string{
					toolchainv1alpha1.NotificationUserNameLabelKey: mur.Name,
					toolchainv1alpha1.NotificationTypeLabelKey:     toolchainv1alpha1.NotificationTypeProvisioned,
				},
			},
		}
		hostClient := test.NewFakeClient(t, userSignup, mur, readyToolchainStatus, dummyNotification, notification)

		// when
		preAlignTime := metav1.Now()
		ready, err := alignReadiness(log, s, hostClient, mur)

		// then
		require.NoError(t, err)
		assert.True(t, ready)
		require.True(t, preAlignTime.Time.Before(mur.Status.ProvisionedTime.Time), "the timestamp just before syncing should be before the ProvisionedTime")
		murtest.AssertThatMasterUserRecord(t, "john", test.NewFakeClient(t, mur)).
			HasConditions(toBeProvisioned(), toBeProvisionedNotificationCreated()).
			HasStatusUserAccounts(test.MemberClusterName).
			AllUserAccountsHaveCondition(toBeProvisioned())
		OnlyOneNotificationExists(t, hostClient, mur.Name, toolchainv1alpha1.NotificationTypeProvisioned)
	})

	t.Run("no new notification created when condition is already set", func(t *testing.T) {
		// given
		mur := preparedMUR.DeepCopy()
		mur.Status.Conditions = append(mur.Status.Conditions, toBeProvisionedNotificationCreated())
		hostClient := test.NewFakeClient(t, mur, readyToolchainStatus)

		// when
		preAlignTime := metav1.Now()
		ready, err := alignReadiness(log, s, hostClient, mur)

		// then
		require.NoError(t, err)
		assert.True(t, ready)
		require.True(t, preAlignTime.Time.Before(mur.Status.ProvisionedTime.Time), "the timestamp just before syncing should be before the ProvisionedTime")
		murtest.AssertThatMasterUserRecord(t, "john", test.NewFakeClient(t, mur)).
			HasConditions(toBeProvisioned(), toBeProvisionedNotificationCreated()).
			HasStatusUserAccounts(test.MemberClusterName).
			AllUserAccountsHaveCondition(toBeProvisioned())
		AssertNoNotificationsExist(t, hostClient)
	})

	t.Run("failed on the host side when creating notification", func(t *testing.T) {
		// given
		mur := preparedMUR.DeepCopy()
		hostClient := test.NewFakeClient(t, mur)
		hostClient.MockCreate = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.CreateOption) error {
			return fmt.Errorf("some error")
		}

		// when
		ready, err := alignReadiness(log, s, hostClient, mur)

		// then
		require.Error(t, err)
		assert.False(t, ready)
		AssertNoNotificationsExist(t, hostClient)
		murtest.AssertThatMasterUserRecord(t, "john", test.NewFakeClient(t, mur)).
			HasConditions(toBeProvisioned()).
			HasStatusUserAccounts(test.MemberClusterName).
			AllUserAccountsHaveCondition(toBeProvisioned())
	})
}

func TestSynchronizeUserAccountFailed(t *testing.T) {
	// given
	l := zap.New(zap.UseDevMode(true))
	scheme := apiScheme(t)

	t.Run("spec synchronization of the UserAccount failed", func(t *testing.T) {
		// given
		mur := murtest.NewMasterUserRecord(t, "john")
		userAcc := uatest.NewUserAccountFromMur(mur)

		memberClient := test.NewFakeClient(t, userAcc)
		memberClient.MockUpdate = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.UpdateOption) error {
			return fmt.Errorf("unable to update user account %s", mur.Name)
		}
		err := murtest.Modify(mur, murtest.UserID("abc123"))
		require.NoError(t, err)
		hostClient := test.NewFakeClient(t, mur, readyToolchainStatus)

		sync := Synchronizer{
			record:        mur,
			hostClient:    hostClient,
			memberCluster: newMemberCluster(memberClient),
			memberUserAcc: userAcc,
			logger:        l,
			scheme:        scheme,
		}

		// when
		createdOrUpdated, err := sync.synchronizeSpec()

		// then
		require.Error(t, err)
		assert.False(t, createdOrUpdated)
		assert.Contains(t, err.Error(), "unable to update user account john")
	})

	t.Run("status synchronization of the UserAccount & MasterUserRecord failed", func(t *testing.T) {
		// given
		provisionedMur := murtest.NewMasterUserRecord(t, "john",
			murtest.StatusCondition(toBeProvisioned()))
		userAcc := uatest.NewUserAccountFromMur(provisionedMur,
			uatest.StatusCondition(toBeNotReady("somethingFailed", "")))
		memberClient := test.NewFakeClient(t, userAcc)
		hostClient := test.NewFakeClient(t, provisionedMur, readyToolchainStatus)
		hostClient.MockStatusUpdate = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.UpdateOption) error {
			return fmt.Errorf("unable to update MUR %s", provisionedMur.Name)
		}
		sync := Synchronizer{
			record:        provisionedMur,
			hostClient:    hostClient,
			memberCluster: newMemberCluster(memberClient),
			memberUserAcc: userAcc,
			logger:        l,
			scheme:        scheme,
		}

		t.Run("with empty set of UserAccounts statuses", func(t *testing.T) {
			// when
			err := sync.synchronizeStatus()

			// then
			require.Error(t, err)
			assert.Contains(t, err.Error(), "unable to update MUR john")
			assert.Len(t, provisionedMur.Status.UserAccounts, 0)
		})

		t.Run("when the UserAccount was added", func(t *testing.T) {
			//given
			additionalUserAcc := toolchainv1alpha1.UserAccountStatusEmbedded{
				Cluster: toolchainv1alpha1.Cluster{
					Name: "some-other",
				},
			}
			provisionedMur.Status.UserAccounts = []toolchainv1alpha1.UserAccountStatusEmbedded{additionalUserAcc}

			// when
			err := sync.synchronizeStatus()

			// then
			require.Error(t, err)
			assert.Contains(t, err.Error(), "unable to update MUR john")
			assert.Len(t, provisionedMur.Status.UserAccounts, 1)
			assert.Contains(t, provisionedMur.Status.UserAccounts, additionalUserAcc)
		})

		t.Run("when the UserAccount was modified", func(t *testing.T) {
			//given
			toBeModified := toolchainv1alpha1.UserAccountStatusEmbedded{
				Cluster: toolchainv1alpha1.Cluster{
					Name: test.MemberClusterName,
				},
			}
			provisionedMur.Status.UserAccounts = []toolchainv1alpha1.UserAccountStatusEmbedded{toBeModified}

			// when
			err := sync.synchronizeStatus()

			// then
			require.Error(t, err)
			assert.Contains(t, err.Error(), "unable to update MUR john")
			assert.Len(t, provisionedMur.Status.UserAccounts, 1)
			assert.Contains(t, provisionedMur.Status.UserAccounts, toBeModified)
		})

		t.Run("when routes are not set", func(t *testing.T) {
			mur := murtest.NewMasterUserRecord(t, "john",
				murtest.StatusCondition(toBeNotReady(toolchainv1alpha1.MasterUserRecordProvisioningReason, "")))

			userAccount := uatest.NewUserAccountFromMur(mur,
				uatest.StatusCondition(toBeNotReady(toolchainv1alpha1.MasterUserRecordProvisioningReason, "")), uatest.ResourceVersion("123abc"))

			mur.Status.UserAccounts = []toolchainv1alpha1.UserAccountStatusEmbedded{{
				Cluster: toolchainv1alpha1.Cluster{
					Name: test.MemberClusterName,
				},
				UserAccountStatus: userAccount.Status,
			}}

			uatest.Modify(userAccount, uatest.StatusCondition(toBeProvisioned()))

			t.Run("condition is not ready", func(t *testing.T) {
				// given
				for _, toolchainStatus := range []*toolchainv1alpha1.ToolchainStatus{
					NewToolchainStatus(WithMember(test.MemberClusterName, WithRoutes("", "", ToBeNotReady()))),
					NewToolchainStatus(WithMember(test.MemberClusterName)),
				} {
					memberClient := test.NewFakeClient(t, userAccount)
					hostClient := test.NewFakeClient(t, mur, toolchainStatus)
					sync := Synchronizer{
						record:        mur,
						hostClient:    hostClient,
						memberCluster: newMemberCluster(memberClient),
						memberUserAcc: userAccount,
						logger:        l,
						scheme:        scheme,
					}

					// when
					err := sync.synchronizeStatus()

					// then
					assert.Error(t, err)
				}
			})
		})
	})
}

func TestRemoveAccountFromStatus(t *testing.T) {
	// given
	apiScheme(t)
	userSignup := commonsignup.NewUserSignup()
	userSignup.Status = toolchainv1alpha1.UserSignupStatus{
		CompliantUsername: "john",
	}

	t.Run("remove UserAccount from the status when there is one item", func(t *testing.T) {
		// given
		mur := murtest.NewMasterUserRecord(t, "john",
			murtest.WithOwnerLabel(userSignup.Name),
			murtest.StatusCondition(toBeNotReady(toolchainv1alpha1.MasterUserRecordProvisioningReason, "")),
			murtest.StatusUserAccount(test.MemberClusterName, toBeProvisioned()))
		hostClient := test.NewFakeClient(t, mur, readyToolchainStatus, userSignup)
		sync, _ := prepareSynchronizer(t, nil, mur, hostClient)

		// when
		err := sync.removeAccountFromStatus()

		// then
		require.NoError(t, err)
		murtest.AssertThatMasterUserRecord(t, mur.Name, hostClient).
			HasStatusUserAccounts().
			HasConditions(toBeProvisioned(), toBeProvisionedNotificationCreated())
	})

	t.Run("remove UserAccount from the status when there are two items and align readiness", func(t *testing.T) {
		// given
		mur := murtest.NewMasterUserRecord(t, "john",
			murtest.WithOwnerLabel(userSignup.Name),
			murtest.StatusCondition(toBeNotReady("Provisioning", "")),
			murtest.StatusUserAccount(test.MemberClusterName, toBeNotReady("terminating", "")),
			murtest.StatusUserAccount(test.Member2ClusterName, toBeProvisioned()))
		hostClient := test.NewFakeClient(t, mur, readyToolchainStatus, userSignup)
		sync, _ := prepareSynchronizer(t, nil, mur, hostClient)

		// when
		err := sync.removeAccountFromStatus()

		// then
		require.NoError(t, err)
		murtest.AssertThatMasterUserRecord(t, mur.Name, hostClient).
			HasStatusUserAccounts(test.Member2ClusterName).
			HasConditions(toBeProvisioned(), toBeProvisionedNotificationCreated())
	})

	t.Run("don't remove anything when UserAccount is not present, don't align readiness", func(t *testing.T) {
		// given
		mur := murtest.NewMasterUserRecord(t, "john",
			murtest.WithOwnerLabel(userSignup.Name),
			murtest.StatusCondition(toBeNotReady("Provisioning", "")),
			murtest.StatusUserAccount(test.Member2ClusterName, toBeProvisioned()))
		hostClient := test.NewFakeClient(t, mur, readyToolchainStatus, userSignup)
		sync, _ := prepareSynchronizer(t, nil, mur, hostClient)

		// when
		err := sync.removeAccountFromStatus()

		// then
		require.NoError(t, err)
		murtest.AssertThatMasterUserRecord(t, mur.Name, hostClient).
			HasStatusUserAccounts(test.Member2ClusterName).
			HasConditions(toBeNotReady("Provisioning", ""))
	})
}

func TestRoutes(t *testing.T) {
	// given
	l := zap.New(zap.UseDevMode(true))
	logf.SetLogger(l)
	apiScheme(t)

	masterUserRec := murtest.NewMasterUserRecord(t, "john",
		murtest.StatusCondition(toBeNotReady(toolchainv1alpha1.MasterUserRecordProvisioningReason, "")))
	userAccount := uatest.NewUserAccountFromMur(masterUserRec,
		uatest.StatusCondition(toBeNotReady("Provisioning", "")), uatest.ResourceVersion("123abc"))
	condition := userAccount.Status.Conditions[0]
	memberClient := test.NewFakeClient(t, userAccount)

	t.Run("routes are set", func(t *testing.T) {
		// given
		toolchainStatus := NewToolchainStatus(
			WithMember(test.MemberClusterName, WithRoutes("https://console.member-cluster/", "https://che-toolchain-che.member-cluster/", ToBeReady())))
		mur := masterUserRec.DeepCopy()

		hostClient := test.NewFakeClient(t, mur, toolchainStatus)
		sync := Synchronizer{
			record:        mur.DeepCopy(),
			hostClient:    hostClient,
			memberCluster: newMemberCluster(memberClient),
			memberUserAcc: userAccount,
			logger:        l,
		}

		// when
		err := sync.synchronizeStatus()

		// then
		require.NoError(t, err)
		uatest.AssertThatUserAccount(t, "john", memberClient).
			Exists().
			MatchMasterUserRecord(mur).
			HasConditions(condition)
		murtest.AssertThatMasterUserRecord(t, "john", hostClient).
			AllUserAccountsHaveCluster(toolchainv1alpha1.Cluster{
				Name:            test.MemberClusterName,
				APIEndpoint:     "https://api.member-cluster:6433",
				ConsoleURL:      "https://console.member-cluster/",
				CheDashboardURL: "https://che-toolchain-che.member-cluster/",
			}).
			AllUserAccountsHaveCondition(condition)
	})

	t.Run("che route is missing but condition is ready", func(t *testing.T) {
		// given
		toolchainStatus := NewToolchainStatus(
			WithMember(test.MemberClusterName, WithRoutes("https://console.member-cluster/", "", ToBeReady())))
		mur := masterUserRec.DeepCopy()

		hostClient := test.NewFakeClient(t, mur, toolchainStatus)
		sync := Synchronizer{
			record:        mur,
			hostClient:    hostClient,
			memberCluster: newMemberCluster(memberClient),
			memberUserAcc: userAccount,
			logger:        l,
		}

		// when
		err := sync.synchronizeStatus()

		// then
		require.NoError(t, err)
		uatest.AssertThatUserAccount(t, "john", memberClient).
			Exists().
			MatchMasterUserRecord(mur).
			HasConditions(condition)
		murtest.AssertThatMasterUserRecord(t, "john", hostClient).
			AllUserAccountsHaveCluster(toolchainv1alpha1.Cluster{
				Name:            test.MemberClusterName,
				APIEndpoint:     "https://api.member-cluster:6433",
				ConsoleURL:      "https://console.member-cluster/",
				CheDashboardURL: "",
			}).
			AllUserAccountsHaveCondition(condition)
	})

	t.Run("condition is not ready", func(t *testing.T) {
		// given
		toolchainStatus := NewToolchainStatus(
			WithMember(test.MemberClusterName, WithRoutes("https://console.member-cluster/", "", ToBeNotReady())))
		mur := masterUserRec.DeepCopy()

		hostClient := test.NewFakeClient(t, mur, toolchainStatus)
		sync := Synchronizer{
			record:        mur,
			hostClient:    hostClient,
			memberCluster: newMemberCluster(memberClient),
			memberUserAcc: userAccount,
			logger:        l,
		}

		// when
		err := sync.synchronizeStatus()

		// then
		require.Error(t, err)
		uatest.AssertThatUserAccount(t, "john", memberClient).
			Exists().
			MatchMasterUserRecord(mur).
			HasConditions(condition)
		murtest.AssertThatMasterUserRecord(t, "john", hostClient).
			AllUserAccountsHaveCluster(toolchainv1alpha1.Cluster{
				Name:            test.MemberClusterName,
				APIEndpoint:     "https://api.member-cluster:6433",
				ConsoleURL:      "",
				CheDashboardURL: "",
			}).
			AllUserAccountsHaveCondition(condition)
	})
}

func prepareSynchronizer(t *testing.T, userAccount *toolchainv1alpha1.UserAccount, mur *toolchainv1alpha1.MasterUserRecord, hostClient *test.FakeClient) (Synchronizer, runtimeclient.Client) {
	require.NoError(t, os.Setenv("WATCH_NAMESPACE", test.HostOperatorNs))
	copiedMur := mur.DeepCopy()
	memberClient := test.NewFakeClient(t)
	if userAccount != nil {
		memberClient = test.NewFakeClient(t, userAccount)
	}

	return Synchronizer{
		record:        copiedMur,
		hostClient:    hostClient,
		memberCluster: newMemberCluster(memberClient),
		memberUserAcc: userAccount,
		logger:        zap.New(zap.UseDevMode(true)),
		scheme:        apiScheme(t),
	}, memberClient
}

func verifySyncMurStatusWithUserAccountStatus(t *testing.T, memberClient, hostClient runtimeclient.Client, userAccount *toolchainv1alpha1.UserAccount, mur *toolchainv1alpha1.MasterUserRecord, expMurCon ...toolchainv1alpha1.Condition) {
	userAccountCondition := userAccount.Status.Conditions[0]
	uatest.AssertThatUserAccount(t, "john", memberClient).
		Exists().
		MatchMasterUserRecord(mur).
		HasConditions(userAccountCondition)
	murtest.AssertThatMasterUserRecord(t, "john", hostClient).
		HasConditions(expMurCon...).
		HasStatusUserAccounts(test.MemberClusterName).
		AllUserAccountsHaveCluster(toolchainv1alpha1.Cluster{
			Name:            test.MemberClusterName,
			APIEndpoint:     "https://api.member-cluster:6433",
			ConsoleURL:      "https://console.member-cluster/",
			CheDashboardURL: "http://che-toolchain-che.member-cluster/",
		}).
		AllUserAccountsHaveCondition(userAccountCondition)
}

func newMemberCluster(cl runtimeclient.Client) cluster.Cluster {
	return cluster.Cluster{
		Config: &commoncluster.Config{
			Name:              test.MemberClusterName,
			APIEndpoint:       fmt.Sprintf("https://api.%s:6433", test.MemberClusterName),
			Type:              commoncluster.Member,
			OperatorNamespace: test.HostOperatorNs,
			OwnerClusterName:  test.HostClusterName,
		},
		Client: cl,
	}
}
