package usersignup

import (
	"context"
	"errors"
	"testing"

	"github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	"github.com/codeready-toolchain/host-operator/pkg/config"
	"github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"

	"github.com/operator-framework/operator-sdk/pkg/log/zap"
	"github.com/satori/go.uuid"
	"github.com/stretchr/testify/require"
	"gopkg.in/h2non/gock.v1"
	v1 "k8s.io/api/core/v1"
	errs "k8s.io/apimachinery/pkg/api/errors"
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

func TestReadUserApprovalPolicy(t *testing.T) {
	r, _, _ := prepareReconcile(t, "test", configMap(config.UserApprovalPolicyAutomatic))

	policy, err := r.ReadUserApprovalPolicyConfig(operatorNamespace)
	require.NoError(t, err)
	require.Equal(t, config.UserApprovalPolicyAutomatic, policy)
}

func TestUserSignupWithAutoApproval(t *testing.T) {
	userSignup := &v1alpha1.UserSignup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: operatorNamespace,
			UID:       types.UID(uuid.NewV4().String()),
		},
		Spec: v1alpha1.UserSignupSpec{
			UserID:   "foo",
			Approved: false,
		},
	}

	r, req, _ := prepareReconcile(t, userSignup.Name, userSignup, configMap(config.UserApprovalPolicyAutomatic))

	createMemberCluster(r.client)
	defer clearMemberClusters(r.client)

	res, err := r.Reconcile(req)
	require.NoError(t, err)
	require.Equal(t, reconcile.Result{}, res)

	mur := &v1alpha1.MasterUserRecord{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, mur)
	require.NoError(t, err)

	require.Equal(t, operatorNamespace, mur.Namespace)
	require.Equal(t, userSignup.Name, mur.Name)
	require.Equal(t, userSignup.Spec.UserID, mur.Spec.UserID)
	require.Len(t, mur.Spec.UserAccounts, 1)

	// Lookup the userSignup again
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

func TestUserSignupWithManualApprovalApproved(t *testing.T) {
	userSignup := &v1alpha1.UserSignup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: operatorNamespace,
			UID:       types.UID(uuid.NewV4().String()),
		},
		Spec: v1alpha1.UserSignupSpec{
			UserID:   "foo",
			Approved: true,
		},
	}

	r, req, _ := prepareReconcile(t, userSignup.Name, userSignup, configMap(config.UserApprovalPolicyManual))

	createMemberCluster(r.client)
	defer clearMemberClusters(r.client)

	res, err := r.Reconcile(req)
	require.NoError(t, err)
	require.Equal(t, reconcile.Result{}, res)

	mur := &v1alpha1.MasterUserRecord{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, mur)
	require.NoError(t, err)

	require.Equal(t, operatorNamespace, mur.Namespace)
	require.Equal(t, userSignup.Name, mur.Name)
	require.Equal(t, userSignup.Spec.UserID, mur.Spec.UserID)
	require.Len(t, mur.Spec.UserAccounts, 1)

	// Lookup the userSignup again
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)

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
			UID:       types.UID(uuid.NewV4().String()),
		},
		Spec: v1alpha1.UserSignupSpec{
			UserID:   "foo",
			Approved: true,
		},
	}

	r, req, _ := prepareReconcile(t, userSignup.Name, userSignup)

	createMemberCluster(r.client)
	defer clearMemberClusters(r.client)

	res, err := r.Reconcile(req)
	require.NoError(t, err)
	require.Equal(t, reconcile.Result{}, res)

	mur := &v1alpha1.MasterUserRecord{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, mur)
	require.NoError(t, err)

	require.Equal(t, operatorNamespace, mur.Namespace)
	require.Equal(t, userSignup.Name, mur.Name)
	require.Equal(t, userSignup.Spec.UserID, mur.Spec.UserID)
	require.Len(t, mur.Spec.UserAccounts, 1)

	// Lookup the userSignup again
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)

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
			UID:       types.UID(uuid.NewV4().String()),
		},
		Spec: v1alpha1.UserSignupSpec{
			UserID:   "foo",
			Approved: false,
		},
	}

	r, req, _ := prepareReconcile(t, userSignup.Name, userSignup, configMap(config.UserApprovalPolicyManual))

	createMemberCluster(r.client)
	defer clearMemberClusters(r.client)

	res, err := r.Reconcile(req)
	require.NoError(t, err)
	require.Equal(t, reconcile.Result{}, res)

	mur := &v1alpha1.MasterUserRecord{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, mur)
	require.Error(t, err)
	require.IsType(t, err, &errs.StatusError{})
	require.Equal(t, metav1.StatusReasonNotFound, err.(*errs.StatusError).ErrStatus.Reason)

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

