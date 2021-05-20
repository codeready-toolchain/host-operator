package changetierrequest

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	"github.com/codeready-toolchain/host-operator/pkg/configuration"
	"github.com/codeready-toolchain/host-operator/pkg/controller/nstemplatetier"
	"github.com/codeready-toolchain/host-operator/pkg/templates/nstemplatetiers"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	murtest "github.com/codeready-toolchain/toolchain-common/pkg/test/masteruserrecord"

	"github.com/spf13/cast"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apiv1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestChangeTierSuccess(t *testing.T) {
	// given
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	restore := test.SetEnvVarAndRestore(t, "HOST_OPERATOR_DURATION_BEFORE_CHANGE_REQUEST_DELETION", "10s")
	defer restore()
	teamTier := NewNSTemplateTier("team", "123team", "123clusterteam", "stage", "dev")

	t.Run("the controller should change tier in MUR", func(t *testing.T) {
		// given
		mur := murtest.NewMasterUserRecord(t, "john")
		changeTierRequest := newChangeTierRequest("john", "team")
		controller, request, cl := newController(t, changeTierRequest, mur, teamTier)

		// when
		_, err := controller.Reconcile(request)

		// then
		require.NoError(t, err)
		murtest.AssertThatMasterUserRecord(t, "john", cl).
			AllUserAccountsHaveTier(*teamTier).
			DoesNotHaveLabel(nstemplatetier.TemplateTierHashLabelKey(murtest.DefaultNSTemplateTier.Name))
		AssertThatChangeTierRequestHasCondition(t, cl, changeTierRequest.Name, toBeComplete())
	})

	t.Run("the controller should change tier in all UserAccounts in MUR", func(t *testing.T) {
		// given
		mur := murtest.NewMasterUserRecord(t, "johny", murtest.AdditionalAccounts("another-cluster"))
		changeTierRequest := newChangeTierRequest("johny", "team")
		controller, request, cl := newController(t, changeTierRequest, mur, teamTier)

		// when
		_, err := controller.Reconcile(request)

		// then
		require.NoError(t, err)
		murtest.AssertThatMasterUserRecord(t, "johny", cl).AllUserAccountsHaveTier(*teamTier).
			DoesNotHaveLabel(nstemplatetier.TemplateTierHashLabelKey(murtest.DefaultNSTemplateTier.Name))
		AssertThatChangeTierRequestHasCondition(t, cl, changeTierRequest.Name, toBeComplete())
	})

	t.Run("the controller should change tier only in specified UserAccount in MUR", func(t *testing.T) {
		// given
		mur := murtest.NewMasterUserRecord(t, "johny", murtest.AdditionalAccounts("another-cluster"))
		changeTierRequest := newChangeTierRequest("johny", "team", targetCluster("another-cluster"))
		controller, request, cl := newController(t, changeTierRequest, mur, teamTier)

		// when
		_, err := controller.Reconcile(request)

		// then
		require.NoError(t, err)
		murtest.AssertThatMasterUserRecord(t, "johny", cl).
			UserAccountHasTier("another-cluster", *teamTier).
			UserAccountHasTier(test.MemberClusterName, murtest.DefaultNSTemplateTier)
		AssertThatChangeTierRequestHasCondition(t, cl, changeTierRequest.Name, toBeComplete())
	})

	t.Run("completed changetierrequest is requeued with the remaining deletion timeout", func(t *testing.T) {
		// given
		mur := murtest.NewMasterUserRecord(t, "johny")
		changeTierRequest := newChangeTierRequest("johny", "team")
		changeTierRequest.Status.Conditions = []v1alpha1.Condition{toBeComplete()}
		controller, request, cl := newController(t, changeTierRequest, mur)

		// when
		result, err := controller.Reconcile(request)

		// then
		require.NoError(t, err)
		assert.True(t, result.Requeue)
		assert.True(t, result.RequeueAfter < cast.ToDuration("10s"))
		assert.True(t, result.RequeueAfter > cast.ToDuration("1s"))
		murtest.AssertThatMasterUserRecord(t, "johny", cl).
			AllUserAccountsHaveTier(murtest.DefaultNSTemplateTier)
		AssertThatChangeTierRequestHasCondition(t, cl, changeTierRequest.Name, toBeComplete())
	})

	t.Run("change request deleted when deletion timeout passed", func(t *testing.T) {
		// given
		mur := murtest.NewMasterUserRecord(t, "johny")
		changeTierRequest := newChangeTierRequest("johny", "team")
		changeTierRequest.Status.Conditions = []v1alpha1.Condition{toBeComplete()}
		changeTierRequest.Status.Conditions[0].LastTransitionTime = v1.Time{Time: time.Now().Add(-cast.ToDuration("10s"))}
		controller, request, cl := newController(t, changeTierRequest, mur)

		// when
		result, err := controller.Reconcile(request)

		// then
		require.NoError(t, err)
		assert.False(t, result.Requeue)
		murtest.AssertThatMasterUserRecord(t, "johny", cl).
			AllUserAccountsHaveTier(murtest.DefaultNSTemplateTier)
		AssertThatChangeTierRequestIsDeleted(t, cl, changeTierRequest.Name)
	})
}

