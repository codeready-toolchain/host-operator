package spacebindingrequest

import (
	"context"
	"fmt"
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/cluster"
	"github.com/codeready-toolchain/toolchain-common/pkg/condition"
	"github.com/codeready-toolchain/toolchain-common/pkg/spacebinding"
	errs "github.com/pkg/errors"
	"github.com/redhat-cop/operator-utils/pkg/util"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// Reconciler reconciles a SpaceBindingRequest object
type Reconciler struct {
	Client         runtimeclient.Client
	Scheme         *runtime.Scheme
	Namespace      string
	MemberClusters map[string]cluster.Cluster
}

// SetupWithManager sets up the controller reconciler with the Manager and the given member clusters.
// Watches SpaceBindingRequests on the member clusters as its primary resources.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager, memberClusters map[string]cluster.Cluster) error {
	// since it's mandatory to add a primary resource when creating a new controller,
	// we add the SpaceBindingRequest CR even if there should be no reconciles triggered from the host cluster,
	// only from member clusters (see watches below)
	// SpaceBindingRequest owns spacebindings so events will be triggered for those from the host cluster.
	b := ctrl.NewControllerManagedBy(mgr).
		For(&toolchainv1alpha1.SpaceBindingRequest{}).
		Watches(&source.Kind{Type: &toolchainv1alpha1.SpaceBinding{}},
			handler.EnqueueRequestsFromMapFunc(MapSpaceBindingToSpaceBindingRequest()),
		)

	// Watch SpaceBindingRequests in all member clusters and all namespaces.
	for _, memberCluster := range memberClusters {
		b = b.Watches(
			source.NewKindWithCache(&toolchainv1alpha1.SpaceBindingRequest{}, memberCluster.Cache),
			&handler.EnqueueRequestForObject{},
			builder.WithPredicates(predicate.GenerationChangedPredicate{}))
	}
	return b.Complete(r)
}

//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=spacebindingrequests,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=spacebindingrequests/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=spacebindingrequests/finalizers,verbs=update

// Reconcile ensures that there is a SpaceBindingRequest resource defined in the target member cluster
func (r *Reconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("reconciling SpaceBindingRequest")

	// Fetch the SpaceBindingRequest
	// search on all member clusters
	spaceBindingRequest := &toolchainv1alpha1.SpaceBindingRequest{}
	memberClusterWithSpaceBindingRequest, found, err := cluster.LookupMember(ctx, r.MemberClusters, types.NamespacedName{
		Namespace: request.Namespace,
		Name:      request.Name,
	}, spaceBindingRequest)
	if err != nil {
		if !found {
			// got error while searching for SpaceBindingRequest CR
			return reconcile.Result{}, err
		}
		// Just log the error but proceed because we did find the member anyway
		logger.Error(err, "error while searching for SpaceBindingRequest")
	} else if !found {
		logger.Info("unable to find SpaceBindingRequest")
		return reconcile.Result{}, nil
	}
	logger.Info("spacebindingrequest found", "member cluster", memberClusterWithSpaceBindingRequest.Name)

	if util.IsBeingDeleted(spaceBindingRequest) {
		logger.Info("spaceBindingRequest is being deleted")
		return reconcile.Result{}, r.ensureSpaceBindingDeletion(ctx, memberClusterWithSpaceBindingRequest, spaceBindingRequest)
	}
	// Add the finalizer if it is not present
	if err := r.addFinalizer(ctx, memberClusterWithSpaceBindingRequest, spaceBindingRequest); err != nil {
		return reconcile.Result{}, err
	}

	err = r.ensureSpaceBinding(ctx, memberClusterWithSpaceBindingRequest, spaceBindingRequest)
	if err != nil {
		if errStatus := r.setStatusFailedToCreateSpaceBinding(ctx, memberClusterWithSpaceBindingRequest, spaceBindingRequest, err); errStatus != nil {
			logger.Error(errStatus, "error updating SpaceBindingRequest status")
		}
		return reconcile.Result{}, err
	}

	// set ready condition on spaceBindingRequest
	err = r.updateStatus(ctx, spaceBindingRequest, memberClusterWithSpaceBindingRequest, toolchainv1alpha1.Condition{
		Type:   toolchainv1alpha1.ConditionReady,
		Status: corev1.ConditionTrue,
		Reason: toolchainv1alpha1.SpaceBindingRequestProvisionedReason,
	})

	return ctrl.Result{}, err
}

