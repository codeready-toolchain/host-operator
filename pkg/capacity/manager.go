package capacity

import (
	"context"
	"sort"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/controllers/toolchainconfig"
	"github.com/codeready-toolchain/host-operator/pkg/counter"
	"github.com/codeready-toolchain/toolchain-common/pkg/cluster"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/types"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func hasNotReachedMaxNumberOfSpacesThreshold(config toolchainconfig.ToolchainConfig, counts counter.Counts) cluster.Condition {
	return func(cluster *cluster.CachedToolchainCluster) bool {
		numberOfSpaces := counts.SpacesPerClusterCounts[cluster.Name]
		threshold := config.CapacityThresholds().MaxNumberOfSpacesSpecificPerMemberCluster()[cluster.Name]
		return threshold == 0 || numberOfSpaces < threshold
	}
}

func hasEnoughResources(config toolchainconfig.ToolchainConfig, status *toolchainv1alpha1.ToolchainStatus) cluster.Condition {
	return func(cluster *cluster.CachedToolchainCluster) bool {
		threshold, found := config.CapacityThresholds().ResourceCapacityThresholdSpecificPerMemberCluster()[cluster.Name]
		if !found {
			threshold = config.CapacityThresholds().ResourceCapacityThresholdDefault()
		}
		if threshold == 0 {
			return true
		}
		for _, memberStatus := range status.Status.Members {
			if memberStatus.ClusterName == cluster.Name {
				return hasMemberREnoughResources(memberStatus, threshold)
			}
		}
		return false
	}
}

func hasMemberREnoughResources(memberStatus toolchainv1alpha1.Member, threshold int) bool {
	if len(memberStatus.MemberStatus.ResourceUsage.MemoryUsagePerNodeRole) > 0 {
		for _, usagePerNode := range memberStatus.MemberStatus.ResourceUsage.MemoryUsagePerNodeRole {
			if usagePerNode >= threshold {
				return false
			}
		}
		return true
	}
	return false
}

func NewClusterManager(getMemberClusters cluster.GetMemberClustersFunc, cl runtimeclient.Client) *ClusterManager {
	return &ClusterManager{
		getMemberClusters: getMemberClusters,
		client:            cl,
	}
}

type ClusterManager struct {
	getMemberClusters cluster.GetMemberClustersFunc
	client            runtimeclient.Client
	lastUsed          string
}

// OptimalTargetClusterFilter is used by GetOptimalTargetCluster
// in order to retrieve an "optimal" cluster for the Space to be provisioned into.
type OptimalTargetClusterFilter struct {
	// PreferredCluster if specified and available,
	// it will be used to find the desired member cluster by name.
	PreferredCluster string
	// ToolchainStatusNamespace is the namespace where the toolchainstatus CR will be searched,
	// in order to check which cluster has enough resources and can be candidate for the "optimal" cluster for Space provisioning.
	ToolchainStatusNamespace string
	// ClusterRoles is a list of cluster-role labels,
	// if provided, only the clusters matching those labels will be selected as candidates for the "optimal" cluster.
	ClusterRoles []string
}

