package toolchainstatus

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"regexp"
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	crtCfg "github.com/codeready-toolchain/host-operator/pkg/configuration"
	"github.com/codeready-toolchain/host-operator/pkg/controller/registrationservice"
	"github.com/codeready-toolchain/host-operator/pkg/counter"
	"github.com/codeready-toolchain/host-operator/version"
	"github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	"github.com/codeready-toolchain/toolchain-common/pkg/condition"
	"github.com/codeready-toolchain/toolchain-common/pkg/status"

	"github.com/ghodss/yaml"
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
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
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
	memberStatusName = "toolchain-member-status"

	registrationServiceHealthURL = "http://registration-service/api/v1/health"
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
	counterTag             statusComponentTag = "MasterUserRecord and UserAccount counter"
	minutesAfterUnready    time.Duration      = 10
)

const (
	adminUnreadyNotificationSubject = "ToolchainStatus has been in an unready status for an extended period"
)

var emailRegex = regexp.MustCompile("^[a-zA-Z0-9.!#$%&'*+\\/=?^_`{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$")

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
	requeueTime := r.config.GetToolchainStatusRefreshTime()

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
		return reconcile.Result{RequeueAfter: requeueTime}, err
	}

	reqLogger.Info(fmt.Sprintf("Finished updating ToolchainStatus, requeueing after %v", requeueTime))
	return reconcile.Result{RequeueAfter: requeueTime}, nil
}

type statusHandler struct {
	name         statusComponentTag
	handleStatus statusHandlerFunc
}

type statusHandlerFunc func(logr.Logger, *toolchainv1alpha1.ToolchainStatus) bool

// aggregateAndUpdateStatus runs each of the status handlers. Each status handler reports readiness for a toolchain component. If any
// component status is not ready then it will set the condition of the top-level status of the ToolchainStatus resource to not ready.
func (r *ReconcileToolchainStatus) aggregateAndUpdateStatus(reqLogger logr.Logger, toolchainStatus *toolchainv1alpha1.ToolchainStatus) error {

	// collect status handlers that will contribute status of various toolchain components
	hostOperatorStatusHandlerFunc := statusHandler{name: hostOperatorTag, handleStatus: r.hostOperatorHandleStatus}
	registrationServiceStatusHandlerFunc := statusHandler{name: registrationServiceTag, handleStatus: r.registrationServiceHandleStatus}
	memberStatusHandlerFunc := statusHandler{name: memberConnectionsTag, handleStatus: r.membersHandleStatus}
	// should be executed as the last one
	counterHandlerFunc := statusHandler{name: counterTag, handleStatus: r.synchronizeWithCounter}

	statusHandlers := []statusHandler{
		hostOperatorStatusHandlerFunc,
		memberStatusHandlerFunc,
		registrationServiceStatusHandlerFunc,
		counterHandlerFunc,
	}

	// track components that are not ready
	var unreadyComponents []string

	// retrieve component statuses eg. ToolchainCluster, host deployment
	for _, statusHandler := range statusHandlers {
		isReady := statusHandler.handleStatus(reqLogger, toolchainStatus)
		if !isReady {
			unreadyComponents = append(unreadyComponents, string(statusHandler.name))
		}
	}

	// if any components were not ready then set the overall status to not ready
	if len(unreadyComponents) > 0 {
		err := r.notificationCheck(reqLogger, toolchainStatus)
		if err != nil {
			return err
		}

		return r.setStatusNotReady(reqLogger, toolchainStatus, fmt.Sprintf("components not ready: %v", unreadyComponents))
	}
	return r.setStatusReady(reqLogger, toolchainStatus)
}

