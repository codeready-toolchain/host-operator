package spacebindingrequestmigration_test

import (
	"context"
	"fmt"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/controllers/spacebindingrequestmigration"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	"github.com/codeready-toolchain/host-operator/pkg/cluster"
	. "github.com/codeready-toolchain/host-operator/test"
	spacebindingtest "github.com/codeready-toolchain/host-operator/test/spacebinding"
	spacebindingrequesttest "github.com/codeready-toolchain/host-operator/test/spacebindingrequest"
	commoncluster "github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/codeready-toolchain/toolchain-common/pkg/test/masteruserrecord"
	spacetest "github.com/codeready-toolchain/toolchain-common/pkg/test/space"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestMigrateSpaceBindingToSBR(t *testing.T) {
	// given
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	err := apis.AddToScheme(scheme.Scheme)
	require.NoError(t, err)
	janeSpace := spacetest.NewSpace(test.HostOperatorNs, "jane",
		spacetest.WithSpecTargetCluster("member-1"),
		spacetest.WithStatusProvisionedNamespaces([]toolchainv1alpha1.SpaceNamespace{{
			Name: "jane-tenant",
			Type: "default",
		}}),
		spacetest.WithLabel(toolchainv1alpha1.SpaceCreatorLabelKey, "jane"),
	)

	janeMur := masteruserrecord.NewMasterUserRecord(t, "jane", masteruserrecord.WithLabel(toolchainv1alpha1.MasterUserRecordOwnerLabelKey, "jane"))
	sbForCreator := spacebindingtest.NewSpaceBinding(janeMur.Name, janeSpace.Name, "admin", janeMur.Name)
	// we have a user which was added to the space via sandbox-cli
	johnMur := masteruserrecord.NewMasterUserRecord(t, "john", masteruserrecord.WithLabel(toolchainv1alpha1.MasterUserRecordOwnerLabelKey, "john"))
	sbForJohn := spacebindingtest.NewSpaceBinding(johnMur.Name, janeSpace.Name, "admin", janeMur.GetName())
	t.Run("success", func(t *testing.T) {

		t.Run("create sbr for sb added via sandbox-cli", func(t *testing.T) {
			// given
			member1 := NewMemberClusterWithClient(test.NewFakeClient(t), "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t, janeSpace, janeMur, johnMur, sbForCreator, sbForJohn)
			ctrl := newReconciler(t, hostClient, member1)

			// when
			res, err := ctrl.Reconcile(context.TODO(), requestFor(sbForJohn))

			// then
			require.NoError(t, err)
			require.True(t, res.Requeue) // requeue should be triggered once SBR is created
			// spaceBindingRequest with expected name, namespace and spec should be created
			spacebindingrequesttest.AssertThatSpaceBindingRequest(t, "jane-tenant", johnMur.Name+"-admin", member1.Client).
				HasSpecSpaceRole("admin").
				HasSpecMasterUserRecord(johnMur.Name)
			// the migrated spacebinding is still there, it will be deleted at the next reconcile loop
			spacebindingtest.AssertThatSpaceBinding(t, test.HostOperatorNs, johnMur.Name, janeSpace.Name, hostClient).
				Exists()
			// the spacebinding for the space creator is still there
			spacebindingtest.AssertThatSpaceBinding(t, test.HostOperatorNs, janeMur.Name, janeSpace.Name, hostClient).
				Exists()

			t.Run("the next reconcile deletes the migrated spacebinding", func(t *testing.T) {
				// when
				res, err := ctrl.Reconcile(context.TODO(), requestFor(sbForJohn))

				// then
				require.NoError(t, err)
				require.False(t, res.Requeue) // no requeue this time
				// the migrated spacebinding was deleted
				spacebindingtest.AssertThatSpaceBinding(t, test.HostOperatorNs, johnMur.Name, janeSpace.Name, hostClient).
					DoesNotExist()
				// spaceBindingRequest with expected name, namespace and spec is still there
				spacebindingrequesttest.AssertThatSpaceBindingRequest(t, "jane-tenant", johnMur.Name+"-admin", member1.Client).
					HasSpecSpaceRole("admin").
					HasSpecMasterUserRecord(johnMur.Name)
				// the spacebinding for the space creator is still there
				spacebindingtest.AssertThatSpaceBinding(t, test.HostOperatorNs, janeMur.Name, janeSpace.Name, hostClient).
					Exists()
			})
		})

		t.Run("skip space creator spacebinding ", func(t *testing.T) {
			// given
			// we have the workspace creator spacebinding, it should not be migrated to SpaceBindingRequest
			member1 := NewMemberClusterWithClient(test.NewFakeClient(t), "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t, janeSpace, janeMur, sbForCreator)
			ctrl := newReconciler(t, hostClient, member1)

			// when
			_, err = ctrl.Reconcile(context.TODO(), requestFor(sbForCreator))

			// then
			require.NoError(t, err)
			// the spacebinding for the space creator is still there
			spacebindingtest.AssertThatSpaceBinding(t, test.HostOperatorNs, janeMur.Name, janeSpace.Name, hostClient).
				Exists()
			// the spaceBindingRequest wasn't created
			spacebindingrequesttest.AssertThatSpaceBindingRequest(t, "jane-tenant", janeMur.Name+"-admin", member1.Client).
				DoesNotExist()
		})

		t.Run("space creator name is different than mur name", func(t *testing.T) {
			// given
			batmanSpace := spacetest.NewSpace(test.HostOperatorNs, "batman",
				spacetest.WithStatusTargetCluster("member-1"),
				spacetest.WithStatusProvisionedNamespaces([]toolchainv1alpha1.SpaceNamespace{{
					Name: "batman-tenant",
					Type: "default",
				}}),
				spacetest.WithLabel(toolchainv1alpha1.SpaceCreatorLabelKey, "batman"),
			)
			// mur name differs from the space creator label
			// but the usersignup matches the space creator name
			batmanMur := masteruserrecord.NewMasterUserRecord(t, "batman123", masteruserrecord.WithLabel(toolchainv1alpha1.MasterUserRecordOwnerLabelKey, "batman"))
			sbForBatman := spacebindingtest.NewSpaceBinding(batmanMur.GetName(), batmanSpace.GetName(), "admin", "batman")
			member1 := NewMemberClusterWithClient(test.NewFakeClient(t), "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t, batmanSpace, batmanMur, sbForBatman)
			ctrl := newReconciler(t, hostClient, member1)

			// when
			_, err = ctrl.Reconcile(context.TODO(), requestFor(sbForBatman))

			// then
			require.NoError(t, err)
			// the spacebinding for the space creator is still there
			spacebindingtest.AssertThatSpaceBinding(t, test.HostOperatorNs, batmanMur.Name, batmanSpace.Name, hostClient).
				Exists()
			// the spaceBindingRequest wasn't created
			spacebindingrequesttest.AssertThatSpaceBindingRequest(t, "batman-tenant", batmanMur.Name+"-admin", member1.Client).
				DoesNotExist()
		})

		t.Run("space not found", func(t *testing.T) {
			// given
			member1 := NewMemberClusterWithClient(test.NewFakeClient(t), "member-1", corev1.ConditionTrue)
			// let's not load the space object
			hostClient := test.NewFakeClient(t, janeMur, johnMur, sbForCreator, sbForJohn)
			ctrl := newReconciler(t, hostClient, member1)

			// when
			_, err = ctrl.Reconcile(context.TODO(), requestFor(sbForJohn))

			// then
			require.NoError(t, err)
			// the spacebinding for the space creator is still there
			spacebindingtest.AssertThatSpaceBinding(t, test.HostOperatorNs, janeMur.Name, janeSpace.Name, hostClient).
				Exists()
			// the spacebinding for john user is still there
			spacebindingtest.AssertThatSpaceBinding(t, test.HostOperatorNs, johnMur.Name, janeSpace.Name, hostClient).
				Exists()
			// the spaceBindingRequest wasn't created
			spacebindingrequesttest.AssertThatSpaceBindingRequest(t, "jane-tenant", johnMur.Name+"-admin", member1.Client).
				DoesNotExist()
		})

		t.Run("mur not found", func(t *testing.T) {
			// given
			member1 := NewMemberClusterWithClient(test.NewFakeClient(t), "member-1", corev1.ConditionTrue)
			// let's not load the mur object
			hostClient := test.NewFakeClient(t, janeMur, janeSpace, sbForCreator, sbForJohn)
			ctrl := newReconciler(t, hostClient, member1)

			// when
			_, err = ctrl.Reconcile(context.TODO(), requestFor(sbForJohn))

			// then
			require.NoError(t, err)
			// the spacebinding for the space creator is still there
			spacebindingtest.AssertThatSpaceBinding(t, test.HostOperatorNs, janeMur.Name, janeSpace.Name, hostClient).
				Exists()
			// the spacebinding for john user is still there
			spacebindingtest.AssertThatSpaceBinding(t, test.HostOperatorNs, johnMur.Name, janeSpace.Name, hostClient).
				Exists()
			// the spaceBindingRequest wasn't created
			spacebindingrequesttest.AssertThatSpaceBindingRequest(t, "jane-tenant", johnMur.Name+"-admin", member1.Client).
				DoesNotExist()
		})

		t.Run("spacebinding has spacebindingrequest", func(t *testing.T) {
			// given
			// the spacebinding has a spacebindingrequest
			sbrForJohn := spacebindingrequesttest.NewSpaceBindingRequest("john-admin", "jane-tenant", spacebindingrequesttest.WithLabel(toolchainv1alpha1.SpaceCreatorLabelKey, "somevalue"))
			sbForJohnWithSBR := spacebindingtest.NewSpaceBinding(johnMur.Name, janeSpace.Name, "admin", janeMur.GetName(), spacebindingtest.WithSpaceBindingRequest(sbrForJohn))
			member1 := NewMemberClusterWithClient(test.NewFakeClient(t, sbrForJohn), "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t, janeMur, janeSpace, johnMur, sbForJohnWithSBR)
			ctrl := newReconciler(t, hostClient, member1)

			// when
			_, err = ctrl.Reconcile(context.TODO(), requestFor(sbForJohnWithSBR))

			// then
			require.NoError(t, err)
			// the spacebinding for john user is still there
			spacebindingtest.AssertThatSpaceBinding(t, test.HostOperatorNs, johnMur.Name, janeSpace.Name, hostClient).
				Exists()
			// the spaceBindingRequest is unchanged
			// no migration label as creator
			spacebindingrequesttest.AssertThatSpaceBindingRequest(t, "jane-tenant", johnMur.Name+"-admin", member1.Client).
				HasLabelWithValue(toolchainv1alpha1.SpaceCreatorLabelKey, "somevalue").
				Exists()
		})

		t.Run("spacebinding is being deleted", func(t *testing.T) {
			// given
			member1 := NewMemberClusterWithClient(test.NewFakeClient(t), "member-1", corev1.ConditionTrue)
			// the spacebinding is being deleted
			sbForJohn := spacebindingtest.NewSpaceBinding(johnMur.Name, janeSpace.Name, "admin", janeMur.GetName(), spacebindingtest.WithDeletionTimestamp())
			hostClient := test.NewFakeClient(t, janeMur, janeSpace, sbForCreator, johnMur, sbForJohn)
			ctrl := newReconciler(t, hostClient, member1)

			// when
			_, err = ctrl.Reconcile(context.TODO(), requestFor(sbForJohn))

			// then
			require.NoError(t, err)
			// the spacebinding for the space creator is still there
			spacebindingtest.AssertThatSpaceBinding(t, test.HostOperatorNs, janeMur.Name, janeSpace.Name, hostClient).
				Exists()
			// the spacebinding for john user is still there
			spacebindingtest.AssertThatSpaceBinding(t, test.HostOperatorNs, johnMur.Name, janeSpace.Name, hostClient).
				Exists()
			// the spaceBindingRequest wasn't created
			spacebindingrequesttest.AssertThatSpaceBindingRequest(t, "jane-tenant", johnMur.Name+"-admin", member1.Client).
				DoesNotExist()
		})
	})

	t.Run("error", func(t *testing.T) {
		t.Run("unable to get spacebinding", func(t *testing.T) {
			hostClient := test.NewFakeClient(t, sbForCreator)
			hostClient.MockGet = mockGetSpaceBindingFail(hostClient)
			ctrl := newReconciler(t, hostClient)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sbForCreator))

			// then
			// space binding request should not be there
			require.EqualError(t, err, "unable to get spacebinding: mock error")
		})

		t.Run("member cluster not found", func(t *testing.T) {
			spaceWithInvalidTargetCluster := spacetest.NewSpace(test.HostOperatorNs, "jane",
				spacetest.WithSpecTargetCluster("invalid"),
				spacetest.WithLabel(toolchainv1alpha1.SpaceCreatorLabelKey, "jane"),
			)
			sb := spacebindingtest.NewSpaceBinding(johnMur.Name, spaceWithInvalidTargetCluster.Name, "admin", janeMur.Name)
			hostClient := test.NewFakeClient(t, sb, spaceWithInvalidTargetCluster, johnMur)
			ctrl := newReconciler(t, hostClient)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sb))

			// then
			// space binding request should not be there
			require.EqualError(t, err, "unable to find member cluster: invalid")
		})

		t.Run("mur has no owner label", func(t *testing.T) {
			murWithNoOwnership := masteruserrecord.NewMasterUserRecord(t, "jane")
			sb := spacebindingtest.NewSpaceBinding(murWithNoOwnership.Name, janeSpace.Name, "admin", janeMur.Name)
			hostClient := test.NewFakeClient(t, sb, janeSpace, murWithNoOwnership)
			ctrl := newReconciler(t, hostClient)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sb))

			// then
			// space binding request should not be there
			require.EqualError(t, err, "mur has no MasterUserRecordOwnerLabelKey set")
		})

	})
}

