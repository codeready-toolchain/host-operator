package spacebindingcleanup

import (
	"context"
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/controllers/toolchainconfig"

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
)

const norequeue = 0 * time.Second
const requeueDelay = 10 * time.Second
const deletionDelay = 2 * time.Second

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	onlyForDeletion := builder.WithPredicates(OnlyDeleteAndGenericPredicate{})
	return ctrl.NewControllerManagedBy(mgr).
		For(&toolchainv1alpha1.SpaceBinding{}).
		Watches(&toolchainv1alpha1.Space{},
			handler.EnqueueRequestsFromMapFunc(MapToSpaceBindingByBoundObjectName(r.Client, toolchainv1alpha1.SpaceBindingSpaceLabelKey)),
			onlyForDeletion).
		Watches(&toolchainv1alpha1.MasterUserRecord{},
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
	err := r.Get(ctx, request.NamespacedName, spaceBinding)
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

	// ensure the space exists
	spaceName := types.NamespacedName{Namespace: spaceBinding.Namespace, Name: spaceBinding.Spec.Space}
	space := &toolchainv1alpha1.Space{}
	if err := r.Get(ctx, spaceName, space); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("Space not found", "space", spaceBinding.Spec.Space)
			requeueAfter, err := r.deleteSpaceBinding(ctx, spaceBinding)
			return ctrl.Result{RequeueAfter: requeueAfter}, err
		}
		// error while reading space
		return ctrl.Result{}, errs.Wrapf(err, "unable to get the bound Space")
	}

	// ensure the MUR exists
	requeueAfter, err := r.ensureMURExists(ctx, spaceBinding)
	return ctrl.Result{RequeueAfter: requeueAfter}, err
}

func (r *Reconciler) ensureMURExists(ctx context.Context, spaceBinding *toolchainv1alpha1.SpaceBinding) (time.Duration, error) {
	logger := log.FromContext(ctx).WithName("cleanup")

	// if publicViewer is enabled and the SpaceBinding is related to the PublicViewer,
	// do not check for MUR existence
	cfg, err := toolchainconfig.GetToolchainConfig(r.Client)
	if err != nil {
		return norequeue, errs.Wrapf(err, "unable to get toolchainconfig")
	}
	if cfg.PublicViewer().Enabled() && spaceBinding.Spec.MasterUserRecord == toolchainv1alpha1.KubesawAuthenticatedUsername {
		return norequeue, nil
	}

	// ensure that MUR exists: if it does not exist, then delete the SpaceBinding
	murName := types.NamespacedName{Namespace: spaceBinding.Namespace, Name: spaceBinding.Spec.MasterUserRecord}
	mur := &toolchainv1alpha1.MasterUserRecord{}
	if err := r.Get(ctx, murName, mur); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("the MUR was not found", "MasterUserRecord", spaceBinding.Spec.MasterUserRecord)
			return r.deleteSpaceBinding(ctx, spaceBinding)
		}
		// error while reading MUR
		return norequeue, errs.Wrapf(err, "unable to get the bound MUR")
	}

	return norequeue, nil
}

func (r *Reconciler) deleteSpaceBinding(ctx context.Context, spaceBinding *toolchainv1alpha1.SpaceBinding) (time.Duration, error) {
	logger := log.FromContext(ctx)

	// Check deletion delay - only proceed if SpaceBinding is old enough
	spaceBindingAge := time.Since(spaceBinding.CreationTimestamp.Time)
	if spaceBindingAge < deletionDelay {
		// Calculate how much time is left in the deletion delay period
		timeLeft := deletionDelay - spaceBindingAge
		logger.Info("SpaceBinding too young - waiting", "age", spaceBindingAge.String(), "timeLeft", timeLeft.String())
		return timeLeft, nil
	}

	logger.Info("deleting the SpaceBinding", "age", spaceBindingAge.String())

	// check if spaceBinding was created from SpaceBindingRequest,
	// in that case we must delete the SBR and then the SBR controller will take care of deleting the SpaceBinding
	spaceBindingReqeuestAssociated := checkSpaceBindingRequestAssociated(spaceBinding)
	if spaceBindingReqeuestAssociated.found {
		if err := r.logErrorIfSBRisDisabled(logger, spaceBindingReqeuestAssociated); err != nil {
			return norequeue, err
		}
		return r.deleteSpaceBindingRequest(ctx, spaceBindingReqeuestAssociated)
	}

	// otherwise delete the SpaceBinding resource directly ...
	return norequeue, r.deleteSpaceBindingResource(ctx, spaceBinding)
}

func (r *Reconciler) logErrorIfSBRisDisabled(logger logr.Logger, spaceBindingReqeuestAssociated *SpaceBindingRequestAssociated) error {
	// check if spacebindinrequest functionality is enabled.
	// if not let's log an error since the SBR controller might be stopped,
	// which means that the SBR resource will remain in terminating state
	config, err := toolchainconfig.GetToolchainConfig(r.Client)
	if err != nil {
		return errs.Wrapf(err, "unable to get toolchainconfig")
	}
	if !config.SpaceConfig().SpaceBindingRequestIsEnabled() {
		logger.Error(errs.New("SpaceBindRequest functionality is disabled in toolchainconfig"),
			"spacebinding was created from spacebindingrequest", "spacebinding.name", spaceBindingReqeuestAssociated.spaceBinding.Name)
	}
	return nil
}

func (r *Reconciler) deleteSpaceBindingResource(ctx context.Context, spaceBinding *toolchainv1alpha1.SpaceBinding) error {
	if err := r.Delete(ctx, spaceBinding); err != nil {
		return errs.Wrapf(err, "unable to delete the SpaceBinding")
	}
	return nil
}

func (r *Reconciler) deleteSpaceBindingRequest(ctx context.Context, sbrAssociated *SpaceBindingRequestAssociated) (time.Duration, error) {
	logger := log.FromContext(ctx)

	spaceBindingRequest := &toolchainv1alpha1.SpaceBindingRequest{}
	memberClusterWithSpaceBindingRequest, found, err := cluster.LookupMember(ctx, r.MemberClusters, types.NamespacedName{
		Namespace: sbrAssociated.namespace,
		Name:      sbrAssociated.name,
	}, spaceBindingRequest)
	if err != nil {
		if !found {
			// got error while searching for SpaceBindingRequest CR
			return norequeue, err
		}
		// Just log the error but proceed because we did find the member anyway
		logger.Error(err, "error while searching for SpaceBindingRequest")
	} else if !found {
		// let's just log the info
		logger.Info("unable to find SpaceBindingRequest", "SpaceBindingRequest.Name", sbrAssociated.name, "SpaceBindingRequest.Namespace", sbrAssociated.namespace)
		// try and delete the SpaceBinding since SBR was not found
		return norequeue, r.deleteSpaceBindingResource(ctx, sbrAssociated.spaceBinding)
	}

	// delete the SBR
	if err := memberClusterWithSpaceBindingRequest.Client.Delete(ctx, spaceBindingRequest); err != nil {
		return norequeue, errs.Wrapf(err, "unable to delete the SpaceBindingRequest")
	}
	return requeueDelay, nil
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
