package toolchainstatus

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/codeready-toolchain/api/pkg/apis"
	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/configuration"
	"github.com/codeready-toolchain/host-operator/pkg/controller/registrationservice"
	"github.com/codeready-toolchain/host-operator/pkg/counter"
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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var requeueResult = reconcile.Result{RequeueAfter: defaultRequeueTime}

const (
	defaultHostOperatorName        = "host-operator"
	defaultRegistrationServiceName = "registration-service"
	defaultToolchainStatusName     = configuration.DefaultToolchainStatusName
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

func prepareReconcile(t *testing.T, requestName string, httpTestClient *fakeHTTPClient, getMemberClustersFunc func(fakeClient client.Client) cluster.GetMemberClustersFunc, initObjs ...runtime.Object) (*ReconcileToolchainStatus, reconcile.Request, *test.FakeClient) {
	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)
	fakeClient := test.NewFakeClient(t, initObjs...)
	hostConfig, err := configuration.LoadConfig(fakeClient)
	require.NoError(t, err)

	r := &ReconcileToolchainStatus{
		client:         fakeClient,
		httpClientImpl: httpTestClient,
		scheme:         s,
		getMembersFunc: getMemberClustersFunc(fakeClient),
		config:         hostConfig,
	}
	return r, reconcile.Request{NamespacedName: test.NamespacedName(test.HostOperatorNs, requestName)}, fakeClient
}

