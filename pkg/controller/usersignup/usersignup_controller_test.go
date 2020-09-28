package usersignup

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/codeready-toolchain/host-operator/pkg/configuration"
	"github.com/codeready-toolchain/host-operator/pkg/counter"
	"github.com/codeready-toolchain/host-operator/pkg/templates/nstemplatetiers"
	"github.com/codeready-toolchain/toolchain-common/pkg/cluster"

	. "github.com/codeready-toolchain/host-operator/test"
	ntest "github.com/codeready-toolchain/host-operator/test/notification"
	murtest "github.com/codeready-toolchain/toolchain-common/pkg/test/masteruserrecord"

	"github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"

	uuid "github.com/satori/go.uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func newNsTemplateTier(tierName, clusterRevision string, nsTypes ...string) *toolchainv1alpha1.NSTemplateTier {
	namespaces := make([]toolchainv1alpha1.NSTemplateTierNamespace, len(nsTypes))
	for i, nsType := range nsTypes {
		revision := fmt.Sprintf("123abc%d", i+1)
		namespaces[i] = toolchainv1alpha1.NSTemplateTierNamespace{
			TemplateRef: nstemplatetiers.NewTierTemplateName(tierName, nsType, revision),
		}
	}

	return &toolchainv1alpha1.NSTemplateTier{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: test.HostOperatorNs,
			Name:      tierName,
		},
		Spec: toolchainv1alpha1.NSTemplateTierSpec{
			Namespaces: namespaces,
			ClusterResources: &toolchainv1alpha1.NSTemplateTierClusterResources{
				TemplateRef: nstemplatetiers.NewTierTemplateName(tierName, "clusterresources", clusterRevision),
			},
		},
	}
}

var basicNSTemplateTier = newNsTemplateTier("basic", "654321b", "code", "dev", "stage")

func TestUserSignupCreateMUROk(t *testing.T) {
	// given
	InitializeCounter(t, 1)
	defer counter.Reset()
	userSignup := NewUserSignup(Approved(), WithTargetCluster("east"))
	r, req, _ := prepareReconcile(t, userSignup.Name, NewGetMemberClusters(), userSignup, basicNSTemplateTier)

	// when
	res, err := r.Reconcile(req)

	// then verify that the MUR exists and is complete
	require.NoError(t, err)
	require.Equal(t, reconcile.Result{}, res)
	murs := &v1alpha1.MasterUserRecordList{}
	err = r.client.List(context.TODO(), murs)
	require.NoError(t, err)
	require.Len(t, murs.Items, 1)
	mur := murs.Items[0]
	require.Equal(t, test.HostOperatorNs, mur.Namespace)
	require.Equal(t, userSignup.Name, mur.Labels[v1alpha1.MasterUserRecordUserIDLabelKey])
	require.Len(t, mur.Spec.UserAccounts, 1)
	assert.Equal(t, "basic", mur.Spec.UserAccounts[0].Spec.NSTemplateSet.TierName)
	assert.Equal(t, []toolchainv1alpha1.NSTemplateSetNamespace{
		{
			TemplateRef: "basic-code-123abc1",
		},
		{
			TemplateRef: "basic-dev-123abc2",
		},
		{
			TemplateRef: "basic-stage-123abc3",
		},
	}, mur.Spec.UserAccounts[0].Spec.NSTemplateSet.Namespaces)
	require.NotNil(t, mur.Spec.UserAccounts[0].Spec.NSTemplateSet.ClusterResources)
	assert.Equal(t, "basic-clusterresources-654321b", mur.Spec.UserAccounts[0].Spec.NSTemplateSet.ClusterResources.TemplateRef)

	err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)
	assert.Equal(t, "true", userSignup.Labels[v1alpha1.UserSignupApprovedLabelKey])
	AssertThatCounterHas(t, 2)
}

func TestUserSignupWithAutoApprovalWithoutTargetCluster(t *testing.T) {
	// given
	InitializeCounter(t, 1)
	defer counter.Reset()
	userSignup := NewUserSignup()

	ready := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue))
	r, req, _ := prepareReconcile(t, userSignup.Name, ready, userSignup, configMap(configuration.UserApprovalPolicyAutomatic), basicNSTemplateTier)

	// when - The first reconcile creates the MasterUserRecord
	res, err := r.Reconcile(req)

	// then
	require.NoError(t, err)
	require.Equal(t, reconcile.Result{}, res)

	// Lookup the user signup again
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)
	assert.Equal(t, "true", userSignup.Labels[v1alpha1.UserSignupApprovedLabelKey])

	murs := &v1alpha1.MasterUserRecordList{}
	err = r.client.List(context.TODO(), murs)
	require.NoError(t, err)
	require.Len(t, murs.Items, 1)

	mur := murs.Items[0]
	require.Equal(t, test.HostOperatorNs, mur.Namespace)
	require.Equal(t, userSignup.Name, mur.Labels[v1alpha1.MasterUserRecordUserIDLabelKey])
	require.Len(t, mur.Spec.UserAccounts, 1)
	assert.Equal(t, "basic", mur.Spec.UserAccounts[0].Spec.NSTemplateSet.TierName)
	require.Len(t, mur.Spec.UserAccounts[0].Spec.NSTemplateSet.Namespaces, 3)
	assert.Contains(t, mur.Spec.UserAccounts[0].Spec.NSTemplateSet.Namespaces,
		toolchainv1alpha1.NSTemplateSetNamespace{
			TemplateRef: "basic-code-123abc1",
			Template:    "",
		})
	assert.Contains(t, mur.Spec.UserAccounts[0].Spec.NSTemplateSet.Namespaces,
		toolchainv1alpha1.NSTemplateSetNamespace{
			TemplateRef: "basic-dev-123abc2",
			Template:    "",
		})
	assert.Contains(t, mur.Spec.UserAccounts[0].Spec.NSTemplateSet.Namespaces,
		toolchainv1alpha1.NSTemplateSetNamespace{
			TemplateRef: "basic-stage-123abc3",
			Template:    "",
		})
	require.NotNil(t, mur.Spec.UserAccounts[0].Spec.NSTemplateSet.ClusterResources)
	assert.Equal(t, "basic-clusterresources-654321b", mur.Spec.UserAccounts[0].Spec.NSTemplateSet.ClusterResources.TemplateRef)

	test.AssertConditionsMatch(t, userSignup.Status.Conditions,
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupApproved,
			Status: v1.ConditionTrue,
			Reason: "ApprovedAutomatically",
		},
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupUserDeactivatedNotificationCreated,
			Status: v1.ConditionFalse,
			Reason: "UserIsActive",
		})

	AssertThatCounterHas(t, 2)

	t.Run("second reconcile", func(t *testing.T) {
		// when
		res, err = r.Reconcile(req)

		// then
		require.NoError(t, err)
		require.Equal(t, reconcile.Result{}, res)

		// Lookup the userSignup one more and check the conditions are updated
		err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
		require.NoError(t, err)
		require.Equal(t, userSignup.Status.CompliantUsername, mur.Name)
		assert.Equal(t, "true", userSignup.Labels[v1alpha1.UserSignupApprovedLabelKey])

		test.AssertConditionsMatch(t, userSignup.Status.Conditions,
			v1alpha1.Condition{
				Type:   v1alpha1.UserSignupApproved,
				Status: v1.ConditionTrue,
				Reason: "ApprovedAutomatically",
			},
			v1alpha1.Condition{
				Type:   v1alpha1.UserSignupComplete,
				Status: v1.ConditionTrue,
			},
			v1alpha1.Condition{
				Type:   v1alpha1.UserSignupUserDeactivatedNotificationCreated,
				Status: v1.ConditionFalse,
				Reason: "UserIsActive",
			})
	})
	AssertThatCounterHas(t, 2)
}

func TestUserSignupWithMissingEmailLabelFails(t *testing.T) {
	// given
	InitializeCounter(t, 1)
	defer counter.Reset()
	userSignup := NewUserSignup()
	userSignup.Annotations = map[string]string{}

	ready := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue))
	r, req, _ := prepareReconcile(t, userSignup.Name, ready, userSignup, configMap(configuration.UserApprovalPolicyAutomatic), basicNSTemplateTier)

	// when
	_, err := r.Reconcile(req)

	// then
	require.Error(t, err)

	// Lookup the user signup again
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)
	assert.Equal(t, "false", userSignup.Labels[v1alpha1.UserSignupApprovedLabelKey])
	test.AssertConditionsMatch(t, userSignup.Status.Conditions,
		v1alpha1.Condition{
			Type:    v1alpha1.UserSignupComplete,
			Status:  v1.ConditionFalse,
			Reason:  "MissingUserEmailAnnotation",
			Message: "missing annotation at usersignup",
		})
	AssertThatCounterHas(t, 1)
}

func TestUserSignupWithInvalidEmailHashLabelFails(t *testing.T) {
	// given
	InitializeCounter(t, 1)
	defer counter.Reset()
	userSignup := NewUserSignup()
	userSignup.Annotations = map[string]string{
		toolchainv1alpha1.UserSignupUserEmailAnnotationKey: "foo@redhat.com",
	}
	userSignup.Labels = map[string]string{
		toolchainv1alpha1.UserSignupUserEmailHashLabelKey: "abcdef0123456789",
		"toolchain.dev.openshift.com/approved":            "false",
	}

	ready := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue))
	r, req, _ := prepareReconcile(t, userSignup.Name, ready, userSignup, configMap(configuration.UserApprovalPolicyAutomatic), basicNSTemplateTier)

	// when
	_, err := r.Reconcile(req)

	// then
	require.Error(t, err)

	// Lookup the user signup again
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)
	assert.Equal(t, "false", userSignup.Labels[v1alpha1.UserSignupApprovedLabelKey])
	test.AssertConditionsMatch(t, userSignup.Status.Conditions,
		v1alpha1.Condition{
			Type:    v1alpha1.UserSignupComplete,
			Status:  v1.ConditionFalse,
			Reason:  "InvalidEmailHashLabel",
			Message: "hash is invalid",
		})
	AssertThatCounterHas(t, 1)
}

