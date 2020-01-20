package usersignup

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"testing"

	"github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	"github.com/codeready-toolchain/host-operator/pkg/config"
	"github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"

	templatev1 "github.com/openshift/api/template/v1"
	"github.com/operator-framework/operator-sdk/pkg/log/zap"
	uuid "github.com/satori/go.uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gock "gopkg.in/h2non/gock.v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/kubefed/pkg/apis/core/common"
	"sigs.k8s.io/kubefed/pkg/apis/core/v1beta1"
)

const (
	nameMember        = "east"
	operatorNamespace = "toolchain-host-operator"
)

var basicNSTemplateTier = &toolchainv1alpha1.NSTemplateTier{
	ObjectMeta: metav1.ObjectMeta{
		Namespace: operatorNamespace,
		Name:      "basic",
	},
	Spec: toolchainv1alpha1.NSTemplateTierSpec{
		Namespaces: []toolchainv1alpha1.NSTemplateTierNamespace{
			{
				Type:     "code",
				Revision: "123456a",
				Template: templatev1.Template{
					// does not need to be filled
				},
			},
			{
				Type:     "dev",
				Revision: "123456b",
				Template: templatev1.Template{
					// does not need to be filled
				},
			},
			{
				Type:     "stage",
				Revision: "123456c",
				Template: templatev1.Template{
					// does not need to be filled
				},
			},
		},
	},
}

func TestReadUserApprovalPolicy(t *testing.T) {
	r, _, _ := prepareReconcile(t, "test", configMap(config.UserApprovalPolicyAutomatic))

	policy, err := r.ReadUserApprovalPolicyConfig(operatorNamespace)
	require.NoError(t, err)
	require.Equal(t, config.UserApprovalPolicyAutomatic, policy)
}

func TestUserSignupWithAutoApprovalWithoutTargetCluster(t *testing.T) {
	userID := uuid.NewV4()
	userSignup := &v1alpha1.UserSignup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      userID.String(),
			Namespace: operatorNamespace,
		},
		Spec: v1alpha1.UserSignupSpec{
			Username: "foo@redhat.com",
			Approved: false,
		},
	}

	r, req, _ := prepareReconcile(t, userSignup.Name, userSignup, configMap(config.UserApprovalPolicyAutomatic), basicNSTemplateTier)

	createMemberCluster(r.client, "member1", ready)
	defer clearMemberClusters(r.client)

	// The first reconcile creates the MasterUserRecord
	res, err := r.Reconcile(req)
	require.NoError(t, err)
	require.Equal(t, reconcile.Result{}, res)

	// Lookup the user signup again
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)

	murs := &v1alpha1.MasterUserRecordList{}
	err = r.client.List(context.TODO(), murs)
	require.NoError(t, err)
	require.Len(t, murs.Items, 1)

	mur := murs.Items[0]
	require.Equal(t, operatorNamespace, mur.Namespace)
	require.Equal(t, userSignup.Name, mur.Labels[v1alpha1.MasterUserRecordUserIDLabelKey])
	require.Len(t, mur.Spec.UserAccounts, 1)
	assert.Equal(t, "basic", mur.Spec.UserAccounts[0].Spec.NSTemplateSet.TierName)
	require.Len(t, mur.Spec.UserAccounts[0].Spec.NSTemplateSet.Namespaces, 3)
	assert.Contains(t, mur.Spec.UserAccounts[0].Spec.NSTemplateSet.Namespaces,
		toolchainv1alpha1.NSTemplateSetNamespace{
			Type:     "code",
			Revision: "123456a",
			Template: "",
		})
	assert.Contains(t, mur.Spec.UserAccounts[0].Spec.NSTemplateSet.Namespaces,
		toolchainv1alpha1.NSTemplateSetNamespace{
			Type:     "dev",
			Revision: "123456b",
			Template: "",
		})
	assert.Contains(t, mur.Spec.UserAccounts[0].Spec.NSTemplateSet.Namespaces,
		toolchainv1alpha1.NSTemplateSetNamespace{
			Type:     "stage",
			Revision: "123456c",
			Template: "",
		})

	// Lookup the user signup again
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)

	test.AssertConditionsMatch(t, userSignup.Status.Conditions,
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupApproved,
			Status: v1.ConditionTrue,
			Reason: "ApprovedAutomatically",
		})

	// Reconcile again
	res, err = r.Reconcile(req)
	require.NoError(t, err)
	require.Equal(t, reconcile.Result{}, res)

	// Lookup the userSignup one more and check the conditions are updated
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)
	require.Equal(t, userSignup.Status.CompliantUsername, mur.Name)

	test.AssertConditionsMatch(t, userSignup.Status.Conditions,
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupApproved,
			Status: v1.ConditionTrue,
			Reason: "ApprovedAutomatically",
		},
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupComplete,
			Status: v1.ConditionTrue,
		})
}

