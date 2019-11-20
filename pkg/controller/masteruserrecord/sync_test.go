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
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/kubefed/pkg/apis/core/common"
	"sigs.k8s.io/kubefed/pkg/apis/core/v1beta1"
)

func TestSynchronizeSpec(t *testing.T) {
	// given
	l := logf.ZapLogger(true)
	logf.SetLogger(l)
	mur := murtest.NewMasterUserRecord("john", murtest.StatusCondition(toBeProvisioned()))

	userAccount := uatest.NewUserAccountFromMur(mur)

	murtest.ModifyUaInMur(mur, test.MemberClusterName, murtest.NsLimit("advanced"),
		murtest.TierName("admin"), murtest.Namespace("ide", "54321"))

	memberClient := test.NewFakeClient(t, userAccount, consoleRoute())
	hostClient := test.NewFakeClient(t, mur)

	sync := Synchronizer{
		record:            mur,
		hostClient:        hostClient,
		memberCluster:     newMemberCluster(memberClient),
		memberUserAcc:     userAccount,
		recordSpecUserAcc: mur.Spec.UserAccounts[0],
		log:               l,
	}

	// when
	err := sync.synchronizeSpec()

	// then
	require.NoError(t, err)

	uatest.AssertThatUserAccount(t, "john", memberClient).
		Exists().
		HasSpec(mur.Spec.UserAccounts[0].Spec)

	murtest.AssertThatMasterUserRecord(t, "john", hostClient).
		HasConditions(toBeNotReady(updatingReason, ""))
}

func TestSynchronizeStatus(t *testing.T) {
	// given
	logf.SetLogger(logf.ZapLogger(true))
	apiScheme(t)

	mur := murtest.NewMasterUserRecord("john",
		murtest.StatusCondition(toBeNotReady(provisioningReason, "")))

	userAccount := uatest.NewUserAccountFromMur(mur,
		uatest.StatusCondition(toBeNotReady("Provisioning", "")), uatest.ResourceVersion("123abc"))

	// when and then
	testSyncMurStatusWithUserAccountStatus(t, userAccount, mur, toBeNotReady(provisioningReason, ""))
}

func TestSyncMurStatusWithUserAccountStatusWhenUpdated(t *testing.T) {
	// given
	logf.SetLogger(logf.ZapLogger(true))
	apiScheme(t)

	mur := murtest.NewMasterUserRecord("john",
		murtest.StatusCondition(toBeNotReady(updatingReason, "")))

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

	// when and then
	testSyncMurStatusWithUserAccountStatus(t, userAccount, mur, toBeNotReady(updatingReason, ""))
}

func TestSyncMurStatusWithUserAccountStatusWhenCompleted(t *testing.T) {
	// given
	logf.SetLogger(logf.ZapLogger(true))
	apiScheme(t)

	mur := murtest.NewMasterUserRecord("john",
		murtest.StatusCondition(toBeNotReady(provisioningReason, "")))

	userAccount := uatest.NewUserAccountFromMur(mur,
		uatest.StatusCondition(toBeNotReady(provisioningReason, "")), uatest.ResourceVersion("123abc"))

	mur.Status.UserAccounts = []toolchainv1alpha1.UserAccountStatusEmbedded{{
		SyncIndex: "111aaa",
		Cluster: toolchainv1alpha1.Cluster{
			Name: test.MemberClusterName,
		},
		UserAccountStatus: userAccount.Status,
	}}

	uatest.Modify(userAccount, uatest.StatusCondition(toBeProvisioned()))

	// when and then
	testSyncMurStatusWithUserAccountStatus(t, userAccount, mur, toBeProvisioned())
}

func TestSynchronizeUserAccountFailed(t *testing.T) {
	// given
	l := logf.ZapLogger(true)
	logf.SetLogger(l)

	t.Run("spec synchronization of the UserAccount failed", func(t *testing.T) {
		// given
		mur := murtest.NewMasterUserRecord("john")
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
			log:               l,
		}

		// when
		err := sync.synchronizeSpec()

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unable to update user account john")
	})

	t.Run("status synchronization of the UserAccount & MasterUserRecord failed", func(t *testing.T) {
		// given
		provisionedMur := murtest.NewMasterUserRecord("john",
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
			log:               l,
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

		t.Run("when unable to get console route", func(t *testing.T) {
			prepareSync := func() (Synchronizer, *test.FakeClient) {
				mur := murtest.NewMasterUserRecord("john",
					murtest.StatusCondition(toBeNotReady(provisioningReason, "")))

				userAccount := uatest.NewUserAccountFromMur(mur,
					uatest.StatusCondition(toBeNotReady(provisioningReason, "")), uatest.ResourceVersion("123abc"))

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
					log:               l,
				}, memberClient
			}

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
	})
}

func testSyncMurStatusWithUserAccountStatus(t *testing.T, userAccount *toolchainv1alpha1.UserAccount, mur *toolchainv1alpha1.MasterUserRecord, expMurCon toolchainv1alpha1.Condition) {
	l := logf.ZapLogger(true)
	condition := userAccount.Status.Conditions[0]

	memberClient := test.NewFakeClient(t, userAccount, consoleRoute())
	hostClient := test.NewFakeClient(t, mur)
	sync := Synchronizer{
		record:            mur,
		hostClient:        hostClient,
		memberCluster:     newMemberCluster(memberClient),
		memberUserAcc:     userAccount,
		recordSpecUserAcc: mur.Spec.UserAccounts[0],
		log:               l,
	}

	// when
	err := sync.synchronizeStatus()

	// then
	require.NoError(t, err)

	uatest.AssertThatUserAccount(t, "john", memberClient).
		Exists().
		HasSpec(mur.Spec.UserAccounts[0].Spec).
		HasConditions(condition)
	murtest.AssertThatMasterUserRecord(t, "john", hostClient).
		HasConditions(expMurCon).
		HasStatusUserAccounts(test.MemberClusterName).
		AllUserAccountsHaveStatusSyncIndex("123abc").
		AllUserAccountsHaveCluster(toolchainv1alpha1.Cluster{
			Name:        test.MemberClusterName,
			APIEndpoint: "https://api.member-cluster:6433",
			ConsoleURL:  "https://console.member-cluster/",
		}).
		AllUserAccountsHaveCondition(condition)
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
