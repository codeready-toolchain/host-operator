package e2e

import (
	"context"
	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/stretchr/testify/require"
	"reflect"

	"github.com/codeready-toolchain/toolchain-common/pkg/test/e2e"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
)

type HostAwaitility struct {
	*e2e.SingleAwaitility
}

func NewHostAwaitility(awaitility *e2e.Awaitility) *HostAwaitility {
	return &HostAwaitility{SingleAwaitility: awaitility.Host()}
}

type MemberAwaitility struct {
	*e2e.SingleAwaitility
}

func NewMemberAwaitility(awaitility *e2e.Awaitility) *MemberAwaitility {
	return &MemberAwaitility{SingleAwaitility: awaitility.Member()}
}

func (a *HostAwaitility) WaitForMasterUserRecord(name string) error {
	return wait.Poll(e2e.RetryInterval, e2e.Timeout, func() (done bool, err error) {
		mur := &toolchainv1alpha1.MasterUserRecord{}
		if err := a.Client.Get(context.TODO(), types.NamespacedName{Namespace: a.Ns, Name: name}, mur); err != nil {
			if errors.IsNotFound(err) {
				a.T.Logf("waiting for availability of MasterUserRecord '%s'", name)
				return false, nil
			}
			return false, err
		}
		a.T.Logf("found MasterUserAccount '%s'", name)
		return true, nil
	})
}

func (a *HostAwaitility) WaitForDeletedMasterUserRecord(name string) error {
	return wait.Poll(e2e.RetryInterval, e2e.Timeout, func() (done bool, err error) {
		mur := &toolchainv1alpha1.MasterUserRecord{}
		if err := a.Client.Get(context.TODO(), types.NamespacedName{Namespace: a.Ns, Name: name}, mur); err != nil {
			if errors.IsNotFound(err) {
				a.T.Logf("MasterUserAccount is checked as deleted '%s'", name)
				return true, nil
			}
			return false, err
		}
		a.T.Logf("waiting until MasterUserAccount is deleted '%s'", name)
		return false, nil
	})
}

func (a *HostAwaitility) GetMasterUserRecord(name string) *toolchainv1alpha1.MasterUserRecord {
	mur := &toolchainv1alpha1.MasterUserRecord{}
	err := a.Client.Get(context.TODO(), types.NamespacedName{Namespace: a.Ns, Name: name}, mur)
	require.NoError(a.T, err)
	return mur
}

func (a *HostAwaitility) WaitForMurConditions(name string, waitCond ...MurWaitCondition) error {
	return wait.Poll(e2e.RetryInterval, e2e.Timeout, func() (done bool, err error) {
		mur := &toolchainv1alpha1.MasterUserRecord{}
		if err := a.Client.Get(context.TODO(), types.NamespacedName{Namespace: a.Ns, Name: name}, mur); err != nil {
			if errors.IsNotFound(err) {
				a.T.Logf("waiting for availability of MasterUserRecord '%s'", name)
				return false, nil
			}
			return false, err
		}
		for _, isMatched := range waitCond {
			if !isMatched(a, mur) {
				return false, nil
			}
		}
		return true, nil
	})
}

type MurWaitCondition func(a *HostAwaitility, mur *toolchainv1alpha1.MasterUserRecord) bool

func UntilHasStatusCondition(conditions ...toolchainv1alpha1.Condition) MurWaitCondition {
	return func(a *HostAwaitility, mur *toolchainv1alpha1.MasterUserRecord) bool {
		if test.ConditionsMatch(mur.Status.Conditions, conditions...) {
			a.T.Logf("status conditions match in MasterUserRecord '%s`", mur.Name)
			return true
		}
		a.T.Logf("waiting for correct status condition of MasterUserRecord '%s`", mur.Name)
		return false
	}
}

func UntilHasUserAccountStatus(expUaStatuses ...toolchainv1alpha1.UserAccountStatusEmbedded) MurWaitCondition {
	return func(a *HostAwaitility, mur *toolchainv1alpha1.MasterUserRecord) bool {
		if len(mur.Status.UserAccounts) != len(expUaStatuses) {
			a.T.Logf("waiting for corrent number of UserAccount statuses in MasterUserRecord '%s`", mur.Name)
			return false
		}
		for _, expUaStatus := range expUaStatuses {
			expUaStatus.SyncIndex = getUaSpecSyncIndex(mur, expUaStatus.TargetCluster)
			if !containsUserAccountStatus(mur.Status.UserAccounts, expUaStatus) {
				a.T.Logf("waiting for UserAccount status to be present in MasterUserRecord '%s`", mur.Name)
				return false
			}

		}
		a.T.Logf("all UserAccount statuses are present in MasterUserRecord '%s`", mur.Name)
		return true
	}
}

func getUaSpecSyncIndex(mur *toolchainv1alpha1.MasterUserRecord, targetCluster string) string {
	for _, ua := range mur.Spec.UserAccounts {
		if ua.TargetCluster == targetCluster {
			return ua.SyncIndex
		}
	}
	return ""
}

func containsUserAccountStatus(uaStatuses []toolchainv1alpha1.UserAccountStatusEmbedded, uaStatus toolchainv1alpha1.UserAccountStatusEmbedded) bool {
	for _, status := range uaStatuses {
		if reflect.DeepEqual(uaStatus, status) {
			return true
		}
	}
	return false
}

func (a *MemberAwaitility) WaitForUserAccount(name string, expSpec toolchainv1alpha1.UserAccountSpec, conditions ...toolchainv1alpha1.Condition) error {
	return wait.Poll(e2e.RetryInterval, e2e.Timeout, func() (done bool, err error) {
		ua := &toolchainv1alpha1.UserAccount{}
		if err := a.Client.Get(context.TODO(), types.NamespacedName{Namespace: a.Ns, Name: name}, ua); err != nil {
			if errors.IsNotFound(err) {
				a.T.Logf("waiting for availability of useraccount '%s'", name)
				return false, nil
			}
			return false, err
		}
		if reflect.DeepEqual(ua.Spec, expSpec) &&
			test.ConditionsMatch(ua.Status.Conditions, conditions...) {
			a.T.Logf("found UserAccount '%s' with expected spec and status condition", name)
			return true, nil
		}
		a.T.Logf("waiting for UserAccount '%s' with expected spec and status condition", name)
		return false, nil
	})
}

func (a *MemberAwaitility) WaitForDeletedUserAccount(name string) error {
	return wait.Poll(e2e.RetryInterval, e2e.Timeout, func() (done bool, err error) {
		ua := &toolchainv1alpha1.UserAccount{}
		if err := a.Client.Get(context.TODO(), types.NamespacedName{Namespace: a.Ns, Name: name}, ua); err != nil {
			if errors.IsNotFound(err) {
				a.T.Logf("UserAccount is checked as deleted '%s'", name)
				return true, nil
			}
			return false, err
		}
		a.T.Logf("waiting until UserAccount is deleted '%s'", name)
		return false, nil
	})
}

func (a *MemberAwaitility) GetUserAccount(name string) *toolchainv1alpha1.UserAccount {
	ua := &toolchainv1alpha1.UserAccount{}
	err := a.Client.Get(context.TODO(), types.NamespacedName{Namespace: a.Ns, Name: name}, ua)
	require.NoError(a.T, err)
	return ua
}