func TestUserSignupFailedMissingNSTemplateTier(t *testing.T) {
	// given
	userSignup := &v1alpha1.UserSignup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: operatorNamespace,
		},
		Spec: v1alpha1.UserSignupSpec{
			Username: "foo@redhat.com",
			Approved: false,
		},
	}
	r, req, _ := prepareReconcile(t, userSignup.Name, userSignup, configMap(config.UserApprovalPolicyAutomatic)) // basicNSTemplateTier does not exist
	createMemberCluster(r.client, "member1", ready)
	defer clearMemberClusters(r.client)
	// when
	res, err := r.Reconcile(req)
	// then
	// error reported, and request is requeued and userSignup status was updated
	require.Error(t, err)
	assert.Equal(t, reconcile.Result{Requeue: true}, res)
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
		})
}

func TestUserSignupFailedNoClusterReady(t *testing.T) {
	// given
	userSignup := &v1alpha1.UserSignup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: operatorNamespace,
		},
		Spec: v1alpha1.UserSignupSpec{
			Username: "foo@redhat.com",
			Approved: false,
		},
	}
	r, req, _ := prepareReconcile(t, userSignup.Name, userSignup, configMap(config.UserApprovalPolicyAutomatic))
	createMemberCluster(r.client, "member1", notReady)
	defer clearMemberClusters(r.client)
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
		})
}

func TestUserSignupFailedNoClusterWithCapacityAvailable(t *testing.T) {
	// given
	userSignup := &v1alpha1.UserSignup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: operatorNamespace,
		},
		Spec: v1alpha1.UserSignupSpec{
			Username: "foo@redhat.com",
			Approved: false,
		},
	}
	r, req, _ := prepareReconcile(t, userSignup.Name, userSignup, configMap(config.UserApprovalPolicyAutomatic))
	createMemberCluster(r.client, "member1", ready, capacityExhausted)
	defer clearMemberClusters(r.client)
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
		})
}

func TestUserSignupWithManualApprovalApproved(t *testing.T) {
	userSignup := &v1alpha1.UserSignup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: operatorNamespace,
		},
		Spec: v1alpha1.UserSignupSpec{
			Username: "foo@redhat.com",
			Approved: true,
		},
	}

	r, req, _ := prepareReconcile(t, userSignup.Name, userSignup, configMap(config.UserApprovalPolicyManual), basicNSTemplateTier)

	createMemberCluster(r.client, "member1", ready)
	defer clearMemberClusters(r.client)

	res, err := r.Reconcile(req)
	require.NoError(t, err)
	require.Equal(t, reconcile.Result{}, res)

	// Lookup the userSignup again
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)

	murs := &v1alpha1.MasterUserRecordList{}
	err = r.client.List(context.TODO(), murs)
	require.NoError(t, err)
	require.Len(t, murs.Items, 1)

	mur := murs.Items[0]

	require.Equal(t, operatorNamespace, mur.Namespace)
	require.Equal(t, userSignup.Name, mur.Labels[v1alpha1.MasterUserRecordUserIDLabelKey])
	require.Len(t, mur.Spec.UserAccounts, 1)

	test.AssertConditionsMatch(t, userSignup.Status.Conditions,
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupApproved,
			Status: v1.ConditionTrue,
			Reason: "ApprovedByAdmin",
		})

	// Reconcile again
	res, err = r.Reconcile(req)
	require.NoError(t, err)
	require.Equal(t, reconcile.Result{}, res)

	// Lookup the userSignup one more time and check the conditions are updated
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)
	require.Equal(t, userSignup.Status.CompliantUsername, mur.Name)

	test.AssertConditionsMatch(t, userSignup.Status.Conditions,
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupApproved,
			Status: v1.ConditionTrue,
			Reason: "ApprovedByAdmin",
		},
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupComplete,
			Status: v1.ConditionTrue,
		})

}

