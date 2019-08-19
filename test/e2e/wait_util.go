package e2e

import (
	"context"
	"fmt"
	"github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
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
	var mismatchErr error
	err := wait.Poll(retryInterval, timeout, func() (done bool, err error) {
		mismatchErr = nil
		userSignup := &v1alpha1.UserSignup{}
		if err := client.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: name}, userSignup); err != nil {
			if errors.IsNotFound(err) {
				t.Logf("waiting for availability of UserSignup '%s'", name)
				return false, nil
			}
			return false, err
		}
		mismatchErr = ConditionsMatch(userSignup.Status.Conditions, conditions...)
		if mismatchErr == nil {
			t.Log("conditions match")
			return true, nil
		}
		t.Logf("waiting for [%d] conditions to match...", len(conditions))
		return false, nil
	})

	if mismatchErr != nil {
		return mismatchErr
	}
	return err
}

func ConditionsMatch(actual []v1alpha1.Condition, expected ...v1alpha1.Condition) error {
	if len(expected) != len(actual) {
		return NewConditionMismatchError(fmt.Sprintf("Conditions length [%d] does not match expected length [%d]",
			len(actual), len(expected)))
	}
	for _, c := range expected {
		if !ContainsCondition(actual, c) {
			return NewConditionMismatchError(fmt.Sprintf("Conditions do not contain expected condition [%s]", c.Type))
		}
	}
	for _, c := range actual {
		if !ContainsCondition(expected, c) {
			return NewConditionMismatchError(fmt.Sprintf("Expected condition [%s] not found in conditions", c.Type))
		}
	}
	return nil
}

func ContainsCondition(conditions []v1alpha1.Condition, contains v1alpha1.Condition) bool {
	for _, c := range conditions {
		if c.Type == contains.Type {
			return contains.Status == c.Status && contains.Reason == c.Reason && contains.Message == c.Message
		}
	}
	return false
}