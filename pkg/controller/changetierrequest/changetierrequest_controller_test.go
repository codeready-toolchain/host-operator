package changetierrequest

import (
	"context"
	"fmt"
	"testing"

	"github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	murtest "github.com/codeready-toolchain/toolchain-common/pkg/test/masteruserrecord"
	templatev1 "github.com/openshift/api/template/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apiv1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestChangeTierSuccess(t *testing.T) {

	t.Run("the controller should change tier in MUR", func(t *testing.T) {
		// given
		mur := murtest.NewMasterUserRecord("johny")
		changeTierRequest := newChangeTierRequest("johny", "team", "", noStatus)
		teamTier := NewNSTemplateTier("team", "123team", "stage", "dev")
		controller, request, cl := newController(t, changeTierRequest, mur, teamTier)

		// when
		_, err := controller.Reconcile(request)

		// then
		require.NoError(t, err)
		murtest.AssertThatMasterUserRecord(t, "johny", cl).
			AllUserAccountsHaveTier("team", teamTier.Spec.Namespaces)
		AssertThatChangeTierRequestIsDeleted(t, cl, changeTierRequest.Name, toBeComplete())
	})

	t.Run("the controller should change tier in all UserAccounts in MUR", func(t *testing.T) {
		// given
		mur := murtest.NewMasterUserRecord("johny", murtest.AdditionalAccounts("another-cluster"))
		changeTierRequest := newChangeTierRequest("johny", "team", "", noStatus)
		teamTier := NewNSTemplateTier("team", "123team", "stage", "dev")
		controller, request, cl := newController(t, changeTierRequest, mur, teamTier)

		// when
		_, err := controller.Reconcile(request)

		// then
		require.NoError(t, err)
		murtest.AssertThatMasterUserRecord(t, "johny", cl).
			AllUserAccountsHaveTier("team", teamTier.Spec.Namespaces)
		AssertThatChangeTierRequestIsDeleted(t, cl, changeTierRequest.Name, toBeComplete())
	})

	t.Run("the controller should change tier only in specified UserAccount in MUR", func(t *testing.T) {
		// given
		mur := murtest.NewMasterUserRecord("johny", murtest.AdditionalAccounts("another-cluster"))
		changeTierRequest := newChangeTierRequest("johny", "team", "another-cluster", noStatus)
		teamTier := NewNSTemplateTier("team", "123team", "stage", "dev")
		basicTier := NewNSTemplateTier("basic", "123abc", "code", "stage", "dev")
		controller, request, cl := newController(t, changeTierRequest, mur, teamTier)

		// when
		_, err := controller.Reconcile(request)

		// then
		require.NoError(t, err)
		murtest.AssertThatMasterUserRecord(t, "johny", cl).
			UserAccountHasTier("another-cluster", "team", teamTier.Spec.Namespaces).
			UserAccountHasTier(test.MemberClusterName, "basic", basicTier.Spec.Namespaces)
		AssertThatChangeTierRequestIsDeleted(t, cl, changeTierRequest.Name, toBeComplete())
	})

	t.Run("the ChangeTierRequest will be ignored as is already complete", func(t *testing.T) {
		// given
		mur := murtest.NewMasterUserRecord("johny")
		changeTierRequest := newChangeTierRequest("johny", "team", "", noStatus)
		changeTierRequest.Status.Conditions = []v1alpha1.Condition{toBeComplete()}
		teamTier := NewNSTemplateTier("team", "123team", "stage", "dev")
		basicTier := NewNSTemplateTier("basic", "123abc", "code", "stage", "dev")
		controller, request, cl := newController(t, changeTierRequest, mur, teamTier)

		// when
		_, err := controller.Reconcile(request)

		// then
		require.NoError(t, err)
		murtest.AssertThatMasterUserRecord(t, "johny", cl).
			AllUserAccountsHaveTier("basic", basicTier.Spec.Namespaces)
		AssertThatChangeTierRequestIsDeleted(t, cl, changeTierRequest.Name, toBeComplete())
	})
}

