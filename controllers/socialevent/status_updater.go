package socialevent

import (
	"context"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	commonCondition "github.com/codeready-toolchain/toolchain-common/pkg/condition"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

type StatusUpdater struct {
	Client client.Client
}

func (u *StatusUpdater) updateStatusConditions(event *toolchainv1alpha1.SocialEvent, newConditions ...toolchainv1alpha1.Condition) error {
	var updated bool
	event.Status.Conditions, updated = commonCondition.AddOrUpdateStatusConditions(event.Status.Conditions, newConditions...)
	if !updated {
		// Nothing changed
		return nil
	}
	return u.Client.Status().Update(context.TODO(), event)
}
