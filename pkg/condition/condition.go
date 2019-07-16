package condition

import (
	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	v1 "k8s.io/api/core/v1"
)

func IsConditionTrue(conditions []toolchainv1alpha1.Condition, conditionType toolchainv1alpha1.ConditionType) bool {
	for _, cond := range(conditions) {
		if cond.Type == conditionType {
			return cond.Status == v1.ConditionTrue
		}
	}
	return false
}

func ReadCondition(conditions []toolchainv1alpha1.Condition, conditionType toolchainv1alpha1.ConditionType) (*toolchainv1alpha1.Condition) {
	for _, cond := range(conditions) {
		if cond.Type == conditionType {
			return &cond
		}
	}

	return nil
}