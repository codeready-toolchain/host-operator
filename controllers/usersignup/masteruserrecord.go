package usersignup

import (
	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func migrateOrFixMurIfNecessary(mur *toolchainv1alpha1.MasterUserRecord, defaultTier *toolchainv1alpha1.UserTier, userSignup *toolchainv1alpha1.UserSignup) bool {
	changed := false

	// TODO remove this after all users migrated to new SSO Provider client that does not modify the original subject
	if mur.Spec.OriginalSub != userSignup.Spec.OriginalSub {
		mur.Spec.OriginalSub = userSignup.Spec.OriginalSub
		changed = true
	}

	// set tierName to default if not set
	if mur.Spec.TierName == "" {
		mur.Spec.TierName = defaultTier.Name
		changed = true
	}

	return changed
}

func newMasterUserRecord(userSignup *toolchainv1alpha1.UserSignup, targetCluster string, userTierName string, compliantUserName string) *toolchainv1alpha1.MasterUserRecord {
	userAccounts := []toolchainv1alpha1.UserAccountEmbedded{
		{
			TargetCluster: targetCluster,
		},
	}
	labels := map[string]string{
		toolchainv1alpha1.MasterUserRecordOwnerLabelKey: userSignup.Name,
	}
	annotations := map[string]string{
		toolchainv1alpha1.MasterUserRecordEmailAnnotationKey: userSignup.Annotations[toolchainv1alpha1.UserSignupUserEmailAnnotationKey],
	}

	mur := &toolchainv1alpha1.MasterUserRecord{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   userSignup.Namespace,
			Name:        compliantUserName,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: toolchainv1alpha1.MasterUserRecordSpec{
			UserAccounts: userAccounts,
			UserID:       userSignup.Spec.Userid,
			OriginalSub:  userSignup.Spec.OriginalSub,
			TierName:     userTierName,
		},
	}
	return mur
}
