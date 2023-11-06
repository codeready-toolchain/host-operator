package space

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/cluster"
	"github.com/codeready-toolchain/host-operator/pkg/counter"
	"github.com/codeready-toolchain/host-operator/pkg/mapper"
	"github.com/codeready-toolchain/toolchain-common/pkg/condition"
	"github.com/codeready-toolchain/toolchain-common/pkg/hash"
	"github.com/codeready-toolchain/toolchain-common/pkg/spacebinding"

	errs "github.com/pkg/errors"
	"github.com/redhat-cop/operator-utils/pkg/util"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

// Reconciler reconciles a Space object
type Reconciler struct {
	Client              runtimeclient.Client
	Namespace           string
	MemberClusters      map[string]cluster.Cluster
	NextScheduledUpdate time.Time
	LastExecutedUpdate  time.Time
}

// SetupWithManager sets up the controller reconciler with the Manager and the given member clusters.
// Watches the Space resources in the current (host) cluster as its primary resources.
// Watches NSTemplateSets on the member clusters as its secondary resources.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager, memberClusters map[string]cluster.Cluster) error {
	b := ctrl.NewControllerManagedBy(mgr).
		// watch Spaces in the host cluster
		For(&toolchainv1alpha1.Space{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Watches(&source.Kind{Type: &toolchainv1alpha1.NSTemplateTier{}},
			handler.EnqueueRequestsFromMapFunc(MapNSTemplateTierToSpaces(r.Namespace, r.Client)),
			builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Watches(&source.Kind{Type: &toolchainv1alpha1.SpaceBinding{}},
			handler.EnqueueRequestsFromMapFunc(MapSpaceBindingToParentAndSubSpaces(r.Client)))
	// watch NSTemplateSets in all the member clusters
	for _, memberCluster := range memberClusters {
		b = b.Watches(source.NewKindWithCache(&toolchainv1alpha1.NSTemplateSet{}, memberCluster.Cache),
			handler.EnqueueRequestsFromMapFunc(mapper.MapByResourceName(r.Namespace)),
		)
	}
	return b.Complete(r)
}

//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=spaces,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=spaces/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=spaces/finalizers,verbs=update

// Reconcile ensures that there is an NSTemplateSet resource defined in the target member cluster
func (r *Reconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("reconciling Space")

	// Fetch the Space
	space := &toolchainv1alpha1.Space{}
	err := r.Client.Get(ctx, types.NamespacedName{
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
		if err := r.addFinalizer(ctx, space); err != nil {
			return reconcile.Result{}, err
		}
	} else {
		return reconcile.Result{}, r.ensureSpaceDeletion(ctx, space)
	}

	// if the NSTemplateSet was created or updated, we want to make sure that the NSTemplateSet Controller was kicked before
	// reconciling the Space again. In particular, when the NSTemplateSet.Spec is updated, if the Space Controller is triggered
	// *before* the NSTemplateSet Controller and the NSTemplateSet's status is still `Provisioned` (as it was with the previous templates)
	// then the Space Controller will immediately set the Space status to `Provisioned` whereas in fact, the template update
	// did not even start yet!
	// Note: there are 2 durations involved here:
	// 1. Within 1 second after the Space status was set to `Updating`, the SpaceController considers it's too early and will requeue
	// 2. The requeue duration is set to 3 seconds, but in practice, the SpaceController will be triggered as soon as the NSTemplateSet
	//    status is updated by its own controller
	if requeueAfter, err := r.ensureNSTemplateSet(ctx, space); err != nil {
		return ctrl.Result{}, err
	} else if requeueAfter > 0 {
		return ctrl.Result{
			Requeue:      true,
			RequeueAfter: requeueAfter,
		}, nil
	}

	return ctrl.Result{}, nil
}

// setFinalizers sets the finalizers for Space
func (r *Reconciler) addFinalizer(ctx context.Context, space *toolchainv1alpha1.Space) error {
	// Add the finalizer if it is not present
	if !util.HasFinalizer(space, toolchainv1alpha1.FinalizerName) {
		logger := log.FromContext(ctx)
		logger.Info("adding finalizer on Space")
		util.AddFinalizer(space, toolchainv1alpha1.FinalizerName)
		if err := r.Client.Update(ctx, space); err != nil {
			return err
		}
	}
	return nil
}

const norequeue = 0 * time.Second
const requeueDelay = 1 * time.Second
const postponeDelay = 2 * time.Second

// ensureNSTemplateSet creates the NSTemplateSet on the target member cluster if it does not exist,
// and updates the space's status accordingly.
// Returns `true`+duration if the NSTemplateSet was *just* created or updated, ie:
// - it has no `Ready` condition yet,
// - the Space's `Ready=false/Updating` condition is too recent, so we're not sure that the NSTemplateSetController was triggered.
// OR if the space needs to be updated but later (use returned duration for as `requeueAfter`)
// Returns `false` otherwise
func (r *Reconciler) ensureNSTemplateSet(ctx context.Context, space *toolchainv1alpha1.Space) (time.Duration, error) { //nolint:gocyclo
	logger := log.FromContext(ctx)

	// deprovision from space.Status.TargetCluster if needed
	if space.Status.TargetCluster != "" && space.Spec.TargetCluster != space.Status.TargetCluster {
		logger.Info("retargeting space", "from_cluster", space.Status.TargetCluster, "to_cluster", space.Spec.TargetCluster)
		// look-up and delete the NSTemplateSet on the current member cluster
		if isBeingDeleted, err := r.deleteNSTemplateSetFromCluster(ctx, space, space.Status.TargetCluster); err != nil {
			return norequeue, r.setStatusRetargetFailed(ctx, space, err)

		} else if isBeingDeleted {
			logger.Info("wait while NSTemplateSet is being deleted", "member_cluster", space.Status.TargetCluster)
			return norequeue, r.setStatusRetargeting(ctx, space)
		} else {
			logger.Info("resetting 'space.Status.TargetCluster' field")
			// NSTemplateSet was removed: reset `space.Status.TargetCluster`
			space.Status.TargetCluster = ""
			if err := r.Client.Status().Update(ctx, space); err != nil {
				return norequeue, err
			}
			// and continue with the provisioning on the new target member cluster (if specified)
		}
	}

	if space.Spec.TierName == "" {
		if err := r.setStateLabel(ctx, space, toolchainv1alpha1.SpaceStateLabelValuePending); err != nil {
			return norequeue, err
		}
		return norequeue, r.setStatusProvisioningPending(ctx, space, "unspecified tier name")
	}
	if space.Spec.TargetCluster == "" {
		if err := r.setStateLabel(ctx, space, toolchainv1alpha1.SpaceStateLabelValuePending); err != nil {
			return norequeue, err
		}
		return norequeue, r.setStatusProvisioningPending(ctx, space, "unspecified target member cluster")
	}
	if err := r.setStateLabel(ctx, space, toolchainv1alpha1.SpaceStateLabelValueClusterAssigned); err != nil {
		return norequeue, err
	}

	memberCluster, found := r.MemberClusters[space.Spec.TargetCluster]
	if !found {
		return norequeue, r.setStatusProvisioningFailed(ctx, space, fmt.Errorf("unknown target member cluster '%s'", space.Spec.TargetCluster))
	}

	logger = logger.WithValues("target_member_cluster", space.Spec.TargetCluster)
	log.IntoContext(ctx, logger)
	// look-up the NSTemplateTier used by this Space
	tmplTier := &toolchainv1alpha1.NSTemplateTier{}
	if err := r.Client.Get(ctx, types.NamespacedName{
		Namespace: space.Namespace,
		Name:      space.Spec.TierName,
	}, tmplTier); err != nil {
		return norequeue, r.setStatusProvisioningFailed(ctx, space, err)
	}

	nsTmplSet, duration, err := r.manageNSTemplateSet(ctx, space, memberCluster, tmplTier)
	// return if there was an error or a requeue was requested
	if err != nil || duration > 0 {
		return duration, err
	}

	// we don't need to check if the condition was found here, since it is already done in the `manageNSTemplateSet` function, if we reach this point,
	// it's guaranteed that it exists and we only need to check the Reason.
	nsTmplSetReady, _ := condition.FindConditionByType(nsTmplSet.Status.Conditions, toolchainv1alpha1.ConditionReady)
	// also, replicates (translate) the NSTemplateSet's `ready` condition into the Space, including when `ready/true/provisioned`
	switch nsTmplSetReady.Reason {
	case toolchainv1alpha1.NSTemplateSetUpdatingReason:
		return norequeue, r.setStatusUpdating(ctx, space)
	case toolchainv1alpha1.NSTemplateSetProvisioningReason:
		return norequeue, r.setStatusProvisioning(ctx, space)
	case toolchainv1alpha1.NSTemplateSetProvisionedReason:
		readyCond, ok := condition.FindConditionByType(space.Status.Conditions, toolchainv1alpha1.ConditionReady)
		logger.Info("checking Space condition", "ready", readyCond)
		if ok && readyCond.Reason == toolchainv1alpha1.SpaceUpdatingReason && time.Since(readyCond.LastTransitionTime.Time) <= requeueDelay {
			// Space status was *just* set to `Ready=false/Updating`, so we need to wait
			return requeueDelay, nil
		}

		if space.Labels == nil {
			space.Labels = map[string]string{}
		}

		// remove any outdated tier hash labels
		for key := range space.GetLabels() {
			if strings.HasPrefix(key, toolchainv1alpha1.LabelKeyPrefix) && strings.HasSuffix(key, "-tier-hash") {
				delete(space.Labels, key)
			}
		}
		// check parentSpace label and update it if needed
		setParentSpaceLabel(space)

		// add a tier hash label matching the current NSTemplateTier
		h, err := hash.ComputeHashForNSTemplateTier(tmplTier)
		if err != nil {
			err = errs.Wrap(err, "error computing hash for NSTemplateTier")
			return norequeue, r.setStatusProvisioningFailed(ctx, space, err)
		}
		space.Labels[hash.TemplateTierHashLabelKey(space.Spec.TierName)] = h
		if err := r.Client.Update(ctx, space); err != nil {
			return norequeue, r.setStatusProvisioningFailed(ctx, space, err)
		}

		// update provisioned namespace list
		err = r.setStatusProvisionedNamespaces(ctx, space, nsTmplSet.Status.ProvisionedNamespaces)
		if err != nil {
			err = errs.Wrap(err, "error setting provisioned namespaces")
			return norequeue, err
		}

		return norequeue, r.setStatusProvisioned(ctx, space)
	default:
		return norequeue, r.setStatusProvisioningFailed(ctx, space, fmt.Errorf(nsTmplSetReady.Message))
	}
}

// manageNSTemplateSet creates or updates the NSTemplateSet of a given space.
// returns NSTemplateSet{}, requeueDelay, error
func (r *Reconciler) manageNSTemplateSet(ctx context.Context, space *toolchainv1alpha1.Space, memberCluster cluster.Cluster, tmplTier *toolchainv1alpha1.NSTemplateTier) (*toolchainv1alpha1.NSTemplateSet, time.Duration, error) {
	logger := log.FromContext(ctx)

	// copying the `space.Spec.TargetCluster` into `space.Status.TargetCluster` in case the former is reset or changed (ie, when retargeting to another cluster)
	// We set the .Status.TargetCluster only when the NSTemplateSet creation was attempted.
	// When deletion of the Space with space.Status.TargetCluster set is triggered, we know that there might be a NSTemplateSet resource to clean up as well.
	space.Status.TargetCluster = space.Spec.TargetCluster
	listSpaceBindingsFunc := func(spaceName string) ([]toolchainv1alpha1.SpaceBinding, error) {
		spacebindings := toolchainv1alpha1.SpaceBindingList{}
		if err := r.Client.List(ctx,
			&spacebindings,
			runtimeclient.InNamespace(space.Namespace),
			runtimeclient.MatchingLabels{
				toolchainv1alpha1.SpaceBindingSpaceLabelKey: spaceName,
			},
		); err != nil {
			return nil, err
		}
		return spacebindings.Items, nil
	}
	getSpaceFunc := func(spaceName string) (*toolchainv1alpha1.Space, error) {
		spaceFound := &toolchainv1alpha1.Space{}
		err := r.Client.Get(ctx, types.NamespacedName{
			Namespace: r.Namespace,
			Name:      spaceName,
		}, spaceFound)
		if err != nil {
			return nil, errs.Wrap(err, "unable to get space")
		}
		return spaceFound, nil
	}
	spaceBindingLister := spacebinding.NewLister(listSpaceBindingsFunc, getSpaceFunc)
	spaceBindings, err := spaceBindingLister.ListForSpace(space, []toolchainv1alpha1.SpaceBinding{})
	if err != nil {
		logger.Error(err, "failed to list space bindings")
	}
	// create if not found on the expected target cluster
	nsTmplSet := &toolchainv1alpha1.NSTemplateSet{}
	if err := memberCluster.Client.Get(ctx, types.NamespacedName{
		Namespace: memberCluster.OperatorNamespace,
		Name:      space.Name,
	}, nsTmplSet); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("creating NSTemplateSet on target member cluster")
			if err := r.setStatusProvisioning(ctx, space); err != nil {
				return nsTmplSet, norequeue, r.setStatusProvisioningFailed(ctx, space, err)
			}
			nsTmplSet = NewNSTemplateSet(memberCluster.OperatorNamespace, space, spaceBindings, tmplTier)
			if err := memberCluster.Client.Create(ctx, nsTmplSet); err != nil {
				if errors.IsAlreadyExists(err) {
					// requeue, there's probably a race condition between the host client cache and the member cluster state
					return nsTmplSet, requeueDelay, nil
				}
				return nsTmplSet, norequeue, r.setStatusNSTemplateSetCreationFailed(ctx, space, err)
			}
			logger.Info("NSTemplateSet created on target member cluster")
			counter.IncrementSpaceCount(logger, space.Spec.TargetCluster)

			return nsTmplSet, requeueDelay, r.setStatusProvisioning(ctx, space)
		}
		return nsTmplSet, norequeue, r.setStatusNSTemplateSetCreationFailed(ctx, space, err)
	}
	logger.Info("NSTemplateSet already exists")

	_, found := condition.FindConditionByType(nsTmplSet.Status.Conditions, toolchainv1alpha1.ConditionReady)
	// skip until there's a `Ready` condition
	if !found {
		// just created, but there is no `Ready` condition yet
		return nsTmplSet, requeueDelay, nil
	}

	// update the NSTemplateSet if needed (including in case of missing space roles)
	nsTmplSetSpec := NewNSTemplateSetSpec(space, spaceBindings, tmplTier)
	if !reflect.DeepEqual(nsTmplSet.Spec, nsTmplSetSpec) {
		logger.Info("NSTemplateSet is not up-to-date")
		// postpone NSTemplateSet updates if needed (but only for NSTemplateTier updates, not tier promotions or changes in spacebindings)
		if space.Labels[hash.TemplateTierHashLabelKey(space.Spec.TierName)] != "" &&
			condition.IsTrue(space.Status.Conditions, toolchainv1alpha1.ConditionReady) {
			// postpone if needed, so we don't overflow the cluster with too many concurrent updates

			logger.Info("time since last tier update", "seconds", time.Since(r.LastExecutedUpdate).Seconds())
			if time.Since(r.LastExecutedUpdate) < postponeDelay { // ie, if last update occurred less than 2s ago
				if time.Now().After(r.NextScheduledUpdate) { // happens when there was no previous update scheduled since controller started
					r.NextScheduledUpdate = time.Now().Add(postponeDelay)
				} else { // if at least one postponed schedule occurred
					r.NextScheduledUpdate = r.NextScheduledUpdate.Add(postponeDelay)
				}
				// return the duration when it should be requeued
				logger.Info("postponing NSTemplateSet update", "until", r.NextScheduledUpdate.String())
				return nsTmplSet, time.Until(r.NextScheduledUpdate), nil
			}
			r.LastExecutedUpdate = time.Now()
		}
		nsTmplSet.Spec = nsTmplSetSpec
		if err := memberCluster.Client.Update(ctx, nsTmplSet); err != nil {
			return nsTmplSet, norequeue, r.setStatusNSTemplateSetUpdateFailed(ctx, space, err)
		}
		// also, immediately update Space condition
		logger.Info("NSTemplateSet updated on target member cluster")
		return nsTmplSet, requeueDelay, r.setStatusUpdating(ctx, space)
	}
	logger.Info("NSTemplateSet is up-to-date")
	return nsTmplSet, 0, nil
}

func setParentSpaceLabel(space *toolchainv1alpha1.Space) {
	if space.Spec.ParentSpace == "" {
		// there is no parent-space label to be set
		return
	}

	// set parent-space label according to .spec.ParentSpace field value
	if parentSpace, found := space.Labels[toolchainv1alpha1.ParentSpaceLabelKey]; !found || parentSpace != space.Spec.ParentSpace {
		space.Labels[toolchainv1alpha1.ParentSpaceLabelKey] = space.Spec.ParentSpace
	}
}

func NewNSTemplateSet(namespace string, space *toolchainv1alpha1.Space, bindings []toolchainv1alpha1.SpaceBinding, tmplTier *toolchainv1alpha1.NSTemplateTier) *toolchainv1alpha1.NSTemplateSet {
	// create the NSTemplateSet from the NSTemplateTier
	nsTmplSet := &toolchainv1alpha1.NSTemplateSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      space.Name,
		},
	}
	nsTmplSet.Spec = NewNSTemplateSetSpec(space, bindings, tmplTier)
	return nsTmplSet
}

