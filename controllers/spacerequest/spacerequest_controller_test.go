package spacerequest_test

import (
	"context"
	"fmt"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/controllers/spacerequest"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	"github.com/codeready-toolchain/host-operator/pkg/cluster"
	spaceutil "github.com/codeready-toolchain/host-operator/pkg/space"
	. "github.com/codeready-toolchain/host-operator/test"
	tiertest "github.com/codeready-toolchain/host-operator/test/nstemplatetier"
	spacerequesttest "github.com/codeready-toolchain/host-operator/test/spacerequest"
	commoncluster "github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	commontest "github.com/codeready-toolchain/toolchain-common/pkg/test"
	spacetest "github.com/codeready-toolchain/toolchain-common/pkg/test/space"

	"github.com/stretchr/testify/require"
	"gopkg.in/h2non/gock.v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
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
	srNamespace := spacerequesttest.NewNamespace("jane")
	srClusterRoles := []string{commoncluster.RoleLabel(commoncluster.Tenant)}
	parentSpace := spacetest.NewSpace(test.HostOperatorNs, "jane")
	t.Run("success", func(t *testing.T) {
		sr := spacerequesttest.NewSpaceRequest("jane", srNamespace.GetName(),
			spacerequesttest.WithTierName("appstudio"),
			spacerequesttest.WithTargetClusterRoles(srClusterRoles))

		t.Run("subSpace doesn't exists it should be created", func(t *testing.T) {
			// given
			member1 := NewMemberClusterWithClient(test.NewFakeClient(t, sr, srNamespace), "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t, appstudioTier, parentSpace)
			ctrl := newReconciler(t, hostClient, member1)

			// when
			_, err = ctrl.Reconcile(context.TODO(), requestFor(sr))

			// then
			require.NoError(t, err)
			// spacerequest exists with expected cluster roles and finalizer
			spacerequesttest.AssertThatSpaceRequest(t, srNamespace.Name, sr.GetName(), member1.Client).
				HasSpecTargetClusterRoles(srClusterRoles).
				HasSpecTierName("appstudio").
				HasConditions(spacetest.Provisioning()).
				HasFinalizer()
			// there should be 1 subSpace that was created from the spacerequest
			spacetest.AssertThatSubSpace(t, hostClient, sr, parentSpace).
				HasTier("appstudio").
				HasSpecTargetClusterRoles(srClusterRoles)
		})

		t.Run("subSpace exists but is not ready yet", func(t *testing.T) {
			// given
			member1 := NewMemberClusterWithClient(test.NewFakeClient(t, sr, srNamespace), "member-1", corev1.ConditionTrue)
			subSpace := spacetest.NewSpace(test.HostOperatorNs, "jane-subs",
				spacetest.WithLabel(toolchainv1alpha1.SpaceRequestLabelKey, sr.GetName()),               // subSpace was created from spaceRequest
				spacetest.WithLabel(toolchainv1alpha1.SpaceRequestNamespaceLabelKey, sr.GetNamespace()), // subSpace was created from spaceRequest
				spacetest.WithLabel(toolchainv1alpha1.ParentSpaceLabelKey, "jane"),
				spacetest.WithCondition(spacetest.Provisioning()),
				spacetest.WithSpecParentSpace("jane"),
				spacetest.WithStatusTargetCluster(member1.Name),
				spacetest.WithSpecTargetClusterRoles(srClusterRoles),
				spacetest.WithTierName(sr.Spec.TierName))
			hostClient := test.NewFakeClient(t, appstudioTier, parentSpace, subSpace)
			ctrl := newReconciler(t, hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sr))

			// then
			require.NoError(t, err)
			// spacerequest exists with expected cluster roles and finalizer
			spacerequesttest.AssertThatSpaceRequest(t, srNamespace.Name, sr.GetName(), member1.Client).
				HasSpecTargetClusterRoles(srClusterRoles).
				HasConditions(spacetest.Provisioning()). // condition is reflected from space status
				HasFinalizer()
			// a subspace is created with the tierName and cluster roles from the spacerequest
			spacetest.AssertThatSpace(t, test.HostOperatorNs, "jane-subs", hostClient).
				HasSpecTargetClusterRoles(srClusterRoles).
				HasConditions(spacetest.Provisioning()).
				HasTier(sr.Spec.TierName).
				HasParentSpace("jane") // the parent space is set as expected
		})

		t.Run("subSpace is provisioned", func(t *testing.T) {
			// given
			member1 := NewMemberClusterWithClient(test.NewFakeClient(t, sr, srNamespace), "member-1", corev1.ConditionTrue)
			commontest.SetupGockForServiceAccounts(t, member1.APIEndpoint, types.NamespacedName{
				Name:      toolchainv1alpha1.AdminServiceAccountName,
				Namespace: "jane-env",
			})
			subSpace := spacetest.NewSpace(test.HostOperatorNs, spaceutil.SubSpaceName(parentSpace, sr),
				spacetest.WithLabel(toolchainv1alpha1.SpaceRequestLabelKey, sr.GetName()),               // subSpace was created from spaceRequest
				spacetest.WithLabel(toolchainv1alpha1.SpaceRequestNamespaceLabelKey, sr.GetNamespace()), // subSpace was created from spaceRequest
				spacetest.WithLabel(toolchainv1alpha1.ParentSpaceLabelKey, "jane"),
				spacetest.WithCondition(spacetest.Ready()),
				spacetest.WithSpecParentSpace("jane"),
				spacetest.WithSpecTargetClusterRoles(srClusterRoles),
				spacetest.WithStatusTargetCluster(member1.Name),
				spacetest.WithStatusProvisionedNamespaces([]toolchainv1alpha1.SpaceNamespace{{
					Name: "jane-env",
					Type: "default",
				}}),
				spacetest.WithTierName(sr.Spec.TierName))
			hostClient := test.NewFakeClient(t, appstudioTier, parentSpace, subSpace)
			ctrl := newReconciler(t, hostClient, member1)

			// when
			_, err = ctrl.Reconcile(context.TODO(), requestFor(sr))

			// then
			require.NoError(t, err)
			// spacerequest exists with expected cluster roles and finalizer
			spacerequesttest.AssertThatSpaceRequest(t, srNamespace.Name, sr.GetName(), member1.Client).
				HasSpecTargetClusterRoles(srClusterRoles).
				HasConditions(spacetest.Ready()).                                            // condition is reflected from space status
				HasStatusTargetClusterURL(member1.APIEndpoint).                              // has new target cluster url
				HasNamespaceAccess([]toolchainv1alpha1.NamespaceAccess{{Name: "jane-env"}}). // has access details for the provisioned namespace
				HasFinalizer()
			// a subspace is created with the tierName and cluster roles from the spacerequest
			spacetest.AssertThatSpace(t, test.HostOperatorNs, spaceutil.SubSpaceName(parentSpace, sr), hostClient).
				HasSpecTargetClusterRoles(srClusterRoles).
				HasConditions(spacetest.Ready()).
				HasTier(sr.Spec.TierName).
				HasParentSpace("jane") // the parent space is set as expected
		})

		t.Run("subSpace has multiple provisioned namespaces", func(t *testing.T) {
			// given
			// the default namespace has already an access secret provisioned
			member1 := NewMemberClusterWithClient(test.NewFakeClient(t, sr, srNamespace), "member-1", corev1.ConditionTrue)
			commontest.SetupGockForServiceAccounts(t, member1.APIEndpoint, types.NamespacedName{
				Name:      toolchainv1alpha1.AdminServiceAccountName,
				Namespace: "jane-env1",
			},
				types.NamespacedName{
					Name:      toolchainv1alpha1.AdminServiceAccountName,
					Namespace: "jane-env2",
				},
			)
			// this space has multiple namespaces provisioned
			subSpace := spacetest.NewSpace(test.HostOperatorNs, spaceutil.SubSpaceName(parentSpace, sr),
				spacetest.WithLabel(toolchainv1alpha1.SpaceRequestLabelKey, sr.GetName()),               // subSpace was created from spaceRequest
				spacetest.WithLabel(toolchainv1alpha1.SpaceRequestNamespaceLabelKey, sr.GetNamespace()), // subSpace was created from spaceRequest
				spacetest.WithLabel(toolchainv1alpha1.ParentSpaceLabelKey, "jane"),
				spacetest.WithCondition(spacetest.Ready()),
				spacetest.WithSpecParentSpace("jane"),
				spacetest.WithSpecTargetClusterRoles(srClusterRoles),
				spacetest.WithStatusTargetCluster(member1.Name),
				spacetest.WithStatusProvisionedNamespaces([]toolchainv1alpha1.SpaceNamespace{
					{
						Name: "jane-env1",
						Type: "default",
					},
					{
						Name: "jane-env2",
						Type: "dev",
					},
				}),
				spacetest.WithTierName(sr.Spec.TierName))
			hostClient := test.NewFakeClient(t, appstudioTier, parentSpace, subSpace)
			ctrl := newReconciler(t, hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sr))

			// then
			require.NoError(t, err)
			// spacerequest exists with expected cluster roles and finalizer
			spacerequesttest.AssertThatSpaceRequest(t, srNamespace.Name, sr.GetName(), member1.Client).
				HasSpecTargetClusterRoles(srClusterRoles).
				HasConditions(spacetest.Ready()).                                                                                                  // condition is reflected from space status
				HasStatusTargetClusterURL(member1.APIEndpoint).                                                                                    // has new target cluster url
				HasNamespaceAccess([]toolchainv1alpha1.NamespaceAccess{{Name: "jane-env1", SecretRef: "existingDevSecret"}, {Name: "jane-env2"}}). // expected secrets are there.
				HasFinalizer()
			// a subspace is created with the tierName and cluster roles from the spacerequest
			spacetest.AssertThatSpace(t, test.HostOperatorNs, spaceutil.SubSpaceName(parentSpace, sr), hostClient).
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
			ctrl := newReconciler(t, hostClient, member1, member2)
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sr))

			// then
			require.NoError(t, err)
			// spacerequest exists with expected cluster roles and finalizer
			spacerequesttest.AssertThatSpaceRequest(t, srNamespace.Name, sr.GetName(), member2.Client).
				HasSpecTierName("appstudio").
				HasSpecTargetClusterRoles(srClusterRoles).
				HasConditions(spacetest.Provisioning()).
				HasFinalizer()
			spacetest.AssertThatSubSpace(t, hostClient, sr, parentSpace).
				HasTier("appstudio").
				HasSpecTargetClusterRoles(srClusterRoles)
		})

		t.Run("spacerequest has empty target cluster roles", func(t *testing.T) {
			// when
			parentSpaceWithTarget := spacetest.NewSpace(test.HostOperatorNs, "jane", spacetest.WithSpecTargetCluster("member-2"))
			spaceRequest := spacerequesttest.NewSpaceRequest("noroles", srNamespace.GetName(),
				spacerequesttest.WithTierName("appstudio"))
			member1 := NewMemberClusterWithClient(test.NewFakeClient(t), "member-1", corev1.ConditionTrue)
			member2 := NewMemberClusterWithClient(test.NewFakeClient(t, spaceRequest, srNamespace), "member-2", corev1.ConditionTrue) // space request and namespace are on member2
			hostClient := test.NewFakeClient(t, appstudioTier, parentSpaceWithTarget)
			ctrl := newReconciler(t, hostClient, member1, member2)
			_, err := ctrl.Reconcile(context.TODO(), requestFor(spaceRequest))

			// then
			require.NoError(t, err)
			// spacerequest exists with expected cluster roles and finalizer
			spacerequesttest.AssertThatSpaceRequest(t, srNamespace.Name, spaceRequest.GetName(), member2.Client).
				HasSpecTierName("appstudio").
				HasSpecTargetClusterRoles([]string(nil)). // has empty target cluster roles
				HasConditions(spacetest.Provisioning()).
				HasFinalizer()
			spacetest.AssertThatSubSpace(t, hostClient, spaceRequest, parentSpace).
				HasTier("appstudio").
				HasSpecTargetClusterRoles([]string(nil)). // has empty target cluster roles
				HasSpecTargetCluster("member-2")          // subSpace has same target cluster as parentSpace
		})

		t.Run("spacerequest has already an existing secret", func(t *testing.T) {
			// given
			kubeconfigSecret1 := test.CreateSecret("jane-xyz1", sr.Namespace, map[string][]byte{
				"kubeconfig": []byte(fakeKubeConfigSecret("member-1")),
			})
			kubeconfigSecret1.StringData = map[string]string{
				"kubeconfig": fakeKubeConfigSecret("member-1"),
			}
			kubeconfigSecret1.Labels = map[string]string{}
			kubeconfigSecret1.Labels[toolchainv1alpha1.SpaceRequestLabelKey] = sr.GetName()
			kubeconfigSecret1.Labels[toolchainv1alpha1.SpaceRequestNamespaceLabelKey] = sr.GetNamespace()
			kubeconfigSecret1.Labels[toolchainv1alpha1.SpaceRequestProvisionedNamespaceLabelKey] = "jane-env1"
			member1 := NewMemberClusterWithClient(test.NewFakeClient(t, sr, srNamespace, kubeconfigSecret1), "member-1", corev1.ConditionTrue)
			commontest.SetupGockForServiceAccounts(t, member1.APIEndpoint, types.NamespacedName{
				Name:      toolchainv1alpha1.AdminServiceAccountName,
				Namespace: "jane-env1",
			},
				types.NamespacedName{
					Name:      toolchainv1alpha1.AdminServiceAccountName,
					Namespace: "jane-env2",
				},
			)
			// this space has multiple namespaces provisioned
			subSpace := spacetest.NewSpace(test.HostOperatorNs, spaceutil.SubSpaceName(parentSpace, sr),
				spacetest.WithLabel(toolchainv1alpha1.SpaceRequestLabelKey, sr.GetName()),               // subSpace was created from spaceRequest
				spacetest.WithLabel(toolchainv1alpha1.SpaceRequestNamespaceLabelKey, sr.GetNamespace()), // subSpace was created from spaceRequest
				spacetest.WithLabel(toolchainv1alpha1.ParentSpaceLabelKey, "jane"),
				spacetest.WithCondition(spacetest.Ready()),
				spacetest.WithSpecParentSpace("jane"),
				spacetest.WithSpecTargetClusterRoles(srClusterRoles),
				spacetest.WithStatusTargetCluster(member1.Name),
				spacetest.WithStatusProvisionedNamespaces([]toolchainv1alpha1.SpaceNamespace{
					{
						Name: "jane-env1",
						Type: "default",
					},
					{
						Name: "jane-env2",
						Type: "dev",
					},
				}),
				spacetest.WithTierName(sr.Spec.TierName))
			hostClient := test.NewFakeClient(t, appstudioTier, parentSpace, subSpace)
			ctrl := newReconciler(t, hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sr))

			// then
			require.NoError(t, err)
			// spacerequest exists with expected cluster roles and finalizer
			spacerequesttest.AssertThatSpaceRequest(t, srNamespace.Name, sr.GetName(), member1.Client).
				HasSpecTargetClusterRoles(srClusterRoles).
				HasConditions(spacetest.Ready()).                                                                                          // condition is reflected from space status
				HasStatusTargetClusterURL(member1.APIEndpoint).                                                                            // has new target cluster url
				HasNamespaceAccess([]toolchainv1alpha1.NamespaceAccess{{Name: "jane-env1", SecretRef: "jane-xyz1"}, {Name: "jane-env2"}}). // first secret was already there while second one just newly created.
				HasFinalizer()
			// a subspace is created with the tierName and cluster roles from the spacerequest
			spacetest.AssertThatSpace(t, test.HostOperatorNs, spaceutil.SubSpaceName(parentSpace, sr), hostClient).
				HasSpecTargetClusterRoles(srClusterRoles).
				HasConditions(spacetest.Ready()).
				HasTier(sr.Spec.TierName).
				HasParentSpace("jane") // the parent space is set as expected
		})

		t.Run("subSpace target cluster is different from spacerequest cluster", func(t *testing.T) {
			// given
			parentSpaceWithTarget := spacetest.NewSpace(test.HostOperatorNs, "jane", spacetest.WithSpecTargetCluster("member-1"))
			spaceRequest := spacerequesttest.NewSpaceRequest("jane", "jane-tenant",
				spacerequesttest.WithTierName("appstudio"),
				spacerequesttest.WithTargetClusterRoles([]string{"member-2"})) // the provisioned namespace is targeted for member-2
			spaceRequestNamespace := spacerequesttest.NewNamespace("jane")
			member1 := NewMemberClusterWithClient(test.NewFakeClient(t, spaceRequest, spaceRequestNamespace), "member-1", corev1.ConditionTrue) // spacerequest is created on member-1 but has target cluster member-2
			// the provisioned namespace is on different cluster then the spacerequest resource
			member2 := NewMemberClusterWithClient(test.NewFakeClient(t), "member-2", corev1.ConditionTrue,
				WithClusterRoleLabel(commoncluster.RoleLabel("member-2")),
			)
			commontest.SetupGockForServiceAccounts(t, member2.APIEndpoint, types.NamespacedName{
				Name:      toolchainv1alpha1.AdminServiceAccountName,
				Namespace: "jane-env1",
			},
			)
			subSpace := spacetest.NewSpace(test.HostOperatorNs, spaceutil.SubSpaceName(parentSpaceWithTarget, spaceRequest),
				spacetest.WithLabel(toolchainv1alpha1.SpaceRequestLabelKey, spaceRequest.GetName()),               // subSpace was created from spaceRequest
				spacetest.WithLabel(toolchainv1alpha1.SpaceRequestNamespaceLabelKey, spaceRequest.GetNamespace()), // subSpace was created from spaceRequest
				spacetest.WithLabel(toolchainv1alpha1.ParentSpaceLabelKey, "jane"),
				spacetest.WithCondition(spacetest.Ready()),
				spacetest.WithSpecParentSpace("jane"),
				spacetest.WithSpecTargetClusterRoles([]string{"member-2"}), // target cluster is member-2
				spacetest.WithStatusTargetCluster(member2.Name),
				spacetest.WithStatusProvisionedNamespaces([]toolchainv1alpha1.SpaceNamespace{
					{
						Name: "jane-env1",
						Type: "default",
					},
				}),
				spacetest.WithTierName(spaceRequest.Spec.TierName))
			hostClient := test.NewFakeClient(t, appstudioTier, parentSpaceWithTarget, subSpace)
			ctrl := newReconciler(t, hostClient, member1, member2)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(spaceRequest))

			// then
			require.NoError(t, err)
			// spacerequest exists with expected cluster roles and finalizer
			spacerequesttest.AssertThatSpaceRequest(t, spaceRequestNamespace.Name, spaceRequest.GetName(), member1.Client).
				HasSpecTargetClusterRoles([]string{"member-2"}). // target cluster is member-2
				HasConditions(spacetest.Ready()).                // condition is reflected from space status
				HasStatusTargetClusterURL(member2.APIEndpoint).  // has new target cluster url
				HasNamespaceAccess([]toolchainv1alpha1.NamespaceAccess{{Name: "jane-env1", SecretRef: "jane-xyz1"}}).
				HasFinalizer()
			// a subspace is created with the tierName and cluster roles from the spacerequest
			spacetest.AssertThatSpace(t, test.HostOperatorNs, spaceutil.SubSpaceName(parentSpaceWithTarget, spaceRequest), hostClient).
				HasSpecTargetClusterRoles([]string{"member-2"}).
				HasConditions(spacetest.Ready()).
				HasTier(spaceRequest.Spec.TierName).
				HasParentSpace("jane")
		})

		t.Run("member1 GET request fails, member2 GET returns not found but SpaceRequest is on member3", func(t *testing.T) {
			member1Client := test.NewFakeClient(t)
			member1Client.MockGet = mockGetSpaceRequestFail(member1Client)
			member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
			member2 := NewMemberClusterWithClient(test.NewFakeClient(t), "member-2", corev1.ConditionTrue)
			member3 := NewMemberClusterWithClient(test.NewFakeClient(t, sr, srNamespace), "member-3", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t, appstudioTier, parentSpace)
			ctrl := newReconciler(t, hostClient, member1, member2, member3)

			// when
			_, err = ctrl.Reconcile(context.TODO(), requestFor(sr))

			// then
			require.NoError(t, err)
			// spacerequest exists with expected cluster roles and finalizer
			spacerequesttest.AssertThatSpaceRequest(t, srNamespace.Name, sr.GetName(), member3.Client).
				HasSpecTargetClusterRoles(srClusterRoles).
				HasSpecTierName("appstudio").
				HasConditions(spacetest.Provisioning()).
				HasFinalizer()
			// there should be 1 subSpace that was created from the spacerequest
			spacetest.AssertThatSubSpace(t, hostClient, sr, parentSpace).
				HasTier("appstudio").
				HasSpecTargetClusterRoles(srClusterRoles)
		})
	})

	t.Run("failure", func(t *testing.T) {
		// given
		sr := spacerequesttest.NewSpaceRequest("jane",
			srNamespace.GetName(),
			spacerequesttest.WithTierName("appstudio"),
			spacerequesttest.WithTargetClusterRoles(srClusterRoles))

		t.Run("unable to find SpaceRequest", func(t *testing.T) {
			member1 := NewMemberClusterWithClient(test.NewFakeClient(t), "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t, appstudioTier)
			ctrl := newReconciler(t, hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sr))

			// then
			// space request should not be there,
			// but it should not return an error by design
			require.NoError(t, err)
		})
		t.Run("unable to get SpaceRequest", func(t *testing.T) {
			member1Client := test.NewFakeClient(t)
			member1Client.MockGet = mockGetSpaceRequestFail(member1Client)
			member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t, appstudioTier)
			ctrl := newReconciler(t, hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sr))

			// then
			// space request should not be there
			require.EqualError(t, err, "unable to get the current *v1alpha1.SpaceRequest: mock error")
		})

		t.Run("error while adding finalizer", func(t *testing.T) {
			member1Client := test.NewFakeClient(t, sr, srNamespace)
			member1Client.MockUpdate = mockUpdateSpaceRequestFail(member1Client)
			member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t, appstudioTier, parentSpace)
			ctrl := newReconciler(t, hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sr))

			// then
			require.EqualError(t, err, "error while adding finalizer: mock error")
		})

		t.Run("error while updating status", func(t *testing.T) {
			member1Client := test.NewFakeClient(t, sr, srNamespace)
			member1Client.MockStatusUpdate = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.UpdateOption) error {
				if _, ok := obj.(*toolchainv1alpha1.SpaceRequest); ok {
					return fmt.Errorf("mock error")
				}
				return member1Client.Status().Update(ctx, obj, opts...)
			}
			member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t, appstudioTier, parentSpace)
			ctrl := newReconciler(t, hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sr))

			// then
			require.EqualError(t, err, "error updating status: mock error")
		})

		t.Run("unable to get parent space name", func(t *testing.T) {
			member1Client := test.NewFakeClient(t, sr)
			member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t, appstudioTier)
			ctrl := newReconciler(t, hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sr))

			// then
			require.EqualError(t, err, "unable to get namespace: namespaces \"jane-tenant\" not found")
		})

		t.Run("unable to find space label in spaceRequest namespace", func(t *testing.T) {
			// given
			srNamespace := spacerequesttest.NewNamespace("nospace")
			sr := spacerequesttest.NewSpaceRequest("jane", srNamespace.GetName(),
				spacerequesttest.WithTierName("appstudio"),
				spacerequesttest.WithTargetClusterRoles(srClusterRoles))
			member1Client := test.NewFakeClient(t, sr, srNamespace)
			member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t, appstudioTier)
			ctrl := newReconciler(t, hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sr))

			// then
			require.EqualError(t, err, "unable to find space label toolchain.dev.openshift.com/space on namespace nospace-tenant")
		})

		t.Run("unable to update space since it's being deleted", func(t *testing.T) {
			// given
			member1 := NewMemberClusterWithClient(test.NewFakeClient(t, sr, srNamespace), "member-1", corev1.ConditionTrue)
			subSpace := spacetest.NewSpace(test.HostOperatorNs, spaceutil.SubSpaceName(parentSpace, sr),
				spacetest.WithLabel(toolchainv1alpha1.SpaceRequestLabelKey, sr.GetName()),               // subSpace was created from spaceRequest
				spacetest.WithLabel(toolchainv1alpha1.SpaceRequestNamespaceLabelKey, sr.GetNamespace()), // subSpace was created from spaceRequest
				spacetest.WithDeletionTimestamp(),                                                       // space is being deleted ...
				spacetest.WithSpecParentSpace("jane"))
			hostClient := test.NewFakeClient(t, appstudioTier, parentSpace, subSpace)
			ctrl := newReconciler(t, hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sr))

			// then
			require.EqualError(t, err, "cannot update subSpace because it is currently being deleted")
		})

		t.Run("unable to find tierName", func(t *testing.T) {
			// given
			sr := spacerequesttest.NewSpaceRequest("jane",
				srNamespace.GetName(),
				spacerequesttest.WithTierName("unknown"), // this tier doesn't exist
				spacerequesttest.WithTargetClusterRoles(srClusterRoles))
			member1Client := test.NewFakeClient(t, sr, srNamespace)
			member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t, appstudioTier, parentSpace)
			ctrl := newReconciler(t, hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sr))

			// then
			require.EqualError(t, err, "nstemplatetiers.toolchain.dev.openshift.com \"unknown\" not found")
		})

		t.Run("tierName field is empty", func(t *testing.T) {
			// given
			// the tier doesn't exist
			sr := spacerequesttest.NewSpaceRequest("jane",
				srNamespace.GetName(),
				spacerequesttest.WithTierName(""),
				spacerequesttest.WithTargetClusterRoles(srClusterRoles))
			member1Client := test.NewFakeClient(t, sr, srNamespace)
			member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t, appstudioTier, parentSpace)
			ctrl := newReconciler(t, hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sr))

			// then
			require.EqualError(t, err, "tierName cannot be blank")
		})

		t.Run("error while getting tier", func(t *testing.T) {
			// given
			member1Client := test.NewFakeClient(t, sr, srNamespace)
			member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t, appstudioTier, parentSpace)
			hostClient.MockGet = func(ctx context.Context, key runtimeclient.ObjectKey, obj runtimeclient.Object, opts ...runtimeclient.GetOption) error {
				if _, ok := obj.(*toolchainv1alpha1.NSTemplateTier); ok {
					return fmt.Errorf("mock error")
				}
				return hostClient.Client.Get(ctx, key, obj, opts...)
			}
			ctrl := newReconciler(t, hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sr))

			// then
			require.EqualError(t, err, "unable to get the current NSTemplateTier: mock error")
		})

		t.Run("error creating subSpace", func(t *testing.T) {
			// given
			member1Client := test.NewFakeClient(t, sr, srNamespace)
			member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t, appstudioTier, parentSpace)
			hostClient.MockCreate = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.CreateOption) error {
				if _, ok := obj.(*toolchainv1alpha1.Space); ok {
					return fmt.Errorf("mock error")
				}
				return hostClient.Client.Create(ctx, obj, opts...)
			}
			ctrl := newReconciler(t, hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sr))

			// then
			require.EqualError(t, err, "unable to create subSpace: mock error")
		})

		t.Run("error listing spaces", func(t *testing.T) {
			// given
			member1Client := test.NewFakeClient(t, sr, srNamespace)
			member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t, appstudioTier, parentSpace)
			hostClient.MockList = func(ctx context.Context, list runtimeclient.ObjectList, opts ...runtimeclient.ListOption) error {
				if _, ok := list.(*toolchainv1alpha1.SpaceList); ok {
					return fmt.Errorf("mock error")
				}
				return hostClient.Client.List(ctx, list, opts...)
			}
			ctrl := newReconciler(t, hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sr))

			// then
			require.EqualError(t, err, "failed to list subspaces: mock error")
		})

		t.Run("listing spaces with label selectors returns more than one result", func(t *testing.T) {
			// given
			member1Client := test.NewFakeClient(t, sr, srNamespace)
			member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
			subSpace1 := spacetest.NewSpace(test.HostOperatorNs, "foo",
				spacetest.WithLabel(toolchainv1alpha1.SpaceRequestLabelKey, sr.GetName()),
				spacetest.WithLabel(toolchainv1alpha1.SpaceRequestNamespaceLabelKey, sr.GetNamespace()),
				spacetest.WithSpecParentSpace(parentSpace.GetName()))
			subSpace2 := spacetest.NewSpace(test.HostOperatorNs, "bar",
				spacetest.WithLabel(toolchainv1alpha1.SpaceRequestLabelKey, sr.GetName()),
				spacetest.WithLabel(toolchainv1alpha1.SpaceRequestNamespaceLabelKey, sr.GetNamespace()),
				spacetest.WithSpecParentSpace(parentSpace.GetName()))
			hostClient := test.NewFakeClient(t, appstudioTier, parentSpace, subSpace1, subSpace2)
			ctrl := newReconciler(t, hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sr))

			// then
			require.EqualError(t, err, "Expected 1 matching subspace for spaceRequest jane in namespace jane-tenant, found 2")
		})

		t.Run("parent space is being deleted", func(t *testing.T) {
			// given
			parentSpace := spacetest.NewSpace(test.HostOperatorNs, "jane",
				spacetest.WithCondition(spacetest.Terminating()),
				spacetest.WithDeletionTimestamp()) // parent space for some reason is being deleted
			member1Client := test.NewFakeClient(t, sr, srNamespace)
			member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t, appstudioTier, parentSpace)
			ctrl := newReconciler(t, hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sr))

			// then
			require.EqualError(t, err, "parentSpace is being deleted")
		})

		t.Run("parent space is deleted", func(t *testing.T) {
			// given
			member1Client := test.NewFakeClient(t, sr, srNamespace)
			member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t, appstudioTier)
			ctrl := newReconciler(t, hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sr))

			// then
			require.EqualError(t, err, "unable to get parentSpace: spaces.toolchain.dev.openshift.com \"jane\" not found")
		})

		t.Run("invalid number of secrets found", func(t *testing.T) {
			// given
			kubeconfigSecret1 := test.CreateSecret("jane-xyz1", sr.Namespace, map[string][]byte{
				"kubeconfig": []byte("kubeconfig1"),
			})
			kubeconfigSecret1.Labels = map[string]string{}
			kubeconfigSecret1.Labels[toolchainv1alpha1.SpaceRequestLabelKey] = sr.GetName()
			kubeconfigSecret1.Labels[toolchainv1alpha1.SpaceRequestNamespaceLabelKey] = sr.GetNamespace()
			kubeconfigSecret1.Labels[toolchainv1alpha1.SpaceRequestProvisionedNamespaceLabelKey] = "jane-env"
			kubeconfigSecret2 := test.CreateSecret("jane-xyz2", sr.Namespace, map[string][]byte{
				"kubeconfig": []byte("kubeconfig2"),
			})
			kubeconfigSecret2.Labels = map[string]string{}
			kubeconfigSecret2.Labels[toolchainv1alpha1.SpaceRequestLabelKey] = sr.GetName()
			kubeconfigSecret2.Labels[toolchainv1alpha1.SpaceRequestNamespaceLabelKey] = sr.GetNamespace()
			kubeconfigSecret2.Labels[toolchainv1alpha1.SpaceRequestProvisionedNamespaceLabelKey] = "jane-env"
			subSpace := spacetest.NewSpace(test.HostOperatorNs, spaceutil.SubSpaceName(parentSpace, sr),
				spacetest.WithLabel(toolchainv1alpha1.SpaceRequestLabelKey, sr.GetName()),               // subSpace was created from spaceRequest
				spacetest.WithLabel(toolchainv1alpha1.SpaceRequestNamespaceLabelKey, sr.GetNamespace()), // subSpace was created from spaceRequest
				spacetest.WithSpecTargetClusterRoles(srClusterRoles),
				spacetest.WithStatusTargetCluster("member-1"),
				spacetest.WithSpecParentSpace("jane"),
				spacetest.WithStatusProvisionedNamespaces([]toolchainv1alpha1.SpaceNamespace{{
					Name: "jane-env",
					Type: "default",
				}}),
				spacetest.WithTierName(sr.Spec.TierName))
			member1Client := test.NewFakeClient(t, sr, srNamespace, kubeconfigSecret1, kubeconfigSecret2)
			member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t, appstudioTier, subSpace, parentSpace)
			ctrl := newReconciler(t, hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sr))

			// then
			require.EqualError(t, err, "invalid number of secrets found. actual 2, expected 1")
		})

		t.Run("unable to get parent space", func(t *testing.T) {
			// given
			member1Client := test.NewFakeClient(t, sr, srNamespace)
			member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t, appstudioTier)
			hostClient.MockGet = func(ctx context.Context, key runtimeclient.ObjectKey, obj runtimeclient.Object, opts ...runtimeclient.GetOption) error {
				if _, ok := obj.(*toolchainv1alpha1.Space); ok {
					return fmt.Errorf("mock error")
				}
				return hostClient.Client.Get(ctx, key, obj, opts...)
			}
			ctrl := newReconciler(t, hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sr))

			// then
			require.EqualError(t, err, "unable to get parentSpace: mock error")
		})

		t.Run("error creating secret for provisioned namespace", func(t *testing.T) {
			// given
			member1Client := test.NewFakeClient(t, sr, srNamespace)
			member1Client.MockCreate = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.CreateOption) error {
				if _, ok := obj.(*corev1.Secret); ok {
					return fmt.Errorf("mock error")
				}
				return member1Client.Client.Create(ctx, obj, opts...)
			}
			member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
			commontest.SetupGockForServiceAccounts(t, member1.APIEndpoint, types.NamespacedName{
				Name:      toolchainv1alpha1.AdminServiceAccountName,
				Namespace: "jane-env",
			})
			subSpace := spacetest.NewSpace(test.HostOperatorNs, spaceutil.SubSpaceName(parentSpace, sr),
				spacetest.WithLabel(toolchainv1alpha1.SpaceRequestLabelKey, sr.GetName()),               // subSpace was created from spaceRequest
				spacetest.WithLabel(toolchainv1alpha1.SpaceRequestNamespaceLabelKey, sr.GetNamespace()), // subSpace was created from spaceRequest
				spacetest.WithLabel(toolchainv1alpha1.ParentSpaceLabelKey, "jane"),
				spacetest.WithCondition(spacetest.Ready()),
				spacetest.WithSpecParentSpace(parentSpace.Name),
				spacetest.WithSpecTargetClusterRoles(srClusterRoles),
				spacetest.WithStatusTargetCluster(member1.Name),
				spacetest.WithStatusProvisionedNamespaces([]toolchainv1alpha1.SpaceNamespace{{
					Name: "jane-env",
					Type: "default",
				}}),
				spacetest.WithTierName(sr.Spec.TierName))
			hostClient := test.NewFakeClient(t, appstudioTier, parentSpace, subSpace)
			ctrl := newReconciler(t, hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sr))

			// then
			cause := "error while creating secret: mock error"
			require.EqualError(t, err, cause)
			// spacerequest exists with expected cluster roles and finalizer
			spacerequesttest.AssertThatSpaceRequest(t, srNamespace.Name, sr.GetName(), member1.Client).
				HasSpecTargetClusterRoles(srClusterRoles).
				HasConditions(spacetest.ProvisioningFailed(cause)). // condition is set to unable to provision Space
				HasFinalizer()
		})
	})
}