func TestChangeTierFailure(t *testing.T) {
	restore := test.SetEnvVarAndRestore(t, "HOST_OPERATOR_DURATION_BEFORE_CHANGE_REQUEST_DELETION", "10s")
	defer restore()

	t.Run("the change will fail since the provided MUR doesn't exist", func(t *testing.T) {
		// given
		changeTierRequest := newChangeTierRequest("johny", "team")
		teamTier := NewNSTemplateTier("team", "123team", "123clusterteam", "stage", "dev")
		controller, request, cl := newController(t, changeTierRequest, teamTier)

		// when
		_, err := controller.Reconcile(request)

		// then
		require.Error(t, err)
		assert.Equal(t, err.Error(), "unable to get MasterUserRecord with name johny: masteruserrecords.toolchain.dev.openshift.com \"johny\" not found")
		AssertThatChangeTierRequestHasCondition(t, cl, changeTierRequest.Name, toBeNotComplete("masteruserrecords.toolchain.dev.openshift.com \"johny\" not found"))
	})

	t.Run("the change will fail since the provided tier doesn't exist", func(t *testing.T) {
		// given
		mur := murtest.NewMasterUserRecord(t, "johny", murtest.AdditionalAccounts("another-cluster"))
		changeTierRequest := newChangeTierRequest("johny", "team")
		controller, request, cl := newController(t, changeTierRequest, mur)

		// when
		_, err := controller.Reconcile(request)

		// then
		require.Error(t, err)
		assert.Equal(t, err.Error(), "unable to get NSTemplateTier with name team: nstemplatetiers.toolchain.dev.openshift.com \"team\" not found")
		AssertThatChangeTierRequestHasCondition(t, cl, changeTierRequest.Name, toBeNotComplete("nstemplatetiers.toolchain.dev.openshift.com \"team\" not found"))
	})

	t.Run("the change will fail since it won't be able to find the correct UserAccount in MUR", func(t *testing.T) {
		// given
		mur := murtest.NewMasterUserRecord(t, "johny")
		changeTierRequest := newChangeTierRequest("johny", "team", targetCluster("some-other-cluster"))
		teamTier := NewNSTemplateTier("team", "123team", "123clusterteam", "stage", "dev")
		controller, request, cl := newController(t, changeTierRequest, mur, teamTier)

		// when
		_, err := controller.Reconcile(request)

		// then
		require.Error(t, err)
		murtest.AssertThatMasterUserRecord(t, "johny", cl).
			AllUserAccountsHaveTier(murtest.DefaultNSTemplateTier)
		assert.Equal(t, err.Error(), "unable to change tier in MasterUserRecord johny: the MasterUserRecord 'johny' doesn't contain UserAccount with cluster 'some-other-cluster' whose tier should be changed")
		AssertThatChangeTierRequestHasCondition(t, cl, changeTierRequest.Name,
			toBeNotComplete("the MasterUserRecord 'johny' doesn't contain UserAccount with cluster 'some-other-cluster' whose tier should be changed"))
	})

	t.Run("the change will fail since the actual update operation will return an error", func(t *testing.T) {
		// given
		mur := murtest.NewMasterUserRecord(t, "johny")
		changeTierRequest := newChangeTierRequest("johny", "team")
		teamTier := NewNSTemplateTier("team", "123team", "123clusterteam", "stage", "dev")
		controller, request, cl := newController(t, changeTierRequest, mur, teamTier)
		cl.MockUpdate = func(ctx context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
			_, ok := obj.(*v1alpha1.MasterUserRecord)
			if ok {
				return fmt.Errorf("error")
			}
			return nil
		}

		// when
		_, err := controller.Reconcile(request)

		// then
		require.Error(t, err)
		assert.Equal(t, err.Error(), "unable to change tier in MasterUserRecord johny: error")
		AssertThatChangeTierRequestHasCondition(t, cl, changeTierRequest.Name, toBeNotComplete("error"))
	})

	t.Run("will return an error since it cannot delete the ChangeTierRequest after successful completion", func(t *testing.T) {
		// given
		mur := murtest.NewMasterUserRecord(t, "johny")
		changeTierRequest := newChangeTierRequest("johny", "faildeletion")
		changeTierRequest.Status.Conditions = []v1alpha1.Condition{toBeComplete()}
		changeTierRequest.Status.Conditions[0].LastTransitionTime = v1.Time{Time: time.Now().Add(-cast.ToDuration("10s"))}
		teamTier := NewNSTemplateTier("team", "123team", "123clusterteam", "stage", "dev")
		controller, request, cl := newController(t, changeTierRequest, mur, teamTier)
		cl.MockDelete = func(ctx context.Context, obj runtime.Object, opts ...client.DeleteOption) error {
			return fmt.Errorf("error")
		}

		// when
		_, err := controller.Reconcile(request)

		// then
		require.Error(t, err)
		assert.Equal(t, err.Error(), "failed to delete changeTierRequest: unable to delete ChangeTierRequest object 'request-name': error")
		murtest.AssertThatMasterUserRecord(t, "johny", cl).
			AllUserAccountsHaveTier(murtest.DefaultNSTemplateTier)
		AssertThatChangeTierRequestHasCondition(t, cl, changeTierRequest.Name, toBeComplete(), toBeDeletionError("unable to delete ChangeTierRequest object 'request-name': error"))
	})
}

