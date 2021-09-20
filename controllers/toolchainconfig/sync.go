package toolchainconfig

import (
	"context"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/cluster"

	"github.com/go-logr/logr"
)

type Synchronizer struct {
	getMembersFunc cluster.GetMemberClustersFunc
	logger         logr.Logger
}

func NewSynchronizer(logger logr.Logger, getMembersFunc cluster.GetMemberClustersFunc) Synchronizer {
	return Synchronizer{
		getMembersFunc: getMembersFunc,
		logger:         logger,
	}
}

// SyncMemberConfigs retrieves member operator configurations and syncs the appropriate configuration to each member cluster
// returns a map of any errors encountered indexed by cluster name
func (s *Synchronizer) SyncMemberConfigs(sourceConfig *toolchainv1alpha1.ToolchainConfig) map[string]string {
	toolchainConfig := sourceConfig.DeepCopy()

	// get configs for member toolchainclusters
	memberToolchainClusters := s.getMembersFunc()
	syncErrors := make(map[string]string, len(memberToolchainClusters))

	if len(memberToolchainClusters) == 0 {
		s.logger.Info("No toolchainclusters were found, skipping MemberOperatorConfig syncing")
		return syncErrors
	}

	membersWithSpecificConfig := toolchainConfig.Spec.Members.SpecificPerMemberCluster

	for _, toolchainCluster := range memberToolchainClusters {
		memberConfigSpec := toolchainConfig.Spec.Members.Default

		if c, ok := membersWithSpecificConfig[toolchainCluster.Name]; ok {
			// member-specific configuration values override default configuration
			memberConfigSpec = c
			delete(membersWithSpecificConfig, toolchainCluster.Name)
		}

		if err := SyncMemberConfig(memberConfigSpec, toolchainCluster); err != nil {
			s.logger.Error(err, "failed to sync MemberOperatorConfig", "cluster_name", toolchainCluster.Name)
			syncErrors[toolchainCluster.Name] = err.Error()
		}
	}

	// add errors for any MemberOperatorConfigs that haven't been synced because there is no matching toolchaincluster
	if len(membersWithSpecificConfig) > 0 {
		for k := range membersWithSpecificConfig {
			syncErrors[k] = "specific member configuration exists but no matching toolchaincluster was found"
		}
	}
	return syncErrors
}

func SyncMemberConfig(memberConfigSpec toolchainv1alpha1.MemberOperatorConfigSpec, memberCluster *cluster.CachedToolchainCluster) error {
	memberConfig := &toolchainv1alpha1.MemberOperatorConfig{}
	if err := memberCluster.Client.Get(context.TODO(), types.NamespacedName{Namespace: memberCluster.OperatorNamespace, Name: configResourceName}, memberConfig); err != nil {
		if errors.IsNotFound(err) {
			// MemberOperatorConfig does not exist - create it
			memberConfig := &toolchainv1alpha1.MemberOperatorConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      configResourceName,
					Namespace: memberCluster.OperatorNamespace,
				},
				Spec: memberConfigSpec,
			}
			return memberCluster.Client.Create(context.TODO(), memberConfig)
		}
		// Error reading the object - try again on the next reconcile
		return err
	}

	// MemberOperatorConfig exists - update spec
	memberConfig.Spec = memberConfigSpec

	return memberCluster.Client.Update(context.TODO(), memberConfig)
}
