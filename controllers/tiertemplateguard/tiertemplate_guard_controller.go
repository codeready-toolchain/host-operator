package tiertemplateguard

import (
	"context"
	"fmt"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/redhat-cop/operator-utils/pkg/util"
	"k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type Reconciler struct {
	Client client.Client
}

func (r *Reconciler) SetupWithManager(mgr manager.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&toolchainv1alpha1.TierTemplate{}).
		Complete(r)
}

func (r *Reconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// fetch the TT
	tt := &toolchainv1alpha1.TierTemplate{}
	if err := r.Client.Get(ctx, request.NamespacedName, tt); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("TierTemplate not found")
			return reconcile.Result{}, nil
		}

		// Error reading the object - requeue the request.
		return reconcile.Result{}, fmt.Errorf("unable to get the current TierTemplate: %w", err)
	}

	if util.IsBeingDeleted(tt) {
		return reconcile.Result{}, r.handleDeletion(ctx, tt)
	}

	if err := r.addFinalizer(ctx, tt); err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to add the finalizer: %w", err)
	}

	return reconcile.Result{}, nil
}

func (r *Reconciler) addFinalizer(ctx context.Context, tt *toolchainv1alpha1.TierTemplate) error {
	// Add the finalizer if it is not present
	if !controllerutil.ContainsFinalizer(tt, toolchainv1alpha1.FinalizerName) {
		logger := log.FromContext(ctx)
		logger.Info("adding finalizer on TierTemplate")
		controllerutil.AddFinalizer(tt, toolchainv1alpha1.FinalizerName)
		if err := r.Client.Update(ctx, tt); err != nil {
			return err
		}
	}
	return nil
}

func (r *Reconciler) handleDeletion(ctx context.Context, tt *toolchainv1alpha1.TierTemplate) error {
	used, err := r.isUsed(ctx, tt)
	if err != nil {
		return err
	}
	if used {
		return fmt.Errorf("TierTemplate still used by some NSTemplateTier")
	}

	// Remove finalizer from the TierTemplate, it's no longer used anywhere
	// so we can let the cluster proceed with the deletion.
	util.RemoveFinalizer(tt, toolchainv1alpha1.FinalizerName)
	return r.Client.Update(ctx, tt)
}

func (r *Reconciler) isUsed(ctx context.Context, tt *toolchainv1alpha1.TierTemplate) (bool, error) {
	list := &toolchainv1alpha1.NSTemplateTierList{}
	if err := r.Client.List(ctx, list, client.InNamespace(tt.GetNamespace())); err != nil {
		return false, err
	}

	for _, nstt := range list.Items {
		if nstt.Spec.ClusterResources != nil && nstt.Spec.ClusterResources.TemplateRef == tt.Name {
			return true, nil
		}
		for _, ns := range nstt.Spec.Namespaces {
			if ns.TemplateRef == tt.Name {
				return true, nil
			}
		}
		for _, sr := range nstt.Spec.SpaceRoles {
			if sr.TemplateRef == tt.Name {
				return true, nil
			}
		}
	}
	return false, nil
}
