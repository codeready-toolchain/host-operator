package masteruserrecord

import (
	"context"
	"fmt"
	"reflect"
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/controllers/toolchainconfig"
	"github.com/codeready-toolchain/host-operator/pkg/cluster"
	"github.com/codeready-toolchain/host-operator/pkg/templates/notificationtemplates"
	"github.com/codeready-toolchain/toolchain-common/pkg/condition"
	notify "github.com/codeready-toolchain/toolchain-common/pkg/notification"
	"github.com/redhat-cop/operator-utils/pkg/util"

	"github.com/go-logr/logr"
	errs "github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type Synchronizer struct {
	hostClient    runtimeclient.Client
	memberCluster cluster.Cluster
	memberUserAcc *toolchainv1alpha1.UserAccount
	record        *toolchainv1alpha1.MasterUserRecord
	scheme        *runtime.Scheme
	logger        logr.Logger
}

// synchronizeSpec synchronizes the useraccount in the MasterUserRecord with the corresponding UserAccount on the member cluster.
func (s *Synchronizer) synchronizeSpec(ctx context.Context) (bool, error) {

	if !s.isSynchronized() {
		s.logger.Info("synchronizing specs")
		if err := updateStatusConditions(ctx, s.hostClient, s.record, toBeNotReady(toolchainv1alpha1.MasterUserRecordUpdatingReason, "")); err != nil {
			return false, err
		}
		s.memberUserAcc.Spec.Disabled = s.record.Spec.Disabled
		s.memberUserAcc.Spec.PropagatedClaims = s.record.Spec.PropagatedClaims

		// In addition to synchronizing the spec, ensure both the tier label and email annotation are set
		// The tier label is used for an appstudio workaround in the member operator, see https://github.com/codeready-toolchain/member-operator/pull/333
		if s.memberUserAcc.Labels == nil {
			s.memberUserAcc.Labels = map[string]string{}
		}
		s.memberUserAcc.Labels[toolchainv1alpha1.TierLabelKey] = s.record.Spec.TierName
		if s.memberUserAcc.Annotations == nil {
			s.memberUserAcc.Annotations = map[string]string{}
		}

		err := s.memberCluster.Client.Update(ctx, s.memberUserAcc)
		if err != nil {
			s.logger.Error(err, "synchronizing failed")
			return false, err
		}
		s.logger.Info("synchronizing complete")
		return true, nil
	}
	return false, nil
}

func (s *Synchronizer) isSynchronized() bool {
	return s.memberUserAcc.Spec.Disabled == s.record.Spec.Disabled &&
		s.memberUserAcc.Spec.PropagatedClaims == s.record.Spec.PropagatedClaims &&
		s.memberUserAcc.Labels != nil && s.memberUserAcc.Labels[toolchainv1alpha1.TierLabelKey] == s.record.Spec.TierName
}

func (s *Synchronizer) removeAccountFromStatus(ctx context.Context) error {
	for i := range s.record.Status.UserAccounts {
		if s.record.Status.UserAccounts[i].Cluster.Name == s.memberCluster.Name {
			s.record.Status.UserAccounts = append(s.record.Status.UserAccounts[:i], s.record.Status.UserAccounts[i+1:]...)
			if !util.IsBeingDeleted(s.record) {
				if _, err := alignReadiness(ctx, s.scheme, s.hostClient, s.record); err != nil {
					return err
				}
			}
			s.logger.Info("updating MUR status")
			return s.hostClient.Status().Update(ctx, s.record)
		}
	}
	return nil
}

