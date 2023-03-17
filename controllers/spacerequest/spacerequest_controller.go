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

const norequeue = 0 * time.Second
const requeueDelay = 1 * time.Second

// Reconciler reconciles a SpaceRequest object
type Reconciler struct {
	Client         client.Client
	Namespace      string
	MemberClusters map[string]cluster.Cluster
}

// SetupWithManager sets up the controller reconciler with the Manager and the given member clusters.
// Watches SpaceRequests on the member clusters as its primary resources.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager, memberClusters map[string]cluster.Cluster) error {
	// since it's mandatory to add a primary resource when creating a new controller,
	// we add the SpaceRequest CR even if there should be not reconciles triggered from the host cluster,
	// only from member clusters, see Watches below.
	b := ctrl.NewControllerManagedBy(mgr).For(&toolchainv1alpha1.SpaceRequest{})
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

// Reconcile ensures that there is an SpaceRequest resource defined in the target member cluster
func (r *Reconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("reconciling SpaceRequest")

	// Fetch the SpaceRequest
	// search on all member clusters
	spaceRequest := &toolchainv1alpha1.SpaceRequest{}
	var memberClusterWithSpaceRequest cluster.Cluster
	var err error
	memberClusterIndex := 0
	for _, memberCluster := range r.MemberClusters {
		memberClusterIndex++
		err = memberCluster.Client.Get(context.TODO(), types.NamespacedName{
			Namespace: request.Namespace,
			Name:      request.Name,
		}, spaceRequest)
		if err != nil {
			if errors.IsNotFound(err) {
				// if this was the last member cluster to check
				// and still we couldn't find the spacerequest object
				// let's return the error
				if memberClusterIndex == len(r.MemberClusters) {
					return reconcile.Result{}, errs.Wrap(err, "unable to find SpaceRequest")
				}

				//  spacerequest CR not found on current membercluster
				continue
			}
			// Error reading the object - requeue the request.
			return reconcile.Result{}, errs.Wrap(err, "unable to get the current SpaceRequest")
		}
		// save the member cluster on which the SpaceRequest CR was found
		memberClusterWithSpaceRequest = memberCluster
	}
	if util.IsBeingDeleted(spaceRequest) {
		requeueTime, err := r.ensureSpaceRequestDeletion(logger, memberClusterWithSpaceRequest, spaceRequest)
		// wait for subSpace to be deleted
		if requeueTime > 0 {
			return reconcile.Result{Requeue: true, RequeueAfter: requeueTime}, err
		}
		return reconcile.Result{}, err
	}
	// Add the finalizer if it is not present
	if err := r.addFinalizer(logger, memberClusterWithSpaceRequest, spaceRequest); err != nil {
		return reconcile.Result{}, err
	}

	subSpace, requeueAfter, err := r.ensureSpace(logger, memberClusterWithSpaceRequest, spaceRequest)
	if err != nil {
		return ctrl.Result{}, err
	} else if requeueAfter > 0 {
		// if requeue was returned means that space was created let's set provisioning status for the SpaceRequest
		err = r.setStatusProvisioning(spaceRequest, memberClusterWithSpaceRequest)
		return ctrl.Result{
			Requeue:      true,
			RequeueAfter: requeueAfter,
		}, err
	}

	// update spaceRequest conditions and target cluster url
	err = updateSpaceRequest(memberClusterWithSpaceRequest, spaceRequest, subSpace)

	return ctrl.Result{}, err
}

