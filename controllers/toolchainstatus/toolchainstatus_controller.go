package toolchainstatus

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"text/template"
	"time"

	"github.com/codeready-toolchain/toolchain-common/pkg/client"
	"github.com/go-logr/logr"
	routev1 "github.com/openshift/api/route/v1"

	"github.com/codeready-toolchain/host-operator/controllers/toolchainconfig"
	notify "github.com/codeready-toolchain/toolchain-common/pkg/notification"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/counter"
	"github.com/codeready-toolchain/host-operator/pkg/templates/registrationservice"
	"github.com/codeready-toolchain/host-operator/version"
	"github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	"github.com/codeready-toolchain/toolchain-common/pkg/condition"
	commonconfig "github.com/codeready-toolchain/toolchain-common/pkg/configuration"
	"github.com/codeready-toolchain/toolchain-common/pkg/status"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/log"

	errs "github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// general toolchainstatus constants
const (
	memberStatusName = "toolchain-member-status"
	proxyPrefix      = "api-toolchain-host-operator.apps."

	registrationServiceHealthURL = "http://registration-service/api/v1/health"

	hostOperatorRepoName              = "host-operator"
	registrationServiceRepoName       = "registration-service"
	hostOperatorRepoBranchName        = "master"
	registrationServiceRepoBranchName = hostOperatorRepoBranchName
)

// error messages
const (
	errMsgRegistrationServiceNotReady              = "the registration service is not ready"
	errMsgRegistrationServiceHealthStatusUnhealthy = "the registration service health endpoint is reporting an unhealthy status"
)

type statusComponentTag string

const (
	registrationServiceTag statusComponentTag = "registrationService"
	hostRoutesTag          statusComponentTag = "hostRoutes"
	hostOperatorTag        statusComponentTag = "hostOperator"
	memberConnectionsTag   statusComponentTag = "members"
	counterTag             statusComponentTag = "MasterUserRecord and UserAccount counter"
	durationAfterUnready   time.Duration      = 10 * time.Minute
)

const (
	adminUnreadyNotificationSubject  = "ToolchainStatus has been in an unready status for an extended period for %v"
	adminRestoredNotificationSubject = "ToolchainStatus has now been restored to ready status for %v"
)

type toolchainStatusNotificationType string

const (
	unreadyStatus  toolchainStatusNotificationType = "unready"
	restoredStatus toolchainStatusNotificationType = "restored"
)

const (
	statusNotificationTemplate = `<h3>The following issues have been detected in the ToolchainStatus<h3>
{{range $key, $value := .clusterURLs}}
<div><span style="font-weight:bold;padding-right:10px">{{$key}}:</span>{{$value}}</div>
{{end}}

{{range .components}}
<h4>{{.ComponentName}} {{.ComponentType}} not ready</h4>

<div style="padding-left: 40px">
<div><span style="font-weight:bold;padding-right:10px">Reason:</span>{{.Reason}}</div>
<div><span style="font-weight:bold;padding-right:10px">Message:</span>{{.Message}}</div>
{{range $key, $value := .Details}}
<div><span style="font-weight:bold;padding-right:10px">{{$key}}:</span>{{$value}}</div>
{{end}}
</div>
{{end}}`
)

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr manager.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&toolchainv1alpha1.ToolchainStatus{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Complete(r)
}

type HTTPClient interface {
	Get(url string) (*http.Response, error)
}

// Reconciler reconciles a ToolchainStatus object
type Reconciler struct {
	Client              runtimeclient.Client
	Scheme              *runtime.Scheme
	GetMembersFunc      cluster.GetMemberClustersFunc
	HTTPClientImpl      HTTPClient
	Namespace           string
	VersionCheckManager status.VersionCheckManager
}

//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=toolchainstatuses,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=toolchainstatuses/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=toolchainstatuses/finalizers,verbs=update

// Reconcile reads the state of toolchain host and member cluster components and updates the ToolchainStatus resource with information useful for observation or troubleshooting
func (r *Reconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	reqLogger := log.FromContext(ctx)
	reqLogger.Info("Reconciling ToolchainStatus")

	toolchainConfig, err := toolchainconfig.GetToolchainConfig(r.Client)
	if err != nil {
		return reconcile.Result{}, errs.Wrapf(err, "unable to get ToolchainConfig")
	}
	requeueTime := toolchainConfig.ToolchainStatus().ToolchainStatusRefreshTime()

	// fetch the ToolchainStatus
	toolchainStatus := &toolchainv1alpha1.ToolchainStatus{}
	err = r.Client.Get(ctx, request.NamespacedName, toolchainStatus)

	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			return reconcile.Result{}, nil
		}
		reqLogger.Error(err, "Unable to fetch ToolchainStatus resource")
		return reconcile.Result{}, err
	}

	err = r.aggregateAndUpdateStatus(ctx, toolchainStatus)
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