func TestChangeTierFailure(t *testing.T) {

	t.Run("the change will fail since the provided MUR doesn't exist", func(t *testing.T) {
		// given
		changeTierRequest := newChangeTierRequest("johny", "team", "", noStatus)
		teamTier := NewNSTemplateTier("team", "123team", "stage", "dev")
		controller, request, cl := newController(t, changeTierRequest, teamTier)

		// when
		_, err := controller.Reconcile(request)

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unable to get MasterUserRecord with name johny")
		AssertThatChangeTierRequestHasCondition(t, cl, changeTierRequest.Name, toBeNotComplete("masteruserrecords.toolchain.dev.openshift.com \"johny\" not found"))
	})

	t.Run("the change will fail since the provided tier doesn't exist", func(t *testing.T) {
		// given
		mur := murtest.NewMasterUserRecord("johny", murtest.AdditionalAccounts("another-cluster"))
		changeTierRequest := newChangeTierRequest("johny", "team", "", noStatus)
		controller, request, cl := newController(t, changeTierRequest, mur)

		// when
		_, err := controller.Reconcile(request)

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unable to get NSTemplateTier with name team")
		AssertThatChangeTierRequestHasCondition(t, cl, changeTierRequest.Name, toBeNotComplete("nstemplatetiers.toolchain.dev.openshift.com \"team\" not found"))
	})

	t.Run("the change will fail since it won't be able to find the correct UserAccount in MUR", func(t *testing.T) {
		// given
		mur := murtest.NewMasterUserRecord("johny")
		changeTierRequest := newChangeTierRequest("johny", "team", "some-other-cluster", noStatus)
		teamTier := NewNSTemplateTier("team", "123team", "stage", "dev")
		basicTier := NewNSTemplateTier("basic", "123abc", "code", "stage", "dev")
		controller, request, cl := newController(t, changeTierRequest, mur, teamTier)

		// when
		_, err := controller.Reconcile(request)

		// then
		require.Error(t, err)
		murtest.AssertThatMasterUserRecord(t, "johny", cl).
			AllUserAccountsHaveTier("basic", basicTier.Spec.Namespaces)
		assert.Contains(t, err.Error(), "unable to change tier in MasterUserRecord johny")
		AssertThatChangeTierRequestHasCondition(t, cl, changeTierRequest.Name,
			toBeNotComplete("the MasterUserRecord 'johny' doesn't contain UserAccount with cluster 'some-other-cluster' whose tier should be changed"))
	})

	t.Run("the change will fail since the actual update operation will return an error", func(t *testing.T) {
		// given
		mur := murtest.NewMasterUserRecord("johny")
		changeTierRequest := newChangeTierRequest("johny", "team", "", noStatus)
		teamTier := NewNSTemplateTier("team", "123team", "stage", "dev")
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
		assert.Contains(t, err.Error(), "unable to change tier in MasterUserRecord johny: error")
		AssertThatChangeTierRequestHasCondition(t, cl, changeTierRequest.Name, toBeNotComplete("error"))
	})

	t.Run("will return an error since it cannot delete the ChangeTierRequest after successful completion", func(t *testing.T) {
		// given
		mur := murtest.NewMasterUserRecord("johny")
		changeTierRequest := newChangeTierRequest("johny", "faildeletion", "", noStatus)
		teamTier := NewNSTemplateTier("faildeletion", "123fail", "stage", "dev")
		controller, request, cl := newController(t, changeTierRequest, mur, teamTier)

		// when
		_, err := controller.Reconcile(request)

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unable to delete ChangeTierRequest object 'request-name'")
		murtest.AssertThatMasterUserRecord(t, "johny", cl).
			AllUserAccountsHaveTier("faildeletion", teamTier.Spec.Namespaces)
		AssertThatChangeTierRequestHasCondition(t, cl, changeTierRequest.Name, toBeComplete())
	})
}