func TestUpdateStatus(t *testing.T) {
	// given
	changeTierRequest := newChangeTierRequest("johny", "team")
	controller, _, _ := newController(t, changeTierRequest)
	log := logf.Log.WithName("test")

	t.Run("status updated", func(t *testing.T) {
		statusUpdater := func(changeTierRequest *v1alpha1.ChangeTierRequest, message string) error {
			assert.Equal(t, "oopsy woopsy", message)
			return nil
		}

		// test
		err := controller.wrapErrorWithStatusUpdate(log, changeTierRequest, statusUpdater, apierrors.NewBadRequest("oopsy woopsy"), "failed to create namespace")

		require.Error(t, err)
		assert.Equal(t, "failed to create namespace: oopsy woopsy", err.Error())
	})

	t.Run("status update failed", func(t *testing.T) {
		statusUpdater := func(changeTierRequest *v1alpha1.ChangeTierRequest, message string) error {
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
	changeTierRequest := &v1alpha1.ChangeTierRequest{}
	err := cl.Get(context.TODO(), test.NamespacedName(test.HostOperatorNs, name), changeTierRequest)
	require.Error(t, err)
	assert.IsType(t, v1.StatusReasonNotFound, apierrors.ReasonForError(err))
}

func AssertThatChangeTierRequestHasCondition(t *testing.T, cl client.Client, name string, condition ...v1alpha1.Condition) {
	changeTierRequest := &v1alpha1.ChangeTierRequest{}
	err := cl.Get(context.TODO(), test.NamespacedName(test.HostOperatorNs, name), changeTierRequest)
	require.NoError(t, err)
	test.AssertConditionsMatch(t, changeTierRequest.Status.Conditions, condition...)
}

func NewNSTemplateTier(tierName, revision, clusterResourcesRevision string, nsTypes ...string) *v1alpha1.NSTemplateTier {
	namespaces := make([]v1alpha1.NSTemplateTierNamespace, len(nsTypes))
	for i, nsType := range nsTypes {
		namespaces[i] = v1alpha1.NSTemplateTierNamespace{
			TemplateRef: nstemplatetiers.NewTierTemplateName(tierName, nsType, revision),
		}
	}
	var clusterResources *v1alpha1.NSTemplateTierClusterResources
	if clusterResourcesRevision != "" {
		clusterResources = &v1alpha1.NSTemplateTierClusterResources{
			TemplateRef: nstemplatetiers.NewTierTemplateName(tierName, "clusterresources", clusterResourcesRevision),
		}
	}
	return &v1alpha1.NSTemplateTier{
		ObjectMeta: v1.ObjectMeta{
			Namespace: test.HostOperatorNs,
			Name:      tierName,
		},
		Spec: v1alpha1.NSTemplateTierSpec{
			Namespaces:       namespaces,
			ClusterResources: clusterResources,
		},
	}
}

func toBeComplete() v1alpha1.Condition {
	return v1alpha1.Condition{
		Type:               v1alpha1.ChangeTierRequestComplete,
		Status:             apiv1.ConditionTrue,
		Reason:             v1alpha1.ChangeTierRequestChangedReason,
		LastTransitionTime: v1.Time{Time: time.Now()},
	}
}

func toBeNotComplete(msg string) v1alpha1.Condition {
	return v1alpha1.Condition{
		Type:    v1alpha1.ChangeTierRequestComplete,
		Status:  apiv1.ConditionFalse,
		Reason:  v1alpha1.ChangeTierRequestChangeFiledReason,
		Message: msg,
	}
}

func toBeDeletionError(msg string) v1alpha1.Condition {
	return v1alpha1.Condition{
		Type:               v1alpha1.ChangeTierRequestDeletionError,
		Status:             apiv1.ConditionTrue,
		Reason:             v1alpha1.ChangeTierRequestDeletionErrorReason,
		Message:            msg,
		LastTransitionTime: v1.Time{Time: time.Now()},
	}
}

type changeTierRequestOption func(*v1alpha1.ChangeTierRequest)

func targetCluster(c string) changeTierRequestOption {
	return func(ctr *v1alpha1.ChangeTierRequest) {
		ctr.Spec.TargetCluster = c
	}
}

func newChangeTierRequest(murName, tierName string, options ...changeTierRequestOption) *v1alpha1.ChangeTierRequest {
	ctr := &v1alpha1.ChangeTierRequest{
		ObjectMeta: v1.ObjectMeta{
			Namespace: test.HostOperatorNs,
			Name:      "request-name",
		},
		Spec: v1alpha1.ChangeTierRequestSpec{
			MurName:  murName,
			TierName: tierName,
		},
	}
	for _, set := range options {
		set(ctr)
	}
	return ctr
}

func newController(t *testing.T, changeTier *v1alpha1.ChangeTierRequest, initObjs ...runtime.Object) (*Reconciler, reconcile.Request, *test.FakeClient) {
	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)
	cl := test.NewFakeClient(t, append(initObjs, changeTier)...)
	config, err := configuration.LoadConfig(cl)
	require.NoError(t, err)
	controller := &Reconciler{
		Client: cl,
		Scheme: s,
		Config: config,
		Log:    ctrl.Log.WithName("controllers").WithName("ChangeTierRequest"),
	}
	request := reconcile.Request{
		NamespacedName: test.NamespacedName(test.HostOperatorNs, changeTier.Name),
	}

	return controller, request, cl
}