func TestUserSignupWithNoApprovalPolicyTreatedAsManualApproved(t *testing.T) {
	userSignup := &v1alpha1.UserSignup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: operatorNamespace,
		},
		Spec: v1alpha1.UserSignupSpec{
			Username: "foo@redhat.com",
			Approved: true,
		},
	}

	r, req, _ := prepareReconcile(t, userSignup.Name, userSignup, basicNSTemplateTier)

	createMemberCluster(r.client, "member1", ready)
	defer clearMemberClusters(r.client)

	res, err := r.Reconcile(req)
	require.NoError(t, err)
	require.Equal(t, reconcile.Result{}, res)

	// Lookup the userSignup again
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)

	murs := &v1alpha1.MasterUserRecordList{}
	err = r.client.List(context.TODO(), murs)
	require.NoError(t, err)
	require.Len(t, murs.Items, 1)

	mur := murs.Items[0]

	require.Equal(t, operatorNamespace, mur.Namespace)
	require.Equal(t, userSignup.Name, mur.Labels[v1alpha1.MasterUserRecordUserIDLabelKey])
	require.Len(t, mur.Spec.UserAccounts, 1)

	test.AssertConditionsMatch(t, userSignup.Status.Conditions,
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupApproved,
			Status: v1.ConditionTrue,
			Reason: "ApprovedByAdmin",
		})

	// Reconcile again
	res, err = r.Reconcile(req)
	require.NoError(t, err)
	require.Equal(t, reconcile.Result{}, res)

	// Lookup the userSignup one more and check the conditions are updated
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)
	require.Equal(t, userSignup.Status.CompliantUsername, mur.Name)

	test.AssertConditionsMatch(t, userSignup.Status.Conditions,
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupApproved,
			Status: v1.ConditionTrue,
			Reason: "ApprovedByAdmin",
		},
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupComplete,
			Status: v1.ConditionTrue,
		})
}

func TestUserSignupWithManualApprovalNotApproved(t *testing.T) {
	userSignup := &v1alpha1.UserSignup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: operatorNamespace,
		},
		Spec: v1alpha1.UserSignupSpec{
			Username: "foo@redhat.com",
			Approved: false,
		},
	}

	r, req, _ := prepareReconcile(t, userSignup.Name, userSignup, configMap(config.UserApprovalPolicyManual))

	createMemberCluster(r.client, "member1", ready)
	defer clearMemberClusters(r.client)

	res, err := r.Reconcile(req)
	require.NoError(t, err)
	require.Equal(t, reconcile.Result{}, res)

	// Lookup the userSignup again
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)

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
		})
}

func TestUserSignupWithAutoApprovalWithTargetCluster(t *testing.T) {
	userSignup := &v1alpha1.UserSignup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: operatorNamespace,
		},
		Spec: v1alpha1.UserSignupSpec{
			Username:      "foo@redhat.com",
			Approved:      false,
			TargetCluster: "east",
		},
	}

	r, req, _ := prepareReconcile(t, userSignup.Name, userSignup, configMap(config.UserApprovalPolicyAutomatic), basicNSTemplateTier)

	createMemberCluster(r.client, "member1", ready)
	defer clearMemberClusters(r.client)

	res, err := r.Reconcile(req)
	require.NoError(t, err)
	require.Equal(t, reconcile.Result{}, res)

	// Lookup the userSignup again
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)

	murs := &v1alpha1.MasterUserRecordList{}
	err = r.client.List(context.TODO(), murs)
	require.NoError(t, err)
	require.Len(t, murs.Items, 1)

	mur := murs.Items[0]
	require.Equal(t, operatorNamespace, mur.Namespace)
	require.Equal(t, userSignup.Name, mur.Labels[v1alpha1.MasterUserRecordUserIDLabelKey])
	require.Len(t, mur.Spec.UserAccounts, 1)
	assert.Equal(t, "basic", mur.Spec.UserAccounts[0].Spec.NSTemplateSet.TierName)
	require.Len(t, mur.Spec.UserAccounts[0].Spec.NSTemplateSet.Namespaces, 3)

	test.AssertConditionsMatch(t, userSignup.Status.Conditions,
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupApproved,
			Status: v1.ConditionTrue,
			Reason: "ApprovedAutomatically",
		})

	// Reconcile again
	res, err = r.Reconcile(req)
	require.NoError(t, err)
	require.Equal(t, reconcile.Result{}, res)

	// Lookup the userSignup one more and check the conditions are updated
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)
	require.Equal(t, userSignup.Status.CompliantUsername, mur.Name)

	test.AssertConditionsMatch(t, userSignup.Status.Conditions,
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupApproved,
			Status: v1.ConditionTrue,
			Reason: "ApprovedAutomatically",
		},
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupComplete,
			Status: v1.ConditionTrue,
		})
}

