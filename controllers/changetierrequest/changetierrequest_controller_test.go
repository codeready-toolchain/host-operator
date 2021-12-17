package changetierrequest

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	tierutil "github.com/codeready-toolchain/host-operator/controllers/nstemplatetier/util"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	"github.com/codeready-toolchain/host-operator/pkg/templates/nstemplatetiers"
	. "github.com/codeready-toolchain/host-operator/test"
	spacetest "github.com/codeready-toolchain/host-operator/test/space"
	commonconfig "github.com/codeready-toolchain/toolchain-common/pkg/configuration"
	"github.com/codeready-toolchain/toolchain-common/pkg/states"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	testconfig "github.com/codeready-toolchain/toolchain-common/pkg/test/config"
	murtest "github.com/codeready-toolchain/toolchain-common/pkg/test/masteruserrecord"

	"github.com/spf13/cast"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apiv1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestChangeTierSuccess(t *testing.T) {
	// given
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	config := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.Tiers().DurationBeforeChangeTierRequestDeletion("10s"))
	teamTier := NewNSTemplateTier("team", "123team", "123clusterteam", "stage", "dev")

	userSignup := NewUserSignup()
	t.Run("should update tier in MUR", func(t *testing.T) {
		// given
		mur := murtest.NewMasterUserRecord(t, "john", murtest.WithOwnerLabel(userSignup.Name))
		changeTierRequest := newChangeTierRequest("john", "team")
		controller, request, cl := newController(t, changeTierRequest, config, userSignup, mur, teamTier)

		// when
		_, err := controller.Reconcile(context.TODO(), request)

		// then
		require.NoError(t, err)
		murtest.AssertThatMasterUserRecord(t, "john", cl).
			HasTier(*teamTier).
			AllUserAccountsHaveTier(*teamTier).
			DoesNotHaveLabel(tierutil.TemplateTierHashLabelKey(murtest.DefaultNSTemplateTierName))
		AssertThatChangeTierRequestHasCondition(t, cl, changeTierRequest.Name, toBeComplete())
	})

	t.Run("should update tier in all UserAccounts in MUR", func(t *testing.T) {
		// given
		mur := murtest.NewMasterUserRecord(t, "johny", murtest.WithOwnerLabel(userSignup.Name), murtest.AdditionalAccounts("another-cluster"))
		changeTierRequest := newChangeTierRequest("johny", "team")
		controller, request, cl := newController(t, changeTierRequest, config, userSignup, mur, teamTier)

		// when
		_, err := controller.Reconcile(context.TODO(), request)

		// then
		require.NoError(t, err)
		murtest.AssertThatMasterUserRecord(t, "johny", cl).
			HasTier(*teamTier).
			AllUserAccountsHaveTier(*teamTier).
			DoesNotHaveLabel(tierutil.TemplateTierHashLabelKey(murtest.DefaultNSTemplateTierName))
		AssertThatChangeTierRequestHasCondition(t, cl, changeTierRequest.Name, toBeComplete())
	})

	t.Run("should update tier only in specified UserAccount in MUR", func(t *testing.T) {
		// given
		mur := murtest.NewMasterUserRecord(t, "johny", murtest.WithOwnerLabel(userSignup.Name), murtest.AdditionalAccounts("another-cluster"))
		changeTierRequest := newChangeTierRequest("johny", "team", targetCluster("another-cluster"))
		controller, request, cl := newController(t, changeTierRequest, config, userSignup, mur, teamTier)

		// when
		_, err := controller.Reconcile(context.TODO(), request)

		// then
		require.NoError(t, err)
		murtest.AssertThatMasterUserRecord(t, "johny", cl).
			UserAccountHasTier("another-cluster", *teamTier).
			UserAccountHasTier(test.MemberClusterName, murtest.DefaultNSTemplateTier())
		AssertThatChangeTierRequestHasCondition(t, cl, changeTierRequest.Name, toBeComplete())
	})

	t.Run("completed changetierrequest is requeued with the remaining deletion timeout", func(t *testing.T) {
		// given
		mur := murtest.NewMasterUserRecord(t, "johny", murtest.WithOwnerLabel(userSignup.Name))
		changeTierRequest := newChangeTierRequest("johny", "team")
		changeTierRequest.Status.Conditions = []toolchainv1alpha1.Condition{toBeComplete()}
		controller, request, cl := newController(t, changeTierRequest, config, userSignup, mur, teamTier)

		// when
		result, err := controller.Reconcile(context.TODO(), request)

		// then
		require.NoError(t, err)
		assert.True(t, result.Requeue)
		assert.True(t, result.RequeueAfter < cast.ToDuration("10s"))
		assert.True(t, result.RequeueAfter > cast.ToDuration("1s"))
		murtest.AssertThatMasterUserRecord(t, "johny", cl).
			HasTier(murtest.DefaultNSTemplateTier()).
			AllUserAccountsHaveTier(murtest.DefaultNSTemplateTier())
		AssertThatChangeTierRequestHasCondition(t, cl, changeTierRequest.Name, toBeComplete())
	})

	t.Run("changetierrequest is deleted when deletion timeout passed", func(t *testing.T) {
		// given
		mur := murtest.NewMasterUserRecord(t, "johny", murtest.WithOwnerLabel(userSignup.Name))
		changeTierRequest := newChangeTierRequest("johny", "team")
		changeTierRequest.Status.Conditions = []toolchainv1alpha1.Condition{toBeComplete()}
		changeTierRequest.Status.Conditions[0].LastTransitionTime = v1.Time{Time: time.Now().Add(-cast.ToDuration("10s"))}
		controller, request, cl := newController(t, changeTierRequest, config, userSignup, mur, teamTier)

		// when
		result, err := controller.Reconcile(context.TODO(), request)

		// then
		require.NoError(t, err)
		assert.False(t, result.Requeue)
		murtest.AssertThatMasterUserRecord(t, "johny", cl).
			HasTier(murtest.DefaultNSTemplateTier()).
			AllUserAccountsHaveTier(murtest.DefaultNSTemplateTier())
		AssertThatChangeTierRequestIsDeleted(t, cl, changeTierRequest.Name)
	})

	t.Run("changetierrequest resets deactivating state of UserSignup", func(t *testing.T) {
		// given
		userSignupDeactivating := userSignup.DeepCopy()
		mur := murtest.NewMasterUserRecord(t, "johny", murtest.WithOwnerLabel(userSignupDeactivating.Name))
		changeTierRequest := newChangeTierRequest("johny", "team")
		teamTier := NewNSTemplateTier("team", "123team", "123clusterteam", "stage", "dev")

		states.SetDeactivating(userSignupDeactivating, true)
		controller, request, cl := newController(t, changeTierRequest, config, userSignupDeactivating, mur, teamTier)

		// when
		_, err := controller.Reconcile(context.TODO(), request)

		// then
		require.NoError(t, err)
		updatedUserSignup := &toolchainv1alpha1.UserSignup{}
		err = cl.Get(context.TODO(), types.NamespacedName{Namespace: test.HostOperatorNs, Name: userSignupDeactivating.Name}, updatedUserSignup)
		require.NoError(t, err)
		require.False(t, states.Deactivating(updatedUserSignup))
	})

	t.Run("should not update tier when not specified in MUR's UserAccount", func(t *testing.T) {
		// given
		mur := murtest.NewMasterUserRecord(t, "johny", murtest.WithOwnerLabel(userSignup.Name), murtest.AdditionalAccounts("another-cluster"))
		for i, ua := range mur.Spec.UserAccounts {
			if ua.TargetCluster == test.MemberClusterName {
				mur.Spec.UserAccounts[i].Spec.NSTemplateSet = nil // no NSTemplateSet for this UserAccount
				break
			}
		}
		changeTierRequest := newChangeTierRequest("johny", "team", targetCluster("another-cluster"))
		controller, request, cl := newController(t, changeTierRequest, config, userSignup, mur, teamTier)

		// when
		_, err := controller.Reconcile(context.TODO(), request)

		// then
		require.NoError(t, err)
		murtest.AssertThatMasterUserRecord(t, "johny", cl).
			UserAccountHasNoTier(test.MemberClusterName). // nothing set since there was no NStemplateSet to begin with
			UserAccountHasTier("another-cluster", *teamTier)
		AssertThatChangeTierRequestHasCondition(t, cl, changeTierRequest.Name, toBeComplete())
	})

	t.Run("should also update the Space with the same name", func(t *testing.T) {

		t.Run("when the MasterUserRecord exists", func(t *testing.T) {
			// given
			changeTierRequest := newChangeTierRequest("john", teamTier.Name)
			mur := murtest.NewMasterUserRecord(t, "john", murtest.WithOwnerLabel(userSignup.Name))
			space := spacetest.NewSpace("john", spacetest.WithTargetCluster("member-1"))
			controller, request, cl := newController(t, changeTierRequest, config, userSignup, mur, space, teamTier)

			// when
			_, err := controller.Reconcile(context.TODO(), request)

			// then
			require.NoError(t, err)
			murtest.AssertThatMasterUserRecord(t, "john", cl).
				HasTier(*teamTier).
				AllUserAccountsHaveTier(*teamTier).
				DoesNotHaveLabel(tierutil.TemplateTierHashLabelKey(murtest.DefaultNSTemplateTierName))
			spacetest.AssertThatSpace(t, space.Namespace, space.Name, cl).
				HasTier(teamTier.Name).
				HasTargetCluster("member-1"). // unchanged
				DoesNotHaveLabel(tierutil.TemplateTierHashLabelKey(changeTierRequest.Spec.TierName))
			AssertThatChangeTierRequestHasCondition(t, cl, changeTierRequest.Name, toBeComplete())
		})

		t.Run("when the MasterUserRecord does not exist", func(t *testing.T) {
			// given
			changeTierRequest := newChangeTierRequest("john", teamTier.Name)
			space := spacetest.NewSpace("john", spacetest.WithTargetCluster("member-1"))
			controller, request, cl := newController(t, changeTierRequest, config, userSignup, space, teamTier)

			// when
			_, err := controller.Reconcile(context.TODO(), request)

			// then
			require.NoError(t, err)
			spacetest.AssertThatSpace(t, space.Namespace, space.Name, cl).
				HasTier(teamTier.Name).
				HasTargetCluster("member-1"). // unchanged
				DoesNotHaveLabel(tierutil.TemplateTierHashLabelKey(changeTierRequest.Spec.TierName))
			AssertThatChangeTierRequestHasCondition(t, cl, changeTierRequest.Name, toBeComplete())
		})

		t.Run("when the Space tier already up-to-date", func(t *testing.T) {
			// given
			changeTierRequest := newChangeTierRequest("john", teamTier.Name)
			space := spacetest.NewSpace("john", spacetest.WithTargetCluster("member-1"), spacetest.WithTierNameAndHashLabelFor(teamTier))
			controller, request, cl := newController(t, changeTierRequest, config, userSignup, space, teamTier)

			// when
			_, err := controller.Reconcile(context.TODO(), request)

			// then
			require.NoError(t, err)
			spacetest.AssertThatSpace(t, space.Namespace, space.Name, cl).
				HasTier(teamTier.Name).                                                      // unchanged
				HasTargetCluster("member-1").                                                // unchanged
				HasLabel(tierutil.TemplateTierHashLabelKey(changeTierRequest.Spec.TierName)) // not removed since there was no update to perform in this case
			AssertThatChangeTierRequestHasCondition(t, cl, changeTierRequest.Name, toBeComplete()) // ChangeTierRequest is complete.
		})
	})
}