func (r *Reconciler) ensureSpaceBindingDeletion(ctx context.Context, memberClusterWithSpaceBindingRequest cluster.Cluster, spaceBindingRequest *toolchainv1alpha1.SpaceBindingRequest) error {
	logger := log.FromContext(ctx)

	logger.Info("ensure spacebinding deletion")
	if !util.HasFinalizer(spaceBindingRequest, toolchainv1alpha1.FinalizerName) {
		// finalizer was already removed, nothing to delete anymore...
		return nil
	}
	spaceBinding, _, err := r.getSpaceBinding(ctx, spaceBindingRequest, memberClusterWithSpaceBindingRequest)
	if err != nil {
		return err
	}

	// delete the SpaceBinding only if it's owned by the current SBR
	if spaceBinding != nil && sbrOwnsSpaceBinding(spaceBinding, spaceBindingRequest) {
		if isBeingDeleted, err := r.deleteSpaceBinding(ctx, spaceBinding); err != nil {
			return r.setStatusTerminatingFailed(ctx, memberClusterWithSpaceBindingRequest, spaceBindingRequest, err)
		} else if isBeingDeleted {
			return r.setStatusTerminating(ctx, memberClusterWithSpaceBindingRequest, spaceBindingRequest)
		}
	}

	// Remove finalizer from SpaceBindingRequest
	util.RemoveFinalizer(spaceBindingRequest, toolchainv1alpha1.FinalizerName)
	if err := memberClusterWithSpaceBindingRequest.Client.Update(ctx, spaceBindingRequest); err != nil {
		return r.setStatusTerminatingFailed(ctx, memberClusterWithSpaceBindingRequest, spaceBindingRequest, errs.Wrap(err, "failed to remove finalizer"))
	}
	logger.Info("removed finalizer")
	return nil
}

// getSpaceBinding retrieves the spacebinding created by the spacebindingrequest
func (r *Reconciler) getSpaceBinding(ctx context.Context, spaceBindingRequest *toolchainv1alpha1.SpaceBindingRequest, memberCluster cluster.Cluster) (*toolchainv1alpha1.SpaceBinding, *toolchainv1alpha1.Space, error) {
	// find space from namespace labels
	space, err := r.getSpace(ctx, memberCluster, spaceBindingRequest)
	if err != nil {
		return nil, nil, err
	}
	spaceBindings := &toolchainv1alpha1.SpaceBindingList{}
	spaceBindingLabels := runtimeclient.MatchingLabels{
		toolchainv1alpha1.SpaceBindingSpaceLabelKey:            space.GetName(),
		toolchainv1alpha1.SpaceBindingMasterUserRecordLabelKey: spaceBindingRequest.Spec.MasterUserRecord,
	}
	err = r.Client.List(ctx, spaceBindings, spaceBindingLabels, runtimeclient.InNamespace(r.Namespace))
	if err != nil {
		return nil, space, errs.Wrap(err, "unable to list spacebindings")
	}

	// spacebinding not found
	if len(spaceBindings.Items) == 0 {
		return nil, space, nil
	}

	// more than one spacebinding
	if len(spaceBindings.Items) > 1 {
		return nil, space, fmt.Errorf("expected 1 spacebinding for Space %s and MUR %s. But found %d", space.GetName(), spaceBindingRequest.Spec.MasterUserRecord, len(spaceBindings.Items))
	}

	return &spaceBindings.Items[0], space, nil // all good
}

// deleteSpaceBinding deletes a given spacebinding object in case deletion was not issued already.
// returns true/nil if the deletion of the spacebinding was triggered
// returns false/nil if the spacebinding was already deleted
// return false/err if something went wrong
func (r *Reconciler) deleteSpaceBinding(ctx context.Context, spaceBinding *toolchainv1alpha1.SpaceBinding) (bool, error) {
	// spacebinding not found, was already deleted
	if spaceBinding == nil {
		return false, nil
	}

	logger := log.FromContext(ctx)
	if util.IsBeingDeleted(spaceBinding) {
		logger.Info("the spacebinding resource is already being deleted")
		deletionTimestamp := spaceBinding.GetDeletionTimestamp()
		if time.Since(deletionTimestamp.Time) > 120*time.Second {
			return false, fmt.Errorf("spacebinding deletion has not completed in over 2 minutes")
		}
		return true, nil // spacebinding is still being deleted
	}

	logger.Info("deleting the spacebinding resource", "spacebinding name", spaceBinding.Name)
	if err := r.Client.Delete(ctx, spaceBinding); err != nil {
		if errors.IsNotFound(err) {
			return false, nil // was already deleted
		}
		return false, errs.Wrap(err, "unable to delete spacebinding") // something wrong happened
	}
	logger.Info("deleted the spacebinding resource", "spacebinding name", spaceBinding.Name)
	return true, nil
}

