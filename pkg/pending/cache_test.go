package pending

import (
	"context"
	"sync"
	"testing"
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	. "github.com/codeready-toolchain/host-operator/test"
	"github.com/codeready-toolchain/host-operator/test/space"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestGetOldestSignupPendingApproval(t *testing.T) {
	// given
	withoutStateLabel := NewUserSignup()
	notReady := NewUserSignup(WithStateLabel("not-ready"))
	pending := NewUserSignup(WithStateLabel("pending"))
	approved := NewUserSignup(WithStateLabel("approved"))
	deactivated := NewUserSignup(WithStateLabel("deactivated"))
	banned := NewUserSignup(WithStateLabel("banned"))

	cache, cl := newCache(t, &toolchainv1alpha1.UserSignup{}, listPendingUserSignups, withoutStateLabel, notReady, pending, approved, deactivated, banned)

	// when
	foundPending := cache.getOldestPendingObject(test.HostOperatorNs)

	// then
	assert.Len(t, cache.sortedObjectNames, 1)
	assert.Equal(t, pending.Name, foundPending.GetName())
	approve(t, cl, pending)

	t.Run("won't return any since all are approved", func(t *testing.T) {
		// when
		foundPending := cache.getOldestPendingObject(test.HostOperatorNs)

		// then
		assert.Empty(t, cache.sortedObjectNames)
		assert.Nil(t, foundPending)
	})

	t.Run("will pick the newly added (reactivated) pending UserSignup", func(t *testing.T) {
		// given
		WithStateLabel("pending")(deactivated)
		err := cl.Update(context.TODO(), deactivated)
		require.NoError(t, err)

		// when
		foundPending := cache.getOldestPendingObject(test.HostOperatorNs)

		// then
		assert.Len(t, cache.sortedObjectNames, 1)
		assert.Equal(t, deactivated.Name, foundPending.GetName())

		t.Run("should keep unapproved resource", func(t *testing.T) {
			// when
			foundPending := cache.getOldestPendingObject(test.HostOperatorNs)

			// then
			assert.Len(t, cache.sortedObjectNames, 1)
			assert.Equal(t, deactivated.Name, foundPending.GetName())
		})
	})
}

func TestGetOldestSpacePendingTargetCluster(t *testing.T) {
	// given
	withoutStateLabel := space.NewSpace("without-state")
	pending := space.NewSpace("pending", space.WithStateLabel("pending"))
	clusterAssigned := space.NewSpace("cluster-assigned", space.WithStateLabel("cluster-assigned"))

	cache, cl := newCache(t, &toolchainv1alpha1.Space{}, listPendingSpaces, withoutStateLabel, pending, clusterAssigned)

	// when
	foundPending := cache.getOldestPendingObject(test.HostOperatorNs)

	// then
	require.Len(t, cache.sortedObjectNames, 1)
	assert.Equal(t, pending.Name, foundPending.GetName())
	assignCluster(t, cl, pending)

	t.Run("won't return any since all have cluster assigned", func(t *testing.T) {
		// when
		foundPending := cache.getOldestPendingObject(test.HostOperatorNs)

		// then
		assert.Empty(t, cache.sortedObjectNames)
		assert.Nil(t, foundPending)
	})

	t.Run("will pick the newly added pending Space", func(t *testing.T) {
		// given
		space.WithStateLabel("pending")(clusterAssigned)
		err := cl.Update(context.TODO(), clusterAssigned)
		require.NoError(t, err)

		// when
		foundPending := cache.getOldestPendingObject(test.HostOperatorNs)

		// then
		assert.Len(t, cache.sortedObjectNames, 1)
		assert.Equal(t, clusterAssigned.Name, foundPending.GetName())

		t.Run("should keep pending resource", func(t *testing.T) {
			// when
			foundPending := cache.getOldestPendingObject(test.HostOperatorNs)

			// then
			assert.Len(t, cache.sortedObjectNames, 1)
			assert.Equal(t, clusterAssigned.Name, foundPending.GetName())
		})
	})
}

func approve(t *testing.T, cl *test.FakeClient, signup *toolchainv1alpha1.UserSignup) {
	WithStateLabel("approved")(signup)
	err := cl.Update(context.TODO(), signup)
	require.NoError(t, err)
}

func assignCluster(t *testing.T, cl *test.FakeClient, sp *toolchainv1alpha1.Space) {
	space.WithStateLabel("cluster-assigned")(sp)
	err := cl.Update(context.TODO(), sp)
	require.NoError(t, err)
}

func TestGetOldestPendingApprovalWithMultipleUserSignups(t *testing.T) {
	// given
	withoutStateLabel := NewUserSignup()
	// we need to create the UserSignups with different timestamp in the range of seconds because
	// the k8s resource keeps the time in RFC3339 format where the smallest unit are seconds.
	pending1 := NewUserSignup(WithStateLabel("pending"), CreatedBefore(5*time.Second), WithName("1-oldest"))
	pending2 := NewUserSignup(WithStateLabel("pending"), CreatedBefore(3*time.Second), WithName("2-oldest"))
	pending3 := NewUserSignup(WithStateLabel("pending"), CreatedBefore(2*time.Second), WithName("3-oldest"))
	cache, cl := newCache(t, &toolchainv1alpha1.UserSignup{}, listPendingUserSignups, withoutStateLabel, pending2, pending3, pending1)

	// when
	foundPending := cache.getOldestPendingObject(test.HostOperatorNs)

	// then
	assert.Len(t, cache.sortedObjectNames, 3)
	assert.Equal(t, pending1.Name, foundPending.GetName())
	approve(t, cl, pending1)

	t.Run("should keep two UserSignup since the pending4 hasn't been loaded yet", func(t *testing.T) {
		// given
		pending4 := NewUserSignup(WithStateLabel("pending"))
		err := cl.Create(context.TODO(), pending4)
		require.NoError(t, err)

		// when
		foundPending := cache.getOldestPendingObject(test.HostOperatorNs)

		// then
		assert.Len(t, cache.sortedObjectNames, 2)
		assert.Equal(t, pending2.Name, foundPending.GetName())
		approve(t, cl, pending2)

		t.Run("should keep one UserSignup since the pending4 hasn't been loaded yet and previous ones were removed", func(t *testing.T) {
			// when
			foundPending := cache.getOldestPendingObject(test.HostOperatorNs)

			// then
			assert.Len(t, cache.sortedObjectNames, 1)
			assert.Equal(t, pending3.Name, foundPending.GetName())
			approve(t, cl, pending3)

			t.Run("should load the pending4 resource", func(t *testing.T) {
				// when
				foundPending := cache.getOldestPendingObject(test.HostOperatorNs)

				// then
				assert.Len(t, cache.sortedObjectNames, 1)
				assert.Equal(t, pending4.Name, foundPending.GetName())
			})
		})
	})
}

func TestGetOldestPendingApprovalWithMultipleSpaces(t *testing.T) {
	// given
	withoutStateLabel := space.NewSpace("without-state")
	// we need to create the Spaces with different timestamp in the range of seconds because
	// the k8s resource keeps the time in RFC3339 format where the smallest unit are seconds.
	pending1 := space.NewSpace("1-oldest", space.WithStateLabel("pending"), space.CreatedBefore(5*time.Second))
	pending2 := space.NewSpace("2-oldest", space.WithStateLabel("pending"), space.CreatedBefore(3*time.Second))
	pending3 := space.NewSpace("3-oldest", space.WithStateLabel("pending"), space.CreatedBefore(2*time.Second))
	cache, cl := newCache(t, &toolchainv1alpha1.Space{}, listPendingSpaces, withoutStateLabel, pending2, pending3, pending1)

	// when
	foundPending := cache.getOldestPendingObject(test.HostOperatorNs)

	// then
	assert.Len(t, cache.sortedObjectNames, 3)
	assert.Equal(t, pending1.Name, foundPending.GetName())
	assignCluster(t, cl, pending1)

	t.Run("should keep two Spaces since the pending4 hasn't been loaded yet", func(t *testing.T) {
		// given
		pending4 := space.NewSpace("pending-4", space.WithStateLabel("pending"))
		err := cl.Create(context.TODO(), pending4)
		require.NoError(t, err)

		// when
		foundPending := cache.getOldestPendingObject(test.HostOperatorNs)

		// then
		assert.Len(t, cache.sortedObjectNames, 2)
		assert.Equal(t, pending2.Name, foundPending.GetName())
		assignCluster(t, cl, pending2)

		t.Run("should keep one Space since the pending4 hasn't been loaded yet and previous ones were removed", func(t *testing.T) {
			// when
			foundPending := cache.getOldestPendingObject(test.HostOperatorNs)

			// then
			assert.Len(t, cache.sortedObjectNames, 1)
			assert.Equal(t, pending3.Name, foundPending.GetName())
			assignCluster(t, cl, pending3)

			t.Run("should load the pending4 resource", func(t *testing.T) {
				// when
				foundPending := cache.getOldestPendingObject(test.HostOperatorNs)

				// then
				assert.Len(t, cache.sortedObjectNames, 1)
				assert.Equal(t, pending4.Name, foundPending.GetName())
			})
		})
	})
}

func TestGetOldestPendingApprovalWithMultipleUserSignupsInParallel(t *testing.T) {
	// given
	cache, cl := newCache(t, &toolchainv1alpha1.UserSignup{}, listPendingUserSignups)

	var latch sync.WaitGroup
	latch.Add(1)
	var waitForFinished sync.WaitGroup

	waitForFinished.Add(3000)
	for i := 0; i < 1000; i++ {
		go func() {
			defer waitForFinished.Done()
			latch.Wait()
			pending1 := NewUserSignup(WithStateLabel("pending"))
			pending2 := NewUserSignup(WithStateLabel("pending"))
			allSingups := []*toolchainv1alpha1.UserSignup{
				NewUserSignup(WithStateLabel("not-ready")),
				NewUserSignup(WithStateLabel("deactivated")),
				NewUserSignup(WithStateLabel("approved")),
				pending1,
				pending2,
			}
			for _, signup := range allSingups {
				err := cl.Create(context.TODO(), signup)
				require.NoError(t, err)
			}

			for _, pending := range []*toolchainv1alpha1.UserSignup{pending1, pending2} {
				go func(toApprove *toolchainv1alpha1.UserSignup) {
					defer waitForFinished.Done()
					oldestPendingApproval := cache.getOldestPendingObject(test.HostOperatorNs)
					require.NotNil(t, oldestPendingApproval)
					approve(t, cl, toApprove)
				}(pending)
			}
		}()
	}

	// when
	latch.Done()
	waitForFinished.Wait()

	// when
	foundPending := cache.getOldestPendingObject(test.HostOperatorNs)

	// then
	assert.Nil(t, foundPending)
}

func newCache(t *testing.T, objectType client.Object, listPendingObjects ListPendingObjects, initObjects ...runtime.Object) (*cache, *test.FakeClient) {
	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)

	fakeClient := test.NewFakeClient(t, initObjects...)
	return &cache{
		client:             fakeClient,
		objectType:         objectType,
		listPendingObjects: listPendingObjects,
	}, fakeClient
}
