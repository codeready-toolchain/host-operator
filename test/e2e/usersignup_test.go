package e2e

import (
	"context"
	"github.com/codeready-toolchain/host-operator/pkg/config"
	"github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	uuid "github.com/satori/go.uuid"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"testing"
	"time"

	"github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/test/e2e"

	framework "github.com/operator-framework/operator-sdk/pkg/test"
)

const (
	cleanupTimeout = time.Second * 5
	cleanupRetryInterval = time.Second * 1
)

type userSignupIntegrationTest struct {
	suite.Suite
	namespace string
	testCtx *framework.TestCtx
	awaitility *e2e.Awaitility
	hostAwait *HostAwaitility
}

func TestRunUserSignupIntegrationTest(t *testing.T) {
	suite.Run(t, &userSignupIntegrationTest{})
}

func (s *userSignupIntegrationTest) SetupSuite() {
	userSignupList := &v1alpha1.UserSignupList{}
	s.testCtx, s.awaitility = e2e.InitializeOperators(s.T(), userSignupList, cluster.Host)
	s.hostAwait = NewHostAwaitility(s.awaitility)
	ns, err := s.testCtx.GetNamespace()
	require.NoError(s.T(), err)
	s.namespace = ns
}

func (s *userSignupIntegrationTest) TestUserSignupCreated() {
	// Create user signup
	s.T().Logf("Creating UserSignup with namespace %s", s.namespace)
	userSignup := s.newUserSignup("foo")

	err := s.awaitility.Client.Create(context.TODO(), userSignup, &framework.CleanupOptions{TestContext: s.testCtx,
		Timeout: cleanupTimeout, RetryInterval: cleanupRetryInterval})
	require.NoError(s.T(), err)
	s.T().Logf("user signup '%s' created", userSignup.Name)

	// Confirm the UserSignup was created
	err = s.hostAwait.waitForUserSignup(userSignup.Name)
	require.NoError(s.T(), err)

	// Confirm that a MasterUserRecord wasn't created
	err = s.hostAwait.waitForMasterUserRecord(userSignup.Name)
	require.Error(s.T(), err)

	// Delete the User Signup
	err = s.awaitility.Client.Delete(context.TODO(), userSignup)
	require.NoError(s.T(), err)
}