// setFinalizers sets the finalizers for the SpaceBindingRequest
func (r *Reconciler) addFinalizer(ctx context.Context, memberCluster cluster.Cluster, spaceBindingRequest *toolchainv1alpha1.SpaceBindingRequest) error {
	// Add the finalizer if it is not present
	if !util.HasFinalizer(spaceBindingRequest, toolchainv1alpha1.FinalizerName) {
		logger := log.FromContext(ctx)
		logger.Info("adding finalizer on SpaceBindingRequest")
		util.AddFinalizer(spaceBindingRequest, toolchainv1alpha1.FinalizerName)
		if err := memberCluster.Client.Update(ctx, spaceBindingRequest); err != nil {
			return errs.Wrap(err, "error while adding finalizer")
		}
	}
	return nil
}

func (r *Reconciler) ensureSpaceBinding(ctx context.Context, memberCluster cluster.Cluster, spaceBindingRequest *toolchainv1alpha1.SpaceBindingRequest) error {
	logger := log.FromContext(ctx)
	logger.Info("ensuring spacebinding")
	// create spacebinding if not found for given spaceBindingRequest
	spaceBinding, space, err := r.getSpaceBinding(ctx, spaceBindingRequest, memberCluster)
	if err != nil {
		return err
	}

	// check if existing spacebinding was created from spaceBindingRequest
	// and return an error in case it was not.
	if spaceBinding != nil && !sbrOwnsSpaceBinding(spaceBinding, spaceBindingRequest) {
		return fmt.Errorf("A SpaceBinding for Space '%s' and MUR '%s' already exists, but it's not managed by this SpaceBindingRequest CR. It's not allowed to create multiple SpaceBindings for the same combination of Space and MasterUserRecord", space.GetName(), spaceBindingRequest.Spec.MasterUserRecord)
	}

	// space is being deleted
	if util.IsBeingDeleted(space) {
		return errs.New("space is being deleted")
	}

	// validate MUR
	mur, err := r.getMUR(ctx, spaceBindingRequest)
	if err != nil {
		return err
	}
	// mur is being deleted
	if util.IsBeingDeleted(mur) {
		return errs.New("mur is being deleted")
	}

	// validate Role
	if err := r.validateRole(ctx, spaceBindingRequest, space); err != nil {
		return err
	}

	// spacebinding not found, creating it
	if spaceBinding == nil {
		return r.createNewSpaceBinding(ctx, memberCluster, spaceBindingRequest, mur, space)
	}

	logger.Info("SpaceBinding already exists")
	return r.updateExistingSpaceBinding(ctx, spaceBindingRequest, spaceBinding)
}

// sbrOwnsSpaceBinding check if the SpaceBinding was created by the given SpaceBindingRequest resource
// returns true if it was created by the SBR
// returns false if it wasn't created by the SBR
func sbrOwnsSpaceBinding(spaceBinding *toolchainv1alpha1.SpaceBinding, spaceBindingRequest *toolchainv1alpha1.SpaceBindingRequest) bool {
	sbrLabel := spaceBinding.Labels[toolchainv1alpha1.SpaceBindingRequestLabelKey]
	sbrNamespaceLabel := spaceBinding.Labels[toolchainv1alpha1.SpaceBindingRequestNamespaceLabelKey]
	return sbrLabel == spaceBindingRequest.GetName() && sbrNamespaceLabel == spaceBindingRequest.GetNamespace()
}

// updateExistingSpaceBinding updates the spacebinding with the config from the spaceBindingRequest.
// returns true/nil if the spacebinding was updated
// returns false/nil if the spacebinding was already up-to-date
// returns false/err if something went wrong or the spacebinding is being deleted
func (r *Reconciler) updateExistingSpaceBinding(ctx context.Context, spaceBindingRequest *toolchainv1alpha1.SpaceBindingRequest, spaceBinding *toolchainv1alpha1.SpaceBinding) error {
	// check if spacebinding is being deleted
	if util.IsBeingDeleted(spaceBinding) {
		return errs.New("cannot update SpaceBinding because it is currently being deleted")
	}
	return r.updateSpaceBinding(ctx, spaceBinding, spaceBindingRequest)
}

