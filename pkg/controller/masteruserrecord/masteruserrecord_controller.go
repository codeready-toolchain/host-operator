package masteruserrecord

import (
	"context"
	"fmt"
	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	"github.com/go-logr/logr"
	"github.com/operator-framework/operator-sdk/pkg/predicate"
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

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new MasterUserRecord Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileMasterUserRecord{
		client:           mgr.GetClient(),
		scheme:           mgr.GetScheme(),
		getMemberCluster: cluster.GetFedCluster}
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
	client           client.Client
	scheme           *runtime.Scheme
	getMemberCluster func(name string) (*cluster.FedCluster, bool)
}

// Reconcile reads that state of the cluster for a MasterUserRecord object and makes changes based on the state read
// and what is in the MasterUserRecord.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileMasterUserRecord) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling MasterUserRecord")

	// Fetch the MasterUserRecord userRecord
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

	// check if the user was already approved
	if userRecord.Spec.State == "pending" { // TODO add statuses to MasterUserRecord in API repo
		reqLogger.Info("The user hasn't been approved yet")
		return reconcile.Result{}, nil
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
	// get & check fed cluster
	fedCluster, ok := r.getMemberCluster(recAccount.TargetCluster)
	if !ok {
		return fmt.Errorf("the fedCluster %s not found in the registry", recAccount.TargetCluster)
	}
	if !util.IsClusterReady(fedCluster.ClusterStatus) {
		return fmt.Errorf("the fedCluster %s is not ready", recAccount.TargetCluster)
	}

	// get UserAccount from member
	nsdName := namespacedName(fedCluster.OperatorNamespace, record.Name)
	userAccount := &toolchainv1alpha1.UserAccount{}
	err := fedCluster.Client.Get(context.TODO(), nsdName, userAccount)
	if err != nil {
		if errors.IsNotFound(err) {
			// does not exist - should create
			userAccount = newUserAccount(nsdName, recAccount.Spec)
			if err := updateStatus(r.client, record, "provisioning", ""); err != nil {
				return err
			}
			if err := fedCluster.Client.Create(context.TODO(), userAccount); err != nil {
				return err
			}
			return nil
		}
		if updateErr := updateStatus(r.client, record, "provisioning", ""); updateErr != nil {
			log.Error(updateErr, "unable to update status")
		}
		return err
	}

	sync := Synchronizer{
		record:        record,
		hostClient:    r.client,
		memberClient:  fedCluster.Client,
		memberUserAcc: userAccount,
		recordUserAcc: recAccount,
	}
	if err := sync.synchronizeSpec(); err != nil {
		return err
	}
	if err := sync.synchronizeStatus(); err != nil {
		return err
	}

	return nil
}

// updateStatus updates user account status to given status with errMsg but only if the current status doesn't match
// If the current status already set to desired state then this method does nothing
// TODO the status should be a specific type of a string or StatusUserAccount - same as for UserAccount
func updateStatus(client client.Client, record *toolchainv1alpha1.MasterUserRecord, status, errMsg string) error {
	// TODO we need to add record.Status.Error field to have place where to store internal errors
	if record.Status.Status == status { //&& record.Status.Error == errMsg {
		// Nothing changed
		return nil
	}
	record.Status = toolchainv1alpha1.MasterUserRecordStatus{
		Status: status,
		//Error:  errMsg,
	}
	return client.Status().Update(context.TODO(), record)
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