func TestUpdateOfApprovedLabelFails(t *testing.T) {
	// given
	InitializeCounter(t, 1)
	defer counter.Reset()
	userSignup := NewUserSignup()

	ready := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue))
	r, req, fakeClient := prepareReconcile(t, userSignup.Name, ready, userSignup, configMap(configuration.UserApprovalPolicyAutomatic), basicNSTemplateTier)
	fakeClient.MockUpdate = func(ctx context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
		return fmt.Errorf("some error")
	}

	// when
	_, err := r.Reconcile(req)

	// then
	require.Error(t, err)

	// Lookup the user signup again
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)
	test.AssertConditionsMatch(t, userSignup.Status.Conditions,
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupApproved,
			Status: v1.ConditionTrue,
			Reason: "ApprovedAutomatically",
		},
		v1alpha1.Condition{
			Type:    v1alpha1.UserSignupComplete,
			Status:  v1.ConditionFalse,
			Reason:  "UnableToUpdateApprovedLabel",
			Message: "some error",
		},
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupUserDeactivatedNotificationCreated,
			Status: v1.ConditionFalse,
			Reason: "UserIsActive",
		})
	AssertThatCounterHas(t, 1)
}

func TestUserSignupWithMissingEmailHashLabelFails(t *testing.T) {
	// given
	InitializeCounter(t, 1)
	defer counter.Reset()
	userSignup := NewUserSignup()
	userSignup.Annotations = map[string]string{
		toolchainv1alpha1.UserSignupUserEmailAnnotationKey: "foo@redhat.com",
	}
	userSignup.Labels = map[string]string{"toolchain.dev.openshift.com/approved": "false"}

	ready := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue))
	r, req, _ := prepareReconcile(t, userSignup.Name, ready, userSignup, configMap(configuration.UserApprovalPolicyAutomatic), basicNSTemplateTier)

	// when
	_, err := r.Reconcile(req)

	// then
	require.Error(t, err)

	// Lookup the user signup again
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)
	assert.Equal(t, "false", userSignup.Labels[v1alpha1.UserSignupApprovedLabelKey])
	test.AssertConditionsMatch(t, userSignup.Status.Conditions,
		v1alpha1.Condition{
			Type:    v1alpha1.UserSignupComplete,
			Status:  v1.ConditionFalse,
			Reason:  "MissingEmailHashLabel",
			Message: "missing label at usersignup",
		})
	AssertThatCounterHas(t, 1)
}

func TestUserSignupFailedMissingNSTemplateTier(t *testing.T) {
	// given
	InitializeCounter(t, 1)
	defer counter.Reset()
	userSignup := NewUserSignup()
	ready := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue))
	r, req, _ := prepareReconcile(t, userSignup.Name, ready, userSignup, configMap(configuration.UserApprovalPolicyAutomatic)) // basicNSTemplateTier does not exist

	// when
	_, err := r.Reconcile(req)

	// then
	// error reported, and request is requeued and userSignup status was updated
	require.Error(t, err)
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)
	t.Logf("usersignup status: %+v", userSignup.Status)
	test.AssertConditionsMatch(t, userSignup.Status.Conditions,
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupApproved,
			Status: v1.ConditionTrue,
			Reason: "ApprovedAutomatically",
		},
		v1alpha1.Condition{
			Type:    v1alpha1.UserSignupComplete,
			Status:  v1.ConditionFalse,
			Reason:  "NoTemplateTierAvailable",
			Message: "nstemplatetiers.toolchain.dev.openshift.com \"basic\" not found",
		},
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupUserDeactivatedNotificationCreated,
			Status: v1.ConditionFalse,
			Reason: "UserIsActive",
		})
	assert.Equal(t, "false", userSignup.Labels[v1alpha1.UserSignupApprovedLabelKey])
	AssertThatCounterHas(t, 1)
}

func TestUserSignupFailedNoClusterReady(t *testing.T) {
	// given
	InitializeCounter(t, 2)
	defer counter.Reset()
	userSignup := NewUserSignup()

	notReady := NewGetMemberClusters(
		NewMemberCluster(t, "member1", v1.ConditionFalse),
		NewMemberCluster(t, "member2", v1.ConditionFalse))
	r, req, _ := prepareReconcile(t, userSignup.Name, notReady, userSignup, configMap(configuration.UserApprovalPolicyAutomatic), basicNSTemplateTier)

	// when
	res, err := r.Reconcile(req)

	// then
	// error reported, and request is requeued and userSignup status was updated
	require.Error(t, err)
	assert.Equal(t, reconcile.Result{Requeue: false}, res)
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)
	t.Logf("usersignup status: %+v", userSignup.Status)
	test.AssertConditionsMatch(t, userSignup.Status.Conditions,
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupApproved,
			Status: v1.ConditionTrue,
			Reason: "ApprovedAutomatically",
		},
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupComplete,
			Status: v1.ConditionFalse,
			Reason: "NoClusterAvailable",
		},
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupUserDeactivatedNotificationCreated,
			Status: v1.ConditionFalse,
			Reason: "UserIsActive",
		})

	assert.Equal(t, "false", userSignup.Labels[v1alpha1.UserSignupApprovedLabelKey])

	AssertThatCounterHas(t, 2)
}

func TestUserSignupFailedNoClusterWithCapacityAvailable(t *testing.T) {
	// given
	InitializeCounter(t, 1)
	defer counter.Reset()
	userSignup := NewUserSignup()
	member1 := NewMemberCluster(t, "member1", v1.ConditionTrue)
	member1.CapacityExhausted = true
	member2 := NewMemberCluster(t, "member2", v1.ConditionTrue)
	member2.CapacityExhausted = true
	noCapacity := NewGetMemberClusters(member1, member2)
	r, req, _ := prepareReconcile(t, userSignup.Name, noCapacity, userSignup, configMap(configuration.UserApprovalPolicyAutomatic), basicNSTemplateTier)

	// when
	res, err := r.Reconcile(req)

	// then
	// error reported, and request is NOT requeued and userSignup status was updated
	require.Error(t, err)
	assert.Equal(t, reconcile.Result{Requeue: false}, res)
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)
	t.Logf("usersignup status: %+v", userSignup.Status)
	test.AssertConditionsMatch(t, userSignup.Status.Conditions,
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupApproved,
			Status: v1.ConditionTrue,
			Reason: "ApprovedAutomatically",
		},
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupComplete,
			Status: v1.ConditionFalse,
			Reason: "NoClusterAvailable",
		},
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupUserDeactivatedNotificationCreated,
			Status: v1.ConditionFalse,
			Reason: "UserIsActive",
		})

	assert.Equal(t, "false", userSignup.Labels[v1alpha1.UserSignupApprovedLabelKey])

	AssertThatCounterHas(t, 1)
}

func TestUserSignupWithManualApprovalApproved(t *testing.T) {
	// given
	InitializeCounter(t, 1)
	defer counter.Reset()
	userSignup := NewUserSignup(Approved())

	ready := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue))
	r, req, _ := prepareReconcile(t, userSignup.Name, ready, userSignup, configMap(configuration.UserApprovalPolicyAutomatic), basicNSTemplateTier)

	// when
	res, err := r.Reconcile(req)

	// then
	require.NoError(t, err)
	require.Equal(t, reconcile.Result{}, res)

	// Lookup the userSignup again
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)
	assert.Equal(t, "true", userSignup.Labels[v1alpha1.UserSignupApprovedLabelKey])

	murs := &v1alpha1.MasterUserRecordList{}
	err = r.client.List(context.TODO(), murs)
	require.NoError(t, err)
	require.Len(t, murs.Items, 1)

	mur := murs.Items[0]

	require.Equal(t, test.HostOperatorNs, mur.Namespace)
	require.Equal(t, userSignup.Name, mur.Labels[v1alpha1.MasterUserRecordUserIDLabelKey])
	require.Len(t, mur.Spec.UserAccounts, 1)

	test.AssertConditionsMatch(t, userSignup.Status.Conditions,
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupApproved,
			Status: v1.ConditionTrue,
			Reason: "ApprovedByAdmin",
		},
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupUserDeactivatedNotificationCreated,
			Status: v1.ConditionFalse,
			Reason: "UserIsActive",
		})
	AssertThatCounterHas(t, 2)

	t.Run("second reconcile", func(t *testing.T) {
		// when
		res, err = r.Reconcile(req)

		// then
		require.NoError(t, err)
		require.Equal(t, reconcile.Result{}, res)

		// Lookup the userSignup one more time and check the conditions are updated
		err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
		require.NoError(t, err)
		require.Equal(t, userSignup.Status.CompliantUsername, mur.Name)
		assert.Equal(t, "true", userSignup.Labels[v1alpha1.UserSignupApprovedLabelKey])
		test.AssertConditionsMatch(t, userSignup.Status.Conditions,
			v1alpha1.Condition{
				Type:   v1alpha1.UserSignupApproved,
				Status: v1.ConditionTrue,
				Reason: "ApprovedByAdmin",
			},
			v1alpha1.Condition{
				Type:   v1alpha1.UserSignupComplete,
				Status: v1.ConditionTrue,
			},
			v1alpha1.Condition{
				Type:   v1alpha1.UserSignupUserDeactivatedNotificationCreated,
				Status: v1.ConditionFalse,
				Reason: "UserIsActive",
			})
		AssertThatCounterHas(t, 2)
	})
}