// updateSpaceBinding updates the Role from the spaceBindingRequest to the spacebinding object
// if they are not up-to-date.
// returns false/nil if everything is up-to-date
// returns true/nil if spacebinding was updated
// returns false/err if something went wrong
func (r *Reconciler) updateSpaceBinding(ctx context.Context, spaceBinding *toolchainv1alpha1.SpaceBinding, spaceBindingRequest *toolchainv1alpha1.SpaceBindingRequest) error {
	logger := log.FromContext(ctx)
	logger.Info("update spaceBinding")
	if spaceBindingRequest.Spec.SpaceRole == spaceBinding.Spec.SpaceRole {
		// everything is up-to-date let's return
		return nil
	}

	// update SpaceRole and MUR
	logger.Info("updating spaceBinding", "spaceBinding.Name", spaceBinding.Name)
	spaceBinding.Spec.SpaceRole = spaceBindingRequest.Spec.SpaceRole
	err := r.Client.Update(ctx, spaceBinding)
	if err != nil {
		return errs.Wrap(err, "unable to update SpaceRole and MasterUserRecord fields")
	}

	logger.Info("spaceBinding updated", "spaceBinding.name", spaceBinding.Name, "spaceBinding.Spec.Space", spaceBinding.Spec.Space, "spaceBinding.Spec.SpaceRole", spaceBinding.Spec.SpaceRole, "spaceBinding.Spec.MasterUserRecord", spaceBinding.Spec.MasterUserRecord)
	return nil
}

func (r *Reconciler) createNewSpaceBinding(ctx context.Context, memberCluster cluster.Cluster, spaceBindingRequest *toolchainv1alpha1.SpaceBindingRequest, mur *toolchainv1alpha1.MasterUserRecord, space *toolchainv1alpha1.Space) error {
	spaceBinding := spacebinding.NewSpaceBinding(mur, space, spaceBindingRequest.Name)
	// set SBR labels on spacebinding
	spaceBinding.Labels[toolchainv1alpha1.SpaceBindingRequestLabelKey] = spaceBindingRequest.GetName()
	spaceBinding.Labels[toolchainv1alpha1.SpaceBindingRequestNamespaceLabelKey] = spaceBindingRequest.GetNamespace()

	logger := log.FromContext(ctx)
	logger.Info("creating spacebinding", "spaceBinding.Name", spaceBinding.Name)
	if err := r.setStatusProvisioning(ctx, memberCluster, spaceBindingRequest); err != nil {
		return err
	}
	if err := r.Client.Create(ctx, spaceBinding); err != nil {
		return errs.Wrap(err, "unable to create SpaceBinding")
	}
	logger.Info("Created SpaceBinding", "MUR", mur.Name, "Space", space.Name)
	return nil
}

// getSpace retrieves the name of the space that provisioned the namespace in which the spacebindingrequest was issued.
func (r *Reconciler) getSpace(ctx context.Context, memberCluster cluster.Cluster, spaceBindingRequest *toolchainv1alpha1.SpaceBindingRequest) (*toolchainv1alpha1.Space, error) {
	space := &toolchainv1alpha1.Space{}
	namespace := &corev1.Namespace{}
	err := memberCluster.Client.Get(ctx, types.NamespacedName{
		Namespace: "",
		Name:      spaceBindingRequest.Namespace,
	}, namespace)
	if err != nil {
		return nil, errs.Wrap(err, "unable to get namespace")
	}
	// get the Space name from the namespace resource
	spaceName, found := namespace.Labels[toolchainv1alpha1.SpaceLabelKey]
	if !found || spaceName == "" {
		return nil, errs.Errorf("unable to find space label %s on namespace %s", toolchainv1alpha1.SpaceLabelKey, namespace.GetName())
	}

	// get space object
	err = r.Client.Get(ctx, types.NamespacedName{
		Namespace: r.Namespace,
		Name:      spaceName,
	}, space)
	if err != nil {
		return nil, errs.Wrap(err, "unable to get space")
	}

	return space, nil // all good
}

