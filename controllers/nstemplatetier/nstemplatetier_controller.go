package nstemplatetier

import (
	"context"
	"fmt"
	"reflect"
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/controllers/toolchainconfig"
	"github.com/redhat-cop/operator-utils/pkg/util"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr manager.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&toolchainv1alpha1.NSTemplateTier{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Watches(&toolchainv1alpha1.TierTemplate{},
			handler.EnqueueRequestsFromMapFunc(MapTierTemplateToNSTemplateTier(r.Client))).
		Complete(r)
}

// Reconciler reconciles a NSTemplateTier object (only when this latter's specs were updated)
type Reconciler struct {
	Client runtimeclient.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=nstemplatetiers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=spaces,verbs=get;list;watch
//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=nstemplatetiers/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=nstemplatetiers/finalizers,verbs=update

// Reconcile takes care of fetching the NSTemplateTier
func (r *Reconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// fetch the NSTemplateTier tier
	tier := &toolchainv1alpha1.NSTemplateTier{}
	if err := r.Client.Get(ctx, request.NamespacedName, tier); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("NSTemplateTier not found")
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		logger.Error(err, "unable to get the current NSTemplateTier")
		return reconcile.Result{}, fmt.Errorf("unable to get the current NSTemplateTier: %w", err)
	}
	// if the NSTemplateTier is being deleted, then do nothing
	if util.IsBeingDeleted(tier) {
		return reconcile.Result{}, nil
	}

	_, err := toolchainconfig.GetToolchainConfig(r.Client)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("unable to get ToolchainConfig: %w", err)
	}

	// check if the `status.revisions` field is up-to-date and create a TTR for each TierTemplate
	if created, err := r.ensureRevision(ctx, tier); err != nil {
		// todo add/update ready condition false in the NSTemplateTier when something fails
		return reconcile.Result{}, fmt.Errorf("unable to create new TierTemplateRevision after NSTemplateTier changed: %w", err)
	} else if created {
		logger.Info("Requeue after creating a new TTR")
		return reconcile.Result{RequeueAfter: time.Second}, nil
	}

	return reconcile.Result{}, nil
}

// ensureRevision ensures that there is a TierTemplateRevision CR for each of the TierTemplate.
// returns `true` if a new TierTemplateRevision CR was created, `err` if something wrong happened
func (r *Reconciler) ensureRevision(ctx context.Context, nsTmplTier *toolchainv1alpha1.NSTemplateTier) (bool, error) {
	logger := log.FromContext(ctx)
	refs := getNSTemplateTierRefs(nsTmplTier)

	// init revisions
	if nsTmplTier.Status.Revisions == nil {
		nsTmplTier.Status.Revisions = map[string]string{}
	}
	// check for TierTemplates and TierTemplateRevisions associated with the NSTemplateTier
	ttrCreated := false
	for _, tierTemplateRef := range refs {
		// get the TierTemplate
		var tierTemplate toolchainv1alpha1.TierTemplate
		if err := r.Client.Get(ctx, types.NamespacedName{Namespace: nsTmplTier.GetNamespace(), Name: tierTemplateRef}, &tierTemplate); err != nil {
			// something went wrong or we haven't found the TierTemplate
			return false, err
		}

		// check if there is TTR associated with this TierTemplate
		ttrCreatedLatest, ttrName, err := r.ensureTTRforTemplate(ctx, nsTmplTier, &tierTemplate)
		ttrCreated = ttrCreated || ttrCreatedLatest
		if err != nil {
			return false, err
		}
		nsTmplTier.Status.Revisions[tierTemplate.GetName()] = ttrName
	}
	// handle removal of TierTemplate from NSTemplateTier
	// scenario:
	// 		a. TierTemplate is removed/replaced from NSTemplateTier.Spec
	//		b. NSTemplateTier.Status.Revisions must be cleaned up
	tierRemoved := false
	for tierTempalateKey := range nsTmplTier.Status.Revisions {
		if !checkIfTierTemplateIsStillUsed(nsTmplTier, tierTempalateKey) {
			// remove old tiermtemplate from revisions
			delete(nsTmplTier.Status.Revisions, tierTempalateKey)
			tierRemoved = true
		}
	}

	if ttrCreated || tierRemoved {
		// we need to update the status.revisions with the new ttrs
		logger.Info("ttr created or tier removed, updating status")
		if err := r.Client.Status().Update(ctx, nsTmplTier); err != nil {
			return ttrCreated, err
		}
	}

	return ttrCreated, nil
}

func checkIfTierTemplateIsStillUsed(nsTmplTier *toolchainv1alpha1.NSTemplateTier, tierTemplateRef string) bool {
	if nsTmplTier.Spec.ClusterResources != nil && tierTemplateRef == nsTmplTier.Spec.ClusterResources.TemplateRef {
		return true
	}
	for _, nsTemplate := range nsTmplTier.Spec.Namespaces {
		if nsTemplate.TemplateRef == tierTemplateRef {
			return true
		}
	}
	for _, nsTemplate := range nsTmplTier.Spec.SpaceRoles {
		if nsTemplate.TemplateRef == tierTemplateRef {
			return true
		}
	}
	// not found
	return false
}

