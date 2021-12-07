package space

import (
	"context"
	"fmt"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/controllers/usersignup"
	"github.com/codeready-toolchain/host-operator/pkg/cluster"
	"github.com/codeready-toolchain/toolchain-common/pkg/condition"
	"github.com/go-logr/logr"
	"github.com/redhat-cop/operator-utils/pkg/util"

	errs "github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	Client         client.Client
	Namespace      string
	MemberClusters map[string]cluster.Cluster
}

// SetupWithManager sets up the controller reconciler with the Manager and the given member clusters.
// Watches the Space resources in the current (host) cluster as its primary resources.
// Watches NSTemplateSets on the member clusters as its secondary resources.
func SetupWithManager(mgr ctrl.Manager, namespace string, memberClusters map[string]cluster.Cluster) error {
	b := ctrl.NewControllerManagedBy(mgr).
		// watch Spaces in the host cluster
		For(&toolchainv1alpha1.Space{}, builder.WithPredicates(predicate.GenerationChangedPredicate{}))
	// watch NSTemplateSets in all the member clusters
	for _, memberCluster := range memberClusters {
		b = b.Watches(source.NewKindWithCache(&toolchainv1alpha1.NSTemplateSet{}, memberCluster.Cache),
			&handler.EnqueueRequestForObject{},
		)
	}

	return b.Complete(&Reconciler{
		Client:         mgr.GetClient(),
		Namespace:      namespace,
		MemberClusters: memberClusters,
	})
}

//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=spaces,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=spaces/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=spaces/finalizers,verbs=update

// Reconcile ensures that there is an NSTemplateSet resource defined in the target member cluster
func (r *Reconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx, "namespace", r.Namespace)
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
	if !util.IsBeingDeleted(space) {
		// Add the finalizer if it is not present
		if err := r.addFinalizer(logger, space); err != nil {
			return reconcile.Result{}, err
		}
	} else {
		return reconcile.Result{}, r.ensureSpaceDeletion(logger, space)
	}
	// ensure that there's a NSTemplateSet on the Target Cluster
	// will trigger a requeue until the NSTemplateSet exists and is ready,
	// so the Space can be in `ready` status as well
	requeue, err := r.ensureNSTemplateSet(logger, space)
	return ctrl.Result{Requeue: requeue}, err
}

// setFinalizers sets the finalizers for Space
func (r *Reconciler) addFinalizer(logger logr.Logger, space *toolchainv1alpha1.Space) error {
	// Add the finalizer if it is not present
	if !util.HasFinalizer(space, toolchainv1alpha1.FinalizerName) {
		logger.Info("adding finalizer on Space")
		util.AddFinalizer(space, toolchainv1alpha1.FinalizerName)
		if err := r.Client.Update(context.TODO(), space); err != nil {
			return err
		}
	}
	return nil
}

