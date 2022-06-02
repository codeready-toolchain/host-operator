package usersignup

import (
	"strings"

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

	// ensure that the MUR does not have any tier hash labels since NSTemplateSet will be handled by Spaces
	for key := range mur.Labels {
		if strings.HasSuffix(key, "-tier-hash") {
			delete(mur.Labels, key)
			changed = true
		}
	}

	// migrate mur TierName from NSTemplateTier to UserTier
	switch mur.Spec.TierName {
	case "":
		mur.Spec.TierName = defaultTier.Name
		changed = true
	case "appstudio", "base", "base1ns", "baseextendedidling", "test":
		mur.Spec.TierName = "deactivate30"
		changed = true
	case "hackathon":
		mur.Spec.TierName = "deactivate80"
		changed = true
	case "baselarge":
		mur.Spec.TierName = "deactivate90"
		changed = true
	case "baseextended":
		mur.Spec.TierName = "deactivate180"
		changed = true
	case "advanced", "basedeactivationdisabled":
		mur.Spec.TierName = "nodeactivation"
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
