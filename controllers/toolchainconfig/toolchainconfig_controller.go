package toolchainconfig

import (
	"context"
	"fmt"
	"os"
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/constants"
	"github.com/codeready-toolchain/host-operator/pkg/templates/registrationservice"
	applycl "github.com/codeready-toolchain/toolchain-common/pkg/client"
	"github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	"github.com/codeready-toolchain/toolchain-common/pkg/condition"
	"github.com/codeready-toolchain/toolchain-common/pkg/template"

	templatev1 "github.com/openshift/api/template/v1"
	errs "github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const configResourceName = "config"

// DefaultReconcile requeue every 10 seconds by default to ensure the MemberOperatorConfig on each member remains synchronized with the ToolchainConfig
var DefaultReconcile = reconcile.Result{RequeueAfter: 10 * time.Second}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr manager.Manager) error {
	regServiceTemplate, err := registrationservice.GetDeploymentTemplate()
	if err != nil {
		return errs.Wrap(err, "unable to decode the registration service deployment")
	}
	r.RegServiceTemplate = regServiceTemplate

	log.Log.Info("Setup ToolchainConfig")

	return ctrl.NewControllerManagedBy(mgr).
		For(&toolchainv1alpha1.ToolchainConfig{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Watches(&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(MapSecretToToolchainConfig())).
		Complete(r)
}

// Reconciler reconciles a ToolchainConfig object
type Reconciler struct {
	Client             runtimeclient.Client
	GetMembersFunc     cluster.GetMemberClustersFunc
	Scheme             *runtime.Scheme
	RegServiceTemplate *templatev1.Template
}

//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=toolchainconfigs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=toolchainconfigs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=toolchainconfigs/finalizers,verbs=update

//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=proxyplugins,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=proxyplugins/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=proxyplugins/finalizers,verbs=update

//+kubebuilder:rbac:groups="",resources=configmaps;services;serviceaccounts,verbs=get;list;watch;update;patch;create;delete
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io;authorization.openshift.io,resources=rolebindings;roles,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;update;patch;create;delete
//+kubebuilder:rbac:groups=route.openshift.io,resources=routes,verbs=get;list;watch;update;patch;create;delete

// Reconcile reads that state of the cluster for a ToolchainConfig object and makes changes based on the state read
// and what is in the ToolchainConfig.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.RequeueAfter > 0 is true, otherwise upon completion it will remove the work from the queue.
func (r *Reconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	reqLogger := log.FromContext(ctx)
	reqLogger.Info("Reconciling ToolchainConfig")

	// Fetch the ToolchainConfig instance
	toolchainConfig := &toolchainv1alpha1.ToolchainConfig{}
	err := r.Client.Get(ctx, request.NamespacedName, toolchainConfig)
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
		return reconcile.Result{}, r.WrapErrorWithStatusUpdate(ctx, toolchainConfig, r.setStatusDeployRegistrationServiceFailed, fmt.Errorf("failed to load the latest configuration: %w", err))
	}

	// Deploy registration service
	if err := r.ensureRegistrationService(ctx, toolchainConfig, getVars(request.Namespace, cfg)); err != nil {
		// immediately reconcile again if there was an error
		return reconcile.Result{}, err
	}

	// Sync member configs to member clusters
	sync := Synchronizer{
		logger:         reqLogger,
		getMembersFunc: r.GetMembersFunc,
	}

	if syncErrs := sync.SyncMemberConfigs(ctx, toolchainConfig); len(syncErrs) > 0 {
		for cluster, errMsg := range syncErrs {
			err := fmt.Errorf("%s", errMsg)
			reqLogger.Error(err, "error syncing configuration to member cluster", "cluster", cluster)
		}
		return DefaultReconcile, r.updateSyncStatus(ctx, toolchainConfig, syncErrs, ToSyncFailure())
	}
	return DefaultReconcile, r.updateSyncStatus(ctx, toolchainConfig, map[string]string{}, ToSyncComplete())
}

func (r *Reconciler) ensureRegistrationService(ctx context.Context, toolchainConfig *toolchainv1alpha1.ToolchainConfig, vars templateVars) error {
	// process template with variables taken from the RegistrationService CRD
	cl := applycl.NewSSAApplyClient(r.Client, constants.HostOperatorFieldManager)
	toolchainObjects, err := template.NewProcessor(r.Scheme).Process(r.RegServiceTemplate.DeepCopy(), vars)
	if err != nil {
		return r.WrapErrorWithStatusUpdate(ctx, toolchainConfig, r.setStatusDeployRegistrationServiceFailed, fmt.Errorf("failed to process registration service template: %w", err))
	}

	// create all objects that are within the template, and update only when the object has changed.
	if err := cl.Apply(ctx, toolchainObjects, applycl.SetOwnerReference(toolchainConfig)); err != nil {
		return r.WrapErrorWithStatusUpdate(ctx, toolchainConfig, r.setStatusDeployRegistrationServiceFailed, fmt.Errorf("failed to apply registration service: %w", err))
	}

	logger := log.FromContext(ctx)
	logger.Info("All objects in registration service template have been created and are up-to-date")
	return r.updateStatusCondition(ctx, toolchainConfig, ToRegServiceDeployComplete(), false)
}

type templateVars map[string]string

func getVars(namespace string, cfg ToolchainConfig) templateVars {
	var vars templateVars = map[string]string{}
	image := os.Getenv(RegistrationServiceImageEnvKey)
	vars["IMAGE"] = image
	vars.addIfNotEmpty("NAMESPACE", namespace)
	vars.addIfNotEmpty("REPLICAS", fmt.Sprint(cfg.RegistrationService().Replicas()))

	// Allow overriding the registration service's command via an environment
	// variable.
	command := os.Getenv(RegistrationServiceCommandEnvKey)
	if command != "" {
		vars["REGISTRATION_SERVICE_COMMAND"] = command
	} else {
		vars["REGISTRATION_SERVICE_COMMAND"] = `["registration-service"]`
	}

	return vars
}

func (v *templateVars) addIfNotEmpty(key, value string) {
	if value != "" {
		(*v)[key] = value
	}
}

func (r *Reconciler) setStatusDeployRegistrationServiceFailed(ctx context.Context, toolchainConfig *toolchainv1alpha1.ToolchainConfig, message string) error {
	return r.updateStatusCondition(ctx, toolchainConfig, ToRegServiceDeployFailure(message), false)
}

func (r *Reconciler) updateSyncStatus(ctx context.Context, toolchainConfig *toolchainv1alpha1.ToolchainConfig, syncErrs map[string]string, newCondition toolchainv1alpha1.Condition) error {
	toolchainConfig.Status.SyncErrors = syncErrs
	return r.updateStatusCondition(ctx, toolchainConfig, newCondition, true)
}

func (r *Reconciler) updateStatusCondition(ctx context.Context, toolchainConfig *toolchainv1alpha1.ToolchainConfig, newCondition toolchainv1alpha1.Condition, updateSyncErrors bool) error {
	var updatedConditions bool
	toolchainConfig.Status.Conditions, updatedConditions = condition.AddOrUpdateStatusConditions(toolchainConfig.Status.Conditions, newCondition)
	if !updatedConditions && !updateSyncErrors {
		// Nothing changed
		return nil
	}
	return r.Client.Status().Update(ctx, toolchainConfig)
}

func (r *Reconciler) WrapErrorWithStatusUpdate(
	ctx context.Context,
	toolchainConfig *toolchainv1alpha1.ToolchainConfig,
	statusUpdater func(ctx context.Context, toolchainConfig *toolchainv1alpha1.ToolchainConfig, message string) error,
	err error,
) error {
	if err == nil {
		return nil
	}
	if err := statusUpdater(ctx, toolchainConfig, err.Error()); err != nil {
		logger := log.FromContext(ctx)
		logger.Error(err, "status update failed")
	}
	return err
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

// ToRegServiceDeployFailure condition when an error occurred
func ToRegServiceDeployFailure(msg string) toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:    toolchainv1alpha1.ToolchainConfigRegServiceDeploy,
		Status:  corev1.ConditionFalse,
		Reason:  toolchainv1alpha1.ToolchainConfigRegServiceDeployFailedReason,
		Message: msg,
	}
}