func TestUserSignupWithMissingApprovalPolicyTreatedAsManual(t *testing.T) {
	userSignup := &v1alpha1.UserSignup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bar",
			Namespace: operatorNamespace,
		},
		Spec: v1alpha1.UserSignupSpec{
			Username:      "bar@redhat.com",
			Approved:      false,
			TargetCluster: "east",
		},
	}

	r, req, _ := prepareReconcile(t, userSignup.Name, userSignup, emptyConfigMap())

	res, err := r.Reconcile(req)
	require.NoError(t, err)
	require.Equal(t, reconcile.Result{}, res)

	// Lookup the userSignup again
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)

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
		})
}

func TestUserSignupMURCreateFails(t *testing.T) {
	userSignup := &v1alpha1.UserSignup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: operatorNamespace,
		},
		Spec: v1alpha1.UserSignupSpec{
			Username: "foo@redhat.com",
			Approved: true,
		},
	}

	r, req, clt := prepareReconcile(t, userSignup.Name, userSignup, basicNSTemplateTier)

	// Add some member clusters
	createMemberCluster(r.client, "member1", ready)
	defer clearMemberClusters(r.client)

	clt.MockCreate = func(ctx context.Context, obj runtime.Object, opts ...client.CreateOption) error {
		switch obj.(type) {
		case *v1alpha1.MasterUserRecord:
			return errors.New("unable to create mur")
		default:
			return clt.Create(ctx, obj)
		}
	}

	res, err := r.Reconcile(req)
	require.Error(t, err)
	require.Equal(t, reconcile.Result{}, res)
}

func TestUserSignupMURReadFails(t *testing.T) {
	userSignup := &v1alpha1.UserSignup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: operatorNamespace,
		},
		Spec: v1alpha1.UserSignupSpec{
			Username: "foo@redhat.com",
			Approved: true,
		},
	}

	r, req, fakeClient := prepareReconcile(t, userSignup.Name, userSignup)

	// Add some member clusters
	createMemberCluster(r.client, "member1", ready)
	defer clearMemberClusters(r.client)

	fakeClient.MockGet = func(ctx context.Context, key client.ObjectKey, obj runtime.Object) error {
		switch obj.(type) {
		case *v1alpha1.MasterUserRecord:
			return errors.New("failed to lookup MUR")
		default:
			return fakeClient.Client.Get(ctx, key, obj)
		}
	}

	_, err := r.Reconcile(req)
	require.Error(t, err)
}

func TestUserSignupSetStatusApprovedByAdminFails(t *testing.T) {
	userSignup := &v1alpha1.UserSignup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: operatorNamespace,
		},
		Spec: v1alpha1.UserSignupSpec{
			Username: "foo@redhat.com",
			Approved: true,
		},
	}

	r, req, fakeClient := prepareReconcile(t, userSignup.Name, userSignup)

	// Add some member clusters
	createMemberCluster(r.client, "member1", ready)
	defer clearMemberClusters(r.client)

	fakeClient.MockStatusUpdate = func(ctx context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
		switch obj.(type) {
		case *v1alpha1.UserSignup:
			return errors.New("failed to update UserSignup status")
		default:
			return fakeClient.Client.Update(ctx, obj)
		}
	}

	res, err := r.Reconcile(req)
	require.Error(t, err)
	require.Equal(t, reconcile.Result{}, res)
}

func TestUserSignupSetStatusApprovedAutomaticallyFails(t *testing.T) {
	userSignup := &v1alpha1.UserSignup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: operatorNamespace,
		},
		Spec: v1alpha1.UserSignupSpec{
			Username: "foo@redhat.com",
		},
	}

	r, req, fakeClient := prepareReconcile(t, userSignup.Name, userSignup, configMap(config.UserApprovalPolicyAutomatic))

	// Add some member clusters
	createMemberCluster(r.client, "member1", ready)
	defer clearMemberClusters(r.client)

	fakeClient.MockStatusUpdate = func(ctx context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
		switch obj.(type) {
		case *v1alpha1.UserSignup:
			return errors.New("failed to update UserSignup status")
		default:
			return fakeClient.Client.Update(ctx, obj)
		}
	}

	res, err := r.Reconcile(req)
	require.Error(t, err)
	require.Equal(t, reconcile.Result{}, res)
}

