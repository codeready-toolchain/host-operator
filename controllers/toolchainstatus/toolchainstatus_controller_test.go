package toolchainstatus

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/controllers/registrationservice"
	"github.com/codeready-toolchain/host-operator/pkg/configuration"
	"github.com/codeready-toolchain/host-operator/pkg/counter"
	"github.com/codeready-toolchain/host-operator/pkg/metrics"
	. "github.com/codeready-toolchain/host-operator/test"
	"github.com/codeready-toolchain/host-operator/version"
	"github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	"github.com/codeready-toolchain/toolchain-common/pkg/status"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"

	"github.com/operator-framework/operator-sdk/pkg/k8sutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var requeueResult = reconcile.Result{RequeueAfter: 5 * time.Second}

const (
	defaultHostOperatorName        = "host-operator"
	defaultRegistrationServiceName = "registration-service"
	unreadyStatusNotification      = "toolchainstatus-unready"
	restoredStatusNotification     = "toolchainstatus-restore"
)

type fakeHTTPClient struct {
	response http.Response
	err      error
}

func (f *fakeHTTPClient) Get(url string) (*http.Response, error) {
	return &f.response, f.err
}

const respBodyGood = `{"alive":true,"environment":"dev","revision":"64af1be5c6011fae5497a7c35e2a986d633b3421","buildTime":"0","startTime":"2020-07-06T13:18:30Z"}`
const respBodyInvalid = `{"not found"}`
const respBodyBad = `{"alive":false,"environment":"dev","revision":"64af1be5c6011fae5497a7c35e2a986d633b3421","buildTime":"0","startTime":"2020-07-06T13:18:30Z"}`

var logger = logf.Log.WithName("toolchainstatus_controller_test")

func prepareReconcile(t *testing.T, requestName string, httpTestClient *fakeHTTPClient,
	memberClusters []string, initObjs ...runtime.Object) (*Reconciler, reconcile.Request, *test.FakeClient) {
	s := scheme.Scheme
	err := toolchainv1alpha1.AddToScheme(s)
	require.NoError(t, err)
	fakeClient := test.NewFakeClient(t, initObjs...)
	hostConfig, err := configuration.LoadConfig(fakeClient)
	require.NoError(t, err)

	r := &Reconciler{
		Client:         fakeClient,
		HTTPClientImpl: httpTestClient,
		Scheme:         s,
		GetMembersFunc: func(conditions ...cluster.Condition) []*cluster.CachedToolchainCluster {
			clusters := make([]*cluster.CachedToolchainCluster, len(memberClusters))
			for i, clusterName := range memberClusters {
				clusters[i] = cachedToolchainCluster(fakeClient, clusterName, corev1.ConditionTrue, metav1.Now())
			}
			return clusters
		},
		Config: hostConfig,
		Log:    ctrl.Log.WithName("controllers").WithName("ToolchainStatus"),
	}
	return r, reconcile.Request{NamespacedName: test.NamespacedName(test.HostOperatorNs, requestName)}, fakeClient
}

func prepareReconcileWithStatusConditions(t *testing.T, requestName string, memberClusters []string, conditions []toolchainv1alpha1.Condition, initObjs ...runtime.Object) (*Reconciler, reconcile.Request, *test.FakeClient) {
	reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), memberClusters, initObjs...)

	// explicitly set the conditions, so they are not empty/unknown
	toolchainStatus := &toolchainv1alpha1.ToolchainStatus{}
	err := fakeClient.Get(context.TODO(), test.NamespacedName(test.HostOperatorNs, requestName), toolchainStatus)
	require.NoError(t, err)
	toolchainStatus.Status.Conditions = conditions
	require.NoError(t, fakeClient.Status().Update(context.TODO(), toolchainStatus))

	return reconciler, req, fakeClient
}

func readyCondition(status bool) toolchainv1alpha1.Condition {
	ready := toolchainv1alpha1.Condition{
		Type:   toolchainv1alpha1.ConditionReady,
		Reason: toolchainv1alpha1.ToolchainStatusAllComponentsReadyReason,
	}
	if status {
		ready.Status = corev1.ConditionTrue
	} else {
		ready.Status = corev1.ConditionFalse
	}
	return ready
}

func TestNoToolchainStatusFound(t *testing.T) {
	t.Run("No toolchainstatus resource found", func(t *testing.T) {
		// given
		requestName := "bad-name"
		reconciler, req, _ := prepareReconcile(t, requestName, newResponseGood(), []string{"member-1", "member-2"})

		// when
		res, err := reconciler.Reconcile(req)

		// then - there should not be any error, the controller should only log that the resource was not found
		require.NoError(t, err)
		assert.Equal(t, reconcile.Result{}, res)
	})

	t.Run("No toolchainstatus resource found - right name but not found", func(t *testing.T) {
		// given
		expectedErrMsg := "get failed"
		requestName := configuration.ToolchainStatusName
		reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), []string{"member-1", "member-2"})
		fakeClient.MockGet = func(ctx context.Context, key types.NamespacedName, obj runtime.Object) error {
			return fmt.Errorf(expectedErrMsg)
		}

		// when
		res, err := reconciler.Reconcile(req)

		// then
		require.Error(t, err)
		require.Equal(t, expectedErrMsg, err.Error())
		assert.Equal(t, reconcile.Result{}, res)
	})
}

