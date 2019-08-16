package e2e

import (
	"context"
	"github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"testing"
	"time"
)

const (
	operatorRetryInterval = time.Second * 5
	operatorTimeout       = time.Second * 60
	retryInterval         = time.Millisecond * 100
	timeout               = time.Second * 3
	cleanupRetryInterval  = time.Second * 1
	cleanupTimeout        = time.Second * 5
)

func waitForUserSignup(t *testing.T, client client.Client, namespace, name string) error {
	return wait.Poll(retryInterval, timeout, func() (done bool, err error) {
		userSignup := &v1alpha1.UserSignup{}
		if err := client.Get(context.TODO(), types.NamespacedName{Name: name, Namespace: namespace,}, userSignup); err != nil {
			if errors.IsNotFound(err) {
				t.Logf("waiting for availability of UserSignup '%s'", name)
				return false, nil
			}
			return false, err
		}
		if userSignup.Name != "" {
			t.Logf("found UserSignup '%s'", name)
			return true, nil
		}
		return false, nil
	})
}

func waitForMasterUserRecord(t *testing.T, client client.Client, namespace, name string) error {
	return wait.Poll(retryInterval, timeout, func() (done bool, err error) {
		mur := &v1alpha1.MasterUserRecord{}
		if err := client.Get(context.TODO(), types.NamespacedName{Name: name, Namespace: namespace,}, mur); err != nil {
			if errors.IsNotFound(err) {
				t.Logf("waiting for availability of MasterUserRecord '%s'", name)
				return false, nil
			}
			return false, err
		}
		if mur.Name != "" {
			t.Logf("found MasterUserRecord '%s'", name)
			return true, nil
		}
		return false, nil
	})
}

func waitForUserSignupStatusConditions(t *testing.T, client client.Client, namespace, name string, conditions ...v1alpha1.Condition) error {
	return wait.Poll(retryInterval, timeout, func() (done bool, err error) {
		userSignup := &v1alpha1.UserSignup{}
		if err := client.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: name}, userSignup); err != nil {
			if errors.IsNotFound(err) {
				t.Logf("waiting for availability of UserSignup '%s'", name)
				return false, nil
			}
			return false, err
		}
		if test.ConditionsMatch(userSignup.Status.Conditions, conditions...) {
			t.Log("conditions match")
			return true, nil
		}
		return false, nil
	})
}