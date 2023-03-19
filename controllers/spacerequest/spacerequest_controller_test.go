package spacerequest_test

import (
	"context"
	"fmt"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/controllers/spacerequest"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	"github.com/codeready-toolchain/host-operator/pkg/cluster"
	. "github.com/codeready-toolchain/host-operator/test"
	tiertest "github.com/codeready-toolchain/host-operator/test/nstemplatetier"
	spacetest "github.com/codeready-toolchain/host-operator/test/space"
	spacerequesttest "github.com/codeready-toolchain/host-operator/test/spacerequest"
	commoncluster "github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestCreateSpaceRequest(t *testing.T) {
	// given
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	err := apis.AddToScheme(scheme.Scheme)
	require.NoError(t, err)
	appstudioTier := tiertest.AppStudioTier(t, tiertest.AppStudioTemplates)
	t.Run("success", func(t *testing.T) {
		// given
		srNamespace := newNamespace("jane")
		srClusterRoles := []string{commoncluster.RoleLabel(commoncluster.Tenant)}
		sr := spacerequesttest.NewSpaceRequest("jane", srNamespace.GetName(),
			spacerequesttest.WithTierName("appstudio"),
			spacerequesttest.WithTargetClusterRoles(srClusterRoles))
		parentSpace := spacetest.NewSpace("jane")
		member1 := NewMemberClusterWithClient(test.NewFakeClient(t, sr, srNamespace), "member-1", corev1.ConditionTrue)
		hostClient := test.NewFakeClient(t, appstudioTier, parentSpace)
		ctrl := newReconciler(hostClient, member1)

		t.Run("space doesn't exists it should be created", func(t *testing.T) {
			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sr))

			// then
			require.NoError(t, err)
			// spacerequest exists with expected cluster roles and finalizer
			spacerequesttest.AssertThatSpaceRequest(t, srNamespace.Name, sr.GetName(), member1.Client).
				Exists().
				HasSpecTargetClusterRoles(srClusterRoles).
				HasFinalizer()
		})

		t.Run("space exists but is not ready yet", func(t *testing.T) {
			// given
			subSpace := spacetest.NewSpace("jane-subs",
				spacetest.WithLabel(toolchainv1alpha1.SpaceRequestLabelKey, sr.GetName()),               // subSpace was created from spaceRequest
				spacetest.WithLabel(toolchainv1alpha1.SpaceRequestNamespaceLabelKey, sr.GetNamespace()), // subSpace was created from spaceRequest
				spacetest.WithCondition(spacetest.Provisioning()),
				spacetest.WithSpecParentSpace("jane"),
				spacetest.WithSpecTargetClusterRoles(srClusterRoles),
				spacetest.WithTierName(sr.Spec.TierName))
			hostClient := test.NewFakeClient(t, appstudioTier, parentSpace, subSpace)
			ctrl := newReconciler(hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sr))

			// then
			require.NoError(t, err)
			// spacerequest exists with expected cluster roles and finalizer
			spacerequesttest.AssertThatSpaceRequest(t, srNamespace.Name, sr.GetName(), member1.Client).
				Exists().
				HasSpecTargetClusterRoles(srClusterRoles).
				HasConditions(spacetest.Provisioning()). // condition is reflected from space status
				HasFinalizer()
			// a subspace is created with the tierName and cluster roles from the spacerequest
			spacetest.AssertThatSpace(t, test.HostOperatorNs, "jane-subs", hostClient).
				Exists().
				HasSpecTargetClusterRoles(srClusterRoles).
				HasConditions(spacetest.Provisioning()).
				HasTier(sr.Spec.TierName).
				HasParentSpace("jane") // the parent space is set as expected
		})

		t.Run("space is provisioned", func(t *testing.T) {
			// given
			subSpace := spacetest.NewSpace("jane-subs",
				spacetest.WithLabel(toolchainv1alpha1.SpaceRequestLabelKey, sr.GetName()),               // subSpace was created from spaceRequest
				spacetest.WithLabel(toolchainv1alpha1.SpaceRequestNamespaceLabelKey, sr.GetNamespace()), // subSpace was created from spaceRequest
				spacetest.WithCondition(spacetest.Ready()),
				spacetest.WithSpecParentSpace("jane"),
				spacetest.WithSpecTargetClusterRoles(srClusterRoles),
				spacetest.WithTierName(sr.Spec.TierName))
			hostClient := test.NewFakeClient(t, appstudioTier, parentSpace, subSpace)
			ctrl := newReconciler(hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sr))

			// then
			require.NoError(t, err)
			// spacerequest exists with expected cluster roles and finalizer
			spacerequesttest.AssertThatSpaceRequest(t, srNamespace.Name, sr.GetName(), member1.Client).
				Exists().
				HasSpecTargetClusterRoles(srClusterRoles).
				HasConditions(spacetest.Ready()). // condition is reflected from space status
				HasFinalizer()
			// a subspace is created with the tierName and cluster roles from the spacerequest
			spacetest.AssertThatSpace(t, test.HostOperatorNs, "jane-subs", hostClient).
				Exists().
				HasSpecTargetClusterRoles(srClusterRoles).
				HasConditions(spacetest.Ready()).
				HasTier(sr.Spec.TierName).
				HasParentSpace("jane") // the parent space is set as expected
		})

		t.Run("spacerequest created on member2", func(t *testing.T) {
			// when
			member1 := NewMemberClusterWithClient(test.NewFakeClient(t), "member-1", corev1.ConditionTrue)
			member2 := NewMemberClusterWithClient(test.NewFakeClient(t, sr, srNamespace), "member-2", corev1.ConditionTrue) // space request and namespace are on member2
			hostClient := test.NewFakeClient(t, appstudioTier, parentSpace)
			ctrl := newReconciler(hostClient, member1, member2)
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sr))

			// then
			require.NoError(t, err)
			// spacerequest exists with expected cluster roles and finalizer
			spacerequesttest.AssertThatSpaceRequest(t, srNamespace.Name, sr.GetName(), member2.Client).
				Exists().
				HasSpecTargetClusterRoles(srClusterRoles).
				HasFinalizer()
		})
	})

	t.Run("failure", func(t *testing.T) {
		t.Run("unable to find SpaceRequest", func(t *testing.T) {
			member1 := NewMemberClusterWithClient(test.NewFakeClient(t), "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t, appstudioTier)
			ctrl := newReconciler(hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(nil))

			// then
			// space request should not be there
			require.EqualError(t, err, "unable to find SpaceRequest: spacerequests.toolchain.dev.openshift.com \"unknown\" not found")
		})
		t.Run("unable to get SpaceRequest", func(t *testing.T) {
			member1Client := test.NewFakeClient(t)
			member1Client.MockGet = mockGetSpaceRequestFail(member1Client)
			member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t, appstudioTier)
			ctrl := newReconciler(hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(nil))

			// then
			// space request should not be there
			require.EqualError(t, err, "unable to get the current SpaceRequest: mock error")
		})

		t.Run("unable to list subSpaces", func(t *testing.T) {
			parentSpace := spacetest.NewSpace("jane")
			srNamespace := newNamespace("jane")
			srClusterRoles := []string{commoncluster.RoleLabel(commoncluster.Tenant)}
			sr := spacerequesttest.NewSpaceRequest("jane", srNamespace.GetName(),
				spacerequesttest.WithTierName("appstudio"),
				spacerequesttest.WithTargetClusterRoles(srClusterRoles))
			member1Client := test.NewFakeClient(t, sr, srNamespace)
			member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t, appstudioTier, parentSpace)
			hostClient.MockList = func(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
				if _, ok := list.(*toolchainv1alpha1.SpaceList); ok {
					return fmt.Errorf("mock error")
				}
				return member1Client.List(ctx, list, opts...)
			}
			ctrl := newReconciler(hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sr))

			// then
			require.EqualError(t, err, "attempt to list Spaces associated with spaceRequest jane in namespace jane-tenant failed: mock error")
		})

		t.Run("error while adding finalizer", func(t *testing.T) {
			srNamespace := newNamespace("jane")
			srClusterRoles := []string{commoncluster.RoleLabel(commoncluster.Tenant)}
			sr := spacerequesttest.NewSpaceRequest("jane", srNamespace.GetName(),
				spacerequesttest.WithTierName("appstudio"),
				spacerequesttest.WithTargetClusterRoles(srClusterRoles))
			member1Client := test.NewFakeClient(t, sr)
			member1Client.MockUpdate = mockUpdateSpaceRequestFail(member1Client)
			member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t, appstudioTier)
			ctrl := newReconciler(hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sr))

			// then
			require.EqualError(t, err, "error while adding finalizer: mock error")
		})

		t.Run("unable to get parent space name", func(t *testing.T) {
			// given
			srClusterRoles := []string{commoncluster.RoleLabel(commoncluster.Tenant)}
			sr := spacerequesttest.NewSpaceRequest("jane", "unknown-namespace",
				spacerequesttest.WithTierName("appstudio"),
				spacerequesttest.WithTargetClusterRoles(srClusterRoles))
			member1Client := test.NewFakeClient(t, sr)
			member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t, appstudioTier)
			ctrl := newReconciler(hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sr))

			// then
			require.EqualError(t, err, "unable to get namespace: namespaces \"unknown-namespace\" not found")
		})

		t.Run("unable to find owner label in spaceRequest namespace", func(t *testing.T) {
			// given
			srNamespace := newNamespace("noowner")
			srClusterRoles := []string{commoncluster.RoleLabel(commoncluster.Tenant)}
			sr := spacerequesttest.NewSpaceRequest("jane", srNamespace.GetName(),
				spacerequesttest.WithTierName("appstudio"),
				spacerequesttest.WithTargetClusterRoles(srClusterRoles))
			member1Client := test.NewFakeClient(t, sr, srNamespace)
			member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t, appstudioTier)
			ctrl := newReconciler(hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sr))

			// then
			require.EqualError(t, err, "unable to find owner label toolchain.dev.openshift.com/owner on namespace noowner-tenant")
		})

		t.Run("unable to update space since it's being deleted", func(t *testing.T) {
			// given
			srNamespace := newNamespace("jane")
			srClusterRoles := []string{commoncluster.RoleLabel(commoncluster.Tenant)}
			sr := spacerequesttest.NewSpaceRequest("jane", srNamespace.GetName(),
				spacerequesttest.WithTierName("appstudio"),
				spacerequesttest.WithTargetClusterRoles(srClusterRoles))
			member1 := NewMemberClusterWithClient(test.NewFakeClient(t, sr, srNamespace), "member-1", corev1.ConditionTrue)
			parentSpace := spacetest.NewSpace("jane")
			subSpace := spacetest.NewSpace("jane-subs",
				spacetest.WithLabel(toolchainv1alpha1.SpaceRequestLabelKey, sr.GetName()),               // subSpace was created from spaceRequest
				spacetest.WithLabel(toolchainv1alpha1.SpaceRequestNamespaceLabelKey, sr.GetNamespace()), // subSpace was created from spaceRequest
				spacetest.WithDeletionTimestamp(),                                                       // space is being deleted ...
				spacetest.WithSpecParentSpace("jane"))
			hostClient := test.NewFakeClient(t, appstudioTier, parentSpace, subSpace)
			ctrl := newReconciler(hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sr))

			// then
			require.EqualError(t, err, "cannot update subSpace because it is currently being deleted")
		})

		t.Run("unable to find tierName", func(t *testing.T) {
			// given
			srNamespace := newNamespace("jane")
			srClusterRoles := []string{commoncluster.RoleLabel(commoncluster.Tenant)}
			parentSpace := spacetest.NewSpace("jane")
			sr := spacerequesttest.NewSpaceRequest("jane",
				srNamespace.GetName(),
				spacerequesttest.WithTierName("unknown"), // this tier doesn't exist
				spacerequesttest.WithTargetClusterRoles(srClusterRoles))
			member1Client := test.NewFakeClient(t, sr, srNamespace)
			member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t, appstudioTier, parentSpace)
			ctrl := newReconciler(hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sr))

			// then
			require.EqualError(t, err, "nstemplatetiers.toolchain.dev.openshift.com \"unknown\" not found")
		})

		t.Run("tierName field is empty", func(t *testing.T) {
			// given
			srNamespace := newNamespace("jane")
			srClusterRoles := []string{commoncluster.RoleLabel(commoncluster.Tenant)}
			// the tier doesn't exist
			sr := spacerequesttest.NewSpaceRequest("jane",
				srNamespace.GetName(),
				spacerequesttest.WithTierName(""),
				spacerequesttest.WithTargetClusterRoles(srClusterRoles))
			member1Client := test.NewFakeClient(t, sr, srNamespace)
			member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t, appstudioTier)
			ctrl := newReconciler(hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sr))

			// then
			require.EqualError(t, err, "tierName cannot be blank")
		})

		t.Run("error while getting tier", func(t *testing.T) {
			// given
			srNamespace := newNamespace("jane")
			srClusterRoles := []string{commoncluster.RoleLabel(commoncluster.Tenant)}
			parentSpace := spacetest.NewSpace("jane")
			// the tier doesn't exist
			sr := spacerequesttest.NewSpaceRequest("jane",
				srNamespace.GetName(),
				spacerequesttest.WithTierName("appstudio"),
				spacerequesttest.WithTargetClusterRoles(srClusterRoles))
			member1Client := test.NewFakeClient(t, sr, srNamespace)
			member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t, appstudioTier, parentSpace)
			hostClient.MockGet = func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
				if _, ok := obj.(*toolchainv1alpha1.NSTemplateTier); ok {
					return fmt.Errorf("mock error")
				}
				return hostClient.Client.Get(ctx, key, obj)
			}
			ctrl := newReconciler(hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sr))

			// then
			require.EqualError(t, err, "unable to get the current NSTemplateTier: mock error")
		})

		t.Run("error creating space", func(t *testing.T) {
			// given
			srNamespace := newNamespace("jane")
			parentSpace := spacetest.NewSpace("jane")
			sr := spacerequesttest.NewSpaceRequest("jane",
				srNamespace.GetName(),
				spacerequesttest.WithTierName("appstudio"))
			member1Client := test.NewFakeClient(t, sr, srNamespace)
			member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t, appstudioTier, parentSpace)
			hostClient.MockCreate = func(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
				if _, ok := obj.(*toolchainv1alpha1.Space); ok {
					return fmt.Errorf("mock error")
				}
				return hostClient.Client.Create(ctx, obj, opts...)
			}
			ctrl := newReconciler(hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sr))

			// then
			require.EqualError(t, err, "unable to create space: mock error")
		})
		t.Run("parent space is being deleted", func(t *testing.T) {
			// given
			parentSpace := spacetest.NewSpace("jane",
				spacetest.WithCondition(spacetest.Terminating()),
				spacetest.WithDeletionTimestamp()) // parent space for some reason is being deleted
			srNamespace := newNamespace("jane")
			srClusterRoles := []string{commoncluster.RoleLabel(commoncluster.Tenant)}
			sr := spacerequesttest.NewSpaceRequest("jane",
				srNamespace.GetName(),
				spacerequesttest.WithTierName("appstudio"),
				spacerequesttest.WithTargetClusterRoles(srClusterRoles))
			member1Client := test.NewFakeClient(t, sr, srNamespace)
			member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t, appstudioTier, parentSpace)
			ctrl := newReconciler(hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sr))

			// then
			require.EqualError(t, err, "parentSpace is being deleted")
		})

		t.Run("parent space is deleted", func(t *testing.T) {
			// given
			srNamespace := newNamespace("jane")
			srClusterRoles := []string{commoncluster.RoleLabel(commoncluster.Tenant)}
			sr := spacerequesttest.NewSpaceRequest("jane",
				srNamespace.GetName(),
				spacerequesttest.WithTierName("appstudio"),
				spacerequesttest.WithTargetClusterRoles(srClusterRoles))
			member1Client := test.NewFakeClient(t, sr, srNamespace)
			member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t, appstudioTier)
			ctrl := newReconciler(hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sr))

			// then
			require.EqualError(t, err, "the parentSpace is deleted: spaces.toolchain.dev.openshift.com \"jane\" not found")
		})

		t.Run("invalid number of subSpaces found", func(t *testing.T) {
			// given
			srNamespace := newNamespace("jane")
			srClusterRoles := []string{commoncluster.RoleLabel(commoncluster.Tenant)}
			parentSpace := spacetest.NewSpace("jane")
			sr := spacerequesttest.NewSpaceRequest("jane",
				srNamespace.GetName(),
				spacerequesttest.WithTierName("appstudio"),
				spacerequesttest.WithTargetClusterRoles(srClusterRoles))
			subSpace1 := spacetest.NewSpace("jane-subs",
				spacetest.WithLabel(toolchainv1alpha1.SpaceRequestLabelKey, sr.GetName()),               // subSpace was created from spaceRequest
				spacetest.WithLabel(toolchainv1alpha1.SpaceRequestNamespaceLabelKey, sr.GetNamespace()), // subSpace was created from spaceRequest
				spacetest.WithSpecParentSpace("jane"))
			subSpace2 := spacetest.NewSpace("jane-subs2",
				spacetest.WithLabel(toolchainv1alpha1.SpaceRequestLabelKey, sr.GetName()),               // subSpace was created from spaceRequest
				spacetest.WithLabel(toolchainv1alpha1.SpaceRequestNamespaceLabelKey, sr.GetNamespace()), // subSpace was created from spaceRequest
				spacetest.WithSpecParentSpace("jane"))
			member1Client := test.NewFakeClient(t, sr, srNamespace)
			member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t, appstudioTier, subSpace1, subSpace2, parentSpace)
			ctrl := newReconciler(hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sr))

			// then
			require.EqualError(t, err, "invalid number of spaces found: 2 for spacerequest: jane")
		})

		t.Run("unable to get parent space", func(t *testing.T) {
			// given
			srNamespace := newNamespace("jane")
			srClusterRoles := []string{commoncluster.RoleLabel(commoncluster.Tenant)}
			sr := spacerequesttest.NewSpaceRequest("jane", srNamespace.GetName(),
				spacerequesttest.WithTierName("appstudio"),
				spacerequesttest.WithTargetClusterRoles(srClusterRoles))
			member1Client := test.NewFakeClient(t, sr, srNamespace)
			member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t, appstudioTier)
			hostClient.MockGet = func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
				if _, ok := obj.(*toolchainv1alpha1.Space); ok {
					return fmt.Errorf("mock error")
				}
				return hostClient.Client.Get(ctx, key, obj)
			}
			ctrl := newReconciler(hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sr))

			// then
			require.EqualError(t, err, "unable to get parentSpace: mock error")
		})
	})
}