func TestToolchainStatusConditions(t *testing.T) {
	// set the operator name environment variable for all the tests which is used to get the host operator deployment name
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	restore := test.SetEnvVarsAndRestore(t, test.Env(k8sutil.OperatorNameEnvVar, defaultHostOperatorName))
	defer restore()
	requestName := configuration.ToolchainStatusName

	t.Run("All components ready", func(t *testing.T) {
		// given
		registrationService := newRegistrationServiceReady()
		hostOperatorDeployment := newDeploymentWithConditions(defaultHostOperatorName, status.DeploymentAvailableCondition(), status.DeploymentProgressingCondition())
		registrationServiceDeployment := newDeploymentWithConditions(registrationservice.ResourceName, status.DeploymentAvailableCondition(), status.DeploymentProgressingCondition())
		memberStatus := newMemberStatus(ready())
		toolchainStatus := NewToolchainStatus()
		reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), []string{"member-1", "member-2"}, hostOperatorDeployment, memberStatus, registrationServiceDeployment, registrationService, toolchainStatus)

		// when
		res, err := reconciler.Reconcile(req)

		// then
		require.NoError(t, err)
		assert.Equal(t, requeueResult, res)
		AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
			HasConditions(componentsReady(), unreadyNotificationNotCreated()).
			HasHostOperatorStatus(hostOperatorStatusReady()).
			HasMemberClusterStatus(memberCluster("member-1", ready()), memberCluster("member-2", ready())).
			HasRegistrationServiceStatus(registrationServiceReady())
	})

	t.Run("HostOperator tests", func(t *testing.T) {
		registrationService := newRegistrationServiceReady()
		toolchainStatus := NewToolchainStatus()
		registrationServiceDeployment := newDeploymentWithConditions(registrationservice.ResourceName, status.DeploymentAvailableCondition(), status.DeploymentProgressingCondition())
		memberStatus := newMemberStatus(ready())

		t.Run("Host operator deployment not found - deployment env var not set", func(t *testing.T) {
			// given
			resetFunc := test.UnsetEnvVarAndRestore(t, k8sutil.OperatorNameEnvVar)
			reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), []string{"member-1", "member-2"}, registrationServiceDeployment, registrationService, memberStatus, toolchainStatus)

			// when
			res, err := reconciler.Reconcile(req)

			// then
			resetFunc()
			require.NoError(t, err)
			assert.Equal(t, requeueResult, res)
			AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
				HasConditions(componentsNotReady(string(hostOperatorTag))).
				HasHostOperatorStatus(hostOperatorStatusNotReady("", "DeploymentNotFound", "unable to get the deployment: OPERATOR_NAME must be set")).
				HasMemberClusterStatus(memberCluster("member-1", ready()), memberCluster("member-2", ready())).
				HasRegistrationServiceStatus(registrationServiceReady())
		})

		t.Run("Host operator deployment not found", func(t *testing.T) {
			// given
			reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), []string{"member-1", "member-2"}, registrationServiceDeployment, registrationService, memberStatus, toolchainStatus)

			// when
			res, err := reconciler.Reconcile(req)

			// then
			require.NoError(t, err)
			assert.Equal(t, requeueResult, res)
			AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
				HasConditions(componentsNotReady(string(hostOperatorTag))).
				HasHostOperatorStatus(hostOperatorStatusNotReady(defaultHostOperatorName, "DeploymentNotFound", "unable to get the deployment: deployments.apps \"host-operator\" not found")).
				HasMemberClusterStatus(memberCluster("member-1", ready()), memberCluster("member-2", ready())).
				HasRegistrationServiceStatus(registrationServiceReady())
		})

		t.Run("Host operator deployment not ready", func(t *testing.T) {
			// given
			hostOperatorDeployment := newDeploymentWithConditions(defaultHostOperatorName, status.DeploymentNotAvailableCondition(), status.DeploymentProgressingCondition())
			reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), []string{"member-1", "member-2"}, hostOperatorDeployment, memberStatus, registrationServiceDeployment, registrationService, toolchainStatus)

			// when
			res, err := reconciler.Reconcile(req)

			// then
			require.NoError(t, err)
			assert.Equal(t, requeueResult, res)
			AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
				HasConditions(componentsNotReady(string(hostOperatorTag))).
				HasHostOperatorStatus(hostOperatorStatusNotReady(defaultHostOperatorName, "DeploymentNotReady", "deployment has unready status conditions: Available")).
				HasMemberClusterStatus(memberCluster("member-1", ready()), memberCluster("member-2", ready())).
				HasRegistrationServiceStatus(registrationServiceReady())
		})

		t.Run("Host operator deployment not progressing", func(t *testing.T) {
			// given
			hostOperatorDeployment := newDeploymentWithConditions(defaultHostOperatorName, status.DeploymentAvailableCondition(), status.DeploymentNotProgressingCondition())
			reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), []string{"member-1", "member-2"}, hostOperatorDeployment, memberStatus, registrationServiceDeployment, registrationService, toolchainStatus)

			// when
			res, err := reconciler.Reconcile(req)

			// then
			require.NoError(t, err)
			assert.Equal(t, requeueResult, res)
			AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
				HasConditions(componentsNotReady(string(hostOperatorTag))).
				HasHostOperatorStatus(hostOperatorStatusNotReady(defaultHostOperatorName, "DeploymentNotReady", "deployment has unready status conditions: Progressing")).
				HasMemberClusterStatus(memberCluster("member-1", ready()), memberCluster("member-2", ready())).
				HasRegistrationServiceStatus(registrationServiceReady())
		})
	})

	t.Run("RegistrationService deployment tests", func(t *testing.T) {
		registrationService := newRegistrationServiceReady()
		toolchainStatus := NewToolchainStatus()
		memberStatus := newMemberStatus(ready())
		hostOperatorDeployment := newDeploymentWithConditions(defaultHostOperatorName, status.DeploymentAvailableCondition(), status.DeploymentProgressingCondition())

		t.Run("Registration service deployment not found", func(t *testing.T) {
			// given
			reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), []string{"member-1", "member-2"}, hostOperatorDeployment, registrationService, memberStatus, toolchainStatus)

			// when
			res, err := reconciler.Reconcile(req)

			// then
			require.NoError(t, err)
			assert.Equal(t, requeueResult, res)
			AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
				HasConditions(componentsNotReady(string(registrationServiceTag))).
				HasHostOperatorStatus(hostOperatorStatusReady()).
				HasMemberClusterStatus(memberCluster("member-1", ready()), memberCluster("member-2", ready())).
				HasRegistrationServiceStatus(registrationServiceDeploymentNotReady("DeploymentNotFound", "unable to get the deployment: deployments.apps \"registration-service\" not found"))
		})

		t.Run("Registration service deployment not ready", func(t *testing.T) {
			// given
			registrationServiceDeployment := newDeploymentWithConditions(registrationservice.ResourceName, status.DeploymentNotAvailableCondition(), status.DeploymentProgressingCondition())
			reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), []string{"member-1", "member-2"}, hostOperatorDeployment, memberStatus, registrationServiceDeployment, registrationService, toolchainStatus)

			// when
			res, err := reconciler.Reconcile(req)

			// then
			require.NoError(t, err)
			assert.Equal(t, requeueResult, res)
			AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
				HasConditions(componentsNotReady(string(registrationServiceTag))).
				HasHostOperatorStatus(hostOperatorStatusReady()).
				HasMemberClusterStatus(memberCluster("member-1", ready()), memberCluster("member-2", ready())).
				HasRegistrationServiceStatus(registrationServiceDeploymentNotReady("DeploymentNotReady", "deployment has unready status conditions: Available"))
		})

		t.Run("Registration service deployment not progressing", func(t *testing.T) {
			// given
			registrationServiceDeployment := newDeploymentWithConditions(registrationservice.ResourceName, status.DeploymentAvailableCondition(), status.DeploymentNotProgressingCondition())
			reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), []string{"member-1", "member-2"}, hostOperatorDeployment, memberStatus, registrationServiceDeployment, registrationService, toolchainStatus)

			// when
			res, err := reconciler.Reconcile(req)

			// then
			require.NoError(t, err)
			assert.Equal(t, requeueResult, res)
			AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
				HasConditions(componentsNotReady(string(registrationServiceTag))).
				HasHostOperatorStatus(hostOperatorStatusReady()).
				HasMemberClusterStatus(memberCluster("member-1", ready()), memberCluster("member-2", ready())).
				HasRegistrationServiceStatus(registrationServiceDeploymentNotReady("DeploymentNotReady", "deployment has unready status conditions: Progressing"))
		})
	})

	t.Run("RegistrationService resource tests", func(t *testing.T) {
		toolchainStatus := NewToolchainStatus()
		memberStatus := newMemberStatus(ready())
		hostOperatorDeployment := newDeploymentWithConditions(defaultHostOperatorName, status.DeploymentAvailableCondition(), status.DeploymentProgressingCondition())
		registrationServiceDeployment := newDeploymentWithConditions(registrationservice.ResourceName, status.DeploymentAvailableCondition(), status.DeploymentProgressingCondition())

		t.Run("Registration service resource not found", func(t *testing.T) {
			// given
			reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), []string{"member-1", "member-2"}, hostOperatorDeployment, registrationServiceDeployment, memberStatus, toolchainStatus)

			// when
			res, err := reconciler.Reconcile(req)

			// then
			require.NoError(t, err)
			assert.Equal(t, requeueResult, res)
			AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
				HasConditions(componentsNotReady(string(registrationServiceTag))).
				HasHostOperatorStatus(hostOperatorStatusReady()).
				HasMemberClusterStatus(memberCluster("member-1", ready()), memberCluster("member-2", ready())).
				HasRegistrationServiceStatus(registrationServiceResourcesNotReady("RegServiceResourceNotFound", "registrationservices.toolchain.dev.openshift.com \"registration-service\" not found"))
		})

		t.Run("Registration service resource not ready", func(t *testing.T) {
			// given
			registrationService := newRegistrationServiceNotReady()
			reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), []string{"member-1", "member-2"}, hostOperatorDeployment, memberStatus, registrationServiceDeployment, registrationService, toolchainStatus)

			// when
			res, err := reconciler.Reconcile(req)

			// then
			require.NoError(t, err)
			assert.Equal(t, requeueResult, res)
			AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
				HasConditions(componentsNotReady(string(registrationServiceTag))).
				HasHostOperatorStatus(hostOperatorStatusReady()).
				HasMemberClusterStatus(memberCluster("member-1", ready()), memberCluster("member-2", ready())).
				HasRegistrationServiceStatus(registrationServiceResourcesNotReady("RegServiceNotReady", "registration service resource not ready"))
		})
	})

	t.Run("RegistrationService health tests", func(t *testing.T) {
		registrationService := newRegistrationServiceReady()
		hostOperatorDeployment := newDeploymentWithConditions(defaultHostOperatorName, status.DeploymentAvailableCondition(), status.DeploymentProgressingCondition())
		registrationServiceDeployment := newDeploymentWithConditions(registrationservice.ResourceName, status.DeploymentAvailableCondition(), status.DeploymentProgressingCondition())
		memberStatus := newMemberStatus(ready())
		toolchainStatus := NewToolchainStatus()

		t.Run("Registration health endpoint - http client error", func(t *testing.T) {
			// given
			reconciler, req, fakeClient := prepareReconcile(t, requestName, httpClientError(), []string{"member-1", "member-2"}, hostOperatorDeployment, registrationServiceDeployment, registrationService, memberStatus, toolchainStatus)

			// when
			res, err := reconciler.Reconcile(req)

			// then
			require.NoError(t, err)
			assert.Equal(t, requeueResult, res)
			AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
				HasConditions(componentsNotReady(string(registrationServiceTag))).
				HasHostOperatorStatus(hostOperatorStatusReady()).
				HasMemberClusterStatus(memberCluster("member-1", ready()), memberCluster("member-2", ready())).
				HasRegistrationServiceStatus(registrationServiceHealthNotReady("http client error"))
		})

		t.Run("Registration health endpoint - bad status code", func(t *testing.T) {
			// given
			reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseBadCode(), []string{"member-1", "member-2"}, hostOperatorDeployment, registrationServiceDeployment, registrationService, memberStatus, toolchainStatus)

			// when
			res, err := reconciler.Reconcile(req)

			// then
			require.NoError(t, err)
			assert.Equal(t, requeueResult, res)
			AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
				HasConditions(componentsNotReady(string(registrationServiceTag))).
				HasHostOperatorStatus(hostOperatorStatusReady()).
				HasMemberClusterStatus(memberCluster("member-1", ready()), memberCluster("member-2", ready())).
				HasRegistrationServiceStatus(registrationServiceHealthNotReady("bad response from http://registration-service/api/v1/health : statusCode=500"))
		})

		t.Run("Registration health endpoint - invalid JSON", func(t *testing.T) {
			// given
			reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseInvalid(), []string{"member-1", "member-2"}, hostOperatorDeployment, memberStatus, registrationServiceDeployment, registrationService, toolchainStatus)

			// when
			res, err := reconciler.Reconcile(req)

			// then
			require.NoError(t, err)
			assert.Equal(t, requeueResult, res)
			AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
				HasConditions(componentsNotReady(string(registrationServiceTag))).
				HasHostOperatorStatus(hostOperatorStatusReady()).
				HasMemberClusterStatus(memberCluster("member-1", ready()), memberCluster("member-2", ready())).
				HasRegistrationServiceStatus(registrationServiceHealthNotReady("invalid character '}' after object key"))
		})

		t.Run("Registration health endpoint - not alive", func(t *testing.T) {
			// given
			reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseBodyNotAlive(), []string{"member-1", "member-2"}, hostOperatorDeployment, memberStatus, registrationServiceDeployment, registrationService, toolchainStatus)

			// when
			res, err := reconciler.Reconcile(req)

			// then
			require.NoError(t, err)
			assert.Equal(t, requeueResult, res)
			AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
				HasConditions(componentsNotReady(string(registrationServiceTag))).
				HasHostOperatorStatus(hostOperatorStatusReady()).
				HasMemberClusterStatus(memberCluster("member-1", ready()), memberCluster("member-2", ready())).
				HasRegistrationServiceStatus(registrationServiceHealthNotReady("the registration service health endpoint is reporting an unhealthy status"))
		})
	})

	t.Run("MemberStatus tests", func(t *testing.T) {
		registrationService := newRegistrationServiceReady()
		hostOperatorDeployment := newDeploymentWithConditions(defaultHostOperatorName, status.DeploymentAvailableCondition(), status.DeploymentProgressingCondition())
		registrationServiceDeployment := newDeploymentWithConditions(registrationservice.ResourceName, status.DeploymentAvailableCondition(), status.DeploymentProgressingCondition())

		t.Run("MemberStatus not found", func(t *testing.T) {
			// given
			emptyToolchainStatus := NewToolchainStatus()
			reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), []string{}, hostOperatorDeployment, registrationServiceDeployment, registrationService, emptyToolchainStatus)

			// when
			res, err := reconciler.Reconcile(req)

			// then
			require.NoError(t, err)
			assert.Equal(t, requeueResult, res)
			AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
				HasConditions(componentsNotReady(string(memberConnectionsTag))).Exists().
				HasHostOperatorStatus(hostOperatorStatusReady()).
				HasMemberClusterStatus().
				HasRegistrationServiceStatus(registrationServiceReady())
		})

		t.Run("ToolchainCluster CR of member-1 and member-2 clusters were removed", func(t *testing.T) {
			// given
			memberStatus := newMemberStatus(ready())
			toolchainStatus := NewToolchainStatus(
				WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
					string(metrics.External): 20,
				}),
				WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
					"1,external": 20,
				}),
				WithMember("member-1", WithUserAccountCount(10)),
				WithMember("member-2", WithUserAccountCount(10)),
			)
			reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), []string{}, hostOperatorDeployment, memberStatus, registrationServiceDeployment, registrationService, toolchainStatus)
			InitializeCounters(t, toolchainStatus)

			// when
			res, err := reconciler.Reconcile(req)

			// then
			require.NoError(t, err)
			assert.Equal(t, requeueResult, res)
			AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
				HasConditions(componentsNotReady(string(memberConnectionsTag))).
				HasHostOperatorStatus(hostOperatorStatusReady()).
				HasMemberClusterStatus(
					memberCluster("member-1", userAccountCount(10), noResourceUsage(), notReady("MemberToolchainClusterMissing", "ToolchainCluster CR wasn't found for member cluster `member-1` that was previously registered in the host")),
					memberCluster("member-2", userAccountCount(10), noResourceUsage(), notReady("MemberToolchainClusterMissing", "ToolchainCluster CR wasn't found for member cluster `member-2` that was previously registered in the host")),
				).
				HasRegistrationServiceStatus(registrationServiceReady())
		})

		t.Run("ToolchainCluster CR of member-1 cluster was removed", func(t *testing.T) {
			// given
			memberStatus := newMemberStatus(ready())
			toolchainStatus := NewToolchainStatus(
				WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
					string(metrics.External): 20,
				}),
				WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
					"1,external": 20,
				}),
			)

			toolchainStatus.Status.Members = []toolchainv1alpha1.Member{
				memberCluster("member-1", ready(), userAccountCount(10)),
				memberCluster("member-2", ready(), userAccountCount(10)),
			}
			reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), []string{"member-2"}, hostOperatorDeployment, memberStatus, registrationServiceDeployment, registrationService, toolchainStatus)
			InitializeCounters(t, toolchainStatus)

			// when
			res, err := reconciler.Reconcile(req)

			// then
			require.NoError(t, err)
			assert.Equal(t, requeueResult, res)
			AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
				HasConditions(componentsNotReady(string(memberConnectionsTag))).
				HasHostOperatorStatus(hostOperatorStatusReady()).
				HasMemberClusterStatus(
					memberCluster("member-1", userAccountCount(10), noResourceUsage(), notReady("MemberToolchainClusterMissing", "ToolchainCluster CR wasn't found for member cluster `member-1` that was previously registered in the host")),
					memberCluster("member-2", userAccountCount(10), ready()),
				).
				HasRegistrationServiceStatus(registrationServiceReady())
		})

		t.Run("ToolchainCluster CR of member-2 cluster was removed", func(t *testing.T) {
			// given
			memberStatus := newMemberStatus(ready())
			toolchainStatus := NewToolchainStatus(
				WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
					string(metrics.External): 20,
				}),
				WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
					"1,external": 20,
				}),
			)
			toolchainStatus.Status.Members = []toolchainv1alpha1.Member{
				memberCluster("member-1", ready(), userAccountCount(10)),
				memberCluster("member-2", ready(), userAccountCount(10)),
			}
			reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), []string{"member-1"}, hostOperatorDeployment, memberStatus, registrationServiceDeployment, registrationService, toolchainStatus)
			InitializeCounters(t, toolchainStatus)

			// when
			res, err := reconciler.Reconcile(req)

			// then
			require.NoError(t, err)
			assert.Equal(t, requeueResult, res)
			AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
				HasConditions(componentsNotReady(string(memberConnectionsTag))).
				HasHostOperatorStatus(hostOperatorStatusReady()).
				HasMemberClusterStatus(
					memberCluster("member-1", userAccountCount(10), ready()),
					memberCluster("member-2", userAccountCount(10), noResourceUsage(), notReady("MemberToolchainClusterMissing", "ToolchainCluster CR wasn't found for member cluster `member-2` that was previously registered in the host")),
				).
				HasRegistrationServiceStatus(registrationServiceReady())
		})

		t.Run("MemberStatus saying that there was no member cluster present should be removed", func(t *testing.T) {
			// given
			memberStatus := newMemberStatus(ready())
			toolchainStatus := NewToolchainStatus()
			toolchainStatus.Status.Members = []toolchainv1alpha1.Member{
				memberCluster("member-1", userAccountCount(0), notReady("NoMemberClustersFound", "no member clusters found")),
			}
			reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), []string{"member-1", "member-2"}, hostOperatorDeployment, memberStatus, registrationServiceDeployment, registrationService, toolchainStatus)
			InitializeCounters(t, toolchainStatus)

			// when
			res, err := reconciler.Reconcile(req)

			// then
			require.NoError(t, err)
			assert.Equal(t, requeueResult, res)
			AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
				HasConditions(componentsReady(), unreadyNotificationNotCreated()).
				HasHostOperatorStatus(hostOperatorStatusReady()).
				HasMemberClusterStatus(memberCluster("member-1", ready()), memberCluster("member-2", ready())).
				HasRegistrationServiceStatus(registrationServiceReady())
		})

		t.Run("MemberStatus not found", func(t *testing.T) {
			// given
			emptyToolchainStatus := NewToolchainStatus()
			reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), []string{"member-1", "member-2"}, hostOperatorDeployment, registrationServiceDeployment, registrationService, emptyToolchainStatus)
			InitializeCounters(t, emptyToolchainStatus)

			// when
			res, err := reconciler.Reconcile(req)

			// then
			require.NoError(t, err)
			assert.Equal(t, requeueResult, res)
			AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
				HasConditions(componentsNotReady(string(memberConnectionsTag))).
				HasHostOperatorStatus(hostOperatorStatusReady()).
				HasMemberClusterStatus(
					memberCluster("member-1", noResourceUsage(), userAccountCount(0), notReady("MemberStatusNotFound", "memberstatuses.toolchain.dev.openshift.com \"toolchain-member-status\" not found")),
					memberCluster("member-2", noResourceUsage(), userAccountCount(0), notReady("MemberStatusNotFound", "memberstatuses.toolchain.dev.openshift.com \"toolchain-member-status\" not found")),
				).
				HasRegistrationServiceStatus(registrationServiceReady())
		})

		t.Run("MemberStatus not ready", func(t *testing.T) {
			// given
			emptyToolchainStatus := NewToolchainStatus()
			memberStatus := newMemberStatus(notReady(toolchainv1alpha1.ToolchainStatusComponentsNotReadyReason, "components not ready: [memberOperator]"))
			reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), []string{"member-1", "member-2"}, hostOperatorDeployment, memberStatus, registrationServiceDeployment, registrationService, emptyToolchainStatus)
			InitializeCounters(t, emptyToolchainStatus)

			// when
			res, err := reconciler.Reconcile(req)

			// then
			require.NoError(t, err)
			assert.Equal(t, requeueResult, res)
			AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
				HasConditions(componentsNotReady(string(memberConnectionsTag))).
				HasHostOperatorStatus(hostOperatorStatusReady()).
				HasMemberClusterStatus(
					memberCluster("member-1", notReady("ComponentsNotReady", "components not ready: [memberOperator]")),
					memberCluster("member-2", notReady("ComponentsNotReady", "components not ready: [memberOperator]")),
				).
				HasRegistrationServiceStatus(registrationServiceReady())
		})

		t.Run("synchronization with the counter fails", func(t *testing.T) {
			// given
			emptyToolchainStatus := NewToolchainStatus()
			memberStatus := newMemberStatus(ready())
			reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), []string{"member-1", "member-2"}, hostOperatorDeployment, memberStatus, registrationServiceDeployment, registrationService, emptyToolchainStatus)
			fakeClient.MockList = func(ctx context.Context, list runtime.Object, opts ...client.ListOption) error {
				return fmt.Errorf("some error")
			}

			// when
			res, err := reconciler.Reconcile(req)

			// then
			require.NoError(t, err)
			assert.Equal(t, requeueResult, res)
			AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
				HasConditions(componentsNotReady(string(counterTag))).
				HasHostOperatorStatus(hostOperatorStatusReady()).
				HasMemberClusterStatus(memberCluster("member-1", ready()), memberCluster("member-2", ready())).
				HasRegistrationServiceStatus(registrationServiceReady())
		})

		t.Run("MemberStatus not ready is changed to ready", func(t *testing.T) {
			// given
			memberStatus := newMemberStatus(ready())
			toolchainStatus := NewToolchainStatus()
			toolchainStatus.Status.Members = []toolchainv1alpha1.Member{
				memberCluster("member-1", notReady("ComponentsNotReady", "some cool error")),
				memberCluster("member-2", ready()),
			}
			reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), []string{"member-1", "member-2"}, hostOperatorDeployment, memberStatus, registrationServiceDeployment, registrationService, toolchainStatus)
			InitializeCounters(t, toolchainStatus)

			// when
			res, err := reconciler.Reconcile(req)

			// then
			require.NoError(t, err)
			assert.Equal(t, requeueResult, res)
			AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
				HasConditions(componentsReady(), unreadyNotificationNotCreated()).
				HasHostOperatorStatus(hostOperatorStatusReady()).
				HasMemberClusterStatus(memberCluster("member-1", ready()), memberCluster("member-2", ready())).
				HasRegistrationServiceStatus(registrationServiceReady())
		})

		t.Run("MemberStatus with no condition", func(t *testing.T) {
			// given
			memberStatus := newMemberStatus()
			toolchainStatus := NewToolchainStatus()
			reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), []string{"member-1", "member-2"}, hostOperatorDeployment, memberStatus, registrationServiceDeployment, registrationService, toolchainStatus)
			InitializeCounters(t, toolchainStatus)

			// when
			res, err := reconciler.Reconcile(req)

			// then
			require.NoError(t, err)
			assert.Equal(t, requeueResult, res)
			AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
				HasConditions(componentsNotReady(string(memberConnectionsTag))).
				HasHostOperatorStatus(hostOperatorStatusReady()).
				HasMemberClusterStatus(memberCluster("member-1"), memberCluster("member-2")).
				HasRegistrationServiceStatus(registrationServiceReady())
		})

		t.Run("All components ready but one member is missing", func(t *testing.T) {
			// given
			registrationService := newRegistrationServiceReady()
			hostOperatorDeployment := newDeploymentWithConditions(defaultHostOperatorName, status.DeploymentAvailableCondition(), status.DeploymentProgressingCondition())
			registrationServiceDeployment := newDeploymentWithConditions(registrationservice.ResourceName, status.DeploymentAvailableCondition(), status.DeploymentProgressingCondition())
			memberStatus := newMemberStatus(ready())
			toolchainStatus := NewToolchainStatus()

			t.Run("with non-zero user accounts", func(t *testing.T) {
				// given
				toolchainStatus.Status.Members = []toolchainv1alpha1.Member{
					// member-1 and member-2 will be added since there are MemberStatus resources for each one of them
					memberCluster("member-3", ready(), userAccountCount(10)), // will move to `NotReady` since there is no CachedToolchainCluster for this member
				}
				reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), []string{"member-1", "member-2"}, hostOperatorDeployment, memberStatus, registrationServiceDeployment, registrationService, toolchainStatus)
				InitializeCounters(t, toolchainStatus)

				// when
				res, err := reconciler.Reconcile(req)

				// then
				require.NoError(t, err)
				assert.Equal(t, requeueResult, res)
				AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
					HasConditions(componentsNotReady(string(memberConnectionsTag))).
					HasHostOperatorStatus(hostOperatorStatusReady()).
					HasMemberClusterStatus(
						memberCluster("member-1", ready()),
						memberCluster("member-2", ready()),
						memberCluster("member-3", noResourceUsage(), userAccountCount(0), notReady("MemberToolchainClusterMissing", "ToolchainCluster CR wasn't found for member cluster `member-3` that was previously registered in the host")),
					).
					HasRegistrationServiceStatus(registrationServiceReady())
			})

			t.Run("with zero user accounts", func(t *testing.T) {
				// given
				toolchainStatus.Status.Members = []toolchainv1alpha1.Member{
					memberCluster("removed-cluster", ready()), // will move to `NotReady` since there is no MemberStatus for this cluster
				}
				reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), []string{"member-1", "member-2"}, hostOperatorDeployment, memberStatus, registrationServiceDeployment, registrationService, toolchainStatus)
				InitializeCounters(t, toolchainStatus)

				// when
				res, err := reconciler.Reconcile(req)

				// then
				require.NoError(t, err)
				assert.Equal(t, requeueResult, res)
				AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
					HasConditions(componentsReady(), unreadyNotificationNotCreated()).
					HasHostOperatorStatus(hostOperatorStatusReady()).
					HasMemberClusterStatus(memberCluster("member-1", ready()), memberCluster("member-2", ready())).
					HasRegistrationServiceStatus(registrationServiceReady())
			})
		})
	})
}