func murWithOwnerReference(t *testing.T, name string) *toolchainv1alpha1.MasterUserRecord {
	janeMur := masteruserrecord.NewMasterUserRecord(t, name)
	janeMur.ObjectMeta.OwnerReferences = []v1.OwnerReference{
		{
			APIVersion: "toolchain.dev.openshift.com/v1alpha1",
			Kind:       "UserSignup",
			Name:       name,
		},
	}
	return janeMur
}

func newReconciler(t *testing.T, hostCl runtimeclient.Client, memberClusters ...*commoncluster.CachedToolchainCluster) *spacebindingrequestmigration.Reconciler {
	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)

	clusters := map[string]cluster.Cluster{}
	for _, c := range memberClusters {
		clusters[c.Name] = cluster.Cluster{
			Config: &commoncluster.Config{
				Type:              commoncluster.Member,
				OperatorNamespace: c.OperatorNamespace,
				OwnerClusterName:  test.MemberClusterName,
			},
			Client: c.Client,
		}
	}
	return &spacebindingrequestmigration.Reconciler{
		Client:         hostCl,
		Scheme:         s,
		Namespace:      test.HostOperatorNs,
		MemberClusters: clusters,
	}
}

func requestFor(s *toolchainv1alpha1.SpaceBinding) reconcile.Request {
	if s != nil {
		return reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: s.Namespace,
				Name:      s.Name,
			},
		}
	}
	return reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: test.HostOperatorNs,
			Name:      "unknown",
		},
	}
}

func mockGetSpaceBindingFail(cl runtimeclient.Client) func(ctx context.Context, key runtimeclient.ObjectKey, obj runtimeclient.Object, opts ...runtimeclient.GetOption) error {
	return func(ctx context.Context, key runtimeclient.ObjectKey, obj runtimeclient.Object, opts ...runtimeclient.GetOption) error {
		if _, ok := obj.(*toolchainv1alpha1.SpaceBinding); ok {
			return fmt.Errorf("mock error")
		}
		return cl.Get(ctx, key, obj, opts...)
	}
}