func TestUpdateSpaceRequest(t *testing.T) {
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	err := apis.AddToScheme(scheme.Scheme)
	require.NoError(t, err)
	appstudioTier := tiertest.AppStudioTier(t, tiertest.AppStudioTemplates)

	t.Run("success", func(t *testing.T) {
		t.Run("update subSpace tier", func(t *testing.T) {
			// given
			newTier := tiertest.Tier(t, "mynewtier", tiertest.AppStudioTemplates)
			srNamespace := newNamespace("jane")
			parentSpace := spacetest.NewSpace("jane")
			srClusterRoles := []string{commoncluster.RoleLabel(commoncluster.Tenant)}
			sr := spacerequesttest.NewSpaceRequest("jane",
				srNamespace.GetName(),
				spacerequesttest.WithTierName(newTier.Name), // space request uses new tier
				spacerequesttest.WithTargetClusterRoles(srClusterRoles),
				spacerequesttest.WithFinalizer())
			subSpace := spacetest.NewSpace("jane-subs",
				spacetest.WithLabel(toolchainv1alpha1.SpaceRequestLabelKey, sr.GetName()),               // subSpace was created from spaceRequest
				spacetest.WithLabel(toolchainv1alpha1.SpaceRequestNamespaceLabelKey, sr.GetNamespace()), // subSpace was created from spaceRequest
				spacetest.WithCondition(spacetest.Ready()),
				spacetest.WithSpecParentSpace("jane"),
				spacetest.WithSpecTargetClusterRoles(srClusterRoles),
				spacetest.WithTierName("appstudio")) // subSpace doesn't have yet the new tier
			member1 := NewMemberClusterWithClient(test.NewFakeClient(t, sr, srNamespace), "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t, appstudioTier, newTier, parentSpace, subSpace)
			ctrl := newReconciler(hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sr))

			// then
			require.NoError(t, err)
			// spacerequest exists with expected cluster roles and finalizer
			spacerequesttest.AssertThatSpaceRequest(t, srNamespace.Name, sr.GetName(), member1.Client).
				Exists().
				HasSpecTargetClusterRoles(srClusterRoles).
				HasSpecTierName(newTier.Name). // space request still has the new tier
				HasFinalizer()
			// the subspace is updated with tierName and cluster roles from the spacerequests
			spacetest.AssertThatSpace(t, test.HostOperatorNs, "jane-subs", hostClient).
				Exists().
				HasSpecTargetClusterRoles(srClusterRoles).
				HasTier(newTier.Name). // now also the subSpace reflects same tier
				HasParentSpace("jane")
		})

		t.Run("update target cluster roles", func(t *testing.T) {
			// given
			srNamespace := newNamespace("jane")
			srClusterRoles := []string{commoncluster.RoleLabel(commoncluster.Tenant)}
			parentSpace := spacetest.NewSpace("jane")
			updatedSRClusterRoles := append(srClusterRoles, commoncluster.RoleLabel("newcoolcluster"))
			sr := spacerequesttest.NewSpaceRequest("jane",
				srNamespace.GetName(),
				spacerequesttest.WithTierName("appstudio"),
				spacerequesttest.WithTargetClusterRoles(updatedSRClusterRoles), // add a new cluster role label to check if it's reflected on the subSpace
				spacerequesttest.WithFinalizer())
			subSpace := spacetest.NewSpace("jane-subs",
				spacetest.WithLabel(toolchainv1alpha1.SpaceRequestLabelKey, sr.GetName()),               // subSpace was created from spaceRequest
				spacetest.WithLabel(toolchainv1alpha1.SpaceRequestNamespaceLabelKey, sr.GetNamespace()), // subSpace was created from spaceRequest
				spacetest.WithSpecTargetCluster("member-1"),
				spacetest.WithCondition(spacetest.Ready()),
				spacetest.WithSpecParentSpace("jane"),
				spacetest.WithSpecTargetClusterRoles(srClusterRoles), // initially the subSpace has only the tenant cluster role label
				spacetest.WithTierName("appstudio"))
			member1 := NewMemberClusterWithClient(test.NewFakeClient(t, sr, srNamespace), "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t, appstudioTier, subSpace, parentSpace)
			ctrl := newReconciler(hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sr))

			// then
			require.NoError(t, err)
			// spacerequest exists with expected cluster roles and finalizer
			spacerequesttest.AssertThatSpaceRequest(t, srNamespace.Name, sr.GetName(), member1.Client).
				Exists().
				HasSpecTargetClusterRoles(updatedSRClusterRoles).
				HasSpecTierName("appstudio").
				HasFinalizer()
			// the subspace is updated with the tierName and new cluster roles from the spacerequest
			spacetest.AssertThatSpace(t, test.HostOperatorNs, "jane-subs", hostClient).
				Exists().
				HasSpecTargetClusterRoles(updatedSRClusterRoles). // now also subSpace has the updated cluster roles
				HasTier("appstudio").
				HasParentSpace("jane")
		})
	})

	t.Run("failure", func(t *testing.T) {
		t.Run("unable to update subSpace", func(t *testing.T) {
			// given
			srNamespace := newNamespace("jane")
			parentSpace := spacetest.NewSpace("jane")
			sr := spacerequesttest.NewSpaceRequest("jane",
				srNamespace.GetName(),
				spacerequesttest.WithTierName("appstudio"),
				spacerequesttest.WithFinalizer())
			subSpace := spacetest.NewSpace("jane-subs",
				spacetest.WithLabel(toolchainv1alpha1.SpaceRequestLabelKey, sr.GetName()),
				spacetest.WithLabel(toolchainv1alpha1.SpaceRequestNamespaceLabelKey, sr.GetNamespace()),
				spacetest.WithSpecTargetCluster("member-1"),
				spacetest.WithCondition(spacetest.Ready()),
				spacetest.WithSpecParentSpace("jane"),
				spacetest.WithTierName("anothertier")) // tier name will have to be updated
			member1Client := test.NewFakeClient(t, sr, srNamespace)
			member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t, appstudioTier, parentSpace, subSpace)
			hostClient.MockUpdate = func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
				if _, ok := obj.(*toolchainv1alpha1.Space); ok {
					return fmt.Errorf("mock error")
				}
				return hostClient.Update(ctx, obj, opts...)
			}
			ctrl := newReconciler(hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sr))

			// then
			require.EqualError(t, err, "unable to update tiername and targetclusterroles: mock error")
		})
	})
}

