package spacerequest

import (
	"context"
	"fmt"
	"reflect"
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/cluster"
	"github.com/codeready-toolchain/toolchain-common/pkg/condition"
	"github.com/go-logr/logr"
	errs "github.com/pkg/errors"
	"github.com/redhat-cop/operator-utils/pkg/util"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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

// Reconciler reconciles a SpaceRequest object
type Reconciler struct {
	Client         client.Client
	Scheme         *runtime.Scheme
	Namespace      string
	MemberClusters map[string]cluster.Cluster
}

// SetupWithManager sets up the controller reconciler with the Manager and the given member clusters.
// Watches SpaceRequests on the member clusters as its primary resources.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager, memberClusters map[string]cluster.Cluster) error {
	// since it's mandatory to add a primary resource when creating a new controller,
	// we add the SpaceRequest CR even if there should be no reconciles triggered from the host cluster,
	// only from member clusters (see watches below)
	// SpaceRequest owns subSpaces so events will be triggered for those from the host cluster.
	b := ctrl.NewControllerManagedBy(mgr).For(&toolchainv1alpha1.SpaceRequest{}).Watches(&source.Kind{Type: &toolchainv1alpha1.Space{}},
		handler.EnqueueRequestsFromMapFunc(MapSubSpaceToSpaceRequest()),
	)

	// Watch SpaceRequests in all member clusters and all namespaces.
	for _, memberCluster := range memberClusters {
		b = b.Watches(
			source.NewKindWithCache(&toolchainv1alpha1.SpaceRequest{}, memberCluster.Cache),
			&handler.EnqueueRequestForObject{},
			builder.WithPredicates(predicate.GenerationChangedPredicate{}),
		)
	}
	return b.Complete(r)
}

//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=spacerequests,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=spacerequests/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=spacerequests/finalizers,verbs=update

// Reconcile ensures that there is a SpaceRequest resource defined in the target member cluster
func (r *Reconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("reconciling SpaceRequest")

	// Fetch the SpaceRequest
	// search on all member clusters
	spaceRequest := &toolchainv1alpha1.SpaceRequest{}
	var memberClusterWithSpaceRequest cluster.Cluster
	memberClusterIndex := 0
	var err error
	for _, memberCluster := range r.MemberClusters {
		memberClusterIndex++
		err = memberCluster.Client.Get(context.TODO(), types.NamespacedName{
			Namespace: request.Namespace,
			Name:      request.Name,
		}, spaceRequest)
		if err != nil {
			if !errors.IsNotFound(err) {
				// Error reading the object - requeue the request.
				return reconcile.Result{}, errs.Wrap(err, "unable to get the current SpaceRequest")
			}

			//  spacerequest CR not found on current membercluster
			continue
		}

		// save the member cluster on which the SpaceRequest CR was found
		memberClusterWithSpaceRequest = memberCluster
	}
	// if we exited with a notFound error
	// it means that we couldn't find the spacerequest object on any of the given member clusters,
	// let's return the error.
	if err != nil && errors.IsNotFound(err) {
		return reconcile.Result{}, errs.Wrap(err, "unable to find SpaceRequest")
	}

	if util.IsBeingDeleted(spaceRequest) {
		logger.Info("spaceRequest is being deleted")
		return reconcile.Result{}, r.ensureSpaceRequestDeletion(logger, memberClusterWithSpaceRequest, spaceRequest)
	}
	// Add the finalizer if it is not present
	if err := r.addFinalizer(logger, memberClusterWithSpaceRequest, spaceRequest); err != nil {
		return reconcile.Result{}, err
	}

	subSpace, err := r.ensureSpace(logger, memberClusterWithSpaceRequest, spaceRequest)
	if err != nil {
		return ctrl.Result{}, err
	}

	// update spaceRequest conditions and target cluster url
	err = updateSpaceRequest(memberClusterWithSpaceRequest, spaceRequest, subSpace)

	return ctrl.Result{}, err
}