// ensureNSTemplateSet creates the NSTemplateSet on the target member cluster if it does not exist,
// and updates the space's status accordingly.
// returns `true` after creating the NSTemplateSet and until it is `ready`
func (r *Reconciler) ensureNSTemplateSet(logger logr.Logger, space *toolchainv1alpha1.Space) (bool, error) {
	if space.Spec.TargetCluster == "" {
		return false, r.setStatusProvisioningFailed(logger, space, fmt.Errorf("unspecified target member cluster"))
	}
	memberCluster, found := r.MemberClusters[space.Spec.TargetCluster]
	if !found {
		return false, r.setStatusProvisioningFailed(logger, space, fmt.Errorf("unknown target member cluster '%s'", space.Spec.TargetCluster))
	}
	logger = logger.WithValues("target_member_cluster", space.Spec.TargetCluster)
	// create if not found
	nsTmplSet := &toolchainv1alpha1.NSTemplateSet{}
	if err := memberCluster.Client.Get(context.TODO(), types.NamespacedName{
		Namespace: memberCluster.OperatorNamespace,
		Name:      space.Name,
	}, nsTmplSet); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("creating NSTemplateSet on target member cluster")
			if err := r.setStatusProvisioning(space); err != nil {
				return false, err
			}
			if nsTmplSet, err = r.newNSTemplateSet(memberCluster.OperatorNamespace, space); err != nil {
				return false, r.setStatusProvisioningFailed(logger, space, err)
			}

			if err := memberCluster.Client.Create(context.TODO(), nsTmplSet); err != nil {
				logger.Error(err, "failed to create NSTemplateSet on target member cluster")
				return false, r.setStatusNSTemplateSetCreationFailed(logger, space, err)
			}
			logger.Info("NSTemplateSet created on target member cluster")
			return false, nil
		}
		return false, r.setStatusNSTemplateSetCreationFailed(logger, space, err)
	}
	logger.Info("NSTemplateSet already exists")

	readyCond, found := condition.FindConditionByType(nsTmplSet.Status.Conditions, toolchainv1alpha1.ConditionReady)
	if !found || readyCond.Status != corev1.ConditionTrue {
		logger.Info("NSTemplateSet is not ready", "ready-condition", readyCond)
		return true, nil // here we need to explicitly requeue since the controller doesn't watch the NSTemplateSetStatus
	}
	return false, r.setStatusReady(space)
}

func (r *Reconciler) newNSTemplateSet(memberOperatorNS string, space *toolchainv1alpha1.Space) (*toolchainv1alpha1.NSTemplateSet, error) {
	// look-up the NSTemplateTier
	tmplTier := &toolchainv1alpha1.NSTemplateTier{}
	if err := r.Client.Get(context.TODO(), types.NamespacedName{
		Namespace: space.Namespace,
		Name:      space.Spec.TierName,
	}, tmplTier); err != nil {
		return nil, err
	}
	// create the NSTemplateSet from the NSTemplateTier
	nsTmplSet := &toolchainv1alpha1.NSTemplateSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: memberOperatorNS,
			Name:      space.Name,
			Finalizers: []string{
				toolchainv1alpha1.FinalizerName,
			},
		},
		Spec: *usersignup.NewNSTemplateSetSpec(tmplTier),
	}
	return nsTmplSet, nil
}

func (r *Reconciler) ensureSpaceDeletion(logger logr.Logger, space *toolchainv1alpha1.Space) error {
	logger.Info("terminating Space")
	if isBeingDeleted, err := r.deleteNSTemplateSet(logger, space); err != nil {
		logger.Error(err, "failed to delete the NSTemplateSet")
		return r.setStatusTerminatingFailed(logger, space, err)
	} else if isBeingDeleted {
		if err := r.setStatusTerminating(space); err != nil {
			logger.Error(err, "error updating status")
			return err
		}
		return nil
	}
	// Remove finalizer from Space
	util.RemoveFinalizer(space, toolchainv1alpha1.FinalizerName)
	if err := r.Client.Update(context.TODO(), space); err != nil {
		logger.Error(err, "failed to remove finalizer")
		return r.setStatusTerminatingFailed(logger, space, err)
	}
	logger.Info("removed finalizer")
	// no need to update the status of the Space once the finalizer has been removed, since
	// the resource will be deleted
	return nil
}

