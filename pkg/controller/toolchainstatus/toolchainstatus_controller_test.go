package toolchainstatus

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/codeready-toolchain/host-operator/pkg/configuration"
	"github.com/codeready-toolchain/host-operator/pkg/controller/registrationservice"
	. "github.com/codeready-toolchain/host-operator/test"

	"github.com/codeready-toolchain/api/pkg/apis"
	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
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
	kubefed_common "sigs.k8s.io/kubefed/pkg/apis/core/common"
	kubefed_v1beta1 "sigs.k8s.io/kubefed/pkg/apis/core/v1beta1"
)

var requeueResult = reconcile.Result{RequeueAfter: defaultRequeueTime}

const defaultHostOperatorName = "host-operator"

const defaultToolchainStatusName = configuration.DefaultToolchainStatusName

type fakeHTTPClient struct {
	response http.Response
	err      error
}

func (f *fakeHTTPClient) Get(url string) (*http.Response, error) {
	return &f.response, f.err
}

const respBodyGood = "{\"alive\":true,\"environment\":\"dev\",\"revision\":\"64af1be5c6011fae5497a7c35e2a986d633b3421\",\"buildTime\":\"0\",\"startTime\":\"2020-07-06T13:18:30Z\"}"
const respBodyInvalid = "{\"not found\"}"
const respBodyBad = "{\"alive\":false,\"environment\":\"dev\",\"revision\":\"64af1be5c6011fae5497a7c35e2a986d633b3421\",\"buildTime\":\"0\",\"startTime\":\"2020-07-06T13:18:30Z\"}"

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
	return r, reconcile.Request{test.NamespacedName(test.HostOperatorNs, requestName)}, fakeClient
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
		hostOperatorDeployment := newDeploymentWithConditions(t, defaultHostOperatorName, status.DeploymentReadyCondition(), status.DeploymentProgressingCondition())
		registrationServiceDeployment := newDeploymentWithConditions(t, registrationservice.ResourceName, status.DeploymentReadyCondition(), status.DeploymentProgressingCondition())
		memberStatus := newMemberStatusReady()
		toolchainStatus := newToolchainStatus()
		reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), newGetMemberClustersFuncReady, hostOperatorDeployment, memberStatus, registrationServiceDeployment, registrationService, toolchainStatus)

		// when
		res, err := reconciler.Reconcile(req)

		// then
		require.NoError(t, err)
		assert.Equal(t, requeueResult, res)
		AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
			HasCondition(ComponentsReady())
	})

	t.Run("HostOperator tests", func(t *testing.T) {
		registrationService := newRegistrationServiceReady()
		toolchainStatus := newToolchainStatus()
		registrationServiceDeployment := newDeploymentWithConditions(t, registrationservice.ResourceName, status.DeploymentReadyCondition(), status.DeploymentProgressingCondition())
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
				HasCondition(ComponentsNotReady(string(hostOperatorTag)))
			AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
				HasHostOperatorConditionErrorMsg("unable to get the operator deployment")
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
				HasCondition(ComponentsNotReady(string(hostOperatorTag)))
		})

		t.Run("Host operator deployment not ready", func(t *testing.T) {
			// given
			hostOperatorDeployment := newDeploymentWithConditions(t, defaultHostOperatorName, status.DeploymentNotReadyCondition(), status.DeploymentProgressingCondition())
			reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), newGetMemberClustersFuncReady, hostOperatorDeployment, memberStatus, registrationServiceDeployment, registrationService, toolchainStatus)

			// when
			res, err := reconciler.Reconcile(req)

			// then
			require.NoError(t, err)
			assert.Equal(t, requeueResult, res)
			AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
				HasCondition(ComponentsNotReady(string(hostOperatorTag)))
		})

		t.Run("Host operator deployment not progressing", func(t *testing.T) {
			// given
			hostOperatorDeployment := newDeploymentWithConditions(t, defaultHostOperatorName, status.DeploymentReadyCondition(), status.DeploymentNotProgressingCondition())
			reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), newGetMemberClustersFuncReady, hostOperatorDeployment, memberStatus, registrationServiceDeployment, registrationService, toolchainStatus)

			// when
			res, err := reconciler.Reconcile(req)

			// then
			require.NoError(t, err)
			assert.Equal(t, requeueResult, res)
			AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
				HasCondition(ComponentsNotReady(string(hostOperatorTag)))
		})
	})

	t.Run("RegistrationService deployment tests", func(t *testing.T) {
		registrationService := newRegistrationServiceReady()
		toolchainStatus := newToolchainStatus()
		memberStatus := newMemberStatusReady()
		hostOperatorDeployment := newDeploymentWithConditions(t, defaultHostOperatorName, status.DeploymentReadyCondition(), status.DeploymentProgressingCondition())

		t.Run("Registration service deployment not found", func(t *testing.T) {
			// given
			reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), newGetMemberClustersFuncReady, hostOperatorDeployment, registrationService, memberStatus, toolchainStatus)

			// when
			res, err := reconciler.Reconcile(req)

			// then
			require.NoError(t, err)
			assert.Equal(t, requeueResult, res)
			AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
				HasCondition(ComponentsNotReady(string(registrationServiceTag)))
		})

		t.Run("Registration service deployment not ready", func(t *testing.T) {
			// given
			registrationServiceDeployment := newDeploymentWithConditions(t, registrationservice.ResourceName, status.DeploymentNotReadyCondition(), status.DeploymentProgressingCondition())
			reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), newGetMemberClustersFuncReady, hostOperatorDeployment, memberStatus, registrationServiceDeployment, registrationService, toolchainStatus)

			// when
			res, err := reconciler.Reconcile(req)

			// then
			require.NoError(t, err)
			assert.Equal(t, requeueResult, res)
			AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
				HasCondition(ComponentsNotReady(string(registrationServiceTag)))
		})

		t.Run("Registration service deployment not progressing", func(t *testing.T) {
			// given
			registrationServiceDeployment := newDeploymentWithConditions(t, registrationservice.ResourceName, status.DeploymentReadyCondition(), status.DeploymentNotProgressingCondition())
			reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), newGetMemberClustersFuncReady, hostOperatorDeployment, memberStatus, registrationServiceDeployment, registrationService, toolchainStatus)

			// when
			res, err := reconciler.Reconcile(req)

			// then
			require.NoError(t, err)
			assert.Equal(t, requeueResult, res)
			AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
				HasCondition(ComponentsNotReady(string(registrationServiceTag)))
		})
	})

	t.Run("RegistrationService resource tests", func(t *testing.T) {
		toolchainStatus := newToolchainStatus()
		memberStatus := newMemberStatusReady()
		hostOperatorDeployment := newDeploymentWithConditions(t, defaultHostOperatorName, status.DeploymentReadyCondition(), status.DeploymentProgressingCondition())
		registrationServiceDeployment := newDeploymentWithConditions(t, registrationservice.ResourceName, status.DeploymentReadyCondition(), status.DeploymentProgressingCondition())

		t.Run("Registration service resource not found", func(t *testing.T) {
			// given
			reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), newGetMemberClustersFuncReady, hostOperatorDeployment, registrationServiceDeployment, memberStatus, toolchainStatus)

			// when
			res, err := reconciler.Reconcile(req)

			// then
			require.NoError(t, err)
			assert.Equal(t, requeueResult, res)
			AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
				HasCondition(ComponentsNotReady(string(registrationServiceTag)))
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
				HasCondition(ComponentsNotReady(string(registrationServiceTag)))
		})
	})

	t.Run("RegistrationService health tests", func(t *testing.T) {
		registrationService := newRegistrationServiceReady()
		hostOperatorDeployment := newDeploymentWithConditions(t, defaultHostOperatorName, status.DeploymentReadyCondition(), status.DeploymentProgressingCondition())
		registrationServiceDeployment := newDeploymentWithConditions(t, registrationservice.ResourceName, status.DeploymentReadyCondition(), status.DeploymentProgressingCondition())
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
				HasCondition(ComponentsNotReady(string(registrationServiceTag)))
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
				HasCondition(ComponentsNotReady(string(registrationServiceTag)))
		})

		t.Run("Registration health endpoint - invalid JSON", func(t *testing.T) {
			// given
			registrationService := newRegistrationServiceNotReady()
			reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseInvalid(), newGetMemberClustersFuncReady, hostOperatorDeployment, memberStatus, registrationServiceDeployment, registrationService, toolchainStatus)

			// when
			res, err := reconciler.Reconcile(req)

			// then
			require.NoError(t, err)
			assert.Equal(t, requeueResult, res)
			AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
				HasCondition(ComponentsNotReady(string(registrationServiceTag)))
		})

		t.Run("Registration health endpoint - not alive", func(t *testing.T) {
			// given
			registrationService := newRegistrationServiceNotReady()
			reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseBodyNotAlive(), newGetMemberClustersFuncReady, hostOperatorDeployment, memberStatus, registrationServiceDeployment, registrationService, toolchainStatus)

			// when
			res, err := reconciler.Reconcile(req)

			// then
			require.NoError(t, err)
			assert.Equal(t, requeueResult, res)
			AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
				HasCondition(ComponentsNotReady(string(registrationServiceTag)))
		})
	})

	t.Run("MemberStatus tests", func(t *testing.T) {
		toolchainStatus := newToolchainStatus()
		registrationService := newRegistrationServiceReady()
		hostOperatorDeployment := newDeploymentWithConditions(t, defaultHostOperatorName, status.DeploymentReadyCondition(), status.DeploymentProgressingCondition())
		registrationServiceDeployment := newDeploymentWithConditions(t, registrationservice.ResourceName, status.DeploymentReadyCondition(), status.DeploymentProgressingCondition())

		t.Run("MemberStatus member clusters not found - no cluster client", func(t *testing.T) {
			// given
			memberStatus := newMemberStatusReady()
			reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), newGetMemberClustersFuncNoClient, hostOperatorDeployment, memberStatus, registrationServiceDeployment, registrationService, toolchainStatus)

			// when
			res, err := reconciler.Reconcile(req)

			// then
			require.NoError(t, err)
			assert.Equal(t, requeueResult, res)
			AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
				HasCondition(ComponentsNotReady(string(memberConnectionsTag)))
		})

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
				HasCondition(ComponentsNotReady(string(memberConnectionsTag)))
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
				HasCondition(ComponentsNotReady(string(memberConnectionsTag)))
		})

		t.Run("MemberStatus not ready", func(t *testing.T) {
			// given
			memberStatus := newMemberStatusNotReady()
			reconciler, req, fakeClient := prepareReconcile(t, requestName, newResponseGood(), newGetMemberClustersFuncReady, hostOperatorDeployment, memberStatus, registrationServiceDeployment, registrationService, toolchainStatus)

			// when
			res, err := reconciler.Reconcile(req)

			// then
			require.NoError(t, err)
			assert.Equal(t, requeueResult, res)
			AssertThatToolchainStatus(t, req.Namespace, requestName, fakeClient).
				HasCondition(ComponentsNotReady(string(memberConnectionsTag)))
		})
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
	return func(conditions ...cluster.Condition) []*cluster.FedCluster {
		clusters := []*cluster.FedCluster{memberCluster(fakeClient, corev1.ConditionTrue, metav1.Now())}
		return clusters
	}
}

