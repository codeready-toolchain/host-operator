package usersignup

import (
	"github.com/codeready-toolchain/host-operator/pkg/config"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	fakeclientset "k8s.io/client-go/kubernetes/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	"k8s.io/client-go/kubernetes/scheme"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestReadAutoApproveConfig(t *testing.T) {
	r, req := prepareReconcile(t, "test")

	res, err := r.Reconcile(req)
	require.NoError(t, err)
	require.Equal(t, reconcile.Result{}, res)

	// Create a new ConfigMap
	cmValues := make(map[string]string)
	cmValues[config.ToolchainConfigMapUserApprovalPolicy] = config.UserApprovalPolicyAutomatic
	cm := &v1.ConfigMap{
		Data: cmValues,
	}
	cm.Name = config.ToolchainConfigMapName

	r.clientset.CoreV1().ConfigMaps(config.GetOperatorNamespace()).Create(cm)

	policy, err := r.ReadAutoApproveConfig()
	require.NoError(t, err)
	require.Equal(t, config.UserApprovalPolicyAutomatic, policy)
}

func prepareReconcile(t *testing.T, username string, initObjs ...runtime.Object) (*ReconcileUserSignup, reconcile.Request) {
	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)
	client := fake.NewFakeClientWithScheme(s, initObjs...)

	r := &ReconcileUserSignup{
		client: client,
		scheme: s,
		clientset: fakeclientset.NewSimpleClientset(),
	}
	return r, newReconcileRequest(username)
}

func newReconcileRequest(name string) reconcile.Request {
	return reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name: name,
			Namespace: config.GetOperatorNamespace(),
		},
	}
}