func (r *ReconcileToolchainStatus) notificationCheck(reqLogger logr.Logger, toolchainStatus *toolchainv1alpha1.ToolchainStatus) error {
	// If the current ToolchainStatus:
	// a) Is currently not ready,
	// b) has not been ready for longer than the configured threshold, and
	// c) no notification has been already sent, then
	// send a notification to the admin mailing list
	c, found := condition.FindConditionByType(toolchainStatus.Status.Conditions, toolchainv1alpha1.ConditionReady)
	if found && c.Status == corev1.ConditionFalse {
		threshold := time.Now().Add(-minutesAfterUnready * time.Minute)
		if c.LastTransitionTime.Before(&metav1.Time{Time: threshold}) {
			if !condition.IsTrue(toolchainStatus.Status.Conditions, toolchainv1alpha1.ToolchainStatusUnreadyNotificationCreated) {
				if err := r.sendToolchainStatusUnreadyNotification(reqLogger, toolchainStatus); err != nil {
					reqLogger.Error(err, "Failed to create toolchain status unready notification")

					// set the failed to create notification status condition
					return r.wrapErrorWithStatusUpdate(reqLogger, toolchainStatus,
						r.setStatusUnreadyNotificationCreationFailed, err,
						"Failed to create user deactivation notification")
				}

				if err := r.setStatusToolchainStatusUnreadyNotificationCreated(reqLogger, toolchainStatus); err != nil {
					reqLogger.Error(err, "Failed to update notification created status")
					return err
				}
			}
		}
	}

	return nil
}

// synchronizeWithCounter synchronizes the ToolchainStatus with the cached counter
func (r *ReconcileToolchainStatus) synchronizeWithCounter(reqLogger logr.Logger, toolchainStatus *toolchainv1alpha1.ToolchainStatus) bool {
	if err := counter.Synchronize(r.client, toolchainStatus); err != nil {
		reqLogger.Error(err, "unable to synchronize with the counter")
		return false
	}
	return true
}

// hostOperatorHandleStatus retrieves the Deployment for the host operator and adds its status to ToolchainStatus. It returns false
// if the deployment is not determined to be ready
func (r *ReconcileToolchainStatus) hostOperatorHandleStatus(reqLogger logr.Logger, toolchainStatus *toolchainv1alpha1.ToolchainStatus) bool {
	operatorStatus := &toolchainv1alpha1.HostOperatorStatus{
		Version:        version.Version,
		Revision:       version.Commit,
		BuildTimestamp: version.BuildTime,
	}
	if toolchainStatus.Status.HostOperator != nil {
		operatorStatus.MasterUserRecordCount = toolchainStatus.Status.HostOperator.MasterUserRecordCount
	}

	// look up name of the host operator deployment
	hostOperatorDeploymentName, err := k8sutil.GetOperatorName()
	if err != nil {
		reqLogger.Error(err, status.ErrMsgCannotGetDeployment)
		errCondition := status.NewComponentErrorCondition(toolchainv1alpha1.ToolchainStatusDeploymentNotFoundReason,
			fmt.Sprintf("%s: %s", status.ErrMsgCannotGetDeployment, err.Error()))
		operatorStatus.Conditions = []toolchainv1alpha1.Condition{*errCondition}
		toolchainStatus.Status.HostOperator = operatorStatus
		return false
	}
	operatorStatus.DeploymentName = hostOperatorDeploymentName

	// check host operator deployment status
	deploymentConditions := status.GetDeploymentStatusConditions(r.client, hostOperatorDeploymentName, toolchainStatus.Namespace)
	err = status.ValidateComponentConditionReady(deploymentConditions...)
	if err != nil {
		reqLogger.Error(err, "host operator deployment is not ready")
	}
	// update toolchainStatus
	operatorStatus.Conditions = deploymentConditions
	toolchainStatus.Status.HostOperator = operatorStatus
	return err == nil
}

// registrationServiceHandleStatus retrieves the Deployment for the registration service and adds its status to ToolchainStatus. It returns false
// if the registration service is not ready
func (r *ReconcileToolchainStatus) registrationServiceHandleStatus(reqLogger logr.Logger, toolchainStatus *toolchainv1alpha1.ToolchainStatus) bool {

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

	ready := true
	// call each of the registration service status handlers
	for _, statusHandler := range substatusHandlers {
		ready = statusHandler(reqLogger, toolchainStatus) && ready
	}

	return ready
}