func TestNoToolchainStatusFound(t *testing.T) {
	t.Run("No toolchainstatus resource found", func(t *testing.T) {
		// given
		requestName := "bad-name"
		reconciler, req, _ := prepareReconcile(t, requestName, newResponseGood(), newGetMemberClustersFuncReady)

		// when
		res, err := reconciler.Reconcile(req)

		// then - there should not be any error, the controller should only log that the resource was not found
		require.NoError(t, err)
		assert.Equal(t, reconcile.Result{}, res)
	})

	t.Run("No toolchainstatus resource found - right name but not found", func(t *testing.T) {
		// given
		expectedErrMsg := "get failed"
		requestName := defaultToolchainStatusName
		reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), newGetMemberClustersFuncReady)
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
	restore := test.SetEnvVarsAndRestore(t, test.Env(k8sutil.OperatorNameEnvVar, defaultHostOperatorName))
	defer restore()
	requestName := defaultToolchainStatusName

	t.Run("All components ready", func(t *testing.T) {
		// given
		registrationService := newRegistrationServiceReady()
		hostOperatorDeployment := newDeploymentWithConditions(t, defaultHostOperatorName, status.DeploymentAvailableCondition(), status.DeploymentProgressingCondition())
		registrationServiceDeployment := newDeploymentWithConditions(t, registrationservice.ResourceName, status.DeploymentAvailableCondition(), status.DeploymentProgressingCondition())
		memberStatus := newMemberStatusReady()
		toolchainStatus := newToolchainStatus()
		reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), newGetMemberClustersFuncReady, hostOperatorDeployment, memberStatus, registrationServiceDeployment, registrationService, toolchainStatus)

		// when
		res, err := reconciler.Reconcile(req)

		// then
		require.NoError(t, err)
		assert.Equal(t, requeueResult, res)
		AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
			HasCondition(componentsReady()).
			HasHostOperatorStatus(hostOperatorStatusReady(defaultHostOperatorName, "DeploymentReady")).
			HasMemberStatus(memberClusterSingleReady()).
			HasRegistrationServiceStatus(registrationServiceReady())
	})

	t.Run("HostOperator tests", func(t *testing.T) {
		registrationService := newRegistrationServiceReady()
		toolchainStatus := newToolchainStatus()
		registrationServiceDeployment := newDeploymentWithConditions(t, registrationservice.ResourceName, status.DeploymentAvailableCondition(), status.DeploymentProgressingCondition())
		memberStatus := newMemberStatusReady()

		t.Run("Host operator deployment not found - deployment env var not set", func(t *testing.T) {
			// given
			resetFunc := test.UnsetEnvVarAndRestore(t, k8sutil.OperatorNameEnvVar)
			reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), newGetMemberClustersFuncReady, registrationServiceDeployment, registrationService, memberStatus, toolchainStatus)

			// when
			res, err := reconciler.Reconcile(req)

			// then
			resetFunc()
			require.NoError(t, err)
			assert.Equal(t, requeueResult, res)
			AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
				HasCondition(componentsNotReady(string(hostOperatorTag))).
				HasHostOperatorStatus(hostOperatorStatusNotReady("", "DeploymentNotFound", "unable to get the deployment: OPERATOR_NAME must be set")).
				HasMemberStatus(memberClusterSingleReady()).
				HasRegistrationServiceStatus(registrationServiceReady())
		})

		t.Run("Host operator deployment not found", func(t *testing.T) {
			// given
			reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), newGetMemberClustersFuncReady, registrationServiceDeployment, registrationService, memberStatus, toolchainStatus)

			// when
			res, err := reconciler.Reconcile(req)

			// then
			require.NoError(t, err)
			assert.Equal(t, requeueResult, res)
			AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
				HasCondition(componentsNotReady(string(hostOperatorTag))).
				HasHostOperatorStatus(hostOperatorStatusNotReady(defaultHostOperatorName, "DeploymentNotFound", "unable to get the deployment: deployments.apps \"host-operator\" not found")).
				HasMemberStatus(memberClusterSingleReady()).
				HasRegistrationServiceStatus(registrationServiceReady())
		})

		t.Run("Host operator deployment not ready", func(t *testing.T) {
			// given
			hostOperatorDeployment := newDeploymentWithConditions(t, defaultHostOperatorName, status.DeploymentNotAvailableCondition(), status.DeploymentProgressingCondition())
			reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), newGetMemberClustersFuncReady, hostOperatorDeployment, memberStatus, registrationServiceDeployment, registrationService, toolchainStatus)

			// when
			res, err := reconciler.Reconcile(req)

			// then
			require.NoError(t, err)
			assert.Equal(t, requeueResult, res)
			AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
				HasCondition(componentsNotReady(string(hostOperatorTag))).
				HasHostOperatorStatus(hostOperatorStatusNotReady(defaultHostOperatorName, "DeploymentNotReady", "deployment has unready status conditions: Available")).
				HasMemberStatus(memberClusterSingleReady()).
				HasRegistrationServiceStatus(registrationServiceReady())
		})

		t.Run("Host operator deployment not progressing", func(t *testing.T) {
			// given
			hostOperatorDeployment := newDeploymentWithConditions(t, defaultHostOperatorName, status.DeploymentAvailableCondition(), status.DeploymentNotProgressingCondition())
			reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), newGetMemberClustersFuncReady, hostOperatorDeployment, memberStatus, registrationServiceDeployment, registrationService, toolchainStatus)

			// when
			res, err := reconciler.Reconcile(req)

			// then
			require.NoError(t, err)
			assert.Equal(t, requeueResult, res)
			AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
				HasCondition(componentsNotReady(string(hostOperatorTag))).
				HasHostOperatorStatus(hostOperatorStatusNotReady(defaultHostOperatorName, "DeploymentNotReady", "deployment has unready status conditions: Progressing")).
				HasMemberStatus(memberClusterSingleReady()).
				HasRegistrationServiceStatus(registrationServiceReady())
		})
	})

	t.Run("RegistrationService deployment tests", func(t *testing.T) {
		registrationService := newRegistrationServiceReady()
		toolchainStatus := newToolchainStatus()
		memberStatus := newMemberStatusReady()
		hostOperatorDeployment := newDeploymentWithConditions(t, defaultHostOperatorName, status.DeploymentAvailableCondition(), status.DeploymentProgressingCondition())

		t.Run("Registration service deployment not found", func(t *testing.T) {
			// given
			reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), newGetMemberClustersFuncReady, hostOperatorDeployment, registrationService, memberStatus, toolchainStatus)

			// when
			res, err := reconciler.Reconcile(req)

			// then
			require.NoError(t, err)
			assert.Equal(t, requeueResult, res)
			AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
				HasCondition(componentsNotReady(string(registrationServiceTag))).
				HasHostOperatorStatus(hostOperatorStatusReady(defaultHostOperatorName, "DeploymentReady")).
				HasMemberStatus(memberClusterSingleReady()).
				HasRegistrationServiceStatus(registrationServiceDeploymentNotReady("DeploymentNotFound", "unable to get the deployment: deployments.apps \"registration-service\" not found"))
		})

		t.Run("Registration service deployment not ready", func(t *testing.T) {
			// given
			registrationServiceDeployment := newDeploymentWithConditions(t, registrationservice.ResourceName, status.DeploymentNotAvailableCondition(), status.DeploymentProgressingCondition())
			reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), newGetMemberClustersFuncReady, hostOperatorDeployment, memberStatus, registrationServiceDeployment, registrationService, toolchainStatus)

			// when
			res, err := reconciler.Reconcile(req)

			// then
			require.NoError(t, err)
			assert.Equal(t, requeueResult, res)
			AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
				HasCondition(componentsNotReady(string(registrationServiceTag))).
				HasHostOperatorStatus(hostOperatorStatusReady(defaultHostOperatorName, "DeploymentReady")).
				HasMemberStatus(memberClusterSingleReady()).
				HasRegistrationServiceStatus(registrationServiceDeploymentNotReady("DeploymentNotReady", "deployment has unready status conditions: Available"))
		})

		t.Run("Registration service deployment not progressing", func(t *testing.T) {
			// given
			registrationServiceDeployment := newDeploymentWithConditions(t, registrationservice.ResourceName, status.DeploymentAvailableCondition(), status.DeploymentNotProgressingCondition())
			reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), newGetMemberClustersFuncReady, hostOperatorDeployment, memberStatus, registrationServiceDeployment, registrationService, toolchainStatus)

			// when
			res, err := reconciler.Reconcile(req)

			// then
			require.NoError(t, err)
			assert.Equal(t, requeueResult, res)
			AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
				HasCondition(componentsNotReady(string(registrationServiceTag))).
				HasHostOperatorStatus(hostOperatorStatusReady(defaultHostOperatorName, "DeploymentReady")).
				HasMemberStatus(memberClusterSingleReady()).
				HasRegistrationServiceStatus(registrationServiceDeploymentNotReady("DeploymentNotReady", "deployment has unready status conditions: Progressing"))
		})
	})

	t.Run("RegistrationService resource tests", func(t *testing.T) {
		toolchainStatus := newToolchainStatus()
		memberStatus := newMemberStatusReady()
		hostOperatorDeployment := newDeploymentWithConditions(t, defaultHostOperatorName, status.DeploymentAvailableCondition(), status.DeploymentProgressingCondition())
		registrationServiceDeployment := newDeploymentWithConditions(t, registrationservice.ResourceName, status.DeploymentAvailableCondition(), status.DeploymentProgressingCondition())

		t.Run("Registration service resource not found", func(t *testing.T) {
			// given
			reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), newGetMemberClustersFuncReady, hostOperatorDeployment, registrationServiceDeployment, memberStatus, toolchainStatus)

			// when
			res, err := reconciler.Reconcile(req)

			// then
			require.NoError(t, err)
			assert.Equal(t, requeueResult, res)
			AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
				HasCondition(componentsNotReady(string(registrationServiceTag))).
				HasHostOperatorStatus(hostOperatorStatusReady(defaultHostOperatorName, "DeploymentReady")).
				HasMemberStatus(memberClusterSingleReady()).
				HasRegistrationServiceStatus(registrationServiceResourcesNotReady("RegServiceResourceNotFound", "registrationservices.toolchain.dev.openshift.com \"registration-service\" not found"))
		})

		t.Run("Registration service resource not ready", func(t *testing.T) {
			// given
			registrationService := newRegistrationServiceNotReady()
			reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), newGetMemberClustersFuncReady, hostOperatorDeployment, memberStatus, registrationServiceDeployment, registrationService, toolchainStatus)

			// when
			res, err := reconciler.Reconcile(req)

			// then
			require.NoError(t, err)
			assert.Equal(t, requeueResult, res)
			AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
				HasCondition(componentsNotReady(string(registrationServiceTag))).
				HasHostOperatorStatus(hostOperatorStatusReady(defaultHostOperatorName, "DeploymentReady")).
				HasMemberStatus(memberClusterSingleReady()).
				HasRegistrationServiceStatus(registrationServiceResourcesNotReady("RegServiceNotReady", "registration service resource not ready"))
		})
	})

	t.Run("RegistrationService health tests", func(t *testing.T) {
		registrationService := newRegistrationServiceReady()
		hostOperatorDeployment := newDeploymentWithConditions(t, defaultHostOperatorName, status.DeploymentAvailableCondition(), status.DeploymentProgressingCondition())
		registrationServiceDeployment := newDeploymentWithConditions(t, registrationservice.ResourceName, status.DeploymentAvailableCondition(), status.DeploymentProgressingCondition())
		memberStatus := newMemberStatusReady()
		toolchainStatus := newToolchainStatus()

		t.Run("Registration health endpoint - http client error", func(t *testing.T) {
			// given
			reconciler, req, fakeClient := prepareReconcile(t, requestName, httpClientError(), newGetMemberClustersFuncReady, hostOperatorDeployment, registrationServiceDeployment, registrationService, memberStatus, toolchainStatus)

			// when
			res, err := reconciler.Reconcile(req)

			// then
			require.NoError(t, err)
			assert.Equal(t, requeueResult, res)
			AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
				HasCondition(componentsNotReady(string(registrationServiceTag))).
				HasHostOperatorStatus(hostOperatorStatusReady(defaultHostOperatorName, "DeploymentReady")).
				HasMemberStatus(memberClusterSingleReady()).
				HasRegistrationServiceStatus(registrationServiceHealthNotReady("http client error"))
		})

		t.Run("Registration health endpoint - bad status code", func(t *testing.T) {
			// given
			reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseBadCode(), newGetMemberClustersFuncReady, hostOperatorDeployment, registrationServiceDeployment, registrationService, memberStatus, toolchainStatus)

			// when
			res, err := reconciler.Reconcile(req)

			// then
			require.NoError(t, err)
			assert.Equal(t, requeueResult, res)
			AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
				HasCondition(componentsNotReady(string(registrationServiceTag))).
				HasHostOperatorStatus(hostOperatorStatusReady(defaultHostOperatorName, "DeploymentReady")).
				HasMemberStatus(memberClusterSingleReady()).
				HasRegistrationServiceStatus(registrationServiceHealthNotReady("bad response from http://registration-service//api/v1/health : statusCode=500"))
		})

		t.Run("Registration health endpoint - invalid JSON", func(t *testing.T) {
			// given
			reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseInvalid(), newGetMemberClustersFuncReady, hostOperatorDeployment, memberStatus, registrationServiceDeployment, registrationService, toolchainStatus)

			// when
			res, err := reconciler.Reconcile(req)

			// then
			require.NoError(t, err)
			assert.Equal(t, requeueResult, res)
			AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
				HasCondition(componentsNotReady(string(registrationServiceTag))).
				HasHostOperatorStatus(hostOperatorStatusReady(defaultHostOperatorName, "DeploymentReady")).
				HasMemberStatus(memberClusterSingleReady()).
				HasRegistrationServiceStatus(registrationServiceHealthNotReady("invalid character '}' after object key"))
		})

		t.Run("Registration health endpoint - not alive", func(t *testing.T) {
			// given
			reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseBodyNotAlive(), newGetMemberClustersFuncReady, hostOperatorDeployment, memberStatus, registrationServiceDeployment, registrationService, toolchainStatus)

			// when
			res, err := reconciler.Reconcile(req)

			// then
			require.NoError(t, err)
			assert.Equal(t, requeueResult, res)
			AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
				HasCondition(componentsNotReady(string(registrationServiceTag))).
				HasHostOperatorStatus(hostOperatorStatusReady(defaultHostOperatorName, "DeploymentReady")).
				HasMemberStatus(memberClusterSingleReady()).
				HasRegistrationServiceStatus(registrationServiceHealthNotReady("the registration service health endpoint is reporting an unhealthy status"))
		})
	})

	t.Run("MemberStatus tests", func(t *testing.T) {
		toolchainStatus := newToolchainStatus()
		registrationService := newRegistrationServiceReady()
		hostOperatorDeployment := newDeploymentWithConditions(t, defaultHostOperatorName, status.DeploymentAvailableCondition(), status.DeploymentProgressingCondition())
		registrationServiceDeployment := newDeploymentWithConditions(t, registrationservice.ResourceName, status.DeploymentAvailableCondition(), status.DeploymentProgressingCondition())

		t.Run("MemberStatus member clusters not found", func(t *testing.T) {
			// given
			memberStatus := newMemberStatusReady()
			reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), newGetMemberClustersFuncEmpty, hostOperatorDeployment, memberStatus, registrationServiceDeployment, registrationService, toolchainStatus)

			// when
			res, err := reconciler.Reconcile(req)

			// then
			require.NoError(t, err)
			assert.Equal(t, requeueResult, res)
			AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
				HasCondition(componentsNotReady(string(memberConnectionsTag))).Exists().
				HasHostOperatorStatus(hostOperatorStatusReady(defaultHostOperatorName, "DeploymentReady")).
				HasMemberStatus(memberClusterSingleNotReady("", "NoMemberClustersFound", "no member clusters found")).
				HasRegistrationServiceStatus(registrationServiceReady())
		})

		t.Run("MemberStatus saying that there was no member cluster present should be removed", func(t *testing.T) {
			// given
			memberStatus := newMemberStatusReady()
			toolchainStatus := newToolchainStatus()
			toolchainStatus.Status.Members = []toolchainv1alpha1.Member{
				memberClusterSingleNotReady("", "NoMemberClustersFound", "no member clusters found"),
			}
			reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), newGetMemberClustersFuncReady, hostOperatorDeployment, memberStatus, registrationServiceDeployment, registrationService, toolchainStatus)

			// when
			res, err := reconciler.Reconcile(req)

			// then
			require.NoError(t, err)
			assert.Equal(t, requeueResult, res)
			AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
				HasCondition(componentsReady()).
				HasHostOperatorStatus(hostOperatorStatusReady(defaultHostOperatorName, "DeploymentReady")).
				HasMemberStatus(memberClusterSingleReady()).
				HasRegistrationServiceStatus(registrationServiceReady())
		})

		t.Run("MemberStatus not found", func(t *testing.T) {
			// given
			reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), newGetMemberClustersFuncReady, hostOperatorDeployment, registrationServiceDeployment, registrationService, toolchainStatus)

			// when
			res, err := reconciler.Reconcile(req)

			// then
			require.NoError(t, err)
			assert.Equal(t, requeueResult, res)
			AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
				HasCondition(componentsNotReady(string(memberConnectionsTag))).
				HasHostOperatorStatus(hostOperatorStatusReady(defaultHostOperatorName, "DeploymentReady")).
				HasMemberStatus(memberClusterSingleNotReady("member-cluster", "MemberStatusNotFound", "memberstatuses.toolchain.dev.openshift.com \"toolchain-member-status\" not found")).
				HasRegistrationServiceStatus(registrationServiceReady())
		})

		t.Run("MemberStatus not ready", func(t *testing.T) {
			// given
			memberStatus := newMemberStatusNotReady("memberOperator")
			reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), newGetMemberClustersFuncReady, hostOperatorDeployment, memberStatus, registrationServiceDeployment, registrationService, toolchainStatus)

			// when
			res, err := reconciler.Reconcile(req)

			// then
			require.NoError(t, err)
			assert.Equal(t, requeueResult, res)
			AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
				HasCondition(componentsNotReady(string(memberConnectionsTag))).
				HasHostOperatorStatus(hostOperatorStatusReady(defaultHostOperatorName, "DeploymentReady")).
				HasMemberStatus(memberClusterSingleNotReady("member-cluster", "ComponentsNotReady", "components not ready: [memberOperator]")).
				HasRegistrationServiceStatus(registrationServiceReady())
		})

		t.Run("MemberStatus not ready is changed to ready", func(t *testing.T) {
			// given
			memberStatus := newMemberStatusReady()
			toolchainStatus := newToolchainStatus()
			toolchainStatus.Status.Members = []toolchainv1alpha1.Member{
				memberClusterSingleNotReady("member-cluster", "ComponentsNotReady", "some cool error"),
			}
			reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), newGetMemberClustersFuncReady, hostOperatorDeployment, memberStatus, registrationServiceDeployment, registrationService, toolchainStatus)

			// when
			res, err := reconciler.Reconcile(req)

			// then
			require.NoError(t, err)
			assert.Equal(t, requeueResult, res)
			AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
				HasCondition(componentsReady()).
				HasHostOperatorStatus(hostOperatorStatusReady(defaultHostOperatorName, "DeploymentReady")).
				HasMemberStatus(memberClusterSingleReady()).
				HasRegistrationServiceStatus(registrationServiceReady())
		})

		t.Run("All components ready, but one member is missing", func(t *testing.T) {
			// given
			registrationService := newRegistrationServiceReady()
			hostOperatorDeployment := newDeploymentWithConditions(t, defaultHostOperatorName, status.DeploymentAvailableCondition(), status.DeploymentProgressingCondition())
			registrationServiceDeployment := newDeploymentWithConditions(t, registrationservice.ResourceName, status.DeploymentAvailableCondition(), status.DeploymentProgressingCondition())
			memberStatus := newMemberStatusReady()
			toolchainStatus := newToolchainStatus()

			t.Run("with non-zero counter", func(t *testing.T) {
				// given
				toolchainStatus.Status.Members = []toolchainv1alpha1.Member{{
					ClusterName:  "removed-cluster",
					MemberStatus: newMemberStatusReady().Status,
					CapacityUsage: toolchainv1alpha1.CapacityUsageMember{
						UserAccountCount: 10,
					},
				}}
				reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), newGetMemberClustersFuncReady, hostOperatorDeployment, memberStatus, registrationServiceDeployment, registrationService, toolchainStatus)

				// when
				res, err := reconciler.Reconcile(req)

				// then
				require.NoError(t, err)
				assert.Equal(t, requeueResult, res)
				AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
					HasCondition(componentsNotReady(string(memberConnectionsTag))).
					HasHostOperatorStatus(hostOperatorStatusReady(defaultHostOperatorName, "DeploymentReady")).
					HasMemberStatus(memberClusterSingleReady(), memberClusterSingleNotReady("removed-cluster", "MemberToolchainClusterRemoved",
						"toolchainCluster not found for member cluster removed-cluster that was previously registered in the host")).
					HasRegistrationServiceStatus(registrationServiceReady())
			})

			t.Run("with zero count", func(t *testing.T) {
				// given
				toolchainStatus.Status.Members = []toolchainv1alpha1.Member{{
					ClusterName:  "removed-cluster",
					MemberStatus: newMemberStatusReady().Status,
					CapacityUsage: toolchainv1alpha1.CapacityUsageMember{
						UserAccountCount: 0,
					},
				}}
				reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), newGetMemberClustersFuncReady, hostOperatorDeployment, memberStatus, registrationServiceDeployment, registrationService, toolchainStatus)

				// when
				res, err := reconciler.Reconcile(req)

				// then
				require.NoError(t, err)
				assert.Equal(t, requeueResult, res)
				AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
					HasCondition(componentsReady()).
					HasHostOperatorStatus(hostOperatorStatusReady(defaultHostOperatorName, "DeploymentReady")).
					HasMemberStatus(memberClusterSingleReady()).
					HasRegistrationServiceStatus(registrationServiceReady())
			})
		})
	})
}