func TestUserSignupWithNoApprovalPolicyTreatedAsManualApproved(t *testing.T) {
	// given
	InitializeCounter(t, 1)
	defer counter.Reset()
	userSignup := NewUserSignup(Approved())

	ready := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue))
	r, req, _ := prepareReconcile(t, userSignup.Name, ready, userSignup, basicNSTemplateTier)

	// when
	res, err := r.Reconcile(req)

	// then
	require.NoError(t, err)
	require.Equal(t, reconcile.Result{}, res)

	// Lookup the userSignup again
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)
	assert.Equal(t, "true", userSignup.Labels[v1alpha1.UserSignupApprovedLabelKey])

	murs := &v1alpha1.MasterUserRecordList{}
	err = r.client.List(context.TODO(), murs)
	require.NoError(t, err)
	require.Len(t, murs.Items, 1)

	mur := murs.Items[0]

	require.Equal(t, test.HostOperatorNs, mur.Namespace)
	require.Equal(t, userSignup.Name, mur.Labels[v1alpha1.MasterUserRecordUserIDLabelKey])
	require.Len(t, mur.Spec.UserAccounts, 1)

	test.AssertConditionsMatch(t, userSignup.Status.Conditions,
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupApproved,
			Status: v1.ConditionTrue,
			Reason: "ApprovedByAdmin",
		},
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupUserDeactivatedNotificationCreated,
			Status: v1.ConditionFalse,
			Reason: "UserIsActive",
		})
	AssertThatCounterHas(t, 2)

	t.Run("second reconcile", func(t *testing.T) {
		// when
		res, err = r.Reconcile(req)

		// then
		require.NoError(t, err)
		require.Equal(t, reconcile.Result{}, res)

		// Lookup the userSignup one more and check the conditions are updated
		err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
		require.NoError(t, err)
		require.Equal(t, userSignup.Status.CompliantUsername, mur.Name)

		assert.Equal(t, "true", userSignup.Labels[v1alpha1.UserSignupApprovedLabelKey])

		test.AssertConditionsMatch(t, userSignup.Status.Conditions,
			v1alpha1.Condition{
				Type:   v1alpha1.UserSignupApproved,
				Status: v1.ConditionTrue,
				Reason: "ApprovedByAdmin",
			},
			v1alpha1.Condition{
				Type:   v1alpha1.UserSignupComplete,
				Status: v1.ConditionTrue,
			},
			v1alpha1.Condition{
				Type:   v1alpha1.UserSignupUserDeactivatedNotificationCreated,
				Status: v1.ConditionFalse,
				Reason: "UserIsActive",
			})
		AssertThatCounterHas(t, 2)
	})
}

func TestUserSignupWithManualApprovalNotApproved(t *testing.T) {
	// given
	InitializeCounter(t, 1)
	defer counter.Reset()
	userSignup := NewUserSignup()

	ready := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue))
	r, req, _ := prepareReconcile(t, userSignup.Name, ready, userSignup, configMap(configuration.UserApprovalPolicyManual), basicNSTemplateTier)

	// when
	res, err := r.Reconcile(req)

	// then
	require.NoError(t, err)
	require.Equal(t, reconcile.Result{}, res)

	// Lookup the userSignup again
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)
	assert.Equal(t, "false", userSignup.Labels[v1alpha1.UserSignupApprovedLabelKey])

	// There should be no MasterUserRecords
	murs := &v1alpha1.MasterUserRecordList{}
	err = r.client.List(context.TODO(), murs)
	require.NoError(t, err)
	require.Len(t, murs.Items, 0)

	test.AssertConditionsMatch(t, userSignup.Status.Conditions,
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupApproved,
			Status: v1.ConditionFalse,
			Reason: "PendingApproval",
		},
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupComplete,
			Status: v1.ConditionFalse,
			Reason: "PendingApproval",
		},
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupUserDeactivatedNotificationCreated,
			Status: v1.ConditionFalse,
			Reason: "UserIsActive",
		})
	AssertThatCounterHas(t, 1)
}

func TestUserSignupWithAutoApprovalWithTargetCluster(t *testing.T) {
	// given
	InitializeCounter(t, 1)
	defer counter.Reset()
	userSignup := NewUserSignup(WithTargetCluster("east"))

	ready := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue))
	r, req, _ := prepareReconcile(t, userSignup.Name, ready, userSignup, configMap(configuration.UserApprovalPolicyAutomatic), basicNSTemplateTier)

	// when
	res, err := r.Reconcile(req)

	// then
	require.NoError(t, err)
	require.Equal(t, reconcile.Result{}, res)

	// Lookup the userSignup again
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)
	assert.Equal(t, "true", userSignup.Labels[v1alpha1.UserSignupApprovedLabelKey])

	murs := &v1alpha1.MasterUserRecordList{}
	err = r.client.List(context.TODO(), murs)
	require.NoError(t, err)
	require.Len(t, murs.Items, 1)

	mur := murs.Items[0]
	require.Equal(t, test.HostOperatorNs, mur.Namespace)
	require.Equal(t, userSignup.Name, mur.Labels[v1alpha1.MasterUserRecordUserIDLabelKey])
	require.Len(t, mur.Spec.UserAccounts, 1)
	assert.Equal(t, "basic", mur.Spec.UserAccounts[0].Spec.NSTemplateSet.TierName)
	require.Len(t, mur.Spec.UserAccounts[0].Spec.NSTemplateSet.Namespaces, 3)

	test.AssertConditionsMatch(t, userSignup.Status.Conditions,
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupApproved,
			Status: v1.ConditionTrue,
			Reason: "ApprovedAutomatically",
		},
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupUserDeactivatedNotificationCreated,
			Status: v1.ConditionFalse,
			Reason: "UserIsActive",
		})
	AssertThatCounterHas(t, 2)

	t.Run("second reconcile", func(t *testing.T) {
		// when
		res, err = r.Reconcile(req)

		// then
		require.NoError(t, err)
		require.Equal(t, reconcile.Result{}, res)

		// Lookup the userSignup one more and check the conditions are updated
		err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
		require.NoError(t, err)
		require.Equal(t, userSignup.Status.CompliantUsername, mur.Name)

		assert.Equal(t, "true", userSignup.Labels[v1alpha1.UserSignupApprovedLabelKey])

		test.AssertConditionsMatch(t, userSignup.Status.Conditions,
			v1alpha1.Condition{
				Type:   v1alpha1.UserSignupApproved,
				Status: v1.ConditionTrue,
				Reason: "ApprovedAutomatically",
			},
			v1alpha1.Condition{
				Type:   v1alpha1.UserSignupComplete,
				Status: v1.ConditionTrue,
			},
			v1alpha1.Condition{
				Type:   v1alpha1.UserSignupUserDeactivatedNotificationCreated,
				Status: v1.ConditionFalse,
				Reason: "UserIsActive",
			})
		AssertThatCounterHas(t, 2)
	})
}

func TestUserSignupWithMissingApprovalPolicyTreatedAsManual(t *testing.T) {
	// given
	InitializeCounter(t, 1)
	defer counter.Reset()
	userSignup := NewUserSignup(WithTargetCluster("east"))

	r, req, _ := prepareReconcile(t, userSignup.Name, NewGetMemberClusters(), userSignup, basicNSTemplateTier)

	// when
	res, err := r.Reconcile(req)

	// then
	require.NoError(t, err)
	require.Equal(t, reconcile.Result{}, res)

	// Lookup the userSignup again
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)
	assert.Equal(t, "false", userSignup.Labels[v1alpha1.UserSignupApprovedLabelKey])

	test.AssertConditionsMatch(t, userSignup.Status.Conditions,
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupApproved,
			Status: v1.ConditionFalse,
			Reason: "PendingApproval",
		},
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupComplete,
			Status: v1.ConditionFalse,
			Reason: "PendingApproval",
		},
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupUserDeactivatedNotificationCreated,
			Status: v1.ConditionFalse,
			Reason: "UserIsActive",
		})
	AssertThatCounterHas(t, 1)
}

func TestUserSignupMURCreateFails(t *testing.T) {
	// given
	InitializeCounter(t, 1)
	defer counter.Reset()
	userSignup := NewUserSignup(Approved())

	ready := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue))
	r, req, clt := prepareReconcile(t, userSignup.Name, ready, userSignup, basicNSTemplateTier)

	clt.MockCreate = func(ctx context.Context, obj runtime.Object, opts ...client.CreateOption) error {
		switch obj.(type) {
		case *v1alpha1.MasterUserRecord:
			return errors.New("unable to create mur")
		default:
			return clt.Create(ctx, obj)
		}
	}

	// when
	res, err := r.Reconcile(req)

	// then
	require.Error(t, err)
	require.Equal(t, reconcile.Result{}, res)
	AssertThatCounterHas(t, 1)

	err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)
	assert.Equal(t, "true", userSignup.Labels[v1alpha1.UserSignupApprovedLabelKey])

}

func TestUserSignupMURReadFails(t *testing.T) {
	// given
	InitializeCounter(t, 1)
	defer counter.Reset()
	userSignup := NewUserSignup(Approved())

	ready := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue))
	r, req, fakeClient := prepareReconcile(t, userSignup.Name, ready, userSignup)

	fakeClient.MockGet = func(ctx context.Context, key client.ObjectKey, obj runtime.Object) error {
		switch obj.(type) {
		case *v1alpha1.MasterUserRecord:
			return errors.New("failed to lookup MUR")
		default:
			return fakeClient.Client.Get(ctx, key, obj)
		}
	}

	// when
	_, err := r.Reconcile(req)

	// then
	require.Error(t, err)
	AssertThatCounterHas(t, 1)

	err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)
	assert.Equal(t, "false", userSignup.Labels[v1alpha1.UserSignupApprovedLabelKey])

}

