package toolchainstatus

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	crtCfg "github.com/codeready-toolchain/host-operator/pkg/configuration"
	"github.com/codeready-toolchain/host-operator/pkg/controller/registrationservice"
	"github.com/codeready-toolchain/host-operator/version"
	"github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	"github.com/codeready-toolchain/toolchain-common/pkg/condition"
	"github.com/codeready-toolchain/toolchain-common/pkg/status"

	"github.com/go-logr/logr"
	"github.com/operator-framework/operator-sdk/pkg/k8sutil"
	errs "github.com/pkg/errors"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_toolchainstatus")

// general toolchainstatus constants
const (
	defaultRequeueTime = time.Second * 5

	memberStatusName = "toolchain-member-status"

	registrationServiceHealthEndpoint = "/api/v1/health"
)

// error messages
const (
	errMsgRegistrationServiceNotReady              = "the registration service is not ready"
	errMsgRegistrationServiceHealthStatusUnhealthy = "the registration service health endpoint is reporting an unhealthy status"
)

type statusComponentTag string

const (
	registrationServiceTag statusComponentTag = "registrationService"
	hostOperatorTag        statusComponentTag = "hostOperator"
	memberConnectionsTag   statusComponentTag = "members"
)

// Add creates a new ToolchainStatus Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, crtConfig *crtCfg.Config) error {
	return add(mgr, newReconciler(mgr, crtConfig))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, crtConfig *crtCfg.Config) *ReconcileToolchainStatus {
	return &ReconcileToolchainStatus{
		client:         mgr.GetClient(),
		httpClientImpl: &http.Client{},
		scheme:         mgr.GetScheme(),
		getMembersFunc: cluster.GetMemberClusters,
		config:         crtConfig,
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r *ReconcileToolchainStatus) error {
	// create a new controller
	c, err := controller.New("toolchainstatus-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// watch for changes to primary resource ToolchainStatus
	err = c.Watch(&source.Kind{Type: &toolchainv1alpha1.ToolchainStatus{}}, &handler.EnqueueRequestForObject{}, predicate.GenerationChangedPredicate{})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileToolchainStatus implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileToolchainStatus{}

type httpClient interface {
	Get(url string) (*http.Response, error)
}

// ReconcileToolchainStatus reconciles a ToolchainStatus object
type ReconcileToolchainStatus struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client         client.Client
	httpClientImpl httpClient
	scheme         *runtime.Scheme
	getMembersFunc cluster.GetMemberClustersFunc
	config         *crtCfg.Config
}

// Reconcile reads the state of toolchain host and member cluster components and updates the ToolchainStatus resource with information useful for observation or troubleshooting
func (r *ReconcileToolchainStatus) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling ToolchainStatus")

	// fetch the ToolchainStatus
	toolchainStatus := &toolchainv1alpha1.ToolchainStatus{}
	err := r.client.Get(context.TODO(), request.NamespacedName, toolchainStatus)

	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			return reconcile.Result{}, nil
		}
		reqLogger.Error(err, "Unable to fetch ToolchainStatus resource")
		return reconcile.Result{}, err
	}

	err = r.aggregateAndUpdateStatus(reqLogger, toolchainStatus)
	if err != nil {
		reqLogger.Error(err, "Failed to update status")
		return reconcile.Result{RequeueAfter: defaultRequeueTime}, err
	}

	reqLogger.Info(fmt.Sprintf("Finished updating ToolchainStatus, requeueing after %v", defaultRequeueTime))
	return reconcile.Result{RequeueAfter: defaultRequeueTime}, nil
}

type statusHandler struct {
	name         statusComponentTag
	handleStatus statusHandlerFunc
}

type statusHandlerFunc func(logr.Logger, *toolchainv1alpha1.ToolchainStatus) error

// aggregateAndUpdateStatus runs each of the status handlers. Each status handler reports readiness for a toolchain component. If any
// component status is not ready then it will set the condition of the top-level status of the ToolchainStatus resource to not ready.
func (r *ReconcileToolchainStatus) aggregateAndUpdateStatus(reqLogger logr.Logger, toolchainStatus *toolchainv1alpha1.ToolchainStatus) error {

	// collect status handlers that will contribute status of various toolchain components
	hostOperatorStatusHandlerFunc := statusHandler{name: hostOperatorTag, handleStatus: r.hostOperatorHandleStatus}
	registrationServiceStatusHandlerFunc := statusHandler{name: registrationServiceTag, handleStatus: r.registrationServiceHandleStatus}
	memberStatusHandlerFunc := statusHandler{name: memberConnectionsTag, handleStatus: r.membersHandleStatus}

	statusHandlers := []statusHandler{
		hostOperatorStatusHandlerFunc,
		memberStatusHandlerFunc,
		registrationServiceStatusHandlerFunc,
	}

	// track components that are not ready
	unreadyComponents := []string{}

	// retrieve component statuses eg. kubefed, host deployment
	for _, handler := range statusHandlers {
		err := handler.handleStatus(reqLogger, toolchainStatus)
		if err != nil {
			reqLogger.Error(err, "status update problem")
			unreadyComponents = append(unreadyComponents, string(handler.name))
		}
	}

	// if any components were not ready then set the overall status to not ready
	if len(unreadyComponents) > 0 {
		return r.setStatusNotReady(toolchainStatus, fmt.Sprintf("components not ready: %v", unreadyComponents))
	}
	return r.setStatusReady(toolchainStatus)
}