// memberHandleStatus retrieves the status of member clusters and adds them to ToolchainStatus. It returns an error
// if any of the members are not ready or if no member clusters are found
func (r *ReconcileToolchainStatus) membersHandleStatus(logger logr.Logger, toolchainStatus *toolchainv1alpha1.ToolchainStatus) bool {
	// get member clusters
	logger.Info("updating member status")
	memberClusters := r.getMembersFunc()
	members := map[string]toolchainv1alpha1.MemberStatusStatus{}
	ready := true
	if len(memberClusters) == 0 {
		err := fmt.Errorf("no member clusters found")
		logger.Error(err, "number of member clusters is zero")
		ready = false
	}
	for _, memberCluster := range memberClusters {
		memberStatusObj := &toolchainv1alpha1.MemberStatus{}
		err := memberCluster.Client.Get(context.TODO(), types.NamespacedName{Namespace: memberCluster.OperatorNamespace, Name: memberStatusName}, memberStatusObj)
		if err != nil {
			// couldn't find the memberstatus resource on the member cluster, create a status condition and add it to this member's status
			logger.Error(err, fmt.Sprintf("cannot find memberstatus resource in namespace %s in cluster %s", memberCluster.OperatorNamespace, memberCluster.Name))
			memberStatusNotFoundCondition := status.NewComponentErrorCondition(toolchainv1alpha1.ToolchainStatusMemberStatusNotFoundReason, err.Error())
			memberStatus := customMemberStatus(*memberStatusNotFoundCondition)
			members[memberCluster.Name] = memberStatus
			ready = false
			continue
		}

		// take only the overall conditions, resource usage and routes from the MemberStatus
		// (ie. don't include status of member operator, host connection, etc.) in order to keep readability.
		// member status details can be viewed on the member cluster directly.
		memberStatus := toolchainv1alpha1.MemberStatusStatus{
			Conditions:    memberStatusObj.Status.Conditions,
			ResourceUsage: memberStatusObj.Status.ResourceUsage,
			Routes:        memberStatusObj.Status.Routes,
		}
		if condition.IsNotTrue(memberStatusObj.Status.Conditions, toolchainv1alpha1.ConditionReady) {
			// the memberstatus is not ready so set the component error to bubble up the error to the overall toolchain status
			logger.Error(fmt.Errorf("member cluster %s not ready", memberCluster.Name), "the memberstatus ready condition is not true")
			ready = false
		}
		log.Info("adding member status", "member_name", memberCluster.Name, string(memberStatus.Conditions[0].Type), memberStatus.Conditions[0].Status)
		members[memberCluster.Name] = memberStatus
	}

	// add member cluster statuses to toolchainstatus, and assign apiEndpoint in members
	ready = compareAndAssignMemberStatuses(logger, toolchainStatus, members,memberClusters) && ready
	return ready
}

func getApiEndpoint(clusterName string, memberClusters []*cluster.CachedToolchainCluster) string {
	for _, memberCluster := range memberClusters {
			if memberCluster.Name == clusterName {
				return memberCluster.APIEndpoint
			}
	}
	return ""
}
func (r *ReconcileToolchainStatus) sendToolchainStatusUnreadyNotification(logger logr.Logger,
	toolchainStatus *toolchainv1alpha1.ToolchainStatus) error {

	if !isValidEmailAddress(r.config.GetAdminEmail()) {
		return errs.New(fmt.Sprintf("cannot create notification due to configuration error - admin.email [%s] is invalid or not set",
			r.config.GetAdminEmail()))
	}
	toolchainStatus = toolchainStatus.DeepCopy()
	toolchainStatus.ManagedFields = nil // we don't need these managed fields in the notification
	statusYaml, err := yaml.Marshal(toolchainStatus)
	if err != nil {
		return err
	}

	tsValue := time.Now().Format("20060102150405")

	notification := &toolchainv1alpha1.Notification{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("toolchainstatus-unready-%s", tsValue),
			Namespace: toolchainStatus.Namespace,
		},
		Spec: toolchainv1alpha1.NotificationSpec{
			Recipient: r.config.GetAdminEmail(),
			Subject:   adminUnreadyNotificationSubject,
			Content:   "<div><pre><code>" + string(statusYaml) + "</code></pre></div>", // wrap with div/pre/code tags so the formatting remains intact in the delivered mail
		},
	}

	if err := controllerutil.SetControllerReference(toolchainStatus, notification, r.scheme); err != nil {
		logger.Error(err, "Failed to set owner reference for toolchain status unready notification resource")
		return err
	}

	if err := r.client.Create(context.TODO(), notification); err != nil {
		logger.Error(err, "Failed to create toolchain status unready notification resource")
		return err
	}

	logger.Info("Toolchain status unready notification resource created")
	return nil
}