// updateSpaceRequest updates conditions and target cluster url of the spaceRequest with the ones on the Space resource
func updateSpaceRequest(memberClusterWithSpaceRequest cluster.Cluster, spaceRequest *toolchainv1alpha1.SpaceRequest, subSpace *toolchainv1alpha1.Space) error {
	// reflect space conditions and target URL to spaceRequest conditions
	spaceRequest.Status.Conditions = subSpace.Status.Conditions
	spaceRequest.Status.TargetClusterURL = subSpace.Spec.TargetCluster
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

func (r *Reconciler) ensureSpace(logger logr.Logger, memberCluster cluster.Cluster, spaceRequest *toolchainv1alpha1.SpaceRequest) (*toolchainv1alpha1.Space, time.Duration, error) {
	logger.Info("ensuring Space", "SpaceRequest", spaceRequest.Name)

	subSpace := &toolchainv1alpha1.Space{}
	// validate tierName
	if spaceRequest.Spec.TierName == "" {
		return subSpace, norequeue, fmt.Errorf("tierName cannot be blank")
	}

	// find parent space from namespace labels
	parentSpace, err := r.getParentSpace(logger, memberCluster, spaceRequest)
	if err != nil {
		logger.Error(err, "unable to get parentSpaceName")
		return subSpace, 0, err
	}
	// parentSpace is being deleted
	if util.IsBeingDeleted(parentSpace) {
		return subSpace, 0, errs.New("parentSpace is being deleted")
	}

	// TODO: subspace name as of now is made by <parentSpace>-subs
	// we may need to review this logic for the naming,
	// also in case we need to support multiple spacerequests per namespace
	subSpaceName := createSubSpaceName(parentSpace.GetName())
	err = r.Client.Get(context.TODO(), types.NamespacedName{
		Namespace: r.Namespace,
		Name:      subSpaceName,
	}, subSpace)
	if err == nil {
		if util.IsBeingDeleted(subSpace) {
			return subSpace, norequeue, fmt.Errorf("cannot create subSpace because it is currently being deleted")
		}
		logger.Info("subSpace already exists")
		if !isSubSpaceReady(subSpace) {
			return subSpace, requeueDelay, nil
		}

		// update tier and clusterroles in the subspace
		// if they do not match the ones on the spaceRequest
		if updated, err := r.updateSubSpace(logger, *subSpace, spaceRequest); err != nil {
			return subSpace, norequeue, errs.Wrap(err, "unable to update subSpace")
		} else if updated {
			// requeue if we updated the subSpace
			return subSpace, requeueDelay, nil
		}

		// space is updated and ready
		return subSpace, norequeue, nil
	}
	if !errors.IsNotFound(err) {
		return subSpace, norequeue, errs.Wrap(err, fmt.Sprintf(`failed to get subSpace "%s"`, subSpaceName))
	}

	// check if requested tier exists
	tier := &toolchainv1alpha1.NSTemplateTier{}
	if err := r.Client.Get(context.TODO(), types.NamespacedName{
		Namespace: r.Namespace,
		Name:      spaceRequest.Spec.TierName,
	}, tier); err != nil {
		if errors.IsNotFound(err) {
			logger.Error(err, "NSTemplateTier not found")
			return subSpace, norequeue, err
		}
		// Error reading the object - requeue the request.
		logger.Error(err, "unable to get the current NSTemplateTier")
		return subSpace, norequeue, errs.Wrap(err, "unable to get the current NSTemplateTier")
	}

	subSpace = newSpace(subSpaceName, r.Namespace, parentSpace.GetName(), spaceRequest)
	err = r.Client.Create(context.TODO(), subSpace)
	if err != nil {
		return subSpace, norequeue, errs.Wrap(err, "unable to create space")
	}

	logger.Info("Created subSpace", "name", subSpace.Name, "target_cluster_roles", spaceRequest.Spec.TargetClusterRoles, "tierName", spaceRequest.Spec.TierName)
	return subSpace, requeueDelay, nil
}

func isSubSpaceReady(subSpace *toolchainv1alpha1.Space) bool {
	readyCond, found := condition.FindConditionByType(subSpace.Status.Conditions, toolchainv1alpha1.ConditionReady)
	// skip until there's a `Ready` condition
	if !found || readyCond.Status != "True" {
		// just created, but there is no `Ready` condition yet
		return false
	}
	// it's ready
	return true
}

// updateSubSpace updates the tierName and targetClusterRoles from the spaceRequest to the subSpace object.
// returns true/nil if the subSpace was updated and a reconcile is required
// returns false/nil if the subSpace was already up to date and no reconcile is required
// returns false/err in case an error occurred
func (r *Reconciler) updateSubSpace(logger logr.Logger, subSpace toolchainv1alpha1.Space, spaceRequest *toolchainv1alpha1.SpaceRequest) (bool, error) {
	logger.Info("update subSpace")
	if spaceRequest.Spec.TierName == subSpace.Spec.TierName &&
		reflect.DeepEqual(spaceRequest.Spec.TargetClusterRoles, subSpace.Spec.TargetClusterRoles) {
		// everything is up to date let's return
		return false, nil
	}

	// update tier name and target cluster roles
	logger.Info("updating subSpace", "name", subSpace.Name)
	subSpace.Spec.TierName = spaceRequest.Spec.TierName
	subSpace.Spec.TargetClusterRoles = spaceRequest.Spec.TargetClusterRoles
	err := r.Client.Update(context.TODO(), &subSpace)
	if err != nil {
		return false, errs.Wrap(err, "unable to update tiername and targetclusterroles")
	}

	logger.Info("subSpace updated", "name", subSpace.Name, "target_cluster_roles", subSpace.Spec.TargetClusterRoles, "tierName", subSpace.Spec.TierName)
	return true, nil
}

func (r *Reconciler) ensureSpaceRequestDeletion(logger logr.Logger, memberClusterWithSpaceRequest cluster.Cluster, spaceRequest *toolchainv1alpha1.SpaceRequest) (time.Duration, error) {
	logger.Info("ensure SpaceRequest deletion")
	if isBeingDeleted, err := r.deleteSubSpace(logger, spaceRequest, memberClusterWithSpaceRequest); err != nil {
		logger.Error(err, "failed to delete the subSpace")
		return norequeue, r.setStatusTerminatingFailed(logger, memberClusterWithSpaceRequest, spaceRequest, err)
	} else if isBeingDeleted {
		if err := r.setStatusTerminating(memberClusterWithSpaceRequest, spaceRequest); err != nil {
			logger.Error(err, "error updating status")
			return norequeue, err
		}
		return requeueDelay, nil // requeue until subSpace is gone
	}
	// Remove finalizer from Space
	util.RemoveFinalizer(spaceRequest, toolchainv1alpha1.FinalizerName)
	if err := memberClusterWithSpaceRequest.Client.Update(context.TODO(), spaceRequest); err != nil {
		logger.Error(err, "failed to remove finalizer")
		return norequeue, r.setStatusTerminatingFailed(logger, memberClusterWithSpaceRequest, spaceRequest, err)
	}
	logger.Info("removed finalizer")
	return norequeue, nil
}

func (r *Reconciler) deleteSubSpace(logger logr.Logger, spaceRequest *toolchainv1alpha1.SpaceRequest, memberCluster cluster.Cluster) (bool, error) {
	parentSpace, err := r.getParentSpace(logger, memberCluster, spaceRequest)
	if err != nil {
		logger.Error(err, "unable to get parentSpaceName")
		return false, errs.Wrap(err, "unable to get parentSpaceName")
	}
	// Get the subSpace created by the SpaceRequest
	subSpace := &toolchainv1alpha1.Space{}
	err = r.Client.Get(context.TODO(), types.NamespacedName{
		Namespace: r.Namespace,
		Name:      createSubSpaceName(parentSpace.GetName()),
	}, subSpace)
	if err != nil {
		if errors.IsNotFound(err) {
			// subSpace was already deleted
			logger.Info("the subSpace is already deleted")
			return false, nil
		}
		logger.Error(err, "unable to get subSpace")
		return false, errs.Wrap(err, "unable to get subSpace")
	}
	if util.IsBeingDeleted(subSpace) {
		logger.Info("the subSpace resource is already being deleted")
		deletionTimestamp := subSpace.GetDeletionTimestamp()
		if time.Since(deletionTimestamp.Time) > 120*time.Second {
			return false, fmt.Errorf("subSpace deletion has not completed in over 2 minutes")
		}
		return true, nil
	}

	logger.Info("deleting the subSpace resource", "subSpace name", subSpace.Name)
	// Delete subSpace associated with SpaceRequest
	if err := r.Client.Delete(context.TODO(), subSpace); err != nil {
		if errors.IsNotFound(err) {
			return false, nil // was already deleted
		}
		return false, err // something wrong happened
	}
	logger.Info("deleted the subSpace resources", "subSpace name", subSpace.Name)
	return true, nil // requeue until fully deleted
}

// createSubSpaceName creates the name of the subSpace based on the parent one.
func createSubSpaceName(parentSpaceName string) string {
	// todo check how many spacerequest exist in current namespace
	// and use that as an id to append to the subSpace name
	return fmt.Sprintf("%s-%s", parentSpaceName, "subs")
}

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

func newSpace(subSpaceName, subSpaceNamespace, parentSpaceName string, spaceRequest *toolchainv1alpha1.SpaceRequest) *toolchainv1alpha1.Space {
	labels := map[string]string{
		toolchainv1alpha1.SpaceCreatorLabelKey: spaceRequest.GetName(),
		toolchainv1alpha1.OwnerLabelKey:        spaceRequest.GetName(),
	}

	space := &toolchainv1alpha1.Space{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: subSpaceNamespace,
			Name:      subSpaceName,
			Labels:    labels,
		},
		Spec: toolchainv1alpha1.SpaceSpec{
			TargetClusterRoles: spaceRequest.Spec.TargetClusterRoles,
			TierName:           spaceRequest.Spec.TierName,
			ParentSpace:        parentSpaceName,
		},
	}
	return space
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

func (r *Reconciler) setStatusProvisioning(spaceRequest *toolchainv1alpha1.SpaceRequest, memberCluster cluster.Cluster) error {
	return r.updateStatus(
		spaceRequest,
		memberCluster,
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.ConditionReady,
			Status: v1.ConditionFalse,
			Reason: toolchainv1alpha1.SpaceProvisioningReason,
		})
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