func TestUserSignupWithAutoApprovalClusterSet(t *testing.T) {
	userSignup := &v1alpha1.UserSignup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: operatorNamespace,
			UID:       types.UID(uuid.NewV4().String()),
		},
		Spec: v1alpha1.UserSignupSpec{
			UserID:        "foo",
			Approved:      false,
			TargetCluster: "east",
		},
	}

	r, req, _ := prepareReconcile(t, userSignup.Name, userSignup, configMap(config.UserApprovalPolicyAutomatic))

	createMemberCluster(r.client)
	defer clearMemberClusters(r.client)

	res, err := r.Reconcile(req)
	require.NoError(t, err)
	require.Equal(t, reconcile.Result{}, res)

	mur := &v1alpha1.MasterUserRecord{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, mur)
	require.NoError(t, err)

	require.Equal(t, operatorNamespace, mur.Namespace)
	require.Equal(t, userSignup.Name, mur.Name)
	require.Equal(t, userSignup.Spec.UserID, mur.Spec.UserID)
	require.Len(t, mur.Spec.UserAccounts, 1)

	// Lookup the userSignup again
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

func TestUserSignupMURCreateFails(t *testing.T) {
	userSignup := &v1alpha1.UserSignup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: operatorNamespace,
			UID:       types.UID(uuid.NewV4().String()),
		},
		Spec: v1alpha1.UserSignupSpec{
			UserID:   "foo",
			Approved: true,
		},
	}

	r, req, client := prepareReconcile(t, userSignup.Name, userSignup)

	// Add some member clusters
	createMemberCluster(r.client)
	defer clearMemberClusters(r.client)

	client.MockCreate = func(ctx context.Context, obj runtime.Object) error {
		switch obj.(type) {
		case *v1alpha1.MasterUserRecord:
			return errors.New("unable to create mur")
		default:
			return client.Create(ctx, obj)
		}
	}

	res, err := r.Reconcile(req)
	require.Error(t, err)
	require.Equal(t, reconcile.Result{}, res)
}

func TestUserSignupMURCreateAlreadyExists(t *testing.T) {
	userSignup := &v1alpha1.UserSignup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: operatorNamespace,
			UID:       types.UID(uuid.NewV4().String()),
		},
		Spec: v1alpha1.UserSignupSpec{
			UserID:   "foo",
			Approved: true,
		},
	}

	r, req, fakeClient := prepareReconcile(t, userSignup.Name, userSignup)

	// Add some member clusters
	createMemberCluster(r.client)
	defer clearMemberClusters(r.client)

	fakeClient.MockCreate = func(ctx context.Context, obj runtime.Object) error {
		switch obj.(type) {
		case *v1alpha1.MasterUserRecord:
			return errs.NewAlreadyExists(v1.Resource("masteruserrecords"), obj.(*v1alpha1.MasterUserRecord).Name)
		default:
			return fakeClient.Client.Create(ctx, obj)
		}
	}

	res, err := r.Reconcile(req)
	require.NoError(t, err)
	require.Equal(t, reconcile.Result{}, res)

	err = r.client.Get(context.TODO(), types.NamespacedName{Namespace: operatorNamespace, Name: userSignup.Name}, userSignup)
	require.NoError(t, err)

	test.AssertConditionsMatch(t, userSignup.Status.Conditions,
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupApproved,
			Status: v1.ConditionTrue,
			Reason: "ApprovedByAdmin",
		},
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupComplete,
			Status: v1.ConditionTrue,
			Reason: "",
		})
}

func TestUserSignupMURReadFails(t *testing.T) {
	userSignup := &v1alpha1.UserSignup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: operatorNamespace,
			UID:       types.UID(uuid.NewV4().String()),
		},
		Spec: v1alpha1.UserSignupSpec{
			UserID:   "foo",
			Approved: true,
		},
	}

	r, req, fakeClient := prepareReconcile(t, userSignup.Name, userSignup)

	// Add some member clusters
	createMemberCluster(r.client)
	defer clearMemberClusters(r.client)

	fakeClient.MockGet = func(ctx context.Context, key client.ObjectKey, obj runtime.Object) error {
		switch obj.(type) {
		case *v1alpha1.MasterUserRecord:
			return errors.New("failed to lookup MUR")
		default:
			return fakeClient.Client.Get(ctx, key, obj)
		}
	}

	res, err := r.Reconcile(req)
	require.Error(t, err)
	require.Equal(t, reconcile.Result{}, res)
}

