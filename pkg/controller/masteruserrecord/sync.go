package masteruserrecord

import (
	"context"
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"net/http"
	"reflect"
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	"github.com/codeready-toolchain/toolchain-common/pkg/condition"

	"github.com/go-logr/logr"
	routev1 "github.com/openshift/api/route/v1"
	"github.com/pkg/errors"
	kuberrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	consoleNamespace = "openshift-console"
	cheNamespace     = "toolchain-che"
)

// consoleClient to be used to test connection to a public Web Console
var consoleClient = &http.Client{
	Timeout: time.Duration(1 * time.Second),
	Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	},
}

type Synchronizer struct {
	hostClient        client.Client
	memberCluster     *cluster.FedCluster
	memberUserAcc     *toolchainv1alpha1.UserAccount
	recordSpecUserAcc toolchainv1alpha1.UserAccountEmbedded
	record            *toolchainv1alpha1.MasterUserRecord
	logger            logr.Logger
}

// synchronizeSpec synhronizes the useraccount in the MasterUserRecord with the corresponding UserAccount on the member cluster.
// returns `true` if the user account was updated on the member cluster (and the MasterUserRecord's status was updated accordingly), `false` otherwise
// returns `false, err` if an error occurred.
func (s *Synchronizer) synchronizeSpec() (bool, error) {

	if !s.isSynchronized() {
		s.logger.Info("synchronizing specs", "UserAccountSpecBase", s.recordSpecUserAcc.Spec.UserAccountSpecBase)
		if err := updateStatusConditions(s.logger, s.hostClient, s.record, toBeNotReady(toolchainv1alpha1.MasterUserRecordUpdatingReason, "")); err != nil {
			return false, err
		}
		s.memberUserAcc.Spec.UserAccountSpecBase = s.recordSpecUserAcc.Spec.UserAccountSpecBase
		s.memberUserAcc.Spec.Disabled = s.record.Spec.Disabled
		s.memberUserAcc.Spec.UserID = s.record.Spec.UserID

		err := s.memberCluster.Client.Update(context.TODO(), s.memberUserAcc)
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

		if !s.alignReadiness() {
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
	s.alignReadiness()
	s.logger.Info("updating MUR status")
	return s.hostClient.Status().Update(context.TODO(), s.record)
}

// withClusterDetails returns the given user account status with additional information about
// the target cluster such as API endpoint and Console URL if not set yet
func (s *Synchronizer) withClusterDetails(status toolchainv1alpha1.UserAccountStatusEmbedded) (toolchainv1alpha1.UserAccountStatusEmbedded, error) {
	var err error
	if status.Cluster.Name != "" {
		if status.Cluster.APIEndpoint == "" {
			status.Cluster.APIEndpoint = s.memberCluster.APIEndpoint
		}
		status, err = s.withConsoleURL(status)
		if err != nil {
			return status, err
		}
		status, err = s.withCheDashboardURL(status)
	}
	return status, err
}

// withConsoleURL returns the given user account status with Console URL set if it's not already set yet
func (s *Synchronizer) withConsoleURL(status toolchainv1alpha1.UserAccountStatusEmbedded) (toolchainv1alpha1.UserAccountStatusEmbedded, error) {
	if status.Cluster.ConsoleURL == "" {
		route := &routev1.Route{}
		namespacedName := types.NamespacedName{Namespace: consoleNamespace, Name: "console"}
		if err := s.memberCluster.Client.Get(context.TODO(), namespacedName, route); err != nil {
			if kuberrors.IsNotFound(err) {
				// It can happen if running in old OpenShift version like 3.x (minishift) in dev environment
				// TODO do this only if run in dev environment
				consoleURL, consoleErr := s.openShift3XConsoleURL(s.memberCluster.APIEndpoint)
				if consoleErr != nil {
					// Log the openShift3XConsoleURL() error but return the original missing route error
					s.logger.Error(err, "OpenShit 3.x web console unreachable", "url", consoleURL)
					return status, errors.Wrapf(err, "unable to get web console route for cluster %s", s.memberCluster.Name)
				}
				s.logger.Info("OpenShit 3.x web console URL used", "url", consoleURL)
				status.Cluster.ConsoleURL = consoleURL
				return status, nil
			}
			s.logger.Error(err, "unable to get console route")
			return status, err
		}

		status.Cluster.ConsoleURL = fmt.Sprintf("https://%s/%s", route.Spec.Host, route.Spec.Path)
	}
	return status, nil
}

// withCheDashboardURL returns the given user account status with Che Dashboard URL set if it's not already set yet
func (s *Synchronizer) withCheDashboardURL(status toolchainv1alpha1.UserAccountStatusEmbedded) (toolchainv1alpha1.UserAccountStatusEmbedded, error) {
	if status.Cluster.CheDashboardURL == "" {
		route := &routev1.Route{}
		namespacedName := types.NamespacedName{Namespace: cheNamespace, Name: "che"}
		err := s.memberCluster.Client.Get(context.TODO(), namespacedName, route)
		if err != nil {
			if kuberrors.IsNotFound(err) {
				// It can happen if Che Operator is not installed
				s.logger.Info("unable to get che dashboard route (operator not installed?)")
				return status, nil
			}
			s.logger.Error(err, "unable to get che dashboard route")
			return status, err
		}
		scheme := "https"
		if route.Spec.TLS == nil || *route.Spec.TLS == (routev1.TLSConfig{}) {
			scheme = "http"
		}
		status.Cluster.CheDashboardURL = fmt.Sprintf("%s://%s/%s", scheme, route.Spec.Host, route.Spec.Path)
	}
	return status, nil
}

// openShift3XConsoleURL checks if <apiEndpoint>/console URL is reachable.
// This URL is used by web console in OpenShift 3.x
func (s *Synchronizer) openShift3XConsoleURL(apiEndpoint string) (string, error) {
	url := fmt.Sprintf("%s/console", apiEndpoint)
	resp, err := consoleClient.Get(url)
	if err != nil {
		return url, err
	}
	defer func() {
		_, _ = ioutil.ReadAll(resp.Body)
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		return url, errors.Errorf("status is not 200 OK: %s", resp.Status)
	}
	return url, nil
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
func (s *Synchronizer) alignReadiness() bool {
	for _, uaStatus := range s.record.Status.UserAccounts {
		if !condition.IsTrue(uaStatus.Conditions, toolchainv1alpha1.ConditionReady) {
			return false
		}
	}
	s.record.Status.Conditions, _ = condition.AddOrUpdateStatusConditions(s.record.Status.Conditions, toBeProvisioned())
	return true
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