func TestDeleteSpaceRequest(t *testing.T) {
	// given
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	err := apis.AddToScheme(scheme.Scheme)
	require.NoError(t, err)
	appstudioTier := tiertest.AppStudioTier(t, tiertest.AppStudioTemplates)
	t.Run("success", func(t *testing.T) {
		// given
		srNamespace := newNamespace("jane")
		srClusterRoles := []string{commoncluster.RoleLabel(commoncluster.Tenant)}
		parentSpace := spacetest.NewSpace("jane")
		sr := spacerequesttest.NewSpaceRequest("jane",
			srNamespace.GetName(),
			spacerequesttest.WithTierName("appstudio"),
			spacerequesttest.WithTargetClusterRoles(srClusterRoles),
			spacerequesttest.WithDeletionTimestamp(), // spaceRequest was deleted
			spacerequesttest.WithFinalizer())         // has finalizer still
		subSpace := spacetest.NewSpace("jane-subs",
			spacetest.WithLabel(toolchainv1alpha1.SpaceRequestLabelKey, sr.GetName()),               // subSpace was created from spaceRequest
			spacetest.WithLabel(toolchainv1alpha1.SpaceRequestNamespaceLabelKey, sr.GetNamespace()), // subSpace was created from spaceRequest
			spacetest.WithSpecTargetCluster("member-1"),
			spacetest.WithCondition(spacetest.Ready()), // space is still in ready state
			spacetest.WithSpecParentSpace("jane"),
			spacetest.WithSpecTargetClusterRoles(srClusterRoles),
			spacetest.WithTierName(sr.Spec.TierName))
		member1 := NewMemberClusterWithClient(test.NewFakeClient(t, sr, srNamespace), "member-1", corev1.ConditionTrue)
		hostClient := test.NewFakeClient(t, appstudioTier, subSpace, parentSpace)
		ctrl := newReconciler(hostClient, member1)

		t.Run("spaceRequest should be in terminating while subSpace is deleted", func(t *testing.T) {
			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sr))

			// then
			require.NoError(t, err)
			// spacerequest has finalizer and is terminating
			spacerequesttest.AssertThatSpaceRequest(t, srNamespace.Name, sr.GetName(), member1.Client).
				Exists().
				HasSpecTargetClusterRoles(srClusterRoles).
				HasConditions(spacetest.Terminating()).
				HasFinalizer() // finalizer is still there until subSpace is gone
			// subSpace is deleted
			spacetest.AssertThatSpace(t, test.HostOperatorNs, "jane-subs", hostClient).
				DoesNotExist() // subSpace is gone

			t.Run("spaceRequest is deleted when subSpace is gone", func(t *testing.T) {
				// when
				_, err := ctrl.Reconcile(context.TODO(), requestFor(sr))

				// then
				// space request was deleted
				require.NoError(t, err)
				spacerequesttest.AssertThatSpaceRequest(t, srNamespace.Name, sr.GetName(), member1.Client).
					DoesNotExist() // spaceRequest is gone
			})
		})

		t.Run("subSpace is being deleted", func(t *testing.T) {
			// given
			srNamespace := newNamespace("jane")
			parentSpace := spacetest.NewSpace("jane",
				spacetest.WithCondition(spacetest.Ready()))
			sr := spacerequesttest.NewSpaceRequest("jane",
				srNamespace.GetName(),
				spacerequesttest.WithTierName("appstudio"),
				spacerequesttest.WithTargetClusterRoles(srClusterRoles),
				spacerequesttest.WithDeletionTimestamp(), // spaceRequest was deleted
				spacerequesttest.WithFinalizer())         // has finalizer still
			subSpace := spacetest.NewSpace("jane-subs",
				spacetest.WithLabel(toolchainv1alpha1.SpaceRequestLabelKey, sr.GetName()),
				spacetest.WithLabel(toolchainv1alpha1.SpaceRequestNamespaceLabelKey, sr.GetNamespace()),
				spacetest.WithSpecTargetCluster("member-1"),
				spacetest.WithDeletionTimestamp(), // subSpace is being deleted
				spacetest.WithCondition(spacetest.Ready()),
				spacetest.WithSpecParentSpace("jane"),
				spacetest.WithSpecTargetClusterRoles(srClusterRoles),
				spacetest.WithTierName(sr.Spec.TierName))
			member1Client := test.NewFakeClient(t, sr, srNamespace)
			member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t, appstudioTier, parentSpace, subSpace)
			ctrl := newReconciler(hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sr))

			// then
			require.NoError(t, err)
			spacerequesttest.AssertThatSpaceRequest(t, srNamespace.Name, sr.GetName(), member1.Client).
				Exists().HasConditions(spacetest.Terminating()) // spaceRequest is in terminating state
		})
	})

	t.Run("failure", func(t *testing.T) {
		t.Run("unexpected number of subSpaces", func(t *testing.T) {
			// given
			srNamespace := newNamespace("jane")
			srClusterRoles := []string{commoncluster.RoleLabel(commoncluster.Tenant)}
			parentSpace := spacetest.NewSpace("jane")
			sr := spacerequesttest.NewSpaceRequest("jane",
				srNamespace.GetName(),
				spacerequesttest.WithDeletionTimestamp(), // space request deletion triggered
				spacerequesttest.WithTierName("appstudio"),
				spacerequesttest.WithTargetClusterRoles(srClusterRoles))
			subSpace1 := spacetest.NewSpace("jane-subs",
				spacetest.WithLabel(toolchainv1alpha1.SpaceRequestLabelKey, sr.GetName()),
				spacetest.WithLabel(toolchainv1alpha1.SpaceRequestNamespaceLabelKey, sr.GetNamespace()),
				spacetest.WithSpecParentSpace("jane"))
			subSpace2 := spacetest.NewSpace("jane-subs2",
				spacetest.WithLabel(toolchainv1alpha1.SpaceRequestLabelKey, sr.GetName()),
				spacetest.WithLabel(toolchainv1alpha1.SpaceRequestNamespaceLabelKey, sr.GetNamespace()),
				spacetest.WithSpecParentSpace("jane"))
			member1Client := test.NewFakeClient(t, sr, srNamespace)
			member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t, appstudioTier, subSpace1, subSpace2, parentSpace)
			ctrl := newReconciler(hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sr))

			// then
			require.EqualError(t, err, "invalid number of subSpaces found")
		})

		t.Run("unable to set status terminating", func(t *testing.T) {
			// given
			srNamespace := newNamespace("jane")
			srClusterRoles := []string{commoncluster.RoleLabel(commoncluster.Tenant)}
			parentSpace := spacetest.NewSpace("jane")
			sr := spacerequesttest.NewSpaceRequest("jane",
				srNamespace.GetName(),
				spacerequesttest.WithDeletionTimestamp(), // space request deletion triggered
				spacerequesttest.WithTierName("appstudio"),
				spacerequesttest.WithTargetClusterRoles(srClusterRoles))
			subSpace := spacetest.NewSpace("jane-subs",
				spacetest.WithLabel(toolchainv1alpha1.SpaceRequestLabelKey, sr.GetName()),
				spacetest.WithLabel(toolchainv1alpha1.SpaceRequestNamespaceLabelKey, sr.GetNamespace()),
				spacetest.WithSpecParentSpace("jane"))
			member1Client := test.NewFakeClient(t, sr, srNamespace)
			member1Client.MockStatusUpdate = func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
				if _, ok := obj.(*toolchainv1alpha1.SpaceRequest); ok {
					return fmt.Errorf("mock error")
				}
				return member1Client.Client.Update(ctx, obj, opts...)
			}
			member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t, appstudioTier, subSpace, parentSpace)
			ctrl := newReconciler(hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sr))

			// then
			require.EqualError(t, err, "error updating status: mock error")
		})

		t.Run("failed to remove finalizer", func(t *testing.T) {
			// given
			srNamespace := newNamespace("jane")
			parentSpace := spacetest.NewSpace("jane")
			sr := spacerequesttest.NewSpaceRequest("jane",
				srNamespace.GetName(),
				spacerequesttest.WithDeletionTimestamp(), // space request deletion triggered
				spacerequesttest.WithTierName("appstudio"),
				spacerequesttest.WithFinalizer())
			member1Client := test.NewFakeClient(t, sr, srNamespace)
			member1Client.MockUpdate = func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
				if _, ok := obj.(*toolchainv1alpha1.SpaceRequest); ok {
					return fmt.Errorf("mock error")
				}
				return member1Client.Client.Update(ctx, obj, opts...)
			}
			member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t, appstudioTier, parentSpace)
			ctrl := newReconciler(hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sr))

			// then
			require.EqualError(t, err, "failed to remove finalizer: mock error")
		})

		t.Run("unable to list subSpaces", func(t *testing.T) {
			// given
			srNamespace := newNamespace("jane")
			sr := spacerequesttest.NewSpaceRequest("jane",
				srNamespace.GetName(),
				spacerequesttest.WithDeletionTimestamp(), // space request deletion triggered
				spacerequesttest.WithTierName("appstudio"))
			member1Client := test.NewFakeClient(t, sr, srNamespace)
			member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t, appstudioTier)
			hostClient.MockList = func(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
				if _, ok := list.(*toolchainv1alpha1.SpaceList); ok {
					return fmt.Errorf("mock error")
				}
				return hostClient.Client.List(ctx, list, opts...)
			}
			ctrl := newReconciler(hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sr))

			// then
			require.EqualError(t, err, "unable to list subSpaces: attempt to list Spaces associated with spaceRequest jane in namespace jane-tenant failed: mock error")
		})

		t.Run("unable to delete subSpace", func(t *testing.T) {
			// given
			srNamespace := newNamespace("jane")
			sr := spacerequesttest.NewSpaceRequest("jane",
				srNamespace.GetName(),
				spacerequesttest.WithDeletionTimestamp(), // space request deletion triggered
				spacerequesttest.WithTierName("appstudio"),
				spacerequesttest.WithFinalizer())
			subSpace := spacetest.NewSpace("jane-subs",
				spacetest.WithLabel(toolchainv1alpha1.SpaceRequestLabelKey, sr.GetName()),
				spacetest.WithLabel(toolchainv1alpha1.SpaceRequestNamespaceLabelKey, sr.GetNamespace()),
				spacetest.WithSpecParentSpace("jane"))
			member1Client := test.NewFakeClient(t, sr, srNamespace)
			member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t, appstudioTier, subSpace)
			hostClient.MockDelete = func(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
				if _, ok := obj.(*toolchainv1alpha1.Space); ok {
					return fmt.Errorf("mock error")
				}
				return hostClient.Client.Delete(ctx, obj, opts...)
			}
			ctrl := newReconciler(hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sr))

			// then
			require.EqualError(t, err, "unable to delete subspace: mock error")
		})

	})
}