func newGetMemberClustersFuncEmpty(fakeClient client.Client) cluster.GetMemberClustersFunc {
	return func(conditions ...cluster.Condition) []*cluster.FedCluster {
		clusters := []*cluster.FedCluster{}
		return clusters
	}
}

func newGetMemberClustersFuncNoClient(_ client.Client) cluster.GetMemberClustersFunc {
	return func(conditions ...cluster.Condition) []*cluster.FedCluster {
		clusters := []*cluster.FedCluster{memberCluster(nil, corev1.ConditionFalse, metav1.Now())}
		return clusters
	}
}

func memberCluster(cl client.Client, status corev1.ConditionStatus, lastProbeTime metav1.Time) *cluster.FedCluster {
	return &cluster.FedCluster{
		Client:            cl,
		Type:              cluster.Host,
		OperatorNamespace: test.MemberOperatorNs,
		OwnerClusterName:  test.MemberClusterName,
		ClusterStatus: &kubefed_v1beta1.KubeFedClusterStatus{
			Conditions: []kubefed_v1beta1.ClusterCondition{{
				Type:          kubefed_common.ClusterReady,
				Status:        status,
				LastProbeTime: lastProbeTime,
			}},
		},
	}
}

func newMemberStatusReady() *toolchainv1alpha1.MemberStatus {
	readyStatus := status.NewComponentReadyCondition(toolchainv1alpha1.ToolchainStatusReasonAllComponentsReady)
	return newMemberStatus(readyStatus)
}

func newMemberStatusNotReady(unreadyComponents ...string) *toolchainv1alpha1.MemberStatus {
	msg := fmt.Sprintf("components not ready: %v", unreadyComponents)
	notReadyStatus := status.NewComponentErrorCondition(toolchainv1alpha1.ToolchainStatusReasonComponentsNotReady, msg)
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
	readyStatus := status.NewComponentReadyCondition(toolchainv1alpha1.ToolchainStatusReasonRegServiceReady)
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
	notReadyStatus := status.NewComponentErrorCondition(toolchainv1alpha1.ToolchainStatusReasonRegServiceNotReady, "registration service resource not ready")
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
