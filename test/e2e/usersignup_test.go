package e2e

import (
	"context"
	"github.com/codeready-toolchain/host-operator/pkg/config"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/operator-framework/operator-sdk/pkg/test/e2eutil"
	uuid "github.com/satori/go.uuid"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
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

type userSignupIntegrationTest struct {
	suite.Suite
	namespace string
	testCtx *framework.TestCtx
	framework *framework.Framework
}

func TestRunUserSignupIntegrationTest(t *testing.T) {
	suite.Run(t, &userSignupIntegrationTest{})
}

func (s *userSignupIntegrationTest) SetupTest() {
	// Test Setup
	userSignupList := &v1alpha1.UserSignupList{}

	err := framework.AddToFrameworkScheme(apis.AddToScheme, userSignupList)
	require.NoError(s.T(), err, "failed to add custom resource scheme to framework")

	s.testCtx = framework.NewTestCtx(s.T())
	defer s.testCtx.Cleanup()

	err = s.testCtx.InitializeClusterResources(&framework.CleanupOptions{TestContext: s.testCtx, Timeout: cleanupTimeout,
		RetryInterval: cleanupRetryInterval})
	require.NoError(s.T(), err, "failed to initialize cluster resources")
	s.T().Log("Initialized cluster resources")

	s.namespace, err = s.testCtx.GetNamespace()
	require.NoError(s.T(), err, "failed to get namespace where operator needs to run")

	s.framework = framework.Global

	err = e2eutil.WaitForDeployment(s.T(), s.framework.KubeClient, s.namespace, "host-operator", 1,
		operatorRetryInterval, operatorTimeout)
	require.NoError(s.T(), err, "failed while waiting for host operator deployment")

	s.T().Log("host-operator is ready and in a running state")
}

func (s *userSignupIntegrationTest) TestUserSignupCreated() {
	// Create user signup
	s.T().Logf("Creating UserSignup with namespace %s", s.namespace)
	userSignup := newUserSignup(s.namespace, "foo")

	err := s.framework.Client.Create(context.TODO(), userSignup, &framework.CleanupOptions{TestContext: s.testCtx,
		Timeout: cleanupTimeout, RetryInterval: cleanupRetryInterval})
	require.NoError(s.T(), err)
	s.T().Logf("user signup '%s' created", userSignup.Name)

	// Confirm the UserSignup was created
	err = waitForUserSignup(s.T(), s.framework.Client.Client, s.namespace, userSignup.Name)
	require.NoError(s.T(), err)

	// Confirm that a MasterUserRecord wasn't created
	err = waitForMasterUserRecord(s.T(), s.framework.Client.Client, s.namespace, userSignup.Name)
	require.Error(s.T(), err)

	// Delete the User Signup
	err = s.framework.Client.Delete(context.TODO(), userSignup)
	require.NoError(s.T(), err)
}

func (s *userSignupIntegrationTest) TestUserSignupWithNoApprovalConfig() {
	// Clear the user approval policy
	err := clearApprovalPolicyConfig(s.framework.KubeClient, s.namespace)
	require.NoError(s.T(), err)

	// Create user signup - no approval set
	s.T().Logf("Creating UserSignup with namespace %s", s.namespace)
	userSignup := newUserSignup(s.namespace, "francis")
	err = s.framework.Client.Create(context.TODO(), userSignup, &framework.CleanupOptions{TestContext: s.testCtx,
		Timeout: cleanupTimeout, RetryInterval: cleanupRetryInterval})
	require.NoError(s.T(), err)
	s.T().Logf("user signup '%s' created", userSignup.Name)

	// Confirm the UserSignup was created
	err = waitForUserSignup(s.T(), s.framework.Client.Client, s.namespace, userSignup.Name)
	require.NoError(s.T(), err)

	// Confirm that:
	// 1) the Approved condition is set to false
	// 2) the Approved reason is set to PendingApproval
	// 3) the Complete condition is set to false
	// 4) the Complete reason is set to PendingApproval
	err = waitForUserSignupStatusConditions(s.T(), s.framework.Client.Client, userSignup.Namespace, userSignup.Name,
		v1alpha1.Condition{
			Type: v1alpha1.UserSignupApproved,
			Status: corev1.ConditionFalse,
			Reason: "PendingApproval",
		},
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupComplete,
			Status: corev1.ConditionFalse,
			Reason: "PendingApproval",
		})
	require.NoError(s.T(), err)

	// Create user signup - approval set to false
	s.T().Logf("Creating UserSignup with namespace %s", s.namespace)
	userSignup = newUserSignup(s.namespace, "gretel")
	userSignup.Spec.Approved = false
	err = s.framework.Client.Create(context.TODO(), userSignup, &framework.CleanupOptions{TestContext: s.testCtx,
		Timeout: cleanupTimeout, RetryInterval: cleanupRetryInterval})
	require.NoError(s.T(), err)
	s.T().Logf("user signup '%s' created", userSignup.Name)

	// Confirm the UserSignup was created
	err = waitForUserSignup(s.T(), s.framework.Client.Client, s.namespace, userSignup.Name)
	require.NoError(s.T(), err)

	// Confirm that the conditions are the same as if no approval value was set
	err = waitForUserSignupStatusConditions(s.T(), s.framework.Client.Client, userSignup.Namespace, userSignup.Name,
		v1alpha1.Condition{
			Type: v1alpha1.UserSignupApproved,
			Status: corev1.ConditionFalse,
			Reason: "PendingApproval",
		},
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupComplete,
			Status: corev1.ConditionFalse,
			Reason: "PendingApproval",
		})
	require.NoError(s.T(), err)

	// Lookup the reconciled UserSignup
	err = s.framework.Client.Get(context.TODO(), types.NamespacedName{Namespace: userSignup.Namespace, Name: userSignup.Name}, userSignup)
	require.NoError(s.T(), err)

	// Now update the same userSignup setting approved to true
	userSignup.Spec.Approved = true
	err = s.framework.Client.Update(context.TODO(), userSignup)
	require.NoError(s.T(), err)

	// Check the updated conditions
	err = waitForUserSignupStatusConditions(s.T(), s.framework.Client.Client, userSignup.Namespace, userSignup.Name,
		v1alpha1.Condition{
			Type: v1alpha1.UserSignupApproved,
			Status: corev1.ConditionTrue,
			Reason: "ApprovedByAdmin",
		},
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupComplete,
			Status: corev1.ConditionTrue,
		})
	require.NoError(s.T(), err)

	// Create user signup - approval set to true
	s.T().Logf("Creating UserSignup with namespace %s", s.namespace)
	userSignup = newUserSignup(s.namespace, "harold")
	userSignup.Spec.Approved = true
	err = s.framework.Client.Create(context.TODO(), userSignup, &framework.CleanupOptions{TestContext: s.testCtx,
		Timeout: cleanupTimeout, RetryInterval: cleanupRetryInterval})
	require.NoError(s.T(), err)
	s.T().Logf("user signup '%s' created", userSignup.Name)

	// Confirm the UserSignup was created
	err = waitForUserSignup(s.T(), s.framework.Client.Client, s.namespace, userSignup.Name)
	require.NoError(s.T(), err)

	// Lookup the reconciled UserSignup
	err = s.framework.Client.Get(context.TODO(), types.NamespacedName{Namespace: userSignup.Namespace, Name: userSignup.Name}, userSignup)
	require.NoError(s.T(), err)

	// Confirm that:
	// 1) the Approved condition is set to true
	// 2) the Approved reason is set to ApprovedByAdmin
	// 3) the Complete condition is set to true
	err = waitForUserSignupStatusConditions(s.T(), s.framework.Client.Client, userSignup.Namespace, userSignup.Name,
		v1alpha1.Condition{
			Type: v1alpha1.UserSignupApproved,
			Status: corev1.ConditionTrue,
			Reason: "ApprovedByAdmin",
		},
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupComplete,
			Status: corev1.ConditionTrue,
		})
	require.NoError(s.T(), err)
}