func TestSynchronizationWithCounter(t *testing.T) {
	// given
	restore := test.SetEnvVarsAndRestore(t, test.Env(k8sutil.OperatorNameEnvVar, defaultHostOperatorName))
	defer restore()
	requestName := defaultToolchainStatusName
	registrationService := newRegistrationServiceReady()
	hostOperatorDeployment := newDeploymentWithConditions(t, defaultHostOperatorName, status.DeploymentAvailableCondition(), status.DeploymentProgressingCondition())
	registrationServiceDeployment := newDeploymentWithConditions(t, registrationservice.ResourceName, status.DeploymentAvailableCondition(), status.DeploymentProgressingCondition())
	toolchainStatus := newToolchainStatus()
	memberStatus := newMemberStatusReady()

	t.Run("Load all current MURs & UAs", func(t *testing.T) {
		// given
		defer counter.Reset()
		initObjects := append(CreateMultipleMurs(t, 10), hostOperatorDeployment, memberStatus, registrationServiceDeployment, registrationService, toolchainStatus)
		reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), newGetMemberClustersFuncReady, initObjects...)

		// when
		res, err := reconciler.Reconcile(req)

		// then
		require.NoError(t, err)
		assert.Equal(t, requeueResult, res)
		AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
			HasCondition(componentsReady()).
			HasHostOperatorStatus(hostOperatorStatusReady(defaultHostOperatorName, "DeploymentReady")).
			HasMurCount(10).
			HasMemberStatus(memberClusterSingleReady()).
			HasUserAccountCount("member-cluster", 10).
			HasRegistrationServiceStatus(registrationServiceReady())

		t.Run("sync with newly added MURs & UAs", func(t *testing.T) {
			// given
			counter.IncrementMasterUserRecordCount()
			counter.IncrementMasterUserRecordCount()
			counter.IncrementUserAccountCount("member-cluster")
			toolchainStatus := newToolchainStatus()
			toolchainStatus.Status.HostOperator = &toolchainv1alpha1.HostOperatorStatus{
				CapacityUsage: toolchainv1alpha1.CapacityUsageHost{
					MasterUserRecordCount: 1,
				},
			}
			reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), newGetMemberClustersFuncReady, hostOperatorDeployment, memberStatus, registrationServiceDeployment, registrationService, toolchainStatus)

			// when
			res, err := reconciler.Reconcile(req)

			// then
			require.NoError(t, err)
			assert.Equal(t, requeueResult, res)
			AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
				HasCondition(componentsReady()).
				HasHostOperatorStatus(hostOperatorStatusReady(defaultHostOperatorName, "DeploymentReady")).
				HasMurCount(12).
				HasMemberStatus(memberClusterSingleReady()).
				HasUserAccountCount("member-cluster", 11).
				HasRegistrationServiceStatus(registrationServiceReady())
		})

	})

	t.Run("initialize the cache using the MURs & UAs from ToolchainStatus", func(t *testing.T) {
		// given
		defer counter.Reset()
		counter.IncrementMasterUserRecordCount()
		counter.IncrementUserAccountCount("member-cluster")
		toolchainStatus := newToolchainStatus()
		toolchainStatus.Status.HostOperator = &toolchainv1alpha1.HostOperatorStatus{
			CapacityUsage: toolchainv1alpha1.CapacityUsageHost{
				MasterUserRecordCount: 8,
			},
		}
		toolchainStatus.Status.Members = []toolchainv1alpha1.Member{{
			ClusterName: "member-cluster",
			CapacityUsage: toolchainv1alpha1.CapacityUsageMember{
				UserAccountCount: 6,
			},
		}}
		reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), newGetMemberClustersFuncReady, hostOperatorDeployment, memberStatus, registrationServiceDeployment, registrationService, toolchainStatus)

		// when
		res, err := reconciler.Reconcile(req)

		// then
		require.NoError(t, err)
		assert.Equal(t, requeueResult, res)
		AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
			HasCondition(componentsReady()).
			HasHostOperatorStatus(hostOperatorStatusReady(defaultHostOperatorName, "DeploymentReady")).
			HasMurCount(9).
			HasMemberStatus(memberClusterSingleReady()).
			HasUserAccountCount("member-cluster", 7).
			HasRegistrationServiceStatus(registrationServiceReady())
		AssertThatCounterHas(t, 9, UserAccountsForCluster("member-cluster", 7))
	})
}

