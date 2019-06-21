package masteruserrecord

import (
	"context"
	"fmt"
	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/cluster"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/kubefed/pkg/controller/util"
	"time"
)

type SyncResult struct {
	reconcile.Result
	Status SyncStatus
	Err    error
}

type SyncStatus string

const (
	ClusterNotFound SyncStatus = "cluster_not_found"
	ClusterNotReady SyncStatus = "cluster_not_ready"
	Error           SyncStatus = "error"
	Ok              SyncStatus = "ok"
)

var OkResult = newSyncResultf(Ok, false, 0, "")

func newSyncResultf(status SyncStatus, requeue bool, delay time.Duration, errMsg string, args ...string) SyncResult {
	return SyncResult{
		Result: reconcile.Result{
			Requeue:      requeue,
			RequeueAfter: delay,
		},
		Status: status,
		Err:    fmt.Errorf(errMsg, args),
	}
}

func newErrSyncResult(requeue bool, delay time.Duration, err error) SyncResult {
	return SyncResult{
		Result: reconcile.Result{
			Requeue:      requeue,
			RequeueAfter: delay,
		},
		Status: Error,
		Err:    err,
	}
}

func syncWithUserAccount(account toolchainv1alpha1.UserAccountEmbedded, record *toolchainv1alpha1.MasterUserRecord) SyncResult {
	// get & check fed cluster
	fedCluster, ok := cluster.GetFedCluster(account.TargetCluster)
	if !ok {
		return newSyncResultf(ClusterNotFound, true, time.Second,
			"the fedCluster %s not found in the registry", account.TargetCluster)
	}
	if !util.IsClusterReady(fedCluster.ClusterStatus) {
		return newSyncResultf(ClusterNotReady, true, time.Second,
			"the fedCluster %s is not ready", account.TargetCluster)
	}

	// get UserAccount from member
	nsdName := namespacedName(fedCluster.OperatorNamespace, record.Name)
	userAccount := &toolchainv1alpha1.UserAccount{}
	err := fedCluster.Client.Get(context.TODO(), nsdName, userAccount)
	if err != nil {
		if errors.IsNotFound(err) {
			// does not exist - should create
			userAccount = newUserAccount(nsdName, account.Spec)
			err := fedCluster.Client.Create(context.TODO(), userAccount)
			// todo set status=provisioning & update status
			if err != nil {
				return newErrSyncResult(true, time.Minute, err)
			}
			return OkResult
		}
		return newErrSyncResult(true, time.Minute, err)
	}

	if !reflect.DeepEqual(userAccount.Spec, account.Spec) {
		// when record is updated
		userAccount.Spec = account.Spec
		err := fedCluster.Client.Update(context.TODO(), userAccount)
		// todo set status=updating & update status
		if err != nil {
			return newErrSyncResult(true, time.Minute, err)
		}

	} else {
		// when record should update status
		recStatus := getUserAccountStatus(account.TargetCluster, record)
		if !reflect.DeepEqual(userAccount.Status, recStatus) {
			// todo update status
		}
	}
	return OkResult
}

func getUserAccountStatus(clusterName string, record *toolchainv1alpha1.MasterUserRecord) toolchainv1alpha1.UserAccountStatus {
	for _, account := range record.Status.UserAccounts {
		if account.TargetCluster == clusterName {
			return account.UserAccountStatus
		}
	}
	return toolchainv1alpha1.UserAccountStatus{}
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