func NewNSTemplateSetSpec(space *toolchainv1alpha1.Space, bindings []toolchainv1alpha1.SpaceBinding, tmplTier *toolchainv1alpha1.NSTemplateTier) toolchainv1alpha1.NSTemplateSetSpec {
	s := toolchainv1alpha1.NSTemplateSetSpec{
		TierName: space.Spec.TierName,
	}
	if tmplTier.Spec.ClusterResources != nil {
		s.ClusterResources = &toolchainv1alpha1.NSTemplateSetClusterResources{
			TemplateRef: tmplTier.Spec.ClusterResources.TemplateRef,
		}
	}
	if len(tmplTier.Spec.Namespaces) > 0 {
		s.Namespaces = make([]toolchainv1alpha1.NSTemplateSetNamespace, len(tmplTier.Spec.Namespaces))
		for i, ns := range tmplTier.Spec.Namespaces {
			s.Namespaces[i] = toolchainv1alpha1.NSTemplateSetNamespace(ns)
		}
	}
	// space roles
	if len(bindings) > 0 {
		s.SpaceRoles = make([]toolchainv1alpha1.NSTemplateSetSpaceRole, 0, len(tmplTier.Spec.SpaceRoles))
		// append by alphabetical order of role names
		roles := make([]string, 0, len(tmplTier.Spec.SpaceRoles))
		for r := range tmplTier.Spec.SpaceRoles {
			roles = append(roles, r)
		}
		sort.Strings(roles)
		for _, r := range roles {
			sr := tmplTier.Spec.SpaceRoles[r]
			usernames := extractUsernames(r, bindings)
			// no need to add an entry in space roles if there is no associated user
			if len(usernames) > 0 {
				s.SpaceRoles = append(s.SpaceRoles, toolchainv1alpha1.NSTemplateSetSpaceRole{
					TemplateRef: sr.TemplateRef,
					Usernames:   usernames,
				})
			}
		}
	}
	return s
}

