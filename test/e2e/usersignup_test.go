package e2e

import (
	"context"
	"github.com/codeready-toolchain/host-operator/pkg/config"
	"github.com/operator-framework/operator-sdk/pkg/test/e2eutil"
	uuid "github.com/satori/go.uuid"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"testing"

	"github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/apis"

	framework "github.com/operator-framework/operator-sdk/pkg/test"
)


func TestUserSignupCreated(t *testing.T) {
	// Test Setup
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
	err = waitForUserSignup(t, f.Client.Client, userSignup.Name, namespace)
	require.NoError(t, err)

	// Confirm that a MasterUserRecord wasn't created
	err = waitForMasterUserRecord(t, f.Client.Client, userSignup.Name, namespace)
	require.Error(t, err)

	// Delete the User Signup
	err = f.Client.Delete(context.TODO(), userSignup)
	require.NoError(t, err)
}

func TestUserSignupWithNoApprovalConfig(t *testing.T) {
	// Test Setup
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

	err = e2eutil.WaitForDeployment(t, f.KubeClient, namespace, "host-operator", 1, operatorRetryInterval, operatorTimeout)
	require.NoError(t, err, "failed while waiting for host operator deployment")

	t.Log("host-operator is ready and in a running state")

	// Clear the user approval policy
	err = clearApprovalPolicyConfig(f.KubeClient, namespace)
	require.NoError(t, err)

	// Create user signup - no approval set
	t.Logf("Creating UserSignup with namespace %s", namespace)
	userSignup := newUserSignup(namespace, "francis")
	err = f.Client.Create(context.TODO(), userSignup, &framework.CleanupOptions{TestContext: ctx, Timeout: cleanupTimeout, RetryInterval: cleanupRetryInterval})
	require.NoError(t, err)
	t.Logf("user signup '%s' created", userSignup.Name)

	// Confirm the UserSignup was created
	err = waitForUserSignup(t, f.Client.Client, userSignup.Name, namespace)
	require.NoError(t, err)

	// Confirm that:
	// 1) the Approved condition is set to false
	// 2) the Approved reason is set to PendingApproval
	// 3) the Complete condition is set to false
	// 4) the Complete reason is set to PendingApproval
	err = f.Client.Get(context.TODO(), types.NamespacedName{Namespace: userSignup.Namespace, Name: userSignup.Name}, userSignup)
	require.NoError(t, err)

	checkUserSignupConditions(t, userSignup.Status.Conditions, []v1alpha1.Condition{
		{
			Type: v1alpha1.UserSignupApproved,
			Status: corev1.ConditionFalse,
			Reason: "PendingApproval",
		},
		{
			Type: v1alpha1.UserSignupComplete,
			Status: corev1.ConditionFalse,
			Reason: "PendingApproval",
		},
	})

	// Create user signup - approval set to false
	t.Logf("Creating UserSignup with namespace %s", namespace)
	userSignup = newUserSignup(namespace, "gretel")
	userSignup.Spec.Approved = false
	err = f.Client.Create(context.TODO(), userSignup, &framework.CleanupOptions{TestContext: ctx, Timeout: cleanupTimeout, RetryInterval: cleanupRetryInterval})
	require.NoError(t, err)
	t.Logf("user signup '%s' created", userSignup.Name)

	// Confirm the UserSignup was created
	err = waitForUserSignup(t, f.Client.Client, userSignup.Name, namespace)
	require.NoError(t, err)

	// Lookup the reconciled UserSignup
	err = f.Client.Get(context.TODO(), types.NamespacedName{Namespace: userSignup.Namespace, Name: userSignup.Name}, userSignup)
	require.NoError(t, err)

	// Confirm that the conditions are the same as if no approval value was set
	checkUserSignupConditions(t, userSignup.Status.Conditions, []v1alpha1.Condition{
		{
			Type: v1alpha1.UserSignupApproved,
			Status: corev1.ConditionFalse,
			Reason: "PendingApproval",
		},
		{
			Type: v1alpha1.UserSignupComplete,
			Status: corev1.ConditionFalse,
			Reason: "PendingApproval",
		},
	})

	// Create user signup - approval set to true
	t.Logf("Creating UserSignup with namespace %s", namespace)
	userSignup = newUserSignup(namespace, "harold")
	userSignup.Spec.Approved = true
	err = f.Client.Create(context.TODO(), userSignup, &framework.CleanupOptions{TestContext: ctx, Timeout: cleanupTimeout, RetryInterval: cleanupRetryInterval})
	require.NoError(t, err)
	t.Logf("user signup '%s' created", userSignup.Name)

	// Confirm the UserSignup was created
	err = waitForUserSignup(t, f.Client.Client, userSignup.Name, namespace)
	require.NoError(t, err)

	// Lookup the reconciled UserSignup
	err = f.Client.Get(context.TODO(), types.NamespacedName{Namespace: userSignup.Namespace, Name: userSignup.Name}, userSignup)
	require.NoError(t, err)

	// Confirm that:
	// 1) the Approved condition is set to true
	// 2) the Approved reason is set to ApprovedByAdmin
	// 3) the Complete condition is set to true
	checkUserSignupConditions(t, userSignup.Status.Conditions, []v1alpha1.Condition{
		{
			Type: v1alpha1.UserSignupApproved,
			Status: corev1.ConditionTrue,
			Reason: "ApprovedByAdmin",
		},
		{
			Type: v1alpha1.UserSignupComplete,
			Status: corev1.ConditionTrue,
		},
	})
}