func (r *Reconciler) ensureTTRforTemplate(ctx context.Context, nsTmplTier *toolchainv1alpha1.NSTemplateTier, tierTemplate *toolchainv1alpha1.TierTemplate) (bool, string, error) {
	logger := log.FromContext(ctx)
	// tierTemplate doesn't support TTRs
	// we set TierTemplate as revisions
	// TODO this step will be removed once we convert all TierTemplates to TTRs
	if tierTemplate.Spec.TemplateObjects == nil {
		_, ok := nsTmplTier.Status.Revisions[tierTemplate.GetName()]
		if !ok {
			return true, tierTemplate.GetName(), nil
		}
		// nothing to update
		return false, "", nil
	}

	tierTemplateRevisionName, found := nsTmplTier.Status.Revisions[tierTemplate.GetName()]
	if !found {
		// no revision was set for this TierTemplate CR, let's create a TTR for it
		ttrName, err := r.createNewTierTemplateRevision(ctx, nsTmplTier, tierTemplate)
		return true, ttrName, err
	}

	logger.Info("TTR set in the status.revisions for tiertemplate", "tierTemplate.Name", tierTemplate.GetName(), "ttr.Name", tierTemplateRevisionName)
	var tierTemplateRevision toolchainv1alpha1.TierTemplateRevision
	if err := r.Client.Get(ctx, types.NamespacedName{Namespace: nsTmplTier.GetNamespace(), Name: tierTemplateRevisionName}, &tierTemplateRevision); err != nil {
		if errors.IsNotFound(err) {
			// no tierTemplateRevision CR was found,
			logger.Info("TTR CR not found", "tierTemplateRevision.Name", tierTemplateRevisionName)
			// let's create one
			ttrName, err := r.createNewTierTemplateRevision(ctx, nsTmplTier, tierTemplate)
			return true, ttrName, err
		} else {
			// something wrong happened
			return false, "", err
		}
	}
	// if the TierTemplate has changes we need to create new TTR
	// if the TierTemplate objects are the same as they are in the TTR
	// and the parameters are the same, too, we don't have to create
	// a new revision.
	if reflect.DeepEqual(tierTemplate.Spec.TemplateObjects, tierTemplateRevision.Spec.TemplateObjects) && reflect.DeepEqual(nsTmplTier.Spec.Parameters, tierTemplateRevision.Spec.Parameters) {
		return false, tierTemplateRevisionName, nil
	}

	// there are some changes, a new TTR is needed
	ttrName, err := r.createNewTierTemplateRevision(ctx, nsTmplTier, tierTemplate)
	if err != nil {
		return false, "", err
	}
	return true, ttrName, nil

}

func (r *Reconciler) createNewTierTemplateRevision(ctx context.Context, nsTmplTier *toolchainv1alpha1.NSTemplateTier, tierTemplate *toolchainv1alpha1.TierTemplate) (string, error) {
	ttr := NewTTR(tierTemplate, nsTmplTier)
	ttr, err := r.createTTR(ctx, ttr, tierTemplate)
	if err != nil {
		// something went wrong while creating new ttr
		return "", err
	}
	return ttr.GetName(), nil
}

// getNSTemplateTierRefs returns a list with all the refs from the NSTemplateTier
func getNSTemplateTierRefs(tmplTier *toolchainv1alpha1.NSTemplateTier) []string {
	var refs []string
	for _, ns := range tmplTier.Spec.Namespaces {
		refs = append(refs, ns.TemplateRef)
	}
	if tmplTier.Spec.ClusterResources != nil {
		refs = append(refs, tmplTier.Spec.ClusterResources.TemplateRef)
	}

	roles := make([]string, 0, len(tmplTier.Spec.SpaceRoles))
	for r := range tmplTier.Spec.SpaceRoles {
		roles = append(roles, r)
	}
	for _, r := range roles {
		refs = append(refs, tmplTier.Spec.SpaceRoles[r].TemplateRef)
	}
	return refs
}

func (r *Reconciler) createTTR(ctx context.Context, ttr *toolchainv1alpha1.TierTemplateRevision, tmplTier *toolchainv1alpha1.TierTemplate) (*toolchainv1alpha1.TierTemplateRevision, error) {
	err := r.Client.Create(ctx, ttr)
	if err != nil {
		return nil, fmt.Errorf("unable to create TierTemplateRevision: %w", err)
	}

	logger := log.FromContext(ctx)
	logger.Info("created TierTemplateRevision", "tierTemplateRevision.Name", ttr.Name, "tierTemplate.Name", tmplTier.Name)
	return ttr, nil
}

// NewTTR creates a TierTemplateRevision CR for a given TierTemplate object.
func NewTTR(tierTmpl *toolchainv1alpha1.TierTemplate, nsTmplTier *toolchainv1alpha1.NSTemplateTier) *toolchainv1alpha1.TierTemplateRevision {
	tierName := nsTmplTier.GetName()
	tierTemplateName := tierTmpl.GetName()
	labels := map[string]string{
		toolchainv1alpha1.TierLabelKey:        tierName,
		toolchainv1alpha1.TemplateRefLabelKey: tierTemplateName,
	}

	newTTRName := fmt.Sprintf("%s-%s-", tierName, tierTemplateName)
	ttr := &toolchainv1alpha1.TierTemplateRevision{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:    tierTmpl.GetNamespace(),
			GenerateName: newTTRName,
			Labels:       labels,
		},
		Spec: toolchainv1alpha1.TierTemplateRevisionSpec{
			TemplateObjects: tierTmpl.Spec.TemplateObjects,
			Parameters:      nsTmplTier.Spec.Parameters, // save the parameters from the NSTemplateTier,to detect further changes and for evaluating those in the TemplateObjects when provisioning Spaces.
		},
	}

	return ttr
}