func extractUsernames(role string, bindings []toolchainv1alpha1.SpaceBinding) []string {
	usernames := map[string]interface{}{} // using a map to avoid duplicate entries
	for _, b := range bindings {
		if b.Spec.SpaceRole == role {
			usernames[b.Spec.MasterUserRecord] = struct{}{} // value doesn't matter
		}
	}
	result := make([]string, 0, len(usernames))
	for u := range usernames {
		result = append(result, u)
	}
	sort.Strings(result)
	return result
}

func (r *Reconciler) ensureSpaceDeletion(ctx context.Context, space *toolchainv1alpha1.Space) error {
	logger := log.FromContext(ctx)

	logger.Info("terminating Space")
	if isBeingDeleted, err := r.deleteNSTemplateSet(ctx, space); err != nil {
		// space was already provisioned to a cluster
		// let's not proceed with deletion
		if space.Status.TargetCluster != "" {
			logger.Error(err, "failed to delete the NSTemplateSet")
			return r.setStatusTerminatingFailed(ctx, space, err)
		}
		logger.Error(err, "error while deleting NSTemplateSet - ignored since the target cluster in the Status is empty")
	} else if isBeingDeleted {
		if err := r.setStatusTerminating(ctx, space); err != nil {
			logger.Error(err, "error updating status")
			return err
		}
		return nil
	}
	// Remove finalizer from Space
	util.RemoveFinalizer(space, toolchainv1alpha1.FinalizerName)
	if err := r.Client.Update(ctx, space); err != nil {
		logger.Error(err, "failed to remove finalizer")
		return r.setStatusTerminatingFailed(ctx, space, err)
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
//
//	or if there is no target cluster specified in the given space, or if the target cluster is unknown.
//
// Returns `false/error` if an error occurred
func (r *Reconciler) deleteNSTemplateSet(ctx context.Context, space *toolchainv1alpha1.Space) (bool, error) {
	targetCluster := space.Spec.TargetCluster
	if targetCluster == "" {
		targetCluster = space.Status.TargetCluster
	}
	if targetCluster == "" {
		logger := log.FromContext(ctx)
		logger.Info("cannot delete NSTemplateSet: no target cluster specified")
		return false, nil // skip NSTemplateSet deletion
	}
	return r.deleteNSTemplateSetFromCluster(ctx, space, targetCluster)
}

// deleteNSTemplateSetFromCluster triggers the deletion of the NSTemplateSet on the given member cluster.
// Returns `false/nil` if the NSTemplateSet is being deleted (whether deletion was triggered during this call,
// or if it was triggered earlier and is still in progress)
// Returns `true/nil` if the NSTemplateSet doesn't exist anymore,
//
//	or if there is no target cluster specified in the given space, or if the target cluster is unknown.
//
// Returns `false/error` if an error occurred
func (r *Reconciler) deleteNSTemplateSetFromCluster(ctx context.Context, space *toolchainv1alpha1.Space, targetCluster string) (bool, error) {
	logger := log.FromContext(ctx)

	memberCluster, found := r.MemberClusters[targetCluster]
	if !found {
		return false, fmt.Errorf("cannot delete NSTemplateSet: unknown target member cluster: '%s'", targetCluster)
	}
	// Get the NSTemplateSet associated with the Space
	nstmplSet := &toolchainv1alpha1.NSTemplateSet{}
	err := memberCluster.Client.Get(ctx,
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
		deletionTimestamp := nstmplSet.GetDeletionTimestamp()
		if time.Since(deletionTimestamp.Time) > 60*time.Second {
			return false, fmt.Errorf("NSTemplateSet deletion has not completed in over 1 minute")
		}
		return true, nil
	}

	logger.Info("deleting the NSTemplateSet resource")
	// Delete NSTemplateSet associated with Space
	if err := memberCluster.Client.Delete(ctx, nstmplSet); err != nil {
		if !errors.IsNotFound(err) {
			return false, err // something wrong happened
		}
		return false, nil // was already deleted in the mean time
	}
	counter.DecrementSpaceCount(logger, space.Status.TargetCluster)
	logger.Info("deleted the NSTemplateSet resource")
	return true, nil // requeue until fully deleted
}

func (r *Reconciler) setStateLabel(ctx context.Context, space *toolchainv1alpha1.Space, state string) error {
	oldState := space.Labels[toolchainv1alpha1.SpaceStateLabelKey]
	if oldState == state {
		// skipping
		return nil
	}
	if space.Labels == nil {
		space.Labels = map[string]string{}
	}
	space.Labels[toolchainv1alpha1.SpaceStateLabelKey] = state
	if err := r.Client.Update(ctx, space); err != nil {
		return r.setStatusProvisioningFailed(ctx, space, errs.Wrapf(err,
			"unable to update state label at Space resource"))
	}

	return nil
}

func (r *Reconciler) setStatusProvisioned(ctx context.Context, space *toolchainv1alpha1.Space) error {
	return r.updateStatus(
		ctx,
		space,
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.ConditionReady,
			Status: corev1.ConditionTrue,
			Reason: toolchainv1alpha1.SpaceProvisionedReason,
		})
}

func (r *Reconciler) setStatusProvisioning(ctx context.Context, space *toolchainv1alpha1.Space) error {
	return r.updateStatus(
		ctx,
		space,
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.ConditionReady,
			Status: corev1.ConditionFalse,
			Reason: toolchainv1alpha1.SpaceProvisioningReason,
		})
}