func TestToolchainStatusReadyConditionTimestamps(t *testing.T) {
	// set the operator name environment variable for all the tests which is used to get the host operator deployment name
	restore := test.SetEnvVarsAndRestore(t, test.Env(k8sutil.OperatorNameEnvVar, defaultHostOperatorName))
	defer restore()
	requestName := configuration.ToolchainStatusName

	registrationService := newRegistrationServiceReady()
	toolchainStatus := NewToolchainStatus()
	memberStatus := newMemberStatus(ready())
	registrationServiceDeployment := newDeploymentWithConditions(registrationservice.ResourceName,
		status.DeploymentAvailableCondition(), status.DeploymentProgressingCondition())
	hostOperatorDeployment := newDeploymentWithConditions(defaultHostOperatorName,
		status.DeploymentAvailableCondition())

	t.Run("Timestamp set for new status object", func(t *testing.T) {
		// given a status with unknown (new) ready condition
		defer counter.Reset()

		reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(),
			[]string{"member-1", "member-2"}, hostOperatorDeployment, memberStatus, registrationServiceDeployment,
			registrationService, toolchainStatus)

		// when
		_, err := reconciler.Reconcile(req)

		// then
		require.NoError(t, err)

		AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
			HasConditions(componentsReady(), unreadyNotificationNotCreated()).
			ReadyConditionLastUpdatedTimeNotEmpty().
			ReadyConditionLastTransitionTimeNotEmpty()
	})

	t.Run("ready condition status has not changed", func(t *testing.T) {
		// given a status with a ready condition with timestamp set
		ready := readyCondition(true)
		before := metav1.NewTime(time.Now().Add(-10 * time.Second))
		ready.LastTransitionTime = before
		ready.LastUpdatedTime = &before
		conditions := []toolchainv1alpha1.Condition{ready}
		reconciler, req, fakeClient := prepareReconcileWithStatusConditions(t, requestName,
			[]string{"member-1", "member-2"},
			conditions,
			hostOperatorDeployment, memberStatus, registrationServiceDeployment, registrationService, toolchainStatus)

		// when no ready condition changed
		_, err := reconciler.Reconcile(req)

		// then
		require.NoError(t, err)

		AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
			HasConditions(componentsReady(), unreadyNotificationNotCreated()).
			ReadyConditionLastUpdatedTimeNotEqual(before). // Last update timestamp updated
			ReadyConditionLastTransitionTimeEqual(before)  // Last transition timestamp is not updated
	})

	t.Run("ready condition status has changed", func(t *testing.T) {
		// given a status with a ready condition with timestamp set
		ready := readyCondition(true)
		before := metav1.NewTime(time.Now().Add(-10 * time.Second))
		ready.LastTransitionTime = before
		ready.LastUpdatedTime = &before
		conditions := []toolchainv1alpha1.Condition{ready}
		hostOperatorDeployment := newDeploymentWithConditions(defaultHostOperatorName,
			status.DeploymentNotAvailableCondition(), status.DeploymentProgressingCondition())
		reconciler, req, fakeClient := prepareReconcileWithStatusConditions(t, requestName,
			[]string{"member-1", "member-2"},
			conditions,
			hostOperatorDeployment, memberStatus, registrationServiceDeployment, registrationService, toolchainStatus)

		// when the ready condition becomes not-ready
		_, err := reconciler.Reconcile(req)

		// then
		require.NoError(t, err)

		AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
			HasConditions(componentsNotReady(string(hostOperatorTag))).
			ReadyConditionLastUpdatedTimeNotEqual(before).   // Last update timestamp updated
			ReadyConditionLastTransitionTimeNotEqual(before) // Last transition timestamp is updated
	})
}

