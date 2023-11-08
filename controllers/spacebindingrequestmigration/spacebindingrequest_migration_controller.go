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
	// since it's mandatory to add a primary resource when creating a new controller,
	// we add the SpaceBindingRequest CR even if there should be no reconciles triggered from the host cluster,
	// only from member clusters (see watches below)
	// SpaceBindingRequest owns spacebindings so events will be triggered for those from the host cluster.
	b := ctrl.NewControllerManagedBy(mgr).
		For(&toolchainv1alpha1.SpaceBinding{})

	// Watch SpaceBindingRequests in all member clusters and all namespaces.
	for _, memberCluster := range memberClusters {
		b = b.Watches(
			source.NewKindWithCache(&toolchainv1alpha1.SpaceBindingRequest{}, memberCluster.Cache),
			handler.EnqueueRequestsFromMapFunc(MapSpaceBindingRequestToSpaceBinding(r.Client)))
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

	// skip workspace creator spacebinding
	// the controller will convert only spacebindings created by system admins using the sandbox-cli.
	// If the creator label on the space matches the usersignup reference on the MUR then this is the owner of the space
	// and the spacebinding should not be migrated.
	if mur.ObjectMeta.OwnerReferences == nil || len(mur.ObjectMeta.OwnerReferences) == 0 {
		return ctrl.Result{}, errs.New("MasterUserRecord has no UserSignup owner reference")
	}
	// skip spaces with no creator label
	if _, ok := space.Labels[toolchainv1alpha1.SpaceCreatorLabelKey]; !ok {
		// let's log an error, since this should not happen in production
		logger.Error(errs.New("space has no SpaceCreatorLabelKey set"), "the spacebindings for this space will not be migrated", "space name", space.Name)
		return ctrl.Result{}, nil
	}
	usersignup := mur.ObjectMeta.OwnerReferences[0]
	if space.Labels[toolchainv1alpha1.SpaceCreatorLabelKey] == usersignup.Name {
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

	// create the SpaceBindingRequest resource
	sbrName := mur.GetName() + "-" + spaceRole
	sbr := &toolchainv1alpha1.SpaceBindingRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sbrName,
			Namespace: defaultNamespace,
			Labels: map[string]string{
				toolchainv1alpha1.SpaceCreatorLabelKey:                 "spacebindingrequestmigration",
				toolchainv1alpha1.SpaceBindingSpaceLabelKey:            space.GetName(),
				toolchainv1alpha1.SpaceBindingMasterUserRecordLabelKey: mur.GetName(),
			},
		},
		Spec: toolchainv1alpha1.SpaceBindingRequestSpec{
			MasterUserRecord: mur.GetName(),
			SpaceRole:        spaceRole,
		},
	}

	// check if SpaceBindingRequest doesn't exist already
	err = memberCluster.Client.Get(context.TODO(), types.NamespacedName{
		Namespace: defaultNamespace,
		Name:      sbrName,
	}, &toolchainv1alpha1.SpaceBindingRequest{})
	if err != nil {
		if errors.IsNotFound(err) {
			// create the sbr resource
			if err := memberCluster.Client.Create(ctx, sbr); err != nil {
				return ctrl.Result{}, errs.Wrapf(err, "unable to create SpaceBindingRequest")
			}

			// delete the SpaceBinding
			if err := r.Client.Delete(ctx, spaceBinding); err != nil {
				return ctrl.Result{}, errs.Wrapf(err, "unable to delete the SpaceBinding")
			}
			return ctrl.Result{}, nil
		}
		// Error reading the object
		return ctrl.Result{}, err
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
