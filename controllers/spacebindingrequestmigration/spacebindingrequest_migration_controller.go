package spacebindingrequestmigration

import (
	"context"
	"fmt"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/cluster"
	errs "github.com/pkg/errors"
	"github.com/redhat-cop/operator-utils/pkg/util"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// Reconciler reconciles a SpaceBindingRequestMigration object
type Reconciler struct {
	Client         runtimeclient.Client
	Scheme         *runtime.Scheme
	Namespace      string
	MemberClusters map[string]cluster.Cluster
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager, memberClusters map[string]cluster.Cluster) error {
	// Watch SpaceBindings from host cluster.
	b := ctrl.NewControllerManagedBy(mgr).
		For(&toolchainv1alpha1.SpaceBinding{})

	// Watch SpaceBindingRequests in all member clusters and all namespaces and map those to their respective SpaceBinding resources.
	for _, memberCluster := range memberClusters {
		b = b.Watches(
			source.NewKindWithCache(&toolchainv1alpha1.SpaceBindingRequest{}, memberCluster.Cache),
			handler.EnqueueRequestsFromMapFunc(MapSpaceBindingRequestToSpaceBinding(r.Client, r.Namespace)))
	}
	return b.Complete(r)
}

//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=spacebindingrequests,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=spacebindingrequests/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=spacebindingrequests/finalizers,verbs=update

// Reconcile converts all the SpaceBindings created using the sandbox-cli to SpaceBindingRequests
func (r *Reconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("reconciling SpaceBindingRequestMigration")

	// Fetch the SpaceBinding instance
	spaceBinding := &toolchainv1alpha1.SpaceBinding{}
	err := r.Client.Get(ctx, request.NamespacedName, spaceBinding)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, errs.Wrapf(err, "unable to get spacebinding")
	}
	if util.IsBeingDeleted(spaceBinding) {
		logger.Info("the SpaceBinding is already being deleted")
		return reconcile.Result{}, nil
	}
	// check if spaceBinding was created from SpaceBindingRequest,
	// in that case we can skip it
	if hasSpaceBindingRequest(spaceBinding) {
		return reconcile.Result{}, nil
	}

	spaceName := types.NamespacedName{Namespace: spaceBinding.Namespace, Name: spaceBinding.Spec.Space}
	space := &toolchainv1alpha1.Space{}
	if err := r.Client.Get(ctx, spaceName, space); err != nil {
		if errors.IsNotFound(err) {
			// space was deleted
			return reconcile.Result{}, nil
		}
		// error while reading space
		return ctrl.Result{}, errs.Wrapf(err, "unable to get the bound Space")
	}

	murName := types.NamespacedName{Namespace: spaceBinding.Namespace, Name: spaceBinding.Spec.MasterUserRecord}
	mur := &toolchainv1alpha1.MasterUserRecord{}
	if err := r.Client.Get(ctx, murName, mur); err != nil {
		if errors.IsNotFound(err) {
			// mur was deleted
			return reconcile.Result{}, nil
		}
		// error while reading MUR
		return ctrl.Result{}, errs.Wrapf(err, "unable to get the bound MUR")
	}

	// error when mur has no owner label (should not happen in prod)
	if _, ok := mur.Labels[toolchainv1alpha1.MasterUserRecordOwnerLabelKey]; !ok {
		return ctrl.Result{}, errs.New("mur has no MasterUserRecordOwnerLabelKey set")
	}
	// error when space has no creator label (should not happen in prod)
	if _, ok := space.Labels[toolchainv1alpha1.SpaceCreatorLabelKey]; !ok {
		return ctrl.Result{}, errs.New("space has no SpaceCreatorLabelKey set")
	}

	// skip workspace creator spacebinding
	// the controller will convert only spacebindings created by system admins using the sandbox-cli.
	// If the creator label on the space matches the  owner label on the MUR then this is the owner of the space
	// and the spacebinding should not be migrated.
	if space.Labels[toolchainv1alpha1.SpaceCreatorLabelKey] == mur.Labels[toolchainv1alpha1.MasterUserRecordOwnerLabelKey] {
		return reconcile.Result{}, nil
	}

	// get the spaceRole
	spaceRole := spaceBinding.Spec.SpaceRole

	// get member cluster name where the space was provisioned
	targetCluster := space.Spec.TargetCluster
	memberCluster, memberClusterFound := r.MemberClusters[targetCluster]
	if !memberClusterFound {
		return ctrl.Result{}, errs.New(fmt.Sprintf("unable to find member cluster: %s", targetCluster))
	}

	// get the home namespace from space
	defaultNamespace := getDefaultNamespace(space.Status.ProvisionedNamespaces)

	// construct a SpaceBindingRequest object
	sbrName := mur.GetName() + "-" + spaceRole
	sbr := &toolchainv1alpha1.SpaceBindingRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sbrName,
			Namespace: defaultNamespace,
		},
	}

	result, err := controllerutil.CreateOrUpdate(ctx, memberCluster.Client, sbr, func() error {
		sbr.Spec = toolchainv1alpha1.SpaceBindingRequestSpec{
			MasterUserRecord: mur.GetName(),
			SpaceRole:        spaceRole,
		}
		return nil
	})

	if err != nil {
		// something happened when we tried to read or write the sbr
		return ctrl.Result{}, errs.Wrapf(err, "Failed to create or update space binding request %v", sbrName)
	}

	if result == controllerutil.OperationResultCreated {
		// let's requeue after we created the SBR, so that in next loop the migrated SpaceBinding object will be deleted
		return ctrl.Result{Requeue: true}, nil
	}
	// if the SBR was found ( was created from the previous reconcile loop), we can now delete the SpaceBinding object
	if err := r.Client.Delete(ctx, spaceBinding); err != nil && !errors.IsNotFound(err) {
		return ctrl.Result{}, errs.Wrapf(err, "unable to delete the SpaceBinding")
	}

	return ctrl.Result{}, nil
}

func getDefaultNamespace(provisionedNamespaces []toolchainv1alpha1.SpaceNamespace) string {
	for _, namespaceObj := range provisionedNamespaces {
		if namespaceObj.Type == "default" {
			return namespaceObj.Name
		}
	}
	return ""
}

func hasSpaceBindingRequest(spaceBinding *toolchainv1alpha1.SpaceBinding) bool {
	_, sbrNameFound := spaceBinding.Labels[toolchainv1alpha1.SpaceBindingRequestLabelKey]
	return sbrNameFound
}