func TestChangeTierFailure(t *testing.T) {
	config := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.Tiers().DurationBeforeChangeTierRequestDeletion("10s"))
	userSignup := NewUserSignup(WithName("john"))

	basicTier := NewNSTemplateTier("basic", "123basic", "123clusterbasic", "stage", "dev")
	teamTier := NewNSTemplateTier("team", "123team", "123clusterteam", "stage", "dev")

	t.Run("the change will fail since the corresponding MUR cannot get fetched", func(t *testing.T) {
		// given
		changeTierRequest := newChangeTierRequest("johny", "team")
		teamTier := NewNSTemplateTier("team", "123team", "123clusterteam", "stage", "dev")
		controller, request, cl := newController(t, changeTierRequest, config, userSignup, teamTier)
		cl.MockGet = func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
			if _, ok := obj.(*toolchainv1alpha1.MasterUserRecord); ok {
				return fmt.Errorf("mock error")
			}
			return cl.Client.Get(ctx, key, obj)
		}
		// when
		_, err := controller.Reconcile(context.TODO(), request)

		// then
		require.EqualError(t, err, "unable to get MasterUserRecord with name johny: mock error")
		AssertThatChangeTierRequestHasCondition(t, cl, changeTierRequest.Name, toBeNotComplete("mock error"))
	})

	t.Run("will fail since the provided tier doesn't exist", func(t *testing.T) {
		// given
		mur := murtest.NewMasterUserRecord(t, "johny", murtest.WithOwnerLabel(userSignup.Name), murtest.AdditionalAccounts("another-cluster"))
		changeTierRequest := newChangeTierRequest("johny", "team")
		controller, request, cl := newController(t, changeTierRequest, config, userSignup, mur)

		// when
		_, err := controller.Reconcile(context.TODO(), request)

		// then
		require.Error(t, err)
		assert.Equal(t, err.Error(), "unable to get NSTemplateTier with name team: nstemplatetiers.toolchain.dev.openshift.com \"team\" not found")
		AssertThatChangeTierRequestHasCondition(t, cl, changeTierRequest.Name, toBeNotComplete("nstemplatetiers.toolchain.dev.openshift.com \"team\" not found"))
	})

	t.Run("will fail since it won't be able to find the correct UserAccount in MUR", func(t *testing.T) {
		// given
		mur := murtest.NewMasterUserRecord(t, "johny", murtest.WithOwnerLabel(userSignup.Name))
		changeTierRequest := newChangeTierRequest("johny", "team", targetCluster("some-other-cluster"))
		controller, request, cl := newController(t, changeTierRequest, config, userSignup, mur, teamTier)

		// when
		_, err := controller.Reconcile(context.TODO(), request)

		// then
		require.EqualError(t, err, "unable to change tier in MasterUserRecord johny: the MasterUserRecord 'johny' doesn't contain UserAccount with cluster 'some-other-cluster' whose tier should be changed")
		murtest.AssertThatMasterUserRecord(t, "johny", cl).
			HasTier(murtest.DefaultNSTemplateTier()).
			AllUserAccountsHaveTier(murtest.DefaultNSTemplateTier())
		AssertThatChangeTierRequestHasCondition(t, cl, changeTierRequest.Name,
			toBeNotComplete("the MasterUserRecord 'johny' doesn't contain UserAccount with cluster 'some-other-cluster' whose tier should be changed"))
	})

	t.Run("will fail since the actual update operation will return an error", func(t *testing.T) {
		// given
		mur := murtest.NewMasterUserRecord(t, "johny", murtest.WithOwnerLabel(userSignup.Name))
		changeTierRequest := newChangeTierRequest("johny", "team")
		controller, request, cl := newController(t, changeTierRequest, config, userSignup, mur, teamTier)
		cl.MockUpdate = func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
			_, ok := obj.(*toolchainv1alpha1.MasterUserRecord)
			if ok {
				return fmt.Errorf("error")
			}
			return nil
		}

		// when
		_, err := controller.Reconcile(context.TODO(), request)

		// then
		require.EqualError(t, err, "unable to change tier in MasterUserRecord johny: error")
		AssertThatChangeTierRequestHasCondition(t, cl, changeTierRequest.Name, toBeNotComplete("error"))
	})

	t.Run("will return an error since it cannot delete the ChangeTierRequest after successful completion", func(t *testing.T) {
		// given
		mur := murtest.NewMasterUserRecord(t, "johny", murtest.WithOwnerLabel(userSignup.Name))
		changeTierRequest := newChangeTierRequest("johny", "faildeletion")
		changeTierRequest.Status.Conditions = []toolchainv1alpha1.Condition{toBeComplete()}
		changeTierRequest.Status.Conditions[0].LastTransitionTime = v1.Time{Time: time.Now().Add(-cast.ToDuration("10s"))}
		controller, request, cl := newController(t, changeTierRequest, config, userSignup, mur, teamTier)
		cl.MockDelete = func(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
			return fmt.Errorf("error")
		}

		// when
		_, err := controller.Reconcile(context.TODO(), request)

		// then
		require.EqualError(t, err, "failed to delete changeTierRequest: unable to delete ChangeTierRequest object 'request-name': error")
		murtest.AssertThatMasterUserRecord(t, "johny", cl).
			HasTier(murtest.DefaultNSTemplateTier()).
			AllUserAccountsHaveTier(murtest.DefaultNSTemplateTier())
		AssertThatChangeTierRequestHasCondition(t, cl, changeTierRequest.Name, toBeComplete(), toBeDeletionError("unable to delete ChangeTierRequest object 'request-name': error"))
	})

	t.Run("will fail because the MUR is missing an owner label", func(t *testing.T) {
		// given
		mur := murtest.NewMasterUserRecord(t, "john")
		changeTierRequest := newChangeTierRequest("john", "team")
		controller, request, cl := newController(t, changeTierRequest, config, userSignup, mur, teamTier)

		// when
		_, err := controller.Reconcile(context.TODO(), request)

		// then
		require.EqualError(t, err, `failed to get corresponding UserSignup for MasterUserRecord with name 'john': MasterUserRecord is missing label 'toolchain.dev.openshift.com/owner'`)
		AssertThatChangeTierRequestHasCondition(t, cl, changeTierRequest.Name, toBeNotComplete(`MasterUserRecord is missing label 'toolchain.dev.openshift.com/owner'`))
	})

	t.Run("will fail because there is no UserSignup found with the name from the MUR owner label", func(t *testing.T) {
		// given
		mur := murtest.NewMasterUserRecord(t, "john", murtest.WithOwnerLabel(userSignup.Name))
		changeTierRequest := newChangeTierRequest("john", "team")
		controller, request, cl := newController(t, changeTierRequest, config, mur, teamTier)

		// when
		_, err := controller.Reconcile(context.TODO(), request)

		// then
		require.EqualError(t, err, `failed to get UserSignup 'john': usersignups.toolchain.dev.openshift.com "john" not found`)
		AssertThatChangeTierRequestHasCondition(t, cl, changeTierRequest.Name, toBeNotComplete(`usersignups.toolchain.dev.openshift.com "john" not found`))
	})

	t.Run("will fail because resetting the deactivating state of the UserSignup fails", func(t *testing.T) {
		// given
		mur := murtest.NewMasterUserRecord(t, "john", murtest.WithOwnerLabel(userSignup.Name))
		changeTierRequest := newChangeTierRequest("john", "team")

		userSignupDeactivating := userSignup.DeepCopy()
		states.SetDeactivating(userSignupDeactivating, true)
		controller, request, cl := newController(t, changeTierRequest, config, userSignupDeactivating, mur, teamTier)
		cl.MockUpdate = func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
			if obj.GetObjectKind().GroupVersionKind().Kind != "MasterUserRecord" {
				return fmt.Errorf("update UserSignup failure")
			}
			return nil
		}

		// when
		_, err := controller.Reconcile(context.TODO(), request)

		// then
		require.EqualError(t, err, `failed to reset deactivating state for UserSignup 'john': update UserSignup failure`, err.Error())
		AssertThatChangeTierRequestHasCondition(t, cl, changeTierRequest.Name, toBeNotComplete(`update UserSignup failure`))
		updatedUserSignup := &toolchainv1alpha1.UserSignup{}
		err = cl.Get(context.TODO(), types.NamespacedName{Namespace: test.HostOperatorNs, Name: userSignupDeactivating.Name}, updatedUserSignup)
		require.NoError(t, err)
		require.True(t, states.Deactivating(updatedUserSignup))
	})

	t.Run("will fail on Space", func(t *testing.T) {

		t.Run("when fetching fails", func(t *testing.T) {
			// given
			changeTierRequest := newChangeTierRequest("john", "team")
			space := spacetest.NewSpace("john", spacetest.WithTargetCluster("member-1"))
			controller, request, cl := newController(t, changeTierRequest, config, userSignup, space, teamTier)
			cl.MockGet = func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
				if _, ok := obj.(*toolchainv1alpha1.Space); ok {
					return fmt.Errorf("mock error")
				}
				return cl.Client.Get(ctx, key, obj)
			}
			// when
			_, err := controller.Reconcile(context.TODO(), request)

			// then
			require.EqualError(t, err, "unable to get Space with name john: mock error")
			cl.MockGet = nil // need to restore the default behaviour otherwise the assertion using the same client will fail, too!
			spacetest.AssertThatSpace(t, space.Namespace, space.Name, cl).
				HasTier(basicTier.Name) // unchanged
			AssertThatChangeTierRequestHasCondition(t, cl, changeTierRequest.Name, toBeNotComplete("mock error"))
		})

		t.Run("when updating fails", func(t *testing.T) {
			// given
			changeTierRequest := newChangeTierRequest("john", "team")
			space := spacetest.NewSpace("john", spacetest.WithTargetCluster("member-1"))
			controller, request, cl := newController(t, changeTierRequest, config, userSignup, space, teamTier)
			cl.MockUpdate = func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
				if _, ok := obj.(*toolchainv1alpha1.Space); ok {
					return fmt.Errorf("mock error")
				}
				return cl.Client.Update(ctx, obj, opts...)
			}

			// when
			_, err := controller.Reconcile(context.TODO(), request)

			// then
			require.EqualError(t, err, "unable to change tier in Space john: mock error")
			spacetest.AssertThatSpace(t, space.Namespace, space.Name, cl).
				HasTier(basicTier.Name) // unchanged
			AssertThatChangeTierRequestHasCondition(t, cl, changeTierRequest.Name, toBeNotComplete("mock error"))
		})
	})

	t.Run("will fail when there is no matching MasterUserRecord nor Space", func(t *testing.T) {
		// given
		changeTierRequest := newChangeTierRequest("john", "team")
		controller, request, cl := newController(t, changeTierRequest, config, teamTier)

		// when
		_, err := controller.Reconcile(context.TODO(), request)

		// then
		require.EqualError(t, err, "no MasterUserRecord nor Space named 'john' matching the ChangeTierRequest")
		AssertThatChangeTierRequestHasCondition(t, cl, changeTierRequest.Name, toBeNotComplete("no MasterUserRecord nor Space named 'john' matching the ChangeTierRequest"))
	})
}