func (r *Reconciler) setStatusProvisioningPending(ctx context.Context, space *toolchainv1alpha1.Space, cause string) error {
	if err := r.updateStatus(
		ctx,
		space,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.ConditionReady,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.SpaceProvisioningPendingReason,
			Message: cause, // the `cause` is just a message
		}); err != nil {
		return err
	}
	// this is a valid state, so we do not return an error
	return nil
}

func (r *Reconciler) setStatusProvisioningFailed(ctx context.Context, space *toolchainv1alpha1.Space, cause error) error {
	if err := r.updateStatus(
		ctx,
		space,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.ConditionReady,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.SpaceProvisioningFailedReason,
			Message: cause.Error(),
		}); err != nil {
		log.FromContext(ctx).Error(cause, "unable to provision Space")
		return err
	}
	return cause
}

func (r *Reconciler) setStatusUpdating(ctx context.Context, space *toolchainv1alpha1.Space) error {
	return r.updateStatus(
		ctx,
		space,
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.ConditionReady,
			Status: corev1.ConditionFalse,
			Reason: toolchainv1alpha1.SpaceUpdatingReason,
		})
}

func (r *Reconciler) setStatusRetargeting(ctx context.Context, space *toolchainv1alpha1.Space) error {
	return r.updateStatus(
		ctx,
		space,
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.ConditionReady,
			Status: corev1.ConditionFalse,
			Reason: toolchainv1alpha1.SpaceRetargetingReason,
		})
}