func TestToolchainStatusNotifications(t *testing.T) {
	// set the operator name environment variable for all the tests which is used to get the host operator deployment name
	restore := test.SetEnvVarsAndRestore(t, test.Env(k8sutil.OperatorNameEnvVar, defaultHostOperatorName))
	defer restore()
	defer counter.Reset()
	requestName := configuration.ToolchainStatusName

	registrationService := newRegistrationServiceReady()
	toolchainStatus := NewToolchainStatus()
	memberStatus := newMemberStatus(ready())
	registrationServiceDeployment := newDeploymentWithConditions(registrationservice.ResourceName,
		status.DeploymentAvailableCondition(), status.DeploymentProgressingCondition())

	t.Run("Notification workflow", func(t *testing.T) {
		// given
		hostOperatorDeployment := newDeploymentWithConditions(defaultHostOperatorName,
			status.DeploymentAvailableCondition())

		os.Setenv("HOST_OPERATOR_CONFIG_MAP_NAME", "notification_test_config")
		os.Setenv("WATCH_NAMESPACE", test.HostOperatorNs)

		config := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "notification_test_config",
				Namespace: test.HostOperatorNs,
			},
			Data: map[string]string{
				"admin.email": "admin@dev.sandbox.com",
			},
		}

		reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(),
			[]string{"member-1", "member-2"}, hostOperatorDeployment, memberStatus, registrationServiceDeployment,
			registrationService, toolchainStatus)

		// when
		res, err := reconciler.Reconcile(req)

		// then
		require.NoError(t, err)
		assert.Equal(t, requeueResult, res)

		AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
			HasConditions(componentsReady(), unreadyNotificationNotCreated()).
			HasHostOperatorStatus(hostOperatorStatusReady()).
			HasMemberClusterStatus(memberCluster("member-1", ready()), memberCluster("member-2", ready())).
			HasRegistrationServiceStatus(registrationServiceReady())

		// Confirm there is no notification
		assertToolchainStatusNotificationNotCreated(t, fakeClient, unreadyStatusNotification)
		assertToolchainStatusNotificationNotCreated(t, fakeClient, restoredStatusNotification)

		t.Run("Notification not created when host operator deployment not ready within threshold", func(t *testing.T) {
			// given
			hostOperatorDeployment := newDeploymentWithConditions(defaultHostOperatorName,
				status.DeploymentNotAvailableCondition(), status.DeploymentProgressingCondition())

			reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(),
				[]string{"member-1", "member-2"}, hostOperatorDeployment, memberStatus, registrationServiceDeployment,
				registrationService, toolchainStatus, config)

			// when
			res, err := reconciler.Reconcile(req)

			// then
			require.NoError(t, err)
			assert.Equal(t, requeueResult, res)

			// Confirm there is no notification
			assertToolchainStatusNotificationNotCreated(t, fakeClient, unreadyStatusNotification)
			assertToolchainStatusNotificationNotCreated(t, fakeClient, restoredStatusNotification)

			t.Run("Notification not created when admin.email not configured", func(t *testing.T) {

				assertInvalidEmailReturnErr := func(email string) {
					invalidConfig := &corev1.ConfigMap{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "notification_test_config",
							Namespace: test.HostOperatorNs,
						},
						Data: map[string]string{
							"admin.email": email,
						},
					}

					// given
					hostOperatorDeployment := newDeploymentWithConditions(defaultHostOperatorName,
						status.DeploymentNotAvailableCondition(), status.DeploymentProgressingCondition())

					// Reload the toolchain status
					require.NoError(t, fakeClient.Get(context.Background(), test.NamespacedName(test.HostOperatorNs,
						toolchainStatus.Name), toolchainStatus))

					overrideLastTransitionTime(t, toolchainStatus, metav1.Time{Time: time.Now().Add(-time.Duration(24) * time.Hour)})

					reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(),
						[]string{"member-1", "member-2"}, hostOperatorDeployment, memberStatus, registrationServiceDeployment,
						registrationService, toolchainStatus, invalidConfig)

					// when
					res, err := reconciler.Reconcile(req)

					// then
					require.Error(t, err)
					require.Equal(t, fmt.Sprintf("Failed to create user deactivation notification: cannot create notification "+
						"due to configuration error - admin.email [%s] is invalid or not set", email),
						err.Error())
					assert.Equal(t, requeueResult, res)

					// Confirm there is no notification
					assertToolchainStatusNotificationNotCreated(t, fakeClient, unreadyStatusNotification)
					assertToolchainStatusNotificationNotCreated(t, fakeClient, restoredStatusNotification)
				}

				assertInvalidEmailReturnErr("")
				assertInvalidEmailReturnErr("   ")
				assertInvalidEmailReturnErr("foo#bar.com")
			})

			t.Run("Notification created when host operator deployment not ready beyond threshold", func(t *testing.T) {
				// given
				hostOperatorDeployment := newDeploymentWithConditions(defaultHostOperatorName,
					status.DeploymentNotAvailableCondition(), status.DeploymentProgressingCondition())

				// Reload the toolchain status
				require.NoError(t, fakeClient.Get(context.Background(), test.NamespacedName(test.HostOperatorNs,
					toolchainStatus.Name), toolchainStatus))

				overrideLastTransitionTime(t, toolchainStatus, metav1.Time{Time: time.Now().Add(-time.Duration(24) * time.Hour)})

				reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(),
					[]string{"member-1", "member-2"}, hostOperatorDeployment, memberStatus, registrationServiceDeployment,
					registrationService, toolchainStatus, config)

				// when
				res, err := reconciler.Reconcile(req)

				// then
				require.NoError(t, err)
				assert.Equal(t, requeueResult, res)
				// confirm restored notification has not been created
				assertToolchainStatusNotificationNotCreated(t, fakeClient, restoredStatusNotification)
				// Confirm the unready notification has been created
				notification := assertToolchainStatusNotificationCreated(t, fakeClient)
				require.True(t, strings.HasPrefix(notification.ObjectMeta.Name, "toolchainstatus-unready-"))

				require.NotNil(t, notification)
				require.Equal(t, notification.Spec.Subject, "ToolchainStatus has been in an unready status for an extended period")
				require.Equal(t, notification.Spec.Recipient, "admin@dev.sandbox.com")

				t.Run("Toolchain status now ok again, notification should be removed", func(t *testing.T) {
					hostOperatorDeployment := newDeploymentWithConditions(defaultHostOperatorName,
						status.DeploymentAvailableCondition())

					// Reload the toolchain status
					require.NoError(t, fakeClient.Get(context.Background(), test.NamespacedName(test.HostOperatorNs,
						toolchainStatus.Name), toolchainStatus))

					reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(),
						[]string{"member-1", "member-2"}, hostOperatorDeployment, memberStatus, registrationServiceDeployment,
						registrationService, toolchainStatus)

					// when
					res, err := reconciler.Reconcile(req)

					// then
					require.NoError(t, err)
					assert.Equal(t, requeueResult, res)

					// Confirm there is no unready notification
					assertToolchainStatusNotificationNotCreated(t, fakeClient, unreadyStatusNotification)

					// Confirm restored notification has been created
					notification := assertToolchainStatusNotificationCreated(t, fakeClient)
					require.True(t, strings.HasPrefix(notification.ObjectMeta.Name, "toolchainstatus-restored-"))

					require.NotNil(t, notification)
					require.Equal(t, notification.Spec.Subject, "ToolchainStatus has now been restored to ready status")
					require.Equal(t, notification.Spec.Recipient, "admin@dev.sandbox.com")

					t.Run("Toolchain status not ready again for extended period, notification is created", func(t *testing.T) {
						// given
						hostOperatorDeployment := newDeploymentWithConditions(defaultHostOperatorName,
							status.DeploymentNotAvailableCondition(), status.DeploymentProgressingCondition())

						// Reload the toolchain status
						require.NoError(t, fakeClient.Get(context.Background(), test.NamespacedName(test.HostOperatorNs,
							toolchainStatus.Name), toolchainStatus))

						// Reconcile in order to update the ready status to false
						reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(),
							[]string{"member-1", "member-2"}, hostOperatorDeployment, memberStatus, registrationServiceDeployment,
							registrationService, toolchainStatus, config)

						// when
						_, err := reconciler.Reconcile(req)

						require.NoError(t, err)
						// Confirm there is no notification
						assertToolchainStatusNotificationNotCreated(t, fakeClient, unreadyStatusNotification)
						assertToolchainStatusNotificationNotCreated(t, fakeClient, restoredStatusNotification)

						// Reload the toolchain status
						require.NoError(t, fakeClient.Get(context.Background(), test.NamespacedName(test.HostOperatorNs,
							toolchainStatus.Name), toolchainStatus))

						// Now override the last transition time again
						overrideLastTransitionTime(t, toolchainStatus, metav1.Time{Time: time.Now().Add(-time.Duration(24) * time.Hour)})

						// Reconcile once more
						reconciler, req, fakeClient = prepareReconcile(t, requestName, newResponseGood(),
							[]string{"member-1", "member-2"}, hostOperatorDeployment, memberStatus, registrationServiceDeployment,
							registrationService, toolchainStatus, config)

						// when
						res, err = reconciler.Reconcile(req)

						// then
						require.NoError(t, err)
						assert.Equal(t, requeueResult, res)
						// Confirm restored notification is not created
						assertToolchainStatusNotificationNotCreated(t, fakeClient, restoredStatusNotification)
						// Confirm the unready notification has been created
						notification := assertToolchainStatusNotificationCreated(t, fakeClient)
						require.True(t, strings.HasPrefix(notification.ObjectMeta.Name, "toolchainstatus-unready-"))
						require.Len(t, notification.ObjectMeta.Name, 38)
						require.NotNil(t, notification)
						assert.Equal(t, notification.Spec.Subject, "ToolchainStatus has been in an unready status for an extended period")
						assert.Equal(t, notification.Spec.Recipient, "admin@dev.sandbox.com")
						assert.True(t, strings.HasPrefix(notification.Spec.Content, "<div><pre><code>"))
						assert.True(t, strings.HasSuffix(notification.Spec.Content, "</code></pre></div>"))
						assert.NotContains(t, notification.Spec.Content, "managedFields")
					})
				})
			})
		})
	})
}

