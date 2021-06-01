package hostoperatorconfig

import (
	"context"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("hostoperatorconfig-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource HostOperatorConfig
	return c.Watch(&source.Kind{Type: &toolchainv1alpha1.HostOperatorConfig{}}, &handler.EnqueueRequestForObject{})
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr manager.Manager) error {
	return add(mgr, r)
}

// Reconciler reconciles a HostOperatorConfig object
type Reconciler struct {
	Client client.Client
	Log    logr.Logger
}

// Reconcile reads that state of the cluster for a HostOperatorConfig object and makes changes based on the state read
// and what is in the HostOperatorConfig.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *Reconciler) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := r.Log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling HostOperatorConfig")

	// Fetch the HostOperatorConfig instance
	hostOperatorConfig := &toolchainv1alpha1.HostOperatorConfig{}
	err := r.Client.Get(context.TODO(), request.NamespacedName, hostOperatorConfig)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			reqLogger.Error(err, "it looks like the HostOperatorConfig resource with the name 'config' was removed - the cache will use the latest version of the resource")
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}
	updateConfig(hostOperatorConfig)
	return reconcile.Result{}, nil
}
