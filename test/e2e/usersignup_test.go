package e2e

import (
	"context"
	"github.com/operator-framework/operator-sdk/pkg/test/e2eutil"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"

	"github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/apis"

	framework "github.com/operator-framework/operator-sdk/pkg/test"
)


func TestUserSignup(t *testing.T) {
	userSignupList := &v1alpha1.UserSignupList{}

	err := framework.AddToFrameworkScheme(apis.AddToScheme, userSignupList)
	require.NoError(t, err, "failed to add custom resource scheme to framework")

	ctx := framework.NewTestCtx(t)
	defer ctx.Cleanup()

	err = ctx.InitializeClusterResources(&framework.CleanupOptions{TestContext: ctx, Timeout: cleanupTimeout, RetryInterval: cleanupRetryInterval})
	require.NoError(t, err, "failed to initialize cluster resources")
	t.Log("Initialized cluster resources")

	namespace, err := ctx.GetNamespace()
	require.NoError(t, err, "failed to get namespace where operator needs to run")

	f := framework.Global
	//client := f.Client.Client

	err = e2eutil.WaitForDeployment(t, f.KubeClient, namespace, "host-operator", 1, operatorRetryInterval, operatorTimeout)
	require.NoError(t, err, "failed while waiting for host operator deployment")

	t.Log("host-operator is ready and in a running state")

	// Create user signup
	t.Logf("Creating UserSignup with namespace %s", namespace)
	userSignup := newUserSignup(namespace, "foo")
	err = f.Client.Create(context.TODO(), userSignup, &framework.CleanupOptions{TestContext: ctx, Timeout: cleanupTimeout, RetryInterval: cleanupRetryInterval})
	require.NoError(t, err)
	t.Logf("user signup '%s' created", userSignup.Name)

	// Confirm the UserSignup was created
	err = verifyResources(t, f, namespace, userSignup)
	require.NoError(t, err)

	// Delete the User Signup
}

func newUserSignup(namespace, name string) *v1alpha1.UserSignup {
	userSignup := &v1alpha1.UserSignup{
		ObjectMeta: v1.ObjectMeta{
			Name: name,
			Namespace: namespace,
		},
		Spec: v1alpha1.UserSignupSpec{
			UserID: name,
			Approved: true,
			TargetCluster: "east",
		},
	}

	return userSignup
}

func verifyResources(t *testing.T, f *framework.Framework, namespace string, userSignup *v1alpha1.UserSignup) error {
	if err := waitForUserSignup(t, f.Client.Client, userSignup.Name, namespace); err != nil {
		return err
	}

	if err := waitForMasterUserRecord(t, f.Client.Client, userSignup.Name, namespace); err != nil {
		return err
	}

	return nil
}

func createUserSignup(t *testing.T, f *framework.Framework, ctx *framework.TestCtx, name string) *v1alpha1.UserSignup {
	namespace, err := ctx.GetNamespace()
	require.NoError(t, err)

	userSignup := newUserSignup(namespace, name)
	err = f.Client.Create(context.TODO(), userSignup, &framework.CleanupOptions{TestContext: ctx, Timeout: cleanupTimeout,
		RetryInterval: cleanupRetryInterval })
	require.NoError(t, err)

	err = verifyResources(t, f, namespace, userSignup)
	require.NoError(t, err)

	return userSignup
}