func overrideLastTransitionTime(t *testing.T, toolchainStatus *toolchainv1alpha1.ToolchainStatus, overrideTime metav1.Time) {
	found := false
	for i, cond := range toolchainStatus.Status.Conditions {
		if cond.Type == toolchainv1alpha1.ConditionReady {
			cond.LastTransitionTime = overrideTime
			toolchainStatus.Status.Conditions[i] = cond
			found = true
			break
		}
	}

	require.True(t, found)
}

func assertToolchainStatusNotificationCreated(t *testing.T, fakeClient *test.FakeClient) *toolchainv1alpha1.Notification {
	notifications := &toolchainv1alpha1.NotificationList{}
	err := fakeClient.List(context.Background(), notifications, &client.ListOptions{
		Namespace: test.HostOperatorNs,
	})
	require.NoError(t, err)
	require.Len(t, notifications.Items, 1)
	return &notifications.Items[0]
}

func assertToolchainStatusNotificationNotCreated(t *testing.T, fakeClient *test.FakeClient, notificationType string) {
	notifications := &toolchainv1alpha1.NotificationList{}
	err := fakeClient.List(context.Background(), notifications, &client.ListOptions{
		Namespace: test.HostOperatorNs,
	})
	require.NoError(t, err)
	for _, notification := range notifications.Items {
		require.False(t, strings.HasPrefix(notification.ObjectMeta.Name, notificationType))
	}
}