func TestUserSignupSetStatusApprovedByAdminFails(t *testing.T) {
	// given
	InitializeCounter(t, 1)
	defer counter.Reset()
	userSignup := NewUserSignup(Approved())

	ready := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue))
	r, req, fakeClient := prepareReconcile(t, userSignup.Name, ready, userSignup)

	fakeClient.MockStatusUpdate = func(ctx context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
		switch obj.(type) {
		case *v1alpha1.UserSignup:
			return errors.New("failed to update UserSignup status")
		default:
			return fakeClient.Client.Update(ctx, obj)
		}
	}

	// when
	res, err := r.Reconcile(req)

	// then
	require.Error(t, err)
	require.Equal(t, reconcile.Result{}, res)
	AssertThatCounterHas(t, 1)

	err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)
	assert.Equal(t, "false", userSignup.Labels[v1alpha1.UserSignupApprovedLabelKey])
	assert.Empty(t, userSignup.Status.Conditions)
}

func TestUserSignupSetStatusApprovedAutomaticallyFails(t *testing.T) {
	// given
	InitializeCounter(t, 1)
	defer counter.Reset()
	userSignup := NewUserSignup()

	ready := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue))
	r, req, fakeClient := prepareReconcile(t, userSignup.Name, ready, userSignup, configMap(configuration.UserApprovalPolicyAutomatic))

	fakeClient.MockStatusUpdate = func(ctx context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
		switch obj.(type) {
		case *v1alpha1.UserSignup:
			return errors.New("failed to update UserSignup status")
		default:
			return fakeClient.Client.Update(ctx, obj)
		}
	}

	// when
	res, err := r.Reconcile(req)

	// then
	require.Error(t, err)
	require.Equal(t, reconcile.Result{}, res)
	AssertThatCounterHas(t, 1)

	err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)
	assert.Equal(t, "false", userSignup.Labels[v1alpha1.UserSignupApprovedLabelKey])
	assert.Empty(t, userSignup.Status.Conditions)

}

func TestUserSignupSetStatusNoClustersAvailableFails(t *testing.T) {
	// given
	InitializeCounter(t, 1)
	defer counter.Reset()
	userSignup := NewUserSignup()

	r, req, fakeClient := prepareReconcile(t, userSignup.Name, NewGetMemberClusters(), userSignup, configMap(configuration.UserApprovalPolicyAutomatic))

	fakeClient.MockStatusUpdate = func(ctx context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
		switch obj := obj.(type) {
		case *v1alpha1.UserSignup:
			for _, cond := range obj.Status.Conditions {
				if cond.Reason == "NoClusterAvailable" {
					return errors.New("failed to update UserSignup status")
				}
			}
			return fakeClient.Client.Update(ctx, obj)
		default:
			return fakeClient.Client.Update(ctx, obj)
		}
	}

	// when
	res, err := r.Reconcile(req)

	// then
	require.Error(t, err)
	require.Equal(t, reconcile.Result{}, res)
	AssertThatCounterHas(t, 1)

	err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)
	assert.Equal(t, "false", userSignup.Labels[v1alpha1.UserSignupApprovedLabelKey])

}

func TestUserSignupWithExistingMUROK(t *testing.T) {
	// given
	InitializeCounter(t, 1)
	defer counter.Reset()
	userSignup := NewUserSignup()
	userSignup.Annotations = map[string]string{
		toolchainv1alpha1.UserSignupUserEmailAnnotationKey: "foo@redhat.com",
	}
	userSignup.Labels = map[string]string{
		toolchainv1alpha1.UserSignupUserEmailHashLabelKey: "fd2addbd8d82f0d2dc088fa122377eaa",
		"toolchain.dev.openshift.com/approved":            "true",
	}

	// Create a MUR with the same UserID
	mur := &v1alpha1.MasterUserRecord{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: test.HostOperatorNs,
			Labels:    map[string]string{v1alpha1.MasterUserRecordUserIDLabelKey: userSignup.Name},
		},
	}

	ready := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue))
	r, req, _ := prepareReconcile(t, userSignup.Name, ready, userSignup, mur, configMap(configuration.UserApprovalPolicyAutomatic), basicNSTemplateTier)

	// when
	_, err := r.Reconcile(req)

	// then
	require.NoError(t, err)

	key := types.NamespacedName{
		Namespace: test.HostOperatorNs,
		Name:      userSignup.Name,
	}
	instance := &v1alpha1.UserSignup{}
	err = r.client.Get(context.TODO(), key, instance)
	require.NoError(t, err)
	assert.Equal(t, "true", userSignup.Labels[v1alpha1.UserSignupApprovedLabelKey])

	require.Equal(t, mur.Name, instance.Status.CompliantUsername)
	test.AssertContainsCondition(t, instance.Status.Conditions, v1alpha1.Condition{
		Type:   v1alpha1.UserSignupComplete,
		Status: v1.ConditionTrue,
	})
	AssertThatCounterHas(t, 1)
}

func TestUserSignupWithExistingMURDifferentUserIDOK(t *testing.T) {
	// given
	InitializeCounter(t, 1)
	defer counter.Reset()
	userSignup := NewUserSignup(Approved())

	// Create a MUR with a different UserID
	mur := &v1alpha1.MasterUserRecord{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: test.HostOperatorNs,
			Labels: map[string]string{
				v1alpha1.MasterUserRecordUserIDLabelKey: uuid.NewV4().String(),
				"toolchain.dev.openshift.com/approved":  "true",
			},
		},
	}

	ready := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue))
	r, req, _ := prepareReconcile(t, userSignup.Name, ready, userSignup, mur, configMap(configuration.UserApprovalPolicyAutomatic), basicNSTemplateTier)

	// when
	_, err := r.Reconcile(req)

	// then
	require.NoError(t, err)

	// We should now have 2 MURs
	murs := &v1alpha1.MasterUserRecordList{}
	err = r.client.List(context.TODO(), murs)
	require.NoError(t, err)
	require.Len(t, murs.Items, 2)
	AssertThatCounterHas(t, 2)

	key := types.NamespacedName{
		Namespace: test.HostOperatorNs,
		Name:      userSignup.Name,
	}
	instance := &v1alpha1.UserSignup{}
	err = r.client.Get(context.TODO(), key, instance)
	require.NoError(t, err)
	assert.Equal(t, "true", instance.Labels[v1alpha1.UserSignupApprovedLabelKey])

	t.Run("second reconcile", func(t *testing.T) {
		// when
		_, err = r.Reconcile(req)

		// then
		require.NoError(t, err)

		err = r.client.Get(context.TODO(), key, instance)
		require.NoError(t, err)
		assert.Equal(t, "true", instance.Labels[v1alpha1.UserSignupApprovedLabelKey])

		require.Equal(t, "foo-2", instance.Status.CompliantUsername)

		// Confirm that the mur exists
		mur = &v1alpha1.MasterUserRecord{}
		err = r.client.Get(context.TODO(), types.NamespacedName{Namespace: test.HostOperatorNs, Name: instance.Status.CompliantUsername}, mur)
		require.NoError(t, err)
		require.Equal(t, instance.Name, mur.Labels[v1alpha1.MasterUserRecordUserIDLabelKey])

		var cond *v1alpha1.Condition
		for _, condition := range instance.Status.Conditions {
			if condition.Type == v1alpha1.UserSignupComplete {
				cond = &condition
			}
		}

		require.Equal(t, mur.Name, instance.Status.CompliantUsername)
		require.NotNil(t, cond)
		require.Equal(t, v1.ConditionTrue, cond.Status)
		AssertThatCounterHas(t, 2)
	})
}

func TestUserSignupWithSpecialCharOK(t *testing.T) {
	// given
	InitializeCounter(t, 1)
	defer counter.Reset()
	userSignup := NewUserSignup(WithUsername("foo#$%^bar@redhat.com"))

	ready := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue))
	r, req, _ := prepareReconcile(t, userSignup.Name, ready, userSignup, configMap(configuration.UserApprovalPolicyAutomatic), basicNSTemplateTier)

	// when
	_, err := r.Reconcile(req)

	// then
	require.NoError(t, err)

	murtest.AssertThatMasterUserRecord(t, "foo-bar", r.client).HasNoConditions()
	AssertThatCounterHas(t, 2)
}

