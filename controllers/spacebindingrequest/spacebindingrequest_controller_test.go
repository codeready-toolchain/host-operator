package spacebindingrequest_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/controllers/spacebindingrequest"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	"github.com/codeready-toolchain/host-operator/pkg/cluster"
	. "github.com/codeready-toolchain/host-operator/test"
	tiertest "github.com/codeready-toolchain/host-operator/test/nstemplatetier"
	spacebindingtest "github.com/codeready-toolchain/host-operator/test/spacebinding"
	spacerequesttest "github.com/codeready-toolchain/host-operator/test/spacerequest"
	commoncluster "github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	spacebindingcommon "github.com/codeready-toolchain/toolchain-common/pkg/spacebinding"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/codeready-toolchain/toolchain-common/pkg/test/masteruserrecord"
	spacetest "github.com/codeready-toolchain/toolchain-common/pkg/test/space"
	sbrtestcommon "github.com/codeready-toolchain/toolchain-common/pkg/test/spacebindingrequest"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestCreateSpaceBindingRequest(t *testing.T) {
	// given
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	err := apis.AddToScheme(scheme.Scheme)
	require.NoError(t, err)
	base1nsTier := tiertest.Base1nsTier(t, tiertest.CurrentBase1nsTemplates)
	janeSpace := spacetest.NewSpace(test.HostOperatorNs, "jane")
	janeMur := masteruserrecord.NewMasterUserRecord(t, "jane")
	sbrNamespace := spacerequesttest.NewNamespace("jane")
	sbr := sbrtestcommon.NewSpaceBindingRequest("jane", "jane-tenant",
		sbrtestcommon.WithMUR("jane"),
		sbrtestcommon.WithSpaceRole("admin"))
	t.Run("success", func(t *testing.T) {

		t.Run("spaceBinding doesn't exists it should be created", func(t *testing.T) {
			// given
			member1 := NewMemberClusterWithClient(test.NewFakeClient(t, sbr, sbrNamespace), "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t, janeSpace, janeMur, base1nsTier)
			ctrl := newReconciler(t, hostClient, member1)

			// when
			_, err = ctrl.Reconcile(context.TODO(), requestFor(sbr))

			// then
			require.NoError(t, err)
			// spaceBindingRequest exists with config and finalizer
			sbrtestcommon.AssertThatSpaceBindingRequest(t, sbr.GetNamespace(), sbr.GetName(), member1.Client).
				HasSpecSpaceRole("admin").
				HasSpecMasterUserRecord(janeMur.Name).
				HasConditions(sbrtestcommon.Ready()).
				HasFinalizer()
			// there should be 1 spacebinding that was created from the SpaceBindingRequest
			spacebindingtest.AssertThatSpaceBinding(t, test.HostOperatorNs, janeMur.Name, janeSpace.Name, hostClient).
				Exists().
				HasLabelWithValue(toolchainv1alpha1.SpaceBindingRequestLabelKey, sbr.GetName()). // check expected labels are there
				HasLabelWithValue(toolchainv1alpha1.SpaceBindingRequestNamespaceLabelKey, sbr.GetNamespace())
		})

		t.Run("spaceBinding exists and is up-to-date", func(t *testing.T) {
			// given
			spaceBinding := spacebindingtest.NewSpaceBinding(janeMur.Name, janeSpace.Name, "admin", sbr.GetName(), spacebindingtest.WithSpaceBindingRequest(sbr))
			member1 := NewMemberClusterWithClient(test.NewFakeClient(t, sbr, sbrNamespace), "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t, janeSpace, janeMur, base1nsTier, spaceBinding)
			ctrl := newReconciler(t, hostClient, member1)

			// when
			_, err = ctrl.Reconcile(context.TODO(), requestFor(sbr))

			// then
			require.NoError(t, err)
			// spaceBindingRequest exists with config and finalizer
			sbrtestcommon.AssertThatSpaceBindingRequest(t, sbr.GetNamespace(), sbr.GetName(), member1.Client).
				HasSpecSpaceRole("admin").
				HasSpecMasterUserRecord(janeMur.Name).
				HasConditions(sbrtestcommon.Ready()).
				HasFinalizer()
			// there should be 1 spacebinding that was created from the SpaceBindingRequest
			spacebindingtest.AssertThatSpaceBinding(t, test.HostOperatorNs, janeMur.Name, janeSpace.Name, hostClient).Exists().
				HasLabelWithValue(toolchainv1alpha1.SpaceBindingRequestLabelKey, sbr.GetName()). // check expected labels are there
				HasLabelWithValue(toolchainv1alpha1.SpaceBindingRequestNamespaceLabelKey, sbr.GetNamespace())
		})

		t.Run("member1 GET request fails, member2 GET returns not found but SpaceBindingRequest is on member3", func(t *testing.T) {
			// given
			member1Client := test.NewFakeClient(t)
			member1Client.MockGet = mockGetSpaceBindingRequestFail(member1Client)
			member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
			member2 := NewMemberClusterWithClient(test.NewFakeClient(t), "member-2", corev1.ConditionTrue)
			member3 := NewMemberClusterWithClient(test.NewFakeClient(t, sbr, sbrNamespace), "member-3", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t, janeSpace, janeMur, base1nsTier)
			ctrl := newReconciler(t, hostClient, member1, member2, member3)

			// when
			_, err = ctrl.Reconcile(context.TODO(), requestFor(sbr))

			// then
			require.NoError(t, err)
			// spaceBindingRequest exists with config and finalizer
			sbrtestcommon.AssertThatSpaceBindingRequest(t, sbr.GetNamespace(), sbr.GetName(), member3.Client).
				HasSpecSpaceRole("admin").
				HasSpecMasterUserRecord(janeMur.Name).
				HasConditions(sbrtestcommon.Ready()).
				HasFinalizer()
			// there should be 1 spacebinding that was created from the SpaceBindingRequest
			spacebindingtest.AssertThatSpaceBinding(t, test.HostOperatorNs, janeMur.Name, janeSpace.Name, hostClient).
				Exists().
				HasLabelWithValue(toolchainv1alpha1.SpaceBindingRequestLabelKey, sbr.GetName()). // check expected labels are there
				HasLabelWithValue(toolchainv1alpha1.SpaceBindingRequestNamespaceLabelKey, sbr.GetNamespace())
		})
	})

	t.Run("failure", func(t *testing.T) {
		t.Run("unable to find SpaceBindingRequest", func(t *testing.T) {
			// given
			member1 := NewMemberClusterWithClient(test.NewFakeClient(t), "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t)
			ctrl := newReconciler(t, hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sbr))

			// then
			// space binding request should not be there,
			// but it should not return an error by design
			require.NoError(t, err)
		})
		t.Run("unable to get SpaceBindingRequest", func(t *testing.T) {
			member1Client := test.NewFakeClient(t)
			member1Client.MockGet = mockGetSpaceBindingRequestFail(member1Client)
			member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t)
			ctrl := newReconciler(t, hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sbr))

			// then
			// space binding request should not be there
			require.EqualError(t, err, "unable to get the current *v1alpha1.SpaceBindingRequest: mock error")
		})

		t.Run("MasterUserRecord cannot be blank", func(t *testing.T) {
			// given
			badSBR := sbrtestcommon.NewSpaceBindingRequest("jane", "jane-tenant",
				sbrtestcommon.WithMUR(""), // empty MUR
				sbrtestcommon.WithSpaceRole("admin"))
			member1 := NewMemberClusterWithClient(test.NewFakeClient(t, badSBR, sbrNamespace), "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t, janeSpace, janeMur, base1nsTier)
			ctrl := newReconciler(t, hostClient, member1)

			// when
			_, err = ctrl.Reconcile(context.TODO(), requestFor(badSBR))

			// then
			// an error should be returned
			cause := "MasterUserRecord cannot be blank"
			require.EqualError(t, err, cause)
			// spaceBindingRequest exists with config and finalizer
			sbrtestcommon.AssertThatSpaceBindingRequest(t, badSBR.GetNamespace(), badSBR.GetName(), member1.Client).
				HasSpecSpaceRole("admin").
				HasSpecMasterUserRecord(""). // empty
				HasConditions(sbrtestcommon.UnableToCreateSpaceBinding(cause)).
				HasFinalizer()
			spacebindingtest.AssertThatSpaceBinding(t, test.HostOperatorNs, janeMur.Name, janeSpace.Name, hostClient).DoesNotExist() // there is no spacebinding created
		})

		t.Run("SpaceRole cannot be blank", func(t *testing.T) {
			// given
			badSBR := sbrtestcommon.NewSpaceBindingRequest("jane", "jane-tenant",
				sbrtestcommon.WithMUR(janeMur.Name),
				sbrtestcommon.WithSpaceRole("")) // empty spacerole
			member1 := NewMemberClusterWithClient(test.NewFakeClient(t, badSBR, sbrNamespace), "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t, janeSpace, janeMur, base1nsTier)
			ctrl := newReconciler(t, hostClient, member1)

			// when
			_, err = ctrl.Reconcile(context.TODO(), requestFor(badSBR))

			// then
			// an error should be returned
			cause := "SpaceRole cannot be blank"
			require.EqualError(t, err, cause)
			// spaceBindingRequest exists with config and finalizer
			sbrtestcommon.AssertThatSpaceBindingRequest(t, badSBR.GetNamespace(), badSBR.GetName(), member1.Client).
				HasSpecSpaceRole(""). // empty
				HasSpecMasterUserRecord(janeMur.Name).
				HasConditions(sbrtestcommon.UnableToCreateSpaceBinding(cause)).
				HasFinalizer()
			spacebindingtest.AssertThatSpaceBinding(t, test.HostOperatorNs, janeMur.Name, janeSpace.Name, hostClient).DoesNotExist() // there is no spacebinding created
		})

		t.Run("error creating spaceBinding", func(t *testing.T) {
			// given
			member1 := NewMemberClusterWithClient(test.NewFakeClient(t, sbr, sbrNamespace), "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t, janeSpace, janeMur, base1nsTier)
			hostClient.MockCreate = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.CreateOption) error {
				if _, ok := obj.(*toolchainv1alpha1.SpaceBinding); ok {
					return fmt.Errorf("mock error")
				}
				return hostClient.Client.Create(ctx, obj, opts...)
			}
			ctrl := newReconciler(t, hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sbr))

			// then
			cause := "unable to create SpaceBinding: mock error"
			require.EqualError(t, err, cause)
			// spaceBindingRequest exists with ready condition set to false
			sbrtestcommon.AssertThatSpaceBindingRequest(t, sbr.GetNamespace(), sbr.GetName(), member1.Client).
				HasSpecSpaceRole(sbr.Spec.SpaceRole).
				HasSpecMasterUserRecord(sbr.Spec.MasterUserRecord).
				HasConditions(sbrtestcommon.UnableToCreateSpaceBinding(cause)).
				HasFinalizer()
			spacebindingtest.AssertThatSpaceBinding(t, test.HostOperatorNs, janeMur.Name, janeSpace.Name, hostClient).DoesNotExist() // there is no spacebinding created
		})

		t.Run("error while adding finalizer", func(t *testing.T) {
			member1Client := test.NewFakeClient(t, sbr, sbrNamespace)
			member1Client.MockUpdate = mockUpdateSpaceBindingRequestFail(member1Client)
			member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t, janeSpace, janeMur, base1nsTier)
			ctrl := newReconciler(t, hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sbr))

			// then
			require.EqualError(t, err, "error while adding finalizer: mock error")
		})

		t.Run("spaceBindingRequest namespace not found", func(t *testing.T) {
			member1Client := test.NewFakeClient(t, sbr)
			member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t, base1nsTier)
			ctrl := newReconciler(t, hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sbr))

			// then
			cause := "unable to get namespace: namespaces \"jane-tenant\" not found"
			require.EqualError(t, err, cause)
			sbrtestcommon.AssertThatSpaceBindingRequest(t, sbr.GetNamespace(), sbr.GetName(), member1.Client).
				HasSpecSpaceRole(sbr.Spec.SpaceRole).
				HasSpecMasterUserRecord(sbr.Spec.MasterUserRecord).
				HasConditions(sbrtestcommon.UnableToCreateSpaceBinding(cause)).
				HasFinalizer()
		})

		t.Run("unable to find space label in spaceBindingRequest namespace", func(t *testing.T) {
			// given
			sbrNamespace := spacerequesttest.NewNamespace("nospace")
			sbr := sbrtestcommon.NewSpaceBindingRequest("jane", sbrNamespace.GetName())
			member1Client := test.NewFakeClient(t, sbr, sbrNamespace)
			member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t, base1nsTier)
			ctrl := newReconciler(t, hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sbr))

			// then
			cause := "unable to find space label toolchain.dev.openshift.com/space on namespace nospace-tenant"
			require.EqualError(t, err, cause)
			sbrtestcommon.AssertThatSpaceBindingRequest(t, sbr.GetNamespace(), sbr.GetName(), member1.Client).
				HasSpecSpaceRole(sbr.Spec.SpaceRole).
				HasSpecMasterUserRecord(sbr.Spec.MasterUserRecord).
				HasConditions(sbrtestcommon.UnableToCreateSpaceBinding(cause)).
				HasFinalizer()
		})

		t.Run("unable to get space", func(t *testing.T) {
			// given
			member1Client := test.NewFakeClient(t, sbr, sbrNamespace)
			member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t, base1nsTier)
			hostClient.MockGet = func(ctx context.Context, key runtimeclient.ObjectKey, obj runtimeclient.Object, opts ...runtimeclient.GetOption) error {
				if _, ok := obj.(*toolchainv1alpha1.Space); ok {
					return fmt.Errorf("mock error")
				}
				return hostClient.Client.Get(ctx, key, obj, opts...)
			}
			ctrl := newReconciler(t, hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sbr))

			// then
			cause := "unable to get space: mock error"
			require.EqualError(t, err, cause)
			sbrtestcommon.AssertThatSpaceBindingRequest(t, sbr.GetNamespace(), sbr.GetName(), member1.Client).
				HasSpecSpaceRole(sbr.Spec.SpaceRole).
				HasSpecMasterUserRecord(sbr.Spec.MasterUserRecord).
				HasConditions(sbrtestcommon.UnableToCreateSpaceBinding(cause)).
				HasFinalizer()
		})

		t.Run("space is being deleted", func(t *testing.T) {
			// given
			member1 := NewMemberClusterWithClient(test.NewFakeClient(t, sbr, sbrNamespace), "member-1", corev1.ConditionTrue)
			janeSpace := spacetest.NewSpace(test.HostOperatorNs, "jane",
				spacetest.WithDeletionTimestamp()) // space is being deleted ...
			hostClient := test.NewFakeClient(t, base1nsTier, janeSpace)
			ctrl := newReconciler(t, hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sbr))

			// then
			cause := "space is being deleted"
			require.EqualError(t, err, cause)
			sbrtestcommon.AssertThatSpaceBindingRequest(t, sbr.GetNamespace(), sbr.GetName(), member1.Client).
				HasSpecSpaceRole(sbr.Spec.SpaceRole).
				HasSpecMasterUserRecord(sbr.Spec.MasterUserRecord).
				HasConditions(sbrtestcommon.UnableToCreateSpaceBinding(cause)).
				HasFinalizer()
		})

		t.Run("unable to get MUR", func(t *testing.T) {
			// given
			member1 := NewMemberClusterWithClient(test.NewFakeClient(t, sbr, sbrNamespace), "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t, base1nsTier, janeSpace)
			hostClient.MockGet = func(ctx context.Context, key runtimeclient.ObjectKey, obj runtimeclient.Object, opts ...runtimeclient.GetOption) error {
				if _, ok := obj.(*toolchainv1alpha1.MasterUserRecord); ok {
					return fmt.Errorf("mock error")
				}
				return hostClient.Client.Get(ctx, key, obj, opts...)
			}
			ctrl := newReconciler(t, hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sbr))

			// then
			cause := "unable to get MUR: mock error"
			require.EqualError(t, err, cause)
			sbrtestcommon.AssertThatSpaceBindingRequest(t, sbr.GetNamespace(), sbr.GetName(), member1.Client).
				HasSpecSpaceRole(sbr.Spec.SpaceRole).
				HasSpecMasterUserRecord(sbr.Spec.MasterUserRecord).
				HasConditions(sbrtestcommon.UnableToCreateSpaceBinding(cause)).
				HasFinalizer()
		})

		t.Run("mur is being deleted", func(t *testing.T) {
			// given
			member1 := NewMemberClusterWithClient(test.NewFakeClient(t, sbr, sbrNamespace), "member-1", corev1.ConditionTrue)
			janeMur := masteruserrecord.NewMasterUserRecord(t, "jane", masteruserrecord.ToBeDeleted())
			hostClient := test.NewFakeClient(t, base1nsTier, janeSpace, janeMur)
			ctrl := newReconciler(t, hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sbr))

			// then
			cause := "mur is being deleted"
			require.EqualError(t, err, cause)
			sbrtestcommon.AssertThatSpaceBindingRequest(t, sbr.GetNamespace(), sbr.GetName(), member1.Client).
				HasSpecSpaceRole(sbr.Spec.SpaceRole).
				HasSpecMasterUserRecord(sbr.Spec.MasterUserRecord).
				HasConditions(sbrtestcommon.UnableToCreateSpaceBinding(cause)).
				HasFinalizer()
		})

		t.Run("unable to get NSTemplateTier", func(t *testing.T) {
			// given
			member1 := NewMemberClusterWithClient(test.NewFakeClient(t, sbr, sbrNamespace), "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t, janeSpace, janeMur)
			ctrl := newReconciler(t, hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sbr))

			// then
			cause := "unable to get the current NSTemplateTier: nstemplatetiers.toolchain.dev.openshift.com \"base1ns\" not found"
			require.EqualError(t, err, cause)
			sbrtestcommon.AssertThatSpaceBindingRequest(t, sbr.GetNamespace(), sbr.GetName(), member1.Client).
				HasSpecSpaceRole(sbr.Spec.SpaceRole).
				HasSpecMasterUserRecord(sbr.Spec.MasterUserRecord).
				HasConditions(sbrtestcommon.UnableToCreateSpaceBinding(cause)).
				HasFinalizer()
		})

		t.Run("invalid SpaceRole", func(t *testing.T) {
			// given
			invalidSBR := sbrtestcommon.NewSpaceBindingRequest("jane", sbrNamespace.GetName(), sbrtestcommon.WithSpaceRole("maintainer"), sbrtestcommon.WithMUR(janeMur.Name)) // invalid role
			member1 := NewMemberClusterWithClient(test.NewFakeClient(t, invalidSBR, sbrNamespace), "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t, base1nsTier, janeSpace, janeMur)
			ctrl := newReconciler(t, hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(invalidSBR))

			// then
			cause := "invalid role 'maintainer' for space 'jane'"
			require.EqualError(t, err, cause)
			sbrtestcommon.AssertThatSpaceBindingRequest(t, invalidSBR.GetNamespace(), invalidSBR.GetName(), member1.Client).
				HasConditions(sbrtestcommon.UnableToCreateSpaceBinding(cause)).
				HasFinalizer()
		})

		t.Run("error listing SpaceBindings", func(t *testing.T) {
			// given
			member1 := NewMemberClusterWithClient(test.NewFakeClient(t, sbr, sbrNamespace), "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t, base1nsTier, janeSpace, janeMur)
			hostClient.MockList = func(ctx context.Context, list runtimeclient.ObjectList, opts ...runtimeclient.ListOption) error {
				if _, ok := list.(*toolchainv1alpha1.SpaceBindingList); ok {
					return fmt.Errorf("mock error")
				}
				return hostClient.Client.List(ctx, list, opts...)
			}
			ctrl := newReconciler(t, hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sbr))

			// then
			require.EqualError(t, err, "unable to list spacebindings: mock error")
		})

		t.Run("SpaceBinding is being deleted", func(t *testing.T) {
			// given
			spaceBinding := spacebindingtest.NewSpaceBinding(janeMur.Name, janeSpace.Name, "admin", sbr.GetName(), spacebindingtest.WithSpaceBindingRequest(sbr), spacebindingtest.WithDeletionTimestamp())
			member1 := NewMemberClusterWithClient(test.NewFakeClient(t, sbr, sbrNamespace), "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t, base1nsTier, janeSpace, janeMur, spaceBinding)
			ctrl := newReconciler(t, hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sbr))

			// then
			cause := "cannot update SpaceBinding because it is currently being deleted"
			require.EqualError(t, err, cause)
			sbrtestcommon.AssertThatSpaceBindingRequest(t, sbr.GetNamespace(), sbr.GetName(), member1.Client).
				HasConditions(sbrtestcommon.UnableToCreateSpaceBinding(cause)).
				HasFinalizer()
		})

		t.Run("There are more than one SpaceBinding for the given MUR and Space", func(t *testing.T) {
			// we have an SBR that will try to create the same SpaceBinding
			sbrForDuplicatedSpaceBinding := sbrtestcommon.NewSpaceBindingRequest("jane", "jane-tenant",
				sbrtestcommon.WithMUR(janeMur.Name),
				sbrtestcommon.WithSpaceRole("admin"))

			// given
			spaceBinding1 := spacebindingcommon.NewSpaceBinding(janeMur, janeSpace, "john") // there is already an admin generated SpaceBinding
			spaceBinding2 := spaceBinding1.DeepCopy()                                       // there is another spacebinding (this should not happen)
			spaceBinding2.Name = "somerandom name"                                          // the name doesn't matter since spacebindings are retrieved using lables
			member1 := NewMemberClusterWithClient(test.NewFakeClient(t, sbrNamespace, sbrForDuplicatedSpaceBinding), "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t, base1nsTier, janeSpace, janeMur, spaceBinding1, spaceBinding2)
			ctrl := newReconciler(t, hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sbrForDuplicatedSpaceBinding))

			// then
			cause := "expected 1 spacebinding for Space jane and MUR jane. But found 2"
			require.EqualError(t, err, cause)
			sbrtestcommon.AssertThatSpaceBindingRequest(t, sbr.GetNamespace(), sbr.GetName(), member1.Client).
				HasConditions(sbrtestcommon.UnableToCreateSpaceBinding(cause)).
				HasFinalizer()
		})

		t.Run("SpaceBinding not managed by this SpaceBindingRequest CR", func(t *testing.T) {
			// given
			spaceBinding := spacebindingcommon.NewSpaceBinding(janeMur, janeSpace, "john") // there is already an admin generated SpaceBinding
			// this SBR will fail for the conflict with the already existing SpaceBinding
			sbrForDuplicatedSpaceBinding := sbrtestcommon.NewSpaceBindingRequest("jane", "jane-tenant",
				sbrtestcommon.WithMUR(janeMur.Name),
				sbrtestcommon.WithSpaceRole("admin"))

			member1 := NewMemberClusterWithClient(test.NewFakeClient(t, sbrNamespace, sbrForDuplicatedSpaceBinding), "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t, base1nsTier, janeSpace, janeMur, spaceBinding)
			ctrl := newReconciler(t, hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sbrForDuplicatedSpaceBinding))

			// then
			cause := fmt.Sprintf("A SpaceBinding for Space '%s' and MUR '%s' already exists, but it's not managed by this SpaceBindingRequest CR. It's not allowed to create multiple SpaceBindings for the same combination of Space and MasterUserRecord", janeSpace.GetName(), janeMur.GetName())
			require.EqualError(t, err, cause)
			sbrtestcommon.AssertThatSpaceBindingRequest(t, sbr.GetNamespace(), sbr.GetName(), member1.Client).
				HasConditions(sbrtestcommon.UnableToCreateSpaceBinding(cause)).
				HasFinalizer()
		})

		t.Run("SpaceBindingRequest CR is deleted but admin-generated SpaceBinding stays there", func(t *testing.T) {
			// given
			spaceBinding := spacebindingcommon.NewSpaceBinding(janeMur, janeSpace, "john") // there is already an admin generated SpaceBinding
			// this SBR will fail for the conflict with the already existing SpaceBinding
			// but we should be able to delete it
			sbrForDuplicatedSpaceBinding := sbrtestcommon.NewSpaceBindingRequest("jane", "jane-tenant",
				sbrtestcommon.WithMUR(janeMur.Name),
				sbrtestcommon.WithSpaceRole("admin"),
				sbrtestcommon.WithFinalizer(), // we set the finalizer so we can check if it's being removed
				sbrtestcommon.WithDeletionTimestamp(),
			)

			member1 := NewMemberClusterWithClient(test.NewFakeClient(t, sbrNamespace, sbrForDuplicatedSpaceBinding), "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t, base1nsTier, janeSpace, janeMur, spaceBinding)
			ctrl := newReconciler(t, hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sbrForDuplicatedSpaceBinding))

			// then
			require.NoError(t, err)
			// finalizer should be removed
			sbrtestcommon.AssertThatSpaceBindingRequest(t, sbr.GetNamespace(), sbr.GetName(), member1.Client).
				DoesNotExist()
			// the spacebinding should not be deleted, since it doesn't belong to the SBR
			spacebindingtest.AssertThatSpaceBinding(t, test.HostOperatorNs, janeMur.Name, janeSpace.Name, hostClient).
				Exists()
		})

	})
}