func TestUserSignupWithManualApproval(t *testing.T) {
	// Test Setup
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

	err = e2eutil.WaitForDeployment(t, f.KubeClient, namespace, "host-operator", 1, operatorRetryInterval, operatorTimeout)
	require.NoError(t, err, "failed while waiting for host operator deployment")

	t.Log("host-operator is ready and in a running state")

	// Set the user approval policy to manual
	err = setApprovalPolicyConfig(f.KubeClient, namespace, config.UserApprovalPolicyManual)
	require.NoError(t, err)

	// Create user signup - no approval set
	t.Logf("Creating UserSignup with namespace %s", namespace)
	userSignup := newUserSignup(namespace, "johnsmith")
	err = f.Client.Create(context.TODO(), userSignup, &framework.CleanupOptions{TestContext: ctx, Timeout: cleanupTimeout, RetryInterval: cleanupRetryInterval})
	require.NoError(t, err)
	t.Logf("user signup '%s' created", userSignup.Name)

	// Confirm the UserSignup was created
	err = waitForUserSignup(t, f.Client.Client, userSignup.Name, namespace)
	require.NoError(t, err)

	// Confirm that:
	// 1) the Approved condition is set to false
	// 2) the Approved reason is set to PendingApproval
	// 3) the Complete condition is set to false
	// 4) the Complete reason is set to PendingApproval
	err = f.Client.Get(context.TODO(), types.NamespacedName{Namespace: userSignup.Namespace, Name: userSignup.Name}, userSignup)
	require.NoError(t, err)

	checkUserSignupConditions(t, userSignup.Status.Conditions, []v1alpha1.Condition{
		{
			Type: v1alpha1.UserSignupApproved,
			Status: corev1.ConditionFalse,
			Reason: "PendingApproval",
		},
		{
			Type: v1alpha1.UserSignupComplete,
			Status: corev1.ConditionFalse,
			Reason: "PendingApproval",
		},
	})

	// Create user signup - approval set to false
	t.Logf("Creating UserSignup with namespace %s", namespace)
	userSignup = newUserSignup(namespace, "janedoe")
	userSignup.Spec.Approved = false
	err = f.Client.Create(context.TODO(), userSignup, &framework.CleanupOptions{TestContext: ctx, Timeout: cleanupTimeout, RetryInterval: cleanupRetryInterval})
	require.NoError(t, err)
	t.Logf("user signup '%s' created", userSignup.Name)

	// Confirm the UserSignup was created
	err = waitForUserSignup(t, f.Client.Client, userSignup.Name, namespace)
	require.NoError(t, err)

	// Lookup the reconciled UserSignup
	err = f.Client.Get(context.TODO(), types.NamespacedName{Namespace: userSignup.Namespace, Name: userSignup.Name}, userSignup)
	require.NoError(t, err)

	// Confirm that the conditions are the same as if no approval value was set
	checkUserSignupConditions(t, userSignup.Status.Conditions, []v1alpha1.Condition{
		{
			Type: v1alpha1.UserSignupApproved,
			Status: corev1.ConditionFalse,
			Reason: "PendingApproval",
		},
		{
			Type: v1alpha1.UserSignupComplete,
			Status: corev1.ConditionFalse,
			Reason: "PendingApproval",
		},
	})

	// Create user signup - approval set to true
	t.Logf("Creating UserSignup with namespace %s", namespace)
	userSignup = newUserSignup(namespace, "robertjones")
	userSignup.Spec.Approved = true
	err = f.Client.Create(context.TODO(), userSignup, &framework.CleanupOptions{TestContext: ctx, Timeout: cleanupTimeout, RetryInterval: cleanupRetryInterval})
	require.NoError(t, err)
	t.Logf("user signup '%s' created", userSignup.Name)

	// Confirm the UserSignup was created
	err = waitForUserSignup(t, f.Client.Client, userSignup.Name, namespace)
	require.NoError(t, err)

	// Lookup the reconciled UserSignup
	err = f.Client.Get(context.TODO(), types.NamespacedName{Namespace: userSignup.Namespace, Name: userSignup.Name}, userSignup)
	require.NoError(t, err)

	// Confirm that:
	// 1) the Approved condition is set to true
	// 2) the Approved reason is set to ApprovedByAdmin
	// 3) the Complete condition is set to true
	checkUserSignupConditions(t, userSignup.Status.Conditions, []v1alpha1.Condition{
		{
			Type: v1alpha1.UserSignupApproved,
			Status: corev1.ConditionTrue,
			Reason: "ApprovedByAdmin",
		},
		{
			Type: v1alpha1.UserSignupComplete,
			Status: corev1.ConditionTrue,
		},
	})
}