// updateSpaceRequest updates conditions and target cluster url of the spaceRequest with the ones on the Space resource
func updateSpaceRequest(memberClusterWithSpaceRequest cluster.Cluster, spaceRequest *toolchainv1alpha1.SpaceRequest, subSpace *toolchainv1alpha1.Space) error {
	// reflect space conditions and target URL to spaceRequest conditions
	spaceRequest.Status.Conditions = subSpace.Status.Conditions
	spaceRequest.Status.TargetClusterURL = subSpace.Status.TargetCluster
	return memberClusterWithSpaceRequest.Client.Status().Update(context.TODO(), spaceRequest)
}

// setFinalizers sets the finalizers for Space
func (r *Reconciler) addFinalizer(logger logr.Logger, memberCluster cluster.Cluster, spaceRequest *toolchainv1alpha1.SpaceRequest) error {
	// Add the finalizer if it is not present
	if !util.HasFinalizer(spaceRequest, toolchainv1alpha1.FinalizerName) {
		logger.Info("adding finalizer on SpaceRequest")
		util.AddFinalizer(spaceRequest, toolchainv1alpha1.FinalizerName)
		if err := memberCluster.Client.Update(context.TODO(), spaceRequest); err != nil {
			return errs.Wrap(err, "error while adding finalizer")
		}
	}
	return nil
}

func (r *Reconciler) ensureSpace(logger logr.Logger, memberCluster cluster.Cluster, spaceRequest *toolchainv1alpha1.SpaceRequest) (*toolchainv1alpha1.Space, error) {
	logger.Info("ensuring Space", "SpaceRequest", spaceRequest.Name)

	// validate tierName
	if err := r.validateNSTemplateTier(logger, spaceRequest.Spec.TierName); err != nil {
		return nil, err
	}

	// find parent space from namespace labels
	parentSpace, err := r.getParentSpace(logger, memberCluster, spaceRequest)
	if err != nil {
		logger.Error(err, "unable to get parentSpaceName")
		return nil, err
	}
	// parentSpace is being deleted
	if util.IsBeingDeleted(parentSpace) {
		return nil, errs.New("parentSpace is being deleted")
	}

	// get subSpace resource
	spaceList, err := r.listSubSpaces(spaceRequest)
	if err != nil {
		return nil, err
	}

	var subSpace *toolchainv1alpha1.Space
	switch {
	case len(spaceList.Items) == 0:
		// no spaces found, let's create it
		subSpace, err = r.createNewSubSpace(logger, spaceRequest, parentSpace)
		if err != nil {
			// failed to create subSpace
			return subSpace, r.setStatusFailedToCreateSubSpace(logger, memberCluster, spaceRequest, err)
		}
		return subSpace, nil

	case len(spaceList.Items) == 1:
		return r.updateExistingSubSpace(logger, spaceRequest, &spaceList.Items[0])

	default:
		// some unexpected issue causing to many subspaces
		return nil, fmt.Errorf("invalid number of spaces found: %d for spacerequest: %s", len(spaceList.Items), spaceRequest.GetName())
	}
}

func (r *Reconciler) listSubSpaces(spaceRequest *toolchainv1alpha1.SpaceRequest) (*toolchainv1alpha1.SpaceList, error) {
	spaceList := &toolchainv1alpha1.SpaceList{}
	spaceRequestLabel := client.MatchingLabels{
		toolchainv1alpha1.SpaceRequestLabelKey:          spaceRequest.GetName(),
		toolchainv1alpha1.SpaceRequestNamespaceLabelKey: spaceRequest.GetNamespace(),
	}
	if err := r.Client.List(context.TODO(), spaceList, spaceRequestLabel); err != nil {
		return nil, errs.Wrap(err, fmt.Sprintf(`attempt to list Spaces associated with spaceRequest %s in namespace %s failed`, spaceRequest.GetName(), spaceRequest.GetNamespace()))
	}
	return spaceList, nil
}