func TestUserSignupDeactivatedAfterMURCreated(t *testing.T) {
	// given
	userSignup := NewUserSignup(Deactivated())
	userSignup.Status = v1alpha1.UserSignupStatus{
		Conditions: []v1alpha1.Condition{
			{
				Type:   v1alpha1.UserSignupComplete,
				Status: v1.ConditionTrue,
			},
			{
				Type:   v1alpha1.UserSignupApproved,
				Status: v1.ConditionTrue,
				Reason: "ApprovedAutomatically",
			},
		},
		CompliantUsername: "john-doe",
	}

	userSignup.Labels["toolchain.dev.openshift.com/approved"] = "true"
	key := test.NamespacedName(test.HostOperatorNs, userSignup.Name)

	t.Run("when MUR exists, then it should be deleted", func(t *testing.T) {
		// given
		InitializeCounter(t, 1)
		defer counter.Reset()
		mur := murtest.NewMasterUserRecord(t, "john-doe", murtest.MetaNamespace(test.HostOperatorNs))
		mur.Labels = map[string]string{toolchainv1alpha1.MasterUserRecordUserIDLabelKey: userSignup.Name}

		r, req, _ := prepareReconcile(t, userSignup.Name, NewGetMemberClusters(), userSignup, mur, configMap(configuration.UserApprovalPolicyAutomatic), basicNSTemplateTier)

		// when
		_, err := r.Reconcile(req)

		// then
		require.NoError(t, err)
		err = r.client.Get(context.TODO(), key, userSignup)
		require.NoError(t, err)
		assert.Equal(t, "false", userSignup.Labels[v1alpha1.UserSignupApprovedLabelKey])

		// Confirm the status is now set to Deactivating
		test.AssertConditionsMatch(t, userSignup.Status.Conditions,
			v1alpha1.Condition{
				Type:   v1alpha1.UserSignupApproved,
				Status: v1.ConditionTrue,
				Reason: "ApprovedAutomatically",
			},
			v1alpha1.Condition{
				Type:   v1alpha1.UserSignupComplete,
				Status: v1.ConditionFalse,
				Reason: "Deactivating",
			})

		murs := &v1alpha1.MasterUserRecordList{}

		// The MUR should have now been deleted
		err = r.client.List(context.TODO(), murs)
		require.NoError(t, err)
		require.Len(t, murs.Items, 0)
		AssertThatCounterHas(t, 1)

		// There should not be a notification created yet, only the next reconcile (with deleted mur) would create the notification
		ntest.AssertNoNotificationsExist(t, r.client)
	})

	t.Run("when MUR doesn't exist, then the condition should be set to Deactivated", func(t *testing.T) {
		// given
		InitializeCounter(t, 2)
		defer counter.Reset()
		r, req, _ := prepareReconcile(t, userSignup.Name, NewGetMemberClusters(), userSignup, configMap(configuration.UserApprovalPolicyAutomatic), basicNSTemplateTier)

		// when
		_, err := r.Reconcile(req)

		// then
		require.NoError(t, err)

		// Lookup the UserSignup
		err = r.client.Get(context.TODO(), key, userSignup)
		require.NoError(t, err)
		assert.Equal(t, "false", userSignup.Labels[v1alpha1.UserSignupApprovedLabelKey])

		// Confirm the status has been set to Deactivated
		test.AssertConditionsMatch(t, userSignup.Status.Conditions,
			v1alpha1.Condition{
				Type:   v1alpha1.UserSignupApproved,
				Status: v1.ConditionTrue,
				Reason: "ApprovedAutomatically",
			},
			v1alpha1.Condition{
				Type:   v1alpha1.UserSignupComplete,
				Status: v1.ConditionTrue,
				Reason: "Deactivated",
			},
			v1alpha1.Condition{
				Type:   v1alpha1.UserSignupUserDeactivatedNotificationCreated,
				Status: v1.ConditionTrue,
				Reason: "NotificationCRCreated",
			})
		AssertThatCounterHas(t, 2)

		// A deactivated notification should have been created
		notification := &v1alpha1.Notification{}
		err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Status.CompliantUsername + "-deactivated", Namespace: userSignup.Namespace}, notification)
		require.NoError(t, err)
		require.Equal(t, "john-doe-deactivated", notification.Name)
		require.Equal(t, userSignup.Name, notification.Spec.UserID)
		assert.Equal(t, "userdeactivated", notification.Spec.Template)
	})

}

func TestUserSignupFailedToCreateDeactivationNotification(t *testing.T) {
	// given
	userSignup := &v1alpha1.UserSignup{
		ObjectMeta: NewUserSignupObjectMeta("", "john.doe@redhat.com"),
		Spec: v1alpha1.UserSignupSpec{
			Username:    "john.doe@redhat.com",
			Deactivated: true,
		},
		Status: v1alpha1.UserSignupStatus{
			Conditions: []v1alpha1.Condition{
				{
					Type:   v1alpha1.UserSignupComplete,
					Status: v1.ConditionTrue,
				},
				{
					Type:   v1alpha1.UserSignupApproved,
					Status: v1.ConditionTrue,
					Reason: "ApprovedAutomatically",
				},
			},
			CompliantUsername: "john-doe",
		},
	}
	userSignup.Labels["toolchain.dev.openshift.com/approved"] = "true"
	key := test.NamespacedName(test.HostOperatorNs, userSignup.Name)

	t.Run("when the deactivation notification cannot be created", func(t *testing.T) {
		// given
		InitializeCounter(t, 2)
		defer counter.Reset()
		r, req, cl := prepareReconcile(t, userSignup.Name, NewGetMemberClusters(), userSignup, configMap(configuration.UserApprovalPolicyAutomatic), basicNSTemplateTier)

		cl.MockCreate = func(ctx context.Context, obj runtime.Object, opts ...client.CreateOption) error {
			switch obj.(type) {
			case *v1alpha1.Notification:
				return errors.New("unable to create deactivation notification")
			default:
				return cl.Create(ctx, obj)
			}
		}

		// when
		_, err := r.Reconcile(req)

		// then
		require.Error(t, err)
		require.Equal(t, "Failed to create user deactivation notification: unable to create deactivation notification", err.Error())

		// Lookup the UserSignup
		err = r.client.Get(context.TODO(), key, userSignup)
		require.NoError(t, err)

		// Confirm the status shows the deactivation notification failure
		test.AssertConditionsMatch(t, userSignup.Status.Conditions,
			v1alpha1.Condition{
				Type:   v1alpha1.UserSignupApproved,
				Status: v1.ConditionTrue,
				Reason: "ApprovedAutomatically",
			},
			v1alpha1.Condition{
				Type:   v1alpha1.UserSignupComplete,
				Status: v1.ConditionTrue,
			},
			v1alpha1.Condition{
				Type:    v1alpha1.UserSignupUserDeactivatedNotificationCreated,
				Status:  v1.ConditionFalse,
				Reason:  "NotificationCRCreationFailed",
				Message: "unable to create deactivation notification",
			})
		AssertThatCounterHas(t, 2)

		// A deactivated notification should not have been created
		notification := &v1alpha1.Notification{}
		err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Status.CompliantUsername + "-deactivated", Namespace: userSignup.Namespace}, notification)
		require.Error(t, err)
		require.Equal(t, "notifications.toolchain.dev.openshift.com \"john-doe-deactivated\" not found", err.Error())
	})
}

func TestUserSignupReactivateAfterDeactivated(t *testing.T) {
	// given
	userSignup := &v1alpha1.UserSignup{
		ObjectMeta: NewUserSignupObjectMeta("", "john.doe@redhat.com"),
		Spec: v1alpha1.UserSignupSpec{
			Username:    "john.doe@redhat.com",
			Deactivated: false,
		},
		Status: v1alpha1.UserSignupStatus{
			CompliantUsername: "john-doe",
		},
	}
	userSignup.Labels["toolchain.dev.openshift.com/approved"] = "true"
	key := test.NamespacedName(test.HostOperatorNs, userSignup.Name)

	t.Run("when reactivating the usersignup successfully", func(t *testing.T) {
		// given
		// start with a usersignup that has the Notification Created status set to "true" but Spec.Deactivated is set to "false" which signals a user which has been just reactivated.
		userSignup.Status.Conditions = []v1alpha1.Condition{
			{
				Type:   v1alpha1.UserSignupComplete,
				Status: v1.ConditionTrue,
				Reason: "Deactivated",
			},
			{
				Type:   v1alpha1.UserSignupApproved,
				Status: v1.ConditionTrue,
				Reason: "ApprovedAutomatically",
			},
			{
				Type:   v1alpha1.UserSignupUserDeactivatedNotificationCreated,
				Status: v1.ConditionTrue,
				Reason: "NotificationCRCreated",
			},
		}
		InitializeCounter(t, 2)
		defer counter.Reset()
		ready := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue))
		r, req, _ := prepareReconcile(t, userSignup.Name, ready, userSignup, configMap(configuration.UserApprovalPolicyAutomatic), basicNSTemplateTier)

		// when
		_, err := r.Reconcile(req)

		// then
		require.NoError(t, err)

		// Lookup the UserSignup
		err = r.client.Get(context.TODO(), key, userSignup)
		require.NoError(t, err)

		// Confirm the status shows the notification created condition is reset to active
		test.AssertConditionsMatch(t, userSignup.Status.Conditions,
			v1alpha1.Condition{
				Type:   v1alpha1.UserSignupApproved,
				Status: v1.ConditionTrue,
				Reason: "ApprovedAutomatically",
			},
			v1alpha1.Condition{
				Type:   v1alpha1.UserSignupComplete,
				Status: v1.ConditionTrue,
				Reason: "Deactivated",
			},
			v1alpha1.Condition{
				Type:   v1alpha1.UserSignupUserDeactivatedNotificationCreated,
				Status: v1.ConditionFalse,
				Reason: "UserIsActive",
			})

		// A mur should be created so the counter should be 3
		AssertThatCounterHas(t, 3)

		// There should not be a notification created because the user was reactivated
		ntest.AssertNoNotificationsExist(t, r.client)
	})

	t.Run("when resetting the usersignup deactivation notification status fails", func(t *testing.T) {
		// given
		// start with a usersignup that has the Notification Created status set to "true" but Spec.Deactivated is set to "false" which signals a user which has been just reactivated.
		userSignup.Status.Conditions = []v1alpha1.Condition{
			{
				Type:   v1alpha1.UserSignupComplete,
				Status: v1.ConditionTrue,
				Reason: "Deactivated",
			},
			{
				Type:   v1alpha1.UserSignupApproved,
				Status: v1.ConditionTrue,
				Reason: "ApprovedAutomatically",
			},
			{
				Type:   v1alpha1.UserSignupUserDeactivatedNotificationCreated,
				Status: v1.ConditionTrue,
				Reason: "NotificationCRCreated",
			},
		}
		InitializeCounter(t, 2)
		defer counter.Reset()
		r, req, cl := prepareReconcile(t, userSignup.Name, NewGetMemberClusters(), userSignup, configMap(configuration.UserApprovalPolicyAutomatic), basicNSTemplateTier)

		cl.MockStatusUpdate = func(ctx context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
			switch obj.(type) {
			case *v1alpha1.UserSignup:
				return errors.New("failed to update UserSignup status")
			default:
				return cl.Client.Update(ctx, obj)
			}
		}

		// when
		_, err := r.Reconcile(req)

		// then
		require.Error(t, err)
		require.Equal(t, "failed to update UserSignup status", err.Error())

		// Lookup the UserSignup
		err = r.client.Get(context.TODO(), key, userSignup)
		require.NoError(t, err)

		// Confirm the status shows the notification is unchanged because the status update failed
		test.AssertConditionsMatch(t, userSignup.Status.Conditions,
			v1alpha1.Condition{
				Type:   v1alpha1.UserSignupApproved,
				Status: v1.ConditionTrue,
				Reason: "ApprovedAutomatically",
			},
			v1alpha1.Condition{
				Type:   v1alpha1.UserSignupComplete,
				Status: v1.ConditionTrue,
				Reason: "Deactivated",
			},
			v1alpha1.Condition{
				Type:   v1alpha1.UserSignupUserDeactivatedNotificationCreated,
				Status: v1.ConditionTrue,
				Reason: "NotificationCRCreated",
			})
		AssertThatCounterHas(t, 2)

		// A deactivation notification should not be created because this is the reactivation case
		ntest.AssertNoNotificationsExist(t, r.client)
	})
}

