package spacebindingrequest

import (
	"context"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/cluster"
	errs "github.com/pkg/errors"
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
	var memberClusterWithSpaceBindingRequest cluster.Cluster
	var err error
	for _, memberCluster := range r.MemberClusters {
		err = memberCluster.Client.Get(context.TODO(), types.NamespacedName{
			Namespace: request.Namespace,
			Name:      request.Name,
		}, spaceBindingRequest)
		if err != nil {
			if !errors.IsNotFound(err) {
				// Error reading the object - requeue the request.
				return reconcile.Result{}, errs.Wrap(err, "unable to get the current SpaceBindingRequest")
			}

			//  spacebindingrequest CR not found on current membercluster
			continue
		}

		// save the member cluster on which the SpaceBindingRequest CR was found
		memberClusterWithSpaceBindingRequest = memberCluster
		break // exit once found
	}
	// if we exited with a notFound error
	// it means that we couldn't find the spacebindingrequest object on any of the given member clusters
	if err != nil && errors.IsNotFound(err) {
		// let's just log the info
		logger.Info("unable to find SpaceBindingRequest")
		return reconcile.Result{}, nil
	}
	logger.Info("spacebindingrequest found", "member cluster", memberClusterWithSpaceBindingRequest.Name)

	// todo implement logic

	return ctrl.Result{}, err
}
