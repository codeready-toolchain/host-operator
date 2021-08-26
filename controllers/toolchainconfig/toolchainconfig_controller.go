package toolchainconfig

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/go-logr/logr"
	errs "github.com/pkg/errors"

	"github.com/codeready-toolchain/host-operator/pkg/templates/registrationservice"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	applycl "github.com/codeready-toolchain/toolchain-common/pkg/client"
	"github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	"github.com/codeready-toolchain/toolchain-common/pkg/condition"
	"github.com/codeready-toolchain/toolchain-common/pkg/template"

	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/log"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const configResourceName = "config"

// DefaultReconcile requeue every 10 seconds by default to ensure the MemberOperatorConfig on each member remains synchronized with the ToolchainConfig
var DefaultReconcile = reconcile.Result{RequeueAfter: 10 * time.Second}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr manager.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&toolchainv1alpha1.ToolchainConfig{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Watches(&source.Kind{Type: &corev1.Secret{}},
			handler.EnqueueRequestsFromMapFunc(MapSecretToToolchainConfig()),
			builder.WithPredicates(&predicate.GenerationChangedPredicate{})).
		Complete(r)
}

// Reconciler reconciles a ToolchainConfig object
type Reconciler struct {
	Client         client.Client
	GetMembersFunc cluster.GetMemberClustersFunc
	Scheme         *runtime.Scheme
}

//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=toolchainconfigs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=toolchainconfigs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=toolchainconfigs/finalizers,verbs=update

//+kubebuilder:rbac:groups="",resources=configmaps;services;serviceaccounts,verbs=get;list;watch;update;patch;create;delete
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io;authorization.openshift.io,resources=rolebindings;roles,verbs=*
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;update;patch;create;delete
//+kubebuilder:rbac:groups=route.openshift.io,resources=routes,verbs=get;list;watch;update;patch;create;delete

// Reconcile reads that state of the cluster for a ToolchainConfig object and makes changes based on the state read
// and what is in the ToolchainConfig.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *Reconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	reqLogger := log.FromContext(ctx)
	reqLogger.Info("Reconciling ToolchainConfig")

	// Fetch the ToolchainConfig instance
	toolchainConfig := &toolchainv1alpha1.ToolchainConfig{}
	err := r.Client.Get(context.TODO(), request.NamespacedName, toolchainConfig)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			reqLogger.Error(err, "it looks like the toolchainconfig resource was removed - the cache will use the latest version of the resource")
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		reqLogger.Error(err, "error getting the toolchainconfig resource")
		return DefaultReconcile, err
	}

	// Load the latest config and secrets into the cache
	cfg, err := ForceLoadToolchainConfig(r.Client)
	if err != nil {
		return reconcile.Result{}, r.wrapErrorWithStatusUpdate(reqLogger, toolchainConfig, r.setStatusDeployRegistrationServiceFailed, err, "failed to load the latest configuration")
	}

	// Deploy registration service
	if err := r.ensureRegistrationService(reqLogger, toolchainConfig, getVars(request.Namespace, cfg)); err != nil {
		// immediately reconcile again if there was an error
		return reconcile.Result{}, err
	}

	// Sync member configs to member clusters
	sync := Synchronizer{
		logger:         reqLogger,
		getMembersFunc: r.GetMembersFunc,
	}

	if syncErrs := sync.SyncMemberConfigs(toolchainConfig); len(syncErrs) > 0 {
		for cluster, errMsg := range syncErrs {
			err := fmt.Errorf(errMsg)
			reqLogger.Error(err, "error syncing configuration to member cluster", "cluster", cluster)
		}
		return DefaultReconcile, r.updateSyncStatus(toolchainConfig, syncErrs, ToSyncFailure())
	}
	return DefaultReconcile, r.updateSyncStatus(toolchainConfig, map[string]string{}, ToSyncComplete())
}

func (r *Reconciler) ensureRegistrationService(reqLogger logr.Logger, toolchainConfig *toolchainv1alpha1.ToolchainConfig, vars templateVars) error {
	regServiceTemplate, err := registrationservice.GetDeploymentTemplate()
	if err != nil {
		return errs.Wrap(err, "unable to decode the registration service deployment")
	}

	// process template with variables taken from the RegistrationService CRD
	cl := applycl.NewApplyClient(r.Client, r.Scheme)
	toolchainObjects, err := template.NewProcessor(r.Scheme).Process(regServiceTemplate.DeepCopy(), vars)
	if err != nil {
		return r.wrapErrorWithStatusUpdate(reqLogger, toolchainConfig, r.setStatusDeployRegistrationServiceFailed, err, "failed to process registration service template")
	}

	// create all objects that are within the template, and update only when the object has changed.
	var updated []string
	for _, toolchainObject := range toolchainObjects {
		createdOrUpdated, err := cl.ApplyObject(toolchainObject.GetClientObject(), applycl.SetOwner(toolchainConfig))
		if err != nil {
			return r.wrapErrorWithStatusUpdate(reqLogger, toolchainConfig, r.setStatusDeployRegistrationServiceFailed, err, "failed to apply registration service object %s", toolchainObject.GetName())
		}
		if createdOrUpdated {
			updated = append(updated, fmt.Sprintf("%s: %s", toolchainObject.GetGvk().Kind, toolchainObject.GetName()))
		}
	}
	if len(updated) > 0 {
		reqLogger.Info("Updated registration service resources", "updated resources", updated)
		return r.updateStatusCondition(toolchainConfig, ToRegServiceDeploying(fmt.Sprintf("updated resources: %s", updated)), false)
	}

	reqLogger.Info("All objects in registration service template have been created and are up-to-date")
	return r.updateStatusCondition(toolchainConfig, ToRegServiceDeployComplete(), false)
}