func newDeploymentWithConditions(t *testing.T, deploymentName string, deploymentConditions ...appsv1.DeploymentCondition) *appsv1.Deployment {
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

func newToolchainStatus() *toolchainv1alpha1.ToolchainStatus {
	return &toolchainv1alpha1.ToolchainStatus{
		ObjectMeta: metav1.ObjectMeta{
			Name:      defaultToolchainStatusName,
			Namespace: test.HostOperatorNs,
		},
	}
}

func newGetMemberClustersFuncReady(fakeClient client.Client) cluster.GetMemberClustersFunc {
	return func(conditions ...cluster.Condition) []*cluster.CachedToolchainCluster {
		clusters := []*cluster.CachedToolchainCluster{memberCluster(fakeClient, corev1.ConditionTrue, metav1.Now())}
		return clusters
	}
}

func newGetMemberClustersFuncEmpty(_ client.Client) cluster.GetMemberClustersFunc {
	return func(conditions ...cluster.Condition) []*cluster.CachedToolchainCluster {
		var clusters []*cluster.CachedToolchainCluster
		return clusters
	}
}

func memberCluster(cl client.Client, status corev1.ConditionStatus, lastProbeTime metav1.Time) *cluster.CachedToolchainCluster {
	return &cluster.CachedToolchainCluster{
		Name:              "member-cluster",
		Client:            cl,
		Type:              cluster.Host,
		OperatorNamespace: test.MemberOperatorNs,
		OwnerClusterName:  test.MemberClusterName,
		ClusterStatus: &toolchainv1alpha1.ToolchainClusterStatus{
			Conditions: []toolchainv1alpha1.ToolchainClusterCondition{{
				Type:          toolchainv1alpha1.ToolchainClusterReady,
				Status:        status,
				LastProbeTime: lastProbeTime,
			}},
		},
	}
}

func newMemberStatusReady() *toolchainv1alpha1.MemberStatus {
	readyStatus := status.NewComponentReadyCondition(toolchainv1alpha1.ToolchainStatusAllComponentsReadyReason)
	return newMemberStatus(readyStatus)
}

func newMemberStatusNotReady(unreadyComponents ...string) *toolchainv1alpha1.MemberStatus {
	msg := fmt.Sprintf("components not ready: %v", unreadyComponents)
	notReadyStatus := status.NewComponentErrorCondition(toolchainv1alpha1.ToolchainStatusComponentsNotReadyReason, msg)
	return newMemberStatus(notReadyStatus)
}

func newMemberStatus(condition *toolchainv1alpha1.Condition) *toolchainv1alpha1.MemberStatus {
	return &toolchainv1alpha1.MemberStatus{
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
			Conditions: []toolchainv1alpha1.Condition{*condition},
		},
	}
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

func componentsNotReady(components ...string) toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:    toolchainv1alpha1.ConditionReady,
		Status:  corev1.ConditionFalse,
		Reason:  toolchainv1alpha1.ToolchainStatusComponentsNotReadyReason,
		Message: fmt.Sprintf("components not ready: %v", components),
	}
}