type statusHandlerFunc func(context.Context, *toolchainv1alpha1.ToolchainStatus) bool

// aggregateAndUpdateStatus runs each of the status handlers. Each status handler reports readiness for a toolchain component. If any
// component status is not ready then it will set the condition of the top-level status of the ToolchainStatus resource to not ready.
func (r *Reconciler) aggregateAndUpdateStatus(ctx context.Context, toolchainStatus *toolchainv1alpha1.ToolchainStatus) error {
	// collect status handlers that will contribute status of various toolchain components
	hostOperatorStatusHandlerFunc := statusHandler{name: hostOperatorTag, handleStatus: r.hostOperatorHandleStatus}
	registrationServiceStatusHandlerFunc := statusHandler{name: registrationServiceTag, handleStatus: r.registrationServiceHandleStatus}
	proxyURLHandlerFunc := statusHandler{name: hostRoutesTag, handleStatus: r.hostRoutesHandleStatus}
	memberStatusHandlerFunc := statusHandler{name: memberConnectionsTag, handleStatus: r.membersHandleStatus}
	// should be executed as the last one
	counterHandlerFunc := statusHandler{name: counterTag, handleStatus: r.synchronizeWithCounter}

	statusHandlers := []statusHandler{
		hostOperatorStatusHandlerFunc,
		memberStatusHandlerFunc,
		registrationServiceStatusHandlerFunc,
		proxyURLHandlerFunc,
		counterHandlerFunc,
	}

	// track components that are not ready
	var unreadyComponents []string

	// retrieve component statuses eg. ToolchainCluster, host deployment
	for _, statusHandler := range statusHandlers {
		isReady := statusHandler.handleStatus(ctx, toolchainStatus)
		if !isReady {
			unreadyComponents = append(unreadyComponents, string(statusHandler.name))
		}
	}

	// if any components were not ready then set the overall status to not ready
	if len(unreadyComponents) > 0 {
		err := r.notificationCheck(ctx, toolchainStatus)
		if err != nil {
			return err
		}

		return r.setStatusNotReady(ctx, toolchainStatus, fmt.Sprintf("components not ready: %v", unreadyComponents))
	}

	return r.setStatusReady(ctx, toolchainStatus)
}

func (r *Reconciler) notificationCheck(ctx context.Context, toolchainStatus *toolchainv1alpha1.ToolchainStatus) error {
	// If the current ToolchainStatus:
	// a) Is currently not ready or the deployments versions are not up-to-date,
	// b) has not been ready for longer than the configured threshold, and
	// c) no notification has been already sent, then
	// send a notification to the admin mailing list
	c, found := condition.FindConditionByType(toolchainStatus.Status.Conditions, toolchainv1alpha1.ConditionReady)
	if found && c.Status == corev1.ConditionFalse {
		threshold := time.Now().Add(-durationAfterUnready)
		if c.LastTransitionTime.Before(&metav1.Time{Time: threshold}) {
			if !condition.IsTrue(toolchainStatus.Status.Conditions, toolchainv1alpha1.ToolchainStatusUnreadyNotificationCreated) {
				logger := log.FromContext(ctx)

				if err := r.sendToolchainStatusNotification(ctx, toolchainStatus, unreadyStatus); err != nil {
					logger.Error(err, "Failed to create toolchain status unready notification")

					// set the failed to create notification status condition
					return r.wrapErrorWithStatusUpdate(ctx, toolchainStatus,
						r.setStatusUnreadyNotificationCreationFailed, err,
						"Failed to create toolchain status unready notification")
				}

				if err := r.setStatusToolchainStatusUnreadyNotificationCreated(ctx, toolchainStatus); err != nil {
					logger.Error(err, "Failed to update notification created status")
					return err
				}
			}
		}
	}

	return nil
}

func (r *Reconciler) restoredCheck(ctx context.Context, toolchainStatus *toolchainv1alpha1.ToolchainStatus) error {
	// If the current ToolchainStatus:
	// a) Was not ready before,
	// b) notification was sent out due prolonged not ready status, and
	// c) status is now restored
	// send a notification to the admin mailing list
	if condition.IsFalse(toolchainStatus.Status.Conditions, toolchainv1alpha1.ConditionReady) {
		if condition.IsTrue(toolchainStatus.Status.Conditions, toolchainv1alpha1.ToolchainStatusUnreadyNotificationCreated) {
			if err := r.sendToolchainStatusNotification(ctx, toolchainStatus, restoredStatus); err != nil {
				logger := log.FromContext(ctx)
				logger.Error(err, "Failed to create toolchain status restored notification")
				// set the failed to create notification status condition
				return r.wrapErrorWithStatusUpdate(ctx, toolchainStatus,
					r.setStatusReadyNotificationCreationFailed, err,
					"Failed to create toolchain restored notification")
			}
		}
	}
	return nil
}