func (r *Reconciler) setStatusRetargetFailed(ctx context.Context, space *toolchainv1alpha1.Space, cause error) error {
	if err := r.updateStatus(
		ctx,
		space,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.ConditionReady,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.SpaceRetargetingFailedReason,
			Message: cause.Error(),
		}); err != nil {
		log.FromContext(ctx).Error(cause, "unable to retarget Space")
		return err
	}
	return cause
}

func (r *Reconciler) setStatusTerminating(ctx context.Context, space *toolchainv1alpha1.Space) error {
	return r.updateStatus(
		ctx,
		space,
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.ConditionReady,
			Status: corev1.ConditionFalse,
			Reason: toolchainv1alpha1.SpaceTerminatingReason,
		})
}

func (r *Reconciler) setStatusTerminatingFailed(ctx context.Context, space *toolchainv1alpha1.Space, cause error) error {
	if err := r.updateStatus(
		ctx,
		space,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.ConditionReady,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.SpaceTerminatingFailedReason,
			Message: cause.Error(),
		}); err != nil {
		log.FromContext(ctx).Error(cause, "unable to terminate Space")
		return err
	}
	return cause
}

func (r *Reconciler) setStatusNSTemplateSetCreationFailed(ctx context.Context, space *toolchainv1alpha1.Space, cause error) error {
	if err := r.updateStatus(
		ctx,
		space,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.ConditionReady,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.SpaceUnableToCreateNSTemplateSetReason,
			Message: cause.Error(),
		}); err != nil {
		log.FromContext(ctx).Error(cause, "unable to create NSTemplateSet")
		return err
	}
	return cause
}

