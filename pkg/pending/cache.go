package pending

import (
	"context"
	"fmt"
	"sort"
	"sync"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"

	errs "github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var log = logf.Log.WithName("pending_object_cache")

type ListPendingObjects func(cl runtimeclient.Client, labelListOption runtimeclient.ListOption) ([]runtimeclient.Object, error)

type cache struct {
	sync.RWMutex
	sortedObjectNames  []string
	client             runtimeclient.Client
	objectType         runtimeclient.Object
	listPendingObjects ListPendingObjects
}

func (c *cache) getOldestPendingObject(namespace string) runtimeclient.Object {
	c.Lock()
	defer c.Unlock()
	oldest := c.getFirstExisting(namespace)
	if oldest == nil {
		c.loadLatest()
		oldest = c.getFirstExisting(namespace)
	}
	return oldest
}

func (c *cache) loadLatest() { //nolint:unparam
	labels := map[string]string{toolchainv1alpha1.StateLabelKey: toolchainv1alpha1.StateLabelValuePending}
	opts := runtimeclient.MatchingLabels(labels)

	pendingObjects, err := c.listPendingObjects(c.client, opts)
	if err != nil {
		err = errs.Wrapf(err, "unable to list %s resources with label '%s' having value '%s'",
			c.objectType.GetObjectKind().GroupVersionKind().Kind, toolchainv1alpha1.StateLabelKey, toolchainv1alpha1.StateLabelValuePending)
		log.Error(err, "could not get the oldest pending object")
		return
	}

	sort.Slice(pendingObjects, func(i, j int) bool {
		return pendingObjects[i].GetCreationTimestamp().Time.Before(pendingObjects[j].GetCreationTimestamp().Time)
	})
	for _, object := range pendingObjects {
		c.sortedObjectNames = append(c.sortedObjectNames, object.GetName())
	}
}

func (c *cache) getFirstExisting(namespace string) runtimeclient.Object {
	if len(c.sortedObjectNames) == 0 {
		return nil
	}
	name := c.sortedObjectNames[0]
	firstExisting := c.objectType.DeepCopyObject().(runtimeclient.Object)
	if err := c.client.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: name}, firstExisting); err != nil {
		if apierrors.IsNotFound(err) {
			c.sortedObjectNames = c.sortedObjectNames[1:]
			return c.getFirstExisting(namespace)
		}
		log.Error(err, fmt.Sprintf("could not get the oldest unapproved '%T'", c.objectType))
		return nil
	}
	if firstExisting.GetLabels()[toolchainv1alpha1.StateLabelKey] != toolchainv1alpha1.StateLabelValuePending {
		c.sortedObjectNames = c.sortedObjectNames[1:]
		return c.getFirstExisting(namespace)
	}

	return firstExisting
}

var listPendingUserSignups ListPendingObjects = func(cl runtimeclient.Client, labelListOption runtimeclient.ListOption) ([]runtimeclient.Object, error) {
	userSignupList := &toolchainv1alpha1.UserSignupList{}
	if err := cl.List(context.TODO(), userSignupList, labelListOption); err != nil {
		return nil, err
	}
	objects := make([]runtimeclient.Object, len(userSignupList.Items))
	for i := range userSignupList.Items {
		userSignup := userSignupList.Items[i]
		objects[i] = &userSignup
	}
	return objects, nil
}

var listPendingSpaces ListPendingObjects = func(cl runtimeclient.Client, labelListOption runtimeclient.ListOption) ([]runtimeclient.Object, error) {
	spaceList := &toolchainv1alpha1.SpaceList{}
	if err := cl.List(context.TODO(), spaceList, labelListOption); err != nil {
		return nil, err
	}
	objects := make([]runtimeclient.Object, len(spaceList.Items))
	for i := range spaceList.Items {
		space := spaceList.Items[i]
		objects[i] = &space
	}
	return objects, nil
}