func TestSynchronizationWithCounter(t *testing.T) {
	// given
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	restore := test.SetEnvVarsAndRestore(t, test.Env(k8sutil.OperatorNameEnvVar, defaultHostOperatorName))
	defer restore()
	requestName := configuration.ToolchainStatusName
	registrationService := newRegistrationServiceReady()
	hostOperatorDeployment := newDeploymentWithConditions(defaultHostOperatorName, status.DeploymentAvailableCondition(), status.DeploymentProgressingCondition())
	registrationServiceDeployment := newDeploymentWithConditions(registrationservice.ResourceName, status.DeploymentAvailableCondition(), status.DeploymentProgressingCondition())
	memberStatus := newMemberStatus(ready())

	t.Run("Load all current resources", func(t *testing.T) {
		// given
		defer counter.Reset()
		toolchainStatus := NewToolchainStatus()
		initObjects := append([]runtime.Object{}, hostOperatorDeployment, memberStatus, registrationServiceDeployment, registrationService, toolchainStatus)
		initObjects = append(initObjects, CreateMultipleUserSignups("cookie-", 8)...)
		initObjects = append(initObjects, CreateMultipleMurs(t, "cookie-", 8, "member-1")...)
		initObjects = append(initObjects, CreateMultipleUserSignups("pasta-", 2)...)
		initObjects = append(initObjects, CreateMultipleMurs(t, "pasta-", 2, "member-2")...)

		reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), []string{"member-1", "member-2"}, initObjects...)

		// when
		res, err := reconciler.Reconcile(req)

		// then
		require.NoError(t, err)
		assert.Equal(t, requeueResult, res)
		AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
			HasConditions(componentsReady(), unreadyNotificationNotCreated()).
			HasHostOperatorStatus(hostOperatorStatusReady()).
			HasMemberClusterStatus(
				memberCluster("member-1", ready(), userAccountCount(8)),
				memberCluster("member-2", ready(), userAccountCount(2))).
			HasRegistrationServiceStatus(registrationServiceReady()).
			Exists().HasUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
			"1,internal": 2, // users "cookie-00" and "pasta-00"
			"2,internal": 2, // users "cookie-01" and "pasta-01"
			"3,internal": 1, // users "cookie-02"
			"4,internal": 1, // users "cookie-03"
			"5,internal": 1, // etc.
			"6,internal": 1,
			"7,internal": 1,
			"8,internal": 1,
		})

		t.Run("sync with newly added MURs and UAs", func(t *testing.T) {
			// given
			counter.IncrementMasterUserRecordCount(logger, metrics.Internal)
			counter.IncrementMasterUserRecordCount(logger, metrics.External)
			counter.IncrementUserAccountCount(logger, "member-1")
			toolchainStatus := NewToolchainStatus()
			reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), []string{"member-1", "member-2"}, hostOperatorDeployment, memberStatus, registrationServiceDeployment, registrationService, toolchainStatus)

			// when
			res, err := reconciler.Reconcile(req)

			// then
			require.NoError(t, err)
			assert.Equal(t, requeueResult, res)
			AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
				HasConditions(componentsReady(), unreadyNotificationNotCreated()).
				HasHostOperatorStatus(hostOperatorStatusReady()).
				HasMemberClusterStatus(
					memberCluster("member-1", ready(), userAccountCount(9)),
					memberCluster("member-2", ready(), userAccountCount(2))).
				HasRegistrationServiceStatus(registrationServiceReady())
		})

	})

	t.Run("initialize the cache using the ToolchainStatus resource", func(t *testing.T) {
		// given
		defer counter.Reset()
		toolchainStatus := NewToolchainStatus(
			WithMember("member-1", WithUserAccountCount(6)), // will increase
			WithMember("member-2", WithUserAccountCount(2)), // will remain the same
			WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
				"1,internal": 4,
				"1,external": 1,
				"2,internal": 1,
				"2,external": 1,
				"3,internal": 1,
			}),
			WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
				string(metrics.External): 8,
			}),
		)
		reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), []string{"member-1", "member-2"}, hostOperatorDeployment, memberStatus, registrationServiceDeployment, registrationService, toolchainStatus)

		// when
		counter.IncrementMasterUserRecordCount(logger, metrics.Internal)
		counter.IncrementUserAccountCount(logger, "member-1")
		counter.UpdateUsersPerActivationCounters(logger, 1, metrics.Internal)
		counter.UpdateUsersPerActivationCounters(logger, 2, metrics.Internal)
		res, err := reconciler.Reconcile(req)

		// then
		require.NoError(t, err)
		assert.Equal(t, requeueResult, res)
		AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
			HasConditions(componentsReady(), unreadyNotificationNotCreated()).
			HasHostOperatorStatus(hostOperatorStatusReady()).
			HasMemberClusterStatus(
				memberCluster("member-1", ready(), userAccountCount(7)), // was incremented
				memberCluster("member-2", ready(), userAccountCount(2))).
			HasRegistrationServiceStatus(registrationServiceReady()).
			HasUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
				"1,internal": 4, // was incremented by `counter.UpdateUsersPerActivationCounters(1)` but decremented `counter.UpdateUsersPerActivationCounters(2)`
				"1,external": 1, // unchanged
				"2,internal": 2, // was incremented by `counter.UpdateUsersPerActivationCounters(2)`
				"2,external": 1, // unchanged
				"3,internal": 1, // unchanged
			})
		AssertThatCountersAndMetrics(t).
			HaveUserAccountsForCluster("member-1", 7).
			HaveUserAccountsForCluster("member-2", 2).
			HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
				string(metrics.Internal): 1,
				string(metrics.External): 8,
			}).
			HaveUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
				"1,internal": 4, // was incremented by `counter.UpdateUsersPerActivationCounters(1)` but decremented `counter.UpdateUsersPerActivationCounters(2)`
				"1,external": 1, // unchanged
				"2,internal": 2, // was incremented by `counter.UpdateUsersPerActivationCounters(2)`
				"2,external": 1, // unchanged
				"3,internal": 1, // unchanged
			})
	})
}

