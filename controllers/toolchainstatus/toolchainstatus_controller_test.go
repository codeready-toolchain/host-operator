package toolchainstatus

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/codeready-toolchain/host-operator/pkg/apis"
	"github.com/codeready-toolchain/toolchain-common/pkg/condition"
	routev1 "github.com/openshift/api/route/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/controllers/toolchainconfig"
	"github.com/codeready-toolchain/host-operator/pkg/counter"
	"github.com/codeready-toolchain/host-operator/pkg/metrics"
	"github.com/codeready-toolchain/host-operator/pkg/templates/registrationservice"
	. "github.com/codeready-toolchain/host-operator/test"
	"github.com/codeready-toolchain/host-operator/version"
	"github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	commonconfig "github.com/codeready-toolchain/toolchain-common/pkg/configuration"
	"github.com/codeready-toolchain/toolchain-common/pkg/status"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	testconfig "github.com/codeready-toolchain/toolchain-common/pkg/test/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var requeueResult = reconcile.Result{RequeueAfter: 5 * time.Second}

const (
	defaultHostOperatorName           = "host-operator"
	defaultHostOperatorDeploymentName = "host-operator-controller-manager"
	defaultRegistrationServiceName    = "registration-service"
	unreadyStatusNotification         = "toolchainstatus-unready"
	restoredStatusNotification        = "toolchainstatus-restore"
)

type fakeHTTPClient struct {
	response http.Response
	err      error
}

func (f *fakeHTTPClient) Get(_ string) (*http.Response, error) {
	return &f.response, f.err
}

const respBodyGood = `{"alive":true,"environment":"dev","revision":"64af1be5c6011fae5497a7c35e2a986d633b3421","buildTime":"0","startTime":"2020-07-06T13:18:30Z"}`
const respBodyInvalid = `{"not found"}`
const respBodyBad = `{"alive":false,"environment":"dev","revision":"64af1be5c6011fae5497a7c35e2a986d633b3421","buildTime":"0","startTime":"2020-07-06T13:18:30Z"}`

var logger = logf.Log.WithName("toolchainstatus_controller_test")

func prepareReconcile(t *testing.T, requestName string, httpTestClient *fakeHTTPClient,
	memberClusters []string, initObjs ...runtime.Object) (*Reconciler, reconcile.Request, *test.FakeClient) {

	os.Setenv("WATCH_NAMESPACE", test.HostOperatorNs)
	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)
	fakeClient := test.NewFakeClient(t, initObjs...)
	require.NoError(t, err)

	r := &Reconciler{
		Client:         fakeClient,
		HTTPClientImpl: httpTestClient,
		Scheme:         s,
		Namespace:      test.HostOperatorNs,
		GetMembersFunc: func(conditions ...cluster.Condition) []*cluster.CachedToolchainCluster {
			clusters := make([]*cluster.CachedToolchainCluster, len(memberClusters))
			for i, clusterName := range memberClusters {
				clusters[i] = cachedToolchainCluster(fakeClient, clusterName, corev1.ConditionTrue, metav1.Now())
			}
			return clusters
		},
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
		res, err := reconciler.Reconcile(context.TODO(), req)

		// then - there should not be any error, the controller should only log that the resource was not found
		require.NoError(t, err)
		assert.Equal(t, reconcile.Result{}, res)
	})

	t.Run("No toolchainstatus resource found - right name but not found", func(t *testing.T) {
		// given
		requestName := toolchainconfig.ToolchainStatusName
		reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), []string{"member-1", "member-2"})
		fakeClient.MockGet = func(ctx context.Context, key types.NamespacedName, obj client.Object) error {
			if _, ok := obj.(*toolchainv1alpha1.ToolchainStatus); ok {
				return fmt.Errorf("get failed")
			}
			return fakeClient.Client.Get(ctx, key, obj)
		}

		// when
		res, err := reconciler.Reconcile(context.TODO(), req)

		// then
		require.Error(t, err)
		require.Equal(t, "get failed", err.Error())
		assert.Equal(t, reconcile.Result{}, res)
	})
}