func (s *userSignupIntegrationTest) TestUserSignupWithNoApprovalConfig() {
	// Clear the user approval policy
	err := clearApprovalPolicyConfig(s.awaitility.KubeClient, s.namespace)
	require.NoError(s.T(), err)

	// Create user signup - no approval set
	s.T().Logf("Creating UserSignup with namespace %s", s.namespace)
	userSignup := s.newUserSignup("francis")
	err = s.awaitility.Client.Create(context.TODO(), userSignup, &framework.CleanupOptions{TestContext: s.testCtx,
		Timeout: cleanupTimeout, RetryInterval: cleanupRetryInterval})
	require.NoError(s.T(), err)
	s.T().Logf("user signup '%s' created", userSignup.Name)

	// Confirm the UserSignup was created
	err = s.hostAwait.waitForUserSignup(userSignup.Name)
	require.NoError(s.T(), err)

	// Confirm that:
	// 1) the Approved condition is set to false
	// 2) the Approved reason is set to PendingApproval
	// 3) the Complete condition is set to false
	// 4) the Complete reason is set to PendingApproval
	err = s.hostAwait.waitForUserSignupStatusConditions(userSignup.Name,
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
	userSignup = s.newUserSignup("gretel")
	userSignup.Spec.Approved = false
	err = s.awaitility.Client.Create(context.TODO(), userSignup, &framework.CleanupOptions{TestContext: s.testCtx,
		Timeout: cleanupTimeout, RetryInterval: cleanupRetryInterval})
	require.NoError(s.T(), err)
	s.T().Logf("user signup '%s' created", userSignup.Name)

	// Confirm the UserSignup was created
	err = s.hostAwait.waitForUserSignup(userSignup.Name)
	require.NoError(s.T(), err)

	// Confirm that the conditions are the same as if no approval value was set
	err = s.hostAwait.waitForUserSignupStatusConditions(userSignup.Name,
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
	err = s.awaitility.Client.Get(context.TODO(), types.NamespacedName{Namespace: userSignup.Namespace, Name: userSignup.Name}, userSignup)
	require.NoError(s.T(), err)

	// Now update the same userSignup setting approved to true
	userSignup.Spec.Approved = true
	err = s.awaitility.Client.Update(context.TODO(), userSignup)
	require.NoError(s.T(), err)

	// Check the updated conditions
	err = s.hostAwait.waitForUserSignupStatusConditions(userSignup.Name,
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
	userSignup = s.newUserSignup("harold")
	userSignup.Spec.Approved = true
	err = s.awaitility.Client.Create(context.TODO(), userSignup, &framework.CleanupOptions{TestContext: s.testCtx,
		Timeout: cleanupTimeout, RetryInterval: cleanupRetryInterval})
	require.NoError(s.T(), err)
	s.T().Logf("user signup '%s' created", userSignup.Name)

	// Confirm the UserSignup was created
	err = s.hostAwait.waitForUserSignup(userSignup.Name)
	require.NoError(s.T(), err)

	// Lookup the reconciled UserSignup
	err = s.awaitility.Client.Get(context.TODO(), types.NamespacedName{Namespace: userSignup.Namespace, Name: userSignup.Name}, userSignup)
	require.NoError(s.T(), err)

	// Confirm that:
	// 1) the Approved condition is set to true
	// 2) the Approved reason is set to ApprovedByAdmin
	// 3) the Complete condition is set to true
	err = s.hostAwait.waitForUserSignupStatusConditions(userSignup.Name,
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
	s.setApprovalPolicyConfig(config.UserApprovalPolicyManual)

	// Create user signup - no approval set
	s.T().Logf("Creating UserSignup with namespace %s", s.namespace)
	userSignup := s.newUserSignup("mariecurie")
	err := s.awaitility.Client.Create(context.TODO(), userSignup, &framework.CleanupOptions{TestContext: s.testCtx,
		Timeout: cleanupTimeout, RetryInterval: cleanupRetryInterval})
	require.NoError(s.T(), err)
	s.T().Logf("user signup '%s' created", userSignup.Name)

	// Confirm the UserSignup was created
	err = s.hostAwait.waitForUserSignup(userSignup.Name)
	require.NoError(s.T(), err)

	// Confirm that:
	// 1) the Approved condition is set to false
	// 2) the Approved reason is set to PendingApproval
	// 3) the Complete condition is set to false
	// 4) the Complete reason is set to PendingApproval
	err = s.hostAwait.waitForUserSignupStatusConditions(userSignup.Name,
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
	userSignup = s.newUserSignup("janedoe")
	userSignup.Spec.Approved = false
	err = s.awaitility.Client.Create(context.TODO(), userSignup, &framework.CleanupOptions{TestContext: s.testCtx,
		Timeout: cleanupTimeout, RetryInterval: cleanupRetryInterval})
	require.NoError(s.T(), err)
	s.T().Logf("user signup '%s' created", userSignup.Name)

	// Confirm the UserSignup was created
	err = s.hostAwait.waitForUserSignup(userSignup.Name)
	require.NoError(s.T(), err)

	// Lookup the reconciled UserSignup
	err = s.awaitility.Client.Get(context.TODO(), types.NamespacedName{Namespace: userSignup.Namespace, Name: userSignup.Name}, userSignup)
	require.NoError(s.T(), err)

	// Confirm that the conditions are the same as if no approval value was set
	err = s.hostAwait.waitForUserSignupStatusConditions(userSignup.Name,
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

	// Create user signup - approval set to true
	s.T().Logf("Creating UserSignup with namespace %s", s.namespace)
	userSignup = s.newUserSignup("robertjones")
	userSignup.Spec.Approved = true
	err = s.awaitility.Client.Create(context.TODO(), userSignup, &framework.CleanupOptions{TestContext: s.testCtx,
		Timeout: cleanupTimeout, RetryInterval: cleanupRetryInterval})
	require.NoError(s.T(), err)
	s.T().Logf("user signup '%s' created", userSignup.Name)

	// Confirm the UserSignup was created
	err = s.hostAwait.waitForUserSignup(userSignup.Name)
	require.NoError(s.T(), err)

	// Confirm that:
	// 1) the Approved condition is set to true
	// 2) the Approved reason is set to ApprovedByAdmin
	// 3) the Complete condition is set to true
	err = s.hostAwait.waitForUserSignupStatusConditions(userSignup.Name,
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
	s.setApprovalPolicyConfig(config.UserApprovalPolicyAutomatic)

	// Create user signup - no approval set
	s.T().Logf("Creating UserSignup with namespace %s", s.namespace)
	userSignup := s.newUserSignup("charles")
	err := s.awaitility.Client.Create(context.TODO(), userSignup, &framework.CleanupOptions{TestContext: s.testCtx,
		Timeout: cleanupTimeout, RetryInterval: cleanupRetryInterval})
	require.NoError(s.T(), err)
	s.T().Logf("user signup '%s' created", userSignup.Name)

	// Confirm the UserSignup was created
	err = s.hostAwait.waitForUserSignup(userSignup.Name)
	require.NoError(s.T(), err)

	// Confirm the MasterUserRecord was created
	//err = waitForMasterUserRecord(s.T(), s.framework.Client.Client, s.namespace, userSignup.Name)
	//require.NoError(s.T(), err)

	// Confirm that:
	// 1) the Approved condition is set to true
	// 2) the Approved reason is set to ApprovedAutomatically
	// 3) the Complete condition is (eventually) set to true
	err = s.hostAwait.waitForUserSignupStatusConditions(userSignup.Name,
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
	userSignup = s.newUserSignup("dorothy")
	userSignup.Spec.Approved = false
	err = s.awaitility.Client.Create(context.TODO(), userSignup, &framework.CleanupOptions{TestContext: s.testCtx,
		Timeout: cleanupTimeout, RetryInterval: cleanupRetryInterval})
	require.NoError(s.T(), err)
	s.T().Logf("user signup '%s' created", userSignup.Name)

	// Confirm the UserSignup was created
	err = s.hostAwait.waitForUserSignup(userSignup.Name)
	require.NoError(s.T(), err)

	// Lookup the reconciled UserSignup
	err = s.awaitility.Client.Get(context.TODO(), types.NamespacedName{Namespace: userSignup.Namespace, Name: userSignup.Name}, userSignup)
	require.NoError(s.T(), err)

	// Confirm that the conditions are as expected
	err = s.hostAwait.waitForUserSignupStatusConditions(userSignup.Name,
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

	// Create user signup - approval set to true
	s.T().Logf("Creating UserSignup with namespace %s", s.namespace)
	userSignup = s.newUserSignup("edith")
	userSignup.Spec.Approved = true
	err = s.awaitility.Client.Create(context.TODO(), userSignup, &framework.CleanupOptions{TestContext: s.testCtx,
		Timeout: cleanupTimeout, RetryInterval: cleanupRetryInterval})
	require.NoError(s.T(), err)
	s.T().Logf("user signup '%s' created", userSignup.Name)

	// Confirm the UserSignup was created
	err = s.hostAwait.waitForUserSignup(userSignup.Name)
	require.NoError(s.T(), err)

	// Lookup the reconciled UserSignup
	err = s.awaitility.Client.Get(context.TODO(), types.NamespacedName{Namespace: userSignup.Namespace, Name: userSignup.Name}, userSignup)
	require.NoError(s.T(), err)

	// Confirm the conditions
	err = s.hostAwait.waitForUserSignupStatusConditions(userSignup.Name,
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

func (s *userSignupIntegrationTest) newUserSignup(name string) *v1alpha1.UserSignup {

	memberCluster, ok, err := s.awaitility.Host().GetKubeFedCluster(s.awaitility.MemberNs, cluster.Member, e2e.ReadyKubeFedCluster)
	require.NoError(s.awaitility.T, err)
	require.True(s.awaitility.T, ok, "KubeFedCluster should exist")

	spec := v1alpha1.UserSignupSpec{
		UserID: uuid.NewV4().String(),
		TargetCluster: memberCluster.Name,
	}

	userSignup := &v1alpha1.UserSignup{
		ObjectMeta: v1.ObjectMeta{
			Name: name,
			Namespace: s.namespace,
		},
		Spec: spec,
	}

	return userSignup
}

func (s *userSignupIntegrationTest) setApprovalPolicyConfig(policy string) {
	// Create a new ConfigMap
	cm := &corev1.ConfigMap{
		ObjectMeta: v1.ObjectMeta{
			Name: config.ToolchainConfigMapName,
		},
	}

	// Clear the current approval policy
	err := clearApprovalPolicyConfig(s.awaitility.KubeClient, s.namespace)
	require.NoError(s.T(), err)

	cmValues := make(map[string]string)
	cmValues[config.ToolchainConfigMapUserApprovalPolicy] = policy
	cm.Data = cmValues
	_, err = s.awaitility.KubeClient.CoreV1().ConfigMaps(s.namespace).Create(cm)
	require.NoError(s.T(), err)

	// Confirm it was updated
	cm, err = s.awaitility.KubeClient.CoreV1().ConfigMaps(s.namespace).Get(config.ToolchainConfigMapName, v1.GetOptions{})
	require.NoError(s.T(), err)
	require.Equal(s.T(), policy, cm.Data[config.ToolchainConfigMapUserApprovalPolicy])
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