func newDeploymentWithConditions(deploymentName string, deploymentConditions ...appsv1.DeploymentCondition) *appsv1.Deployment {
	replicas := int32(1)
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploymentName,
			Namespace: test.HostOperatorNs,
			Labels: map[string]string{
				"foo": "bar",
			},
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: "apps/v1",
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
		},
		Status: appsv1.DeploymentStatus{
			Conditions: deploymentConditions,
		},
	}
}

func cachedToolchainCluster(cl client.Client, name string, status corev1.ConditionStatus, lastProbeTime metav1.Time) *cluster.CachedToolchainCluster {
	return &cluster.CachedToolchainCluster{
		Name:              name,
		Client:            cl,
		Type:              cluster.Host,
		OperatorNamespace: test.MemberOperatorNs,
		OwnerClusterName:  test.MemberClusterName,
		APIEndpoint:       "http://api.devcluster.openshift.com",
		ClusterStatus: &toolchainv1alpha1.ToolchainClusterStatus{
			Conditions: []toolchainv1alpha1.ToolchainClusterCondition{{
				Type:          toolchainv1alpha1.ToolchainClusterReady,
				Status:        status,
				LastProbeTime: lastProbeTime,
			}},
		},
	}
}

type memberstatusOptions interface {
	applyToMemberStatus(*toolchainv1alpha1.MemberStatus)
}

func newMemberStatus(options ...memberstatusOptions) *toolchainv1alpha1.MemberStatus {
	status := &toolchainv1alpha1.MemberStatus{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: test.MemberOperatorNs,
			Name:      memberStatusName,
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "MemberStatus",
			APIVersion: "toolchain.dev.openshift.com/v1alpha1",
		},
		Spec: toolchainv1alpha1.MemberStatusSpec{},
		Status: toolchainv1alpha1.MemberStatusStatus{
			Conditions: []toolchainv1alpha1.Condition{},
			ResourceUsage: toolchainv1alpha1.ResourceUsage{
				MemoryUsagePerNodeRole: map[string]int{
					"worker": 60,
					"master": 45,
				},
			},
			Routes: &toolchainv1alpha1.Routes{
				ConsoleURL:      "http://console.openshift.com/url",
				CheDashboardURL: "http://console.openshift.com/url",
				Conditions:      []toolchainv1alpha1.Condition{ToBeReady()},
			},
		},
	}
	for _, opt := range options {
		opt.applyToMemberStatus(status)
	}

	return status
}

func newRegistrationServiceReady() *toolchainv1alpha1.RegistrationService {
	readyStatus := status.NewComponentReadyCondition(toolchainv1alpha1.ToolchainStatusRegServiceReadyReason)
	return &toolchainv1alpha1.RegistrationService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      registrationservice.ResourceName,
			Namespace: test.HostOperatorNs,
			Labels: map[string]string{
				"foo": "bar",
			},
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "RegistrationService",
			APIVersion: "toolchain.dev.openshift.com/v1alpha1",
		},
		Status: toolchainv1alpha1.RegistrationServiceStatus{
			Conditions: []toolchainv1alpha1.Condition{*readyStatus},
		},
	}
}

func newRegistrationServiceNotReady() *toolchainv1alpha1.RegistrationService {
	notReadyStatus := status.NewComponentErrorCondition(toolchainv1alpha1.ToolchainStatusRegServiceNotReadyReason, "registration service resource not ready")
	return &toolchainv1alpha1.RegistrationService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      registrationservice.ResourceName,
			Namespace: test.HostOperatorNs,
			Labels: map[string]string{
				"foo": "bar",
			},
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "RegistrationService",
			APIVersion: "toolchain.dev.openshift.com/v1alpha1",
		},
		Status: toolchainv1alpha1.RegistrationServiceStatus{
			Conditions: []toolchainv1alpha1.Condition{*notReadyStatus},
		},
	}
}

func httpClientError() *fakeHTTPClient {
	return &fakeHTTPClient{
		err: fmt.Errorf("http client error"),
	}
}

func newResponseGood() *fakeHTTPClient {
	return &fakeHTTPClient{
		response: http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(bytes.NewReader([]byte(respBodyGood))),
		},
	}
}

func newResponseBadCode() *fakeHTTPClient {
	return &fakeHTTPClient{
		response: http.Response{
			StatusCode: 500,
			Body:       ioutil.NopCloser(bytes.NewReader([]byte(""))),
		},
	}
}

func newResponseBodyNotAlive() *fakeHTTPClient {
	return &fakeHTTPClient{
		response: http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(bytes.NewReader([]byte(respBodyBad))),
		},
	}
}