func TestToolchainStatusConditions(t *testing.T) {
	// set the operator name environment variable for all the tests which is used to get the host operator deployment name
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	restore := test.SetEnvVarsAndRestore(t, test.Env(commonconfig.OperatorNameEnvVar, defaultHostOperatorName))
	defer restore()
	requestName := toolchainconfig.ToolchainStatusName

	t.Run("All components ready", func(t *testing.T) {
		// given
		hostOperatorDeployment := newDeploymentWithConditions(defaultHostOperatorDeploymentName, status.DeploymentAvailableCondition(), status.DeploymentProgressingCondition())
		registrationServiceDeployment := newDeploymentWithConditions(registrationservice.ResourceName, status.DeploymentAvailableCondition(), status.DeploymentProgressingCondition())
		memberStatus := newMemberStatus(ready())
		toolchainStatus := NewToolchainStatus()
		reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), []string{"member-1", "member-2"},
			hostOperatorDeployment, memberStatus, registrationServiceDeployment, toolchainStatus, proxyRoute())

		// when
		res, err := reconciler.Reconcile(context.TODO(), req)

		// then
		require.NoError(t, err)
		assert.Equal(t, requeueResult, res)
		AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
			HasConditions(componentsReady(), unreadyNotificationNotCreated()).
			HasHostOperatorStatus(hostOperatorStatusReady()).
			HasMemberClusterStatus(memberCluster("member-1", ready()), memberCluster("member-2", ready())).
			HasRegistrationServiceStatus(registrationServiceReady()).
			HasHostRoutesStatus("https://api-toolchain-host-operator.host-cluster", hostRoutesAvailable())
	})

	t.Run("HostOperator tests", func(t *testing.T) {
		toolchainStatus := NewToolchainStatus()
		registrationServiceDeployment := newDeploymentWithConditions(registrationservice.ResourceName, status.DeploymentAvailableCondition(), status.DeploymentProgressingCondition())
		memberStatus := newMemberStatus(ready())

		t.Run("Host operator deployment not found - deployment env var not set", func(t *testing.T) {
			// given
			resetFunc := test.UnsetEnvVarAndRestore(t, commonconfig.OperatorNameEnvVar)
			reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), []string{"member-1", "member-2"},
				registrationServiceDeployment, memberStatus, toolchainStatus, proxyRoute())

			// when
			res, err := reconciler.Reconcile(context.TODO(), req)

			// then
			resetFunc()
			require.NoError(t, err)
			assert.Equal(t, requeueResult, res)
			AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
				HasConditions(componentsNotReady(string(hostOperatorTag))).
				HasHostOperatorStatus(hostOperatorStatusNotReady("", "DeploymentNotFound", "unable to get the deployment: OPERATOR_NAME must be set")).
				HasMemberClusterStatus(memberCluster("member-1", ready()), memberCluster("member-2", ready())).
				HasRegistrationServiceStatus(registrationServiceReady()).
				HasHostRoutesStatus("https://api-toolchain-host-operator.host-cluster", hostRoutesAvailable())
		})

		t.Run("Host operator deployment not found", func(t *testing.T) {
			// given
			reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), []string{"member-1", "member-2"},
				registrationServiceDeployment, memberStatus, toolchainStatus, proxyRoute())

			// when
			res, err := reconciler.Reconcile(context.TODO(), req)

			// then
			require.NoError(t, err)
			assert.Equal(t, requeueResult, res)
			AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
				HasConditions(componentsNotReady(string(hostOperatorTag))).
				HasHostOperatorStatus(hostOperatorStatusNotReady(defaultHostOperatorDeploymentName, "DeploymentNotFound", "unable to get the deployment: deployments.apps \"host-operator-controller-manager\" not found")).
				HasMemberClusterStatus(memberCluster("member-1", ready()), memberCluster("member-2", ready())).
				HasRegistrationServiceStatus(registrationServiceReady()).
				HasHostRoutesStatus("https://api-toolchain-host-operator.host-cluster", hostRoutesAvailable())
		})

		t.Run("Host operator deployment not ready", func(t *testing.T) {
			// given
			hostOperatorDeployment := newDeploymentWithConditions(defaultHostOperatorDeploymentName, status.DeploymentNotAvailableCondition(), status.DeploymentProgressingCondition())
			reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), []string{"member-1", "member-2"},
				hostOperatorDeployment, memberStatus, registrationServiceDeployment, toolchainStatus, proxyRoute())

			// when
			res, err := reconciler.Reconcile(context.TODO(), req)

			// then
			require.NoError(t, err)
			assert.Equal(t, requeueResult, res)
			AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
				HasConditions(componentsNotReady(string(hostOperatorTag))).
				HasHostOperatorStatus(hostOperatorStatusNotReady(defaultHostOperatorDeploymentName, "DeploymentNotReady", "deployment has unready status conditions: Available")).
				HasMemberClusterStatus(memberCluster("member-1", ready()), memberCluster("member-2", ready())).
				HasRegistrationServiceStatus(registrationServiceReady()).
				HasHostRoutesStatus("https://api-toolchain-host-operator.host-cluster", hostRoutesAvailable())
		})

		t.Run("Host operator deployment not progressing", func(t *testing.T) {
			// given
			hostOperatorDeployment := newDeploymentWithConditions(defaultHostOperatorDeploymentName, status.DeploymentAvailableCondition(), status.DeploymentNotProgressingCondition())
			reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), []string{"member-1", "member-2"},
				hostOperatorDeployment, memberStatus, registrationServiceDeployment, toolchainStatus, proxyRoute())

			// when
			res, err := reconciler.Reconcile(context.TODO(), req)

			// then
			require.NoError(t, err)
			assert.Equal(t, requeueResult, res)
			AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
				HasConditions(componentsNotReady(string(hostOperatorTag))).
				HasHostOperatorStatus(hostOperatorStatusNotReady(defaultHostOperatorDeploymentName, "DeploymentNotReady", "deployment has unready status conditions: Progressing")).
				HasMemberClusterStatus(memberCluster("member-1", ready()), memberCluster("member-2", ready())).
				HasRegistrationServiceStatus(registrationServiceReady()).
				HasHostRoutesStatus("https://api-toolchain-host-operator.host-cluster", hostRoutesAvailable())
		})
	})

	t.Run("RegistrationService deployment tests", func(t *testing.T) {
		toolchainStatus := NewToolchainStatus()
		memberStatus := newMemberStatus(ready())
		hostOperatorDeployment := newDeploymentWithConditions(defaultHostOperatorDeploymentName, status.DeploymentAvailableCondition(), status.DeploymentProgressingCondition())

		t.Run("Registration service deployment not found", func(t *testing.T) {
			// given
			reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), []string{"member-1", "member-2"},
				hostOperatorDeployment, memberStatus, toolchainStatus, proxyRoute())

			// when
			res, err := reconciler.Reconcile(context.TODO(), req)

			// then
			require.NoError(t, err)
			assert.Equal(t, requeueResult, res)
			AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
				HasConditions(componentsNotReady(string(registrationServiceTag))).
				HasHostOperatorStatus(hostOperatorStatusReady()).
				HasMemberClusterStatus(memberCluster("member-1", ready()), memberCluster("member-2", ready())).
				HasHostRoutesStatus("https://api-toolchain-host-operator.host-cluster", hostRoutesAvailable()).
				HasRegistrationServiceStatus(registrationServiceDeploymentNotReady("DeploymentNotFound", "unable to get the deployment: deployments.apps \"registration-service\" not found"))
		})

		t.Run("Registration service deployment not ready", func(t *testing.T) {
			// given
			registrationServiceDeployment := newDeploymentWithConditions(registrationservice.ResourceName, status.DeploymentNotAvailableCondition(), status.DeploymentProgressingCondition())
			reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), []string{"member-1", "member-2"},
				hostOperatorDeployment, memberStatus, registrationServiceDeployment, toolchainStatus, proxyRoute())

			// when
			res, err := reconciler.Reconcile(context.TODO(), req)

			// then
			require.NoError(t, err)
			assert.Equal(t, requeueResult, res)
			AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
				HasConditions(componentsNotReady(string(registrationServiceTag))).
				HasHostOperatorStatus(hostOperatorStatusReady()).
				HasMemberClusterStatus(memberCluster("member-1", ready()), memberCluster("member-2", ready())).
				HasHostRoutesStatus("https://api-toolchain-host-operator.host-cluster", hostRoutesAvailable()).
				HasRegistrationServiceStatus(registrationServiceDeploymentNotReady("DeploymentNotReady", "deployment has unready status conditions: Available"))
		})

		t.Run("Registration service deployment not progressing", func(t *testing.T) {
			// given
			registrationServiceDeployment := newDeploymentWithConditions(registrationservice.ResourceName, status.DeploymentAvailableCondition(), status.DeploymentNotProgressingCondition())
			reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), []string{"member-1", "member-2"},
				hostOperatorDeployment, memberStatus, registrationServiceDeployment, toolchainStatus, proxyRoute())

			// when
			res, err := reconciler.Reconcile(context.TODO(), req)

			// then
			require.NoError(t, err)
			assert.Equal(t, requeueResult, res)
			AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
				HasConditions(componentsNotReady(string(registrationServiceTag))).
				HasHostOperatorStatus(hostOperatorStatusReady()).
				HasMemberClusterStatus(memberCluster("member-1", ready()), memberCluster("member-2", ready())).
				HasHostRoutesStatus("https://api-toolchain-host-operator.host-cluster", hostRoutesAvailable()).
				HasRegistrationServiceStatus(registrationServiceDeploymentNotReady("DeploymentNotReady", "deployment has unready status conditions: Progressing"))
		})
	})

	t.Run("RegistrationService health tests", func(t *testing.T) {
		hostOperatorDeployment := newDeploymentWithConditions(defaultHostOperatorDeploymentName, status.DeploymentAvailableCondition(), status.DeploymentProgressingCondition())
		registrationServiceDeployment := newDeploymentWithConditions(registrationservice.ResourceName, status.DeploymentAvailableCondition(), status.DeploymentProgressingCondition())
		memberStatus := newMemberStatus(ready())
		toolchainStatus := NewToolchainStatus()

		t.Run("Registration health endpoint - http client error", func(t *testing.T) {
			// given
			reconciler, req, fakeClient := prepareReconcile(t, requestName, httpClientError(), []string{"member-1", "member-2"},
				hostOperatorDeployment, registrationServiceDeployment, memberStatus, toolchainStatus, proxyRoute())

			// when
			res, err := reconciler.Reconcile(context.TODO(), req)

			// then
			require.NoError(t, err)
			assert.Equal(t, requeueResult, res)
			AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
				HasConditions(componentsNotReady(string(registrationServiceTag))).
				HasHostOperatorStatus(hostOperatorStatusReady()).
				HasMemberClusterStatus(memberCluster("member-1", ready()), memberCluster("member-2", ready())).
				HasRegistrationServiceStatus(registrationServiceHealthNotReady("http client error")).
				HasHostRoutesStatus("https://api-toolchain-host-operator.host-cluster", hostRoutesAvailable())
		})

		t.Run("Registration health endpoint - bad status code", func(t *testing.T) {
			// given
			reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseBadCode(), []string{"member-1", "member-2"},
				hostOperatorDeployment, registrationServiceDeployment, memberStatus, toolchainStatus, proxyRoute())

			// when
			res, err := reconciler.Reconcile(context.TODO(), req)

			// then
			require.NoError(t, err)
			assert.Equal(t, requeueResult, res)
			AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
				HasConditions(componentsNotReady(string(registrationServiceTag))).
				HasHostOperatorStatus(hostOperatorStatusReady()).
				HasMemberClusterStatus(memberCluster("member-1", ready()), memberCluster("member-2", ready())).
				HasRegistrationServiceStatus(registrationServiceHealthNotReady("bad response from http://registration-service/api/v1/health : statusCode=500")).
				HasHostRoutesStatus("https://api-toolchain-host-operator.host-cluster", hostRoutesAvailable())
		})

		t.Run("Registration health endpoint - invalid JSON", func(t *testing.T) {
			// given
			reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseInvalid(), []string{"member-1", "member-2"},
				hostOperatorDeployment, memberStatus, registrationServiceDeployment, toolchainStatus, proxyRoute())

			// when
			res, err := reconciler.Reconcile(context.TODO(), req)

			// then
			require.NoError(t, err)
			assert.Equal(t, requeueResult, res)
			AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
				HasConditions(componentsNotReady(string(registrationServiceTag))).
				HasHostOperatorStatus(hostOperatorStatusReady()).
				HasMemberClusterStatus(memberCluster("member-1", ready()), memberCluster("member-2", ready())).
				HasRegistrationServiceStatus(registrationServiceHealthNotReady("invalid character '}' after object key")).
				HasHostRoutesStatus("https://api-toolchain-host-operator.host-cluster", hostRoutesAvailable())
		})

		t.Run("Registration health endpoint - not alive", func(t *testing.T) {
			// given
			reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseBodyNotAlive(), []string{"member-1", "member-2"},
				hostOperatorDeployment, memberStatus, registrationServiceDeployment, toolchainStatus, proxyRoute())

			// when
			res, err := reconciler.Reconcile(context.TODO(), req)

			// then
			require.NoError(t, err)
			assert.Equal(t, requeueResult, res)
			AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
				HasConditions(componentsNotReady(string(registrationServiceTag))).
				HasHostOperatorStatus(hostOperatorStatusReady()).
				HasMemberClusterStatus(memberCluster("member-1", ready()), memberCluster("member-2", ready())).
				HasRegistrationServiceStatus(registrationServiceHealthNotReady("the registration service health endpoint is reporting an unhealthy status")).
				HasHostRoutesStatus("https://api-toolchain-host-operator.host-cluster", hostRoutesAvailable())
		})
	})

	t.Run("Proxy status tests", func(t *testing.T) {
		// given
		hostOperatorDeployment := newDeploymentWithConditions(defaultHostOperatorDeploymentName, status.DeploymentAvailableCondition(), status.DeploymentProgressingCondition())
		registrationServiceDeployment := newDeploymentWithConditions(registrationservice.ResourceName, status.DeploymentAvailableCondition(), status.DeploymentProgressingCondition())
		memberStatus := newMemberStatus(ready())
		toolchainStatus := NewToolchainStatus()

		t.Run("proxy route not found", func(t *testing.T) {
			// given
			reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), []string{"member-1", "member-2"},
				hostOperatorDeployment, registrationServiceDeployment, memberStatus, toolchainStatus)

			// when
			res, err := reconciler.Reconcile(context.TODO(), req)

			// then
			require.NoError(t, err)
			assert.Equal(t, requeueResult, res)
			AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
				HasConditions(componentsNotReady(string(hostRoutesTag))).
				HasHostOperatorStatus(hostOperatorStatusReady()).
				HasMemberClusterStatus(memberCluster("member-1", ready()), memberCluster("member-2", ready())).
				HasRegistrationServiceStatus(registrationServiceReady()).
				HasHostRoutesStatus("", proxyRouteUnavailable("routes.route.openshift.io \"api\" not found"))
		})

		t.Run("proxy without tls and with path", func(t *testing.T) {
			// given
			route := proxyRoute()
			route.Spec.TLS = nil
			route.Spec.Path = "/api"
			reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), []string{"member-1", "member-2"},
				hostOperatorDeployment, memberStatus, registrationServiceDeployment, toolchainStatus, route)

			// when
			res, err := reconciler.Reconcile(context.TODO(), req)

			// then
			require.NoError(t, err)
			assert.Equal(t, requeueResult, res)
			AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
				HasConditions(componentsReady(), unreadyNotificationNotCreated()).
				HasHostOperatorStatus(hostOperatorStatusReady()).
				HasMemberClusterStatus(memberCluster("member-1", ready()), memberCluster("member-2", ready())).
				HasRegistrationServiceStatus(registrationServiceReady()).
				HasHostRoutesStatus("http://api-toolchain-host-operator.host-cluster/api", hostRoutesAvailable())
		})
	})

	t.Run("MemberStatus tests", func(t *testing.T) {
		hostOperatorDeployment := newDeploymentWithConditions(defaultHostOperatorDeploymentName, status.DeploymentAvailableCondition(), status.DeploymentProgressingCondition())
		registrationServiceDeployment := newDeploymentWithConditions(registrationservice.ResourceName, status.DeploymentAvailableCondition(), status.DeploymentProgressingCondition())

		t.Run("MemberStatus not found", func(t *testing.T) {
			// given
			emptyToolchainStatus := NewToolchainStatus()
			reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), []string{},
				hostOperatorDeployment, registrationServiceDeployment, emptyToolchainStatus, proxyRoute())

			// when
			res, err := reconciler.Reconcile(context.TODO(), req)

			// then
			require.NoError(t, err)
			assert.Equal(t, requeueResult, res)
			AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
				HasConditions(componentsNotReady(string(memberConnectionsTag))).Exists().
				HasHostOperatorStatus(hostOperatorStatusReady()).
				HasMemberClusterStatus().
				HasRegistrationServiceStatus(registrationServiceReady()).
				HasHostRoutesStatus("https://api-toolchain-host-operator.host-cluster", hostRoutesAvailable())
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
				WithMember("member-1", WithSpaceCount(10)),
				WithMember("member-2", WithSpaceCount(10)),
			)
			reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), []string{},
				hostOperatorDeployment, memberStatus, registrationServiceDeployment, toolchainStatus, proxyRoute())
			InitializeCounters(t, toolchainStatus)

			// when
			res, err := reconciler.Reconcile(context.TODO(), req)

			// then
			require.NoError(t, err)
			assert.Equal(t, requeueResult, res)
			AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
				HasConditions(componentsNotReady(string(memberConnectionsTag))).
				HasHostOperatorStatus(hostOperatorStatusReady()).
				HasMemberClusterStatus(
					memberCluster("member-1", spaceCount(10), noResourceUsage(), notReady("MemberToolchainClusterMissing", "ToolchainCluster CR wasn't found for member cluster `member-1` that was previously registered in the host")),
					memberCluster("member-2", spaceCount(10), noResourceUsage(), notReady("MemberToolchainClusterMissing", "ToolchainCluster CR wasn't found for member cluster `member-2` that was previously registered in the host")),
				).
				HasRegistrationServiceStatus(registrationServiceReady()).
				HasHostRoutesStatus("https://api-toolchain-host-operator.host-cluster", hostRoutesAvailable())
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
				memberCluster("member-1", ready(), spaceCount(10)),
				memberCluster("member-2", ready(), spaceCount(10)),
			}
			reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), []string{"member-2"},
				hostOperatorDeployment, memberStatus, registrationServiceDeployment, toolchainStatus, proxyRoute())
			InitializeCounters(t, toolchainStatus)

			// when
			res, err := reconciler.Reconcile(context.TODO(), req)

			// then
			require.NoError(t, err)
			assert.Equal(t, requeueResult, res)
			AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
				HasConditions(componentsNotReady(string(memberConnectionsTag))).
				HasHostOperatorStatus(hostOperatorStatusReady()).
				HasMemberClusterStatus(
					memberCluster("member-1", spaceCount(10), noResourceUsage(), notReady("MemberToolchainClusterMissing", "ToolchainCluster CR wasn't found for member cluster `member-1` that was previously registered in the host")),
					memberCluster("member-2", spaceCount(10), ready()),
				).
				HasRegistrationServiceStatus(registrationServiceReady()).
				HasHostRoutesStatus("https://api-toolchain-host-operator.host-cluster", hostRoutesAvailable())
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
				memberCluster("member-1", ready(), spaceCount(10)),
				memberCluster("member-2", ready(), spaceCount(10)),
			}
			reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), []string{"member-1"},
				hostOperatorDeployment, memberStatus, registrationServiceDeployment, toolchainStatus, proxyRoute())
			InitializeCounters(t, toolchainStatus)

			// when
			res, err := reconciler.Reconcile(context.TODO(), req)

			// then
			require.NoError(t, err)
			assert.Equal(t, requeueResult, res)
			AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
				HasConditions(componentsNotReady(string(memberConnectionsTag))).
				HasHostOperatorStatus(hostOperatorStatusReady()).
				HasMemberClusterStatus(
					memberCluster("member-1", spaceCount(10), ready()),
					memberCluster("member-2", spaceCount(10), noResourceUsage(), notReady("MemberToolchainClusterMissing", "ToolchainCluster CR wasn't found for member cluster `member-2` that was previously registered in the host")),
				).
				HasRegistrationServiceStatus(registrationServiceReady()).
				HasHostRoutesStatus("https://api-toolchain-host-operator.host-cluster", hostRoutesAvailable())
		})

		t.Run("MemberStatus saying that there was no member cluster present should be removed", func(t *testing.T) {
			// given
			memberStatus := newMemberStatus(ready())
			toolchainStatus := NewToolchainStatus()
			toolchainStatus.Status.Members = []toolchainv1alpha1.Member{
				memberCluster("member-1", spaceCount(10), notReady("NoMemberClustersFound", "no member clusters found")),
			}
			reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), []string{"member-1", "member-2"},
				hostOperatorDeployment, memberStatus, registrationServiceDeployment, toolchainStatus, proxyRoute())
			InitializeCounters(t, toolchainStatus)

			// when
			res, err := reconciler.Reconcile(context.TODO(), req)

			// then
			require.NoError(t, err)
			assert.Equal(t, requeueResult, res)
			AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
				HasConditions(componentsReady(), unreadyNotificationNotCreated()).
				HasHostOperatorStatus(hostOperatorStatusReady()).
				HasMemberClusterStatus(memberCluster("member-1", ready()), memberCluster("member-2", ready())).
				HasRegistrationServiceStatus(registrationServiceReady()).
				HasHostRoutesStatus("https://api-toolchain-host-operator.host-cluster", hostRoutesAvailable())
		})

		t.Run("MemberStatus not found", func(t *testing.T) {
			// given
			emptyToolchainStatus := NewToolchainStatus()
			reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), []string{"member-1", "member-2"},
				hostOperatorDeployment, registrationServiceDeployment, emptyToolchainStatus, proxyRoute())
			InitializeCounters(t, emptyToolchainStatus)

			// when
			res, err := reconciler.Reconcile(context.TODO(), req)

			// then
			require.NoError(t, err)
			assert.Equal(t, requeueResult, res)
			AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
				HasConditions(componentsNotReady(string(memberConnectionsTag))).
				HasHostOperatorStatus(hostOperatorStatusReady()).
				HasMemberClusterStatus(
					memberCluster("member-1", noResourceUsage(), spaceCount(0), notReady("MemberStatusNotFound", "memberstatuses.toolchain.dev.openshift.com \"toolchain-member-status\" not found")),
					memberCluster("member-2", noResourceUsage(), spaceCount(0), notReady("MemberStatusNotFound", "memberstatuses.toolchain.dev.openshift.com \"toolchain-member-status\" not found")),
				).
				HasRegistrationServiceStatus(registrationServiceReady()).
				HasHostRoutesStatus("https://api-toolchain-host-operator.host-cluster", hostRoutesAvailable())
		})

		t.Run("MemberStatus not ready", func(t *testing.T) {
			// given
			emptyToolchainStatus := NewToolchainStatus()
			memberStatus := newMemberStatus(notReady(toolchainv1alpha1.ToolchainStatusComponentsNotReadyReason, "components not ready: [memberOperator]"))
			reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), []string{"member-1", "member-2"},
				hostOperatorDeployment, memberStatus, registrationServiceDeployment, emptyToolchainStatus, proxyRoute())
			InitializeCounters(t, emptyToolchainStatus)

			// when
			res, err := reconciler.Reconcile(context.TODO(), req)

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
				HasRegistrationServiceStatus(registrationServiceReady()).
				HasHostRoutesStatus("https://api-toolchain-host-operator.host-cluster", hostRoutesAvailable())
		})

		t.Run("synchronization with the counter fails", func(t *testing.T) {
			// given
			emptyToolchainStatus := NewToolchainStatus()
			memberStatus := newMemberStatus(ready())
			reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), []string{"member-1", "member-2"},
				hostOperatorDeployment, memberStatus, registrationServiceDeployment, emptyToolchainStatus, proxyRoute())
			fakeClient.MockList = func(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
				return fmt.Errorf("some error")
			}

			// when
			res, err := reconciler.Reconcile(context.TODO(), req)

			// then
			require.NoError(t, err)
			assert.Equal(t, requeueResult, res)
			AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
				HasConditions(componentsNotReady(string(counterTag))).
				HasHostOperatorStatus(hostOperatorStatusReady()).
				HasMemberClusterStatus(memberCluster("member-1", ready()), memberCluster("member-2", ready())).
				HasRegistrationServiceStatus(registrationServiceReady()).
				HasHostRoutesStatus("https://api-toolchain-host-operator.host-cluster", hostRoutesAvailable())
		})

		t.Run("MemberStatus not ready is changed to ready", func(t *testing.T) {
			// given
			memberStatus := newMemberStatus(ready())
			toolchainStatus := NewToolchainStatus()
			toolchainStatus.Status.Members = []toolchainv1alpha1.Member{
				memberCluster("member-1", notReady("ComponentsNotReady", "some cool error")),
				memberCluster("member-2", ready()),
			}
			reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), []string{"member-1", "member-2"},
				hostOperatorDeployment, memberStatus, registrationServiceDeployment, toolchainStatus, proxyRoute())
			InitializeCounters(t, toolchainStatus)

			// when
			res, err := reconciler.Reconcile(context.TODO(), req)

			// then
			require.NoError(t, err)
			assert.Equal(t, requeueResult, res)
			AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
				HasConditions(componentsReady(), unreadyNotificationNotCreated()).
				HasHostOperatorStatus(hostOperatorStatusReady()).
				HasMemberClusterStatus(memberCluster("member-1", ready()), memberCluster("member-2", ready())).
				HasRegistrationServiceStatus(registrationServiceReady()).
				HasHostRoutesStatus("https://api-toolchain-host-operator.host-cluster", hostRoutesAvailable())
		})

		t.Run("MemberStatus with no condition", func(t *testing.T) {
			// given
			memberStatus := newMemberStatus()
			toolchainStatus := NewToolchainStatus()
			reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), []string{"member-1", "member-2"},
				hostOperatorDeployment, memberStatus, registrationServiceDeployment, toolchainStatus, proxyRoute())
			InitializeCounters(t, toolchainStatus)

			// when
			res, err := reconciler.Reconcile(context.TODO(), req)

			// then
			require.NoError(t, err)
			assert.Equal(t, requeueResult, res)
			AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
				HasConditions(componentsNotReady(string(memberConnectionsTag))).
				HasHostOperatorStatus(hostOperatorStatusReady()).
				HasMemberClusterStatus(memberCluster("member-1"), memberCluster("member-2")).
				HasRegistrationServiceStatus(registrationServiceReady()).
				HasHostRoutesStatus("https://api-toolchain-host-operator.host-cluster", hostRoutesAvailable())
		})

		t.Run("All components ready but one member is missing", func(t *testing.T) {
			// given
			hostOperatorDeployment := newDeploymentWithConditions(defaultHostOperatorDeploymentName, status.DeploymentAvailableCondition(), status.DeploymentProgressingCondition())
			registrationServiceDeployment := newDeploymentWithConditions(registrationservice.ResourceName, status.DeploymentAvailableCondition(), status.DeploymentProgressingCondition())
			memberStatus := newMemberStatus(ready())
			toolchainStatus := NewToolchainStatus()

			t.Run("with non-zero user accounts", func(t *testing.T) {
				// given
				toolchainStatus.Status.Members = []toolchainv1alpha1.Member{
					// member-1 and member-2 will be added since there are MemberStatus resources for each one of them
					memberCluster("member-3", ready(), spaceCount(10)), // will move to `NotReady` since there is no CachedToolchainCluster for this member
				}
				reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), []string{"member-1", "member-2"},
					hostOperatorDeployment, memberStatus, registrationServiceDeployment, toolchainStatus, proxyRoute())
				InitializeCounters(t, toolchainStatus)

				// when
				res, err := reconciler.Reconcile(context.TODO(), req)

				// then
				require.NoError(t, err)
				assert.Equal(t, requeueResult, res)
				AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
					HasConditions(componentsNotReady(string(memberConnectionsTag))).
					HasHostOperatorStatus(hostOperatorStatusReady()).
					HasMemberClusterStatus(
						memberCluster("member-1", ready()),
						memberCluster("member-2", ready()),
						memberCluster("member-3", noResourceUsage(), spaceCount(0), notReady("MemberToolchainClusterMissing", "ToolchainCluster CR wasn't found for member cluster `member-3` that was previously registered in the host")),
					).
					HasRegistrationServiceStatus(registrationServiceReady()).
					HasHostRoutesStatus("https://api-toolchain-host-operator.host-cluster", hostRoutesAvailable())
			})

			t.Run("with zero user accounts", func(t *testing.T) {
				// given
				toolchainStatus.Status.Members = []toolchainv1alpha1.Member{
					memberCluster("removed-cluster", ready()), // will move to `NotReady` since there is no MemberStatus for this cluster
				}
				reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), []string{"member-1", "member-2"},
					hostOperatorDeployment, memberStatus, registrationServiceDeployment, toolchainStatus, proxyRoute())
				InitializeCounters(t, toolchainStatus)

				// when
				res, err := reconciler.Reconcile(context.TODO(), req)

				// then
				require.NoError(t, err)
				assert.Equal(t, requeueResult, res)
				AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
					HasConditions(componentsReady(), unreadyNotificationNotCreated()).
					HasHostOperatorStatus(hostOperatorStatusReady()).
					HasMemberClusterStatus(memberCluster("member-1", ready()), memberCluster("member-2", ready())).
					HasRegistrationServiceStatus(registrationServiceReady()).
					HasHostRoutesStatus("https://api-toolchain-host-operator.host-cluster", hostRoutesAvailable())
			})
		})
	})
}

