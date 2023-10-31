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
	tiertest "github.com/codeready-toolchain/host-operator/test/nstemplatetier"
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
	base1nsTier := tiertest.Base1nsTier(t, tiertest.CurrentBase1nsTemplates)
	janeSpace := spacetest.NewSpace(test.HostOperatorNs, "jane",
		spacetest.WithStatusTargetCluster("member-1"),
		spacetest.WithStatusProvisionedNamespaces([]toolchainv1alpha1.SpaceNamespace{{
			Name: "jane-tenant",
			Type: "default",
		}}),
	)
	janeMur := masteruserrecord.NewMasterUserRecord(t, "jane")
	janeMur.ObjectMeta.OwnerReferences = []v1.OwnerReference{
		{
			APIVersion: "toolchain.dev.openshift.com/v1alpha1",
			Kind:       "UserSignup",
			Name:       "jane",
		},
	}
	t.Run("success", func(t *testing.T) {

		t.Run("create sbr for sb added via sandbox-cli", func(t *testing.T) {
			// given
			// we have the workspace creator spacebinding, it should not be migrated to SpaceBindingRequest
			member1 := NewMemberClusterWithClient(test.NewFakeClient(t), "member-1", corev1.ConditionTrue)
			sbForCreator := spacebindingtest.NewSpaceBinding(janeMur.Name, janeSpace.Name, "admin", janeMur.Name)
			// we have a user which was added to the space via sandbox-cli
			johnMur := masteruserrecord.NewMasterUserRecord(t, "john")
			johnMur.ObjectMeta.OwnerReferences = []v1.OwnerReference{
				{
					APIVersion: "toolchain.dev.openshift.com/v1alpha1",
					Kind:       "UserSignup",
					Name:       "john",
				},
			}
			sbForJohn := spacebindingtest.NewSpaceBinding(johnMur.Name, janeSpace.Name, "admin", janeMur.GetName())
			hostClient := test.NewFakeClient(t, janeSpace, janeMur, johnMur, base1nsTier, sbForCreator, sbForJohn)
			ctrl := newReconciler(t, hostClient, member1)

			// when
			_, err = ctrl.Reconcile(context.TODO(), requestFor(sbForJohn))

			// then
			require.NoError(t, err)
			// spaceBindingRequest with expected name, namespace and spec
			spacebindingrequesttest.AssertThatSpaceBindingRequest(t, "jane-tenant", johnMur.Name+"-admin", member1.Client).
				HasSpecSpaceRole("admin").
				HasSpecMasterUserRecord(johnMur.Name)
			// the spacebinding was deleted
			spacebindingtest.AssertThatSpaceBinding(t, test.HostOperatorNs, johnMur.Name, janeSpace.Name, hostClient).
				DoesNotExist()
			// the spacebinding for the space creator is still there
			spacebindingtest.AssertThatSpaceBinding(t, test.HostOperatorNs, janeMur.Name, janeSpace.Name, hostClient).
				Exists()
		})

		t.Run("skip space creator spacebinding ", func(t *testing.T) {
			// given
			// we have the workspace creator spacebinding, it should not be migrated to SpaceBindingRequest
			member1 := NewMemberClusterWithClient(test.NewFakeClient(t), "member-1", corev1.ConditionTrue)
			sbForCreator := spacebindingtest.NewSpaceBinding(janeMur.Name, janeSpace.Name, "admin", janeMur.Name)
			hostClient := test.NewFakeClient(t, janeSpace, janeMur, base1nsTier, sbForCreator)
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

		t.Run("space not found", func(t *testing.T) {
			// given
			member1 := NewMemberClusterWithClient(test.NewFakeClient(t), "member-1", corev1.ConditionTrue)
			sbForCreator := spacebindingtest.NewSpaceBinding(janeMur.Name, janeSpace.Name, "admin", janeMur.Name)
			// we have a user which was added to the space via sandbox-cli
			johnMur := masteruserrecord.NewMasterUserRecord(t, "john")
			johnMur.ObjectMeta.OwnerReferences = []v1.OwnerReference{
				{
					APIVersion: "toolchain.dev.openshift.com/v1alpha1",
					Kind:       "UserSignup",
					Name:       "john",
				},
			}
			sbForJohn := spacebindingtest.NewSpaceBinding(johnMur.Name, janeSpace.Name, "admin", janeMur.GetName())
			// let's not load the space object
			hostClient := test.NewFakeClient(t, janeMur, johnMur, base1nsTier, sbForCreator, sbForJohn)
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
			sbForCreator := spacebindingtest.NewSpaceBinding(janeMur.Name, janeSpace.Name, "admin", janeMur.Name)
			// we have a user which was added to the space via sandbox-cli
			johnMur := masteruserrecord.NewMasterUserRecord(t, "john")
			johnMur.ObjectMeta.OwnerReferences = []v1.OwnerReference{
				{
					APIVersion: "toolchain.dev.openshift.com/v1alpha1",
					Kind:       "UserSignup",
					Name:       "john",
				},
			}
			sbForJohn := spacebindingtest.NewSpaceBinding(johnMur.Name, janeSpace.Name, "admin", janeMur.GetName())
			// let's not load the mur object
			hostClient := test.NewFakeClient(t, janeMur, janeSpace, base1nsTier, sbForCreator, sbForJohn)
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

		t.Run("spacebinding is being deleted", func(t *testing.T) {
			// given
			member1 := NewMemberClusterWithClient(test.NewFakeClient(t), "member-1", corev1.ConditionTrue)
			sbForCreator := spacebindingtest.NewSpaceBinding(janeMur.Name, janeSpace.Name, "admin", janeMur.Name)
			// we have a user which was added to the space via sandbox-cli
			johnMur := masteruserrecord.NewMasterUserRecord(t, "john")
			johnMur.ObjectMeta.OwnerReferences = []v1.OwnerReference{
				{
					APIVersion: "toolchain.dev.openshift.com/v1alpha1",
					Kind:       "UserSignup",
					Name:       "john",
				},
			}
			// the spacebinding is being deleted
			sbForJohn := spacebindingtest.NewSpaceBinding(johnMur.Name, janeSpace.Name, "admin", janeMur.GetName(), spacebindingtest.WithDeletionTimestamp())
			// let's not load the mur object
			hostClient := test.NewFakeClient(t, janeMur, janeSpace, base1nsTier, sbForCreator, sbForJohn)
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
			// we have a user which was added to the space via sandbox-cli
			johnMur := masteruserrecord.NewMasterUserRecord(t, "john")
			johnMur.ObjectMeta.OwnerReferences = []v1.OwnerReference{
				{
					APIVersion: "toolchain.dev.openshift.com/v1alpha1",
					Kind:       "UserSignup",
					Name:       "john",
				},
			}
			// the spacebinding has a spacebindingrequest
			sbrForJohn := spacebindingrequesttest.NewSpaceBindingRequest("john-admin", "jane-tenant", spacebindingrequesttest.WithLabel(toolchainv1alpha1.SpaceCreatorLabelKey, "somevalue"))
			sbForJohn := spacebindingtest.NewSpaceBinding(johnMur.Name, janeSpace.Name, "admin", janeMur.GetName(), spacebindingtest.WithSpaceBindingRequest(sbrForJohn))
			// let's not load the mur object
			member1 := NewMemberClusterWithClient(test.NewFakeClient(t, sbrForJohn), "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t, janeMur, janeSpace, base1nsTier, sbForJohn)
			ctrl := newReconciler(t, hostClient, member1)

			// when
			_, err = ctrl.Reconcile(context.TODO(), requestFor(sbForJohn))

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
	})

	t.Run("error", func(t *testing.T) {
		t.Run("unable to get spacebinding", func(t *testing.T) {
			sbForCreator := spacebindingtest.NewSpaceBinding(janeMur.Name, janeSpace.Name, "admin", janeMur.Name)
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
				spacetest.WithStatusTargetCluster("invalid"),
			)
			johnMur := masteruserrecord.NewMasterUserRecord(t, "john")
			johnMur.ObjectMeta.OwnerReferences = []v1.OwnerReference{
				{
					APIVersion: "toolchain.dev.openshift.com/v1alpha1",
					Kind:       "UserSignup",
					Name:       "john",
				},
			}
			sb := spacebindingtest.NewSpaceBinding(johnMur.Name, spaceWithInvalidTargetCluster.Name, "admin", janeMur.Name)
			hostClient := test.NewFakeClient(t, sb, spaceWithInvalidTargetCluster, johnMur)
			ctrl := newReconciler(t, hostClient)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sb))

			// then
			// space binding request should not be there
			require.EqualError(t, err, "unable to find member cluster: invalid")
		})
	})
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