func TestUserSignupDeactivatingWhenMURExists(t *testing.T) {
	userSignup := &v1alpha1.UserSignup{
		ObjectMeta: NewUserSignupObjectMeta("", "edward.jones@redhat.com"),
		Spec: v1alpha1.UserSignupSpec{
			Username:    "edward.jones@redhat.com",
			Deactivated: true,
		},
		Status: v1alpha1.UserSignupStatus{
			Conditions: []v1alpha1.Condition{
				{
					Type:   v1alpha1.UserSignupComplete,
					Status: v1.ConditionFalse,
					Reason: "Deactivating",
				},
				{
					Type:   v1alpha1.UserSignupApproved,
					Status: v1.ConditionTrue,
					Reason: "ApprovedAutomatically",
				},
			},
			CompliantUsername: "edward-jones",
		},
	}
	userSignup.Labels["toolchain.dev.openshift.com/approved"] = "true"
	key := test.NamespacedName(test.HostOperatorNs, userSignup.Name)

	t.Run("when MUR exists, then it should be deleted", func(t *testing.T) {
		// given
		InitializeCounter(t, 1)
		defer counter.Reset()
		mur := murtest.NewMasterUserRecord(t, "edward-jones", murtest.MetaNamespace(test.HostOperatorNs))
		mur.Labels = map[string]string{toolchainv1alpha1.MasterUserRecordUserIDLabelKey: userSignup.Name}

		r, req, _ := prepareReconcile(t, userSignup.Name, NewGetMemberClusters(), userSignup, mur, configMap(configuration.UserApprovalPolicyAutomatic), basicNSTemplateTier)

		t.Run("first reconcile - status should be deactivating and mur should be deleted", func(t *testing.T) {
			// when
			_, err := r.Reconcile(req)

			// then
			require.NoError(t, err)
			err = r.client.Get(context.TODO(), key, userSignup)
			require.NoError(t, err)
			assert.Equal(t, "false", userSignup.Labels[v1alpha1.UserSignupApprovedLabelKey])

			// Confirm the status is still set to Deactivating
			test.AssertConditionsMatch(t, userSignup.Status.Conditions,
				v1alpha1.Condition{
					Type:   v1alpha1.UserSignupApproved,
					Status: v1.ConditionTrue,
					Reason: "ApprovedAutomatically",
				},
				v1alpha1.Condition{
					Type:   v1alpha1.UserSignupComplete,
					Status: v1.ConditionFalse,
					Reason: "Deactivating",
				})

			murs := &v1alpha1.MasterUserRecordList{}

			// The MUR should have now been deleted
			err = r.client.List(context.TODO(), murs)
			require.NoError(t, err)
			require.Len(t, murs.Items, 0)
			AssertThatCounterHas(t, 1)

			// There should not be a notification created yet, only the next reconcile (with deleted mur) would create the notification
			ntest.AssertNoNotificationsExist(t, r.client)
		})

		t.Run("second reconcile - condition should be deactivated and deactivation notification created", func(t *testing.T) {
			res, err := r.Reconcile(req)
			require.NoError(t, err)
			require.Equal(t, reconcile.Result{}, res)

			// lookup the userSignup and check the conditions
			err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
			require.NoError(t, err)

			// Confirm the status has been set to Deactivated and the deactivation notification is created
			test.AssertConditionsMatch(t, userSignup.Status.Conditions,
				v1alpha1.Condition{
					Type:   v1alpha1.UserSignupApproved,
					Status: v1.ConditionTrue,
					Reason: "ApprovedAutomatically",
				},
				v1alpha1.Condition{
					Type:   v1alpha1.UserSignupComplete,
					Status: v1.ConditionTrue,
					Reason: "Deactivated",
				},
				v1alpha1.Condition{
					Type:   v1alpha1.UserSignupUserDeactivatedNotificationCreated,
					Status: v1.ConditionTrue,
					Reason: "NotificationCRCreated",
				})
		})
	})
}

func TestUserSignupBanned(t *testing.T) {
	// given
	InitializeCounter(t, 1)
	defer counter.Reset()
	userSignup := NewUserSignup()

	bannedUser := &toolchainv1alpha1.BannedUser{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				toolchainv1alpha1.BannedUserEmailHashLabelKey: "fd2addbd8d82f0d2dc088fa122377eaa",
			},
		},
		Spec: toolchainv1alpha1.BannedUserSpec{
			Email: "foo@redhat.com",
		},
	}

	r, req, _ := prepareReconcile(t, userSignup.Name, NewGetMemberClusters(), userSignup, bannedUser, configMap(configuration.UserApprovalPolicyAutomatic), basicNSTemplateTier)

	// when
	_, err := r.Reconcile(req)

	// then
	require.NoError(t, err)
	err = r.client.Get(context.TODO(), test.NamespacedName(test.HostOperatorNs, userSignup.Name), userSignup)
	require.NoError(t, err)
	assert.Equal(t, "false", userSignup.Labels[v1alpha1.UserSignupApprovedLabelKey])

	// Confirm the status is set to Banned
	test.AssertConditionsMatch(t, userSignup.Status.Conditions,
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupComplete,
			Status: v1.ConditionTrue,
			Reason: "Banned",
		})

	// Confirm that no MUR is created
	murs := &v1alpha1.MasterUserRecordList{}

	// Confirm that the MUR has now been deleted
	err = r.client.List(context.TODO(), murs)
	require.NoError(t, err)
	require.Len(t, murs.Items, 0)
	AssertThatCounterHas(t, 1)
}

func TestUserSignupVerificationRequired(t *testing.T) {
	// given
	InitializeCounter(t, 1)
	defer counter.Reset()
	userSignup := NewUserSignup(VerificationRequired())

	r, req, _ := prepareReconcile(t, userSignup.Name, NewGetMemberClusters(), userSignup, configMap(configuration.UserApprovalPolicyAutomatic), basicNSTemplateTier)

	// when
	_, err := r.Reconcile(req)

	// then
	require.NoError(t, err)
	err = r.client.Get(context.TODO(), test.NamespacedName(test.HostOperatorNs, userSignup.Name), userSignup)
	require.NoError(t, err)
	assert.Equal(t, "false", userSignup.Labels[v1alpha1.UserSignupApprovedLabelKey])

	// Confirm the status is set to VerificationRequired
	test.AssertConditionsMatch(t, userSignup.Status.Conditions,
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupComplete,
			Status: v1.ConditionFalse,
			Reason: "VerificationRequired",
		},
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupApproved,
			Status: v1.ConditionTrue,
			Reason: "ApprovedAutomatically",
		},
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupUserDeactivatedNotificationCreated,
			Status: v1.ConditionFalse,
			Reason: "UserIsActive",
		})

	// Confirm that no MUR is created
	murs := &v1alpha1.MasterUserRecordList{}
	err = r.client.List(context.TODO(), murs)
	require.NoError(t, err)
	require.Len(t, murs.Items, 0)
	AssertThatCounterHas(t, 1)
}