func newReconciler(hostCl client.Client, memberClusters ...*commoncluster.CachedToolchainCluster) *spacerequest.Reconciler {
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
	return &spacerequest.Reconciler{
		Client:         hostCl,
		Namespace:      test.HostOperatorNs,
		MemberClusters: clusters,
	}
}

func requestFor(s *toolchainv1alpha1.SpaceRequest) reconcile.Request {
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
			Namespace: "john-tenant",
			Name:      "unknown",
		},
	}
}

func newNamespace(owner string) *corev1.Namespace {
	labels := map[string]string{
		"toolchain.dev.openshift.com/type":     "tenant",
		"toolchain.dev.openshift.com/provider": "codeready-toolchain",
	}
	if owner != "noowner" {
		labels["toolchain.dev.openshift.com/owner"] = owner
	}
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   fmt.Sprintf("%s-tenant", owner),
			Labels: labels,
		},
		Status: corev1.NamespaceStatus{Phase: corev1.NamespaceActive},
	}
	return ns
}

func mockGetSpaceRequestFail(cl client.Client) func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
	return func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
		if _, ok := obj.(*toolchainv1alpha1.SpaceRequest); ok {
			return fmt.Errorf("mock error")
		}
		return cl.Get(ctx, key, obj)
	}
}

func mockUpdateSpaceRequestFail(cl client.Client) func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	return func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
		if _, ok := obj.(*toolchainv1alpha1.SpaceRequest); ok {
			return fmt.Errorf("mock error")
		}
		return cl.Update(ctx, obj, opts...)
	}
}
