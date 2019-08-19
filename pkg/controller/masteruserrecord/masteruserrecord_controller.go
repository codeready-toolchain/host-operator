package masteruserrecord

import (
	"context"
	"fmt"
	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	"github.com/codeready-toolchain/toolchain-common/pkg/condition"
	"github.com/go-logr/logr"
	"github.com/operator-framework/operator-sdk/pkg/predicate"
	errs "github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"sigs.k8s.io/kubefed/pkg/controller/util"
)

var log = logf.Log.WithName("controller_masteruserrecord")

const (
	// Status condition reasons
	unableToGetUserAccountReason = "UnableToGetUserAccount"
	targetClusterNotReadyReason  = "TargetClusterNotReady"
	provisioningReason           = "Provisioning"
	updatingReason               = "Updating"
	provisionedReason            = "Provisioned"
)

// Add creates a new MasterUserRecord Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileMasterUserRecord{
		client:                mgr.GetClient(),
		scheme:                mgr.GetScheme(),
		retrieveMemberCluster: cluster.GetFedCluster}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("masteruserrecord-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource MasterUserRecord
	err = c.Watch(&source.Kind{
		Type: &toolchainv1alpha1.MasterUserRecord{}},
		&handler.EnqueueRequestForObject{},
		predicate.GenerationChangedPredicate{})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileMasterUserRecord{}

// ReconcileMasterUserRecord reconciles a MasterUserRecord object
type ReconcileMasterUserRecord struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client                client.Client
	scheme                *runtime.Scheme
	retrieveMemberCluster func(name string) (*cluster.FedCluster, bool)
}

// Reconcile reads that state of the cluster for a MasterUserRecord object and makes changes based on the state read
// and what is in the MasterUserRecord.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileMasterUserRecord) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling MasterUserRecord")

	// Fetch the MasterUserRecord instance
	userRecord := &toolchainv1alpha1.MasterUserRecord{}
	err := r.client.Get(context.TODO(), request.NamespacedName, userRecord)
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

	for _, account := range userRecord.Spec.UserAccounts {
		err = r.ensureUserAccount(reqLogger, account, userRecord)
		if err != nil {
			reqLogger.Error(err, "unable to synchronize with member UserAccount")
			return reconcile.Result{}, err
		}
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileMasterUserRecord) ensureUserAccount(log logr.Logger, recAccount toolchainv1alpha1.UserAccountEmbedded, record *toolchainv1alpha1.MasterUserRecord) error {
	// get & check member cluster
	memberCluster, err := r.getMemberCluster(recAccount)
	if err != nil {
		return r.wrapErrorWithStatusUpdate(log, record, r.setStatusFailed(targetClusterNotReadyReason), err,
			"failed to get the member cluster '%s'", recAccount.TargetCluster)
	}

	// get UserAccount from member
	nsdName := namespacedName(memberCluster.OperatorNamespace, record.Name)
	userAccount := &toolchainv1alpha1.UserAccount{}
	err = memberCluster.Client.Get(context.TODO(), nsdName, userAccount)
	if err != nil {
		if errors.IsNotFound(err) {
			// does not exist - should create
			userAccount = newUserAccount(nsdName, recAccount.Spec)
			if err := updateStatusConditions(r.client, record, toBeNotReady(provisioningReason, "")); err != nil {
				return err
			}
			if err := memberCluster.Client.Create(context.TODO(), userAccount); err != nil {
				return r.wrapErrorWithStatusUpdate(log, record, r.setStatusFailed(targetClusterNotReadyReason), err,
					"failed to create UserAccount in the member cluster '%s'", recAccount.TargetCluster)
			}
			return nil
		}
		return r.wrapErrorWithStatusUpdate(log, record, r.setStatusFailed(unableToGetUserAccountReason), err,
			"failed to get userAccount '%s' from cluster '%s'", record.Name, recAccount.TargetCluster)
	}

	sync := Synchronizer{
		record:            record,
		hostClient:        r.client,
		memberClient:      memberCluster.Client,
		memberUserAcc:     userAccount,
		recordSpecUserAcc: recAccount,
	}
	if err := sync.synchronizeSpec(); err != nil {
		return errs.Wrapf(err, "update of the UserAccount.spec in the cluster '%s' failed", recAccount.TargetCluster)
	}
	if err := sync.synchronizeStatus(); err != nil {
		return errs.Wrapf(err, "update of the UserAccount.status in the cluster '%s' failed", recAccount.TargetCluster)
	}
	return nil
}

func (r *ReconcileMasterUserRecord) getMemberCluster(recAccount toolchainv1alpha1.UserAccountEmbedded) (*cluster.FedCluster, error) {
	// get & check fed cluster
	fedCluster, ok := r.retrieveMemberCluster(recAccount.TargetCluster)
	if !ok {
		return nil, fmt.Errorf("the member cluster %s not found in the registry", recAccount.TargetCluster)
	}
	if !util.IsClusterReady(fedCluster.ClusterStatus) {
		return nil, fmt.Errorf("the member cluster %s is not ready", recAccount.TargetCluster)
	}
	return fedCluster, nil
}

// wrapErrorWithStatusUpdate wraps the error and update the user account status. If the update failed then logs the error.
func (r *ReconcileMasterUserRecord) wrapErrorWithStatusUpdate(logger logr.Logger, record *toolchainv1alpha1.MasterUserRecord, statusUpdater func(record *toolchainv1alpha1.MasterUserRecord, message string) error, err error, format string, args ...interface{}) error {
	if err == nil {
		return nil
	}
	if err := statusUpdater(record, err.Error()); err != nil {
		logger.Error(err, "status update failed")
	}
	return errs.Wrapf(err, format, args...)
}

func (r *ReconcileMasterUserRecord) setStatusFailed(reason string) func(record *toolchainv1alpha1.MasterUserRecord, message string) error {
	return func(record *toolchainv1alpha1.MasterUserRecord, message string) error {
		return updateStatusConditions(
			r.client,
			record,
			toBeNotReady(reason, message))
	}
}

func toBeProvisioned() toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:   toolchainv1alpha1.ConditionReady,
		Status: corev1.ConditionTrue,
		Reason: provisionedReason,
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

// updateStatusConditions updates user account status conditions with the new conditions
func updateStatusConditions(cl client.Client, record *toolchainv1alpha1.MasterUserRecord, newConditions ...toolchainv1alpha1.Condition) error {
	var updated bool
	record.Status.Conditions, updated = condition.AddOrUpdateStatusConditions(record.Status.Conditions, newConditions...)
	if !updated {
		// Nothing changed
		return nil
	}
	return cl.Status().Update(context.TODO(), record)
}

func newUserAccount(nsdName types.NamespacedName, spec toolchainv1alpha1.UserAccountSpec) *toolchainv1alpha1.UserAccount {
	return &toolchainv1alpha1.UserAccount{
		ObjectMeta: v1.ObjectMeta{
			Name:      nsdName.Name,
			Namespace: nsdName.Namespace,
		},
		Spec: spec,
	}
}

func namespacedName(namespace, name string) types.NamespacedName {
	return types.NamespacedName{Namespace: namespace, Name: name}
}
