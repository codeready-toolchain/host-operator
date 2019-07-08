package masteruserrecord

import (
	"context"
	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Synchronizer struct {
	hostClient    client.Client
	memberClient  client.Client
	memberUserAcc *toolchainv1alpha1.UserAccount
	recordUserAcc toolchainv1alpha1.UserAccountEmbedded
	record        *toolchainv1alpha1.MasterUserRecord
}

func (s *Synchronizer) synchronizeSpec() error {
	if !reflect.DeepEqual(s.memberUserAcc.Spec, s.recordUserAcc.Spec) { // TODO maybe do not use reflect :-)
		// when UserAccount spec in record is updated - is not same as in member
		s.memberUserAcc.Spec = s.recordUserAcc.Spec
		if err := updateStatus(s.hostClient, s.record, "updating", ""); err != nil {
			return err
		}
		err := s.memberClient.Update(context.TODO(), s.memberUserAcc)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Synchronizer) synchronizeStatus() error {
	recAccStatus, index := getUserAccountStatus(s.recordUserAcc.TargetCluster, s.record)
	if index < 0 || s.recordUserAcc.SyncIndex == recAccStatus.SyncIndex {
		// when record should update status
		recAccStatus.SyncIndex = s.recordUserAcc.SyncIndex
		recAccStatus.UserAccountStatus = s.memberUserAcc.Status
		if index < 0 {
			s.record.Status.UserAccounts = append(s.record.Status.UserAccounts, recAccStatus)
		} else {
			s.record.Status.UserAccounts[index] = recAccStatus
		}

		s.checkStatus("provisioning", "provisioned")
		s.checkStatus("updating", "provisioned")

		return s.hostClient.Satus().Update(context.TODO(), s.record)
	}
	return nil
}

// checkStatus checks if all embedded SAs has finished the ongoing job (has provisioned/updated)
func (s *Synchronizer) checkStatus(currentStatus, followingStatus string) {
	if s.record.Status.Status == currentStatus {
		for _, ua := range s.record.Status.UserAccounts {
			if string(ua.Status) != followingStatus {
				return
			}
		}
		s.record.Status.Status = followingStatus
	}
}

func getUserAccountStatus(clusterName string, record *toolchainv1alpha1.MasterUserRecord) (toolchainv1alpha1.UserAccountStatusEmbedded, int) {
	for i, account := range record.Status.UserAccounts {
		if account.TargetCluster == clusterName {
			return account, i
		}
	}
	return toolchainv1alpha1.UserAccountStatusEmbedded{
		TargetCluster: clusterName,
	}, -1
}