// synchronizeWithCounter synchronizes the ToolchainStatus with the cached counter
func (r *Reconciler) synchronizeWithCounter(ctx context.Context, toolchainStatus *toolchainv1alpha1.ToolchainStatus) bool {
	if err := counter.Synchronize(ctx, r.Client, toolchainStatus); err != nil {
		logger := log.FromContext(ctx)
		logger.Error(err, "unable to synchronize with the counter")
		return false
	}
	return true
}

// hostOperatorHandleStatus retrieves the Deployment for the host operator and adds its status to ToolchainStatus. It returns false
// if the deployment is not determined to be ready
func (r *Reconciler) hostOperatorHandleStatus(ctx context.Context, toolchainStatus *toolchainv1alpha1.ToolchainStatus) bool {
	logger := log.FromContext(ctx)
	// ensure host operator status is set
	if toolchainStatus.Status.HostOperator == nil {
		toolchainStatus.Status.HostOperator = &toolchainv1alpha1.HostOperatorStatus{}
	}

	operatorStatus := &toolchainv1alpha1.HostOperatorStatus{
		Version:        version.Version,
		Revision:       version.Commit,
		BuildTimestamp: version.BuildTime,
		RevisionCheck:  toolchainStatus.Status.HostOperator.RevisionCheck, // let's copy the last revision check object if any
	}
	// look up name of the host operator deployment
	hostOperatorName, errDeploy := commonconfig.GetOperatorName()
	if errDeploy != nil {
		logger.Error(errDeploy, status.ErrMsgCannotGetDeployment)
		errCondition := status.NewComponentErrorCondition(toolchainv1alpha1.ToolchainStatusDeploymentNotFoundReason,
			fmt.Sprintf("%s: %s", status.ErrMsgCannotGetDeployment, errDeploy.Error()))
		operatorStatus.Conditions = []toolchainv1alpha1.Condition{*errCondition}
		toolchainStatus.Status.HostOperator = operatorStatus
		return false
	}
	hostOperatorDeploymentName := fmt.Sprintf("%s-controller-manager", hostOperatorName)
	operatorStatus.DeploymentName = hostOperatorDeploymentName

	// save host operator deployment check status in this flag, it will be returned in case no errors are found by the following checks.
	allOK := true
	// check host operator deployment status
	deploymentConditions := status.GetDeploymentStatusConditions(ctx, r.Client, hostOperatorDeploymentName, toolchainStatus.Namespace)
	errDeploy = status.ValidateComponentConditionReady(deploymentConditions...)
	if errDeploy != nil {
		logger.Error(errDeploy, "host operator deployment is not ready")
		allOK = false
	}
	operatorStatus.Conditions = deploymentConditions

	// if we are running in production we also
	// check that deployed version matches the latest commit from source code repository.
	// we also need to check when we called GitHub api last time, in order to avoid rate limiting issues.
	toolchainConfig, errToolchainConfig := toolchainconfig.GetToolchainConfig(r.Client)
	if errToolchainConfig != nil {
		logger.Error(errToolchainConfig, "unable to get toolchainconfig")
		errCondition := status.NewComponentErrorCondition(toolchainv1alpha1.ToolchainStatusDeploymentRevisionCheckOperatorErrorReason,
			fmt.Sprintf("unable to get ToolchainConfig: %s", errToolchainConfig.Error()))
		operatorStatus.RevisionCheck.Conditions = []toolchainv1alpha1.Condition{*errCondition}
		toolchainStatus.Status.HostOperator = operatorStatus
		return false
	}
	isProd := isProdEnvironment(toolchainConfig)
	githubRepo := client.GitHubRepository{
		Org:               toolchainv1alpha1.ProviderLabelValue,
		Name:              hostOperatorRepoName,
		Branch:            hostOperatorRepoBranchName,
		DeployedCommitSHA: version.Commit,
	}

	// verify deployment version
	versionCondition := r.VersionCheckManager.CheckDeployedVersionIsUpToDate(ctx, isProd, toolchainConfig.GitHubSecret().AccessTokenKey(), toolchainStatus.Status.HostOperator.RevisionCheck.Conditions, githubRepo)
	errVersionCheck := status.ValidateComponentConditionReady(*versionCondition)
	if errVersionCheck != nil {
		// let's set deployment is not up-to-date reason
		logger.Error(errVersionCheck, "host operator deployment is not up to date")
		allOK = false
	}
	operatorStatus.RevisionCheck.Conditions = []toolchainv1alpha1.Condition{*versionCondition}
	toolchainStatus.Status.HostOperator = operatorStatus
	return allOK
}

// check if we are running in a production environment
func isProdEnvironment(toolchainConfig toolchainconfig.ToolchainConfig) bool {
	return toolchainConfig.Environment() == "prod"
}