func (r *Reconciler) setStatusNSTemplateSetUpdateFailed(ctx context.Context, space *toolchainv1alpha1.Space, cause error) error {
	if err := r.updateStatus(
		ctx,
		space,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.ConditionReady,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.SpaceUnableToUpdateNSTemplateSetReason,
			Message: cause.Error(),
		}); err != nil {
		log.FromContext(ctx).Error(cause, "unable to create NSTemplateSet")
		return err
	}
	return cause
}

// setStatusProvisionedNamespaces updates the provisioned namespace list status of the space if something changed.
func (r *Reconciler) setStatusProvisionedNamespaces(ctx context.Context, space *toolchainv1alpha1.Space, provisionedNamespaces []toolchainv1alpha1.SpaceNamespace) error {
	if reflect.DeepEqual(space.Status.ProvisionedNamespaces, provisionedNamespaces) {
		// nothing changed
		return nil
	}

	// update provisioned namespace list with the one from the NSTemplateSet
	space.Status.ProvisionedNamespaces = provisionedNamespaces
	return r.Client.Status().Update(ctx, space)
}

// updateStatus updates space status conditions with the new conditions
func (r *Reconciler) updateStatus(ctx context.Context, space *toolchainv1alpha1.Space, conditions ...toolchainv1alpha1.Condition) error {
	var updated bool
	space.Status.Conditions, updated = condition.AddOrUpdateStatusConditions(space.Status.Conditions, conditions...)
	if !updated {
		// Nothing changed
		return nil
	}
	return r.Client.Status().Update(ctx, space)
}
