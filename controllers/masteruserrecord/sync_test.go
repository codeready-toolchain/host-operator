package masteruserrecord

import (
	"context"
	"fmt"
	"testing"
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	. "github.com/codeready-toolchain/host-operator/test"
	. "github.com/codeready-toolchain/host-operator/test/notification"
	tiertest "github.com/codeready-toolchain/host-operator/test/nstemplatetier"
	"github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	murtest "github.com/codeready-toolchain/toolchain-common/pkg/test/masteruserrecord"
	uatest "github.com/codeready-toolchain/toolchain-common/pkg/test/useraccount"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var readyToolchainStatus = NewToolchainStatus(
	WithMember(test.MemberClusterName, WithRoutes("https://console.member-cluster/", "http://che-toolchain-che.member-cluster/", ToBeReady())))

func TestIsSynchronized(t *testing.T) {

	t.Run("synchronized", func(t *testing.T) {
		// given
		record, recordSpecUserAcc, memberUserAcc := setupSynchronizerItems()
		s := Synchronizer{
			memberUserAcc:     &memberUserAcc,
			record:            &record,
			recordSpecUserAcc: recordSpecUserAcc,
		}
		// when/then
		assert.True(t, s.isSynchronized())
	})

	t.Run("not synchronized", func(t *testing.T) {

		t.Run("missing label", func(t *testing.T) {
			// given
			record, recordSpecUserAcc, memberUserAcc := setupSynchronizerItems()
			delete(memberUserAcc.Labels, toolchainv1alpha1.TierLabelKey)
			s := Synchronizer{
				memberUserAcc:     &memberUserAcc,
				record:            &record,
				recordSpecUserAcc: recordSpecUserAcc,
			}
			// when/then
			assert.False(t, s.isSynchronized())
		})

		t.Run("label does not match", func(t *testing.T) {
			// given
			record, recordSpecUserAcc, memberUserAcc := setupSynchronizerItems()
			memberUserAcc.Labels[toolchainv1alpha1.TierLabelKey] = "foo"
			s := Synchronizer{
				memberUserAcc:     &memberUserAcc,
				record:            &record,
				recordSpecUserAcc: recordSpecUserAcc,
			}
			// when/then
			assert.False(t, s.isSynchronized())
		})

		t.Run("different user account", func(t *testing.T) {
			// given
			record, recordSpecUserAcc, memberUserAcc := setupSynchronizerItems()
			recordSpecUserAcc.Spec.NSLimit = "bar"
			s := Synchronizer{
				memberUserAcc:     &memberUserAcc,
				record:            &record,
				recordSpecUserAcc: recordSpecUserAcc,
			}
			// when/then
			assert.False(t, s.isSynchronized())
		})

		t.Run("different disable", func(t *testing.T) {
			// given
			record, recordSpecUserAcc, memberUserAcc := setupSynchronizerItems()
			record.Spec.Disabled = true
			s := Synchronizer{
				memberUserAcc:     &memberUserAcc,
				record:            &record,
				recordSpecUserAcc: recordSpecUserAcc,
			}
			// when/then
			assert.False(t, s.isSynchronized())
		})

		t.Run("different userID", func(t *testing.T) {
			// given
			record, recordSpecUserAcc, memberUserAcc := setupSynchronizerItems()
			record.Spec.UserID = "bar"
			s := Synchronizer{
				memberUserAcc:     &memberUserAcc,
				record:            &record,
				recordSpecUserAcc: recordSpecUserAcc,
			}
			// when/then
			assert.False(t, s.isSynchronized())
		})
	})
}

func setupSynchronizerItems() (toolchainv1alpha1.MasterUserRecord, toolchainv1alpha1.UserAccountEmbedded, toolchainv1alpha1.UserAccount) {
	base := toolchainv1alpha1.UserAccountSpecBase{
		NSLimit: "limit",
		NSTemplateSet: &toolchainv1alpha1.NSTemplateSetSpec{
			TierName: "basic",
			ClusterResources: &toolchainv1alpha1.NSTemplateSetClusterResources{
				TemplateRef: "basic-clusterresources-654321a",
			},
			Namespaces: []toolchainv1alpha1.NSTemplateSetNamespace{
				{
					TemplateRef: "basic-code-123456a",
				},
			},
		},
	}
	memberUserAcc := toolchainv1alpha1.UserAccount{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				toolchainv1alpha1.TierLabelKey: "basic",
			},
		},
		Spec: toolchainv1alpha1.UserAccountSpec{
			UserID:              "foo",
			Disabled:            false,
			UserAccountSpecBase: base,
		},
	}
	recordSpecUserAcc := toolchainv1alpha1.UserAccountEmbedded{
		Spec: toolchainv1alpha1.UserAccountSpecEmbedded{
			UserAccountSpecBase: base,
		},
	}
	record := toolchainv1alpha1.MasterUserRecord{
		Spec: toolchainv1alpha1.MasterUserRecordSpec{
			UserID:   "foo",
			Disabled: false,
			UserAccounts: []toolchainv1alpha1.UserAccountEmbedded{
				recordSpecUserAcc,
			},
			TierName: "basic",
		},
	}
	return record, recordSpecUserAcc, memberUserAcc
}

