package spaceprovisionerconfig

import (
	"context"
	"fmt"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/controllers/toolchainconfig"
	"github.com/codeready-toolchain/toolchain-common/pkg/condition"
	"github.com/redhat-cop/operator-utils/pkg/util"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type GetUsageFunc func(ctx context.Context, cl runtimeclient.Client, clusterName string, toolchainStatusNs string) (*toolchainv1alpha1.ConsumedCapacity, error)

// Reconciler is the reconciler for the SpaceProvisionerConfig CRs.
type Reconciler struct {
	Client runtimeclient.Client
	// GetUsageFunc is a function that can be used in the tests to mock the fetching of the consumed usage. It defaults to collecting the actual usage
	// from the ToolchainStatus.
	GetUsageFunc GetUsageFunc
}

var _ reconcile.Reconciler = (*Reconciler)(nil)

func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&toolchainv1alpha1.SpaceProvisionerConfig{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Watches(
			&toolchainv1alpha1.ToolchainCluster{},
			handler.EnqueueRequestsFromMapFunc(MapToolchainClusterToSpaceProvisionerConfigs(r.Client)),
		).
		// we use the same information as the ToolchainStatus specific for the SPCs. Because memory consumption is
		// read directly out of the member clusters using remote connections, let's look for it only once
		// in ToolchainStatus and just read it out "locally" here without needing to reach out to the member clusters
		// again.
		Watches(
			&toolchainv1alpha1.ToolchainStatus{},
			handler.EnqueueRequestsFromMapFunc(MapToolchainStatusToSpaceProvisionerConfigs(r.Client)),
		).
		Complete(r)
}

//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=spaceprovisionerconfigs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=spaceprovisionerconfigs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=toolchainclusters,verbs=get;list;watch
//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=toolchainstatuses,verbs=get;list;watch

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

	reportedErr := r.refreshStatus(ctx, spaceProvisionerConfig)

	if err := r.Client.Status().Update(ctx, spaceProvisionerConfig); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, reportedErr
}

func (r *Reconciler) refreshStatus(ctx context.Context, spc *toolchainv1alpha1.SpaceProvisionerConfig) error {
	// clear out the consumed capacity - this will advertise to the user that we either failed before it made sense
	// to collect it (and therefore we don't know it) or it was not available (and therefore we again don't know it)
	spc.Status.ConsumedCapacity = nil

	if !spc.Spec.Enabled {
		updateReadyCondition(spc, corev1.ConditionFalse, toolchainv1alpha1.SpaceProvisionerConfigDisabledReason, "")
		return nil
	}

	clusterCondition, err := r.determineClusterReadyState(ctx, spc)
	if err != nil {
		updateReadyCondition(spc, clusterCondition, toolchainv1alpha1.SpaceProvisionerConfigToolchainClusterNotFoundReason, err.Error())

		// the reconciler reacts on ToolchainCluster changes so it will be triggered once a new TC appears
		// we therefore don't need to return error from the reconciler in the case the TC is not found.
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}

	var reason string
	resultCondition := clusterCondition

	if clusterCondition == corev1.ConditionTrue {
		collectUsage := r.GetUsageFunc
		if collectUsage == nil {
			collectUsage = collectConsumedCapacity
		}
		cc, err := collectUsage(ctx, r.Client, spc.Spec.ToolchainCluster, spc.Namespace)
		if err != nil {
			updateReadyCondition(spc, corev1.ConditionUnknown, toolchainv1alpha1.SpaceProvisionerConfigFailedToDetermineCapacityReason, err.Error())
			return err
		}
		spc.Status.ConsumedCapacity = cc

		capacityCondition := r.determineCapacityReadyState(spc)
		if capacityCondition != corev1.ConditionTrue {
			reason = toolchainv1alpha1.SpaceProvisionerConfigInsufficientCapacityReason
		} else {
			reason = toolchainv1alpha1.SpaceProvisionerConfigValidReason
		}

		resultCondition = capacityCondition
	} else {
		reason = toolchainv1alpha1.SpaceProvisionerConfigToolchainClusterNotReadyReason
	}

	readyCondition := toolchainv1alpha1.Condition{
		Type:   toolchainv1alpha1.ConditionReady,
		Status: resultCondition,
		Reason: reason,
	}

	spc.Status.Conditions, _ = condition.AddOrUpdateStatusConditions(spc.Status.Conditions, readyCondition)

	return nil
}