func compareAndAssignMemberStatuses(logger logr.Logger, toolchainStatus *toolchainv1alpha1.ToolchainStatus, members map[string]toolchainv1alpha1.MemberStatusStatus, memberClusters []*cluster.CachedToolchainCluster) bool {
	allOk := true
	for index, member := range toolchainStatus.Status.Members {
		newMemberStatus, ok := members[member.ClusterName]
		apiEndpoint := getApiEndpoint(member.ClusterName,memberClusters)
		if apiEndpoint != ""{
			toolchainStatus.Status.Members[index].ApiEndpoint = apiEndpoint
		}

		if ok {
			toolchainStatus.Status.Members[index].MemberStatus = newMemberStatus
			delete(members, member.ClusterName)
		} else if member.UserAccountCount > 0 {
			err := fmt.Errorf("ToolchainCluster CR wasn't found for member cluster `%s` that was previously registered in the host", member.ClusterName)
			logger.Error(err, "the member cluster seems to be removed")
			memberStatusNotFoundCondition := status.NewComponentErrorCondition(toolchainv1alpha1.ToolchainStatusMemberToolchainClusterMissingReason, err.Error())
			toolchainStatus.Status.Members[index].MemberStatus = customMemberStatus(*memberStatusNotFoundCondition)
			allOk = false
		} else {
			toolchainStatus.Status.Members = append(toolchainStatus.Status.Members[:index], toolchainStatus.Status.Members[index+1:]...)
		}
	}
	for clusterName, memberStatus := range members {
		apiEndpoint := getApiEndpoint(clusterName,memberClusters)
		toolchainStatus.Status.Members = append(toolchainStatus.Status.Members, toolchainv1alpha1.Member{
			ApiEndpoint: apiEndpoint,
			ClusterName:      clusterName,
			MemberStatus:     memberStatus,
			UserAccountCount: 0,
		})
		logger.Info("added member status", "cluster_name", clusterName)
	}
	return allOk
}

// updateStatusConditions updates ToolchainStatus status conditions with new conditions
// the controller should always update at least the last updated timestamp of the status so the status should be updated
// regardless of whether any specific fields were updated. This way a problem with the controller can be indicated if
// the last updated timestamp was not updated.
func (r *ReconcileToolchainStatus) updateStatusConditions(logger logr.Logger, status *toolchainv1alpha1.ToolchainStatus,
	newConditions ...toolchainv1alpha1.Condition) error {
	status.Status.Conditions = condition.AddOrUpdateStatusConditionsWithLastUpdatedTimestamp(status.Status.Conditions, newConditions...)
	logger.Info("updating ToolchainStatus status conditions", "resource_version", status.ResourceVersion)
	err := r.client.Status().Update(context.TODO(), status)
	logger.Info("updated ToolchainStatus status conditions", "resource_version", status.ResourceVersion)
	return err
}