func fakeKubeConfigSecret(memberName string) string {
	return `apiVersion: v1
clusters:
- cluster:
    certificate-authority-data: QVNEZjMzPT0=
    server: https://api.` + memberName + `:6433
  name: default-cluster
contexts:
- context:
    cluster: default-cluster
    namespace: jane-env
    user: namespace-manager
  name: default-context
current-context: default-context
kind: Config
preferences: {}
users:
- name: namespace-manager
  user:
    token: token-secret-for-namespace-manager
`
}

func TestUpdateSpaceRequest(t *testing.T) {
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	err := apis.AddToScheme(scheme.Scheme)
	require.NoError(t, err)
	appstudioTier := tiertest.AppStudioTier(t, tiertest.AppStudioTemplates)
	srNamespace := spacerequesttest.NewNamespace("jane")
	parentSpace := spacetest.NewSpace(test.HostOperatorNs, "jane")
	srClusterRoles := []string{commoncluster.RoleLabel(commoncluster.Tenant)}

	t.Run("success", func(t *testing.T) {
		t.Run("update subSpace tier", func(t *testing.T) {
			// given
			newTier := tiertest.Tier(t, "mynewtier", tiertest.AppStudioTemplates)
			sr := spacerequesttest.NewSpaceRequest("jane",
				srNamespace.GetName(),
				spacerequesttest.WithTierName(newTier.Name), // space request uses new tier
				spacerequesttest.WithTargetClusterRoles(srClusterRoles),
				spacerequesttest.WithFinalizer())
			subSpace := spacetest.NewSpace(test.HostOperatorNs, spaceutil.SubSpaceName(parentSpace, sr),
				spacetest.WithLabel(toolchainv1alpha1.SpaceRequestLabelKey, sr.GetName()),               // subSpace was created from spaceRequest
				spacetest.WithLabel(toolchainv1alpha1.SpaceRequestNamespaceLabelKey, sr.GetNamespace()), // subSpace was created from spaceRequest
				spacetest.WithLabel(toolchainv1alpha1.ParentSpaceLabelKey, parentSpace.GetName()),
				spacetest.WithCondition(spacetest.Ready()),
				spacetest.WithSpecParentSpace("jane"),
				spacetest.WithSpecTargetClusterRoles(srClusterRoles),
				spacetest.WithTierName("appstudio")) // subSpace doesn't have yet the new tier
			member1 := NewMemberClusterWithClient(test.NewFakeClient(t, sr, srNamespace), "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t, appstudioTier, newTier, parentSpace, subSpace)
			ctrl := newReconciler(t, hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sr))

			// then
			require.NoError(t, err)
			// spacerequest exists with expected cluster roles and finalizer
			spacerequesttest.AssertThatSpaceRequest(t, srNamespace.Name, sr.GetName(), member1.Client).
				HasSpecTargetClusterRoles(srClusterRoles).
				HasSpecTierName(newTier.Name). // space request still has the new tier
				HasFinalizer()
			// the subspace is updated with tierName and cluster roles from the spacerequests
			spacetest.AssertThatSpace(t, test.HostOperatorNs, spaceutil.SubSpaceName(parentSpace, sr), hostClient).
				HasSpecTargetClusterRoles(srClusterRoles).
				HasTier(newTier.Name). // now also the subSpace reflects same tier
				HasParentSpace("jane")
		})

		t.Run("update target cluster roles", func(t *testing.T) {
			// given
			updatedSRClusterRoles := append(srClusterRoles, commoncluster.RoleLabel("newcoolcluster"))
			sr := spacerequesttest.NewSpaceRequest("jane",
				srNamespace.GetName(),
				spacerequesttest.WithTierName("appstudio"),
				spacerequesttest.WithTargetClusterRoles(updatedSRClusterRoles), // add a new cluster role label to check if it's reflected on the subSpace
				spacerequesttest.WithFinalizer())
			subSpace := spacetest.NewSpace(test.HostOperatorNs, spaceutil.SubSpaceName(parentSpace, sr),
				spacetest.WithLabel(toolchainv1alpha1.SpaceRequestLabelKey, sr.GetName()),               // subSpace was created from spaceRequest
				spacetest.WithLabel(toolchainv1alpha1.SpaceRequestNamespaceLabelKey, sr.GetNamespace()), // subSpace was created from spaceRequest
				spacetest.WithLabel(toolchainv1alpha1.ParentSpaceLabelKey, parentSpace.GetName()),
				spacetest.WithSpecTargetCluster("member-1"),
				spacetest.WithCondition(spacetest.Ready()),
				spacetest.WithSpecParentSpace("jane"),
				spacetest.WithSpecTargetClusterRoles(srClusterRoles), // initially the subSpace has only the tenant cluster role label
				spacetest.WithTierName("appstudio"))
			member1 := NewMemberClusterWithClient(test.NewFakeClient(t, sr, srNamespace), "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t, appstudioTier, subSpace, parentSpace)
			ctrl := newReconciler(t, hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sr))

			// then
			require.NoError(t, err)
			// spacerequest exists with expected cluster roles and finalizer
			spacerequesttest.AssertThatSpaceRequest(t, srNamespace.Name, sr.GetName(), member1.Client).
				HasSpecTargetClusterRoles(updatedSRClusterRoles).
				HasSpecTierName("appstudio").
				HasFinalizer()
			// the subspace is updated with the tierName and new cluster roles from the spacerequest
			spacetest.AssertThatSpace(t, test.HostOperatorNs, spaceutil.SubSpaceName(parentSpace, sr), hostClient).
				HasSpecTargetClusterRoles(updatedSRClusterRoles). // now also subSpace has the updated cluster roles
				HasTier("appstudio").
				HasParentSpace("jane")
		})
		t.Run("update namespace access secret name", func(t *testing.T) {
			// given
			sr := spacerequesttest.NewSpaceRequest("jane",
				srNamespace.GetName(),
				spacerequesttest.WithTierName("appstudio"),
				spacerequesttest.WithTargetClusterRoles(srClusterRoles),
				spacerequesttest.WithStatusNamespaceAccess(toolchainv1alpha1.NamespaceAccess{
					Name:      "jane-env",
					SecretRef: "jane-qwerty", // secret doesn't exist and name of the secret will change when it will be created
				}),
				spacerequesttest.WithFinalizer())
			subSpace := spacetest.NewSpace(test.HostOperatorNs, spaceutil.SubSpaceName(parentSpace, sr),
				spacetest.WithLabel(toolchainv1alpha1.SpaceRequestLabelKey, sr.GetName()),               // subSpace was created from spaceRequest
				spacetest.WithLabel(toolchainv1alpha1.SpaceRequestNamespaceLabelKey, sr.GetNamespace()), // subSpace was created from spaceRequest
				spacetest.WithLabel(toolchainv1alpha1.ParentSpaceLabelKey, parentSpace.GetName()),
				spacetest.WithSpecTargetCluster("member-1"),
				spacetest.WithStatusTargetCluster("member-1"),
				spacetest.WithCondition(spacetest.Ready()),
				spacetest.WithSpecParentSpace("jane"),
				spacetest.WithSpecTargetClusterRoles(srClusterRoles),
				spacetest.WithStatusProvisionedNamespaces([]toolchainv1alpha1.SpaceNamespace{{
					Name: "jane-env",
					Type: "default",
				}}),
				spacetest.WithTierName("appstudio"))
			member1 := NewMemberClusterWithClient(test.NewFakeClient(t, sr, srNamespace), "member-1", corev1.ConditionTrue)
			commontest.SetupGockForServiceAccounts(t, member1.APIEndpoint, types.NamespacedName{
				Name:      toolchainv1alpha1.AdminServiceAccountName,
				Namespace: "jane-env",
			})
			hostClient := test.NewFakeClient(t, appstudioTier, subSpace, parentSpace)
			ctrl := newReconciler(t, hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sr))

			// then
			require.NoError(t, err)
			// spacerequest exists with expected cluster roles and finalizer
			spacerequesttest.AssertThatSpaceRequest(t, srNamespace.Name, sr.GetName(), member1.Client).
				HasSpecTierName("appstudio").
				HasNamespaceAccess([]toolchainv1alpha1.NamespaceAccess{{Name: "jane-env"}}).
				HasFinalizer()
		})
	})

	t.Run("failure", func(t *testing.T) {
		t.Run("unable to update subSpace", func(t *testing.T) {
			// given
			sr := spacerequesttest.NewSpaceRequest("jane",
				srNamespace.GetName(),
				spacerequesttest.WithTierName("appstudio"),
				spacerequesttest.WithFinalizer())
			subSpace := spacetest.NewSpace(test.HostOperatorNs, spaceutil.SubSpaceName(parentSpace, sr),
				spacetest.WithLabel(toolchainv1alpha1.SpaceRequestLabelKey, sr.GetName()),
				spacetest.WithLabel(toolchainv1alpha1.SpaceRequestNamespaceLabelKey, sr.GetNamespace()),
				spacetest.WithSpecTargetCluster("member-1"),
				spacetest.WithCondition(spacetest.Ready()),
				spacetest.WithSpecParentSpace("jane"),
				spacetest.WithTierName("anothertier")) // tier name will have to be updated
			member1Client := test.NewFakeClient(t, sr, srNamespace)
			member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t, appstudioTier, parentSpace, subSpace)
			hostClient.MockUpdate = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.UpdateOption) error {
				if _, ok := obj.(*toolchainv1alpha1.Space); ok {
					return fmt.Errorf("mock error")
				}
				return hostClient.Update(ctx, obj, opts...)
			}
			ctrl := newReconciler(t, hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sr))

			// then
			require.EqualError(t, err, "unable to update tiername and targetclusterroles: mock error")
		})

		t.Run("target cluster name not found", func(t *testing.T) {
			// given
			sr := spacerequesttest.NewSpaceRequest("jane",
				srNamespace.GetName(),
				spacerequesttest.WithTierName("appstudio"),
				spacerequesttest.WithTargetClusterRoles(srClusterRoles),
				spacerequesttest.WithStatusTargetClusterURL(""), // target cluster URL is empty
				spacerequesttest.WithFinalizer())
			subSpace := spacetest.NewSpace(test.HostOperatorNs, spaceutil.SubSpaceName(parentSpace, sr),
				spacetest.WithLabel(toolchainv1alpha1.SpaceRequestLabelKey, sr.GetName()),               // subSpace was created from spaceRequest
				spacetest.WithLabel(toolchainv1alpha1.SpaceRequestNamespaceLabelKey, sr.GetNamespace()), // subSpace was created from spaceRequest
				spacetest.WithStatusTargetCluster("invalid"),                                            // let's force an invalid name in the subSpace
				spacetest.WithCondition(spacetest.Ready()),
				spacetest.WithSpecParentSpace("jane"),
				spacetest.WithSpecTargetClusterRoles(srClusterRoles),
				spacetest.WithTierName("appstudio"))
			member1 := NewMemberClusterWithClient(test.NewFakeClient(t, sr, srNamespace), "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t, appstudioTier, subSpace, parentSpace)
			ctrl := newReconciler(t, hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sr))

			// then
			require.EqualError(t, err, "unable to find target cluster with name: invalid")
		})
	})
}