func hostOperatorStatusReady(deploymentName, reason string) toolchainv1alpha1.HostOperatorStatus {
	return toolchainv1alpha1.HostOperatorStatus{
		Conditions: []toolchainv1alpha1.Condition{
			{
				Type:   toolchainv1alpha1.ConditionReady,
				Status: corev1.ConditionTrue,
				Reason: reason,
			},
		},
		BuildTimestamp: version.BuildTime,
		DeploymentName: deploymentName,
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

func memberClusterSingleReady() toolchainv1alpha1.Member {
	return toolchainv1alpha1.Member{
		ClusterName: "member-cluster",
		MemberStatus: toolchainv1alpha1.MemberStatusStatus{
			Conditions: []toolchainv1alpha1.Condition{
				{
					Type:   toolchainv1alpha1.ConditionReady,
					Status: corev1.ConditionTrue,
					Reason: "AllComponentsReady",
				},
			},
		},
	}
}

func memberClusterSingleNotReady(name, reason, msg string) toolchainv1alpha1.Member {
	return toolchainv1alpha1.Member{
		ClusterName: name,
		MemberStatus: toolchainv1alpha1.MemberStatusStatus{
			Conditions: []toolchainv1alpha1.Condition{
				{
					Type:    toolchainv1alpha1.ConditionReady,
					Status:  corev1.ConditionFalse,
					Reason:  reason,
					Message: msg,
				},
			},
		},
	}
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