func (r *Reconciler) createNewSubSpace(logger logr.Logger, spaceRequest *toolchainv1alpha1.SpaceRequest, parentSpace *toolchainv1alpha1.Space) (*toolchainv1alpha1.Space, error) {
	subSpace := newSpace(spaceRequest, parentSpace.GetName(), r.Namespace)
	err := r.Client.Create(context.TODO(), subSpace)
	if err != nil {
		return subSpace, errs.Wrap(err, "unable to create space")
	}

	logger.Info("Created subSpace", "name", subSpace.Name, "target_cluster_roles", spaceRequest.Spec.TargetClusterRoles, "tierName", spaceRequest.Spec.TierName)
	return subSpace, nil
}

func (r *Reconciler) updateExistingSubSpace(logger logr.Logger, spaceRequest *toolchainv1alpha1.SpaceRequest, subSpace *toolchainv1alpha1.Space) (*toolchainv1alpha1.Space, error) {
	// subSpace found let's check if it's up to date
	logger.Info("subSpace already exists")
	// check if subSpace is being deleted
	if util.IsBeingDeleted(subSpace) {
		return nil, fmt.Errorf("cannot update subSpace because it is currently being deleted")
	}
	// update tier and clusterroles in the subspace
	// if they do not match the ones on the spaceRequest
	return r.updateSubSpace(logger, subSpace, spaceRequest)
}

// validateNSTemplateTier checks if the provided tierName in the spaceRequest exists and is valid
func (r *Reconciler) validateNSTemplateTier(logger logr.Logger, tierName string) error {
	if tierName == "" {
		return fmt.Errorf("tierName cannot be blank")
	}
	// check if requested tier exists
	tier := &toolchainv1alpha1.NSTemplateTier{}
	if err := r.Client.Get(context.TODO(), types.NamespacedName{
		Namespace: r.Namespace,
		Name:      tierName,
	}, tier); err != nil {
		if errors.IsNotFound(err) {
			logger.Error(err, "NSTemplateTier not found")
			return err
		}
		// Error reading the object - requeue the request.
		logger.Error(err, "unable to get the current NSTemplateTier")
		return errs.Wrap(err, "unable to get the current NSTemplateTier")
	}
	return nil
}

// updateSubSpace updates the tierName and targetClusterRoles from the spaceRequest to the subSpace object.
func (r *Reconciler) updateSubSpace(logger logr.Logger, subSpace *toolchainv1alpha1.Space, spaceRequest *toolchainv1alpha1.SpaceRequest) (*toolchainv1alpha1.Space, error) {
	logger.Info("update subSpace")
	if spaceRequest.Spec.TierName == subSpace.Spec.TierName &&
		reflect.DeepEqual(spaceRequest.Spec.TargetClusterRoles, subSpace.Spec.TargetClusterRoles) {
		// everything is up to date let's return
		return subSpace, nil
	}

	// update tier name and target cluster roles
	logger.Info("updating subSpace", "name", subSpace.Name)
	subSpace.Spec.TierName = spaceRequest.Spec.TierName
	subSpace.Spec.TargetClusterRoles = spaceRequest.Spec.TargetClusterRoles
	err := r.Client.Update(context.TODO(), subSpace)
	if err != nil {
		return subSpace, errs.Wrap(err, "unable to update tiername and targetclusterroles")
	}

	logger.Info("subSpace updated", "name", subSpace.Name, "target_cluster_roles", subSpace.Spec.TargetClusterRoles, "tierName", subSpace.Spec.TierName)
	return subSpace, nil
}