func TestUserSignupSetStatusNoClustersAvailableFails(t *testing.T) {
	userSignup := &v1alpha1.UserSignup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: operatorNamespace,
		},
		Spec: v1alpha1.UserSignupSpec{
			Username: "foo@redhat.com",
		},
	}

	r, req, fakeClient := prepareReconcile(t, userSignup.Name, userSignup, configMap(config.UserApprovalPolicyAutomatic))

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

	res, err := r.Reconcile(req)
	require.Error(t, err)
	require.Equal(t, reconcile.Result{}, res)
}

func TestUserSignupWithExistingMUROK(t *testing.T) {
	userSignup := &v1alpha1.UserSignup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      uuid.NewV4().String(),
			Namespace: operatorNamespace,
		},
		Spec: v1alpha1.UserSignupSpec{
			Username: "foo@redhat.com",
			Approved: false,
		},
	}

	// Create a MUR with the same UserID
	mur := &v1alpha1.MasterUserRecord{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo-at-redhat-com-1",
			Namespace: operatorNamespace,
			Labels:    map[string]string{v1alpha1.MasterUserRecordUserIDLabelKey: userSignup.Name},
		},
	}

	r, req, _ := prepareReconcile(t, userSignup.Name, userSignup, mur, configMap(config.UserApprovalPolicyAutomatic), basicNSTemplateTier)

	createMemberCluster(r.client, "member1", ready)
	defer clearMemberClusters(r.client)

	_, err := r.Reconcile(req)
	require.NoError(t, err)

	key := types.NamespacedName{
		Namespace: operatorNamespace,
		Name:      userSignup.Name,
	}
	instance := &v1alpha1.UserSignup{}
	err = r.client.Get(context.TODO(), key, instance)
	require.NoError(t, err)

	require.Equal(t, mur.Name, instance.Status.CompliantUsername)
	test.AssertContainsCondition(t, instance.Status.Conditions, v1alpha1.Condition{
		Type:   v1alpha1.UserSignupComplete,
		Status: v1.ConditionTrue,
	})
}

func TestUserSignupWithExistingMURDifferentUserIDOK(t *testing.T) {
	userSignup := &v1alpha1.UserSignup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      uuid.NewV4().String(),
			Namespace: operatorNamespace,
		},
		Spec: v1alpha1.UserSignupSpec{
			Username: "foo@redhat.com",
			Approved: true,
		},
	}

	// Create a MUR with a different UserID
	mur := &v1alpha1.MasterUserRecord{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo-at-redhat-com",
			Namespace: operatorNamespace,
			Labels:    map[string]string{v1alpha1.MasterUserRecordUserIDLabelKey: uuid.NewV4().String()},
		},
	}

	r, req, _ := prepareReconcile(t, userSignup.Name, userSignup, mur, configMap(config.UserApprovalPolicyAutomatic), basicNSTemplateTier)

	createMemberCluster(r.client, "member1", ready)
	defer clearMemberClusters(r.client)

	// First reconcile loop
	_, err := r.Reconcile(req)
	require.NoError(t, err)

	// We should now have 2 MURs
	murs := &v1alpha1.MasterUserRecordList{}
	err = r.client.List(context.TODO(), murs)
	require.NoError(t, err)
	require.Len(t, murs.Items, 2)

	// Second reconcile loop
	_, err = r.Reconcile(req)
	require.NoError(t, err)

	key := types.NamespacedName{
		Namespace: operatorNamespace,
		Name:      userSignup.Name,
	}
	instance := &v1alpha1.UserSignup{}
	err = r.client.Get(context.TODO(), key, instance)
	require.NoError(t, err)

	require.Equal(t, "foo-at-redhat-com-1", instance.Status.CompliantUsername)

	// Confirm that the mur exists
	mur = &v1alpha1.MasterUserRecord{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Namespace: operatorNamespace, Name: instance.Status.CompliantUsername}, mur)
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
}