// hostOperatorHandleStatus retrieves the Deployment for the host operator and adds its status to ToolchainStatus. It returns an error
// if the deployment is not determined to be ready
func (r *ReconcileToolchainStatus) hostOperatorHandleStatus(reqLogger logr.Logger, toolchainStatus *toolchainv1alpha1.ToolchainStatus) error {
	operatorStatus := &toolchainv1alpha1.HostOperatorStatus{
		Version:        version.Version,
		Revision:       version.Commit,
		BuildTimestamp: version.BuildTime,
	}

	// look up name of the host operator deployment
	hostOperatorDeploymentName, err := k8sutil.GetOperatorName()
	if err != nil {
		err = errs.Wrap(err, status.ErrMsgCannotGetDeployment)
		errCondition := status.NewComponentErrorCondition(toolchainv1alpha1.ToolchainStatusDeploymentNotFoundReason, err.Error())
		operatorStatus.Conditions = []toolchainv1alpha1.Condition{*errCondition}
		toolchainStatus.Status.HostOperator = operatorStatus
		return err
	}
	operatorStatus.DeploymentName = hostOperatorDeploymentName

	// check host operator deployment status
	deploymentConditions := status.GetDeploymentStatusConditions(r.client, hostOperatorDeploymentName, toolchainStatus.Namespace)
	err = status.ValidateComponentConditionReady(deploymentConditions...)

	// update toolchainStatus
	operatorStatus.Conditions = deploymentConditions
	toolchainStatus.Status.HostOperator = operatorStatus
	return err
}

// registrationServiceHandleStatus retrieves the Deployment for the registration service and adds its status to ToolchainStatus. It returns an error
// if the registration service is not ready
func (r *ReconcileToolchainStatus) registrationServiceHandleStatus(reqLogger logr.Logger, toolchainStatus *toolchainv1alpha1.ToolchainStatus) (err error) {

	s := regServiceSubstatusHandler{
		controllerClient: r.client,
		httpClientImpl:   r.httpClientImpl,
	}

	// gather the functions for handling registration service status eg. resource templates, deployment, health endpoint
	substatusHandlers := []statusHandlerFunc{
		s.addRegistrationServiceResourceStatus,
		s.addRegistrationServiceDeploymentStatus,
		s.addRegistrationServiceHealthStatus,
	}

	// ensure the registrationservice part of the status is created
	toolchainStatus.Status.RegistrationService = &toolchainv1alpha1.HostRegistrationServiceStatus{}

	// call each of the registration service status handlers
	for _, handler := range substatusHandlers {
		handlerErr := handler(reqLogger, toolchainStatus)
		if handlerErr != nil {
			err = handlerErr
		}
	}

	return err
}