// getParentSpace retrieves the name of the space that provisioned the namespace in which the spacerequest was issued.
// we need to find the parentSpace so we can `link` the 2 spaces and inherit the spacebindings from the parent.
func (r *Reconciler) getParentSpace(logger logr.Logger, memberCluster cluster.Cluster, spaceRequest *toolchainv1alpha1.SpaceRequest) (*toolchainv1alpha1.Space, error) {
	parentSpace := &toolchainv1alpha1.Space{}
	namespace := &v1.Namespace{}
	err := memberCluster.Client.Get(context.TODO(), types.NamespacedName{
		Namespace: "",
		Name:      spaceRequest.Namespace,
	}, namespace)
	if err != nil {
		logger.Error(err, "unable to get namespace")
		return parentSpace, errs.Wrap(err, "unable to get namespace")
	}
	// get owner name which is equal to NSTemplateSet name and Space name
	parentSpaceName, found := namespace.Labels[toolchainv1alpha1.OwnerLabelKey]
	if !found || parentSpaceName == "" {
		err := errs.Errorf("unable to find owner label %s on namespace %s", toolchainv1alpha1.OwnerLabelKey, namespace.GetName())
		logger.Error(err, "unable to find owner label")
		return parentSpace, err
	}

	// check that parentSpaceStill exists and is not being deleted
	err = r.Client.Get(context.TODO(), types.NamespacedName{
		Namespace: r.Namespace,
		Name:      parentSpaceName,
	}, parentSpace)
	if err != nil {
		if errors.IsNotFound(err) {
			// parentSpace was deleted
			logger.Error(err, "the parentSpace is deleted")
			return parentSpace, errs.Wrap(err, "the parentSpace is deleted")
		}
		logger.Error(err, "unable to get parentSpace")
		return parentSpace, errs.Wrap(err, "unable to get parentSpace")
	}

	return parentSpace, nil // all good
}

func newSpace(spaceRequest *toolchainv1alpha1.SpaceRequest, parentSpaceName, subSpaceNamespace string) *toolchainv1alpha1.Space {
	labels := map[string]string{
		toolchainv1alpha1.SpaceRequestLabelKey:          spaceRequest.GetName(),
		toolchainv1alpha1.SpaceRequestNamespaceLabelKey: spaceRequest.GetNamespace(),
	}

	space := &toolchainv1alpha1.Space{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:    subSpaceNamespace,
			GenerateName: parentSpaceName + "-",
			Labels:       labels,
		},
		Spec: toolchainv1alpha1.SpaceSpec{
			TargetClusterRoles: spaceRequest.Spec.TargetClusterRoles,
			TierName:           spaceRequest.Spec.TierName,
			ParentSpace:        parentSpaceName,
		},
	}
	return space
}

func (r *Reconciler) ensureSpaceRequestDeletion(logger logr.Logger, memberClusterWithSpaceRequest cluster.Cluster, spaceRequest *toolchainv1alpha1.SpaceRequest) error {
	logger.Info("ensure SpaceRequest deletion")
	if isBeingDeleted, err := r.deleteSubSpace(logger, spaceRequest); err != nil {
		logger.Error(err, "failed to delete the subSpace")
		return r.setStatusTerminatingFailed(logger, memberClusterWithSpaceRequest, spaceRequest, err)
	} else if isBeingDeleted {
		if err := r.setStatusTerminating(memberClusterWithSpaceRequest, spaceRequest); err != nil {
			logger.Error(err, "error updating status")
			return errs.Wrap(err, "error updating status")
		}
		return nil
	}

	// Remove finalizer from SpaceRequest
	util.RemoveFinalizer(spaceRequest, toolchainv1alpha1.FinalizerName)
	if err := memberClusterWithSpaceRequest.Client.Update(context.TODO(), spaceRequest); err != nil {
		logger.Error(err, "failed to remove finalizer")
		return r.setStatusTerminatingFailed(logger, memberClusterWithSpaceRequest, spaceRequest, errs.Wrap(err, "failed to remove finalizer"))
	}
	logger.Info("removed finalizer")
	return nil
}