func TestToolchainStatusReadyConditionTimestamps(t *testing.T) {
	// set the operator name environment variable for all the tests which is used to get the host operator deployment name
	restore := test.SetEnvVarsAndRestore(t, test.Env(commonconfig.OperatorNameEnvVar, defaultHostOperatorName))
	defer restore()
	requestName := toolchainconfig.ToolchainStatusName

	toolchainStatus := NewToolchainStatus()
	memberStatus := newMemberStatus(ready())
	registrationServiceDeployment := newDeploymentWithConditions(registrationservice.ResourceName,
		status.DeploymentAvailableCondition(), status.DeploymentProgressingCondition())
	hostOperatorDeployment := newDeploymentWithConditions(defaultHostOperatorDeploymentName,
		status.DeploymentAvailableCondition())

	t.Run("Timestamp set for new status object", func(t *testing.T) {
		// given a status with unknown (new) ready condition
		defer counter.Reset()

		reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(),
			[]string{"member-1", "member-2"},
			hostOperatorDeployment, memberStatus, registrationServiceDeployment, toolchainStatus, proxyRoute())

		// when
		_, err := reconciler.Reconcile(context.TODO(), req)

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
			hostOperatorDeployment, memberStatus, registrationServiceDeployment, toolchainStatus, proxyRoute())

		// when no ready condition changed
		_, err := reconciler.Reconcile(context.TODO(), req)

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
		hostOperatorDeployment := newDeploymentWithConditions(defaultHostOperatorDeploymentName,
			status.DeploymentNotAvailableCondition(), status.DeploymentProgressingCondition())
		reconciler, req, fakeClient := prepareReconcileWithStatusConditions(t, requestName,
			[]string{"member-1", "member-2"},
			conditions,
			hostOperatorDeployment, memberStatus, registrationServiceDeployment, toolchainStatus, proxyRoute())

		// when the ready condition becomes not-ready
		_, err := reconciler.Reconcile(context.TODO(), req)

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
	restore := test.SetEnvVarsAndRestore(t, test.Env(commonconfig.OperatorNameEnvVar, defaultHostOperatorName))
	defer restore()
	defer counter.Reset()
	requestName := toolchainconfig.ToolchainStatusName

	toolchainStatus := NewToolchainStatus()
	memberStatus := newMemberStatus(ready())
	registrationServiceDeployment := newDeploymentWithConditions(registrationservice.ResourceName,
		status.DeploymentAvailableCondition(), status.DeploymentProgressingCondition())

	t.Run("Notification workflow", func(t *testing.T) {
		// given
		hostOperatorDeployment := newDeploymentWithConditions(defaultHostOperatorDeploymentName,
			status.DeploymentAvailableCondition())

		os.Setenv("WATCH_NAMESPACE", test.HostOperatorNs)

		toolchainConfig := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.Notifications().AdminEmail("admin@dev.sandbox.com"))

		reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), []string{"member-1", "member-2"},
			hostOperatorDeployment, memberStatus, registrationServiceDeployment, toolchainStatus, toolchainConfig, proxyRoute())

		// when
		res, err := reconciler.Reconcile(context.TODO(), req)

		// then
		require.NoError(t, err)
		assert.Equal(t, requeueResult, res)

		AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
			HasConditions(componentsReady(), unreadyNotificationNotCreated()).
			HasHostOperatorStatus(hostOperatorStatusReady()).
			HasMemberClusterStatus(memberCluster("member-1", ready()), memberCluster("member-2", ready())).
			HasRegistrationServiceStatus(registrationServiceReady()).
			HasHostRoutesStatus("https://api-toolchain-host-operator.host-cluster", hostRoutesAvailable())

		// Confirm there is no notification
		assertToolchainStatusNotificationNotCreated(t, fakeClient, unreadyStatusNotification)
		assertToolchainStatusNotificationNotCreated(t, fakeClient, restoredStatusNotification)

		t.Run("Notification not created when host operator deployment not ready within threshold", func(t *testing.T) {
			// given
			hostOperatorDeployment := newDeploymentWithConditions(defaultHostOperatorDeploymentName,
				status.DeploymentNotAvailableCondition(), status.DeploymentProgressingCondition())

			reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), []string{"member-1", "member-2"},
				hostOperatorDeployment, memberStatus, registrationServiceDeployment, toolchainStatus, toolchainConfig, proxyRoute())

			// when
			res, err := reconciler.Reconcile(context.TODO(), req)

			// then
			require.NoError(t, err)
			assert.Equal(t, requeueResult, res)

			// Confirm there is no notification
			assertToolchainStatusNotificationNotCreated(t, fakeClient, unreadyStatusNotification)
			assertToolchainStatusNotificationNotCreated(t, fakeClient, restoredStatusNotification)

			t.Run("Notification not created when admin.email not configured", func(t *testing.T) {

				assertInvalidEmailReturnErr := func(email string) {
					commonconfig.ResetCache() // clear the config cache so that this invalid config will be picked up
					invalidConfig := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.Notifications().AdminEmail(email))

					// given
					hostOperatorDeployment := newDeploymentWithConditions(defaultHostOperatorDeploymentName,
						status.DeploymentNotAvailableCondition(), status.DeploymentProgressingCondition())

					// Reload the toolchain status
					require.NoError(t, fakeClient.Get(context.Background(), test.NamespacedName(test.HostOperatorNs,
						toolchainStatus.Name), toolchainStatus))

					overrideLastTransitionTime(t, toolchainStatus, metav1.Time{Time: time.Now().Add(-time.Duration(24) * time.Hour)})

					reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), []string{"member-1", "member-2"},
						hostOperatorDeployment, memberStatus, registrationServiceDeployment, toolchainStatus, invalidConfig, proxyRoute())

					// when
					res, err := reconciler.Reconcile(context.TODO(), req)

					// then
					require.Error(t, err)
					require.Equal(t, fmt.Sprintf("Failed to create toolchain status unready notification: cannot create notification "+
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
				hostOperatorDeployment := newDeploymentWithConditions(defaultHostOperatorDeploymentName,
					status.DeploymentNotAvailableCondition(), status.DeploymentProgressingCondition())

				// Reload the toolchain status
				require.NoError(t, fakeClient.Get(context.Background(), test.NamespacedName(test.HostOperatorNs,
					toolchainStatus.Name), toolchainStatus))

				overrideLastTransitionTime(t, toolchainStatus, metav1.Time{Time: time.Now().Add(-time.Duration(24) * time.Hour)})

				reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), []string{"member-1", "member-2"},
					hostOperatorDeployment, memberStatus, registrationServiceDeployment, toolchainStatus, toolchainConfig, proxyRoute())

				// when
				res, err := reconciler.Reconcile(context.TODO(), req)

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
					hostOperatorDeployment := newDeploymentWithConditions(defaultHostOperatorDeploymentName,
						status.DeploymentAvailableCondition())

					// Reload the toolchain status
					require.NoError(t, fakeClient.Get(context.Background(), test.NamespacedName(test.HostOperatorNs,
						toolchainStatus.Name), toolchainStatus))

					reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), []string{"member-1", "member-2"},
						hostOperatorDeployment, memberStatus, registrationServiceDeployment, toolchainStatus, proxyRoute())

					// when
					res, err := reconciler.Reconcile(context.TODO(), req)

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
						hostOperatorDeployment := newDeploymentWithConditions(defaultHostOperatorDeploymentName,
							status.DeploymentNotAvailableCondition(), status.DeploymentProgressingCondition())

						// Reload the toolchain status
						require.NoError(t, fakeClient.Get(context.Background(), test.NamespacedName(test.HostOperatorNs,
							toolchainStatus.Name), toolchainStatus))

						// Reconcile in order to update the ready status to false
						reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), []string{"member-1", "member-2"},
							hostOperatorDeployment, memberStatus, registrationServiceDeployment, toolchainStatus, toolchainConfig, proxyRoute())

						// when
						_, err := reconciler.Reconcile(context.TODO(), req)

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
						reconciler, req, fakeClient = prepareReconcile(t, requestName, newResponseGood(), []string{"member-1", "member-2"},
							hostOperatorDeployment, memberStatus, registrationServiceDeployment, toolchainStatus, toolchainConfig, proxyRoute())

						// when
						res, err = reconciler.Reconcile(context.TODO(), req)

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
						assert.True(t, strings.HasPrefix(notification.Spec.Content, "<h3>The following issues"))
						assert.True(t, strings.HasSuffix(strings.TrimSpace(notification.Spec.Content), "</div>"))
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
	restore := test.SetEnvVarsAndRestore(t, test.Env(commonconfig.OperatorNameEnvVar, defaultHostOperatorName))
	defer restore()
	requestName := toolchainconfig.ToolchainStatusName
	hostOperatorDeployment := newDeploymentWithConditions(defaultHostOperatorDeploymentName, status.DeploymentAvailableCondition(), status.DeploymentProgressingCondition())
	registrationServiceDeployment := newDeploymentWithConditions(registrationservice.ResourceName, status.DeploymentAvailableCondition(), status.DeploymentProgressingCondition())
	memberStatus := newMemberStatus(ready())

	t.Run("Load all current resources", func(t *testing.T) {
		// given
		defer counter.Reset()
		toolchainStatus := NewToolchainStatus()
		initObjects := append([]runtime.Object{}, hostOperatorDeployment, memberStatus, registrationServiceDeployment, toolchainStatus, proxyRoute())
		initObjects = append(initObjects, CreateMultipleUserSignups("cookie-", 8)...)
		initObjects = append(initObjects, CreateMultipleMurs(t, "cookie-", 8, "member-1")...)
		initObjects = append(initObjects, CreateMultipleSpaces("cookie-", 8, "member-1")...)
		initObjects = append(initObjects, CreateMultipleUserSignups("pasta-", 2)...)
		initObjects = append(initObjects, CreateMultipleMurs(t, "pasta-", 2, "member-2")...)
		initObjects = append(initObjects, CreateMultipleSpaces("pasta-", 2, "member-2")...)

		reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), []string{"member-1", "member-2"}, initObjects...)

		// when
		res, err := reconciler.Reconcile(context.TODO(), req)

		// then
		require.NoError(t, err)
		assert.Equal(t, requeueResult, res)
		AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
			HasConditions(componentsReady(), unreadyNotificationNotCreated()).
			HasHostOperatorStatus(hostOperatorStatusReady()).
			HasMemberClusterStatus(
				memberCluster("member-1", ready(), spaceCount(8)),
				memberCluster("member-2", ready(), spaceCount(2))).
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
			counter.IncrementSpaceCount(logger, "member-1")
			toolchainStatus := NewToolchainStatus()
			reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), []string{"member-1", "member-2"},
				hostOperatorDeployment, memberStatus, registrationServiceDeployment, toolchainStatus, proxyRoute())

			// when
			res, err := reconciler.Reconcile(context.TODO(), req)

			// then
			require.NoError(t, err)
			assert.Equal(t, requeueResult, res)
			AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
				HasConditions(componentsReady(), unreadyNotificationNotCreated()).
				HasHostOperatorStatus(hostOperatorStatusReady()).
				HasMemberClusterStatus(
					memberCluster("member-1", ready(), spaceCount(9)),
					memberCluster("member-2", ready(), spaceCount(2))).
				HasRegistrationServiceStatus(registrationServiceReady())
		})

	})

	t.Run("initialize the cache using the ToolchainStatus resource", func(t *testing.T) {
		// given
		defer counter.Reset()
		toolchainStatus := NewToolchainStatus(
			WithMember("member-1", WithSpaceCount(6)), // will increase
			WithMember("member-2", WithSpaceCount(2)), // will remain the same
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
		reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), []string{"member-1", "member-2"},
			hostOperatorDeployment, memberStatus, registrationServiceDeployment, toolchainStatus, proxyRoute())

		// when
		counter.IncrementMasterUserRecordCount(logger, metrics.Internal)
		counter.IncrementSpaceCount(logger, "member-1")
		counter.UpdateUsersPerActivationCounters(logger, 1, metrics.Internal)
		counter.UpdateUsersPerActivationCounters(logger, 2, metrics.Internal)
		res, err := reconciler.Reconcile(context.TODO(), req)

		// then
		require.NoError(t, err)
		assert.Equal(t, requeueResult, res)
		AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
			HasConditions(componentsReady(), unreadyNotificationNotCreated()).
			HasHostOperatorStatus(hostOperatorStatusReady()).
			HasMemberClusterStatus(
				memberCluster("member-1", ready(), spaceCount(7)), // was incremented
				memberCluster("member-2", ready(), spaceCount(2))).
			HasRegistrationServiceStatus(registrationServiceReady()).
			HasUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
				"1,internal": 4, // was incremented by `counter.UpdateUsersPerActivationCounters(1)` but decremented `counter.UpdateUsersPerActivationCounters(2)`
				"1,external": 1, // unchanged
				"2,internal": 2, // was incremented by `counter.UpdateUsersPerActivationCounters(2)`
				"2,external": 1, // unchanged
				"3,internal": 1, // unchanged
			})
		AssertThatCountersAndMetrics(t).
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

