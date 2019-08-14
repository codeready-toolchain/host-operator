package usersignup

import (
	"context"
	"github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	"github.com/codeready-toolchain/host-operator/pkg/config"
	"github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	"github.com/operator-framework/operator-sdk/pkg/log/zap"
	"github.com/satori/go.uuid"
	"github.com/stretchr/testify/require"
	"gopkg.in/h2non/gock.v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	fakeclientset "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/kubefed/pkg/apis/core/common"
	"sigs.k8s.io/kubefed/pkg/apis/core/v1beta1"
	"testing"
)

const (
	nameMember = "east"
)

func TestReadUserApprovalPolicy(t *testing.T) {
	r, _ := prepareReconcile(t, "test")

	// Create a new ConfigMap
	cmValues := make(map[string]string)
	cmValues[config.ToolchainConfigMapUserApprovalPolicy] = config.UserApprovalPolicyAutomatic
	cm := &v1.ConfigMap{
		Data: cmValues,
	}
	cm.Name = config.ToolchainConfigMapName

	_, err := r.clientset.CoreV1().ConfigMaps(config.GetOperatorNamespace()).Create(cm)
	require.NoError(t, err)

	policy, err := r.ReadUserApprovalPolicyConfig()
	require.NoError(t, err)
	require.Equal(t, config.UserApprovalPolicyAutomatic, policy)
}

func TestUserSignupWithAutoApproval(t *testing.T) {
	userSignup := &v1alpha1.UserSignup{
		ObjectMeta: metav1.ObjectMeta{
			Name: "foo",
			Namespace: config.GetOperatorNamespace(),
			UID: types.UID(uuid.NewV4().String()),
		},
		Spec: v1alpha1.UserSignupSpec{
			UserID: "foo",
			Approved: false,
		},
	}

	r, req := prepareReconcile(t, userSignup.Spec.UserID, userSignup)

	createMemberCluster(r.client)
	defer clearMemberClusters(r.client)

	// Create a new ConfigMap and set the user approval policy to automatic
	cmValues := make(map[string]string)
	cmValues[config.ToolchainConfigMapUserApprovalPolicy] = config.UserApprovalPolicyAutomatic
	cm := &v1.ConfigMap{
		Data: cmValues,
	}
	cm.Name = config.ToolchainConfigMapName
	_, err := r.clientset.CoreV1().ConfigMaps(config.GetOperatorNamespace()).Create(cm)
	require.NoError(t, err)

	res, err := r.Reconcile(req)
	require.NoError(t, err)
	require.Equal(t, reconcile.Result{}, res)

	mur := &v1alpha1.MasterUserRecord{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, mur)
	require.NoError(t, err)

	require.Equal(t, config.GetOperatorNamespace(), mur.Namespace)
	require.Equal(t, userSignup.Name, mur.Name)
	require.Equal(t, userSignup.Spec.UserID, mur.Spec.UserID)
	require.Len(t, mur.Spec.UserAccounts, 1)
}

func TestUserSignupWithManualApprovalApproved(t *testing.T) {
	userSignup := &v1alpha1.UserSignup{
		ObjectMeta: metav1.ObjectMeta{
			Name: "foo",
			Namespace: config.GetOperatorNamespace(),
			UID: types.UID(uuid.NewV4().String()),
		},
		Spec: v1alpha1.UserSignupSpec{
			UserID: "foo",
			Approved: true,
		},
	}

	r, req := prepareReconcile(t, userSignup.Spec.UserID, userSignup)

	createMemberCluster(r.client)
	defer clearMemberClusters(r.client)

	// Create a new ConfigMap and set the user approval policy to manual
	cmValues := make(map[string]string)
	cmValues[config.ToolchainConfigMapUserApprovalPolicy] = config.UserApprovalPolicyManual
	cm := &v1.ConfigMap{
		Data: cmValues,
	}
	cm.Name = config.ToolchainConfigMapName
	_, err := r.clientset.CoreV1().ConfigMaps(config.GetOperatorNamespace()).Create(cm)
	require.NoError(t, err)

	res, err := r.Reconcile(req)
	require.NoError(t, err)
	require.Equal(t, reconcile.Result{}, res)

	mur := &v1alpha1.MasterUserRecord{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, mur)
	require.NoError(t, err)

	require.Equal(t, config.GetOperatorNamespace(), mur.Namespace)
	require.Equal(t, userSignup.Name, mur.Name)
	require.Equal(t, userSignup.Spec.UserID, mur.Spec.UserID)
	require.Len(t, mur.Spec.UserAccounts, 1)

}

func TestUserSignupWithNoApprovalPolicyTreatedAsManualApproved(t *testing.T) {
	userSignup := &v1alpha1.UserSignup{
		ObjectMeta: metav1.ObjectMeta{
			Name: "foo",
			Namespace: config.GetOperatorNamespace(),
			UID: types.UID(uuid.NewV4().String()),
		},
		Spec: v1alpha1.UserSignupSpec{
			UserID: "foo",
			Approved: true,
		},
	}

	r, req := prepareReconcile(t, userSignup.Spec.UserID, userSignup)

	createMemberCluster(r.client)
	defer clearMemberClusters(r.client)

	res, err := r.Reconcile(req)
	require.NoError(t, err)
	require.Equal(t, reconcile.Result{}, res)

	mur := &v1alpha1.MasterUserRecord{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, mur)
	require.NoError(t, err)

	require.Equal(t, config.GetOperatorNamespace(), mur.Namespace)
	require.Equal(t, userSignup.Name, mur.Name)
	require.Equal(t, userSignup.Spec.UserID, mur.Spec.UserID)
	require.Len(t, mur.Spec.UserAccounts, 1)
}