type templateVars map[string]string

func getVars(namespace string, cfg ToolchainConfig) templateVars {
	var vars templateVars = map[string]string{}
	image := os.Getenv(RegistrationServiceImageEnvKey)
	vars["IMAGE"] = image
	vars.addIfNotEmpty("NAMESPACE", namespace)
	vars.addIfNotEmpty("REPLICAS", fmt.Sprint(cfg.RegistrationService().Replicas()))
	return vars
}

func (v *templateVars) addIfNotEmpty(key, value string) {
	if value != "" {
		(*v)[key] = value
	}
}

func (r *Reconciler) setStatusDeployRegistrationServiceFailed(toolchainConfig *toolchainv1alpha1.ToolchainConfig, message string) error {
	return r.updateStatusCondition(toolchainConfig, ToRegServiceDeployFailure(message), false)
}

func (r *Reconciler) updateSyncStatus(toolchainConfig *toolchainv1alpha1.ToolchainConfig, syncErrs map[string]string, newCondition toolchainv1alpha1.Condition) error {
	toolchainConfig.Status.SyncErrors = syncErrs
	return r.updateStatusCondition(toolchainConfig, newCondition, true)
}

func (r *Reconciler) updateStatusCondition(toolchainConfig *toolchainv1alpha1.ToolchainConfig, newCondition toolchainv1alpha1.Condition, updateSyncErrors bool) error {
	var updatedConditions bool
	toolchainConfig.Status.Conditions, updatedConditions = condition.AddOrUpdateStatusConditions(toolchainConfig.Status.Conditions, newCondition)
	if !updatedConditions && !updateSyncErrors {
		// Nothing changed
		return nil
	}
	return r.Client.Status().Update(context.TODO(), toolchainConfig)
}

func (r *Reconciler) wrapErrorWithStatusUpdate(logger logr.Logger, toolchainConfig *toolchainv1alpha1.ToolchainConfig, statusUpdater func(toolchainConfig *toolchainv1alpha1.ToolchainConfig, message string) error, err error, format string, args ...interface{}) error {
	if err == nil {
		return nil
	}
	if err := statusUpdater(toolchainConfig, err.Error()); err != nil {
		logger.Error(err, "status update failed")
	}
	return errs.Wrapf(err, format, args...)
}

// ToSyncComplete condition when the sync completed with success
func ToSyncComplete() toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:   toolchainv1alpha1.ToolchainConfigSyncComplete,
		Status: corev1.ConditionTrue,
		Reason: toolchainv1alpha1.ToolchainConfigSyncedReason,
	}
}

// ToSyncFailure condition when a sync error occurred while syncing
func ToSyncFailure() toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:    toolchainv1alpha1.ToolchainConfigSyncComplete,
		Status:  corev1.ConditionFalse,
		Reason:  toolchainv1alpha1.ToolchainConfigSyncFailedReason,
		Message: "errors occurred while syncing MemberOperatorConfigs to the member clusters",
	}
}

// ToRegServiceDeployComplete condition when deploying is completed
func ToRegServiceDeployComplete() toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:   toolchainv1alpha1.ToolchainConfigRegServiceDeploy,
		Status: corev1.ConditionTrue,
		Reason: toolchainv1alpha1.ToolchainConfigRegServiceDeployedReason,
	}
}

// ToRegServiceDeploying condition when deploying is in progress
func ToRegServiceDeploying(msg string) toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:    toolchainv1alpha1.ToolchainConfigRegServiceDeploy,
		Status:  corev1.ConditionFalse,
		Reason:  toolchainv1alpha1.RegistrationServiceDeployingReason,
		Message: msg,
	}
}

// ToRegServiceDeployFailure condition when an error occurred
func ToRegServiceDeployFailure(msg string) toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:    toolchainv1alpha1.ToolchainConfigRegServiceDeploy,
		Status:  corev1.ConditionFalse,
		Reason:  toolchainv1alpha1.ToolchainConfigRegServiceDeployFailedReason,
		Message: msg,
	}
}
