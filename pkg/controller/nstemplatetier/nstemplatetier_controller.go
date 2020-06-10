package nstemplatetier

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"reflect"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/configuration"
	"github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/selection"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_nstemplatetier")

const (
	// maxPoolSize the maximum number of TemplateUpdateRequest resources that can exist at the same time
	maxPoolSize = 5
)

// Add creates a new NSTemplateTier Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, config *configuration.Config) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileNSTemplateTier{client: mgr.GetClient(), scheme: mgr.GetScheme()}
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
	})
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
	client                 client.Client
	scheme                 *runtime.Scheme
	retrieveMemberClusters cluster.GetMemberClustersFunc
}

// Reconcile reads that state of the cluster for a NSTemplateTier object and makes changes based on the state read
// and what is in the NSTemplateTier.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileNSTemplateTier) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	logger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	logger.Info("Reconciling NSTemplateTier")

	// Fetch the NSTemplateTier instance
	instance := toolchainv1alpha1.NSTemplateTier{}
	err := r.client.Get(context.TODO(), request.NamespacedName, &instance)
	if err != nil {
		if errors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		logger.Error(err, "unable to get the current NSTemplateTier instance")
		return reconcile.Result{}, err
	}

	activeTemplateUpdateRequests, err := r.activeTemplateUpdateRequests(instance)
	if err != nil {
		logger.Error(err, "unable to get active TemplateUpdateRequests")
		return reconcile.Result{}, err
	}

	if activeTemplateUpdateRequests < maxPoolSize {
		members := r.retrieveMemberClusters()
	loop:
		for _, member := range members {
			// create a TemplateUpdateRequest if active count < max pol size
			// find a MasterUserRecord which is not already up-to-date
			// and for which there is no TemplateUpdateRequest yet
			// fetch by subsets of "maxPoolSize + 1" size until a MasterUserRecord candidate is found
			murs := toolchainv1alpha1.MasterUserRecordList{}
			matchingLabels, err := newLabelSelector(instance, *member)
			if err != nil {
				logger.Error(err, "unable to get MasterUserRecords to update")
				return reconcile.Result{}, err
			}
			r.client.List(context.Background(), &murs,
				client.InNamespace(instance.Namespace),
				client.Limit(maxPoolSize+1),
				matchingLabels,
			)
			logger.Info("listed MasterUserRecords", "count", len(murs.Items), "cluster", member.Name)
			for _, mur := range murs.Items {
				for _, account := range mur.Spec.UserAccounts {
					accountTmpls := account.Spec.NSTemplateSet
					logger.Info("checking mur account", "name", mur.Name, "tier", accountTmpls.TierName)
					if accountTmpls.TierName != instance.Name {
						// skip if the user account is not based on the current Tier
						continue
					}
					// compare all TemplateRefs in the current MUR.UserAccount vs NSTemplateTier instance
					if !reflect.DeepEqual(templateRefs(accountTmpls), templateRefs(instance.Spec)) {
						logger.Info("creating a TemplateUpdateRequest to update the MasterUserRecord", "name", mur.Name, "tier", instance.Name)
						r.client.Create(context.TODO(), &toolchainv1alpha1.TemplateUpdateRequest{
							ObjectMeta: metav1.ObjectMeta{
								Name: mur.Name,
								Labels: map[string]string{
									toolchainv1alpha1.NSTemplateTierNameLabelKey: instance.Name,
								},
							},
							Spec: toolchainv1alpha1.TemplateUpdateRequestSpec{
								TierName:           instance.Name,
								NSTemplateTierSpec: instance.Spec,
							},
						})
						break loop // create a single TemplateUpdateRequest resource per reconcile loop
					}
				}
			}
		}
	}
	logger.Info("done with creating TemplateUpdateRequest resources after update of NSTemplateTier", "tier", instance.Name)
	return reconcile.Result{}, nil
}

func newLabelSelector(instance toolchainv1alpha1.NSTemplateTier, member cluster.FedCluster) (client.MatchingLabelsSelector, error) {
	// compute the hash of the `.spec.namespaces[].templateRef` + `.spec.clusteResource.TemplateRef`
	hash, err := computeTemplateRefsHash(instance)
	if err != nil {
		return client.MatchingLabelsSelector{}, err
	}
	selector := labels.NewSelector()
	tierLabel, err := labels.NewRequirement(toolchainv1alpha1.LabelKeyPrefix+member.Name+"-templates-tier", selection.Equals, []string{instance.Name})
	selector = selector.Add(*tierLabel)
	templateHashLabel, err := labels.NewRequirement(toolchainv1alpha1.LabelKeyPrefix+member.Name+"-templates-tier-hash", selection.NotEquals, []string{hash})
	selector = selector.Add(*templateHashLabel)
	return client.MatchingLabelsSelector{
		Selector: selector,
	}, nil
}

func (r *ReconcileNSTemplateTier) activeTemplateUpdateRequests(instance toolchainv1alpha1.NSTemplateTier) (int, error) {
	// fetch the list of TemplateUpdateRequest owned by the NSTemplateTier instance
	templateUpdateRequests := toolchainv1alpha1.TemplateUpdateRequestList{}
	if err := r.client.List(context.Background(), &templateUpdateRequests, client.MatchingLabels{
		toolchainv1alpha1.NSTemplateTierNameLabelKey: instance.Name,
	}); err != nil {
		return -1, err
	}

	// count non-deleted templateUpdateRequest items
	activeTemplateUpdateRequests := 0
	for _, r := range templateUpdateRequests.Items {
		if r.DeletionTimestamp == nil {
			activeTemplateUpdateRequests++
		}
	}
	return activeTemplateUpdateRequests, nil
}

// compute the hash of the `.spec.namespaces[].templateRef` + `.spec.clusteResource.TemplateRef`
func computeTemplateRefsHash(instance toolchainv1alpha1.NSTemplateTier) (string, error) {
	spec, err := json.Marshal(instance.Spec)
	if err != nil {
		return "", err
	}
	md5hash := md5.New()
	// Ignore the error, as this implementation cannot return one
	md5hash.Write([]byte(spec))
	return hex.EncodeToString(md5hash.Sum(nil)), nil
}

func templateRefs(obj interface{}) []string {
	switch obj := obj.(type) {
	case toolchainv1alpha1.NSTemplateSetSpec:
		templateRefs := []string{}
		for _, ns := range obj.Namespaces {
			templateRefs = append(templateRefs, ns.TemplateRef)
		}
		if obj.ClusterResources != nil {
			templateRefs = append(templateRefs, obj.ClusterResources.TemplateRef)
		}
		return templateRefs
	case toolchainv1alpha1.NSTemplateTierSpec:
		templateRefs := []string{}
		for _, ns := range obj.Namespaces {
			templateRefs = append(templateRefs, ns.TemplateRef)
		}
		if obj.ClusterResources != nil {
			templateRefs = append(templateRefs, obj.ClusterResources.TemplateRef)
		}
		return templateRefs
	default:
		return []string{}
	}
}