// memberHandleStatus retrieves the status of member clusters and adds them to ToolchainStatus. It returns an error
// if any of the members are not ready or if no member clusters are found
func (r *ReconcileToolchainStatus) membersHandleStatus(reqLogger logr.Logger, toolchainStatus *toolchainv1alpha1.ToolchainStatus) (componentError error) {
	// get member clusters
	memberClusters := r.getMembersFunc()
	members := []toolchainv1alpha1.Member{}
	if len(memberClusters) == 0 {
		err := fmt.Errorf("no member clusters found")
		memberStatusNotFoundCondition := status.NewComponentErrorCondition(toolchainv1alpha1.ToolchainStatusMemberStatusNoClustersFoundReason, err.Error())
		memberStatus := customMemberStatus(*memberStatusNotFoundCondition)
		noMemberClusterStatus := toolchainv1alpha1.Member{
			ClusterName:  "",
			MemberStatus: memberStatus,
		}
		members = append(members, noMemberClusterStatus)
		toolchainStatus.Status.Members = members
		return err
	}
	for _, cluster := range memberClusters {
		memberClient := cluster.Client
		if memberClient == nil {
			// there is no client for the member cluster, create a status condition and add it to this member's status
			componentError = fmt.Errorf("member cluster does not have a client assigned")
			notFoundCondition := status.NewComponentErrorCondition(toolchainv1alpha1.ToolchainStatusMemberStatusNotFoundReason, componentError.Error())
			memberStatus := customMemberStatus(*notFoundCondition)
			members = append(members, memberResult(cluster, memberStatus))
			continue
		}
		memberStatusObj := &toolchainv1alpha1.MemberStatus{}
		err := memberClient.Get(context.TODO(), types.NamespacedName{Namespace: cluster.OperatorNamespace, Name: memberStatusName}, memberStatusObj)
		if err != nil {
			// couldn't find the memberstatus resource on the member cluster, create a status condition and add it to this member's status
			wrappedErr := errs.Wrap(err, fmt.Sprintf("cannot find memberstatus resource in namespace %s", cluster.OperatorNamespace))
			componentError = wrappedErr
			memberStatusNotFoundCondition := status.NewComponentErrorCondition(toolchainv1alpha1.ToolchainStatusMemberStatusNotFoundReason, wrappedErr.Error())
			memberStatus := customMemberStatus(*memberStatusNotFoundCondition)
			members = append(members, memberResult(cluster, memberStatus))
			continue
		}

		// take only the overall conditions from the memberstatus (ie. don't include status of member operator, host connection, etc.) in order to keep readability.
		// member status details can be viewed on the member cluster directly.
		memberStatus := toolchainv1alpha1.MemberStatusStatus{Conditions: memberStatusObj.Status.Conditions}
		if condition.IsNotTrue(memberStatusObj.Status.Conditions, toolchainv1alpha1.ConditionReady) {
			// the memberstatus is not ready so set the component error to bubble up the error to the overall toolchain status
			componentError = fmt.Errorf("member cluster %s not ready: the memberstatus ready condition is not true", cluster.Name)
		}
		members = append(members, memberResult(cluster, memberStatus))
	}

	// add member cluster statuses to toolchainstatus
	toolchainStatus.Status.Members = members
	return componentError
}

// updateStatusConditions updates Member status conditions with the new conditions
func (r *ReconcileToolchainStatus) updateStatusConditions(memberStatus *toolchainv1alpha1.ToolchainStatus, newConditions ...toolchainv1alpha1.Condition) error {
	// the controller should always update at least the last updated timestamp of the status so the status should be updated regardless of whether
	// any specific fields were updated. This way a problem with the controller can be indicated if the last updated timestamp was not updated.
	conditionsWithTimestamps := []toolchainv1alpha1.Condition{}
	for _, condition := range newConditions {
		condition.LastTransitionTime = metav1.Now()
		condition.LastUpdatedTime = &metav1.Time{Time: condition.LastTransitionTime.Time}
		conditionsWithTimestamps = append(conditionsWithTimestamps, condition)
	}
	memberStatus.Status.Conditions = conditionsWithTimestamps
	return r.client.Status().Update(context.TODO(), memberStatus)
}

func (r *ReconcileToolchainStatus) setStatusReady(toolchainStatus *toolchainv1alpha1.ToolchainStatus) error {
	return r.updateStatusConditions(
		toolchainStatus,
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.ConditionReady,
			Status: corev1.ConditionTrue,
			Reason: toolchainv1alpha1.ToolchainStatusAllComponentsReadyReason,
		})
}

func (r *ReconcileToolchainStatus) setStatusNotReady(toolchainStatus *toolchainv1alpha1.ToolchainStatus, message string) error {
	return r.updateStatusConditions(
		toolchainStatus,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.ConditionReady,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.ToolchainStatusComponentsNotReadyReason,
			Message: message,
		})
}

func customMemberStatus(conditions ...toolchainv1alpha1.Condition) toolchainv1alpha1.MemberStatusStatus {
	return toolchainv1alpha1.MemberStatusStatus{
		Conditions: conditions,
	}
}

func memberResult(cluster *cluster.FedCluster, memberStatus toolchainv1alpha1.MemberStatusStatus) toolchainv1alpha1.Member {
	return toolchainv1alpha1.Member{
		ClusterName:  cluster.Name,
		MemberStatus: memberStatus,
	}
}

type regServiceSubstatusHandler struct {
	httpClientImpl   httpClient
	controllerClient client.Client
}