func (s *Synchronizer) synchronizeStatus(ctx context.Context) error {
	recordStatusUserAcc, index := getUserAccountStatus(s.memberCluster.Name, s.record)

	expectedRecordStatus := *(&recordStatusUserAcc).DeepCopy()
	expectedRecordStatus.UserAccountStatus = s.memberUserAcc.Status
	expectedRecordStatus, err := s.withClusterDetails(ctx, expectedRecordStatus)
	if err != nil {
		return err
	}
	if index < 0 || !reflect.DeepEqual(recordStatusUserAcc, expectedRecordStatus) {
		// when record should update status
		recordStatusUserAcc = expectedRecordStatus
		var originalStatusUserAcc toolchainv1alpha1.UserAccountStatusEmbedded
		if index < 0 {
			s.record.Status.UserAccounts = append(s.record.Status.UserAccounts, recordStatusUserAcc)
		} else {
			originalStatusUserAcc = s.record.Status.UserAccounts[index]
			s.record.Status.UserAccounts[index] = recordStatusUserAcc
		}

		ready, err := alignReadiness(ctx, s.scheme, s.hostClient, s.record)
		if err != nil {
			return err
		}

		if !ready {
			s.alignDisabled()
		}

		err = s.hostClient.Status().Update(ctx, s.record)
		if err != nil {
			// if there is an error during status update then we "roll back" all the status changes we did above
			// (don't add new user account status if it was added or don't change it if it's updated)
			if index < 0 {
				s.record.Status.UserAccounts = s.record.Status.UserAccounts[:len(s.record.Status.UserAccounts)-1]
			} else {
				s.record.Status.UserAccounts[index] = originalStatusUserAcc
			}
		}
		return err
	}

	// Align readiness even if the user account statuses were not changed.
	// We need to do it to cleanup outdated errors (for example if the target cluster was unavailable) if any
	_, err = alignReadiness(ctx, s.scheme, s.hostClient, s.record)
	if err != nil {
		return err
	}
	s.logger.Info("updating MUR status")
	return s.hostClient.Status().Update(ctx, s.record)
}

// withClusterDetails returns the given user account status with additional information about
// the target cluster such as API endpoint and Console URL if not set yet
func (s *Synchronizer) withClusterDetails(ctx context.Context, status toolchainv1alpha1.UserAccountStatusEmbedded) (toolchainv1alpha1.UserAccountStatusEmbedded, error) {
	if status.Cluster.Name != "" {
		toolchainStatus := &toolchainv1alpha1.ToolchainStatus{}
		if err := s.hostClient.Get(ctx, types.NamespacedName{Namespace: s.record.Namespace, Name: toolchainconfig.ToolchainStatusName}, toolchainStatus); err != nil {
			return status, errs.Wrapf(err, "unable to read ToolchainStatus resource")
		}

		for _, memberStatus := range toolchainStatus.Status.Members {
			if memberStatus.ClusterName == status.Cluster.Name {
				if memberStatus.MemberStatus.Routes == nil {
					return status, fmt.Errorf("routes are not set in ToolchainStatus resource")
				}
				if condition.IsNotTrue(memberStatus.MemberStatus.Routes.Conditions, toolchainv1alpha1.ConditionReady) {
					ready, _ := condition.FindConditionByType(memberStatus.MemberStatus.Routes.Conditions, toolchainv1alpha1.ConditionReady)
					return status, fmt.Errorf("routes are not properly set in ToolchainStatus - the reason is: `%s` with message: `%s`", ready.Reason, ready.Message)
				}
				return status, nil
			}
		}
		return status, fmt.Errorf("the appropriate status of the member cluster '%s' wasn't found in ToolchainCluster CR - the present member statuses: %v",
			status.Cluster.Name, toolchainStatus.Status.Members)
	}
	return status, nil
}

// alignDisabled updates the status to Disabled if all all the embedded UserAccounts have Disabled status
func (s *Synchronizer) alignDisabled() {
	for _, uaStatus := range s.record.Status.UserAccounts {
		if !condition.HasConditionReason(uaStatus.Conditions, toolchainv1alpha1.ConditionReady, toolchainv1alpha1.UserAccountDisabledReason) {
			return
		}
	}

	if len(s.record.Status.UserAccounts) > 0 {
		s.record.Status.Conditions, _ = condition.AddOrUpdateStatusConditions(s.record.Status.Conditions, toBeDisabled())
	}
}