func TestSynchronizeSpec(t *testing.T) {
	// given
	apiScheme(t)
	otherTier := tiertest.OtherTier()
	mur := murtest.NewMasterUserRecord(t, "john", murtest.StatusCondition(toBeProvisioned()), murtest.TierName(otherTier.Name))

	userAccount := uatest.NewUserAccountFromMur(mur)

	err := murtest.Modify(mur, murtest.UserID("abc123"))
	require.NoError(t, err)

	hostClient := test.NewFakeClient(t, mur)
	sync, memberClient := prepareSynchronizer(t, userAccount, mur, hostClient)

	// when
	err = sync.synchronizeSpec()

	// then
	require.NoError(t, err)
	uatest.AssertThatUserAccount(t, "john", memberClient).
		Exists().
		MatchMasterUserRecord(mur, mur.Spec.UserAccounts[0].Spec)

	murtest.AssertThatMasterUserRecord(t, "john", hostClient).
		HasTier(*otherTier).
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
		hostClient.MockStatusUpdate = func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
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
	mur := murtest.NewMasterUserRecord(t, "john",
		murtest.StatusCondition(toBeNotReady(toolchainv1alpha1.MasterUserRecordUpdatingReason, "")))

	userAccount := uatest.NewUserAccountFromMur(mur,
		uatest.StatusCondition(toBeProvisioned()), uatest.ResourceVersion("123abc"))

	mur.Status.UserAccounts = []toolchainv1alpha1.UserAccountStatusEmbedded{{
		SyncIndex: "111aaa",
		Cluster: toolchainv1alpha1.Cluster{
			Name: test.MemberClusterName,
		},
		UserAccountStatus: userAccount.Status,
	}}

	uatest.Modify(userAccount, uatest.StatusCondition(toBeNotReady("Updating", "")))

	t.Run("successful", func(t *testing.T) {
		// given
		hostClient := test.NewFakeClient(t, mur, readyToolchainStatus)
		sync, memberClient := prepareSynchronizer(t, userAccount, mur, hostClient)

		// when
		err := sync.synchronizeStatus()

		// then
		require.NoError(t, err)
		require.Nil(t, sync.record.Status.ProvisionedTime)
		verifySyncMurStatusWithUserAccountStatus(t, memberClient, hostClient, userAccount, mur, toBeNotReady(toolchainv1alpha1.MasterUserRecordUpdatingReason, ""))
	})

	t.Run("failed on the host side", func(t *testing.T) {
		// given
		hostClient := test.NewFakeClient(t, mur)
		hostClient.MockStatusUpdate = func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
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
		SyncIndex: "111aaa",
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
		hostClient.MockStatusUpdate = func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
			return fmt.Errorf("some error")
		}
		sync, _ := prepareSynchronizer(t, userAccount, mur, hostClient)

		// when
		err := sync.synchronizeStatus()

		// then
		require.Error(t, err)
	})
}

