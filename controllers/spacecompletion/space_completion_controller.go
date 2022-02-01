package spacecompletion

import (
	"context"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/controllers/toolchainconfig"
	"github.com/codeready-toolchain/host-operator/pkg/capacity"
	"github.com/codeready-toolchain/host-operator/pkg/pending"
	"github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	"github.com/go-logr/logr"
	"github.com/redhat-cop/operator-utils/pkg/util"

	errs "github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// Reconciler reconciles a Space object
type Reconciler struct {
	Client            client.Client
	Namespace         string
	GetMemberClusters cluster.GetMemberClustersFunc
}

// SetupWithManager sets up the controller reconciler with the Manager
// Watches the Space resources and the ToolchainStatus CRD
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		// watch Spaces in the host cluster
		For(&toolchainv1alpha1.Space{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Watches(
			&source.Kind{Type: &toolchainv1alpha1.ToolchainStatus{}},
			handler.EnqueueRequestsFromMapFunc(pending.NewSpaceMapper(mgr.GetClient()).MapToOldestPending)).
		Complete(r)
}

//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=spaces,verbs=get;list;watch;create;update;patch;delete

// Reconcile ensures that Space has set all missing fields that are needed for proper provisoning
func (r *Reconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx, "namespace", r.Namespace).WithName("completion")
	logger.Info("reconciling Space")

	// Fetch the Space
	space := &toolchainv1alpha1.Space{}
	err := r.Client.Get(context.TODO(), types.NamespacedName{
		Namespace: r.Namespace,
		Name:      request.Name,
	}, space)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Info("Space not found")
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, errs.Wrap(err, "unable to get the current Space")
	}

	// if is being deleted, then skip it as there is nothing te be completed
	if util.IsBeingDeleted(space) {
		logger.Info("Space is being deleted - skipping...")
		return reconcile.Result{}, nil
	}

	if changed, err := r.ensureFields(logger, space); err != nil || !changed {
		return reconcile.Result{}, err
	}

	return ctrl.Result{}, r.Client.Update(context.TODO(), space)
}

func (r *Reconciler) ensureFields(logger logr.Logger, space *toolchainv1alpha1.Space) (bool, error) {
	if space.Spec.TierName == "" {
		config, err := toolchainconfig.GetToolchainConfig(r.Client)
		if err != nil {
			return false, errs.Wrapf(err, "unable to set the TierName")
		}
		space.Spec.TierName = config.Tiers().DefaultSpaceTier()
		logger.Info("TierName has been set", "tierName", config.Tiers().DefaultSpaceTier())
		return true, nil
	}

	if space.Spec.TargetCluster == "" {
		targetCluster, err := capacity.GetOptimalTargetCluster("", space.Namespace, r.GetMemberClusters, r.Client)
		if err != nil {
			return false, errs.Wrapf(err, "unable to get the optimal target cluster")
		}
		if targetCluster == "" {
			logger.Info("no cluster available")
			return false, nil
		}
		space.Spec.TargetCluster = targetCluster
		logger.Info("TargetCluster has been set", "targetCluster", targetCluster)
		return true, nil
	}

	return false, nil
}