func TestExtractStatusMetadata(t *testing.T) {
	t.Run("test status metadata for ToolchainStatus not ready", func(t *testing.T) {
		// given
		toolchainStatus := NewToolchainStatus()
		toolchainStatus.Status.Conditions, _ = condition.AddOrUpdateStatusConditions(toolchainStatus.Status.Conditions, toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.ConditionReady,
			Status:  corev1.ConditionFalse,
			Reason:  "SomeReason",
			Message: "A message",
		})

		meta := ExtractStatusMetadata(toolchainStatus)
		require.Len(t, meta, 1)
		require.Equal(t, "", meta[0].ComponentType)
		require.Equal(t, "SomeReason", meta[0].Reason)
		require.Equal(t, "A message", meta[0].Message)
		require.Equal(t, "ToolchainStatus", meta[0].ComponentName)
	})

	t.Run("test status metadata for host operator not ready", func(t *testing.T) {
		// given
		toolchainStatus := NewToolchainStatus(WithHost())
		toolchainStatus.Status.HostOperator.Conditions, _ = condition.AddOrUpdateStatusConditions(
			toolchainStatus.Status.HostOperator.Conditions, toolchainv1alpha1.Condition{
				Type:    toolchainv1alpha1.ConditionReady,
				Status:  corev1.ConditionFalse,
				Reason:  "HostNotReadyReason",
				Message: "Host error message",
			})

		meta := ExtractStatusMetadata(toolchainStatus)
		require.Len(t, meta, 1)
		require.Equal(t, "", meta[0].ComponentType)
		require.Equal(t, "HostNotReadyReason", meta[0].Reason)
		require.Equal(t, "Host error message", meta[0].Message)
		require.Equal(t, "Host Operator", meta[0].ComponentName)
	})

	t.Run("test status metadata for one of two members not ready", func(t *testing.T) {
		// given
		toolchainStatus := NewToolchainStatus(
			WithMember("member-sandbox.aaa.openshiftapps.com",
				WithRoutes("http://console.url", "http://che.dashboard.url",
					toolchainv1alpha1.Condition{
						Type:    toolchainv1alpha1.ConditionReady,
						Status:  corev1.ConditionFalse,
						Reason:  "RoutesNotReadyReason",
						Message: "Member routes error message"})),
			WithMember("member-sandbox.bbb.openshiftapps.com"))
		toolchainStatus.Status.Members[0].MemberStatus.Conditions, _ = condition.AddOrUpdateStatusConditions(
			toolchainStatus.Status.Members[0].MemberStatus.Conditions, toolchainv1alpha1.Condition{
				Type:    toolchainv1alpha1.ConditionReady,
				Status:  corev1.ConditionFalse,
				Reason:  "MemberNotReadyReason",
				Message: "Member error message",
			})

		meta := ExtractStatusMetadata(toolchainStatus)
		require.Len(t, meta, 2)
		require.Equal(t, "Member", meta[0].ComponentType)
		require.Equal(t, "MemberNotReadyReason", meta[0].Reason)
		require.Equal(t, "Member error message", meta[0].Message)
		require.Equal(t, "member-sandbox.aaa.openshiftapps.com", meta[0].ComponentName)

		require.Equal(t, "Member Routes", meta[1].ComponentType)
		require.Equal(t, "member-sandbox.aaa.openshiftapps.com", meta[1].ComponentName)
		require.Equal(t, "RoutesNotReadyReason", meta[1].Reason)
		require.Equal(t, "Member routes error message", meta[1].Message)

		require.Equal(t, "http://che.dashboard.url", meta[1].Details["Che dashboard URL"])
		require.Equal(t, "http://console.url", meta[1].Details["Console URL"])
		require.Len(t, meta[1].Details, 2)
	})

	t.Run("test status metadata for member route not ready", func(t *testing.T) {
		// given
		toolchainStatus := NewToolchainStatus(WithMember("member-sandbox.ccc.openshiftapps.com",
			WithRoutes("https://console.url", "https://che.url",
				toolchainv1alpha1.Condition{
					Type:    toolchainv1alpha1.ConditionReady,
					Status:  corev1.ConditionFalse,
					Reason:  "RouteNotReadyReason",
					Message: "Route error message",
				})))

		meta := ExtractStatusMetadata(toolchainStatus)
		require.Len(t, meta, 1)
		require.Equal(t, "Member Routes", meta[0].ComponentType)
		require.Equal(t, "RouteNotReadyReason", meta[0].Reason)
		require.Equal(t, "Route error message", meta[0].Message)
		require.Equal(t, "member-sandbox.ccc.openshiftapps.com", meta[0].ComponentName)
	})

	t.Run("test status metadata for registration service deployment not ready", func(t *testing.T) {
		// given
		toolchainStatus := NewToolchainStatus(WithRegistrationService(WithDeploymentCondition(
			toolchainv1alpha1.Condition{
				Type:    toolchainv1alpha1.ConditionReady,
				Status:  corev1.ConditionFalse,
				Reason:  "DeploymentNotReadyReason",
				Message: "Deployment error message",
			})))

		meta := ExtractStatusMetadata(toolchainStatus)
		require.Len(t, meta, 1)
		require.Equal(t, "Registration service", meta[0].ComponentType)
		require.Equal(t, "DeploymentNotReadyReason", meta[0].Reason)
		require.Equal(t, "Deployment error message", meta[0].Message)
		require.Equal(t, "deployment", meta[0].ComponentName)
	})

	t.Run("test status metadata for registration service health not ready", func(t *testing.T) {
		// given
		toolchainStatus := NewToolchainStatus(WithRegistrationService(WithHealthCondition(
			toolchainv1alpha1.Condition{
				Type:    toolchainv1alpha1.ConditionReady,
				Status:  corev1.ConditionFalse,
				Reason:  "HealthNotReadyReason",
				Message: "Health error message",
			})))

		meta := ExtractStatusMetadata(toolchainStatus)
		require.Len(t, meta, 1)
		require.Equal(t, "Registration service", meta[0].ComponentType)
		require.Equal(t, "HealthNotReadyReason", meta[0].Reason)
		require.Equal(t, "Health error message", meta[0].Message)
		require.Equal(t, "health", meta[0].ComponentName)
	})
}

