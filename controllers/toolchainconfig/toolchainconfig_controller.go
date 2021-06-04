package toolchainconfig

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"

	"github.com/codeready-toolchain/toolchain-common/pkg/cluster"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"

	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const configResourceName = "config"

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("toolchainconfig-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource ToolchainConfig
	return c.Watch(&source.Kind{Type: &toolchainv1alpha1.ToolchainConfig{}}, &handler.EnqueueRequestForObject{}, &predicate.GenerationChangedPredicate{})
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr manager.Manager) error {
	return add(mgr, r)
}

// Reconciler reconciles a ToolchainConfig object
type Reconciler struct {
	Client         client.Client
	Log            logr.Logger
	GetMembersFunc cluster.GetMemberClustersFunc
}

// Reconcile reads that state of the cluster for a ToolchainConfig object and makes changes based on the state read
// and what is in the ToolchainConfig.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *Reconciler) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := r.Log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling ToolchainConfig")

	// Requeue every 10 seconds by default to ensure the MemberOperatorConfig on each member remains synchronized with the ToolchainConfig
	defaultReconcile := reconcile.Result{RequeueAfter: 10 * time.Second}

	// Fetch the ToolchainConfig instance
	toolchainConfig := &toolchainv1alpha1.ToolchainConfig{}
	err := r.Client.Get(context.TODO(), request.NamespacedName, toolchainConfig)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			reqLogger.Error(err, "it looks like the ToolchainConfig resource with the name 'config' was removed - the cache will use the latest version of the resource")
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}
	updateConfig(toolchainConfig)

	// Sync member configs to member clusters
	sync := synchronizer{
		logger:         r.Log,
		getMembersFunc: r.GetMembersFunc,
	}

	if syncErrs := sync.syncMemberConfigs(toolchainConfig.DeepCopy()); len(syncErrs) > 0 {
		// wrapErrorWithStatusUpdate
		return defaultReconcile, fmt.Errorf("member cluster configuration sync failed")
	}
	// 	if err := updateStatus(logger, toolchainConfig, msg); err != nil {
	// 		logger.Error(err, "status update failed")
	// 	}

	return defaultReconcile, nil
}

// type StatusUpdaterFunc func(logger logr.Logger, toolchainConfig *toolchainv1alpha1.ToolchainConfig, message string) error

// // wrapErrorWithStatusUpdate wraps the error and updates the ToolchainConfig status. If the update failed then logs the error.
// func (r *Reconciler) wrapErrorWithStatusUpdate(logger logr.Logger, toolchainConfig *toolchainv1alpha1.ToolchainConfig, updateStatus StatusUpdaterFunc, err error, format string, args ...interface{}) error {
// 	if err == nil {
// 		return nil
// 	}
// 	if err := updateStatus(logger, toolchainConfig, err.Error()); err != nil {
// 		logger.Error(err, "status update failed")
// 	}
// 	if format != "" {
// 		return errs.Wrapf(err, format, args...)
// 	}
// 	return err
// }

// func (r *Reconciler) setStatusFailedAndSyncErrors(reason string) StatusUpdaterFunc {
// 	return func(logger logr.Logger, toolchainConfig *toolchainv1alpha1.ToolchainConfig, message string) error {
// 		return updateStatusConditions(
// 			logger,
// 			r.Client,
// 			toolchainConfig,
// 			toBeNotReady(reason, message))
// 	}
// }

// // updateStatusConditions updates user account status conditions with the new conditions
// func updateStatusConditions(logger logr.Logger, cl client.Client, toolchainConfig *toolchainv1alpha1.ToolchainConfig, newConditions ...toolchainv1alpha1.Condition) error {
// 	var updated bool
// 	toolchainConfig.Status.Conditions, updated = condition.AddOrUpdateStatusConditions(toolchainConfig.Status.Conditions, newConditions...)
// 	if !updated {
// 		// Nothing changed
// 		logger.Info("ToolchainConfig status conditions unchanged")
// 		return nil
// 	}
// 	logger.Info("updating MUR status conditions", "generation", toolchainConfig.Generation, "resource_version", toolchainConfig.ResourceVersion)
// 	err := cl.Status().Update(context.TODO(), toolchainConfig)
// 	logger.Info("updated MUR status conditions", "generation", toolchainConfig.Generation, "resource_version", toolchainConfig.ResourceVersion)
// 	return err
// }
