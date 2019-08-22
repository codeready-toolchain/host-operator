package e2e

import (
	"context"
	"fmt"
	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/stretchr/testify/require"
	"reflect"
	"github.com/codeready-toolchain/toolchain-common/pkg/test/e2e"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
)

type ConditionMismatchError struct {
	message string
}

func (err *ConditionMismatchError) Error() string {
	return err.message
}

func NewConditionMismatchError(msg string) *ConditionMismatchError {
	return &ConditionMismatchError{
		message: msg,
	}
}

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

func (a *HostAwaitility) waitForUserSignup(name string) error {
	return wait.Poll(e2e.RetryInterval, e2e.Timeout, func() (done bool, err error) {
		userSignup := &toolchainv1alpha1.UserSignup{}
		if err := a.Client.Get(context.TODO(), types.NamespacedName{Name: name, Namespace: a.Ns}, userSignup); err != nil {
			if errors.IsNotFound(err) {
				a.T.Logf("waiting for availability of UserSignup '%s'", name)
				return false, nil
			}
			return false, err
		}
		a.T.Logf("found UserSignup '%s'", name)
		return true, nil
	})
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
		a.T.Logf("found MasterUserRecord '%s'", name)
		return true, nil
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

func (a *HostAwaitility) waitForUserSignupStatusConditions(name string, conditions ...toolchainv1alpha1.Condition) error {
	var mismatchErr error
	err := wait.Poll(e2e.RetryInterval, e2e.Timeout, func() (done bool, err error) {
		mismatchErr = nil
		userSignup := &toolchainv1alpha1.UserSignup{}
		if err := a.Client.Get(context.TODO(), types.NamespacedName{Namespace: a.Ns, Name: name}, userSignup); err != nil {
			if errors.IsNotFound(err) {
				a.T.Logf("waiting for availability of UserSignup '%s'", name)
				return false, nil
			}
			return false, err
		}

		mismatchErr = ConditionsMatch(userSignup.Status.Conditions, conditions...)
		if mismatchErr == nil {
			a.T.Log("conditions match")
			return true, nil
		}
		a.T.Logf("waiting for [%d] conditions to match...", len(conditions))
		return false, nil
	})

	if mismatchErr != nil {
		return mismatchErr
	}
	return err
}

func ConditionsMatch(actual []toolchainv1alpha1.Condition, expected ...toolchainv1alpha1.Condition) error {
	if len(expected) != len(actual) {
		return NewConditionMismatchError(fmt.Sprintf("Conditions length [%d] does not match expected length [%d]",
			len(actual), len(expected)))
	}
	for _, c := range expected {
		if !test.ContainsCondition(actual, c) {
			return NewConditionMismatchError(fmt.Sprintf("Conditions do not contain expected condition [%s]", c.Type))
		}
	}
	for _, c := range actual {
		if !test.ContainsCondition(expected, c) {
			return NewConditionMismatchError(fmt.Sprintf("Expected condition [%s] not found in conditions", c.Type))
		}
	}
	return nil
}

func (a *MemberAwaitility) GetUserAccount(name string) *toolchainv1alpha1.UserAccount {
	ua := &toolchainv1alpha1.UserAccount{}
	err := a.Client.Get(context.TODO(), types.NamespacedName{Namespace: a.Ns, Name: name}, ua)
	require.NoError(a.T, err)
	return ua
}