func TestUserSignupWithAutoApproval(t *testing.T) {
	// Test Setup
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

	err = e2eutil.WaitForDeployment(t, f.KubeClient, namespace, "host-operator", 1, operatorRetryInterval, operatorTimeout)
	require.NoError(t, err, "failed while waiting for host operator deployment")

	t.Log("host-operator is ready and in a running state")

	// Set the user approval policy to automatic
	err = setApprovalPolicyConfig(f.KubeClient, namespace, config.UserApprovalPolicyAutomatic)
	require.NoError(t, err)

	// Create user signup - no approval set
	t.Logf("Creating UserSignup with namespace %s", namespace)
	userSignup := newUserSignup(namespace, "charles")
	err = f.Client.Create(context.TODO(), userSignup, &framework.CleanupOptions{TestContext: ctx, Timeout: cleanupTimeout, RetryInterval: cleanupRetryInterval})
	require.NoError(t, err)
	t.Logf("user signup '%s' created", userSignup.Name)

	// Confirm the UserSignup was created
	err = waitForUserSignup(t, f.Client.Client, userSignup.Name, namespace)
	require.NoError(t, err)

	// Confirm that:
	// 1) the Approved condition is set to true
	// 2) the Approved reason is set to ApprovedAutomatically
	// 3) the Complete condition is set to true
	err = f.Client.Get(context.TODO(), types.NamespacedName{Namespace: userSignup.Namespace, Name: userSignup.Name}, userSignup)
	require.NoError(t, err)

	checkUserSignupConditions(t, userSignup.Status.Conditions, []v1alpha1.Condition{
		{
			Type: v1alpha1.UserSignupApproved,
			Status: corev1.ConditionTrue,
			Reason: "ApprovedAutomatically",
		},
		{
			Type: v1alpha1.UserSignupComplete,
			Status: corev1.ConditionTrue,
		},
	})

	// Create user signup - approval set to false
	t.Logf("Creating UserSignup with namespace %s", namespace)
	userSignup = newUserSignup(namespace, "dorothy")
	userSignup.Spec.Approved = false
	err = f.Client.Create(context.TODO(), userSignup, &framework.CleanupOptions{TestContext: ctx, Timeout: cleanupTimeout, RetryInterval: cleanupRetryInterval})
	require.NoError(t, err)
	t.Logf("user signup '%s' created", userSignup.Name)

	// Confirm the UserSignup was created
	err = waitForUserSignup(t, f.Client.Client, userSignup.Name, namespace)
	require.NoError(t, err)

	// Lookup the reconciled UserSignup
	err = f.Client.Get(context.TODO(), types.NamespacedName{Namespace: userSignup.Namespace, Name: userSignup.Name}, userSignup)
	require.NoError(t, err)

	// Confirm that the conditions are as expected
	checkUserSignupConditions(t, userSignup.Status.Conditions, []v1alpha1.Condition{
		{
			Type: v1alpha1.UserSignupApproved,
			Status: corev1.ConditionTrue,
			Reason: "ApprovedAutomatically",
		},
		{
			Type: v1alpha1.UserSignupComplete,
			Status: corev1.ConditionTrue,
		},
	})

	// Create user signup - approval set to true
	t.Logf("Creating UserSignup with namespace %s", namespace)
	userSignup = newUserSignup(namespace, "edith")
	userSignup.Spec.Approved = true
	err = f.Client.Create(context.TODO(), userSignup, &framework.CleanupOptions{TestContext: ctx, Timeout: cleanupTimeout, RetryInterval: cleanupRetryInterval})
	require.NoError(t, err)
	t.Logf("user signup '%s' created", userSignup.Name)

	// Confirm the UserSignup was created
	err = waitForUserSignup(t, f.Client.Client, userSignup.Name, namespace)
	require.NoError(t, err)

	// Lookup the reconciled UserSignup
	err = f.Client.Get(context.TODO(), types.NamespacedName{Namespace: userSignup.Namespace, Name: userSignup.Name}, userSignup)
	require.NoError(t, err)

	// Confirm that:
	// 1) the Approved condition is set to true
	// 2) the Approved reason is set to ApprovedByAdmin
	// 3) the Complete condition is set to true
	checkUserSignupConditions(t, userSignup.Status.Conditions, []v1alpha1.Condition{
		{
			Type: v1alpha1.UserSignupApproved,
			Status: corev1.ConditionTrue,
			Reason: "ApprovedByAdmin",
		},
		{
			Type: v1alpha1.UserSignupComplete,
			Status: corev1.ConditionTrue,
		},
	})
}