func TestSyncMurStatusWithUserAccountStatusWhenCompleted(t *testing.T) {
	// given
	apiScheme(t)

	// A basic userSignup to set as the mur owner
	userSignup := NewUserSignup()
	userSignup.Status = toolchainv1alpha1.UserSignupStatus{
		CompliantUsername: "john",
	}

	mur := murtest.NewMasterUserRecord(t, "john",
		murtest.StatusCondition(toBeNotReady(toolchainv1alpha1.MasterUserRecordProvisioningReason, "")))
	mur.Labels[toolchainv1alpha1.MasterUserRecordOwnerLabelKey] = userSignup.Name

	userAccount := uatest.NewUserAccountFromMur(mur,
		uatest.StatusCondition(toBeNotReady(toolchainv1alpha1.MasterUserRecordProvisioningReason, "")), uatest.ResourceVersion("123abc"))

	mur.Status.UserAccounts = []toolchainv1alpha1.UserAccountStatusEmbedded{{
		SyncIndex: "111aaa",
		Cluster: toolchainv1alpha1.Cluster{
			Name: test.MemberClusterName,
		},
		UserAccountStatus: userAccount.Status,
	}}
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

	uatest.Modify(userAccount, uatest.StatusCondition(toBeProvisioned()))

	t.Run("successful", func(t *testing.T) {
		// given
		hostClient := test.NewFakeClient(t, userSignup, mur, readyToolchainStatus, dummyNotification)
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

	t.Run("ProvisionedTime should not be updated when synced more than once", func(t *testing.T) {
		// given
		hostClient := test.NewFakeClient(t, userSignup, mur, readyToolchainStatus, dummyNotification)
		sync, memberClient := prepareSynchronizer(t, userAccount, mur, hostClient)
		provisionTime := metav1.NewTime(time.Now().Add(-time.Hour))
		sync.record.Status.ProvisionedTime = &provisionTime
		// when
		preSyncTime := metav1.Now()
		err := sync.synchronizeStatus()
		// then
		require.NoError(t, err)
		require.True(t, preSyncTime.Time.After(sync.record.Status.ProvisionedTime.Time), "the timestamp just before syncing should be after the ProvisionedTime because this is simulating the case where the record was already provisioned before")
		verifySyncMurStatusWithUserAccountStatus(t, memberClient, hostClient, userAccount, mur, toBeProvisioned(), toBeProvisionedNotificationCreated())
		assert.Equal(t, provisionTime.Time, sync.record.Status.ProvisionedTime.Time) // timestamp should be the same
		OnlyOneNotificationExists(t, hostClient, mur.Name, toolchainv1alpha1.NotificationTypeProvisioned, HasContext("RegistrationURL", "https://registration.crt-placeholder.com"))
	})

	t.Run("When notification was already created, but the update of status failed before, which means that the condition is not set", func(t *testing.T) {
		// given
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
		sync, memberClient := prepareSynchronizer(t, userAccount, mur, hostClient)

		// when
		preSyncTime := metav1.Now()
		err := sync.synchronizeStatus()

		// then
		require.NoError(t, err)
		require.True(t, preSyncTime.Time.Before(sync.record.Status.ProvisionedTime.Time), "the timestamp just before syncing should be before the ProvisionedTime")
		verifySyncMurStatusWithUserAccountStatus(t, memberClient, hostClient, userAccount, mur, toBeProvisioned(), toBeProvisionedNotificationCreated())
		OnlyOneNotificationExists(t, hostClient, mur.Name, toolchainv1alpha1.NotificationTypeProvisioned)
	})

	t.Run("no new notification created when condition is already set", func(t *testing.T) {
		// given
		murCopy := mur.DeepCopy()
		murCopy.Status.Conditions = append(murCopy.Status.Conditions, toBeProvisionedNotificationCreated())
		hostClient := test.NewFakeClient(t, murCopy, readyToolchainStatus)
		sync, memberClient := prepareSynchronizer(t, userAccount, murCopy, hostClient)

		// when
		preSyncTime := metav1.Now()
		err := sync.synchronizeStatus()

		// then
		require.NoError(t, err)
		require.True(t, preSyncTime.Time.Before(sync.record.Status.ProvisionedTime.Time), "the timestamp just before syncing should be before the ProvisionedTime")
		verifySyncMurStatusWithUserAccountStatus(t, memberClient, hostClient, userAccount, murCopy, toBeProvisioned(), toBeProvisionedNotificationCreated())
		AssertNoNotificationsExist(t, hostClient)
	})

	t.Run("failed on the host side when doing update", func(t *testing.T) {
		// given
		hostClient := test.NewFakeClient(t)
		hostClient.MockStatusUpdate = func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
			return fmt.Errorf("some error")
		}
		sync, _ := prepareSynchronizer(t, userAccount, mur, hostClient)

		// when
		err := sync.synchronizeStatus()

		// then
		require.Error(t, err)
		AssertNoNotificationsExist(t, hostClient)
	})

	t.Run("failed on the host side when creating notification", func(t *testing.T) {
		// given
		hostClient := test.NewFakeClient(t, mur.DeepCopy())
		hostClient.MockCreate = func(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
			return fmt.Errorf("some error")
		}
		sync, _ := prepareSynchronizer(t, userAccount, mur, hostClient)

		// when
		err := sync.synchronizeStatus()

		// then
		require.Error(t, err)
		AssertNoNotificationsExist(t, hostClient)
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
		memberClient.MockUpdate = func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
			return fmt.Errorf("unable to update user account %s", mur.Name)
		}
		err := murtest.Modify(mur, murtest.UserID("abc123"))
		require.NoError(t, err)
		hostClient := test.NewFakeClient(t, mur, readyToolchainStatus)

		sync := Synchronizer{
			record:            mur,
			hostClient:        hostClient,
			memberCluster:     newMemberCluster(memberClient),
			memberUserAcc:     userAcc,
			recordSpecUserAcc: mur.Spec.UserAccounts[0],
			logger:            l,
			scheme:            scheme,
		}

		// when
		err = sync.synchronizeSpec()

		// then
		require.Error(t, err)
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
		hostClient.MockStatusUpdate = func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
			return fmt.Errorf("unable to update MUR %s", provisionedMur.Name)
		}
		sync := Synchronizer{
			record:            provisionedMur,
			hostClient:        hostClient,
			memberCluster:     newMemberCluster(memberClient),
			memberUserAcc:     userAcc,
			recordSpecUserAcc: provisionedMur.Spec.UserAccounts[0],
			logger:            l,
			scheme:            scheme,
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
				SyncIndex: "somethingCrazy",
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
				SyncIndex: "111aaa",
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
						record:            mur,
						hostClient:        hostClient,
						memberCluster:     newMemberCluster(memberClient),
						memberUserAcc:     userAccount,
						recordSpecUserAcc: mur.Spec.UserAccounts[0],
						logger:            l,
						scheme:            scheme,
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
			record:            mur.DeepCopy(),
			hostClient:        hostClient,
			memberCluster:     newMemberCluster(memberClient),
			memberUserAcc:     userAccount,
			recordSpecUserAcc: mur.Spec.UserAccounts[0],
			logger:            l,
		}

		// when
		err := sync.synchronizeStatus()

		// then
		require.NoError(t, err)
		uatest.AssertThatUserAccount(t, "john", memberClient).
			Exists().
			MatchMasterUserRecord(mur, mur.Spec.UserAccounts[0].Spec).
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
			record:            mur,
			hostClient:        hostClient,
			memberCluster:     newMemberCluster(memberClient),
			memberUserAcc:     userAccount,
			recordSpecUserAcc: mur.Spec.UserAccounts[0],
			logger:            l,
		}

		// when
		err := sync.synchronizeStatus()

		// then
		require.NoError(t, err)
		uatest.AssertThatUserAccount(t, "john", memberClient).
			Exists().
			MatchMasterUserRecord(mur, mur.Spec.UserAccounts[0].Spec).
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
			record:            mur,
			hostClient:        hostClient,
			memberCluster:     newMemberCluster(memberClient),
			memberUserAcc:     userAccount,
			recordSpecUserAcc: mur.Spec.UserAccounts[0],
			logger:            l,
		}

		// when
		err := sync.synchronizeStatus()

		// then
		require.Error(t, err)
		uatest.AssertThatUserAccount(t, "john", memberClient).
			Exists().
			MatchMasterUserRecord(mur, mur.Spec.UserAccounts[0].Spec).
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