func TestUpdateStatus(t *testing.T) {
	// given
	changeTierRequest := newChangeTierRequest("johny", "team", "", noStatus)
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

func AssertThatChangeTierRequestIsDeleted(t *testing.T, cl client.Client, name string, condition v1alpha1.Condition) {
	changeTierRequest := &v1alpha1.ChangeTierRequest{}
	err := cl.Get(context.TODO(), test.NamespacedName(test.HostOperatorNs, name), changeTierRequest)
	require.Error(t, err)
	assert.IsType(t, v1.StatusReasonNotFound, apierrors.ReasonForError(err))
}

func AssertThatChangeTierRequestHasCondition(t *testing.T, cl client.Client, name string, condition v1alpha1.Condition) {
	changeTierRequest := &v1alpha1.ChangeTierRequest{}
	err := cl.Get(context.TODO(), test.NamespacedName(test.HostOperatorNs, name), changeTierRequest)
	require.NoError(t, err)
	test.AssertConditionsMatch(t, changeTierRequest.Status.Conditions, condition)
}

func NewNSTemplateTier(tierName, revision string, nsTypes ...string) *v1alpha1.NSTemplateTier {
	namespaces := make([]v1alpha1.NSTemplateTierNamespace, len(nsTypes))
	for i, nsType := range nsTypes {
		namespaces[i] = v1alpha1.NSTemplateTierNamespace{
			Type:     nsType,
			Revision: revision,
			Template: templatev1.Template{},
		}
	}
	return &v1alpha1.NSTemplateTier{
		ObjectMeta: v1.ObjectMeta{
			Namespace: test.HostOperatorNs,
			Name:      tierName,
		},
		Spec: v1alpha1.NSTemplateTierSpec{
			Namespaces: namespaces,
		},
	}
}

var (
	notComplete = v1alpha1.ChangeTierRequestStatus{
		Conditions: []v1alpha1.Condition{toBeNotComplete("")},
	}

	noStatus = v1alpha1.ChangeTierRequestStatus{}

	complete = v1alpha1.ChangeTierRequestStatus{
		Conditions: []v1alpha1.Condition{toBeComplete()},
	}
)

func toBeComplete() v1alpha1.Condition {
	return v1alpha1.Condition{
		Type:   v1alpha1.ChangeTierRequestComplete,
		Status: apiv1.ConditionTrue,
		Reason: v1alpha1.ChangeTierRequestChangedReason,
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

func newChangeTierRequest(murName, tierName, targetCluster string, status v1alpha1.ChangeTierRequestStatus) *v1alpha1.ChangeTierRequest {
	return &v1alpha1.ChangeTierRequest{
		ObjectMeta: v1.ObjectMeta{
			Namespace: test.HostOperatorNs,
			Name:      "request-name",
		},
		Spec: v1alpha1.ChangeTierRequestSpec{
			MurName:       murName,
			TierName:      tierName,
			TargetCluster: targetCluster,
		},
		Status: status,
	}
}

func newController(t *testing.T, changeTier *v1alpha1.ChangeTierRequest, initObjs ...runtime.Object) (*ReconcileChangeTierRequest, reconcile.Request, *test.FakeClient) {
	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)
	cl := test.NewFakeClient(t, append(initObjs, changeTier)...)
	cl.MockDelete = func(ctx context.Context, obj runtime.Object, opts ...client.DeleteOption) error {
		changeTierRequest, ok := obj.(*v1alpha1.ChangeTierRequest)
		if ok {
			test.AssertConditionsMatch(t, changeTierRequest.Status.Conditions, toBeComplete())
			require.Len(t, opts, 1)
			deleteOptions := &client.DeleteOptions{}
			opts[0].ApplyToDelete(deleteOptions)
			require.Equal(t, int64(86400), *deleteOptions.GracePeriodSeconds)
			if changeTierRequest.Spec.TierName == "faildeletion" {
				return fmt.Errorf("error")
			}
		}
		return cl.Client.Delete(ctx, obj, opts...)
	}
	controller := &ReconcileChangeTierRequest{
		client: cl,
		scheme: s,
	}
	request := reconcile.Request{
		NamespacedName: test.NamespacedName(test.HostOperatorNs, changeTier.Name),
	}

	return controller, request, cl
}