func checkUserSignupConditions(t *testing.T, conditions, expectedConditions []v1alpha1.Condition) {
	for _, expected := range expectedConditions {
		found := false

		for _, condition := range conditions {
			if condition.Type == expected.Type {
				found = true

				// Check the expected status is correct
				require.Equal(t, expected.Status, condition.Status, "Expected condition status not set",
					expected.Type, expected.Status)

				// If a reason was supplied, confirm it is equal
				if expected.Reason != "" {
					require.Equal(t, expected.Reason, condition.Reason)
				}
			}
		}

		require.True(t, found, "Expected condition not found", expected.Type)
	}
}

func newUserSignup(namespace, name string) *v1alpha1.UserSignup {

	spec := v1alpha1.UserSignupSpec{
		UserID: uuid.NewV4().String(),
		TargetCluster: "east",
	}

	userSignup := &v1alpha1.UserSignup{
		ObjectMeta: v1.ObjectMeta{
			Name: name,
			Namespace: namespace,
		},
		Spec: spec,
	}

	return userSignup
}

func setApprovalPolicyConfig(clientSet kubernetes.Interface, namespace, policy string) error {
	// Create a new ConfigMap
	cm := &corev1.ConfigMap{
		ObjectMeta: v1.ObjectMeta{
			Name: config.ToolchainConfigMapName,
		},
	}

	err := clientSet.CoreV1().ConfigMaps(namespace).Delete(cm.Name, nil)
	if err != nil {
		if !errors.IsNotFound(err) {
			return err
		}
	}

	cmValues := make(map[string]string)
	cmValues[config.ToolchainConfigMapUserApprovalPolicy] = policy
	cm.Data = cmValues
	_, err = clientSet.CoreV1().ConfigMaps(namespace).Create(cm)
	return err
}

func clearApprovalPolicyConfig(clientSet kubernetes.Interface, namespace string) error {
	cm := &corev1.ConfigMap{
		ObjectMeta: v1.ObjectMeta{
			Name: config.ToolchainConfigMapName,
		},
	}

	err := clientSet.CoreV1().ConfigMaps(namespace).Delete(cm.Name, nil)
	if err != nil {
		if !errors.IsNotFound(err) {
			return err
		}
	}
	return nil
}