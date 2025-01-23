package nstemplatetier

import (
	"context"
	"fmt"
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/controllers/toolchainconfig"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/builder"
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
	// TODO handle removal of TierTemplate from NSTemplateTier
	// scenario:
	// 		a. TierTemplate is removed/replaced from NSTemplateTier.Spec
	//		b. NSTemplateTier.Status.Revisions must be cleaned up
	// Thus here we should iterate over the Status.Revisions field
	// and check if there is any reference to a TierTemplate that is not in the Spec anymore
	if ttrCreated {
		// we need to update the status.revisions with the new ttrs
		logger.Info("ttr created updating status")
		if err := r.Client.Status().Update(ctx, nsTmplTier); err != nil {
			return ttrCreated, err
		}
	}

	return ttrCreated, nil
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

	if tierTemplateRevisionName, found := nsTmplTier.Status.Revisions[tierTemplate.GetName()]; found {
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
		// TODO compare TierTemplate content with TTR content
		// if the TierTemplate has changes we need to create new TTR
	} else {
		// no revision was set for this TierTemplate CR, let's create a TTR for it
		ttrName, err := r.createNewTierTemplateRevision(ctx, nsTmplTier, tierTemplate)
		return true, ttrName, err
	}
	// nothing changed
	return false, "", nil
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
			GenerateName: newTTRName + "-",
			Labels:       labels,
		},
		Spec: toolchainv1alpha1.TierTemplateRevisionSpec{
			TemplateObjects: tierTmpl.Spec.TemplateObjects,
			Parameters:      nsTmplTier.Spec.Parameters, // save the parameters from the NSTemplateTier,to detect further changes and for evaluating those in the TemplateObjects when provisioning Spaces.
		},
	}

	return ttr
}