func TestUserSignupBannedMURExists(t *testing.T) {
	// given
	InitializeCounter(t, 1)
	defer counter.Reset()
	userSignup := NewUserSignup()
	userSignup.Status = v1alpha1.UserSignupStatus{
		Conditions: []v1alpha1.Condition{
			{
				Type:   v1alpha1.UserSignupComplete,
				Status: v1.ConditionTrue,
			},
			{
				Type:   v1alpha1.UserSignupApproved,
				Status: v1.ConditionTrue,
				Reason: "ApprovedAutomatically",
			},
		},
		CompliantUsername: "foo",
	}
	userSignup.Labels["toolchain.dev.openshift.com/approved"] = "true"

	bannedUser := &toolchainv1alpha1.BannedUser{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				toolchainv1alpha1.BannedUserEmailHashLabelKey: "fd2addbd8d82f0d2dc088fa122377eaa",
			},
		},
		Spec: toolchainv1alpha1.BannedUserSpec{
			Email: "foo@redhat.com",
		},
	}

	mur := murtest.NewMasterUserRecord(t, "foo", murtest.MetaNamespace(test.HostOperatorNs))
	mur.Labels = map[string]string{toolchainv1alpha1.MasterUserRecordUserIDLabelKey: userSignup.Name}

	r, req, _ := prepareReconcile(t, userSignup.Name, NewGetMemberClusters(), userSignup, mur, bannedUser, configMap(configuration.UserApprovalPolicyAutomatic), basicNSTemplateTier)

	// when
	_, err := r.Reconcile(req)

	// then
	require.NoError(t, err)
	key := test.NamespacedName(test.HostOperatorNs, userSignup.Name)
	err = r.client.Get(context.TODO(), key, userSignup)
	require.NoError(t, err)
	assert.Equal(t, "false", userSignup.Labels[v1alpha1.UserSignupApprovedLabelKey])

	// Confirm the status is set to Banning
	test.AssertConditionsMatch(t, userSignup.Status.Conditions,
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupComplete,
			Status: v1.ConditionFalse,
			Reason: "Banning",
		},
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupApproved,
			Status: v1.ConditionTrue,
			Reason: "ApprovedAutomatically",
		})

	murs := &v1alpha1.MasterUserRecordList{}

	// Confirm that the MUR has now been deleted
	err = r.client.List(context.TODO(), murs)
	require.NoError(t, err)
	require.Len(t, murs.Items, 0)
	AssertThatCounterHas(t, 1)

	t.Run("second reconcile", func(t *testing.T) {
		// when
		_, err = r.Reconcile(req)
		require.NoError(t, err)

		err = r.client.Get(context.TODO(), key, userSignup)
		require.NoError(t, err)

		assert.Equal(t, "false", userSignup.Labels[v1alpha1.UserSignupApprovedLabelKey])

		// Confirm the status is now set to Banned
		test.AssertConditionsMatch(t, userSignup.Status.Conditions,
			v1alpha1.Condition{
				Type:   v1alpha1.UserSignupComplete,
				Status: v1.ConditionTrue,
				Reason: "Banned",
			},
			v1alpha1.Condition{
				Type:   v1alpha1.UserSignupApproved,
				Status: v1.ConditionTrue,
				Reason: "ApprovedAutomatically",
			})

		// Confirm that there is still no MUR
		err = r.client.List(context.TODO(), murs)
		require.NoError(t, err)
		require.Len(t, murs.Items, 0)
		AssertThatCounterHas(t, 1)
	})
}

func TestUserSignupListBannedUsersFails(t *testing.T) {
	// given
	InitializeCounter(t, 1)
	defer counter.Reset()
	userSignup := NewUserSignup()

	r, req, clt := prepareReconcile(t, userSignup.Name, NewGetMemberClusters(), userSignup, configMap(configuration.UserApprovalPolicyAutomatic), basicNSTemplateTier)

	clt.MockList = func(ctx context.Context, list runtime.Object, opts ...client.ListOption) error {
		return errors.New("err happened")
	}

	// when
	_, err := r.Reconcile(req)

	// then
	require.Error(t, err)
	AssertThatCounterHas(t, 1)
}

func TestUserSignupDeactivatedButMURDeleteFails(t *testing.T) {
	// given
	InitializeCounter(t, 1)
	defer counter.Reset()
	userSignup := &v1alpha1.UserSignup{
		ObjectMeta: NewUserSignupObjectMeta("", "alice.mayweather.doe@redhat.com"),
		Spec: v1alpha1.UserSignupSpec{
			Username:    "alice.mayweather.doe@redhat.com",
			Deactivated: true,
		},
		Status: v1alpha1.UserSignupStatus{
			Conditions: []v1alpha1.Condition{
				{
					Type:   v1alpha1.UserSignupComplete,
					Status: v1.ConditionTrue,
				},
				{
					Type:   v1alpha1.UserSignupApproved,
					Status: v1.ConditionTrue,
					Reason: "ApprovedAutomatically",
				},
			},
			CompliantUsername: "alice-mayweather",
		},
	}
	userSignup.Labels["toolchain.dev.openshift.com/approved"] = "true"

	key := test.NamespacedName(test.HostOperatorNs, userSignup.Name)

	mur := murtest.NewMasterUserRecord(t, "john-doe", murtest.MetaNamespace(test.HostOperatorNs))
	mur.Labels = map[string]string{toolchainv1alpha1.MasterUserRecordUserIDLabelKey: userSignup.Name}

	r, req, clt := prepareReconcile(t, userSignup.Name, NewGetMemberClusters(), userSignup, mur, configMap(configuration.UserApprovalPolicyAutomatic), basicNSTemplateTier)

	clt.MockDelete = func(ctx context.Context, obj runtime.Object, opts ...client.DeleteOption) error {
		switch obj.(type) {
		case *v1alpha1.MasterUserRecord:
			return errors.New("unable to delete mur")
		default:
			return clt.Delete(ctx, obj)
		}
	}

	t.Run("first reconcile - status should show message about mur deletion failure", func(t *testing.T) {
		// when
		_, err := r.Reconcile(req)
		require.Error(t, err)

		// then

		// Lookup the UserSignup
		err = r.client.Get(context.TODO(), key, userSignup)
		require.NoError(t, err)
		assert.Equal(t, "false", userSignup.Labels[v1alpha1.UserSignupApprovedLabelKey])

		// Confirm the status is set to UnableToDeleteMUR
		test.AssertConditionsMatch(t, userSignup.Status.Conditions,
			v1alpha1.Condition{
				Type:   v1alpha1.UserSignupApproved,
				Status: v1.ConditionTrue,
				Reason: "ApprovedAutomatically",
			},
			v1alpha1.Condition{
				Type:    v1alpha1.UserSignupComplete,
				Status:  v1.ConditionFalse,
				Reason:  "UnableToDeleteMUR",
				Message: "unable to delete mur",
			})
		AssertThatCounterHas(t, 1)
	})

	t.Run("second reconcile - there should not be a notification created since the mur deletion failed even if reconciled again", func(t *testing.T) {
		_, err := r.Reconcile(req)
		require.Error(t, err)
		ntest.AssertNoNotificationsExist(t, r.client)
	})
}

func TestDeathBy100Signups(t *testing.T) {
	// given
	InitializeCounter(t, 100)
	defer counter.Reset()
	userSignup := NewUserSignup(Approved())

	args := make([]runtime.Object, 0)
	args = append(args, userSignup)
	args = append(args, configMap(configuration.UserApprovalPolicyAutomatic))

	args = append(args, &v1alpha1.MasterUserRecord{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: test.HostOperatorNs,
			Labels:    map[string]string{v1alpha1.MasterUserRecordUserIDLabelKey: uuid.NewV4().String()},
		},
	})

	for i := 2; i < 101; i++ {
		args = append(args, &v1alpha1.MasterUserRecord{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("foo-%d", i),
				Namespace: test.HostOperatorNs,
				Labels:    map[string]string{v1alpha1.MasterUserRecordUserIDLabelKey: uuid.NewV4().String()},
			},
		})
	}

	args = append(args, basicNSTemplateTier)

	ready := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue))
	r, req, _ := prepareReconcile(t, userSignup.Name, ready, args...)

	// when
	res, err := r.Reconcile(req)

	// then
	require.Error(t, err)
	assert.EqualError(t, err, "Error generating compliant username for foo@redhat.com: unable to transform username [foo@redhat.com] even after 100 attempts")
	require.Equal(t, reconcile.Result{}, res)

	// Lookup the user signup again
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)
	assert.Equal(t, "true", userSignup.Labels[v1alpha1.UserSignupApprovedLabelKey])

	test.AssertConditionsMatch(t, userSignup.Status.Conditions,
		v1alpha1.Condition{
			Type:    v1alpha1.UserSignupComplete,
			Status:  v1.ConditionFalse,
			Reason:  "UnableToCreateMUR",
			Message: "unable to transform username [foo@redhat.com] even after 100 attempts",
		},
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupApproved,
			Status: v1.ConditionTrue,
			Reason: "ApprovedByAdmin",
		},
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupUserDeactivatedNotificationCreated,
			Status: v1.ConditionFalse,
			Reason: "UserIsActive",
		},
	)
	AssertThatCounterHas(t, 100)
}

func TestUserSignupWithMultipleExistingMURNotOK(t *testing.T) {
	// given
	InitializeCounter(t, 1)
	defer counter.Reset()
	userSignup := NewUserSignup()

	// Create a MUR with the same UserID
	mur := &v1alpha1.MasterUserRecord{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: test.HostOperatorNs,
			Labels:    map[string]string{v1alpha1.MasterUserRecordUserIDLabelKey: userSignup.Name},
		},
	}

	// Create another MUR with the same UserID
	mur2 := &v1alpha1.MasterUserRecord{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bar",
			Namespace: test.HostOperatorNs,
			Labels:    map[string]string{v1alpha1.MasterUserRecordUserIDLabelKey: userSignup.Name},
		},
	}

	ready := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue))
	r, req, _ := prepareReconcile(t, userSignup.Name, ready, userSignup, mur, mur2, configMap(configuration.UserApprovalPolicyAutomatic), basicNSTemplateTier)

	// when
	_, err := r.Reconcile(req)

	// then
	assert.EqualError(t, err, "Multiple MasterUserRecords found: multiple matching MasterUserRecord resources found")

	key := types.NamespacedName{
		Namespace: test.HostOperatorNs,
		Name:      userSignup.Name,
	}
	instance := &v1alpha1.UserSignup{}
	err = r.client.Get(context.TODO(), key, instance)
	require.NoError(t, err)

	test.AssertConditionsMatch(t, instance.Status.Conditions,
		v1alpha1.Condition{
			Type:    v1alpha1.UserSignupComplete,
			Status:  v1.ConditionFalse,
			Reason:  "InvalidMURState",
			Message: "multiple matching MasterUserRecord resources found",
		},
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupUserDeactivatedNotificationCreated,
			Status: v1.ConditionFalse,
			Reason: "UserIsActive",
		},
	)
	AssertThatCounterHas(t, 1)
}