func TestUpdateStatus(t *testing.T) {
	// given
	changeTierRequest := newChangeTierRequest("johny", "team")
	controller, _, _ := newController(t, changeTierRequest)
	log := logf.Log.WithName("test")

	t.Run("status updated", func(t *testing.T) {
		statusUpdater := func(changeTierRequest *toolchainv1alpha1.ChangeTierRequest, message string) error {
			assert.Equal(t, "oopsy woopsy", message)
			return nil
		}

		// test
		err := controller.wrapErrorWithStatusUpdate(log, changeTierRequest, statusUpdater, apierrors.NewBadRequest("oopsy woopsy"), "failed to create namespace")

		require.Error(t, err)
		assert.Equal(t, "failed to create namespace: oopsy woopsy", err.Error())
	})

	t.Run("status update failed", func(t *testing.T) {
		statusUpdater := func(changeTierRequest *toolchainv1alpha1.ChangeTierRequest, message string) error {
			return fmt.Errorf("unable to update status")
		}

		// when
		err := controller.wrapErrorWithStatusUpdate(log, changeTierRequest, statusUpdater, apierrors.NewBadRequest("oopsy woopsy"), "failed to create namespace")

		// then
		require.Error(t, err)
		assert.Equal(t, "failed to create namespace: oopsy woopsy", err.Error())
	})
}

