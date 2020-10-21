package unapproved

import (
	"context"
	"sort"
	"sync"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var log = logf.Log.WithName("unapproved_usersignup_cache")

type cache struct {
	sync.RWMutex
	userSignupsByCreation []string
	client                client.Client
}

func (c *cache) getOldestPendingApproval(namespace string) *toolchainv1alpha1.UserSignup {
	c.Lock()
	defer c.Unlock()
	oldest := c.getFirstExisting(namespace)
	if oldest == nil {
		c.loadLatest(namespace)
		oldest = c.getFirstExisting(namespace)
	}
	return oldest
}

func (c *cache) loadLatest(namespace string) {
	labels := map[string]string{toolchainv1alpha1.UserSignupStateLabelKey: toolchainv1alpha1.UserSignupStateLabelValuePending}
	opts := client.MatchingLabels(labels)
	userSignupList := &toolchainv1alpha1.UserSignupList{}
	if err := c.client.List(context.TODO(), userSignupList, opts); err != nil {
		err = errors.Wrapf(err, "unable to list UserSignup resources with label '%s' having value '%s'",
			toolchainv1alpha1.UserSignupStateLabelKey, toolchainv1alpha1.UserSignupStateLabelValuePending)
		log.Error(err, "could not get the oldest unapproved UserSignup")
		return
	}
	userSignups := userSignupList.Items
	sort.Slice(userSignups, func(i, j int) bool {
		return userSignups[i].CreationTimestamp.Time.Before(userSignups[j].CreationTimestamp.Time)
	})
	for _, userSignup := range userSignups {
		c.userSignupsByCreation = append(c.userSignupsByCreation, userSignup.Name)
	}
}

func (c *cache) getFirstExisting(namespace string) *toolchainv1alpha1.UserSignup {
	if len(c.userSignupsByCreation) == 0 {
		return nil
	}
	name := c.userSignupsByCreation[0]
	userSignup := &toolchainv1alpha1.UserSignup{}
	if err := c.client.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: name}, userSignup); err != nil {
		if apierrors.IsNotFound(err) {
			c.userSignupsByCreation = c.userSignupsByCreation[1:]
			return c.getFirstExisting(namespace)
		}
		log.Error(err, "could not get the oldest unapproved UserSignup")
		return nil
	}
	if userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey] != toolchainv1alpha1.UserSignupStateLabelValuePending {
		c.userSignupsByCreation = c.userSignupsByCreation[1:]
		return c.getFirstExisting(namespace)
	}

	return userSignup
}
