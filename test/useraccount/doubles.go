package uatest

import (
	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/host-operator/test"
	"github.com/codeready-toolchain/toolchain-common/pkg/condition"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type UaModifier func(ua *toolchainv1alpha1.UserAccount)

func NewUserAccountFromMur(mur *toolchainv1alpha1.MasterUserRecord, modifiers ...UaModifier) *toolchainv1alpha1.UserAccount {
	ua := &toolchainv1alpha1.UserAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mur.Name,
			Namespace: test.MemberOperatorNs,
		},
		Spec: mur.Spec.UserAccounts[0].Spec,
	}
	ModifyUa(ua, modifiers...)
	return ua
}

func ModifyUa(ua *toolchainv1alpha1.UserAccount, modifiers ...UaModifier) {
	for _, modify := range modifiers {
		modify(ua)
	}
}

func StatusCondition(con toolchainv1alpha1.Condition) UaModifier {
	return func(ua *toolchainv1alpha1.UserAccount) {
		ua.Status.Conditions, _ = condition.AddOrUpdateStatusConditions(ua.Status.Conditions, con)
	}
}

func ResourceVersion(resVer string) UaModifier {
	return func(ua *toolchainv1alpha1.UserAccount) {
		ua.ResourceVersion = resVer
	}
}
