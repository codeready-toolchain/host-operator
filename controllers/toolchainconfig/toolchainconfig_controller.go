package toolchainconfig

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	applycl "github.com/codeready-toolchain/toolchain-common/pkg/client"
	"github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	"github.com/codeready-toolchain/toolchain-common/pkg/condition"
	"github.com/codeready-toolchain/toolchain-common/pkg/template"

	v1 "github.com/openshift/api/template/v1"

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
	Client             client.Client
	GetMembersFunc     cluster.GetMemberClustersFunc
	Scheme             *runtime.Scheme
	regServiceTemplate *v1.Template
}

//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=toolchainconfigs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=toolchainconfigs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=toolchainconfigs/finalizers,verbs=update

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

	// load the latest config and secrets into the cache
	cfg, err := ForceLoadToolchainConfig(r.Client)
	if err != nil {
		reqLogger.Error(err, "failed to load the latest configuration")
		return DefaultReconcile, err
	}

	// Deploy registration service
	if err := r.ensureRegistrationService(getVars(request.Namespace, cfg), toolchainConfig); err != nil {
		return DefaultReconcile, err
	}

	// Sync member configs to member clusters
	sync := Synchronizer{
		logger:         reqLogger,
		getMembersFunc: r.GetMembersFunc,
	}

	if syncErrs := sync.SyncMemberConfigs(toolchainConfig); len(syncErrs) > 0 {
		for cluster, errMsg := range syncErrs {
			err := fmt.Errorf(errMsg)
			reqLogger.Error(err, "error syncing to member cluster", "cluster", cluster)
		}
		return DefaultReconcile, r.updateSyncStatus(reqLogger, toolchainConfig, syncErrs, ToSyncFailure())
	}
	return DefaultReconcile, r.updateSyncStatus(reqLogger, toolchainConfig, map[string]string{}, ToBeSyncComplete())
}

func (r *Reconciler) ensureRegistrationService(reqLogger logr.Logger, vars templateVars, owner *toolchainv1alpha1.ToolchainConfig) error {
	// process template with variables taken from the RegistrationService CRD
	cl := applycl.NewApplyClient(r.Client, r.Scheme)
	toolchainObjects, err := template.NewProcessor(r.Scheme).Process(r.regServiceTemplate.DeepCopy(), vars)
	if err != nil {
		return err
	}

	// create all objects that are within the template, and update only when the object has changed.
	var updated []string
	for _, toolchainObject := range toolchainObjects {
		createdOrUpdated, err := cl.ApplyObject(toolchainObject.GetClientObject(), applycl.SetOwner(owner))
		if err != nil {
			return err
		}
		if createdOrUpdated {
			updated = append(updated, fmt.Sprintf("%s: %s", toolchainObject.GetGvk().Kind, toolchainObject.GetName()))
		}
	}
	if len(updated) > 0 {
		return reconcile.Result{
			Requeue:      true,
			RequeueAfter: time.Second, // le't put one second there in case it wasn't re-queued by some resource change
		}, updateStatusConditions(r.Client, regService, toBeNotReady(toolchainv1alpha1.RegistrationServiceDeployingReason, fmt.Sprintf("updated resources: %s", updated)))
	}

	reqLogger.Info("All objects in registration service template has been created and are up-to-date")
	return updateStatusConditions(r.Client, regService, toBeDeployed())
}

type templateVars map[string]string

func getVars(namespace string, cfg ToolchainConfig) templateVars {
	var vars templateVars = map[string]string{}
	vars.addIfNotEmpty("NAMESPACE", namespace)
	vars.addIfNotEmpty()
	return vars
}

func (v *templateVars) addIfNotEmpty(key, value string) {
	if value != "" {
		(*v)[key] = value
	}
}

func (r *Reconciler) updateSyncStatus(reqLogger logr.Logger, toolchainConfig *toolchainv1alpha1.ToolchainConfig, syncErrs map[string]string, newCondition toolchainv1alpha1.Condition) error {
	toolchainConfig.Status.SyncErrors = syncErrs
	return r.updateDeployStatus(reqLogger, toolchainConfig, newCondition)
}

func (r *Reconciler) updateDeployStatus(reqLogger logr.Logger, toolchainConfig *toolchainv1alpha1.ToolchainConfig, newCondition toolchainv1alpha1.Condition) error {
	toolchainConfig.Status.Conditions = condition.AddOrUpdateStatusConditionsWithLastUpdatedTimestamp(toolchainConfig.Status.Conditions, newCondition)
	err := r.Client.Status().Update(context.TODO(), toolchainConfig)
	if err != nil {
		reqLogger.Error(err, "failed to update status for toolchainconfig")
	}
	return err
}

// ToBeSyncComplete condition when the sync completed with success
func ToBeSyncComplete() toolchainv1alpha1.Condition {
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

// ToRegServiceDeployFailure condition when an error occurred
func ToRegServiceDeployComplete() toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:   toolchainv1alpha1.ToolchainConfigRegServiceDeploy,
		Status: corev1.ConditionFalse,
		Reason: toolchainv1alpha1.ToolchainConfigRegServiceDeployedReason,
	}
}

// ToRegServiceDeployFailure condition when an error occurred
func ToRegServiceDeployFailure(err error) toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:    toolchainv1alpha1.ToolchainConfigRegServiceDeploy,
		Status:  corev1.ConditionFalse,
		Reason:  toolchainv1alpha1.ToolchainConfigRegServiceDeployFailedReason,
		Message: err.Error(),
	}
}