func TestUpdateSpaceBindingRequest(t *testing.T) {
	// given
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	err := apis.AddToScheme(scheme.Scheme)
	require.NoError(t, err)
	base1nsTier := tiertest.Base1nsTier(t, tiertest.CurrentBase1nsTemplates)
	janeSpace := spacetest.NewSpace(test.HostOperatorNs, "jane")
	janeMur := masteruserrecord.NewMasterUserRecord(t, "jane")
	sbrNamespace := spacerequesttest.NewNamespace("jane")
	t.Run("success", func(t *testing.T) {

		t.Run("update SpaceRole", func(t *testing.T) {
			// given
			sbr := sbrtestcommon.NewSpaceBindingRequest("jane", "jane-tenant",
				sbrtestcommon.WithMUR("jane"),
				sbrtestcommon.WithSpaceRole("admin"))
			member1 := NewMemberClusterWithClient(test.NewFakeClient(t, sbr, sbrNamespace), "member-1", corev1.ConditionTrue)
			spaceBinding := spacebindingtest.NewSpaceBinding(janeMur.Name, janeSpace.Name, "maintainer", sbr.Name, spacebindingtest.WithSpaceBindingRequest(sbr)) // jane has maintainer, but SBR has admin
			hostClient := test.NewFakeClient(t, base1nsTier, spaceBinding, janeSpace, janeMur)
			ctrl := newReconciler(t, hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sbr))

			// then
			require.NoError(t, err)
			// spacebindingrequest exists with expected config and finalizer
			sbrtestcommon.AssertThatSpaceBindingRequest(t, sbr.GetNamespace(), sbr.GetName(), member1.Client).
				HasSpecSpaceRole("admin").
				HasSpecMasterUserRecord(janeMur.Name).
				HasFinalizer()
			// spacebinding was updated
			spacebindingtest.AssertThatSpaceBinding(t, test.HostOperatorNs, janeMur.Name, janeSpace.Name, hostClient).
				Exists().
				HasSpec(janeMur.Name, janeSpace.Name, "admin") // check that admin was set
		})
	})

	t.Run("failure", func(t *testing.T) {
		t.Run("unable to update SpaceBinding", func(t *testing.T) {
			sbr := sbrtestcommon.NewSpaceBindingRequest("jane", "jane-tenant",
				sbrtestcommon.WithMUR("jane"),
				sbrtestcommon.WithSpaceRole("admin"))
			// given
			spaceBinding := spacebindingtest.NewSpaceBinding(janeMur.Name, janeSpace.Name, "oldrole", sbr.GetName(), spacebindingtest.WithSpaceBindingRequest(sbr)) // spacebinding role needs to be updated
			member1 := NewMemberClusterWithClient(test.NewFakeClient(t, sbr, sbrNamespace), "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t, base1nsTier, janeSpace, janeMur, spaceBinding)
			hostClient.MockUpdate = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.UpdateOption) error {
				if _, ok := obj.(*toolchainv1alpha1.SpaceBinding); ok {
					return fmt.Errorf("mock error")
				}
				return hostClient.Client.Update(ctx, obj, opts...)
			}
			ctrl := newReconciler(t, hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sbr))

			// then
			cause := "unable to update SpaceRole and MasterUserRecord fields: mock error"
			require.EqualError(t, err, cause)
			sbrtestcommon.AssertThatSpaceBindingRequest(t, sbr.GetNamespace(), sbr.GetName(), member1.Client).
				HasConditions(sbrtestcommon.UnableToCreateSpaceBinding(cause)).
				HasFinalizer()
		})
	})
}