func (s *userSignupIntegrationTest) TestUserSignupWithManualApproval() {
	// Set the user approval policy to manual
	err := setApprovalPolicyConfig(s.framework.KubeClient, s.namespace, config.UserApprovalPolicyManual)
	require.NoError(s.T(), err)

	// Create user signup - no approval set
	s.T().Logf("Creating UserSignup with namespace %s", s.namespace)
	userSignup := newUserSignup(s.namespace, "johnsmith")
	err = s.framework.Client.Create(context.TODO(), userSignup, &framework.CleanupOptions{TestContext: s.testCtx,
		Timeout: cleanupTimeout, RetryInterval: cleanupRetryInterval})
	require.NoError(s.T(), err)
	s.T().Logf("user signup '%s' created", userSignup.Name)

	// Confirm the UserSignup was created
	err = waitForUserSignup(s.T(), s.framework.Client.Client, s.namespace, userSignup.Name)
	require.NoError(s.T(), err)

	// Confirm that:
	// 1) the Approved condition is set to false
	// 2) the Approved reason is set to PendingApproval
	// 3) the Complete condition is set to false
	// 4) the Complete reason is set to PendingApproval
	err = s.framework.Client.Get(context.TODO(), types.NamespacedName{Namespace: userSignup.Namespace, Name: userSignup.Name}, userSignup)
	require.NoError(s.T(), err)

	checkUserSignupConditions(s.T(), userSignup.Status.Conditions, []v1alpha1.Condition{
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
	s.T().Logf("Creating UserSignup with namespace %s", s.namespace)
	userSignup = newUserSignup(s.namespace, "janedoe")
	userSignup.Spec.Approved = false
	err = s.framework.Client.Create(context.TODO(), userSignup, &framework.CleanupOptions{TestContext: s.testCtx,
		Timeout: cleanupTimeout, RetryInterval: cleanupRetryInterval})
	require.NoError(s.T(), err)
	s.T().Logf("user signup '%s' created", userSignup.Name)

	// Confirm the UserSignup was created
	err = waitForUserSignup(s.T(), s.framework.Client.Client, s.namespace, userSignup.Name)
	require.NoError(s.T(), err)

	// Lookup the reconciled UserSignup
	err = s.framework.Client.Get(context.TODO(), types.NamespacedName{Namespace: userSignup.Namespace, Name: userSignup.Name}, userSignup)
	require.NoError(s.T(), err)

	// Confirm that the conditions are the same as if no approval value was set
	checkUserSignupConditions(s.T(), userSignup.Status.Conditions, []v1alpha1.Condition{
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
	s.T().Logf("Creating UserSignup with namespace %s", s.namespace)
	userSignup = newUserSignup(s.namespace, "robertjones")
	userSignup.Spec.Approved = true
	err = s.framework.Client.Create(context.TODO(), userSignup, &framework.CleanupOptions{TestContext: s.testCtx,
		Timeout: cleanupTimeout, RetryInterval: cleanupRetryInterval})
	require.NoError(s.T(), err)
	s.T().Logf("user signup '%s' created", userSignup.Name)

	// Confirm the UserSignup was created
	err = waitForUserSignup(s.T(), s.framework.Client.Client, s.namespace, userSignup.Name)
	require.NoError(s.T(), err)

	// Confirm that:
	// 1) the Approved condition is set to true
	// 2) the Approved reason is set to ApprovedByAdmin
	// 3) the Complete condition is set to true
	err = waitForUserSignupStatusConditions(s.T(), s.framework.Client.Client, userSignup.Namespace, userSignup.Name,
		v1alpha1.Condition{
			Type: v1alpha1.UserSignupApproved,
			Status: corev1.ConditionTrue,
			Reason: "ApprovedByAdmin",
		},
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupComplete,
			Status: corev1.ConditionTrue,
		})
	require.NoError(s.T(), err)
}

func (s *userSignupIntegrationTest) TestUserSignupWithAutoApproval() {
	// Set the user approval policy to automatic
	err := setApprovalPolicyConfig(s.framework.KubeClient, s.namespace, config.UserApprovalPolicyAutomatic)
	require.NoError(s.T(), err)

	// Create user signup - no approval set
	s.T().Logf("Creating UserSignup with namespace %s", s.namespace)
	userSignup := newUserSignup(s.namespace, "charles")
	err = s.framework.Client.Create(context.TODO(), userSignup, &framework.CleanupOptions{TestContext: s.testCtx,
		Timeout: cleanupTimeout, RetryInterval: cleanupRetryInterval})
	require.NoError(s.T(), err)
	s.T().Logf("user signup '%s' created", userSignup.Name)

	// Confirm the UserSignup was created
	err = waitForUserSignup(s.T(), s.framework.Client.Client, s.namespace, userSignup.Name)
	require.NoError(s.T(), err)

	// Confirm that:
	// 1) the Approved condition is set to true
	// 2) the Approved reason is set to ApprovedByAdmin
	// 3) the Complete condition is (eventually) set to true
	err = waitForUserSignupStatusConditions(s.T(), s.framework.Client.Client, userSignup.Namespace, userSignup.Name,
		v1alpha1.Condition{
			Type: v1alpha1.UserSignupApproved,
			Status: corev1.ConditionTrue,
			Reason: "ApprovedAutomatically",
		},
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupComplete,
			Status: corev1.ConditionTrue,
		})
	require.NoError(s.T(), err)

	// Create user signup - approval set to false
	s.T().Logf("Creating UserSignup with namespace %s", s.namespace)
	userSignup = newUserSignup(s.namespace, "dorothy")
	userSignup.Spec.Approved = false
	err = s.framework.Client.Create(context.TODO(), userSignup, &framework.CleanupOptions{TestContext: s.testCtx,
		Timeout: cleanupTimeout, RetryInterval: cleanupRetryInterval})
	require.NoError(s.T(), err)
	s.T().Logf("user signup '%s' created", userSignup.Name)

	// Confirm the UserSignup was created
	err = waitForUserSignup(s.T(), s.framework.Client.Client, s.namespace, userSignup.Name)
	require.NoError(s.T(), err)

	// Lookup the reconciled UserSignup
	err = s.framework.Client.Get(context.TODO(), types.NamespacedName{Namespace: userSignup.Namespace, Name: userSignup.Name}, userSignup)
	require.NoError(s.T(), err)

	// Confirm that the conditions are as expected
	checkUserSignupConditions(s.T(), userSignup.Status.Conditions, []v1alpha1.Condition{
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
	s.T().Logf("Creating UserSignup with namespace %s", s.namespace)
	userSignup = newUserSignup(s.namespace, "edith")
	userSignup.Spec.Approved = true
	err = s.framework.Client.Create(context.TODO(), userSignup, &framework.CleanupOptions{TestContext: s.testCtx,
		Timeout: cleanupTimeout, RetryInterval: cleanupRetryInterval})
	require.NoError(s.T(), err)
	s.T().Logf("user signup '%s' created", userSignup.Name)

	// Confirm the UserSignup was created
	err = waitForUserSignup(s.T(), s.framework.Client.Client, s.namespace, userSignup.Name)
	require.NoError(s.T(), err)

	// Lookup the reconciled UserSignup
	err = s.framework.Client.Get(context.TODO(), types.NamespacedName{Namespace: userSignup.Namespace, Name: userSignup.Name}, userSignup)
	require.NoError(s.T(), err)

	// Confirm that:
	// 1) the Approved condition is set to true
	// 2) the Approved reason is set to ApprovedByAdmin
	test.AssertConditionsMatch(s.T(), userSignup.Status.Conditions, v1alpha1.Condition{
			Type: v1alpha1.UserSignupApproved,
			Status: corev1.ConditionTrue,
			Reason: "ApprovedByAdmin",
		},
	)

	// Confirm that the Complete condition is (eventually) set to true
	err = waitForUserSignupStatusConditions(s.T(), s.framework.Client.Client, userSignup.Namespace, userSignup.Name, v1alpha1.Condition{
		Type:   v1alpha1.UserSignupComplete,
		Status: corev1.ConditionTrue,
	})
	require.NoError(s.T(), err)
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