func newResponseInvalid() *fakeHTTPClient {
	return &fakeHTTPClient{
		response: http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(bytes.NewReader([]byte(respBodyInvalid))),
		},
	}
}

func componentsReady() toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:   toolchainv1alpha1.ConditionReady,
		Status: corev1.ConditionTrue,
		Reason: toolchainv1alpha1.ToolchainStatusAllComponentsReadyReason,
	}
}

func unreadyNotificationNotCreated() toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:   toolchainv1alpha1.ToolchainStatusUnreadyNotificationCreated,
		Status: corev1.ConditionFalse,
		Reason: "AllComponentsReady",
	}
}

func componentsNotReady(components ...string) toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:    toolchainv1alpha1.ConditionReady,
		Status:  corev1.ConditionFalse,
		Reason:  toolchainv1alpha1.ToolchainStatusComponentsNotReadyReason,
		Message: fmt.Sprintf("components not ready: %v", components),
	}
}

func hostOperatorStatusReady() toolchainv1alpha1.HostOperatorStatus {
	return toolchainv1alpha1.HostOperatorStatus{
		Conditions: []toolchainv1alpha1.Condition{
			{
				Type:   toolchainv1alpha1.ConditionReady,
				Status: corev1.ConditionTrue,
				Reason: "DeploymentReady",
			},
		},
		BuildTimestamp: version.BuildTime,
		DeploymentName: defaultHostOperatorName,
		Revision:       version.Commit,
		Version:        version.Version,
	}
}

func hostOperatorStatusNotReady(deploymentName, reason, msg string) toolchainv1alpha1.HostOperatorStatus {
	return toolchainv1alpha1.HostOperatorStatus{
		Conditions: []toolchainv1alpha1.Condition{
			{
				Type:    toolchainv1alpha1.ConditionReady,
				Status:  corev1.ConditionFalse,
				Reason:  reason,
				Message: msg,
			},
		},
		BuildTimestamp: version.BuildTime,
		DeploymentName: deploymentName,
		Revision:       version.Commit,
		Version:        version.Version,
	}
}

type statusCondition toolchainv1alpha1.Condition

func (c statusCondition) applyToMemberStatus(s *toolchainv1alpha1.MemberStatus) {
	s.Status.Conditions = append(s.Status.Conditions, toolchainv1alpha1.Condition(c))
}

func (c statusCondition) applyToMember(m *toolchainv1alpha1.Member) {
	m.MemberStatus.Conditions = append(m.MemberStatus.Conditions, toolchainv1alpha1.Condition(c))
}

func ready() statusCondition {
	return statusCondition(toolchainv1alpha1.Condition{
		Type:   toolchainv1alpha1.ConditionReady,
		Status: corev1.ConditionTrue,
		Reason: "AllComponentsReady",
	})
}

func notReady(reason, msg string) statusCondition {
	return statusCondition(toolchainv1alpha1.Condition{
		Type:    toolchainv1alpha1.ConditionReady,
		Status:  corev1.ConditionFalse,
		Reason:  reason,
		Message: msg,
	})
}

func noResourceUsage() resourceUsage {
	return resourceUsage(nil)
}

type resourceUsage map[string]int

func (n resourceUsage) applyToMember(m *toolchainv1alpha1.Member) {
	m.MemberStatus.ResourceUsage = toolchainv1alpha1.ResourceUsage{
		MemoryUsagePerNodeRole: n,
	}
}

type userAccountCount int

func (c userAccountCount) applyToMember(m *toolchainv1alpha1.Member) {
	m.UserAccountCount = int(c)
}

type memberClusterOption interface {
	applyToMember(*toolchainv1alpha1.Member)
}

func memberCluster(name string, options ...memberClusterOption) toolchainv1alpha1.Member {
	m := toolchainv1alpha1.Member{
		ApiEndpoint: "http://api.devcluster.openshift.com",
		ClusterName: name,
		MemberStatus: toolchainv1alpha1.MemberStatusStatus{
			Conditions: []toolchainv1alpha1.Condition{},
			ResourceUsage: toolchainv1alpha1.ResourceUsage{
				MemoryUsagePerNodeRole: map[string]int{
					"worker": 60,
					"master": 45,
				},
			},
			Routes: &toolchainv1alpha1.Routes{
				ConsoleURL:      "http://console.openshift.com/url",
				CheDashboardURL: "http://console.openshift.com/url",
				Conditions:      []toolchainv1alpha1.Condition{ToBeReady()},
			},
		},
		UserAccountCount: 0,
	}
	for _, opt := range options {
		opt.applyToMember(&m)
	}
	return m
}

type regTestDeployStatus struct {
	deploymentName string
	condition      toolchainv1alpha1.Condition
}

func registrationServiceReady() toolchainv1alpha1.HostRegistrationServiceStatus {
	deploy := regTestDeployStatus{
		deploymentName: defaultRegistrationServiceName,
		condition: toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.ConditionReady,
			Status: corev1.ConditionTrue,
			Reason: "DeploymentReady",
		},
	}

	res := toolchainv1alpha1.Condition{
		Type:   toolchainv1alpha1.ConditionReady,
		Status: corev1.ConditionTrue,
		Reason: "RegServiceReady",
	}

	health := toolchainv1alpha1.Condition{
		Type:   toolchainv1alpha1.ConditionReady,
		Status: corev1.ConditionTrue,
		Reason: "RegServiceReady",
	}
	return registrationServiceStatus(deploy, res, health)
}

func registrationServiceDeploymentNotReady(reason, msg string) toolchainv1alpha1.HostRegistrationServiceStatus {
	deploy := regTestDeployStatus{
		deploymentName: defaultRegistrationServiceName,
		condition: toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.ConditionReady,
			Status:  corev1.ConditionFalse,
			Reason:  reason,
			Message: msg,
		},
	}

	res := toolchainv1alpha1.Condition{
		Type:   toolchainv1alpha1.ConditionReady,
		Status: corev1.ConditionTrue,
		Reason: "RegServiceReady",
	}

	health := toolchainv1alpha1.Condition{
		Type:   toolchainv1alpha1.ConditionReady,
		Status: corev1.ConditionTrue,
		Reason: "RegServiceReady",
	}
	return registrationServiceStatus(deploy, res, health)
}

func registrationServiceResourcesNotReady(reason, msg string) toolchainv1alpha1.HostRegistrationServiceStatus {
	deploy := regTestDeployStatus{
		deploymentName: defaultRegistrationServiceName,
		condition: toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.ConditionReady,
			Status: corev1.ConditionTrue,
			Reason: "DeploymentReady",
		},
	}

	res := toolchainv1alpha1.Condition{
		Type:    toolchainv1alpha1.ConditionReady,
		Status:  corev1.ConditionFalse,
		Reason:  reason,
		Message: msg,
	}

	health := toolchainv1alpha1.Condition{
		Type:   toolchainv1alpha1.ConditionReady,
		Status: corev1.ConditionTrue,
		Reason: "RegServiceReady",
	}
	return registrationServiceStatus(deploy, res, health)
}

func registrationServiceHealthNotReady(msg string) toolchainv1alpha1.HostRegistrationServiceStatus {
	deploy := regTestDeployStatus{
		deploymentName: defaultRegistrationServiceName,
		condition: toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.ConditionReady,
			Status: corev1.ConditionTrue,
			Reason: "DeploymentReady",
		},
	}

	res := toolchainv1alpha1.Condition{
		Type:   toolchainv1alpha1.ConditionReady,
		Status: corev1.ConditionTrue,
		Reason: "RegServiceReady",
	}

	health := toolchainv1alpha1.Condition{
		Type:    toolchainv1alpha1.ConditionReady,
		Status:  corev1.ConditionFalse,
		Reason:  "RegServiceNotReady",
		Message: msg,
	}
	return registrationServiceStatus(deploy, res, health)
}

func registrationServiceStatus(deploy regTestDeployStatus, res toolchainv1alpha1.Condition, health toolchainv1alpha1.Condition) toolchainv1alpha1.HostRegistrationServiceStatus {
	return toolchainv1alpha1.HostRegistrationServiceStatus{
		Deployment: toolchainv1alpha1.RegistrationServiceDeploymentStatus{
			Name:       deploy.deploymentName,
			Conditions: []toolchainv1alpha1.Condition{deploy.condition},
		},
		RegistrationServiceResources: toolchainv1alpha1.RegistrationServiceResourcesStatus{
			Conditions: []toolchainv1alpha1.Condition{res},
		},
		Health: toolchainv1alpha1.RegistrationServiceHealth{
			Conditions: []toolchainv1alpha1.Condition{health},
		},
	}
}