func TestDeleteSpaceRequest(t *testing.T) {
	// given
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	err := apis.AddToScheme(scheme.Scheme)
	require.NoError(t, err)
	appstudioTier := tiertest.AppStudioTier(t, tiertest.AppStudioTemplates)
	srNamespace := spacerequesttest.NewNamespace("jane")
	srClusterRoles := []string{commoncluster.RoleLabel(commoncluster.Tenant)}
	parentSpace := spacetest.NewSpace(test.HostOperatorNs, "jane")
	sr := spacerequesttest.NewSpaceRequest("jane",
		srNamespace.GetName(),
		spacerequesttest.WithTierName("appstudio"),
		spacerequesttest.WithTargetClusterRoles(srClusterRoles),
		spacerequesttest.WithDeletionTimestamp(), // spaceRequest was deleted
		spacerequesttest.WithFinalizer())         // has finalizer still
	t.Run("success", func(t *testing.T) {
		t.Run("spaceRequest should be in terminating while subSpace is deleted", func(t *testing.T) {
			// given
			subSpace := spacetest.NewSpace(test.HostOperatorNs, spaceutil.SubSpaceName(parentSpace, sr),
				spacetest.WithLabel(toolchainv1alpha1.SpaceRequestLabelKey, sr.GetName()),               // subSpace was created from spaceRequest
				spacetest.WithLabel(toolchainv1alpha1.SpaceRequestNamespaceLabelKey, sr.GetNamespace()), // subSpace was created from spaceRequest
				spacetest.WithSpecTargetCluster("member-1"),
				spacetest.WithCondition(spacetest.Ready()), // space is still in ready state
				spacetest.WithSpecParentSpace("jane"),
				spacetest.WithSpecTargetClusterRoles(srClusterRoles),
				spacetest.WithTierName(sr.Spec.TierName))
			member1 := NewMemberClusterWithClient(test.NewFakeClient(t, sr, srNamespace), "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t, appstudioTier, subSpace, parentSpace)
			ctrl := newReconciler(t, hostClient, member1)
			// when
			_, err = ctrl.Reconcile(context.TODO(), requestFor(sr))

			// then
			require.NoError(t, err)
			// spacerequest has finalizer and is terminating
			spacerequesttest.AssertThatSpaceRequest(t, srNamespace.Name, sr.GetName(), member1.Client).
				HasSpecTargetClusterRoles(srClusterRoles).
				HasConditions(spacetest.Terminating()).
				HasFinalizer() // finalizer is still there until subSpace is gone
			// subSpace is deleted
			spacetest.AssertThatSpace(t, test.HostOperatorNs, spaceutil.SubSpaceName(parentSpace, sr), hostClient).
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

		t.Run("finalizer was already removed", func(t *testing.T) {
			// given
			// space request has no finalizer
			sr := spacerequesttest.NewSpaceRequest("jane",
				"jane-tenant",
				spacerequesttest.WithTierName("appstudio"),
				spacerequesttest.WithTargetClusterRoles(srClusterRoles),
				spacerequesttest.WithDeletionTimestamp()) // spaceRequest was deleted
			member1Client := test.NewFakeClient(t, sr, srNamespace)
			member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t, appstudioTier, parentSpace)
			ctrl := newReconciler(t, hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sr))

			// then
			// space request is gone
			require.NoError(t, err)
			spacerequesttest.AssertThatSpaceRequest(t, srNamespace.Name, sr.GetName(), member1.Client).
				HasNoFinalizers()
		})
	})

	t.Run("failure", func(t *testing.T) {
		t.Run("unable to set status terminating", func(t *testing.T) {
			// given
			subSpace := spacetest.NewSpace(test.HostOperatorNs, spaceutil.SubSpaceName(parentSpace, sr),
				spacetest.WithLabel(toolchainv1alpha1.SpaceRequestLabelKey, sr.GetName()),
				spacetest.WithLabel(toolchainv1alpha1.SpaceRequestNamespaceLabelKey, sr.GetNamespace()),
				spacetest.WithSpecParentSpace("jane"))
			member1Client := test.NewFakeClient(t, sr, srNamespace)
			member1Client.MockStatusUpdate = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.UpdateOption) error {
				if _, ok := obj.(*toolchainv1alpha1.SpaceRequest); ok {
					return fmt.Errorf("mock error")
				}
				return member1Client.Client.Update(ctx, obj, opts...)
			}
			member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t, appstudioTier, subSpace, parentSpace)
			ctrl := newReconciler(t, hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sr))

			// then
			require.EqualError(t, err, "error updating status: mock error")
		})

		t.Run("failed to remove finalizer", func(t *testing.T) {
			// given
			member1Client := test.NewFakeClient(t, sr, srNamespace)
			member1Client.MockUpdate = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.UpdateOption) error {
				if _, ok := obj.(*toolchainv1alpha1.SpaceRequest); ok {
					return fmt.Errorf("mock error")
				}
				return member1Client.Client.Update(ctx, obj, opts...)
			}
			member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t, appstudioTier, parentSpace)
			ctrl := newReconciler(t, hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sr))

			// then
			require.EqualError(t, err, "failed to remove finalizer: mock error")
		})

		t.Run("unable to list subspaces", func(t *testing.T) {
			// given
			member1Client := test.NewFakeClient(t, sr, srNamespace)
			member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t, appstudioTier, parentSpace)
			hostClient.MockList = func(ctx context.Context, list runtimeclient.ObjectList, opts ...runtimeclient.ListOption) error {
				if _, ok := list.(*toolchainv1alpha1.SpaceList); ok {
					return fmt.Errorf("mock error")
				}
				return member1Client.Client.List(ctx, list, opts...)
			}
			ctrl := newReconciler(t, hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sr))

			// then
			require.EqualError(t, err, "failed to list subspaces: mock error")
		})

		t.Run("unable to delete subSpace", func(t *testing.T) {
			// given
			subSpace := spacetest.NewSpace(test.HostOperatorNs, spaceutil.SubSpaceName(parentSpace, sr),
				spacetest.WithLabel(toolchainv1alpha1.SpaceRequestLabelKey, sr.GetName()),
				spacetest.WithLabel(toolchainv1alpha1.SpaceRequestNamespaceLabelKey, sr.GetNamespace()),
				spacetest.WithSpecParentSpace("jane"))
			member1Client := test.NewFakeClient(t, sr, srNamespace)
			member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t, appstudioTier, subSpace, parentSpace)
			hostClient.MockDelete = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.DeleteOption) error {
				if _, ok := obj.(*toolchainv1alpha1.Space); ok {
					return fmt.Errorf("mock error")
				}
				return hostClient.Client.Delete(ctx, obj, opts...)
			}
			ctrl := newReconciler(t, hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sr))

			// then
			require.EqualError(t, err, "unable to delete subspace: mock error")
		})

	})
}