func TestUserSignupNoMembersAvailableFails(t *testing.T) {
	// given
	InitializeCounter(t, 1)
	defer counter.Reset()
	userSignup := NewUserSignup(Approved())

	r, req, _ := prepareReconcile(t, userSignup.Name, NewGetMemberClusters(), userSignup, configMap(configuration.UserApprovalPolicyAutomatic), basicNSTemplateTier)

	// when
	_, err := r.Reconcile(req)

	// then
	assert.EqualError(t, err, "no target clusters available")
	AssertThatCounterHas(t, 1)

	err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)
	assert.Equal(t, "false", userSignup.Labels[v1alpha1.UserSignupApprovedLabelKey])

}

func prepareReconcile(t *testing.T, name string, getMemberClusters cluster.GetMemberClustersFunc, initObjs ...runtime.Object) (*ReconcileUserSignup, reconcile.Request, *test.FakeClient) {
	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)

	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret",
			Namespace: "test-namespace",
		},
		Type: v1.SecretTypeOpaque,
		Data: map[string][]byte{
			"token": []byte("mycooltoken"),
		},
	}

	toolchainStatus := NewToolchainStatus(
		WithMember("member1", WithNodeRoleUsage("worker", 68), WithNodeRoleUsage("master", 65)))

	initObjs = append(initObjs, secret, toolchainStatus)

	fakeClient := test.NewFakeClient(t, initObjs...)

	r := &ReconcileUserSignup{
		statusUpdater: &statusUpdater{
			client: fakeClient,
		},
		scheme:            s,
		getMemberClusters: getMemberClusters,
	}
	return r, newReconcileRequest(name), fakeClient
}

func newReconcileRequest(name string) reconcile.Request {
	return reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      name,
			Namespace: test.HostOperatorNs,
		},
	}
}

func configMap(approvalPolicy string) *v1.ConfigMap {
	// Create a new ConfigMap
	cmValues := make(map[string]string)
	cmValues[configuration.ToolchainConfigMapUserApprovalPolicy] = approvalPolicy
	cm := &v1.ConfigMap{
		Data: cmValues,
	}
	cm.Name = configuration.ToolchainConfigMapName
	cm.ObjectMeta.Namespace = test.HostOperatorNs
	return cm
}

func TestTransformUsername(t *testing.T) {
	assertName(t, "some", "some@email.com")
	assertName(t, "so-me", "so-me@email.com")
	assertName(t, "at-email-com", "@email.com")
	assertName(t, "at-crt", "@")
	assertName(t, "some", "some")
	assertName(t, "so-me", "so-me")
	assertName(t, "so-me", "so-----me")
	assertName(t, "so-me", "so_me")
	assertName(t, "so-me", "so me")
	assertName(t, "so-me", "so me@email.com")
	assertName(t, "so-me", "so.me")
	assertName(t, "so-me", "so?me")
	assertName(t, "so-me", "so:me")
	assertName(t, "so-me", "so:#$%!$%^&me")
	assertName(t, "crt-crt", ":#$%!$%^&")
	assertName(t, "some1", "some1")
	assertName(t, "so1me1", "so1me1")
	assertName(t, "crt-me", "-me")
	assertName(t, "crt-me", "_me")
	assertName(t, "me-crt", "me-")
	assertName(t, "me-crt", "me_")
	assertName(t, "crt-me-crt", "_me_")
	assertName(t, "crt-me-crt", "-me-")
	assertName(t, "crt-12345", "12345")
}

var dnsRegExp = "^[a-z0-9]([-a-z0-9]*[a-z0-9])?$"

func assertName(t *testing.T, expected, username string) {
	assert.Regexp(t, dnsRegExp, transformUsername(username))
	assert.Equal(t, expected, transformUsername(username))
}

// Test the scenario where the existing usersignup CompliantUsername becomes outdated eg. transformUsername func is changed
func TestChangedCompliantUsername(t *testing.T) {
	// starting with a UserSignup that exists and was approved and has the now outdated CompliantUsername
	userSignup := NewUserSignup(Approved(), WithTargetCluster("east"))
	userSignup.Status = v1alpha1.UserSignupStatus{
		Conditions: []toolchainv1alpha1.Condition{
			{
				Type:   toolchainv1alpha1.UserSignupApproved,
				Status: v1.ConditionTrue,
				Reason: toolchainv1alpha1.UserSignupApprovedByAdminReason,
			},
			{
				Status: v1.ConditionTrue,
				Type:   toolchainv1alpha1.UserSignupComplete,
			},
		},
		CompliantUsername: "foo-old",
	}

	// also starting with the old MUR whose name matches the outdated UserSignup CompliantUsername
	oldMur := &v1alpha1.MasterUserRecord{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo-old",
			Namespace: test.HostOperatorNs,
			Labels:    map[string]string{v1alpha1.MasterUserRecordUserIDLabelKey: userSignup.Name},
		},
	}

	// create the initial resources
	r, req, _ := prepareReconcile(t, userSignup.Name, NewGetMemberClusters(), userSignup, oldMur, basicNSTemplateTier)

	// 1st reconcile should effectively be a no op because the MUR name and UserSignup CompliantUsername match and status is all good
	res, err := r.Reconcile(req)
	require.NoError(t, err)
	require.Equal(t, reconcile.Result{}, res)

	// after the 1st reconcile verify that the MUR still exists and its name still matches the initial UserSignup CompliantUsername
	murs := &v1alpha1.MasterUserRecordList{}
	err = r.client.List(context.TODO(), murs, client.InNamespace(test.HostOperatorNs))
	require.NoError(t, err)
	require.Len(t, murs.Items, 1)
	mur := murs.Items[0]
	require.Equal(t, userSignup.Name, mur.Labels[v1alpha1.MasterUserRecordUserIDLabelKey])
	require.Equal(t, mur.Name, "foo-old")
	require.Equal(t, userSignup.Status.CompliantUsername, "foo-old")

	// delete the old MUR to trigger creation of a new MUR using the new username
	err = r.client.Delete(context.TODO(), oldMur)
	require.NoError(t, err)

	// 2nd reconcile should handle the deleted MUR and provision a new one
	res, err = r.Reconcile(req)
	require.NoError(t, err)
	require.Equal(t, reconcile.Result{}, res)

	// verify the new MUR is provisioned
	murs = &v1alpha1.MasterUserRecordList{}
	err = r.client.List(context.TODO(), murs)
	require.NoError(t, err)
	require.Len(t, murs.Items, 1)
	mur = murs.Items[0]

	// the MUR name should match the new CompliantUserName
	assert.Equal(t, "foo", mur.Name)
	require.Equal(t, test.HostOperatorNs, mur.Namespace)
	require.Equal(t, userSignup.Name, mur.Labels[v1alpha1.MasterUserRecordUserIDLabelKey])
	require.Len(t, mur.Spec.UserAccounts, 1)
	assert.Equal(t, "basic", mur.Spec.UserAccounts[0].Spec.NSTemplateSet.TierName)
	require.Len(t, mur.Spec.UserAccounts[0].Spec.NSTemplateSet.Namespaces, 3)

	// lookup the userSignup and check the conditions are updated but the CompliantUsername is still the old one
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)
	require.Equal(t, userSignup.Status.CompliantUsername, "foo-old")

	// 3rd reconcile should update the CompliantUsername on the UserSignup status
	res, err = r.Reconcile(req)
	require.NoError(t, err)
	require.Equal(t, reconcile.Result{}, res)

	// lookup the userSignup one more time and verify that the CompliantUsername was updated using the current transformUsername logic
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)

	// the CompliantUsername and MUR name should now match
	require.Equal(t, userSignup.Status.CompliantUsername, mur.Name)
}

func TestMigrateMur(t *testing.T) {
	// given
	userSignup := NewUserSignup(Approved(), WithTargetCluster("east"))
	mur, err := newMasterUserRecord(basicNSTemplateTier, "foo", test.HostOperatorNs, "east", "foo")
	require.NoError(t, err)

	// set NSLimit and NSTemplateSet to be empty
	mur.Spec.UserAccounts[0].Spec.NSTemplateSet = toolchainv1alpha1.NSTemplateSetSpec{}
	mur.Spec.UserAccounts[0].Spec.NSLimit = ""

	expectedMur := mur.DeepCopy()
	expectedMur.Generation = 1
	expectedMur.ResourceVersion = "1"
	expectedMur.Spec.UserAccounts[0].Spec.NSTemplateSet.TierName = "basic"
	expectedMur.Spec.UserAccounts[0].Spec.NSLimit = "default"
	expectedMur.Spec.UserAccounts[0].Spec.NSTemplateSet.Namespaces = []v1alpha1.NSTemplateSetNamespace{
		{
			TemplateRef: "basic-code-123abc1",
		},
		{
			TemplateRef: "basic-dev-123abc2",
		},
		{
			TemplateRef: "basic-stage-123abc3",
		},
	}
	expectedMur.Spec.UserAccounts[0].Spec.NSTemplateSet.ClusterResources = &toolchainv1alpha1.NSTemplateSetClusterResources{
		TemplateRef: "basic-clusterresources-654321b",
	}

	t.Run("add missing tierName and nsLimit fields", func(t *testing.T) {
		// given
		r, req, _ := prepareReconcile(t, userSignup.Name, NewGetMemberClusters(), userSignup, basicNSTemplateTier, mur)

		// when
		_, err := r.Reconcile(req)
		// then verify that the MUR exists and is complete
		require.NoError(t, err)
		murs := &v1alpha1.MasterUserRecordList{}
		err = r.client.List(context.TODO(), murs)
		require.NoError(t, err)
		require.Len(t, murs.Items, 1)
		assert.Equal(t, *expectedMur, murs.Items[0])
	})
}