// wrapErrorWithStatusUpdate wraps the error and update the UserSignup status. If the update fails then the error is logged.
func (r *ReconcileToolchainStatus) wrapErrorWithStatusUpdate(logger logr.Logger, toolchainStatus *toolchainv1alpha1.ToolchainStatus,
	statusUpdater func(reqLogger logr.Logger, userAcc *toolchainv1alpha1.ToolchainStatus, message string) error, err error, format string,
	args ...interface{}) error {
	if err == nil {
		return nil
	}
	if err := statusUpdater(logger, toolchainStatus, err.Error()); err != nil {
		logger.Error(err, "Error updating ToolchainStatus status")
	}
	return errs.Wrapf(err, format, args...)
}

func (r *ReconcileToolchainStatus) setStatusReady(reqLogger logr.Logger, toolchainStatus *toolchainv1alpha1.ToolchainStatus) error {
	return r.updateStatusConditions(
		reqLogger,
		toolchainStatus,
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.ConditionReady,
			Status: corev1.ConditionTrue,
			Reason: toolchainv1alpha1.ToolchainStatusAllComponentsReadyReason,
		},
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.ToolchainStatusUnreadyNotificationCreated,
			Status: corev1.ConditionFalse,
			Reason: toolchainv1alpha1.ToolchainStatusAllComponentsReadyReason,
		})
}

func (r *ReconcileToolchainStatus) setStatusNotReady(reqLogger logr.Logger, toolchainStatus *toolchainv1alpha1.ToolchainStatus, message string) error {
	return r.updateStatusConditions(
		reqLogger,
		toolchainStatus,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.ConditionReady,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.ToolchainStatusComponentsNotReadyReason,
			Message: message,
		})
}

func (r *ReconcileToolchainStatus) setStatusToolchainStatusUnreadyNotificationCreated(reqLogger logr.Logger,
	toolchainStatus *toolchainv1alpha1.ToolchainStatus) error {

	return r.updateStatusConditions(
		reqLogger,
		toolchainStatus,
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.ToolchainStatusUnreadyNotificationCreated,
			Status: corev1.ConditionTrue,
			Reason: toolchainv1alpha1.ToolchainStatusUnreadyNotificationCRCreatedReason,
		})
}

func (r *ReconcileToolchainStatus) setStatusUnreadyNotificationCreationFailed(reqLogger logr.Logger,
	toolchainStatus *toolchainv1alpha1.ToolchainStatus, message string) error {

	return r.updateStatusConditions(
		reqLogger,
		toolchainStatus,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.ConditionReady,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.ToolchainStatusUnreadyNotificationCRCreationFailedReason,
			Message: message,
		})
}

func customMemberStatus(conditions ...toolchainv1alpha1.Condition) toolchainv1alpha1.MemberStatusStatus {
	return toolchainv1alpha1.MemberStatusStatus{
		Conditions: conditions,
	}
}

type regServiceSubstatusHandler struct {
	httpClientImpl   httpClient
	controllerClient client.Client
}

// addRegistrationServiceResourceStatus handles the RegistrationService.RegistrationServiceResources part of the toolchainstatus
func (s regServiceSubstatusHandler) addRegistrationServiceResourceStatus(reqLogger logr.Logger, toolchainStatus *toolchainv1alpha1.ToolchainStatus) bool {
	// get the registrationservice resource
	registrationServiceName := types.NamespacedName{Namespace: toolchainStatus.Namespace, Name: registrationservice.ResourceName}
	registrationService := &toolchainv1alpha1.RegistrationService{}
	err := s.controllerClient.Get(context.TODO(), registrationServiceName, registrationService)
	if err != nil {
		reqLogger.Error(err, "unable to get the registrationservice resource")
		errCondition := status.NewComponentErrorCondition(toolchainv1alpha1.ToolchainStatusRegServiceResourceNotFoundReason, err.Error())
		toolchainStatus.Status.RegistrationService.RegistrationServiceResources.Conditions = []toolchainv1alpha1.Condition{*errCondition}
		return false
	}

	// use the registrationservice resource directly in the toolchainstatus
	toolchainStatus.Status.RegistrationService.RegistrationServiceResources.Conditions = registrationService.Status.Conditions
	if condition.IsNotTrue(registrationService.Status.Conditions, toolchainv1alpha1.ConditionReady) {
		reqLogger.Error(fmt.Errorf("deployment is not ready"), "the registrationservice resource is not ready")
		return false
	}

	return true
}

