package registrationservice

import (
	"context"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/configuration"
	commonclient "github.com/codeready-toolchain/toolchain-common/pkg/client"
	"github.com/codeready-toolchain/toolchain-common/pkg/condition"
	"github.com/codeready-toolchain/toolchain-common/pkg/template"

	"github.com/go-logr/logr"
	v1 "github.com/openshift/api/template/v1"
	errs "github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_registrationservice")

// Add creates a new RegistrationService Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, _ *configuration.Registry) error {
	deploymentTemplate, err := getDeploymentTemplate(mgr.GetScheme())
	if err != nil {
		return errs.Wrap(err, "unable to decode the registration service deployment")
	}

	return add(mgr, newReconciler(mgr, deploymentTemplate))
}

func getDeploymentTemplate(s *runtime.Scheme) (*v1.Template, error) {
	deployment, err := Asset("registration-service.yaml")
	if err != nil {
		return nil, err
	}
	decoder := serializer.NewCodecFactory(s).UniversalDeserializer()
	deploymentTemplate := &v1.Template{}
	_, _, err = decoder.Decode([]byte(deployment), nil, deploymentTemplate)
	return deploymentTemplate, err
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, regServiceDeployment *v1.Template) *ReconcileRegistrationService {
	return &ReconcileRegistrationService{
		client:             mgr.GetClient(),
		scheme:             mgr.GetScheme(),
		regServiceTemplate: regServiceDeployment,
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r *ReconcileRegistrationService) error {
	// Create a new controller
	c, err := controller.New("registrationservice-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource RegistrationService
	err = c.Watch(&source.Kind{Type: &toolchainv1alpha1.RegistrationService{}}, &handler.EnqueueRequestForObject{}, predicate.GenerationChangedPredicate{})
	if err != nil {
		return err
	}

	// process with default variables - we need to get just the list of objects - we don't care about their content
	processor := template.NewProcessor(r.scheme)
	objects, err := processor.Process(r.regServiceTemplate.DeepCopy(), map[string]string{})
	if err != nil {
		return err
	}

	// call watch for all objects contained within the template
	for _, object := range objects {
		if object.Object == nil {
			continue
		}
		err = c.Watch(&source.Kind{Type: object.Object}, &handler.EnqueueRequestForOwner{
			IsController: true,
			OwnerType:    &toolchainv1alpha1.RegistrationService{},
		})
		if err != nil {
			return err
		}
	}

	return nil
}

// blank assignment to verify that ReconcileRegistrationService implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileRegistrationService{}

// ReconcileRegistrationService reconciles a RegistrationService object
type ReconcileRegistrationService struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client             client.Client
	scheme             *runtime.Scheme
	regServiceTemplate *v1.Template
}

// Reconcile reads that state of the cluster for a RegistrationService object and makes changes based on the state read
// and what is in the RegistrationService.Spec
func (r *ReconcileRegistrationService) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling RegistrationService")

	// Fetch the RegistrationService regService
	regService := &toolchainv1alpha1.RegistrationService{}
	err := r.client.Get(context.TODO(), request.NamespacedName, regService)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	// process template with variables taken from the RegistrationService CRD
	client := commonclient.NewApplyClient(r.client, r.scheme)
	objects, err := template.NewProcessor(r.scheme).Process(r.regServiceTemplate.DeepCopy(), getVars(regService))
	if err != nil {
		return reconcile.Result{}, err
	}

	// create all objects that are within the template, and update only when the object has changed.
	// if the object was either created or updated, then return and wait for another reconcile
	for _, object := range objects {
		createdOrUpdated, err := client.CreateOrUpdateObject(object.Object, false, regService)
		if err != nil {
			return reconcile.Result{}, r.wrapErrorWithStatusUpdate(reqLogger, regService, r.setStatusFailed(toolchainv1alpha1.RegistrationServiceDeployingFailedReason), err, "cannot deploy registration service template")
		}
		if createdOrUpdated {
			return reconcile.Result{}, updateStatusConditions(r.client, regService, toBeNotReady(toolchainv1alpha1.RegistrationServiceDeployingReason, ""))
		}
	}

	reqLogger.Info("All objects in registration service template has been created and are up-to-date")
	return reconcile.Result{}, updateStatusConditions(r.client, regService, toBeDeployed())
}

type templateVars map[string]string

func getVars(regService *toolchainv1alpha1.RegistrationService) map[string]string {
	var vars templateVars = map[string]string{}
	vars.addIfNotEmpty("NAMESPACE", regService.Namespace)
	for key, value := range regService.Spec.EnvironmentVariables {
		vars.addIfNotEmpty(key, value)
	}
	return vars
}

func (v *templateVars) addIfNotEmpty(key, value string) {
	if value != "" {
		(*v)[key] = value
	}
}

// updateStatusConditions updates RegistrationService status conditions with the new conditions
func updateStatusConditions(cl client.Client, regServ *toolchainv1alpha1.RegistrationService, newConditions ...toolchainv1alpha1.Condition) error {
	var updated bool
	regServ.Status.Conditions, updated = condition.AddOrUpdateStatusConditions(regServ.Status.Conditions, newConditions...)
	if !updated {
		// Nothing changed
		return nil
	}
	return cl.Status().Update(context.TODO(), regServ)
}

func (r *ReconcileRegistrationService) setStatusFailed(reason string) func(regServ *toolchainv1alpha1.RegistrationService, message string) error {
	return func(regServ *toolchainv1alpha1.RegistrationService, message string) error {
		return updateStatusConditions(
			r.client,
			regServ,
			toBeNotReady(reason, message))
	}
}

// wrapErrorWithStatusUpdate wraps the error and update the RegistrationService status. If the update fails then the error is logged.
func (r *ReconcileRegistrationService) wrapErrorWithStatusUpdate(logger logr.Logger, regServ *toolchainv1alpha1.RegistrationService,
	statusUpdater func(regServ *toolchainv1alpha1.RegistrationService, message string) error, err error, format string, args ...interface{}) error {
	if err == nil {
		return nil
	}
	if err := statusUpdater(regServ, err.Error()); err != nil {
		logger.Error(err, "Error updating RegistrationService status")
	}
	return errs.Wrapf(err, format, args...)
}

func toBeDeployed() toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:   toolchainv1alpha1.ConditionReady,
		Status: corev1.ConditionTrue,
		Reason: toolchainv1alpha1.RegistrationServiceDeployedReason,
	}
}

func toBeNotReady(reason, msg string) toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:    toolchainv1alpha1.ConditionReady,
		Status:  corev1.ConditionFalse,
		Reason:  reason,
		Message: msg,
	}
}