// registrationServiceHandleStatus retrieves the Deployment for the registration service and adds its status to ToolchainStatus. It returns false
// if the registration service is not ready
func (r *Reconciler) registrationServiceHandleStatus(ctx context.Context, toolchainStatus *toolchainv1alpha1.ToolchainStatus) bool {

	s := regServiceSubstatusHandler{
		controllerClient:    r.Client,
		httpClientImpl:      r.HTTPClientImpl,
		versionCheckManager: r.VersionCheckManager,
	}

	// gather the functions for handling registration service status eg. deployment, health endpoint
	substatusHandlers := []statusHandlerFunc{
		s.addRegistrationServiceDeploymentStatus,
		s.addRegistrationServiceHealthAndRevisionCheckStatus,
	}

	// ensure the registrationservice part of the status is created
	if toolchainStatus.Status.RegistrationService == nil {
		toolchainStatus.Status.RegistrationService = &toolchainv1alpha1.HostRegistrationServiceStatus{}
	}

	ready := true
	// call each of the registration service status handlers
	for _, statusHandler := range substatusHandlers {
		ready = statusHandler(ctx, toolchainStatus) && ready
	}

	return ready
}

// hostRoutesHandleStatus retrieves the public routes which should be exposed to the users. Such as Proxy URL.
// Returns false if any route is not available.
func (r *Reconciler) hostRoutesHandleStatus(ctx context.Context, toolchainStatus *toolchainv1alpha1.ToolchainStatus) bool {
	proxyURL, err := r.proxyURL(ctx)
	if err != nil {
		logger := log.FromContext(ctx)
		logger.Error(err, "Proxy route was not found")
		errCondition := status.NewComponentErrorCondition(toolchainv1alpha1.ToolchainStatusProxyRouteUnavailableReason, err.Error())
		toolchainStatus.Status.HostRoutes.Conditions = []toolchainv1alpha1.Condition{*errCondition}
		return false
	}
	toolchainStatus.Status.HostRoutes.ProxyURL = proxyURL

	readyCondition := status.NewComponentReadyCondition(toolchainv1alpha1.ToolchainStatusHostRoutesAvailableReason)
	toolchainStatus.Status.HostRoutes.Conditions = []toolchainv1alpha1.Condition{*readyCondition}

	return true
}

func (r *Reconciler) proxyURL(ctx context.Context) (string, error) {
	route := &routev1.Route{}
	namespacedName := types.NamespacedName{Namespace: r.Namespace, Name: registrationservice.ProxyRouteName}
	if err := r.Client.Get(ctx, namespacedName, route); err != nil {
		return "", err
	}

	scheme := "https"
	if route.Spec.TLS == nil || *route.Spec.TLS == (routev1.TLSConfig{}) {
		scheme = "http"
	}
	routeURL := &url.URL{
		Scheme: scheme,
		Host:   route.Spec.Host,
		Path:   route.Spec.Path,
	}
	return routeURL.String(), nil
}

// memberHandleStatus retrieves the status of member clusters and adds them to ToolchainStatus. It returns an error
// if any of the members are not ready or if no member clusters are found
func (r *Reconciler) membersHandleStatus(ctx context.Context, toolchainStatus *toolchainv1alpha1.ToolchainStatus) bool {
	logger := log.FromContext(ctx)

	// get member clusters
	logger.Info("updating member status")
	memberClusters := r.GetMembersFunc()
	members := map[string]toolchainv1alpha1.MemberStatusStatus{}
	ready := true
	if len(memberClusters) == 0 {
		err := fmt.Errorf("no member clusters found")
		logger.Error(err, "number of member clusters is zero")
		ready = false
	}
	for _, memberCluster := range memberClusters {
		memberStatusObj := &toolchainv1alpha1.MemberStatus{}
		err := memberCluster.Client.Get(ctx, types.NamespacedName{Namespace: memberCluster.OperatorNamespace, Name: memberStatusName}, memberStatusObj)
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

		readyCond, found := condition.FindConditionByType(memberStatusObj.Status.Conditions, toolchainv1alpha1.ConditionReady)
		if !found || readyCond.Status != corev1.ConditionTrue {
			// the memberstatus is not ready so set the component error to bubble up the error to the overall toolchain status
			logger.Error(fmt.Errorf("member cluster %s not ready", memberCluster.Name), "the memberstatus ready condition is not true")
			ready = false
		}
		if found {
			logger.Info("adding member status", "member_name", memberCluster.Name, string(toolchainv1alpha1.ConditionReady), readyCond.Status)
		} else {
			logger.Info("adding member status", "member_name", memberCluster.Name, string(toolchainv1alpha1.ConditionReady), "unknown")
		}
		members[memberCluster.Name] = memberStatus
	}

	// add member cluster statuses to toolchainstatus, and assign apiEndpoint in members
	ready = compareAndAssignMemberStatuses(ctx, toolchainStatus, members, memberClusters) && ready
	return ready
}

