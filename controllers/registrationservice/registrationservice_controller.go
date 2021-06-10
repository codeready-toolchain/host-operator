package registrationservice

import (
	"context"
	"fmt"
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	applycl "github.com/codeready-toolchain/toolchain-common/pkg/client"
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
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

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

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r *Reconciler) error {
	deploymentTemplate, err := getDeploymentTemplate(mgr.GetScheme())
	if err != nil {
		return errs.Wrap(err, "unable to decode the registration service deployment")
	}
	r.regServiceTemplate = deploymentTemplate

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
	processor := template.NewProcessor(r.Scheme)
	toolchainObjects, err := processor.Process(r.regServiceTemplate.DeepCopy(), map[string]string{})
	if err != nil {
		return err
	}

	// call watch for all objects contained within the template
	for _, toolchainObject := range toolchainObjects {
		if toolchainObject.GetRuntimeObject() == nil {
			continue
		}
		err = c.Watch(&source.Kind{Type: toolchainObject.GetRuntimeObject()}, &handler.EnqueueRequestForOwner{
			IsController: true,
			OwnerType:    &toolchainv1alpha1.RegistrationService{},
		})
		if err != nil {
			return err
		}
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr manager.Manager) error {
	return add(mgr, r)
}

// Reconciler reconciles a RegistrationService object
type Reconciler struct {
	Client             client.Client
	Log                logr.Logger
	Scheme             *runtime.Scheme
	regServiceTemplate *v1.Template
}

//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=registrationservices,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=registrationservices/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=registrationservices/finalizers,verbs=update

//+kubebuilder:rbac:groups="",resources=configmaps;services;serviceaccounts,verbs=get;list;watch;update;patch;create;delete
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io;authorization.openshift.io,resources=rolebindings;roles,verbs=*
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;update;patch;create;delete
//+kubebuilder:rbac:groups=route.openshift.io,resources=routes,verbs=get;list;watch;update;patch;create;delete

// Reconcile reads that state of the cluster for a RegistrationService object and makes changes based on the state read
// and what is in the RegistrationService.Spec
func (r *Reconciler) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := r.Log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling RegistrationService")

	// Fetch the RegistrationService regService
	regService := &toolchainv1alpha1.RegistrationService{}
	err := r.Client.Get(context.TODO(), request.NamespacedName, regService)
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
	cl := applycl.NewApplyClient(r.Client, r.Scheme)
	toolchainObjects, err := template.NewProcessor(r.Scheme).Process(r.regServiceTemplate.DeepCopy(), getVars(regService))
	if err != nil {
		return reconcile.Result{}, err
	}

	// create all objects that are within the template, and update only when the object has changed.
	var updated []string
	for _, toolchainObject := range toolchainObjects {
		createdOrUpdated, err := cl.ApplyObject(toolchainObject.GetRuntimeObject(), applycl.SetOwner(regService))
		if err != nil {
			return reconcile.Result{}, r.wrapErrorWithStatusUpdate(reqLogger, regService, r.setStatusFailed(toolchainv1alpha1.RegistrationServiceDeployingFailedReason), err, "cannot deploy registration service template")
		}
		if createdOrUpdated {
			updated = append(updated, fmt.Sprintf("%s: %s", toolchainObject.GetGvk().Kind, toolchainObject.GetName()))
		}
	}
	if len(updated) > 0 {
		return reconcile.Result{
			Requeue:      true,
			RequeueAfter: time.Second, // le't put one second there in case it wasn't re-queued by some resource change
		}, updateStatusConditions(r.Client, regService, toBeNotReady(toolchainv1alpha1.RegistrationServiceDeployingReason, fmt.Sprintf("updated resources: %s", updated)))
	}

	reqLogger.Info("All objects in registration service template has been created and are up-to-date")
	return reconcile.Result{}, updateStatusConditions(r.Client, regService, toBeDeployed())
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

func (r *Reconciler) setStatusFailed(reason string) func(regServ *toolchainv1alpha1.RegistrationService, message string) error {
	return func(regServ *toolchainv1alpha1.RegistrationService, message string) error {
		return updateStatusConditions(
			r.Client,
			regServ,
			toBeNotReady(reason, message))
	}
}

// wrapErrorWithStatusUpdate wraps the error and update the RegistrationService status. If the update fails then the error is logged.
func (r *Reconciler) wrapErrorWithStatusUpdate(logger logr.Logger, regServ *toolchainv1alpha1.RegistrationService,
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