func TestUserSignupWithInvalidNameNotOK(t *testing.T) {
	userSignup := &v1alpha1.UserSignup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      uuid.NewV4().String(),
			Namespace: operatorNamespace,
		},
		Spec: v1alpha1.UserSignupSpec{
			Username: "foo#bar@redhat.com",
			Approved: false,
		},
	}

	r, req, _ := prepareReconcile(t, userSignup.Name, userSignup, configMap(config.UserApprovalPolicyAutomatic), basicNSTemplateTier)

	createMemberCluster(r.client, "member1", ready)
	defer clearMemberClusters(r.client)

	_, err := r.Reconcile(req)
	assert.EqualError(t, err, "Error generating compliant username for foo#bar@redhat.com: transformed username [foo#bar-at-redhat-com] is invalid")

	key := types.NamespacedName{
		Namespace: operatorNamespace,
		Name:      userSignup.Name,
	}
	instance := &v1alpha1.UserSignup{}
	err = r.client.Get(context.TODO(), key, instance)
	require.NoError(t, err)

	test.AssertConditionsMatch(t, instance.Status.Conditions,
		v1alpha1.Condition{
			Type:    v1alpha1.UserSignupComplete,
			Status:  v1.ConditionFalse,
			Reason:  "UnableToCreateMUR",
			Message: "transformed username [foo#bar-at-redhat-com] is invalid",
		},
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupApproved,
			Status: v1.ConditionTrue,
			Reason: "ApprovedAutomatically",
		},
	)
}

func TestDeathBy100Signups(t *testing.T) {
	userID := uuid.NewV4().String()

	userSignup := &v1alpha1.UserSignup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      userID,
			Namespace: operatorNamespace,
		},
		Spec: v1alpha1.UserSignupSpec{
			Username: "foo@redhat.com",
			Approved: true,
		},
	}

	args := make([]runtime.Object, 0)
	args = append(args, userSignup)
	args = append(args, configMap(config.UserApprovalPolicyAutomatic))

	args = append(args, &v1alpha1.MasterUserRecord{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo-at-redhat-com",
			Namespace: operatorNamespace,
			Labels:    map[string]string{v1alpha1.MasterUserRecordUserIDLabelKey: uuid.NewV4().String()},
		},
	})

	for i := 1; i < 101; i++ {
		args = append(args, &v1alpha1.MasterUserRecord{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("foo-at-redhat-com-%d", i),
				Namespace: operatorNamespace,
				Labels:    map[string]string{v1alpha1.MasterUserRecordUserIDLabelKey: uuid.NewV4().String()},
			},
		})
	}

	args = append(args, basicNSTemplateTier)

	r, req, _ := prepareReconcile(t, userSignup.Name, args...)

	createMemberCluster(r.client, "member1", ready)
	defer clearMemberClusters(r.client)

	res, err := r.Reconcile(req)
	require.Error(t, err)
	assert.EqualError(t, err, "Error generating compliant username for foo@redhat.com: unable to transform username [foo@redhat.com] even after 100 attempts")
	require.Equal(t, reconcile.Result{}, res)

	// Lookup the user signup again
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)

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
	)
}

func TestUserSignupWithMultipleExistingMURNotOK(t *testing.T) {
	userSignup := &v1alpha1.UserSignup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      uuid.NewV4().String(),
			Namespace: operatorNamespace,
		},
		Spec: v1alpha1.UserSignupSpec{
			Username: "foo@redhat.com",
			Approved: false,
		},
	}

	// Create a MUR with the same UserID
	mur := &v1alpha1.MasterUserRecord{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo-at-redhat-com",
			Namespace: operatorNamespace,
			Labels:    map[string]string{v1alpha1.MasterUserRecordUserIDLabelKey: userSignup.Name},
		},
	}

	// Create another MUR with the same UserID
	mur2 := &v1alpha1.MasterUserRecord{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bar-at-redhat-com",
			Namespace: operatorNamespace,
			Labels:    map[string]string{v1alpha1.MasterUserRecordUserIDLabelKey: userSignup.Name},
		},
	}

	r, req, _ := prepareReconcile(t, userSignup.Name, userSignup, mur, mur2, configMap(config.UserApprovalPolicyAutomatic))

	createMemberCluster(r.client, "member1", ready)
	defer clearMemberClusters(r.client)

	_, err := r.Reconcile(req)
	assert.EqualError(t, err, "Multiple MasterUserRecords found: multiple matching MasterUserRecord resources found")

	key := types.NamespacedName{
		Namespace: operatorNamespace,
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
	)
}