func TestGenerateUnreadyNotificationContent(t *testing.T) {
	t.Run("test generate notification content", func(t *testing.T) {
		// given
		toolchainStatus := NewToolchainStatus(
			WithHost(),
			WithMember("member-sandbox.ccc.openshiftapps.com",
				WithRoutes("https://console.url", "https://che.url",
					toolchainv1alpha1.Condition{
						Type:    toolchainv1alpha1.ConditionReady,
						Status:  corev1.ConditionFalse,
						Reason:  "RouteNotReadyReason",
						Message: "Route error message",
					})),
			WithRegistrationService(WithDeploymentCondition(
				toolchainv1alpha1.Condition{
					Type:    toolchainv1alpha1.ConditionReady,
					Status:  corev1.ConditionFalse,
					Reason:  "ResourcesNotReadyReason",
					Message: "Resources error message",
				}),
			))

		toolchainStatus.Status.HostOperator.Conditions, _ = condition.AddOrUpdateStatusConditions(
			toolchainStatus.Status.HostOperator.Conditions, toolchainv1alpha1.Condition{
				Type:    toolchainv1alpha1.ConditionReady,
				Status:  corev1.ConditionFalse,
				Reason:  "HostNotReadyReason",
				Message: "Host error message",
			})

		toolchainStatus.Status.Conditions, _ = condition.AddOrUpdateStatusConditions(toolchainStatus.Status.Conditions, toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.ConditionReady,
			Status:  corev1.ConditionFalse,
			Reason:  "SomeReason",
			Message: "A message",
		})

		meta := ExtractStatusMetadata(toolchainStatus)
		require.Len(t, meta, 4)
		content, err := GenerateUnreadyNotificationContent(ClusterURLs(toolchainStatus), meta)
		require.NoError(t, err)
		require.Len(t, content, 1512)
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
		Config: &cluster.Config{
			Name:              name,
			Type:              cluster.Host,
			OperatorNamespace: test.MemberOperatorNs,
			OwnerClusterName:  test.MemberClusterName,
			APIEndpoint:       "http://api.devcluster.openshift.com",
		},
		Client: cl,
		ClusterStatus: &toolchainv1alpha1.ToolchainClusterStatus{
			Conditions: []toolchainv1alpha1.ToolchainClusterCondition{{
				Type:          toolchainv1alpha1.ToolchainClusterReady,
				Status:        status,
				LastProbeTime: lastProbeTime,
			}},
		},
	}
}

