package masteruserrecord

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"reflect"
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/configuration"
	"github.com/codeready-toolchain/host-operator/pkg/templates/notificationtemplates"
	"github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	"github.com/codeready-toolchain/toolchain-common/pkg/condition"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/types"

	"github.com/go-logr/logr"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// consoleClient to be used to test connection to a public Web Console
var consoleClient = &http.Client{ //nolint:deadcode,unused,varcheck
	Timeout: time.Duration(1 * time.Second),
	Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	},
}

type Synchronizer struct {
	hostClient        client.Client
	memberCluster     *cluster.CachedToolchainCluster
	memberUserAcc     *toolchainv1alpha1.UserAccount
	recordSpecUserAcc toolchainv1alpha1.UserAccountEmbedded
	record            *toolchainv1alpha1.MasterUserRecord
	scheme            *runtime.Scheme
	logger            logr.Logger
	config            *configuration.Config
}

// synchronizeSpec synhronizes the useraccount in the MasterUserRecord with the corresponding UserAccount on the member cluster.
func (s *Synchronizer) synchronizeSpec() error {

	if !s.isSynchronized() {
		s.logger.Info("synchronizing specs", "UserAccountSpecBase", s.recordSpecUserAcc.Spec.UserAccountSpecBase)
		if err := updateStatusConditions(s.logger, s.hostClient, s.record, toBeNotReady(toolchainv1alpha1.MasterUserRecordUpdatingReason, "")); err != nil {
			return err
		}
		s.memberUserAcc.Spec.UserAccountSpecBase = s.recordSpecUserAcc.Spec.UserAccountSpecBase
		s.memberUserAcc.Spec.Disabled = s.record.Spec.Disabled
		s.memberUserAcc.Spec.UserID = s.record.Spec.UserID

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
		s.memberUserAcc.Spec.UserID == s.record.Spec.UserID
}

func (s *Synchronizer) synchronizeStatus() error {
	recordStatusUserAcc, index := getUserAccountStatus(s.recordSpecUserAcc.TargetCluster, s.record)
	if index < 0 || s.recordSpecUserAcc.SyncIndex != recordStatusUserAcc.SyncIndex {
		// when record should update status
		recordStatusUserAcc.SyncIndex = s.recordSpecUserAcc.SyncIndex
		recordStatusUserAcc.UserAccountStatus = s.memberUserAcc.Status
		recordStatusUserAcc, err := s.withClusterDetails(recordStatusUserAcc)
		if err != nil {
			return err
		}
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
	_, err := s.alignReadiness()
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
		if err := s.hostClient.Get(context.TODO(), types.NamespacedName{Namespace: s.record.Namespace, Name: configuration.ToolchainStatusName}, toolchainStatus); err != nil {
			return status, errors.Wrapf(err, "unable to read ToolchainStatus resource")
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
			notification := &toolchainv1alpha1.Notification{
				ObjectMeta: v1.ObjectMeta{
					GenerateName: fmt.Sprintf("%s-%s-", s.record.Name, toolchainv1alpha1.NotificationTypeProvisioned),
					Namespace:    s.record.Namespace,
					Labels:       labels,
				},
				Spec: toolchainv1alpha1.NotificationSpec{
					// The UserID property actually refers to the UserSignup resource name.  This will be renamed
					// (or removed) in a future PR
					UserID:   s.record.Labels[toolchainv1alpha1.MasterUserRecordOwnerLabelKey],
					Template: notificationtemplates.UserProvisioned.Name,
				},
			}

			err := controllerutil.SetControllerReference(s.record, notification, s.scheme)
			if err != nil {
				return false, err
			}

			if err := s.hostClient.Create(context.TODO(), notification); err != nil {
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