func TestUserSignupNoMembersAvailableFails(t *testing.T) {
	userSignup := &v1alpha1.UserSignup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: operatorNamespace,
		},
		Spec: v1alpha1.UserSignupSpec{
			Username: "foo@redhat.com",

			Approved: true,
		},
	}

	r, req, _ := prepareReconcile(t, userSignup.Name, userSignup, configMap(config.UserApprovalPolicyAutomatic))

	_, err := r.Reconcile(req)
	require.Error(t, err)
	require.IsType(t, SignupError{}, err)
}

func prepareReconcile(t *testing.T, name string, initObjs ...runtime.Object) (*ReconcileUserSignup, reconcile.Request, *test.FakeClient) {
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

	initObjs = append(initObjs, secret)

	client := test.NewFakeClient(t, initObjs...)

	r := &ReconcileUserSignup{
		client: client,
		scheme: s,
	}
	return r, newReconcileRequest(name), client
}

func newReconcileRequest(name string) reconcile.Request {
	return reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      name,
			Namespace: operatorNamespace,
		},
	}
}

// clusterOption an option to configure the cluster to use in the tests
type clusterOption func(*v1beta1.KubeFedCluster)

// ready an option to state the cluster as "ready"
var ready clusterOption = func(c *v1beta1.KubeFedCluster) {
	c.Status = v1beta1.KubeFedClusterStatus{
		Conditions: []v1beta1.ClusterCondition{
			{
				Type:   common.ClusterReady,
				Status: v1.ConditionTrue,
			},
		},
	}
}

// notReady an option to state the cluster as "not ready"
var notReady clusterOption = func(c *v1beta1.KubeFedCluster) {
	c.Status = v1beta1.KubeFedClusterStatus{
		Conditions: []v1beta1.ClusterCondition{
			{
				Type:   common.ClusterReady,
				Status: v1.ConditionFalse,
			},
		},
	}
}

// capacityExhausted an option to state that the cluster capacity has exhausted
var capacityExhausted clusterOption = func(c *v1beta1.KubeFedCluster) {
	c.Labels["toolchain.dev.openshift.com/capacity-exhausted"] = strconv.FormatBool(true)
}

func createMemberCluster(client client.Client, name string, options ...clusterOption) {
	logf.SetLogger(zap.Logger())
	gock.New("http://cluster.com").
		Get("api").
		Persist().
		Reply(200).
		BodyString("{}")
	kubeFedCluster := &v1beta1.KubeFedCluster{
		Spec: v1beta1.KubeFedClusterSpec{
			SecretRef: v1beta1.LocalSecretReference{
				Name: "secret",
			},
			APIEndpoint: "http://cluster.com",
			CABundle:    []byte{},
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "test-namespace",
			Labels: map[string]string{
				"type":             "member",
				"ownerClusterName": "east",
			},
		},
	}
	for _, configure := range options {
		configure(kubeFedCluster)
	}
	service := cluster.NewKubeFedClusterService(client, logf.Log, operatorNamespace)
	service.AddKubeFedCluster(kubeFedCluster)
}

func clearMemberClusters(client client.Client) {
	service := cluster.NewKubeFedClusterService(client, logf.Log, operatorNamespace)
	clusters := cluster.GetMemberClusters()

	for _, cluster := range clusters {
		service.DeleteKubeFedCluster(&v1beta1.KubeFedCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name: cluster.Name,
			},
		})
	}
}

func configMap(approvalPolicy string) *v1.ConfigMap {
	// Create a new ConfigMap
	cmValues := make(map[string]string)
	cmValues[config.ToolchainConfigMapUserApprovalPolicy] = approvalPolicy
	cm := &v1.ConfigMap{
		Data: cmValues,
	}
	cm.Name = config.ToolchainConfigMapName
	cm.ObjectMeta.Namespace = operatorNamespace
	return cm
}

func emptyConfigMap() *v1.ConfigMap {
	// Create a new ConfigMap
	cmValues := make(map[string]string)
	cm := &v1.ConfigMap{
		Data: cmValues,
	}
	cm.Name = config.ToolchainConfigMapName
	cm.ObjectMeta.Namespace = operatorNamespace
	return cm
}
