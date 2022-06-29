package socialevent_test

import (
	"context"
	"fmt"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/controllers/socialevent"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	tiertest "github.com/codeready-toolchain/host-operator/test/nstemplatetier"
	socialeventtest "github.com/codeready-toolchain/host-operator/test/socialevent"
	"github.com/codeready-toolchain/host-operator/test/usertier"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	commonsignup "github.com/codeready-toolchain/toolchain-common/pkg/test/usersignup"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestReconcileSocialEvent(t *testing.T) {

	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	err := apis.AddToScheme(scheme.Scheme)
	require.NoError(t, err)
	baseSpaceTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates)
	baseUserTier := usertier.NewUserTier("deactivate30", 30)

	t.Run("valid tier", func(t *testing.T) {
		// given
		event := socialeventtest.NewSocialEvent("deactivate30", "basic")
		hostClient := test.NewFakeClient(t, event, baseUserTier, baseSpaceTier)
		ctrl := newReconciler(hostClient)

		// when
		_, err := ctrl.Reconcile(context.TODO(), requestFor(event))

		// then
		require.NoError(t, err)

		// check the social event status
		socialeventtest.AssertThatSocialEvent(t, test.HostOperatorNs, event.Name, hostClient).HasConditions(
			toolchainv1alpha1.Condition{
				Type:   toolchainv1alpha1.ConditionReady,
				Status: corev1.ConditionTrue,
			},
		)
	})

	t.Run("with usersignups", func(t *testing.T) {
		// given 2 approved users and 1 not yet approved
		event := socialeventtest.NewSocialEvent("deactivate30", "basic")
		approvedUser1 := commonsignup.NewUserSignup(commonsignup.WithName("user1"),
			commonsignup.WithStateLabel(toolchainv1alpha1.UserSignupStateLabelValueApproved),
			commonsignup.WithLabel(toolchainv1alpha1.SocialEventUserSignupLabelKey, event.Name))
		approvedUser2 := commonsignup.NewUserSignup(commonsignup.WithName("user2"),
			commonsignup.WithStateLabel(toolchainv1alpha1.UserSignupStateLabelValueApproved),
			commonsignup.WithLabel(toolchainv1alpha1.SocialEventUserSignupLabelKey, event.Name))
		unapprovedUser3 := commonsignup.NewUserSignup(commonsignup.WithName("user3"),
			commonsignup.WithLabel(toolchainv1alpha1.SocialEventUserSignupLabelKey, event.Name))
		hostClient := test.NewFakeClient(t, event, baseUserTier, baseSpaceTier, approvedUser1, approvedUser2, unapprovedUser3)
		ctrl := newReconciler(hostClient)

		// when
		res, err := ctrl.Reconcile(context.TODO(), requestFor(event))

		// then
		require.NoError(t, err)
		assert.False(t, res.Requeue)
		// check the social event status
		socialeventtest.AssertThatSocialEvent(t, test.HostOperatorNs, event.Name, hostClient).
			HasStatusActivations(2).
			HasConditions(
				toolchainv1alpha1.Condition{
					Type:   toolchainv1alpha1.ConditionReady,
					Status: corev1.ConditionTrue,
				},
			)
	})

	t.Run("failures", func(t *testing.T) {

		t.Run("unable to get user tier", func(t *testing.T) {
			// given
			event := socialeventtest.NewSocialEvent("notfound", "basic")
			hostClient := test.NewFakeClient(t, event, baseUserTier, baseSpaceTier)
			hostClient.MockGet = func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
				if _, ok := obj.(*toolchainv1alpha1.UserTier); ok && key.Name == "notfound" {
					return fmt.Errorf("mock error")
				}
				return hostClient.Client.Get(ctx, key, obj)
			}
			ctrl := newReconciler(hostClient)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(event))

			// then
			require.Error(t, err)
			assert.EqualError(t, err, "unable to get the 'notfound' UserTier: mock error")
			// check the social event status
			socialeventtest.AssertThatSocialEvent(t, test.HostOperatorNs, event.Name, hostClient).
				HasConditions(toolchainv1alpha1.Condition{
					Type:    toolchainv1alpha1.ConditionReady,
					Status:  corev1.ConditionFalse,
					Reason:  toolchainv1alpha1.SocialEventUnableToGetUserTierReason,
					Message: "unable to get the 'notfound' UserTier: mock error",
				})
		})

		t.Run("unknown user tier", func(t *testing.T) {
			// given
			event := socialeventtest.NewSocialEvent("unknown", "basic")
			hostClient := test.NewFakeClient(t, event, baseUserTier, baseSpaceTier)
			ctrl := newReconciler(hostClient)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(event))

			// then
			require.NoError(t, err)
			// check the social event status
			socialeventtest.AssertThatSocialEvent(t, test.HostOperatorNs, event.Name, hostClient).HasConditions(
				toolchainv1alpha1.Condition{
					Type:    toolchainv1alpha1.ConditionReady,
					Status:  corev1.ConditionFalse,
					Reason:  toolchainv1alpha1.SocialEventInvalidUserTierReason,
					Message: "UserTier 'unknown' not found",
				},
			)
		})

		t.Run("unable to get space tier", func(t *testing.T) {
			// given
			event := socialeventtest.NewSocialEvent("deactivate30", "notfound")
			hostClient := test.NewFakeClient(t, event, baseUserTier, baseSpaceTier)
			hostClient.MockGet = func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
				if _, ok := obj.(*toolchainv1alpha1.NSTemplateTier); ok && key.Name == "notfound" {
					return fmt.Errorf("mock error")
				}
				return hostClient.Client.Get(ctx, key, obj)
			}
			ctrl := newReconciler(hostClient)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(event))

			// then
			require.Error(t, err)
			assert.EqualError(t, err, "unable to get the 'notfound' NSTemplateTier: mock error")
			// check the social event status
			socialeventtest.AssertThatSocialEvent(t, test.HostOperatorNs, event.Name, hostClient).
				HasConditions(toolchainv1alpha1.Condition{
					Type:    toolchainv1alpha1.ConditionReady,
					Status:  corev1.ConditionFalse,
					Reason:  toolchainv1alpha1.SocialEventUnableToGetSpaceTierReason,
					Message: "unable to get the 'notfound' NSTemplateTier: mock error",
				})
		})

		t.Run("unknown space tier", func(t *testing.T) {
			// given
			event := socialeventtest.NewSocialEvent("deactivate30", "unknown")
			hostClient := test.NewFakeClient(t, event, baseUserTier, baseSpaceTier)
			ctrl := newReconciler(hostClient)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(event))

			// then
			require.NoError(t, err)
			// check the social event status
			socialeventtest.AssertThatSocialEvent(t, test.HostOperatorNs, event.Name, hostClient).HasConditions(
				toolchainv1alpha1.Condition{
					Type:    toolchainv1alpha1.ConditionReady,
					Status:  corev1.ConditionFalse,
					Reason:  toolchainv1alpha1.SocialEventInvalidSpaceTierReason,
					Message: "NSTemplateTier 'unknown' not found",
				},
			)
		})
	})
}

func newReconciler(hostClient client.Client) *socialevent.Reconciler {
	return &socialevent.Reconciler{
		Client:    hostClient,
		Namespace: test.HostOperatorNs,
		StatusUpdater: &socialevent.StatusUpdater{
			Client: hostClient,
		},
	}
}

func requestFor(s *toolchainv1alpha1.SocialEvent) reconcile.Request {
	return reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: s.Namespace,
			Name:      s.Name,
		},
	}
}
