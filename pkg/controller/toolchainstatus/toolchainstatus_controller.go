package toolchainstatus

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	crtCfg "github.com/codeready-toolchain/host-operator/pkg/configuration"
	"github.com/codeready-toolchain/host-operator/pkg/controller/registrationservice"
	"github.com/codeready-toolchain/host-operator/version"
	"github.com/codeready-toolchain/toolchain-common/pkg/cluster"
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

// error messages & reasons
const (
	reasonNotFound  = "NotFound"
	reasonUnhealthy = "Unhealthy"

	errMsgCannotGetRegistrationService                     = "unable to get the registrationservice resource"
	errMsgCannotGetRegistrationServiceDeploymentConditions = "unable to determine the registration service deployment status"
	errMsgRegistrationServiceNotReady                      = "the registration service is not ready"
	errMsgRegistrationServiceHealthStatusUnhealthy         = "the registration service health endpoint is reporting an unhealthy status"

	errMsgCannotGetMemberStatus = "unable to get the member cluster status"
)

type statusComponentTag string

const (
	registrationService statusComponentTag = "registrationService"
	hostOperator        statusComponentTag = "hostOperator"
	memberConnections   statusComponentTag = "memberConnections"
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
		scheme:         mgr.GetScheme(),
		getHostCluster: cluster.GetHostCluster,
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

// ReconcileToolchainStatus reconciles a ToolchainStatus object
type ReconcileToolchainStatus struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client         client.Client
	scheme         *runtime.Scheme
	getHostCluster func() (*cluster.FedCluster, bool)
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
	handleStatus func(logger logr.Logger, toolchainStatus *toolchainv1alpha1.ToolchainStatus) error
}

// aggregateAndUpdateStatus runs each of the status handlers. Each status handler reports readiness for a toolchain component. If any
// component status is not ready then it will set the condition of the top-level status of the ToolchainStatus resource to not ready.
func (r *ReconcileToolchainStatus) aggregateAndUpdateStatus(reqLogger logr.Logger, toolchainStatus *toolchainv1alpha1.ToolchainStatus) error {
	hostOperatorStatusHandler := statusHandler{name: hostOperator, handleStatus: r.hostOperatorHandleStatus}
	registrationServiceStatusHandler := statusHandler{name: registrationService, handleStatus: r.registrationServiceHandleStatus}
	memberStatusHandler := statusHandler{name: memberConnections, handleStatus: r.memberHandleStatus}

	statusHandlers := []statusHandler{
		registrationServiceStatusHandler,
		hostOperatorStatusHandler,
		memberStatusHandler,
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
// if any of the conditions have a status that is not 'true'
func (r *ReconcileToolchainStatus) hostOperatorHandleStatus(reqLogger logr.Logger, toolchainStatus *toolchainv1alpha1.ToolchainStatus) error {
	operatorStatus := toolchainv1alpha1.HostOperatorStatus{
		Version:        version.Version,
		Revision:       version.Commit,
		BuildTimestamp: version.BuildTime,
	}

	// look up status of host deployment
	hostOperatorDeploymentName, err := k8sutil.GetOperatorName()
	if err != nil {
		err = errs.Wrap(err, status.ErrMsgCannotGetDeployment)
		errCondition := status.NewComponentErrorCondition(toolchainv1alpha1.ToolchainStatusReasonNoDeployment, err.Error())
		operatorStatus.Conditions = []toolchainv1alpha1.Condition{*errCondition}
		toolchainStatus.Status.HostOperator = operatorStatus
		return err
	}
	operatorStatus.Deployment.Name = hostOperatorDeploymentName

	// check host operator deployment status
	deploymentConditions, err := status.GetDeploymentStatusConditions(r.client, hostOperatorDeploymentName, toolchainStatus.Namespace)

	// update toolchainStatus
	operatorStatus.Conditions = deploymentConditions
	toolchainStatus.Status.HostOperator = operatorStatus
	return err
}

// registrationServiceHandleStatus retrieves the Deployment for the registration service and adds its status to ToolchainStatus. It returns an error
// if any of the conditions have a status that is not 'true'
func (r *ReconcileToolchainStatus) registrationServiceHandleStatus(reqLogger logr.Logger, toolchainStatus *toolchainv1alpha1.ToolchainStatus) (err error) {
	// look up status of registrationservice resource status
	resourceErr := addRegistrationServiceResourceStatus(toolchainStatus, r.client)
	if resourceErr != nil {
		err = resourceErr
	}

	// add registrationservice deployment status
	deploymentErr := addRegistrationServiceDeploymentStatus(toolchainStatus, r.client)
	if deploymentErr != nil {
		err = deploymentErr
	}

	// add health endpoint status
	healthErr := addRegistrationServiceHealthStatus(toolchainStatus, r.client)
	if healthErr != nil {
		err = healthErr
	}

	return err
}

// memberHandleStatus retrieves the status of member clusters and adds them to ToolchainStatus. It returns an error
// if any of the conditions have a status that is not 'true' or if no member clusters are found.
func (r *ReconcileToolchainStatus) memberHandleStatus(reqLogger logr.Logger, toolchainStatus *toolchainv1alpha1.ToolchainStatus) error {
	// look up member clusters
	memberClusters := cluster.GetMemberClusters()
	members := []toolchainv1alpha1.Member{}
	for _, cluster := range memberClusters {
		memberClient := cluster.Client
		if memberClient == nil {
			msg := errs.Wrap(fmt.Errorf(errMsgCannotGetMemberStatus), "member cluster does not have a client assigned")
			notFoundCondition := status.NewComponentErrorCondition(reasonNotFound, msg.Error())
			memberStatus := customMemberStatus(*notFoundCondition)
			members = append(members, memberResult(cluster, memberStatus))
			continue
		}
		memberStatusObj := &toolchainv1alpha1.MemberStatus{}
		err := memberClient.Get(context.TODO(), types.NamespacedName{cluster.OperatorNamespace, memberStatusName}, memberStatusObj)
		if err != nil {
			wrappedErr := errs.Wrap(err, fmt.Sprintf("cannot find memberstatus resource in namespace %s", cluster.OperatorNamespace))
			memberStatusNotFoundCondition := status.NewComponentErrorCondition(reasonNotFound, wrappedErr.Error())
			memberStatus := customMemberStatus(*memberStatusNotFoundCondition)
			members = append(members, memberResult(cluster, memberStatus))
			continue
		}

		// take only the overall conditions from the memberstatus (ie. don't include status of member operator, host connection, etc.) in order to keep readability.
		// member status details can be viewed on the member cluster directly.
		// memberStatus := toolchainv1alpha1.MemberStatusStatus{Conditions: memberStatusObj.Status.Conditions}
		// members = append(members, memberResult(cluster, memberStatus))
		members = append(members, memberResult(cluster, memberStatusObj.Status))
	}

	// add member cluster statuses to toolchainstatus
	toolchainStatus.Status.Members = members
	return nil
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
			Reason: toolchainv1alpha1.ToolchainStatusReasonAllComponentsReady,
		})
}