func AssertThatChangeTierRequestIsDeleted(t *testing.T, cl client.Client, name string) {
	changeTierRequest := &toolchainv1alpha1.ChangeTierRequest{}
	err := cl.Get(context.TODO(), test.NamespacedName(test.HostOperatorNs, name), changeTierRequest)
	require.Error(t, err)
	assert.IsType(t, v1.StatusReasonNotFound, apierrors.ReasonForError(err))
}

func AssertThatChangeTierRequestHasCondition(t *testing.T, cl client.Client, name string, condition ...toolchainv1alpha1.Condition) {
	changeTierRequest := &toolchainv1alpha1.ChangeTierRequest{}
	err := cl.Get(context.TODO(), test.NamespacedName(test.HostOperatorNs, name), changeTierRequest)
	require.NoError(t, err)
	test.AssertConditionsMatch(t, changeTierRequest.Status.Conditions, condition...)
}

func NewNSTemplateTier(tierName, revision, clusterResourcesRevision string, nsTypes ...string) *toolchainv1alpha1.NSTemplateTier {
	namespaces := make([]toolchainv1alpha1.NSTemplateTierNamespace, len(nsTypes))
	for i, nsType := range nsTypes {
		namespaces[i] = toolchainv1alpha1.NSTemplateTierNamespace{
			TemplateRef: nstemplatetiers.NewTierTemplateName(tierName, nsType, revision),
		}
	}
	var clusterResources *toolchainv1alpha1.NSTemplateTierClusterResources
	if clusterResourcesRevision != "" {
		clusterResources = &toolchainv1alpha1.NSTemplateTierClusterResources{
			TemplateRef: nstemplatetiers.NewTierTemplateName(tierName, "clusterresources", clusterResourcesRevision),
		}
	}
	return &toolchainv1alpha1.NSTemplateTier{
		ObjectMeta: v1.ObjectMeta{
			Namespace: test.HostOperatorNs,
			Name:      tierName,
		},
		Spec: toolchainv1alpha1.NSTemplateTierSpec{
			Namespaces:       namespaces,
			ClusterResources: clusterResources,
		},
	}
}