func newReconciler(t *testing.T, hostCl runtimeclient.Client, memberClusters ...*commoncluster.CachedToolchainCluster) *spacerequest.Reconciler {
	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)

	clusters := map[string]cluster.Cluster{}
	for _, c := range memberClusters {
		restClient, err := commontest.NewRESTClient("fake_secret", c.APIEndpoint)
		restClient.Client.Transport = gock.DefaultTransport // make sure that the underlying client's request are intercepted by Gock
		require.NoError(t, err)
		clusters[c.Name] = cluster.Cluster{
			Config: &commoncluster.Config{
				Type:              commoncluster.Member,
				APIEndpoint:       c.APIEndpoint,
				OperatorNamespace: c.OperatorNamespace,
				OwnerClusterName:  test.MemberClusterName,
				RestConfig: &rest.Config{
					Host: c.APIEndpoint,
					TLSClientConfig: rest.TLSClientConfig{
						CAData: []byte("ASDf33=="),
					},
				},
			},
			RESTClient: restClient,
			Client:     c.Client,
		}
	}
	return &spacerequest.Reconciler{
		Client:         hostCl,
		Scheme:         s,
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

func mockGetSpaceRequestFail(cl runtimeclient.Client) func(ctx context.Context, key runtimeclient.ObjectKey, obj runtimeclient.Object, opts ...runtimeclient.GetOption) error {
	return func(ctx context.Context, key runtimeclient.ObjectKey, obj runtimeclient.Object, opts ...runtimeclient.GetOption) error {
		if _, ok := obj.(*toolchainv1alpha1.SpaceRequest); ok {
			return fmt.Errorf("mock error")
		}
		return cl.Get(ctx, key, obj, opts...)
	}
}

func mockUpdateSpaceRequestFail(cl runtimeclient.Client) func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.UpdateOption) error {
	return func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.UpdateOption) error {
		if _, ok := obj.(*toolchainv1alpha1.SpaceRequest); ok {
			return fmt.Errorf("mock error")
		}
		return cl.Update(ctx, obj, opts...)
	}
}
