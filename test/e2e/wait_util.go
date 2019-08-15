package e2e

import (
	"context"
	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
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

func (a *HostAwaitility) waitForMasterUserRecord(name string) error {
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

func (a *HostAwaitility) waitForMurConditions(name string, conditions ...toolchainv1alpha1.Condition) error {
	return wait.Poll(e2e.RetryInterval, e2e.Timeout, func() (done bool, err error) {
		mur := &toolchainv1alpha1.MasterUserRecord{}
		if err := a.Client.Get(context.TODO(), types.NamespacedName{Namespace: a.Ns, Name: name}, mur); err != nil {
			if errors.IsNotFound(err) {
				a.T.Logf("waiting for availability of MasterUserRecord '%s'", name)
				return false, nil
			}
			return false, err
		}
		if test.ConditionsMatch(mur.Status.Conditions, conditions...) {
			a.T.Log("conditions match")
			return true, nil
		}
		a.T.Logf("waiting for correct condition of MasterUserRecord '%s", name)
		return false, nil
	})
}

func (a *MemberAwaitility) waitForUserAccount(name string, expSpec toolchainv1alpha1.UserAccountSpec) error {
	return wait.Poll(e2e.RetryInterval, e2e.Timeout, func() (done bool, err error) {
		ua := &toolchainv1alpha1.UserAccount{}
		if err := a.Client.Get(context.TODO(), types.NamespacedName{Namespace: a.Ns, Name: name}, ua); err != nil {
			if errors.IsNotFound(err) {
				a.T.Logf("waiting for availability of useraccount '%s'", name)
				return false, nil
			}
			return false, err
		}
		if reflect.DeepEqual(ua.Spec, expSpec) {
			a.T.Logf("found UserAccount '%s' with expected spec", name)
			return true, nil
		}
		return false, nil
	})
}