func getAPIEndpoint(clusterName string, memberClusters []*cluster.CachedToolchainCluster) string {
	for _, memberCluster := range memberClusters {
		if memberCluster.Name == clusterName {
			return memberCluster.APIEndpoint
		}
	}
	return ""
}

func removeSchemeFromURL(proxyURL string) (string, error) {
	url, err := url.Parse(proxyURL)
	if err != nil {
		return "", err
	}
	hostName := url.Hostname()
	hostName = strings.TrimPrefix(hostName, proxyPrefix)
	return hostName, nil
}

func (r *Reconciler) sendToolchainStatusNotification(ctx context.Context,
	toolchainStatus *toolchainv1alpha1.ToolchainStatus, status toolchainStatusNotificationType) error {
	logger := log.FromContext(ctx)
	config, err := toolchainconfig.GetToolchainConfig(r.Client)
	if err != nil {
		return errs.Wrapf(err, "unable to get ToolchainConfig")
	}

	tsValue := time.Now().Format("20060102150405")
	contentString := ""
	subjectString := ""
	domain := ""
	if domain, err = removeSchemeFromURL(toolchainStatus.Status.HostRoutes.ProxyURL); err != nil {
		logger.Error(err, fmt.Sprintf("Error while parsing proxyUrl %v", toolchainStatus.Status.HostRoutes.ProxyURL))
	}
	switch status {
	case unreadyStatus:
		toolchainStatus = toolchainStatus.DeepCopy()
		toolchainStatus.ManagedFields = nil // we don't need these managed fields in the notification
		clusterURLs := ClusterURLs(logger, toolchainStatus)
		contentString, err = GenerateUnreadyNotificationContent(clusterURLs, ExtractStatusMetadata(toolchainStatus))
		if err != nil {
			return err
		}
		subjectString = fmt.Sprintf(adminUnreadyNotificationSubject, domain)
	case restoredStatus:
		contentString = "<div><pre>ToolchainStatus is back to ready status.</pre></div>"
		subjectString = fmt.Sprintf(adminRestoredNotificationSubject, domain)
	default:
		return fmt.Errorf("invalid ToolchainStatusNotification status type - %s", status)
	}

	notification, err := notify.NewNotificationBuilder(r.Client, toolchainStatus.Namespace).
		WithName(fmt.Sprintf("toolchainstatus-%s-%s", string(status), tsValue)).
		WithControllerReference(toolchainStatus, r.Scheme).
		WithSubjectAndContent(subjectString, contentString).
		Create(ctx, config.Notifications().AdminEmail())

	if err != nil {
		logger.Error(err, fmt.Sprintf("Failed to create toolchain status %s notification resource", status))
		return err
	}

	logger.Info(fmt.Sprintf("Toolchain status[%s] notification resource created", notification.Name))
	return nil
}

func ClusterURLs(logger logr.Logger, instance *toolchainv1alpha1.ToolchainStatus) map[string]string {

	if instance.Status.HostRoutes.ProxyURL != "" {
		var domain string
		var err error
		if domain, err = removeSchemeFromURL(instance.Status.HostRoutes.ProxyURL); err != nil {
			logger.Error(err, fmt.Sprintf("Error while parsing proxyUrl %v", instance.Status.HostRoutes.ProxyURL))
		} else {
			return map[string]string{
				"Cluster URL": domain,
			}
		}
	}

	result := map[string]string{}

	for _, mbr := range instance.Status.Members {
		result["Member cluster"] = mbr.ClusterName
	}

	return result
}

type ComponentNotReadyStatus struct {
	ComponentType string
	ComponentName string
	Reason        string
	Message       string
	Details       map[string]string
}

func GenerateUnreadyNotificationContent(clusterURLs map[string]string, statusMeta []*ComponentNotReadyStatus) (string, error) {
	tmpl, err := template.New("status").Parse(statusNotificationTemplate)
	if err != nil {
		return "", err
	}

	var output bytes.Buffer

	templateContext := map[string]interface{}{
		"clusterURLs": clusterURLs,
		"components":  statusMeta,
	}

	err = tmpl.Execute(&output, templateContext)
	if err != nil {
		return "", err
	}

	return output.String(), nil
}

