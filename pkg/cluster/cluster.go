package cluster

import (
	"context"
	"fmt"
	"reflect"

	commoncluster "github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	errs "github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

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

// LookupMember takes in a list of member clusters and a Request, it then searches for the member cluster that triggered the request.
// returns memberCluster/true/nil in case the Object was found on one of the given member clusters
// returns nil/false/nil in case the Object was NOT found on any of the given member clusters
// returns nil/false/error in case the Object was NOT found on any of the given member clusters and there was an error while trying to read one or more of the memberclusters
//
// CAVEAT: if the object was not found and there is an error reading from more than one member cluster, only the last read error will be returned.
func LookupMember(MemberClusters map[string]Cluster, request ctrl.Request, object runtimeclient.Object) (Cluster, bool, error) {
	var memberClusterWithObject Cluster
	var err, getError error
	for _, memberCluster := range MemberClusters {
		err = memberCluster.Client.Get(context.TODO(), types.NamespacedName{
			Namespace: request.Namespace,
			Name:      request.Name,
		}, object)
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
		break // exit once found
	}

	// if we haven't found the Object, the memberCluster is an empty struct,
	// we can't do the same check on the runtimeclient.Object since for some reason it has the TypeMeta field on it
	// and the check fails.
	if reflect.ValueOf(memberClusterWithObject).IsZero() {
		if getError != nil {
			// if we haven't found the Object, and we got an error while trying to retrieve it
			// then let's return the error
			return memberClusterWithObject, false, errs.Wrap(getError, fmt.Sprintf("unable to get the current %T", object))
		} else if err != nil && errors.IsNotFound(err) {
			// if we exited with a notFound error
			// it means that we couldn't find the Object on any of the given member clusters
			// let's just return false (as not found) and no error
			return memberClusterWithObject, false, nil
		}
	}
	// Object found, return the member cluster that has it
	return memberClusterWithObject, true, nil
}