func TestDeleteSpaceBindingRequest(t *testing.T) {
	// given
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	err := apis.AddToScheme(scheme.Scheme)
	require.NoError(t, err)
	base1nsTier := tiertest.Base1nsTier(t, tiertest.CurrentBase1nsTemplates)
	janeSpace := spacetest.NewSpace(test.HostOperatorNs, "jane")
	janeMur := masteruserrecord.NewMasterUserRecord(t, "jane")
	sbrNamespace := spacerequesttest.NewNamespace("jane")
	sbr := sbrtestcommon.NewSpaceBindingRequest("jane", "jane-tenant",
		sbrtestcommon.WithMUR("jane"),
		sbrtestcommon.WithSpaceRole("admin"),
		sbrtestcommon.WithDeletionTimestamp(), // spaceBindingRequest was deleted
		sbrtestcommon.WithFinalizer())         // has finalizer still
	t.Run("success", func(t *testing.T) {
		t.Run("spaceBindingRequest should be in terminating while spacebinding is deleted", func(t *testing.T) {
			// given
			member1 := NewMemberClusterWithClient(test.NewFakeClient(t, sbr, sbrNamespace), "member-1", corev1.ConditionTrue)
			spaceBinding := spacebindingtest.NewSpaceBinding(janeMur.Name, janeSpace.Name, "maintainer", sbr.Name, spacebindingtest.WithSpaceBindingRequest(sbr)) // jane has maintainer, but SBR has admin
			hostClient := test.NewFakeClient(t, base1nsTier, spaceBinding, janeSpace, janeMur)
			ctrl := newReconciler(t, hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sbr))

			// then
			require.NoError(t, err)
			// spacebindingrequest exists with expected config and finalizer
			sbrtestcommon.AssertThatSpaceBindingRequest(t, sbr.GetNamespace(), sbr.GetName(), member1.Client).
				HasSpecSpaceRole(sbr.Spec.SpaceRole).
				HasSpecMasterUserRecord(janeMur.Name).
				HasConditions(sbrtestcommon.Terminating()).
				HasFinalizer()
			// spacebinding was deleted
			spacebindingtest.AssertThatSpaceBinding(t, test.HostOperatorNs, janeMur.Name, janeSpace.Name, hostClient).
				DoesNotExist()

			t.Run("spaceBindingRequest is deleted when spacebinding is gone", func(t *testing.T) {
				// when
				_, err := ctrl.Reconcile(context.TODO(), requestFor(sbr))

				// then
				// spaceBindingRequest was deleted
				require.NoError(t, err)
				sbrtestcommon.AssertThatSpaceBindingRequest(t, sbr.GetNamespace(), sbr.GetName(), member1.Client).DoesNotExist() // spaceBindingRequest is gone
			})
		})

		t.Run("finalizer was already removed", func(t *testing.T) {
			// given
			// spaceBindingRequest has no finalizer
			sbrNoFinalizer := sbrtestcommon.NewSpaceBindingRequest("lana", "lana-tenant",
				sbrtestcommon.WithMUR("lana"),
				sbrtestcommon.WithSpaceRole("admin"),
				sbrtestcommon.WithDeletionTimestamp()) // spaceBindingRequest was deleted
			member1 := NewMemberClusterWithClient(test.NewFakeClient(t, sbrNoFinalizer, sbrNamespace), "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t, base1nsTier, janeSpace, janeMur)
			ctrl := newReconciler(t, hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sbrNoFinalizer))

			// then
			require.NoError(t, err)
			sbrtestcommon.AssertThatSpaceBindingRequest(t, sbrNoFinalizer.GetNamespace(), sbrNoFinalizer.GetName(), member1.Client).HasNoFinalizers() // spaceBindingRequest is gone
		})
	})

	t.Run("failure", func(t *testing.T) {
		t.Run("SpaceBinding resource is already being deleted for more than 2 minutes", func(t *testing.T) {
			// given
			sbr := sbrtestcommon.NewSpaceBindingRequest("jane", sbrNamespace.GetName(),
				sbrtestcommon.WithDeletionTimestamp(),
				sbrtestcommon.WithFinalizer(),
				sbrtestcommon.WithMUR(janeMur.Name),
			) // sbr is being deleted
			spaceBinding := spacebindingtest.NewSpaceBinding(janeMur.Name, janeSpace.Name, "admin", sbr.Name, spacebindingtest.WithSpaceBindingRequest(sbr))
			spaceBinding.DeletionTimestamp = &metav1.Time{Time: time.Now().Add(-121 * time.Second)} // is being deleted since more than 2 minutes
			member1 := NewMemberClusterWithClient(test.NewFakeClient(t, sbr, sbrNamespace), "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t, base1nsTier, janeSpace, janeMur, spaceBinding)
			ctrl := newReconciler(t, hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sbr))

			// then
			cause := "spacebinding deletion has not completed in over 2 minutes"
			require.EqualError(t, err, cause)
			sbrtestcommon.AssertThatSpaceBindingRequest(t, sbr.GetNamespace(), sbr.GetName(), member1.Client).
				HasConditions(sbrtestcommon.TerminatingFailed(cause)).
				HasFinalizer()
		})

		t.Run("unable to delete SpaceBinding", func(t *testing.T) {
			// given
			sbr := sbrtestcommon.NewSpaceBindingRequest("jane", sbrNamespace.GetName(),
				sbrtestcommon.WithDeletionTimestamp(),
				sbrtestcommon.WithFinalizer(),
				sbrtestcommon.WithMUR(janeMur.Name),
			) // sbr is being deleted
			spaceBinding := spacebindingtest.NewSpaceBinding(janeMur.Name, janeSpace.Name, "admin", sbr.Name, spacebindingtest.WithSpaceBindingRequest(sbr))
			member1 := NewMemberClusterWithClient(test.NewFakeClient(t, sbr, sbrNamespace), "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t, base1nsTier, janeSpace, janeMur, spaceBinding)
			hostClient.MockDelete = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.DeleteOption) error {
				if _, ok := obj.(*toolchainv1alpha1.SpaceBinding); ok {
					return fmt.Errorf("mock error")
				}
				return hostClient.Client.Delete(ctx, obj, opts...)
			}
			ctrl := newReconciler(t, hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sbr))

			// then
			cause := "unable to delete spacebinding: mock error"
			require.EqualError(t, err, cause)
			sbrtestcommon.AssertThatSpaceBindingRequest(t, sbr.GetNamespace(), sbr.GetName(), member1.Client).
				HasConditions(sbrtestcommon.TerminatingFailed(cause)).
				HasFinalizer()
		})

		t.Run("failed to remove finalizer", func(t *testing.T) {
			// given
			member1Client := test.NewFakeClient(t, sbr, sbrNamespace)
			member1Client.MockUpdate = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.UpdateOption) error {
				if _, ok := obj.(*toolchainv1alpha1.SpaceBindingRequest); ok {
					return fmt.Errorf("mock error")
				}
				return member1Client.Client.Update(ctx, obj, opts...)
			}
			member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
			hostClient := test.NewFakeClient(t, janeSpace)
			ctrl := newReconciler(t, hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(sbr))

			// then
			require.EqualError(t, err, "failed to remove finalizer: mock error")
		})
	})
}

