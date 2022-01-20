package spacebindingcleanup

import (
	"context"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/go-logr/logr"
	errs "github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	onlyForDeletion := builder.WithPredicates(OnlyDeleteAndGenericPredicate{})
	return ctrl.NewControllerManagedBy(mgr).
		For(&toolchainv1alpha1.SpaceBinding{}).
		Watches(&source.Kind{Type: &toolchainv1alpha1.Space{}},
			handler.EnqueueRequestsFromMapFunc(MapToSpaceBindingByBoundObjectName(r.Client, toolchainv1alpha1.SpaceBindingSpaceLabelKey)),
			onlyForDeletion).
		Watches(&source.Kind{Type: &toolchainv1alpha1.MasterUserRecord{}},
			handler.EnqueueRequestsFromMapFunc(MapToSpaceBindingByBoundObjectName(r.Client, toolchainv1alpha1.SpaceBindingMasterUserRecordLabelKey)),
			onlyForDeletion).
		Complete(r)
}

// Reconciler reconciles a SpaceBinding object
type Reconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	Namespace string
}

//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=spacebindings,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=spacebindings/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=spacebindings/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *Reconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithName("cleanup")
	logger.Info("Reconciling SpaceBinding")

	// Fetch the UserSignup instance
	spaceBinding := &toolchainv1alpha1.SpaceBinding{}
	err := r.Client.Get(context.TODO(), request.NamespacedName, spaceBinding)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	spaceName := types.NamespacedName{Namespace: spaceBinding.Namespace, Name: spaceBinding.Spec.Space}
	space := &toolchainv1alpha1.Space{}
	if err := r.Client.Get(context.TODO(), spaceName, space); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("the Space was not found", "Space", spaceBinding.Spec.Space)
			return ctrl.Result{}, r.deleteSpaceBinding(logger, spaceBinding)

		}
		return ctrl.Result{}, errs.Wrapf(err, "unable to get the bound Space")
	}

	murName := types.NamespacedName{Namespace: spaceBinding.Namespace, Name: spaceBinding.Spec.MasterUserRecord}
	mur := &toolchainv1alpha1.MasterUserRecord{}
	if err := r.Client.Get(context.TODO(), murName, mur); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("the MUR was not found", "MasterUserRecord", spaceBinding.Spec.MasterUserRecord)
			return ctrl.Result{}, r.deleteSpaceBinding(logger, spaceBinding)
		}
		return ctrl.Result{}, errs.Wrapf(err, "unable to get the bound MUR")
	}

	return ctrl.Result{}, nil
}

func (r *Reconciler) deleteSpaceBinding(logger logr.Logger, spaceBinding *toolchainv1alpha1.SpaceBinding) error {
	logger.Info("deleting the SpaceBinding")
	if err := r.Delete(context.TODO(), spaceBinding); err != nil {
		return errs.Wrapf(err, "unable to delete the SpaceBinding")
	}
	return nil
}
