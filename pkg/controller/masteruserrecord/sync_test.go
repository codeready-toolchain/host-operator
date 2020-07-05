package masteruserrecord

import (
	"context"
	"fmt"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	murtest "github.com/codeready-toolchain/toolchain-common/pkg/test/masteruserrecord"
	uatest "github.com/codeready-toolchain/toolchain-common/pkg/test/useraccount"
	"github.com/pkg/errors"

	routev1 "github.com/openshift/api/route/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gock "gopkg.in/h2non/gock.v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/kubefed/pkg/apis/core/common"
	"sigs.k8s.io/kubefed/pkg/apis/core/v1beta1"
)

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
		NSTemplateSet: toolchainv1alpha1.NSTemplateSetSpec{
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
		},
	}
	return record, recordSpecUserAcc, memberUserAcc
}

func TestSynchronizeSpec(t *testing.T) {
	// given
	apiScheme(t)
	mur := murtest.NewMasterUserRecord(t, "john", murtest.StatusCondition(toBeProvisioned()))

	userAccount := uatest.NewUserAccountFromMur(mur)

	murtest.ModifyUaInMur(mur, test.MemberClusterName, murtest.NsLimit("advanced"),
		murtest.TierName("admin"), murtest.Namespace("ide", "54321"))

	hostClient := test.NewFakeClient(t, mur)
	sync, memberClient := prepareSynchronizer(t, userAccount, mur, hostClient)

	// when
	err := sync.synchronizeSpec()

	// then
	require.NoError(t, err)
	uatest.AssertThatUserAccount(t, "john", memberClient).
		Exists().
		MatchMasterUserRecord(mur, mur.Spec.UserAccounts[0].Spec)

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
		hostClient := test.NewFakeClient(t, mur)
		sync, memberClient := prepareSynchronizer(t, userAccount, mur, hostClient)

		// when
		err := sync.synchronizeStatus()

		// then
		require.NoError(t, err)
		verifySyncMurStatusWithUserAccountStatus(t, memberClient, hostClient, userAccount, mur, toBeNotReady(toolchainv1alpha1.MasterUserRecordProvisioningReason, ""))
	})

	t.Run("failed on the host side", func(t *testing.T) {
		// given
		hostClient := test.NewFakeClient(t, mur)
		hostClient.MockStatusUpdate = func(ctx context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
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
		hostClient := test.NewFakeClient(t, mur)
		sync, memberClient := prepareSynchronizer(t, userAccount, mur, hostClient)

		// when
		err := sync.synchronizeStatus()

		// then
		require.NoError(t, err)
		verifySyncMurStatusWithUserAccountStatus(t, memberClient, hostClient, userAccount, mur, toBeNotReady(toolchainv1alpha1.MasterUserRecordUpdatingReason, ""))
	})

	t.Run("failed on the host side", func(t *testing.T) {
		// given
		hostClient := test.NewFakeClient(t, mur)
		hostClient.MockStatusUpdate = func(ctx context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
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
		hostClient := test.NewFakeClient(t, mur)
		sync, memberClient := prepareSynchronizer(t, userAccount, mur, hostClient)

		// when
		err := sync.synchronizeStatus()

		// then
		require.NoError(t, err)
		verifySyncMurStatusWithUserAccountStatus(t, memberClient, hostClient, userAccount, mur, toBeDisabled())
	})

	t.Run("failed on the host side", func(t *testing.T) {
		// given
		hostClient := test.NewFakeClient(t, mur.DeepCopy())
		hostClient.MockStatusUpdate = func(ctx context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
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

	t.Run("successful", func(t *testing.T) {
		// given
		hostClient := test.NewFakeClient(t, mur)
		sync, memberClient := prepareSynchronizer(t, userAccount, mur, hostClient)

		// when
		err := sync.synchronizeStatus()

		// then
		require.NoError(t, err)
		verifySyncMurStatusWithUserAccountStatus(t, memberClient, hostClient, userAccount, mur, toBeProvisioned(), toBeProvisionedNotificationCreated())
	})

	t.Run("failed on the host side when doing update", func(t *testing.T) {
		// given
		hostClient := test.NewFakeClient(t)
		hostClient.MockStatusUpdate = func(ctx context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
			return fmt.Errorf("some error")
		}
		sync, _ := prepareSynchronizer(t, userAccount, mur, hostClient)

		// when
		err := sync.synchronizeStatus()

		// then
		require.Error(t, err)
	})

	t.Run("failed on the host side when creating notification", func(t *testing.T) {
		// given
		hostClient := test.NewFakeClient(t, mur.DeepCopy())
		hostClient.MockCreate = func(ctx context.Context, obj runtime.Object, opts ...client.CreateOption) error {
			return fmt.Errorf("some error")
		}
		sync, _ := prepareSynchronizer(t, userAccount, mur, hostClient)

		// when
		err := sync.synchronizeStatus()

		// then
		require.Error(t, err)
	})
}

func TestSynchronizeUserAccountFailed(t *testing.T) {
	// given
	l := logf.ZapLogger(false)
	scheme := apiScheme(t)

	t.Run("spec synchronization of the UserAccount failed", func(t *testing.T) {
		// given
		mur := murtest.NewMasterUserRecord(t, "john")
		userAcc := uatest.NewUserAccountFromMur(mur)
		memberClient := test.NewFakeClient(t, userAcc, consoleRoute())
		memberClient.MockUpdate = func(ctx context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
			return fmt.Errorf("unable to update user account %s", mur.Name)
		}
		murtest.ModifyUaInMur(mur, test.MemberClusterName, murtest.TierName("admin"))
		hostClient := test.NewFakeClient(t, mur)

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
		err := sync.synchronizeSpec()

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
		memberClient := test.NewFakeClient(t, userAcc, consoleRoute())
		hostClient := test.NewFakeClient(t, provisionedMur)
		hostClient.MockStatusUpdate = func(ctx context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
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

		prepareSync := func() (Synchronizer, *test.FakeClient) {
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

			memberClient := test.NewFakeClient(t, userAccount)
			hostClient := test.NewFakeClient(t, mur)
			return Synchronizer{
				record:            mur,
				hostClient:        hostClient,
				memberCluster:     newMemberCluster(memberClient),
				memberUserAcc:     userAccount,
				recordSpecUserAcc: mur.Spec.UserAccounts[0],
				logger:            l,
				scheme:            scheme,
			}, memberClient
		}

		t.Run("when unable to get console route", func(t *testing.T) {
			t.Run("no OpenShift 3.11 console available", func(t *testing.T) {
				// given
				sync, _ := prepareSync()

				// when
				err := sync.synchronizeStatus()

				// then
				assert.EqualError(t, err, `unable to get web console route for cluster member-cluster: routes.route.openshift.io "console" not found`)
			})

			t.Run("OpenShift 3.11 console available", func(t *testing.T) {
				// given
				sync, _ := prepareSync()
				defer gock.OffAll()
				gock.InterceptClient(consoleClient)
				gock.
					New(fmt.Sprintf("https://api.%s:6433", test.MemberClusterName)).
					Get("console").
					Reply(200)

				// when
				err := sync.synchronizeStatus()
				require.NoError(t, err)

				// then
				require.Len(t, sync.record.Status.UserAccounts, 1)
				murtest.AssertThatMasterUserRecord(t, "john", hostClient).
					AllUserAccountsHaveCluster(toolchainv1alpha1.Cluster{
						Name:        test.MemberClusterName,
						APIEndpoint: "https://api.member-cluster:6433",
						ConsoleURL:  "https://api.member-cluster:6433/console",
					})
			})

			t.Run("OpenShift 3.11 console returns status other than 200", func(t *testing.T) {
				// given
				sync, _ := prepareSync()
				defer gock.OffAll()
				gock.InterceptClient(consoleClient)
				gock.
					New(fmt.Sprintf("https://api.%s:6433", test.MemberClusterName)).
					Get("console").
					Reply(404)

				// when
				err := sync.synchronizeStatus()

				// then
				assert.EqualError(t, err, `unable to get web console route for cluster member-cluster: routes.route.openshift.io "console" not found`)
			})

			t.Run("get route fails with error other than 404", func(t *testing.T) {
				// given
				sync, memberClient := prepareSync()
				memberClient.MockGet = func(ctx context.Context, key client.ObjectKey, obj runtime.Object) error {
					if key.Name == "console" {
						return errors.New("something went wrong")
					}
					return memberClient.Client.Get(ctx, key, obj)
				}

				// when
				err := sync.synchronizeStatus()

				// then
				assert.EqualError(t, err, "something went wrong")
			})
		})

		t.Run("when unable to get che route", func(t *testing.T) {
			t.Run("get route fails with error other than 404", func(t *testing.T) {
				// given
				sync, memberClient := prepareSync()
				memberClient.MockGet = func(ctx context.Context, key client.ObjectKey, obj runtime.Object) error {
					if key.Name == "che" {
						return errors.New("something went wrong")
					}
					if key.Name == "console" {
						return nil
					}
					return memberClient.Client.Get(ctx, key, obj)
				}

				// when
				err := sync.synchronizeStatus()

				// then
				assert.EqualError(t, err, "something went wrong")
			})
		})
	})
}

func TestCheURL(t *testing.T) {
	// given
	l := logf.ZapLogger(true)
	logf.SetLogger(l)
	apiScheme(t)

	testConsoleURL := func(cheRoute *routev1.Route, expectedConsoleURL string) {
		// given
		mur := murtest.NewMasterUserRecord(t, "john",
			murtest.StatusCondition(toBeNotReady(toolchainv1alpha1.MasterUserRecordProvisioningReason, "")))
		userAccount := uatest.NewUserAccountFromMur(mur,
			uatest.StatusCondition(toBeNotReady("Provisioning", "")), uatest.ResourceVersion("123abc"))
		condition := userAccount.Status.Conditions[0]

		var memberClient *test.FakeClient
		if cheRoute != nil {
			memberClient = test.NewFakeClient(t, userAccount, consoleRoute(), cheRoute)
		} else {
			memberClient = test.NewFakeClient(t, userAccount, consoleRoute())
		}
		hostClient := test.NewFakeClient(t, mur)
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
		require.NoError(t, err)

		// then
		uatest.AssertThatUserAccount(t, "john", memberClient).
			Exists().
			MatchMasterUserRecord(mur, mur.Spec.UserAccounts[0].Spec).
			HasConditions(condition)
		murtest.AssertThatMasterUserRecord(t, "john", hostClient).
			AllUserAccountsHaveCluster(toolchainv1alpha1.Cluster{
				Name:            test.MemberClusterName,
				APIEndpoint:     "https://api.member-cluster:6433",
				ConsoleURL:      "https://console.member-cluster/",
				CheDashboardURL: expectedConsoleURL,
			}).
			AllUserAccountsHaveCondition(condition)
	}

	t.Run("tls enabled", func(t *testing.T) {
		testConsoleURL(cheRoute(true), "https://che-toolchain-che.member-cluster/")
	})

	t.Run("tls disabled", func(t *testing.T) {
		testConsoleURL(cheRoute(false), "http://che-toolchain-che.member-cluster/")
	})

	t.Run("no route", func(t *testing.T) {
		testConsoleURL(nil, "")
	})
}

func prepareSynchronizer(t *testing.T, userAccount *toolchainv1alpha1.UserAccount, mur *toolchainv1alpha1.MasterUserRecord, hostClient *test.FakeClient) (Synchronizer, client.Client) {
	copiedMur := mur.DeepCopy()
	memberClient := test.NewFakeClient(t, userAccount, consoleRoute(), cheRoute(false))
	return Synchronizer{
		record:            copiedMur,
		hostClient:        hostClient,
		memberCluster:     newMemberCluster(memberClient),
		memberUserAcc:     userAccount,
		recordSpecUserAcc: copiedMur.Spec.UserAccounts[0],
		logger:            logf.ZapLogger(true),
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

func newMemberCluster(cl client.Client) *cluster.FedCluster {
	return &cluster.FedCluster{
		Name:              test.MemberClusterName,
		APIEndpoint:       fmt.Sprintf("https://api.%s:6433", test.MemberClusterName),
		Client:            cl,
		Type:              cluster.Member,
		OperatorNamespace: test.HostOperatorNs,
		OwnerClusterName:  test.HostClusterName,
		ClusterStatus: &v1beta1.KubeFedClusterStatus{
			Conditions: []v1beta1.ClusterCondition{{
				Type:   common.ClusterReady,
				Status: v1.ConditionTrue,
			}},
		},
	}
}

func consoleRoute() *routev1.Route {
	return &routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "console",
			Namespace: "openshift-console",
		},
		Spec: routev1.RouteSpec{Host: fmt.Sprintf("console.%s", test.MemberClusterName)},
	}
}

func cheRoute(tls bool) *routev1.Route {
	r := &routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "che",
			Namespace: "toolchain-che",
		},
		Spec: routev1.RouteSpec{Host: fmt.Sprintf("che-toolchain-che.%s", test.MemberClusterName)},
	}
	if tls {
		r.Spec.TLS = &routev1.TLSConfig{Termination: "edge"}
	}
	return r
}