func TestUserSignupWithManualApprovalNotApproved(t *testing.T) {
	userSignup := &v1alpha1.UserSignup{
		ObjectMeta: metav1.ObjectMeta{
			Name: "foo",
			Namespace: config.GetOperatorNamespace(),
			UID: types.UID(uuid.NewV4().String()),
		},
		Spec: v1alpha1.UserSignupSpec{
			UserID: "foo",
			Approved: false,
		},
	}

	r, req := prepareReconcile(t, userSignup.Spec.UserID, userSignup)

	createMemberCluster(r.client)
	defer clearMemberClusters(r.client)

	// Create a new ConfigMap and set the user approval policy to manual
	cmValues := make(map[string]string)
	cmValues[config.ToolchainConfigMapUserApprovalPolicy] = config.UserApprovalPolicyManual
	cm := &v1.ConfigMap{
		Data: cmValues,
	}
	cm.Name = config.ToolchainConfigMapName
	_, err := r.clientset.CoreV1().ConfigMaps(config.GetOperatorNamespace()).Create(cm)
	require.NoError(t, err)

	res, err := r.Reconcile(req)
	require.NoError(t, err)
	require.Equal(t, reconcile.Result{}, res)

	mur := &v1alpha1.MasterUserRecord{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, mur)
	require.Error(t, err)
	require.IsType(t, err, &errors.StatusError{})
	require.Equal(t, metav1.StatusReasonNotFound, err.(*errors.StatusError).ErrStatus.Reason)
}

func TestUserSignupWithExistingMUROK(t *testing.T) {
	userSignup := &v1alpha1.UserSignup{
		ObjectMeta: metav1.ObjectMeta{
			Name: "foo",
			Namespace: config.GetOperatorNamespace(),
			UID: types.UID(uuid.NewV4().String()),
		},
		Spec: v1alpha1.UserSignupSpec{
			UserID: "foo",
			Approved: false,
		},
	}

	// Create a MUR with the same name
	mur := &v1alpha1.MasterUserRecord{
		ObjectMeta: metav1.ObjectMeta{
			Name: "foo",
			Namespace: config.GetOperatorNamespace(),
			UID: types.UID(uuid.NewV4().String()),
		},
		Spec: v1alpha1.MasterUserRecordSpec{
			UserID: "foo",
		},
	}

	r, req := prepareReconcile(t, userSignup.Spec.UserID, userSignup, mur)

	createMemberCluster(r.client)
	defer clearMemberClusters(r.client)

	// Create a new ConfigMap and set the user approval policy to automatic
	cmValues := make(map[string]string)
	cmValues[config.ToolchainConfigMapUserApprovalPolicy] = config.UserApprovalPolicyAutomatic
	cm := &v1.ConfigMap{
		Data: cmValues,
	}
	cm.Name = config.ToolchainConfigMapName
	_, err := r.clientset.CoreV1().ConfigMaps(config.GetOperatorNamespace()).Create(cm)
	require.NoError(t, err)

	_, err = r.Reconcile(req)
	require.NoError(t, err)

	key := types.NamespacedName{
		Namespace: config.GetOperatorNamespace(),
		Name: "foo",
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
			Name: "foo",
			Namespace: config.GetOperatorNamespace(),
			UID: types.UID(uuid.NewV4().String()),
		},
		Spec: v1alpha1.UserSignupSpec{
			UserID: "foo",
			Approved: true,
		},
	}

	r, req := prepareReconcile(t, userSignup.Spec.UserID, userSignup)

	// Create a new ConfigMap and set the user approval policy to automatic
	cmValues := make(map[string]string)
	cmValues[config.ToolchainConfigMapUserApprovalPolicy] = config.UserApprovalPolicyAutomatic
	cm := &v1.ConfigMap{
		Data: cmValues,
	}
	cm.Name = config.ToolchainConfigMapName
	_, err := r.clientset.CoreV1().ConfigMaps(config.GetOperatorNamespace()).Create(cm)
	require.NoError(t, err)

	_, err = r.Reconcile(req)
	require.Error(t, err)
	require.IsType(t, SignupError{}, err)
}

func prepareReconcile(t *testing.T, userID string, initObjs ...runtime.Object) (*ReconcileUserSignup, reconcile.Request) {
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

	client := fake.NewFakeClientWithScheme(s, initObjs...)

	r := &ReconcileUserSignup{
		client: client,
		scheme: s,
		clientset: fakeclientset.NewSimpleClientset(),
	}
	return r, newReconcileRequest(userID)
}

func newReconcileRequest(name string) reconcile.Request {
	return reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name: name,
			Namespace: config.GetOperatorNamespace(),
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

	for _, cluster := range(clusters) {
		service.DeleteKubeFedCluster(&v1beta1.KubeFedCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cluster.Name,
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

func newKubeFedCluster(name, secName string, status v1beta1.KubeFedClusterStatus, labels map[string]string) (*v1beta1.KubeFedCluster) {
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