func TestUserSignupSetStatusApprovedByAdminFails(t *testing.T) {
	userSignup := &v1alpha1.UserSignup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: operatorNamespace,
			UID:       types.UID(uuid.NewV4().String()),
		},
		Spec: v1alpha1.UserSignupSpec{
			UserID:   "foo",
			Approved: true,
		},
	}

	r, req, fakeClient := prepareReconcile(t, userSignup.Name, userSignup)

	// Add some member clusters
	createMemberCluster(r.client)
	defer clearMemberClusters(r.client)

	fakeClient.MockStatusUpdate = func(ctx context.Context, obj runtime.Object) error {
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
			UID:       types.UID(uuid.NewV4().String()),
		},
		Spec: v1alpha1.UserSignupSpec{
			UserID: "foo",
		},
	}

	r, req, fakeClient := prepareReconcile(t, userSignup.Name, userSignup, configMap(config.UserApprovalPolicyAutomatic))

	// Add some member clusters
	createMemberCluster(r.client)
	defer clearMemberClusters(r.client)

	fakeClient.MockStatusUpdate = func(ctx context.Context, obj runtime.Object) error {
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
			UID:       types.UID(uuid.NewV4().String()),
		},
		Spec: v1alpha1.UserSignupSpec{
			UserID: "foo",
		},
	}

	r, req, fakeClient := prepareReconcile(t, userSignup.Name, userSignup, configMap(config.UserApprovalPolicyAutomatic))

	fakeClient.MockStatusUpdate = func(ctx context.Context, obj runtime.Object) error {
		switch obj.(type) {
		case *v1alpha1.UserSignup:
			for _, cond := range obj.(*v1alpha1.UserSignup).Status.Conditions {
				if cond.Reason == "NoClustersAvailable" {
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
			Name:      "foo",
			Namespace: operatorNamespace,
			UID:       types.UID(uuid.NewV4().String()),
		},
		Spec: v1alpha1.UserSignupSpec{
			UserID:   "foo",
			Approved: false,
		},
	}

	// Create a MUR with the same name
	mur := &v1alpha1.MasterUserRecord{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: operatorNamespace,
			UID:       types.UID(uuid.NewV4().String()),
		},
		Spec: v1alpha1.MasterUserRecordSpec{
			UserID: "foo",
		},
	}

	r, req, _ := prepareReconcile(t, userSignup.Name, userSignup, mur, configMap(config.UserApprovalPolicyAutomatic))

	createMemberCluster(r.client)
	defer clearMemberClusters(r.client)

	_, err := r.Reconcile(req)
	require.NoError(t, err)

	key := types.NamespacedName{
		Namespace: operatorNamespace,
		Name:      "foo",
	}
	instance := &v1alpha1.UserSignup{}
	err = r.client.Get(context.TODO(), key, instance)
	require.NoError(t, err)

	var cond *v1alpha1.Condition
	for _, condition := range instance.Status.Conditions {
		if condition.Type == v1alpha1.UserSignupComplete {
			cond = &condition
		}
	}

	require.NotNil(t, cond)
	require.Equal(t, v1.ConditionTrue, cond.Status)
}

func TestUserSignupNoMembersAvailableFails(t *testing.T) {
	userSignup := &v1alpha1.UserSignup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: operatorNamespace,
			UID:       types.UID(uuid.NewV4().String()),
		},
		Spec: v1alpha1.UserSignupSpec{
			UserID:   "foo",
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

func createMemberCluster(client client.Client) {
	status := newClusterStatus(common.ClusterReady, v1.ConditionTrue)

	kubeFedCluster := newKubeFedCluster("east", "secret", status, labels(cluster.Member, "", nameMember))

	service := cluster.KubeFedClusterService{Log: logf.Log, Client: client}
	service.AddKubeFedCluster(kubeFedCluster)
}

func clearMemberClusters(client client.Client) {
	service := cluster.KubeFedClusterService{Log: logf.Log, Client: client}
	clusters := cluster.GetMemberClusters()

	for _, cluster := range clusters {
		service.DeleteKubeFedCluster(&v1beta1.KubeFedCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name: cluster.Name,
			},
		})
	}
}

func newClusterStatus(conType common.ClusterConditionType, conStatus v1.ConditionStatus) v1beta1.KubeFedClusterStatus {
	return v1beta1.KubeFedClusterStatus{
		Conditions: []v1beta1.ClusterCondition{{
			Type:   conType,
			Status: conStatus,
		}},
	}
}

func labels(clType cluster.Type, ns, ownerClusterName string) map[string]string {
	labels := map[string]string{}
	if clType != "" {
		labels["type"] = string(clType)
	}
	if ns != "" {
		labels["namespace"] = ns
	}
	labels["ownerClusterName"] = ownerClusterName
	return labels
}

func newKubeFedCluster(name, secName string, status v1beta1.KubeFedClusterStatus, labels map[string]string) *v1beta1.KubeFedCluster {
	logf.SetLogger(zap.Logger())
	gock.New("http://cluster.com").
		Get("api").
		Persist().
		Reply(200).
		BodyString("{}")

	return &v1beta1.KubeFedCluster{
		Spec: v1beta1.KubeFedClusterSpec{
			SecretRef: v1beta1.LocalSecretReference{
				Name: secName,
			},
			APIEndpoint: "http://cluster.com",
			CABundle:    []byte{},
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "test-namespace",
			Labels:    labels,
		},
		Status: status,
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