// alignReadiness updates the status to Provisioned and returns true if all the embedded UserAccounts are ready
func alignReadiness(ctx context.Context, scheme *runtime.Scheme, hostClient runtimeclient.Client, mur *toolchainv1alpha1.MasterUserRecord) (bool, error) {
	// Lookup the UserSignup
	for _, uaStatus := range mur.Status.UserAccounts {
		if !condition.IsTrue(uaStatus.Conditions, toolchainv1alpha1.ConditionReady) {
			return false, nil
		}
	}

	// If the creation of the default space wasn't skipped and there is still no UserAccount in the status, then let's wait before making it ready
	if mur.Annotations[toolchainv1alpha1.SkipAutoCreateSpaceAnnotationKey] != "true" && len(mur.Status.UserAccounts) == 0 {
		// We can mark the MUR ready with no UserAccount status set only when the MUR was already provisioned before - this may mean that
		// all UserAccounts were removed, which is a valid case and should be OK.
		// In other words, let's set unready "Provisioning" condition and exit the function if the provisioned time is not set for the MUR yet
		if mur.Status.ProvisionedTime == nil {
			mur.Status.Conditions, _ = condition.AddOrUpdateStatusConditions(mur.Status.Conditions, toBeNotReady(toolchainv1alpha1.MasterUserRecordProvisioningReason, ""))
			return false, nil
		}
	}

	mur.Status.Conditions, _ = condition.AddOrUpdateStatusConditions(mur.Status.Conditions, toBeProvisioned())

	// set ProvisionedTime if it is not already set, this information will be used for things like the start time for automatic user deactivation.
	// the MUR status can change from provisioned to something else and back to provisioned but the time should only be set the first time.
	if mur.Status.ProvisionedTime == nil {
		mur.Status.ProvisionedTime = &metav1.Time{Time: time.Now()}
	}

	if condition.IsNotTrue(mur.Status.Conditions, toolchainv1alpha1.MasterUserRecordUserProvisionedNotificationCreated) {
		labels := map[string]string{
			toolchainv1alpha1.NotificationUserNameLabelKey: mur.Name,
			toolchainv1alpha1.NotificationTypeLabelKey:     toolchainv1alpha1.NotificationTypeProvisioned,
		}
		opts := runtimeclient.MatchingLabels(labels)
		notificationList := &toolchainv1alpha1.NotificationList{}
		if err := hostClient.List(ctx, notificationList, opts); err != nil {
			return false, err
		}
		// if there is no existing notification with these labels
		if len(notificationList.Items) == 0 {

			config, err := toolchainconfig.GetToolchainConfig(hostClient)
			if err != nil {
				return false, errs.Wrapf(err, "unable to get ToolchainConfig")
			}

			keysAndVals := map[string]string{
				toolchainconfig.NotificationContextRegistrationURLKey: config.RegistrationService().RegistrationServiceURL(),
			}

			// Lookup the UserSignup
			userSignup := &toolchainv1alpha1.UserSignup{}
			err = hostClient.Get(ctx, types.NamespacedName{
				Namespace: mur.Namespace,
				Name:      mur.Labels[toolchainv1alpha1.MasterUserRecordOwnerLabelKey],
			}, userSignup)
			if err != nil {
				return false, err
			}

			_, err = notify.NewNotificationBuilder(hostClient, mur.Namespace).
				WithNotificationType(toolchainv1alpha1.NotificationTypeProvisioned).
				WithControllerReference(mur, scheme).
				WithTemplate(notificationtemplates.UserProvisionedTemplateName).
				WithUserContext(userSignup).
				WithKeysAndValues(keysAndVals).
				Create(userSignup.Annotations[toolchainv1alpha1.UserSignupUserEmailAnnotationKey])

			if err != nil {
				return false, err
			}
		} else {
			log.FromContext(ctx).Info(fmt.Sprintf("The %s notification for user %s was not created because it already exists: %v",
				toolchainv1alpha1.NotificationTypeProvisioned, mur.Name, notificationList.Items[0]))
		}
		mur.Status.Conditions, _ = condition.AddOrUpdateStatusConditions(mur.Status.Conditions, toBeProvisionedNotificationCreated())
	}

	return true, nil
}

func getUserAccountStatus(clusterName string, record *toolchainv1alpha1.MasterUserRecord) (toolchainv1alpha1.UserAccountStatusEmbedded, int) {
	for i, account := range record.Status.UserAccounts {
		if account.Cluster.Name == clusterName {
			return account, i
		}
	}
	return toolchainv1alpha1.UserAccountStatusEmbedded{
		Cluster: toolchainv1alpha1.Cluster{
			Name: clusterName,
		},
	}, -1
}