func ExtractStatusMetadata(instance *toolchainv1alpha1.ToolchainStatus) []*ComponentNotReadyStatus {
	result := []*ComponentNotReadyStatus{}

	cond, found := condition.FindConditionByType(instance.Status.Conditions, toolchainv1alpha1.ConditionReady)
	if found && cond.Status != corev1.ConditionTrue {
		result = append(result, &ComponentNotReadyStatus{
			ComponentType: "",
			ComponentName: "ToolchainStatus",
			Reason:        cond.Reason,
			Message:       cond.Message,
		})
	}

	if instance.Status.HostOperator != nil {
		cond, found = condition.FindConditionByType(instance.Status.HostOperator.Conditions, toolchainv1alpha1.ConditionReady)
		if found && cond.Status != corev1.ConditionTrue {
			result = append(result, &ComponentNotReadyStatus{
				ComponentName: "Host Operator",
				ComponentType: "Deployment",
				Reason:        cond.Reason,
				Message:       cond.Message,
			})
		}

		cond, found = condition.FindConditionByType(instance.Status.HostOperator.RevisionCheck.Conditions, toolchainv1alpha1.ConditionReady)
		if found && cond.Status != corev1.ConditionTrue {
			result = append(result, &ComponentNotReadyStatus{
				ComponentName: "Host Operator",
				ComponentType: "Revision",
				Reason:        cond.Reason,
				Message:       cond.Message,
			})
		}
	}

	if instance.Status.Members != nil {
		for _, member := range instance.Status.Members {
			cond, found := condition.FindConditionByType(member.MemberStatus.Conditions, toolchainv1alpha1.ConditionReady)
			if found && cond.Status != corev1.ConditionTrue {
				result = append(result, &ComponentNotReadyStatus{
					ComponentType: "Member",
					ComponentName: member.ClusterName,
					Reason:        cond.Reason,
					Message:       cond.Message,
				})
			}

			if member.MemberStatus.Routes != nil {
				cond, found = condition.FindConditionByType(member.MemberStatus.Routes.Conditions, toolchainv1alpha1.ConditionReady)
				if found && cond.Status != corev1.ConditionTrue {
					result = append(result, &ComponentNotReadyStatus{
						ComponentType: "Member Routes",
						ComponentName: member.ClusterName,
						Reason:        cond.Reason,
						Message:       cond.Message,
						Details: map[string]string{
							"Che dashboard URL": member.MemberStatus.Routes.CheDashboardURL,
							"Console URL":       member.MemberStatus.Routes.ConsoleURL,
						},
					})
				}
			}

			if member.MemberStatus.MemberOperator != nil {
				cond, found = condition.FindConditionByType(member.MemberStatus.MemberOperator.RevisionCheck.Conditions, toolchainv1alpha1.ConditionReady)
				if found && cond.Status != corev1.ConditionTrue {
					result = append(result, &ComponentNotReadyStatus{
						ComponentType: "Member Operator Revision",
						ComponentName: member.ClusterName,
						Reason:        cond.Reason,
						Message:       cond.Message,
					})
				}
			}
		}
	}

	if instance.Status.RegistrationService != nil {
		cond, found = condition.FindConditionByType(instance.Status.RegistrationService.Deployment.Conditions, toolchainv1alpha1.ConditionReady)
		if found && cond.Status != corev1.ConditionTrue {
			result = append(result, &ComponentNotReadyStatus{
				ComponentName: "Registration Service",
				ComponentType: "Deployment",
				Reason:        cond.Reason,
				Message:       cond.Message,
			})
		}

		cond, found = condition.FindConditionByType(instance.Status.RegistrationService.Health.Conditions, toolchainv1alpha1.ConditionReady)
		if found && cond.Status != corev1.ConditionTrue {
			result = append(result, &ComponentNotReadyStatus{
				ComponentName: "Registration Service",
				ComponentType: "Health",
				Reason:        cond.Reason,
				Message:       cond.Message,
			})
		}

		cond, found = condition.FindConditionByType(instance.Status.RegistrationService.RevisionCheck.Conditions, toolchainv1alpha1.ConditionReady)
		if found && cond.Status != corev1.ConditionTrue {
			result = append(result, &ComponentNotReadyStatus{
				ComponentName: "Registration Service",
				ComponentType: "Revision",
				Reason:        cond.Reason,
				Message:       cond.Message,
			})
		}
	}

	// Safety check - confirm the status metadata has all nil details values initialized to an empty map
	for _, meta := range result {
		if meta.Details == nil {
			meta.Details = map[string]string{}
		}
	}

	return result
}