// getMUR retrieves the MUR specified in the spaceBindingRequest.
func (r *Reconciler) getMUR(ctx context.Context, spaceBindingRequest *toolchainv1alpha1.SpaceBindingRequest) (*toolchainv1alpha1.MasterUserRecord, error) {
	if spaceBindingRequest.Spec.MasterUserRecord == "" {
		return nil, fmt.Errorf("MasterUserRecord cannot be blank")
	}
	mur := &toolchainv1alpha1.MasterUserRecord{}
	// check that MUR object exists
	err := r.Client.Get(ctx, types.NamespacedName{
		Namespace: r.Namespace,
		Name:      spaceBindingRequest.Spec.MasterUserRecord,
	}, mur)
	if err != nil {
		return nil, errs.Wrap(err, "unable to get MUR")
	}

	return mur, nil // all good
}

// validateRole checks if the role is within the allowed spaceroles from the NSTemplateTier
func (r *Reconciler) validateRole(ctx context.Context, spaceBindingRequest *toolchainv1alpha1.SpaceBindingRequest, space *toolchainv1alpha1.Space) error {
	if spaceBindingRequest.Spec.SpaceRole == "" {
		return fmt.Errorf("SpaceRole cannot be blank")
	}
	// get the tier
	nsTemplTier := &toolchainv1alpha1.NSTemplateTier{}
	if err := r.Client.Get(ctx, types.NamespacedName{
		Namespace: r.Namespace,
		Name:      space.Spec.TierName,
	}, nsTemplTier); err != nil {
		// Error reading the object - requeue the request.
		return errs.Wrap(err, "unable to get the current NSTemplateTier")
	}

	// search for the role
	for actual := range nsTemplTier.Spec.SpaceRoles {
		if spaceBindingRequest.Spec.SpaceRole == actual {
			return nil
		}
	}
	return fmt.Errorf("invalid role '%s' for space '%s'", spaceBindingRequest.Spec.SpaceRole, space.Name)
}

func (r *Reconciler) setStatusTerminating(ctx context.Context, memberCluster cluster.Cluster, spaceBindingRequest *toolchainv1alpha1.SpaceBindingRequest) error {
	return r.updateStatus(
		ctx,
		spaceBindingRequest,
		memberCluster,
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.ConditionReady,
			Status: corev1.ConditionFalse,
			Reason: toolchainv1alpha1.SpaceBindingRequestTerminatingReason,
		})
}

func (r *Reconciler) setStatusTerminatingFailed(ctx context.Context, memberCluster cluster.Cluster, spaceBindingRequest *toolchainv1alpha1.SpaceBindingRequest, cause error) error {
	if err := r.updateStatus(
		ctx,
		spaceBindingRequest,
		memberCluster,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.ConditionReady,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.SpaceBindingRequestTerminatingFailedReason,
			Message: cause.Error(),
		}); err != nil {
		logger := log.FromContext(ctx)
		logger.Error(cause, "unable to terminate SpaceBinding")
		return err
	}
	return cause
}

func (r *Reconciler) setStatusFailedToCreateSpaceBinding(ctx context.Context, memberCluster cluster.Cluster, spaceBindingRequest *toolchainv1alpha1.SpaceBindingRequest, cause error) error {
	if err := r.updateStatus(
		ctx,
		spaceBindingRequest,
		memberCluster,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.ConditionReady,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.SpaceBindingRequestUnableToCreateSpaceBindingReason,
			Message: cause.Error(),
		}); err != nil {
		logger := log.FromContext(ctx)
		logger.Error(err, "unable to create SpaceBinding")
		return err
	}
	return cause
}

func (r *Reconciler) setStatusProvisioning(ctx context.Context, memberCluster cluster.Cluster, spaceBindingRequest *toolchainv1alpha1.SpaceBindingRequest) error {
	return r.updateStatus(
		ctx,
		spaceBindingRequest,
		memberCluster,
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.ConditionReady,
			Status: corev1.ConditionFalse,
			Reason: toolchainv1alpha1.SpaceBindingRequestProvisioningReason,
		})
}

func (r *Reconciler) updateStatus(ctx context.Context, spaceBindingRequest *toolchainv1alpha1.SpaceBindingRequest, memberCluster cluster.Cluster, conditions ...toolchainv1alpha1.Condition) error {
	var updated bool
	spaceBindingRequest.Status.Conditions, updated = condition.AddOrUpdateStatusConditions(spaceBindingRequest.Status.Conditions, conditions...)
	if !updated {
		// Nothing changed
		return nil
	}
	return memberCluster.Client.Status().Update(ctx, spaceBindingRequest)
}