func toBeComplete() toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:               toolchainv1alpha1.ChangeTierRequestComplete,
		Status:             apiv1.ConditionTrue,
		Reason:             toolchainv1alpha1.ChangeTierRequestChangedReason,
		LastTransitionTime: v1.Time{Time: time.Now()},
	}
}

func toBeNotComplete(msg string) toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:    toolchainv1alpha1.ChangeTierRequestComplete,
		Status:  apiv1.ConditionFalse,
		Reason:  toolchainv1alpha1.ChangeTierRequestChangeFailedReason,
		Message: msg,
	}
}

func toBeDeletionError(msg string) toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:               toolchainv1alpha1.ChangeTierRequestDeletionError,
		Status:             apiv1.ConditionTrue,
		Reason:             toolchainv1alpha1.ChangeTierRequestDeletionErrorReason,
		Message:            msg,
		LastTransitionTime: v1.Time{Time: time.Now()},
	}
}

type changeTierRequestOption func(*toolchainv1alpha1.ChangeTierRequest)

func targetCluster(c string) changeTierRequestOption {
	return func(ctr *toolchainv1alpha1.ChangeTierRequest) {
		ctr.Spec.TargetCluster = c
	}
}

func newChangeTierRequest(murName, tierName string, options ...changeTierRequestOption) *toolchainv1alpha1.ChangeTierRequest {
	ctr := &toolchainv1alpha1.ChangeTierRequest{
		ObjectMeta: v1.ObjectMeta{
			Namespace: test.HostOperatorNs,
			Name:      "request-name",
		},
		Spec: toolchainv1alpha1.ChangeTierRequestSpec{
			MurName:  murName,
			TierName: tierName,
		},
	}
	for _, set := range options {
		set(ctr)
	}
	return ctr
}

func newController(t *testing.T, changeTier *toolchainv1alpha1.ChangeTierRequest, initObjs ...runtime.Object) (*Reconciler, reconcile.Request, *test.FakeClient) {
	os.Setenv("WATCH_NAMESPACE", test.HostOperatorNs)
	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)
	cl := test.NewFakeClient(t, append(initObjs, changeTier)...)
	controller := &Reconciler{
		Client: cl,
		Scheme: s,
	}
	request := reconcile.Request{
		NamespacedName: test.NamespacedName(test.HostOperatorNs, changeTier.Name),
	}

	return controller, request, cl
}