func compareAndAssignMemberStatuses(ctx context.Context, toolchainStatus *toolchainv1alpha1.ToolchainStatus, members map[string]toolchainv1alpha1.MemberStatusStatus, memberClusters []*cluster.CachedToolchainCluster) bool {
	logger := log.FromContext(ctx)
	allOk := true
	for index, member := range toolchainStatus.Status.Members {
		newMemberStatus, ok := members[member.ClusterName]
		apiEndpoint := getAPIEndpoint(member.ClusterName, memberClusters)
		if apiEndpoint != "" {
			toolchainStatus.Status.Members[index].APIEndpoint = apiEndpoint
		}

		if ok {
			toolchainStatus.Status.Members[index].MemberStatus = newMemberStatus
			delete(members, member.ClusterName)
		} else if member.SpaceCount > 0 {
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
		apiEndpoint := getAPIEndpoint(clusterName, memberClusters)
		toolchainStatus.Status.Members = append(toolchainStatus.Status.Members, toolchainv1alpha1.Member{
			APIEndpoint:  apiEndpoint,
			ClusterName:  clusterName,
			MemberStatus: memberStatus,
			SpaceCount:   0,
		})
		logger.Info("added member status", "cluster_name", clusterName)
	}
	return allOk
}

// updateStatusConditions updates ToolchainStatus status conditions with new conditions
// the controller should always update at least the last updated timestamp of the status so the status should be updated
// regardless of whether any specific fields were updated. This way a problem with the controller can be indicated if
// the last updated timestamp was not updated.
func (r *Reconciler) updateStatusConditions(ctx context.Context, status *toolchainv1alpha1.ToolchainStatus,
	newConditions ...toolchainv1alpha1.Condition) error {
	logger := log.FromContext(ctx)
	status.Status.Conditions = condition.AddOrUpdateStatusConditionsWithLastUpdatedTimestamp(status.Status.Conditions, newConditions...)
	logger.Info("updating ToolchainStatus status conditions", "resource_version", status.ResourceVersion)
	err := r.Client.Status().Update(ctx, status)
	logger.Info("updated ToolchainStatus status conditions", "resource_version", status.ResourceVersion)
	return err
}

// wrapErrorWithStatusUpdate wraps the error and update the UserSignup status. If the update fails then the error is logged.
func (r *Reconciler) wrapErrorWithStatusUpdate(ctx context.Context, toolchainStatus *toolchainv1alpha1.ToolchainStatus,
	statusUpdater func(ctx context.Context, userAcc *toolchainv1alpha1.ToolchainStatus, message string) error, err error, format string,
	args ...interface{}) error {
	if err == nil {
		return nil
	}
	if err := statusUpdater(ctx, toolchainStatus, err.Error()); err != nil {
		logger := log.FromContext(ctx)
		logger.Error(err, "Error updating ToolchainStatus status")
	}
	return errs.Wrapf(err, format, args...)
}

func (r *Reconciler) setStatusReady(ctx context.Context, toolchainStatus *toolchainv1alpha1.ToolchainStatus) error {
	err := r.restoredCheck(ctx, toolchainStatus)
	if err != nil {
		return err
	}
	return r.updateStatusConditions(
		ctx,
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

func (r *Reconciler) setStatusNotReady(ctx context.Context, toolchainStatus *toolchainv1alpha1.ToolchainStatus, message string) error {
	return r.updateStatusConditions(
		ctx,
		toolchainStatus,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.ConditionReady,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.ToolchainStatusComponentsNotReadyReason,
			Message: message,
		})
}

func (r *Reconciler) setStatusToolchainStatusUnreadyNotificationCreated(
	ctx context.Context,
	toolchainStatus *toolchainv1alpha1.ToolchainStatus) error {
	return r.updateStatusConditions(
		ctx,
		toolchainStatus,
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.ToolchainStatusUnreadyNotificationCreated,
			Status: corev1.ConditionTrue,
			Reason: toolchainv1alpha1.ToolchainStatusUnreadyNotificationCRCreatedReason,
		})
}

func (r *Reconciler) setStatusUnreadyNotificationCreationFailed(ctx context.Context,
	toolchainStatus *toolchainv1alpha1.ToolchainStatus, message string) error {

	return r.updateStatusConditions(
		ctx,
		toolchainStatus,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.ConditionReady,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.ToolchainStatusUnreadyNotificationCRCreationFailedReason,
			Message: message,
		})
}

func (r *Reconciler) setStatusReadyNotificationCreationFailed(ctx context.Context,
	toolchainStatus *toolchainv1alpha1.ToolchainStatus, message string) error {

	return r.updateStatusConditions(
		ctx,
		toolchainStatus,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.ConditionReady,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.ToolchainStatusRestoredNotificationCRCreationFailedReason,
			Message: message,
		})
}

func customMemberStatus(conditions ...toolchainv1alpha1.Condition) toolchainv1alpha1.MemberStatusStatus {
	return toolchainv1alpha1.MemberStatusStatus{
		Conditions: conditions,
	}
}

type regServiceSubstatusHandler struct {
	httpClientImpl      HTTPClient
	controllerClient    runtimeclient.Client
	versionCheckManager status.VersionCheckManager
}

// addRegistrationServiceDeploymentStatus handles the RegistrationService.Deployment part of the toolchainstatus
func (s *regServiceSubstatusHandler) addRegistrationServiceDeploymentStatus(ctx context.Context, toolchainStatus *toolchainv1alpha1.ToolchainStatus) bool {
	deploymentConditions := status.GetDeploymentStatusConditions(ctx, s.controllerClient, registrationservice.ResourceName, toolchainStatus.Namespace)
	toolchainStatus.Status.RegistrationService.Deployment.Name = registrationservice.ResourceName
	toolchainStatus.Status.RegistrationService.Deployment.Conditions = deploymentConditions

	err := status.ValidateComponentConditionReady(deploymentConditions...)
	if err != nil {
		logger := log.FromContext(ctx)
		logger.Error(err, "a problem was detected in the deployment status")
		return false
	}

	return true
}

