package toolchainconfig

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"

	"github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	"github.com/codeready-toolchain/toolchain-common/pkg/condition"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/log"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const configResourceName = "config"

// DefaultReconcile requeue every 10 seconds by default to ensure the MemberOperatorConfig on each member remains synchronized with the ToolchainConfig
var DefaultReconcile = reconcile.Result{RequeueAfter: 10 * time.Second}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr manager.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&toolchainv1alpha1.ToolchainConfig{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Complete(r)
}

// Reconciler reconciles a ToolchainConfig object
type Reconciler struct {
	Client         client.Client
	GetMembersFunc cluster.GetMemberClustersFunc
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
	UpdateConfig(toolchainConfig)

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
		return DefaultReconcile, r.updateStatus(reqLogger, toolchainConfig, syncErrs, ToSyncFailure())
	}
	return DefaultReconcile, r.updateStatus(reqLogger, toolchainConfig, map[string]string{}, ToBeComplete())
}

func (r *Reconciler) updateStatus(reqLogger logr.Logger, toolchainConfig *toolchainv1alpha1.ToolchainConfig, syncErrs map[string]string, newCondition toolchainv1alpha1.Condition) error {
	toolchainConfig.Status.SyncErrors = syncErrs
	toolchainConfig.Status.Conditions = condition.AddOrUpdateStatusConditionsWithLastUpdatedTimestamp(toolchainConfig.Status.Conditions, newCondition)
	err := r.Client.Status().Update(context.TODO(), toolchainConfig)
	if err != nil {
		reqLogger.Error(err, "failed to update status for toolchainconfig")
	}
	return err
}

// ToBeComplete condition when the update completed with success
func ToBeComplete() toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:   toolchainv1alpha1.ToolchainConfigSyncComplete,
		Status: corev1.ConditionTrue,
		Reason: toolchainv1alpha1.ToolchainConfigSyncedReason,
	}
}

// ToFailure condition when an error occurred
func ToSyncFailure() toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:    toolchainv1alpha1.ToolchainConfigSyncComplete,
		Status:  corev1.ConditionFalse,
		Reason:  toolchainv1alpha1.ToolchainConfigSyncFailedReason,
		Message: "errors occurred while syncing MemberOperatorConfigs to the member clusters",
	}
}