func prepareSynchronizer(t *testing.T, userAccount *toolchainv1alpha1.UserAccount, mur *toolchainv1alpha1.MasterUserRecord, hostClient *test.FakeClient) (Synchronizer, client.Client) {
	copiedMur := mur.DeepCopy()
	toolchainStatus := NewToolchainStatus(
		WithMember(test.MemberClusterName, WithRoutes("https://console.member-cluster/", "http://che-toolchain-che.member-cluster/", ToBeReady())))
	memberClient := test.NewFakeClient(t, userAccount, toolchainStatus)

	return Synchronizer{
		record:            copiedMur,
		hostClient:        hostClient,
		memberCluster:     newMemberCluster(memberClient),
		memberUserAcc:     userAccount,
		recordSpecUserAcc: copiedMur.Spec.UserAccounts[0],
		logger:            zap.New(zap.UseDevMode(true)),
		scheme:            apiScheme(t),
	}, memberClient
}

func verifySyncMurStatusWithUserAccountStatus(t *testing.T, memberClient, hostClient client.Client, userAccount *toolchainv1alpha1.UserAccount, mur *toolchainv1alpha1.MasterUserRecord, expMurCon ...toolchainv1alpha1.Condition) {
	userAccountCondition := userAccount.Status.Conditions[0]
	uatest.AssertThatUserAccount(t, "john", memberClient).
		Exists().
		MatchMasterUserRecord(mur, mur.Spec.UserAccounts[0].Spec).
		HasConditions(userAccountCondition)
	murtest.AssertThatMasterUserRecord(t, "john", hostClient).
		HasConditions(expMurCon...).
		HasStatusUserAccounts(test.MemberClusterName).
		AllUserAccountsHaveStatusSyncIndex("123abc").
		AllUserAccountsHaveCluster(toolchainv1alpha1.Cluster{
			Name:            test.MemberClusterName,
			APIEndpoint:     "https://api.member-cluster:6433",
			ConsoleURL:      "https://console.member-cluster/",
			CheDashboardURL: "http://che-toolchain-che.member-cluster/",
		}).
		AllUserAccountsHaveCondition(userAccountCondition)
}

func newMemberCluster(cl client.Client) *cluster.CachedToolchainCluster {
	return &cluster.CachedToolchainCluster{
		Config: &cluster.Config{
			Name:              test.MemberClusterName,
			APIEndpoint:       fmt.Sprintf("https://api.%s:6433", test.MemberClusterName),
			Type:              cluster.Member,
			OperatorNamespace: test.HostOperatorNs,
			OwnerClusterName:  test.HostClusterName,
		},
		Client: cl,
		ClusterStatus: &toolchainv1alpha1.ToolchainClusterStatus{
			Conditions: []toolchainv1alpha1.ToolchainClusterCondition{{
				Type:   toolchainv1alpha1.ToolchainClusterReady,
				Status: v1.ConditionTrue,
			}},
		},
	}
}
