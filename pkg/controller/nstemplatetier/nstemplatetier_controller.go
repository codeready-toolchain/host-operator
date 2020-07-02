package nstemplatetier

import (
	"context"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/configuration"
	"github.com/codeready-toolchain/toolchain-common/pkg/condition"
	"github.com/redhat-cop/operator-utils/pkg/util"

	"github.com/go-logr/logr"
	errs "github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	controllerutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_nstemplatetier")

const (
	// MaxPoolSize the maximum number of TemplateUpdateRequest resources that can exist at the same time
	MaxPoolSize = 5
)

// Add creates a new NSTemplateTier Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, config *configuration.Config) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileNSTemplateTier{
		client: mgr.GetClient(),
		scheme: mgr.GetScheme(),
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("nstemplatetier-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource NSTemplateTier
	err = c.Watch(&source.Kind{Type: &toolchainv1alpha1.NSTemplateTier{}}, &handler.EnqueueRequestForObject{}, predicate.GenerationChangedPredicate{})
	if err != nil {
		return err
	}

	// Watch for changes to secondary resource TemplateUpdateRequest and requeue the owner NSTemplateTier
	err = c.Watch(&source.Kind{Type: &toolchainv1alpha1.TemplateUpdateRequest{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &toolchainv1alpha1.NSTemplateTier{},
	}) // we need to track changes in the status, so we can't use the `GenerationChangedPredicate`
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileNSTemplateTier implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileNSTemplateTier{}

// ReconcileNSTemplateTier reconciles a NSTemplateTier object
type ReconcileNSTemplateTier struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a NSTemplateTier object and makes changes based on the state read
// and what is in the NSTemplateTier.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileNSTemplateTier) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	logger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	logger.Info("Reconciling NSTemplateTier")

	// Fetch the NSTemplateTier tier
	tier := &toolchainv1alpha1.NSTemplateTier{}
	err := r.client.Get(context.TODO(), request.NamespacedName, tier)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("NSTemplateTier not found")
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		logger.Error(err, "unable to get the current NSTemplateTier tier")
		return reconcile.Result{}, err
	}
	if err := r.ensureTemplateUpdateRequest(logger, tier); err != nil {
		logger.Error(err, "unable to ensure TemplateRequestUpdate resource after NSTemplateTier changed.")
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, nil
}

// ensureTemplateUpdateRequest ensures that all relared MasterUserRecords are up-to-date with the NSTemplateTier that changed.
// If not, then it creates a TemplateUpdateRequest resource for the first MasterUserRecord not up-to-date with the tier, and
// returns `nil` so the controller will wait for the next reconcile loop to create subsequent TemplateUpdateRequest resources,
// until the `MaxPoolSize` threashold is reached.
func (r *ReconcileNSTemplateTier) ensureTemplateUpdateRequest(logger logr.Logger, tier *toolchainv1alpha1.NSTemplateTier) error {
	if activeTemplateUpdateRequests, deleted, err := r.activeTemplateUpdateRequests(logger, tier); err != nil {
		return errs.Wrap(err, "unable to get active TemplateUpdateRequests")
	} else if deleted {
		// skip TemplateUpdateRequest creation in this reconcile loop since one was deleted
		return nil
	} else if activeTemplateUpdateRequests < MaxPoolSize {
		// create a TemplateUpdateRequest if active count < MaxPoolSize,
		// ie, find a MasterUserRecord which is not already up-to-date
		// and for which there is no TemplateUpdateRequest yet

		// fetch by subsets of "MaxPoolSize + 1" size until a MasterUserRecord candidate is found
		murs := toolchainv1alpha1.MasterUserRecordList{}
		matchingLabels, err := murSelector(tier)
		if err != nil {
			return errs.Wrap(err, "unable to get MasterUserRecords to update")
		}
		err = r.client.List(context.Background(), &murs,
			client.InNamespace(tier.Namespace),
			client.Limit(MaxPoolSize+1),
			matchingLabels,
		)
		if err != nil {
			return errs.Wrap(err, "unable to get MasterUserRecords to update")
		}
		logger.Info("listed MasterUserRecords", "count", len(murs.Items), "selector", matchingLabels)
		for _, mur := range murs.Items {
			// check if there's already a TemplateUpdateRequest for this MasterUserRecord
			templateUpdateRequest := toolchainv1alpha1.TemplateUpdateRequest{}
			if err := r.client.Get(context.TODO(), types.NamespacedName{
				Namespace: tier.Namespace,
				Name:      mur.Name,
			}, &templateUpdateRequest); err == nil {
				logger.Info("MasterUserRecord already has an associated TemplateUpdateRequest", "name", mur.Name)
				continue
			} else if !errors.IsNotFound(err) {
				return errs.Wrapf(err, "unable to get TemplateUpdateRequest for MasterUserRecord '%s'", mur.Name)
			}
			logger.Info("creating a TemplateUpdateRequest to update the MasterUserRecord", "name", mur.Name, "tier", tier.Name)
			tur := &toolchainv1alpha1.TemplateUpdateRequest{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: tier.Namespace,
					Name:      mur.Name,
					Labels: map[string]string{
						toolchainv1alpha1.NSTemplateTierNameLabelKey: tier.Name,
					},
				},
				Spec: toolchainv1alpha1.TemplateUpdateRequestSpec{
					TierName:         tier.Name,
					Namespaces:       tier.Spec.Namespaces,
					ClusterResources: tier.Spec.ClusterResources,
				},
			}
			if err = controllerutil.SetControllerReference(tier, tur, r.scheme); err != nil {
				return err
			}
			// the controller creates a single TemplateUpdateRequest resource per reconcile loop,
			// and the creation of this TemplateUpdateRequest will trigger another reconcile loop
			// since the controller watches TemplateUpdateRequests owned by the NSTemplateTier
			return r.client.Create(context.TODO(), tur)
		}
	}
	logger.Info("done with creating TemplateUpdateRequest resources after update of NSTemplateTier", "tier", tier.Name)
	return nil
}
func (r *ReconcileNSTemplateTier) activeTemplateUpdateRequests(logger logr.Logger, tier *toolchainv1alpha1.NSTemplateTier) (int, bool, error) {
	// fetch the list of TemplateUpdateRequest owned by the NSTemplateTier tier
	templateUpdateRequests := toolchainv1alpha1.TemplateUpdateRequestList{}
	if err := r.client.List(context.TODO(), &templateUpdateRequests, client.MatchingLabels{
		toolchainv1alpha1.NSTemplateTierNameLabelKey: tier.Name,
	}); err != nil {
		return -1, false, err
	}

	// count non-deleted templateUpdateRequest items
	count := 0
	for _, tur := range templateUpdateRequests.Items {
		if util.IsBeingDeleted(&tur) {
			// ignore when already being deleted
			continue
		}
		if condition.IsTrue(tur.Status.Conditions, toolchainv1alpha1.TemplateUpdateRequestComplete) ||
			condition.IsFalseWithReason(tur.Status.Conditions, toolchainv1alpha1.TemplateUpdateRequestComplete, toolchainv1alpha1.TemplateUpdateRequestUnableToUpdateReason) {
			// delete when in `complete=true` (reason=updated) or when in `complete=false/reason=failed` status conditions
			if err := r.client.Delete(context.TODO(), &tur); err != nil {
				return -1, false, errs.Wrapf(err, "unable to delete the TemplateUpdateRequest resource '%s'", tur.Name)
			}
			log.Info("deleted TemplateUpdateRequest", "name", tur.Name)
			return -1, true, nil
		}
		count++
	}
	logger.Info("found active TemplateUpdateRequest for the current tier", "count", count)
	return count, false, nil
}