// GetOptimalTargetCluster returns the name of the cluster with the most available capacity where a Space could be provisioned.
//
// If two clusters have the same limit and they both have the same usage, then the logic distributes spaces in a batches of 50.
//
// If the two clusters don't have the same limit, then the batch is based on the scale of the limits.
// Let's say that the limit for member1 is 1000 and for member2 is 2000, then the batch of spaces would be 50 for member1 and 100 for member2.
//
// If the preferredCluster is provided and it is also one of the available clusters, then the same name is returned.
// In case the preferredCluster was not provided or not found/available and the clusterRoles are provided then the candidates optimal cluster pool will be made out by only those matching the labels, if any available.
func (b *ClusterManager) GetOptimalTargetCluster(optimalClusterFilter OptimalTargetClusterFilter) (string, error) {
	config, err := toolchainconfig.GetToolchainConfig(b.client)
	if err != nil {
		return "", errors.Wrapf(err, "unable to get ToolchainConfig")
	}

	counts, err := counter.GetCounts()
	if err != nil {
		return "", errors.Wrapf(err, "unable to get the number of provisioned spaces")
	}

	status := &toolchainv1alpha1.ToolchainStatus{}
	if err := b.client.Get(context.TODO(), types.NamespacedName{Namespace: optimalClusterFilter.ToolchainStatusNamespace, Name: toolchainconfig.ToolchainStatusName}, status); err != nil {
		return "", errors.Wrapf(err, "unable to read ToolchainStatus resource")
	}
	optimalTargetClusters := getOptimalTargetClusters(optimalClusterFilter.PreferredCluster, b.getMemberClusters, optimalClusterFilter.ClusterRoles, hasNotReachedMaxNumberOfSpacesThreshold(config, counts), hasEnoughResources(config, status))

	if len(optimalTargetClusters) == 1 {
		return optimalTargetClusters[0], nil
	}

	for _, cluster := range optimalTargetClusters {
		if cluster == b.lastUsed {
			provisioned := counts.SpacesPerClusterCounts[cluster]
			if provisioned%50 != 0 {
				return cluster, nil
			}
		}
	}

	sort.Slice(optimalTargetClusters, func(i, j int) bool {
		provisioned1 := counts.SpacesPerClusterCounts[optimalTargetClusters[i]]
		threshold1 := config.CapacityThresholds().MaxNumberOfSpacesSpecificPerMemberCluster()[optimalTargetClusters[i]]

		provisioned2 := counts.SpacesPerClusterCounts[optimalTargetClusters[j]]
		threshold2 := config.CapacityThresholds().MaxNumberOfSpacesSpecificPerMemberCluster()[optimalTargetClusters[j]]

		// Let's round the number of provisioned users down to the closest multiple of 50
		// This is a trick we need to do before comparing the capacity, so we can distribute the users in batches by 50 (if the clusters have the same limit)
		provisioned1By50 := (provisioned1 / 50) * 50
		provisioned2By50 := (provisioned2 / 50) * 50

		// now we can calculate what is the actual usage of the clusters (how many users are provisioned there compared to the threshold) and compare them
		return float64(provisioned1By50)/float64(threshold1) < float64(provisioned2By50)/float64(threshold2)
	})

	b.lastUsed = optimalTargetClusters[0]
	return optimalTargetClusters[0], nil
}

// getOptimalTargetClusters checks if a preferred target cluster was provided and available from the cluster pool.
// If the preferred target cluster was not provided or not available, but a list of clusterRoles was provided, then it filters only the available clusters matching all those roles.
// If no cluster roles were provided then it returns all the available clusters.
// The function returns a slice with an empty string if not optimal target clusters where found.
func getOptimalTargetClusters(preferredCluster string, getMemberClusters cluster.GetMemberClustersFunc, clusterRoles []string, conditions ...cluster.Condition) []string {
	emptyTargetCluster := []string{""}
	// Automatic cluster selection based on cluster readiness
	members := getMemberClusters(append(conditions, cluster.Ready)...)
	if len(members) == 0 {
		return emptyTargetCluster
	}

	// extract only names of the available clusters
	var memberNames []string
	for _, member := range members {
		// if cluster-role labels were provided, it will check for matching on the member labels
		// if no clusterRoles labels are required, then the function will return all member cluster with the `tenant` cluster role label
		if hasClusterRoles(clusterRoles, member) {
			memberNames = append(memberNames, member.Name)
		}
	}

	// return empty string if no members available with roles
	if len(memberNames) == 0 {
		return emptyTargetCluster
	}

	// if the preferred cluster is provided and it is also one of the available clusters, then the same name is returned, otherwise, it returns the first available one
	if preferredCluster != "" {
		for _, memberName := range memberNames {
			if preferredCluster == memberName {
				return []string{memberName}
			}
		}
	}

	// return the member names in case some were found
	return memberNames
}

func hasClusterRoles(clusterRoles []string, member *cluster.CachedToolchainCluster) bool {
	if len(clusterRoles) == 0 {
		// by default it should pick the `tenant` cluster role, if no specific cluster role was provided
		clusterRoles = []string{cluster.RoleLabel(cluster.Tenant)}
	}

	// filter member cluster having the required cluster role
	for _, clusterRoleLabel := range clusterRoles {
		if _, hasRole := member.Labels[clusterRoleLabel]; !hasRole {
			// missing cluster role
			return false
		}
	}

	// all cluster roles were matched
	return true
}
