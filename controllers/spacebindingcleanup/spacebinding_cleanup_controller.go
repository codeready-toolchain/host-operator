package spacebindingcleanup

import (
	"context"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"

	"github.com/codeready-toolchain/host-operator/pkg/cluster"
	"github.com/go-logr/logr"
	errs "github.com/pkg/errors"
	"github.com/redhat-cop/operator-utils/pkg/util"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
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
	runtimeclient.Client
	Scheme         *runtime.Scheme
	Namespace      string
	MemberClusters map[string]cluster.Cluster
}

//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=spacebindings,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=spacebindings/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=spacebindings/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *Reconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithName("cleanup")
	logger.Info("Reconciling SpaceBinding")

	// Fetch the SpaceBinding instance
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
	if util.IsBeingDeleted(spaceBinding) {
		logger.Info("the SpaceBinding is already being deleted")
		return reconcile.Result{}, nil
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

	// check if spaceBinding was created from SpaceBindingRequest,
	// in that case we must delete the SBR and then the SBR controller will take care of deleting the SpaceBinding
	spaceBindingReqeuestAssociated := checkSpaceBindingRequestAssociated(spaceBinding)
	if spaceBindingReqeuestAssociated.found {
		return r.deleteSpaceBindingRequest(logger, spaceBindingReqeuestAssociated)
	}

	// otherwise delete the SpaceBinding resource directly ...
	return r.DeleteSpaceBinding(spaceBinding)
}

func (r *Reconciler) DeleteSpaceBinding(spaceBinding *toolchainv1alpha1.SpaceBinding) error {
	if err := r.Delete(context.TODO(), spaceBinding); err != nil {
		return errs.Wrapf(err, "unable to delete the SpaceBinding")
	}
	return nil
}

func (r *Reconciler) deleteSpaceBindingRequest(logger logr.Logger, sbrAssociated *SpaceBindingRequestAssociated) error {
	spaceBindingRequest := &toolchainv1alpha1.SpaceBindingRequest{}
	memberClusterWithSpaceBindingRequest, found, err := cluster.LookupMember(r.MemberClusters, types.NamespacedName{
		Namespace: sbrAssociated.namespace,
		Name:      sbrAssociated.name,
	}, spaceBindingRequest)
	if err != nil {
		if !found {
			// got error while searching for SpaceBindingRequest CR
			return err
		}
		// Just log the error but proceed because we did find the member anyway
		logger.Error(err, "error while searching for SpaceBindingRequest")
	} else if !found {
		// let's just log the info
		logger.Info("unable to find SpaceBindingRequest", "SpaceBindingRequest.Name", sbrAssociated.name, "SpaceBindingRequest.Namespace", sbrAssociated.namespace)
		// try and delete the SpaceBinding since SBR was not found
		return r.DeleteSpaceBinding(sbrAssociated.spaceBinding)
	}

	// delete the SBR
	if err := memberClusterWithSpaceBindingRequest.Client.Delete(context.TODO(), spaceBindingRequest); err != nil {
		return errs.Wrapf(err, "unable to delete the SpaceBindingRequest")
	}
	return nil
}

// SpaceBindingRequestAssociated is a wrapper that holds details regarding SB, and it's SBR if there is any associated.
type SpaceBindingRequestAssociated struct {
	// name and namespace of the SpaceBindingRequest
	name, namespace string
	// found is true if there is a SpaceBindingRequest associated with the SB
	found bool
	// spaceBinding is the resource that is being reconciled
	spaceBinding *toolchainv1alpha1.SpaceBinding
}

func checkSpaceBindingRequestAssociated(spaceBinding *toolchainv1alpha1.SpaceBinding) *SpaceBindingRequestAssociated {
	sbrName, sbrNameFound := spaceBinding.Labels[toolchainv1alpha1.SpaceBindingRequestLabelKey]
	sbrNamespace, sbrNamespaceFound := spaceBinding.Labels[toolchainv1alpha1.SpaceBindingRequestNamespaceLabelKey]
	// if both are found then there is a SBR associated with this spacebinding
	hasSpaceBinding := sbrNamespaceFound && sbrNameFound
	return &SpaceBindingRequestAssociated{
		name:         sbrName,
		namespace:    sbrNamespace,
		found:        hasSpaceBinding,
		spaceBinding: spaceBinding,
	}
}
