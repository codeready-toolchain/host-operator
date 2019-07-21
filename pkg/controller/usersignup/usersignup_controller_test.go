package usersignup

import (
	"context"
	"github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	"github.com/codeready-toolchain/host-operator/pkg/config"
	"github.com/satori/go.uuid"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	fakeclientset "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"testing"
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

	r.clientset.CoreV1().ConfigMaps(config.GetOperatorNamespace()).Create(cm)

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

	// Create a new ConfigMap and set the user approval policy to automatic
	cmValues := make(map[string]string)
	cmValues[config.ToolchainConfigMapUserApprovalPolicy] = config.UserApprovalPolicyAutomatic
	cm := &v1.ConfigMap{
		Data: cmValues,
	}
	cm.Name = config.ToolchainConfigMapName
	r.clientset.CoreV1().ConfigMaps(config.GetOperatorNamespace()).Create(cm)

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

	// Create a new ConfigMap and set the user approval policy to manual
	cmValues := make(map[string]string)
	cmValues[config.ToolchainConfigMapUserApprovalPolicy] = config.UserApprovalPolicyManual
	cm := &v1.ConfigMap{
		Data: cmValues,
	}
	cm.Name = config.ToolchainConfigMapName
	r.clientset.CoreV1().ConfigMaps(config.GetOperatorNamespace()).Create(cm)

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

	// Create a new ConfigMap and set the user approval policy to manual
	cmValues := make(map[string]string)
	cmValues[config.ToolchainConfigMapUserApprovalPolicy] = config.UserApprovalPolicyManual
	cm := &v1.ConfigMap{
		Data: cmValues,
	}
	cm.Name = config.ToolchainConfigMapName
	r.clientset.CoreV1().ConfigMaps(config.GetOperatorNamespace()).Create(cm)

	res, err := r.Reconcile(req)
	require.NoError(t, err)
	require.Equal(t, reconcile.Result{}, res)

	mur := &v1alpha1.MasterUserRecord{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, mur)
	require.Error(t, err)
	require.IsType(t, err, &errors.StatusError{})
	require.Equal(t, metav1.StatusReasonNotFound, err.(*errors.StatusError).ErrStatus.Reason)
}

func prepareReconcile(t *testing.T, userID string, initObjs ...runtime.Object) (*ReconcileUserSignup, reconcile.Request) {
	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)
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