// deleteNSTemplateSet triggers the deletion of the NSTemplateSet on the target member cluster.
// Returns `true/nil` if the NSTemplateSet is being deleted (whether deletion was triggered during this call,
// or if it was triggered earlier and is still in progress)
// Returns `false/nil` if the NSTemplateSet doesn't exist anymore,
//   or if there is no target cluster specified in the given space, or if the target cluster is unknown.
// Returns `false/error` if an error occurred
func (r *Reconciler) deleteNSTemplateSet(logger logr.Logger, space *toolchainv1alpha1.Space) (bool, error) {
	targetCluster := space.Spec.TargetCluster
	if targetCluster == "" {
		targetCluster = space.Status.TargetCluster
	}
	if targetCluster == "" {
		logger.Info("cannot delete NSTemplateSet: no target cluster specified")
		return false, nil // skip NSTemplateSet deletion
	}
	memberCluster, found := r.MemberClusters[targetCluster]
	if !found {
		logger.WithValues("target_cluster", targetCluster).Info("Cannot delete NSTemplateSet: unknown target member cluster")
		return false, nil // skip NSTemplateSet deletion
	}
	// Get the NSTemplateSet associated with the Space
	nstmplSet := &toolchainv1alpha1.NSTemplateSet{}
	err := memberCluster.Client.Get(context.TODO(),
		types.NamespacedName{
			Namespace: memberCluster.OperatorNamespace,
			Name:      space.Name},
		nstmplSet)
	if err != nil {
		if !errors.IsNotFound(err) {
			return false, err // something wrong happened
		}
		logger.Info("the NSTemplateSet resource is already deleted")
		return false, nil // NSTemplateSet was already deleted
	}
	if util.IsBeingDeleted(nstmplSet) {
		logger.Info("the NSTemplateSet resource is already being deleted")
		return true, nil // requeue until fully deleted
	}
	logger.Info("deleting the NSTemplateSet resource")
	// Delete NSTemplateSet associated with Space
	if err := memberCluster.Client.Delete(context.TODO(), nstmplSet); err != nil {
		if !errors.IsNotFound(err) {
			return false, err // something wrong happened
		}
		return false, nil // was already deleted in the mean time
	}
	logger.Info("deleted the NSTemplateSet resource")
	return true, nil // requeue until fully deleted
}

func (r *Reconciler) setStatusProvisioning(space *toolchainv1alpha1.Space) error {
	return r.updateStatus(
		space,
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.ConditionReady,
			Status: corev1.ConditionFalse,
			Reason: toolchainv1alpha1.SpaceProvisioningReason,
		})
}

func (r *Reconciler) setStatusProvisioningFailed(logger logr.Logger, space *toolchainv1alpha1.Space, cause error) error {
	if err := r.updateStatus(
		space,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.ConditionReady,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.SpaceProvisioningFailedReason,
			Message: cause.Error(),
		}); err != nil {
		logger.Error(cause, "unable to provision Space")
		return err
	}
	return cause
}

func (r *Reconciler) setStatusReady(space *toolchainv1alpha1.Space) error {
	return r.updateStatus(
		space,
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.ConditionReady,
			Status: corev1.ConditionTrue,
			Reason: toolchainv1alpha1.SpaceProvisionedReason,
		})
}

func (r *Reconciler) setStatusTerminating(space *toolchainv1alpha1.Space) error {
	return r.updateStatus(
		space,
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.ConditionReady,
			Status: corev1.ConditionFalse,
			Reason: toolchainv1alpha1.SpaceTerminatingReason,
		})
}

func (r *Reconciler) setStatusTerminatingFailed(logger logr.Logger, space *toolchainv1alpha1.Space, cause error) error {
	if err := r.updateStatus(
		space,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.ConditionReady,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.SpaceTerminatingFailedReason,
			Message: cause.Error(),
		}); err != nil {
		logger.Error(cause, "unable to terminate Space")
		return err
	}
	return cause
}

func (r *Reconciler) setStatusNSTemplateSetCreationFailed(logger logr.Logger, space *toolchainv1alpha1.Space, cause error) error {
	if err := r.updateStatus(
		space,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.ConditionReady,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.SpaceUnableToCreateNSTemplateSetReason,
			Message: cause.Error(),
		}); err != nil {
		logger.Error(cause, "unable to create NSTemplateSet")
		return err
	}
	return cause
}

// updateStatus updates space status conditions with the new conditions
func (r *Reconciler) updateStatus(space *toolchainv1alpha1.Space, conditions ...toolchainv1alpha1.Condition) error {
	var updated bool
	space.Status.TargetCluster = space.Spec.TargetCluster
	space.Status.Conditions, updated = condition.AddOrUpdateStatusConditions(space.Status.Conditions, conditions...)
	if !updated {
		// Nothing changed
		return nil
	}
	return r.Client.Status().Update(context.TODO(), space)
}