// Note that this function merely mirrors the usage information found in the ToolchainStatus. This means that it actually may work
// with slightly stale data because the counter.Counts cache might not have been synced yet. This is ok though because the capacity manager
// doesn't completely rely on the readiness status of the SPC and will re-evaluate the decision taking into the account the contents of
// the counter cache and therefore completely "fresh" data.
func collectConsumedCapacity(ctx context.Context, cl runtimeclient.Client, clusterName string, toolchainStatusNs string) (*toolchainv1alpha1.ConsumedCapacity, error) {
	status := &toolchainv1alpha1.ToolchainStatus{}
	if err := cl.Get(ctx, types.NamespacedName{Namespace: toolchainStatusNs, Name: toolchainconfig.ToolchainStatusName}, status); err != nil {
		return nil, fmt.Errorf("unable to read ToolchainStatus resource: %w", err)
	}

	for _, m := range status.Status.Members {
		if m.ClusterName == clusterName {
			cc := toolchainv1alpha1.ConsumedCapacity{}
			cc.MemoryUsagePercentPerNodeRole = m.MemberStatus.ResourceUsage.MemoryUsagePerNodeRole
			cc.SpaceCount = m.SpaceCount

			return &cc, nil
		}
	}

	return nil, nil
}

func (r *Reconciler) determineClusterReadyState(ctx context.Context, spc *toolchainv1alpha1.SpaceProvisionerConfig) (corev1.ConditionStatus, error) {
	toolchainCluster := &toolchainv1alpha1.ToolchainCluster{}
	toolchainClusterKey := runtimeclient.ObjectKey{Name: spc.Spec.ToolchainCluster, Namespace: spc.Namespace}
	var toolchainPresent corev1.ConditionStatus
	var err error
	if err = r.Client.Get(ctx, toolchainClusterKey, toolchainCluster); err != nil {
		if !errors.IsNotFound(err) {
			// IsNotFound is self-explanatory but let's add a little bit of context to the error
			// in other cases
			err = fmt.Errorf("failed to get the referenced ToolchainCluster: %w", err)
			toolchainPresent = corev1.ConditionUnknown
		} else {
			toolchainPresent = corev1.ConditionFalse
		}
	} else {
		readyCond, found := condition.FindConditionByType(toolchainCluster.Status.Conditions, toolchainv1alpha1.ConditionReady)
		if !found {
			toolchainPresent = corev1.ConditionFalse
		} else {
			toolchainPresent = readyCond.Status
		}
	}

	return toolchainPresent, err
}

func (r *Reconciler) determineCapacityReadyState(spc *toolchainv1alpha1.SpaceProvisionerConfig) corev1.ConditionStatus {
	if spc.Status.ConsumedCapacity == nil {
		return corev1.ConditionUnknown
	}

	var freeSpaces corev1.ConditionStatus
	max := spc.Spec.CapacityThresholds.MaxNumberOfSpaces
	if max == 0 || max > uint(spc.Status.ConsumedCapacity.SpaceCount) {
		freeSpaces = corev1.ConditionTrue
	} else {
		freeSpaces = corev1.ConditionFalse
	}

	enoughMemory := corev1.ConditionUnknown

	if spc.Spec.CapacityThresholds.MaxMemoryUtilizationPercent == 0 { // unlimited
		enoughMemory = corev1.ConditionTrue
	} else if len(spc.Status.ConsumedCapacity.MemoryUsagePercentPerNodeRole) > 0 { // let the state be unknown if we have no information
		enoughMemory = corev1.ConditionTrue
		for _, val := range spc.Status.ConsumedCapacity.MemoryUsagePercentPerNodeRole {
			if uint(val) >= spc.Spec.CapacityThresholds.MaxMemoryUtilizationPercent {
				enoughMemory = corev1.ConditionFalse
				break
			}
		}
	}

	return And(freeSpaces, enoughMemory)
}

func And(a, b corev1.ConditionStatus) corev1.ConditionStatus {
	switch a {
	case corev1.ConditionTrue:
		return b
	case corev1.ConditionFalse:
		return corev1.ConditionFalse
	case corev1.ConditionUnknown:
		if b == corev1.ConditionFalse {
			return b
		}
		return corev1.ConditionUnknown
	}

	// the above switch covers all supported states of condition status
	// but since it is a mere type alias of string, we need to "cover"
	// the rest of the cases (i.e. free-form strings), too.
	// Yay for stringly typed types...
	return corev1.ConditionUnknown
}

func updateReadyCondition(spc *toolchainv1alpha1.SpaceProvisionerConfig, status corev1.ConditionStatus, reason, message string) {
	readyCondition := toolchainv1alpha1.Condition{
		Type:    toolchainv1alpha1.ConditionReady,
		Status:  status,
		Reason:  reason,
		Message: message,
	}
	spc.Status.Conditions, _ = condition.AddOrUpdateStatusConditions(spc.Status.Conditions, readyCondition)
}
