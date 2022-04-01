package masteruserrecord

import (
	"context"
	"fmt"
	"reflect"
	"time"

	notify "github.com/codeready-toolchain/host-operator/controllers/notification"
	"github.com/codeready-toolchain/host-operator/controllers/toolchainconfig"
	"github.com/codeready-toolchain/host-operator/pkg/cluster"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/templates/notificationtemplates"
	"github.com/codeready-toolchain/toolchain-common/pkg/condition"
	errs "github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/types"

	"github.com/go-logr/logr"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Synchronizer struct {
	hostClient        client.Client
	memberCluster     cluster.Cluster
	memberUserAcc     *toolchainv1alpha1.UserAccount
	recordSpecUserAcc toolchainv1alpha1.UserAccountEmbedded
	record            *toolchainv1alpha1.MasterUserRecord
	scheme            *runtime.Scheme
	logger            logr.Logger
}

// synchronizeSpec synchronizes the useraccount in the MasterUserRecord with the corresponding UserAccount on the member cluster.
func (s *Synchronizer) synchronizeSpec() error {

	if !s.isSynchronized() {
		s.logger.Info("synchronizing specs", "UserAccountSpecBase", s.recordSpecUserAcc.Spec.UserAccountSpecBase)
		if err := updateStatusConditions(s.logger, s.hostClient, s.record, toBeNotReady(toolchainv1alpha1.MasterUserRecordUpdatingReason, "")); err != nil {
			return err
		}
		s.memberUserAcc.Spec.UserAccountSpecBase = s.recordSpecUserAcc.Spec.UserAccountSpecBase
		s.memberUserAcc.Spec.Disabled = s.record.Spec.Disabled
		s.memberUserAcc.Spec.UserID = s.record.Spec.UserID
		s.memberUserAcc.Spec.OriginalSub = s.record.Spec.OriginalSub

		// In addition to synchronizing the spec, ensure both the tier label and email annotation are set
		// The tier label is used for an appstudio workaround in the member operator, see https://github.com/codeready-toolchain/member-operator/pull/333
		if s.memberUserAcc.Labels == nil {
			s.memberUserAcc.Labels = map[string]string{}
		}
		s.memberUserAcc.Labels[toolchainv1alpha1.TierLabelKey] = s.record.Spec.TierName
		if s.memberUserAcc.Annotations == nil {
			s.memberUserAcc.Annotations = map[string]string{}
		}
		s.memberUserAcc.Annotations[toolchainv1alpha1.UserEmailAnnotationKey] = s.record.Annotations[toolchainv1alpha1.MasterUserRecordEmailAnnotationKey]

		err := s.memberCluster.Client.Update(context.TODO(), s.memberUserAcc)
		if err != nil {
			s.logger.Error(err, "synchronizing failed")
			return err
		}
		s.logger.Info("synchronizing complete")
	}
	return nil
}

func (s *Synchronizer) isSynchronized() bool {
	return reflect.DeepEqual(s.memberUserAcc.Spec.UserAccountSpecBase, s.recordSpecUserAcc.Spec.UserAccountSpecBase) &&
		s.memberUserAcc.Spec.Disabled == s.record.Spec.Disabled &&
		s.memberUserAcc.Spec.UserID == s.record.Spec.UserID &&
		s.memberUserAcc.Labels != nil && s.memberUserAcc.Labels[toolchainv1alpha1.TierLabelKey] == s.record.Spec.TierName &&
		s.memberUserAcc.Annotations != nil && s.memberUserAcc.Annotations[toolchainv1alpha1.UserEmailAnnotationKey] == s.record.Annotations[toolchainv1alpha1.MasterUserRecordEmailAnnotationKey]
}

func (s *Synchronizer) synchronizeStatus() error {
	recordStatusUserAcc, index := getUserAccountStatus(s.recordSpecUserAcc.TargetCluster, s.record)

	expectedRecordStatus := *(&recordStatusUserAcc).DeepCopy()
	expectedRecordStatus.UserAccountStatus = s.memberUserAcc.Status
	expectedRecordStatus, err := s.withClusterDetails(expectedRecordStatus)
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

		ready, err := s.alignReadiness()
		if err != nil {
			return err
		}

		if !ready {
			s.alignDisabled()
		}

		err = s.hostClient.Status().Update(context.TODO(), s.record)
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
	_, err = s.alignReadiness()
	if err != nil {
		return err
	}
	s.logger.Info("updating MUR status")
	return s.hostClient.Status().Update(context.TODO(), s.record)
}

// withClusterDetails returns the given user account status with additional information about
// the target cluster such as API endpoint and Console URL if not set yet
func (s *Synchronizer) withClusterDetails(status toolchainv1alpha1.UserAccountStatusEmbedded) (toolchainv1alpha1.UserAccountStatusEmbedded, error) {
	if status.Cluster.Name != "" {
		toolchainStatus := &toolchainv1alpha1.ToolchainStatus{}
		if err := s.hostClient.Get(context.TODO(), types.NamespacedName{Namespace: s.record.Namespace, Name: toolchainconfig.ToolchainStatusName}, toolchainStatus); err != nil {
			return status, errs.Wrapf(err, "unable to read ToolchainStatus resource")
		}

		if status.Cluster.APIEndpoint == "" {
			status.Cluster.APIEndpoint = s.memberCluster.APIEndpoint
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
				status.Cluster.ConsoleURL = memberStatus.MemberStatus.Routes.ConsoleURL
				status.Cluster.CheDashboardURL = memberStatus.MemberStatus.Routes.CheDashboardURL
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
func (s *Synchronizer) alignReadiness() (bool, error) {
	for _, uaStatus := range s.record.Status.UserAccounts {
		if !condition.IsTrue(uaStatus.Conditions, toolchainv1alpha1.ConditionReady) {
			return false, nil
		}
	}

	s.record.Status.Conditions, _ = condition.AddOrUpdateStatusConditions(s.record.Status.Conditions, toBeProvisioned())

	// set ProvisionedTime if it is not already set, this information will be used for things like the start time for automatic user deactivation.
	// the MUR status can change from provisioned to something else and back to provisioned but the time should only be set the first time.
	if s.record.Status.ProvisionedTime == nil {
		s.record.Status.ProvisionedTime = &v1.Time{Time: time.Now()}
	}

	if condition.IsNotTrue(s.record.Status.Conditions, toolchainv1alpha1.MasterUserRecordUserProvisionedNotificationCreated) {
		labels := map[string]string{
			toolchainv1alpha1.NotificationUserNameLabelKey: s.record.Name,
			toolchainv1alpha1.NotificationTypeLabelKey:     toolchainv1alpha1.NotificationTypeProvisioned,
		}
		opts := client.MatchingLabels(labels)
		notificationList := &toolchainv1alpha1.NotificationList{}
		if err := s.hostClient.List(context.TODO(), notificationList, opts); err != nil {
			return false, err
		}
		// if there is no existing notification with these labels
		if len(notificationList.Items) == 0 {

			config, err := toolchainconfig.GetToolchainConfig(s.hostClient)
			if err != nil {
				return false, errs.Wrapf(err, "unable to get ToolchainConfig")
			}

			keysAndVals := map[string]string{
				toolchainconfig.NotificationContextRegistrationURLKey: config.RegistrationService().RegistrationServiceURL(),
			}

			// Lookup the UserSignup
			userSignup := &toolchainv1alpha1.UserSignup{}
			err = s.hostClient.Get(context.TODO(), types.NamespacedName{
				Namespace: s.record.Namespace,
				Name:      s.record.Labels[toolchainv1alpha1.MasterUserRecordOwnerLabelKey],
			}, userSignup)
			if err != nil {
				return false, err
			}

			_, err = notify.NewNotificationBuilder(s.hostClient, s.record.Namespace).
				WithNotificationType(toolchainv1alpha1.NotificationTypeProvisioned).
				WithControllerReference(s.record, s.scheme).
				WithTemplate(notificationtemplates.UserProvisioned.Name).
				WithUserContext(userSignup).
				WithKeysAndValues(keysAndVals).
				Create(userSignup.Annotations[toolchainv1alpha1.UserSignupUserEmailAnnotationKey])

			if err != nil {
				return false, err
			}
		} else {
			s.logger.Info(fmt.Sprintf("The %s notification for user %s was not created because it already exists: %v",
				toolchainv1alpha1.NotificationTypeProvisioned, s.record.Name, notificationList.Items[0]))
		}
		s.record.Status.Conditions, _ = condition.AddOrUpdateStatusConditions(s.record.Status.Conditions, toBeProvisionedNotificationCreated())
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
