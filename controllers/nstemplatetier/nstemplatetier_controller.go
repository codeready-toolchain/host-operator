package nstemplatetier

import (
	"context"
	"fmt"
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/controllers/toolchainconfig"
	"github.com/codeready-toolchain/toolchain-common/pkg/hash"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"
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

// ----------------------------------------------------------------------------------------------------------------------------
// NSTemplateTier Controller Reconciler:
// . in case of a new NSTemplateTier update to process:
// .. inserts a new record in the `status.updates` history
// ----------------------------------------------------------------------------------------------------------------------------

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

// Reconcile takes care of:
// - inserting a new entry in the `status.updates`
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

	// create a new entry in the `status.history` if needed
	if added, err := r.ensureStatusUpdateRecord(ctx, tier); err != nil {
		logger.Error(err, "unable to insert a new entry in status.updates after NSTemplateTier changed")
		return reconcile.Result{}, fmt.Errorf("unable to insert a new entry in status.updates after NSTemplateTier changed: %w", err)
	} else if added {
		logger.Info("Requeue after adding a new entry in tier.status.updates")
		return reconcile.Result{Requeue: true}, nil
	}

	// check if the `status.revisions` field is up-to-date and create a TTR for each TierTemplate
	if created, err := r.ensureRevision(ctx, tier); err != nil {
		return reconcile.Result{}, fmt.Errorf("unable to create new TierTemplateRevision after NSTemplateTier changed: %w", err)
	} else if created {
		logger.Info("Requeue after creating a new TTR")
		return reconcile.Result{Requeue: true, RequeueAfter: 10 * time.Second}, nil
	}

	return reconcile.Result{}, nil
}

// ensureStatusUpdateRecord adds a new entry in the `status.updates` with the current date/time
// if needed and the hash of the NSTemplateTier
// returns `true` if an entry was added, `err` if something wrong happened
func (r *Reconciler) ensureStatusUpdateRecord(ctx context.Context, tier *toolchainv1alpha1.NSTemplateTier) (bool, error) {
	hash, err := hash.ComputeHashForNSTemplateTier(tier)
	if err != nil {
		return false, fmt.Errorf("unable to append an entry in the `status.updates` for NSTemplateTier '%s' : %w", tier.Name, err)
	}
	// if there was no previous status:
	if len(tier.Status.Updates) == 0 {
		return true, r.addNewTierUpdate(ctx, tier, hash)
	}

	// check whether the entry was already added
	logger := log.FromContext(ctx)
	if tier.Status.Updates[len(tier.Status.Updates)-1].Hash == hash {
		logger.Info("current tier template already exists in tier.status.updates")
		return false, nil
	}
	logger.Info("Adding a new entry in tier.status.updates")
	return true, r.addNewTierUpdate(ctx, tier, hash)
}

func (r *Reconciler) addNewTierUpdate(ctx context.Context, tier *toolchainv1alpha1.NSTemplateTier, hash string) error {
	tier.Status.Updates = append(tier.Status.Updates, toolchainv1alpha1.NSTemplateTierHistory{
		StartTime: metav1.Now(),
		Hash:      hash,
	})
	return r.Client.Status().Update(ctx, tier)
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
				if err != nil {
					return false, "", err
				}
				// new ttr created
				return true, ttrName, nil
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
		if err != nil {
			return false, ttrName, err
		}
		// new ttr created
		return true, ttrName, nil
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
	if err != nil && !errors.IsAlreadyExists(err) {
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
	// we have to cut it down to the max allowed length
	// 6 = 5 is generateName suffix length + 1 the "-" char
	mexNameLength := validation.DNS1123LabelMaxLength + 6
	if len(newTTRName) > mexNameLength {
		newTTRName = newTTRName[0:mexNameLength]
	}
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