// addRegistrationServiceDeploymentStatus handles the RegistrationService.Deployment part of the toolchainstatus
func (s regServiceSubstatusHandler) addRegistrationServiceDeploymentStatus(reqLogger logr.Logger, toolchainStatus *toolchainv1alpha1.ToolchainStatus) bool {
	deploymentConditions := status.GetDeploymentStatusConditions(s.controllerClient, registrationservice.ResourceName, toolchainStatus.Namespace)
	toolchainStatus.Status.RegistrationService.Deployment.Name = registrationservice.ResourceName
	toolchainStatus.Status.RegistrationService.Deployment.Conditions = deploymentConditions

	err := status.ValidateComponentConditionReady(deploymentConditions...)
	if err != nil {
		reqLogger.Error(err, "a problem was detected in the deployment status")
		return false
	}

	return true
}

// addRegistrationServiceHealthStatus handles the RegistrationService.Health part of the toolchainstatus
func (s regServiceSubstatusHandler) addRegistrationServiceHealthStatus(reqLogger logr.Logger, toolchainStatus *toolchainv1alpha1.ToolchainStatus) bool {
	// get the JSON payload from the health endpoint
	resp, err := s.httpClientImpl.Get(registrationServiceHealthURL)
	if err != nil {
		reqLogger.Error(err, errMsgRegistrationServiceNotReady)
		errCondition := status.NewComponentErrorCondition(toolchainv1alpha1.ToolchainStatusRegServiceNotReadyReason, err.Error())
		toolchainStatus.Status.RegistrationService.Health.Conditions = []toolchainv1alpha1.Condition{*errCondition}
		return false
	}

	// bad response
	if resp.StatusCode != http.StatusOK {
		err = fmt.Errorf("bad response from %s : statusCode=%d", registrationServiceHealthURL, resp.StatusCode)
		reqLogger.Error(err, errMsgRegistrationServiceNotReady)
		errCondition := status.NewComponentErrorCondition(toolchainv1alpha1.ToolchainStatusRegServiceNotReadyReason, err.Error())
		toolchainStatus.Status.RegistrationService.Health.Conditions = []toolchainv1alpha1.Condition{*errCondition}
		return false
	}

	// decode the response to JSON
	defer func() {
		if _, err := ioutil.ReadAll(resp.Body); err != nil {
			reqLogger.Error(err, "unable to read the response")
		}
		if err := resp.Body.Close(); err != nil {
			reqLogger.Error(err, "unable to close the body")
		}
	}()
	healthValues := status.Health{}
	err = json.NewDecoder(resp.Body).Decode(&healthValues)
	if err != nil {
		reqLogger.Error(err, errMsgRegistrationServiceNotReady)
		errCondition := status.NewComponentErrorCondition(toolchainv1alpha1.ToolchainStatusRegServiceNotReadyReason, err.Error())
		toolchainStatus.Status.RegistrationService.Health.Conditions = []toolchainv1alpha1.Condition{*errCondition}
		return false
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
		reqLogger.Error(err, "registration service is unhealthy")
		errCondition := status.NewComponentErrorCondition(toolchainv1alpha1.ToolchainStatusRegServiceNotReadyReason, err.Error())
		toolchainStatus.Status.RegistrationService.Health.Conditions = []toolchainv1alpha1.Condition{*errCondition}
		return false
	}

	componentReadyCondition := status.NewComponentReadyCondition(toolchainv1alpha1.ToolchainStatusRegServiceReadyReason)
	toolchainStatus.Status.RegistrationService.Health.Conditions = []toolchainv1alpha1.Condition{*componentReadyCondition}
	return true
}

func isValidEmailAddress(email string) bool {
	if len(email) < 3 && len(email) > 254 {
		return false
	}
	return emailRegex.MatchString(email)
}
