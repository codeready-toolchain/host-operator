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
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var log = logf.Log.WithName("pending_object_cache")

type GetListOfPendingObjects func(cl client.Client, labelListOption client.ListOption) ([]client.Object, error)

type cache struct {
	sync.RWMutex
	objectsByCreation       []string
	client                  client.Client
	objectType              client.Object
	getListOfPendingObjects GetListOfPendingObjects
}

func (c *cache) getOldestPendingObject(namespace string) client.Object {
	c.Lock()
	defer c.Unlock()
	oldest := c.getFirstExisting(namespace)
	if oldest == nil {
		c.loadLatest(namespace)
		oldest = c.getFirstExisting(namespace)
	}
	return oldest
}

func (c *cache) loadLatest(namespace string) { //nolint:unparam
	labels := map[string]string{toolchainv1alpha1.StateLabelKey: toolchainv1alpha1.StateLabelValuePending}
	opts := client.MatchingLabels(labels)

	pendingObjects, err := c.getListOfPendingObjects(c.client, opts)
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
		c.objectsByCreation = append(c.objectsByCreation, object.GetName())
	}
}

func (c *cache) getFirstExisting(namespace string) client.Object {
	if len(c.objectsByCreation) == 0 {
		return nil
	}
	name := c.objectsByCreation[0]
	firstExisting := c.objectType.DeepCopyObject().(client.Object)
	if err := c.client.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: name}, firstExisting); err != nil {
		if apierrors.IsNotFound(err) {
			c.objectsByCreation = c.objectsByCreation[1:]
			return c.getFirstExisting(namespace)
		}
		log.Error(err, fmt.Sprintf("could not get the oldest unapproved '%T'", c.objectType))
		return nil
	}
	if firstExisting.GetLabels()[toolchainv1alpha1.StateLabelKey] != toolchainv1alpha1.StateLabelValuePending {
		c.objectsByCreation = c.objectsByCreation[1:]
		return c.getFirstExisting(namespace)
	}

	return firstExisting
}

var getListOfPendingUserSignups GetListOfPendingObjects = func(cl client.Client, labelListOption client.ListOption) ([]client.Object, error) {
	userSignupList := &toolchainv1alpha1.UserSignupList{}
	if err := cl.List(context.TODO(), userSignupList, labelListOption); err != nil {
		return nil, err
	}
	objects := make([]client.Object, len(userSignupList.Items))
	for i := range userSignupList.Items {
		userSignup := userSignupList.Items[i]
		objects[i] = &userSignup
	}
	return objects, nil
}

var getListOfPendingSpaces GetListOfPendingObjects = func(cl client.Client, labelListOption client.ListOption) ([]client.Object, error) {
	spaceList := &toolchainv1alpha1.SpaceList{}
	if err := cl.List(context.TODO(), spaceList, labelListOption); err != nil {
		return nil, err
	}
	objects := make([]client.Object, len(spaceList.Items))
	for i := range spaceList.Items {
		space := spaceList.Items[i]
		objects[i] = &space
	}
	return objects, nil
}