// addRegistrationServiceResourceStatus handles the RegistrationService.RegistrationServiceResources part of the toolchainstatus
func (s regServiceSubstatusHandler) addRegistrationServiceResourceStatus(reqLogger logr.Logger, toolchainStatus *toolchainv1alpha1.ToolchainStatus) error {
	// get the registrationservice resource
	registrationServiceName := types.NamespacedName{Namespace: toolchainStatus.Namespace, Name: registrationservice.ResourceName}
	registrationService := &toolchainv1alpha1.RegistrationService{}
	err := s.controllerClient.Get(context.TODO(), registrationServiceName, registrationService)
	if err != nil {
		err = errs.Wrap(err, "unable to get the registrationservice resource")
		errCondition := status.NewComponentErrorCondition(toolchainv1alpha1.ToolchainStatusRegServiceResourceNotFoundReason, err.Error())
		toolchainStatus.Status.RegistrationService.RegistrationServiceResources.Conditions = []toolchainv1alpha1.Condition{*errCondition}
		return err
	}

	// use the registrationservice resource directly in the toolchainstatus
	toolchainStatus.Status.RegistrationService.RegistrationServiceResources.Conditions = registrationService.Status.Conditions
	if condition.IsNotTrue(registrationService.Status.Conditions, toolchainv1alpha1.ConditionReady) {
		return fmt.Errorf("the registrationservice resource is not ready")
	}

	return nil
}

// addRegistrationServiceDeploymentStatus handles the RegistrationService.Deployment part of the toolchainstatus
func (s regServiceSubstatusHandler) addRegistrationServiceDeploymentStatus(reqLogger logr.Logger, toolchainStatus *toolchainv1alpha1.ToolchainStatus) error {
	deploymentConditions := status.GetDeploymentStatusConditions(s.controllerClient, registrationservice.ResourceName, toolchainStatus.Namespace)
	toolchainStatus.Status.RegistrationService.Deployment.Name = registrationservice.ResourceName
	toolchainStatus.Status.RegistrationService.Deployment.Conditions = deploymentConditions

	err := status.ValidateComponentConditionReady(deploymentConditions...)
	if err != nil {
		err = errs.Wrap(err, "a problem was detected in the deployment status")
		return err
	}

	return nil
}

// addRegistrationServiceHealthStatus handles the RegistrationService.Health part of the toolchainstatus
func (s regServiceSubstatusHandler) addRegistrationServiceHealthStatus(reqLogger logr.Logger, toolchainStatus *toolchainv1alpha1.ToolchainStatus) error {
	// get the JSON payload from the health endpoint
	registrationServiceHealthURL := "http://registration-service/" + registrationServiceHealthEndpoint
	resp, err := s.httpClientImpl.Get(registrationServiceHealthURL)
	if err != nil {
		err = errs.Wrap(err, errMsgRegistrationServiceNotReady)
		errCondition := status.NewComponentErrorCondition(toolchainv1alpha1.ToolchainStatusRegServiceNotReadyReason, err.Error())
		toolchainStatus.Status.RegistrationService.Health.Conditions = []toolchainv1alpha1.Condition{*errCondition}
		return err
	}

	// bad response
	if resp.StatusCode != http.StatusOK {
		err = errs.Wrap(fmt.Errorf("bad response from %s : statusCode=%d", registrationServiceHealthURL, resp.StatusCode), errMsgRegistrationServiceNotReady)
		errCondition := status.NewComponentErrorCondition(toolchainv1alpha1.ToolchainStatusRegServiceNotReadyReason, err.Error())
		toolchainStatus.Status.RegistrationService.Health.Conditions = []toolchainv1alpha1.Condition{*errCondition}
		return err
	}

	// decode the response to JSON
	defer func() {
		ioutil.ReadAll(resp.Body)
		resp.Body.Close()
	}()
	healthValues := status.Health{}
	err = json.NewDecoder(resp.Body).Decode(&healthValues)
	if err != nil {
		err = errs.Wrap(err, errMsgRegistrationServiceNotReady)
		errCondition := status.NewComponentErrorCondition(toolchainv1alpha1.ToolchainStatusRegServiceNotReadyReason, err.Error())
		toolchainStatus.Status.RegistrationService.Health.Conditions = []toolchainv1alpha1.Condition{*errCondition}
		return err
	}

	// get the health status values
	healthStatus := toolchainv1alpha1.RegistrationServiceHealth{
		Alive:       fmt.Sprintf("%t", healthValues.Alive),
		BuildTime:   healthValues.BuildTime,
		Environment: healthValues.Environment,
		Revision:    healthValues.Revision,
		StartTime:   healthValues.StartTime,
	}

	// add the health status to the toolchainstatus
	toolchainStatus.Status.RegistrationService.Health = healthStatus
	if !healthValues.Alive {
		err = fmt.Errorf(errMsgRegistrationServiceHealthStatusUnhealthy)
		errCondition := status.NewComponentErrorCondition(toolchainv1alpha1.ToolchainStatusRegServiceNotReadyReason, err.Error())
		toolchainStatus.Status.RegistrationService.Health.Conditions = []toolchainv1alpha1.Condition{*errCondition}
		return err
	}

	componentReadyCondition := status.NewComponentReadyCondition(toolchainv1alpha1.ToolchainStatusRegServiceReadyReason)
	toolchainStatus.Status.RegistrationService.Health.Conditions = []toolchainv1alpha1.Condition{*componentReadyCondition}
	return nil
}