func proxyRoute() *routev1.Route {
	return &routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "api",
			Namespace: test.HostOperatorNs,
		},
		Spec: routev1.RouteSpec{
			Host: fmt.Sprintf("api-%s.%s", test.HostOperatorNs, test.HostClusterName),
			Port: &routev1.RoutePort{
				TargetPort: intstr.FromInt(8081),
			},
			TLS: &routev1.TLSConfig{
				Termination: routev1.TLSTerminationEdge,
			},
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

func httpClientError() *fakeHTTPClient {
	return &fakeHTTPClient{
		err: fmt.Errorf("http client error"),
	}
}

func newResponseGood() *fakeHTTPClient {
	return &fakeHTTPClient{
		response: http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewReader([]byte(respBodyGood))),
		},
	}
}

func newResponseBadCode() *fakeHTTPClient {
	return &fakeHTTPClient{
		response: http.Response{
			StatusCode: 500,
			Body:       io.NopCloser(bytes.NewReader([]byte(""))),
		},
	}
}

func newResponseBodyNotAlive() *fakeHTTPClient {
	return &fakeHTTPClient{
		response: http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewReader([]byte(respBodyBad))),
		},
	}
}

func newResponseInvalid() *fakeHTTPClient {
	return &fakeHTTPClient{
		response: http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewReader([]byte(respBodyInvalid))),
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
		DeploymentName: defaultHostOperatorDeploymentName,
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

type spaceCount int

func (s spaceCount) applyToMember(m *toolchainv1alpha1.Member) {
	m.SpaceCount = int(s)
}

type memberClusterOption interface {
	applyToMember(*toolchainv1alpha1.Member)
}

func memberCluster(name string, options ...memberClusterOption) toolchainv1alpha1.Member {
	m := toolchainv1alpha1.Member{
		APIEndpoint: "http://api.devcluster.openshift.com",
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
		SpaceCount: 0,
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

	health := toolchainv1alpha1.Condition{
		Type:   toolchainv1alpha1.ConditionReady,
		Status: corev1.ConditionTrue,
		Reason: "RegServiceReady",
	}
	return registrationServiceStatus(deploy, health)
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

	health := toolchainv1alpha1.Condition{
		Type:   toolchainv1alpha1.ConditionReady,
		Status: corev1.ConditionTrue,
		Reason: "RegServiceReady",
	}
	return registrationServiceStatus(deploy, health)
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

	health := toolchainv1alpha1.Condition{
		Type:    toolchainv1alpha1.ConditionReady,
		Status:  corev1.ConditionFalse,
		Reason:  "RegServiceNotReady",
		Message: msg,
	}
	return registrationServiceStatus(deploy, health)
}

func registrationServiceStatus(deploy regTestDeployStatus, health toolchainv1alpha1.Condition) toolchainv1alpha1.HostRegistrationServiceStatus {
	return toolchainv1alpha1.HostRegistrationServiceStatus{
		Deployment: toolchainv1alpha1.RegistrationServiceDeploymentStatus{
			Name:       deploy.deploymentName,
			Conditions: []toolchainv1alpha1.Condition{deploy.condition},
		},
		Health: toolchainv1alpha1.RegistrationServiceHealth{
			Conditions: []toolchainv1alpha1.Condition{health},
		},
	}
}

func proxyRouteUnavailable(msg string) toolchainv1alpha1.Condition {
	return *status.NewComponentErrorCondition("ProxyRouteUnavailable", msg)
}

func hostRoutesAvailable() toolchainv1alpha1.Condition {
	return *status.NewComponentReadyCondition("HostRoutesAvailable")
}