func newReconciler(t *testing.T, hostCl runtimeclient.Client, memberClusters ...*commoncluster.CachedToolchainCluster) *spacebindingrequest.Reconciler {
	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)

	clusters := map[string]cluster.Cluster{}
	for _, c := range memberClusters {
		clusters[c.Name] = cluster.Cluster{
			Config: &commoncluster.Config{
				OperatorNamespace: c.OperatorNamespace,
				OwnerClusterName:  test.MemberClusterName,
			},
			Client: c.Client,
		}
	}
	return &spacebindingrequest.Reconciler{
		Client:         hostCl,
		Scheme:         s,
		Namespace:      test.HostOperatorNs,
		MemberClusters: clusters,
	}
}

func requestFor(s *toolchainv1alpha1.SpaceBindingRequest) reconcile.Request {
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

func mockGetSpaceBindingRequestFail(cl runtimeclient.Client) func(ctx context.Context, key runtimeclient.ObjectKey, obj runtimeclient.Object, opts ...runtimeclient.GetOption) error {
	return func(ctx context.Context, key runtimeclient.ObjectKey, obj runtimeclient.Object, opts ...runtimeclient.GetOption) error {
		if _, ok := obj.(*toolchainv1alpha1.SpaceBindingRequest); ok {
			return fmt.Errorf("mock error")
		}
		return cl.Get(ctx, key, obj, opts...)
	}
}

func mockUpdateSpaceBindingRequestFail(cl runtimeclient.Client) func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.UpdateOption) error {
	return func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.UpdateOption) error {
		if _, ok := obj.(*toolchainv1alpha1.SpaceBindingRequest); ok {
			return fmt.Errorf("mock error")
		}
		return cl.Update(ctx, obj, opts...)
	}
}