// addRegistrationServiceHealthAndRevisionCheckStatus handles the RegistrationService.Health part of the toolchainstatus
func (s *regServiceSubstatusHandler) addRegistrationServiceHealthAndRevisionCheckStatus(ctx context.Context, toolchainStatus *toolchainv1alpha1.ToolchainStatus) bool {
	logger := log.FromContext(ctx)
	// get the JSON payload from the health endpoint
	resp, err := s.httpClientImpl.Get(registrationServiceHealthURL)
	if err != nil {
		logger.Error(err, errMsgRegistrationServiceNotReady)
		errCondition := status.NewComponentErrorCondition(toolchainv1alpha1.ToolchainStatusRegServiceNotReadyReason, err.Error())
		toolchainStatus.Status.RegistrationService.Health.Conditions = []toolchainv1alpha1.Condition{*errCondition}
		return false
	}

	// bad response
	if resp.StatusCode != http.StatusOK {
		err = fmt.Errorf("bad response from %s : statusCode=%d", registrationServiceHealthURL, resp.StatusCode)
		logger.Error(err, errMsgRegistrationServiceNotReady)
		errCondition := status.NewComponentErrorCondition(toolchainv1alpha1.ToolchainStatusRegServiceNotReadyReason, err.Error())
		toolchainStatus.Status.RegistrationService.Health.Conditions = []toolchainv1alpha1.Condition{*errCondition}
		return false
	}

	// decode the response to JSON
	defer func() {
		if _, err := io.ReadAll(resp.Body); err != nil {
			logger.Error(err, "unable to read the response")
		}
		if err := resp.Body.Close(); err != nil {
			logger.Error(err, "unable to close the body")
		}
	}()
	healthValues := status.Health{}
	err = json.NewDecoder(resp.Body).Decode(&healthValues)
	if err != nil {
		logger.Error(err, errMsgRegistrationServiceNotReady)
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
		logger.Error(err, "registration service is unhealthy")
		errCondition := status.NewComponentErrorCondition(toolchainv1alpha1.ToolchainStatusRegServiceNotReadyReason, err.Error())
		toolchainStatus.Status.RegistrationService.Health.Conditions = []toolchainv1alpha1.Condition{*errCondition}
		return false
	}

	// we can set component ready condition to true
	componentReadyCondition := status.NewComponentReadyCondition(toolchainv1alpha1.ToolchainStatusRegServiceReadyReason)
	toolchainStatus.Status.RegistrationService.Health.Conditions = []toolchainv1alpha1.Condition{*componentReadyCondition}

	// if we are running in production we also
	// check that deployed version matches source code repository commit
	toolchainConfig, errToolchainConfig := toolchainconfig.GetToolchainConfig(s.controllerClient)
	if errToolchainConfig != nil {
		logger.Error(errToolchainConfig, status.ErrMsgCannotGetDeployment)
		errCondition := status.NewComponentErrorCondition(toolchainv1alpha1.ToolchainStatusDeploymentRevisionCheckOperatorErrorReason,
			fmt.Sprintf("unable to get ToolchainConfig: %s", errToolchainConfig.Error()))
		toolchainStatus.Status.RegistrationService.RevisionCheck.Conditions = []toolchainv1alpha1.Condition{*errCondition}
		return false
	}

	// validate deployed version
	isProd := isProdEnvironment(toolchainConfig)
	githubRepo := client.GitHubRepository{
		Org:               toolchainv1alpha1.ProviderLabelValue,
		Name:              registrationServiceRepoName,
		Branch:            registrationServiceRepoBranchName,
		DeployedCommitSHA: healthValues.Revision,
	}
	versionCondition := s.versionCheckManager.CheckDeployedVersionIsUpToDate(ctx, isProd, toolchainConfig.GitHubSecret().AccessTokenKey(), toolchainStatus.Status.RegistrationService.RevisionCheck.Conditions, githubRepo)
	err = status.ValidateComponentConditionReady(*versionCondition)
	if err != nil {
		// add version is not up-to-date condition
		logger.Error(err, "registration service deployment is not up to date")
		toolchainStatus.Status.RegistrationService.RevisionCheck.Conditions = []toolchainv1alpha1.Condition{*versionCondition}
		return false
	}
	// add version is up-to-date condition
	toolchainStatus.Status.RegistrationService.RevisionCheck.Conditions = []toolchainv1alpha1.Condition{*versionCondition}
	// if we get here it means that component health is ok
	return true
}
