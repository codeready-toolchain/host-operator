package cluster

import (
	"context"
	"fmt"

	commoncluster "github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	errs "github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type Cluster struct {
	*commoncluster.Config
	Client     runtimeclient.Client
	RESTClient *rest.RESTClient
	Cache      cache.Cache
}

// LookupMember takes in a list of member clusters and the NamespacedName of the Object, it then searches for the member cluster that triggered the request.
// returns memberCluster/true/nil in case the Object was found on one of the given member clusters
// returns memberCluster/true/error in case the Object was found on one of the given member clusters but there was also an error while searching it
// returns memberCluster/false/nil in case the Object was NOT found on any of the given member clusters
// returns memberCluster/false/error in case the Object was NOT found on any of the given member clusters and there was an error while trying to read one or more of the memberclusters
//
// CAVEAT: if the object was not found and there is an error reading from more than one member cluster, only the last read error will be returned.
func LookupMember(memberClusters map[string]Cluster, namespacedName types.NamespacedName, object runtimeclient.Object) (Cluster, bool, error) {
	var memberClusterWithObject Cluster
	var getError error
	var found bool
	for _, memberCluster := range memberClusters {
		err := memberCluster.Client.Get(context.TODO(), namespacedName, object)
		if err != nil {
			if !errors.IsNotFound(err) {
				// Error reading the object
				getError = err // save the error and return it only later in case the Object was not found on any of the clusters
			}
			//  Object not found on current member cluster
			continue
		}
		// save the member cluster on which the Object was found
		memberClusterWithObject = memberCluster
		found = true
		break // exit once found
	}
	if !found {
		if getError != nil {
			// if we haven't found the Object, and we got an error while trying to retrieve it
			// then let's return the error
			return memberClusterWithObject, found, errs.Wrap(getError, fmt.Sprintf("unable to get the current %T", object))
		}
		// Not found. No Error.
		return memberClusterWithObject, found, nil
	}
	// Object found, return the member cluster that has it. Also, if we got an error when trying to connect to at least one of them member then return it too.
	return memberClusterWithObject, found, getError
}