func (r *ReconcileToolchainStatus) setStatusNotReady(toolchainStatus *toolchainv1alpha1.ToolchainStatus, message string) error {
	return r.updateStatusConditions(
		toolchainStatus,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.ConditionReady,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.ToolchainStatusReasonComponentsNotReady,
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
		ClusterID:    cluster.Name,
		MemberStatus: memberStatus,
	}
}

func addRegistrationServiceResourceStatus(toolchainStatus *toolchainv1alpha1.ToolchainStatus, client client.Client) error {
	registrationServiceName := types.NamespacedName{Namespace: toolchainStatus.Namespace, Name: registrationservice.ResourceName}
	registrationService := &toolchainv1alpha1.RegistrationService{}
	err := client.Get(context.TODO(), registrationServiceName, registrationService)
	if err != nil {
		err = errs.Wrap(err, errMsgCannotGetRegistrationService)
		errCondition := status.NewComponentErrorCondition(reasonNotFound, err.Error())
		toolchainStatus.Status.RegistrationService.RegistrationServiceResources.Conditions = []toolchainv1alpha1.Condition{*errCondition}
		return err
	}
	toolchainStatus.Status.RegistrationService.RegistrationServiceResources.Conditions = registrationService.Status.Conditions
	return nil
}

func addRegistrationServiceDeploymentStatus(toolchainStatus *toolchainv1alpha1.ToolchainStatus, client client.Client) error {
	deploymentConditions, err := status.GetDeploymentStatusConditions(client, registrationservice.ResourceName, toolchainStatus.Namespace)
	if err != nil {
		err = errs.Wrap(err, errMsgCannotGetRegistrationServiceDeploymentConditions)
		errCondition := status.NewComponentErrorCondition(reasonNotFound, err.Error())
		toolchainStatus.Status.RegistrationService.Deployment.Conditions = []toolchainv1alpha1.Condition{*errCondition}
		return err
	}

	toolchainStatus.Status.RegistrationService.Deployment.Name = registrationservice.ResourceName
	toolchainStatus.Status.RegistrationService.Deployment.Conditions = deploymentConditions
	return nil
}

func addRegistrationServiceHealthStatus(toolchainStatus *toolchainv1alpha1.ToolchainStatus, client client.Client) error {
	registrationServiceHealthURI := "http://registration-service/" + registrationServiceHealthEndpoint
	resp, err := http.Get(registrationServiceHealthURI)
	if err != nil {
		err = errs.Wrap(err, errMsgRegistrationServiceNotReady)
		errCondition := status.NewComponentErrorCondition(reasonNotFound, err.Error())
		toolchainStatus.Status.RegistrationService.Health.Conditions = []toolchainv1alpha1.Condition{*errCondition}
		return err
	}

	defer resp.Body.Close()
	healthValues := status.Health{}
	err = json.NewDecoder(resp.Body).Decode(&healthValues)
	if err != nil {
		err = errs.Wrap(err, errMsgRegistrationServiceNotReady)
		errCondition := status.NewComponentErrorCondition(reasonNotFound, err.Error())
		toolchainStatus.Status.RegistrationService.Health.Conditions = []toolchainv1alpha1.Condition{*errCondition}
		return err
	}

	healthStatus := toolchainv1alpha1.RegistrationServiceHealth{
		Alive:       fmt.Sprintf("%t", healthValues.Alive),
		BuildTime:   healthValues.BuildTime,
		Environment: healthValues.Environment,
		Revision:    healthValues.Revision,
		StartTime:   healthValues.StartTime,
	}
	toolchainStatus.Status.RegistrationService.Health = healthStatus
	if !healthValues.Alive {
		errCondition := status.NewComponentErrorCondition(reasonUnhealthy, errMsgRegistrationServiceHealthStatusUnhealthy)
		toolchainStatus.Status.RegistrationService.Health.Conditions = []toolchainv1alpha1.Condition{*errCondition}
		return fmt.Errorf(errMsgRegistrationServiceHealthStatusUnhealthy)
	}

	componentReadyCondition := status.NewComponentReadyCondition(toolchainv1alpha1.ToolchainStatusReasonRegServiceReady)
	toolchainStatus.Status.RegistrationService.Health.Conditions = []toolchainv1alpha1.Condition{*componentReadyCondition}
	return nil
}