// deleteSubSpace checks if the subSpace is still provisioned and in case deletes it.
// returns true/nil if the deletion of the subSpace was triggered
// returns false/nil if the subSpace was already deleted
// return false/err if something went wrong
func (r *Reconciler) deleteSubSpace(logger logr.Logger, spaceRequest *toolchainv1alpha1.SpaceRequest) (bool, error) {
	subSpaces, err := r.listSubSpaces(spaceRequest)
	if err != nil {
		logger.Error(err, "unable to list subSpaces")
		return false, errs.Wrap(err, "unable to list subSpaces")
	}

	switch {
	case len(subSpaces.Items) == 0:
		// subSpace was already deleted
		return false, nil

	case len(subSpaces.Items) == 1:
		// deleting subSpace
		return r.deleteExistingSubSpace(logger, &subSpaces.Items[0])

	default:
		// something went wrong and there are too many subspaces
		logger.Error(err, "invalid number of subSpaces found")
		return false, fmt.Errorf("invalid number of subSpaces found")
	}

}

// deleteExistingSubSpace deletes a given space object in case deletion was not issued already.
// returns true/nil if the deletion of the space was triggered
// returns false/nil if the space was already deleted
// return false/err if something went wrong
func (r *Reconciler) deleteExistingSubSpace(logger logr.Logger, subSpace *toolchainv1alpha1.Space) (bool, error) {
	if util.IsBeingDeleted(subSpace) {
		logger.Info("the subSpace resource is already being deleted")
		deletionTimestamp := subSpace.GetDeletionTimestamp()
		if time.Since(deletionTimestamp.Time) > 120*time.Second {
			return false, fmt.Errorf("subSpace deletion has not completed in over 2 minutes")
		}
		return true, nil // subspace is still being deleted
	}

	logger.Info("deleting the subSpace resource", "subSpace name", subSpace.Name)
	// Delete subSpace associated with SpaceRequest
	if err := r.Client.Delete(context.TODO(), subSpace); err != nil {
		if errors.IsNotFound(err) {
			return false, nil // was already deleted
		}
		return false, errs.Wrap(err, "unable to delete subspace") // something wrong happened
	}
	logger.Info("deleted the subSpace resource", "subSpace name", subSpace.Name)
	return true, nil
}

func (r *Reconciler) setStatusTerminating(memberCluster cluster.Cluster, spaceRequest *toolchainv1alpha1.SpaceRequest) error {
	return r.updateStatus(
		spaceRequest,
		memberCluster,
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.ConditionReady,
			Status: v1.ConditionFalse,
			Reason: toolchainv1alpha1.SpaceTerminatingReason,
		})
}

func (r *Reconciler) setStatusTerminatingFailed(logger logr.Logger, memberCluster cluster.Cluster, spaceRequest *toolchainv1alpha1.SpaceRequest, cause error) error {
	if err := r.updateStatus(
		spaceRequest,
		memberCluster,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.ConditionReady,
			Status:  v1.ConditionFalse,
			Reason:  toolchainv1alpha1.SpaceTerminatingFailedReason,
			Message: cause.Error(),
		}); err != nil {
		logger.Error(cause, "unable to terminate Space")
		return err
	}
	return cause
}

func (r *Reconciler) setStatusFailedToCreateSubSpace(logger logr.Logger, memberCluster cluster.Cluster, spaceRequest *toolchainv1alpha1.SpaceRequest, cause error) error {
	if err := r.updateStatus(
		spaceRequest,
		memberCluster,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.ConditionReady,
			Status:  v1.ConditionFalse,
			Reason:  toolchainv1alpha1.SpaceProvisioningFailedReason,
			Message: cause.Error(),
		}); err != nil {
		logger.Error(err, "unable to terminate Space")
		return err
	}
	return cause
}

func (r *Reconciler) updateStatus(spaceRequest *toolchainv1alpha1.SpaceRequest, memberCluster cluster.Cluster, conditions ...toolchainv1alpha1.Condition) error {
	var updated bool
	spaceRequest.Status.Conditions, updated = condition.AddOrUpdateStatusConditions(spaceRequest.Status.Conditions, conditions...)
	if !updated {
		// Nothing changed
		return nil
	}
	return memberCluster.Client.Status().Update(context.TODO(), spaceRequest)
}
