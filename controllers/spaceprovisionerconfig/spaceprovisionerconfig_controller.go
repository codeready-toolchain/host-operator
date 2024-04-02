package spaceprovisionerconfig

import (
	"context"
	"fmt"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/condition"
	"github.com/redhat-cop/operator-utils/pkg/util"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// Reconciler is the reconciler for the SpaceProvisionerConfig CRs.
type Reconciler struct {
	Client runtimeclient.Client
}

var _ reconcile.Reconciler = (*Reconciler)(nil)

func (r *Reconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&toolchainv1alpha1.SpaceProvisionerConfig{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Watches(
			// We want to trigger the reconciliation of SpaceProvisionerConfigs that reference some ToolChainCluster whenever
			// the ToolChainClusters are created or deleted. We don't have to care about updates, the mere existence of the
			// ToolchainCluster is enough for us.
			&toolchainv1alpha1.ToolchainCluster{},
			handler.EnqueueRequestsFromMapFunc(MapToolchainClusterToSpaceProvisionerConfigs(ctx, r.Client)),
			builder.WithPredicates(predicate.Funcs{
				CreateFunc: func(event.CreateEvent) bool {
					return true
				},
				DeleteFunc: func(event.DeleteEvent) bool {
					return true
				},
			},
			)).
		Complete(r)
}

//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=spaceprovisionerconfigs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=spaceprovisionerconfigs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=toolchainclusters,verbs=get;list;watch

// Reconcile ensures that SpaceProvisionerConfig is valid and points to an existing ToolchainCluster.
func (r *Reconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("reconciling SpaceProvisonerConfig")

	spaceProvisionerConfig := &toolchainv1alpha1.SpaceProvisionerConfig{}
	if err := r.Client.Get(ctx, request.NamespacedName, spaceProvisionerConfig); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("SpaceProvisionerConfig not found anymore")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed to get SpaceProvisionerConfig: %w", err)
	}

	if util.IsBeingDeleted(spaceProvisionerConfig) {
		logger.Info("SpaceProvisionerConfig is being deleted - skipping reconciliation")
		return ctrl.Result{}, nil
	}

	// check that there is a ToolchainCluster CR in the same namespace
	toolchainCluster := &toolchainv1alpha1.ToolchainCluster{}
	toolchainClusterKey := runtimeclient.ObjectKey{Name: spaceProvisionerConfig.Spec.ToolchainCluster, Namespace: spaceProvisionerConfig.Namespace}
	toolchainPresent := corev1.ConditionTrue
	toolchainPresenceReason := toolchainv1alpha1.SpaceProvisionerConfigValidReason
	var reportedError error
	toolchainPresenceMessage := ""
	if err := r.Client.Get(ctx, toolchainClusterKey, toolchainCluster); err != nil {
		if !errors.IsNotFound(err) {
			// we need to requeue the reconciliation in this case because we cannot be sure whether the ToolchainCluster
			// is really present in the cluster or not. If we did not do that and instead just reported the error in
			// the status, we could eventually leave the SPC in an incorrect state once the error condition in the cluster,
			// that prevents us from reading the ToolchainCluster, clears. I.e. we need the requeue to keep the promise
			// of eventual consistency.

			reportedError = fmt.Errorf("failed to get the referenced ToolchainCluster: %w", err)
			toolchainPresenceMessage = reportedError.Error()
		}
		toolchainPresenceReason = toolchainv1alpha1.SpaceProvisionerConfigToolchainClusterNotFoundReason
		toolchainPresent = corev1.ConditionFalse
	}

	spaceProvisionerConfig.Status.Conditions, _ = condition.AddOrUpdateStatusConditions(spaceProvisionerConfig.Status.Conditions,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.ConditionReady,
			Status:  toolchainPresent,
			Reason:  toolchainPresenceReason,
			Message: toolchainPresenceMessage,
		})

	logger.Info("updating SpaceProvisionerConfig", "status", toolchainPresent, "reason", toolchainPresenceReason)
	if err := r.Client.Status().Update(ctx, spaceProvisionerConfig); err != nil {
		if reportedError != nil {
			logger.Info("failed to update the status (reported as failed reconciliation) with a previous unreported error during reconciliation", "unreportedError", reportedError)
		}
		reportedError = fmt.Errorf("failed to update the SpaceProvisionerConfig status: %w", err)
	}

	return ctrl.Result{}, reportedError
}
