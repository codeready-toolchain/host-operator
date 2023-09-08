package spacecleanup

import (
	"context"
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	commoncontrollers "github.com/codeready-toolchain/toolchain-common/controllers"

	"github.com/go-logr/logr"
	errs "github.com/pkg/errors"
	"github.com/redhat-cop/operator-utils/pkg/util"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// Reconciler reconciles a Space object
type Reconciler struct {
	Client    runtimeclient.Client
	Namespace string
}

// SetupWithManager sets up the controller reconciler with the Manager
// Watches the Space and SpaceBinding resources
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named("spacecleanup").
		For(&toolchainv1alpha1.Space{}).
		Watches(
			&source.Kind{Type: &toolchainv1alpha1.SpaceBinding{}},
			handler.EnqueueRequestsFromMapFunc(commoncontrollers.MapToOwnerByLabel(r.Namespace, toolchainv1alpha1.SpaceBindingSpaceLabelKey))).
		Complete(r)
}

const deletionTimeThreshold = 30 * time.Second

//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=spaces,verbs=get;list;watch;delete
//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=spacebindings,verbs=get;list;watch

// Reconcile ensures that Space which doesn't have any SpaceBinding is deleted
func (r *Reconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
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

	// if is already being deleted, then skip it
	if util.IsBeingDeleted(space) {
		logger.Info("Space is already being deleted - skipping...")
		return reconcile.Result{}, nil
	}

	requeue, requeueAfter, err := r.ensureDeletionIfNeeded(logger, space)

	return ctrl.Result{
		Requeue:      requeue,
		RequeueAfter: requeueAfter,
	}, err
}

func (r *Reconciler) ensureDeletionIfNeeded(logger logr.Logger, space *toolchainv1alpha1.Space) (bool, time.Duration, error) {
	bindings := &toolchainv1alpha1.SpaceBindingList{}
	labelMatch := runtimeclient.MatchingLabels{toolchainv1alpha1.SpaceBindingSpaceLabelKey: space.Name}
	if err := r.Client.List(context.TODO(), bindings, runtimeclient.InNamespace(space.Namespace), labelMatch); err != nil {
		return false, 0, errs.Wrap(err, "unable to list SpaceBindings")
	}

	if len(bindings.Items) > 0 {
		logger.Info("Space has SpaceBindings - skipping...", "number-of-spacebindings", len(bindings.Items))
		return false, 0, nil
	}

	// check if space has a parentSpace
	// in this case the deletion will be handled by the SR controller
	if found := r.hasParentSpaceSpec(space); found {
		// do not delete this space since has a parent-space set
		return false, 0, nil
	}

	timeSinceCreation := time.Since(space.GetCreationTimestamp().Time)
	if timeSinceCreation > deletionTimeThreshold {
		if err := r.Client.Delete(context.TODO(), space); err != nil {
			return false, 0, errs.Wrap(err, "unable to delete Space")
		}

		logger.Info("Space has been deleted")
		return false, 0, nil
	}

	requeueAfter := deletionTimeThreshold - timeSinceCreation
	logger.Info("Space is not ready for deletion yet", "requeue-after", requeueAfter, "created", space.CreationTimestamp)

	return true, requeueAfter, nil
}

// hasParentSpaceSpec verifies if there .spec.ParentSpace field is set in the current Space.
// return true if .spec.ParentSpace is set
// return false if .spec.ParentSpace is not set
func (r *Reconciler) hasParentSpaceSpec(space *toolchainv1alpha1.Space) bool {
	return space.Spec.ParentSpace != ""
}
