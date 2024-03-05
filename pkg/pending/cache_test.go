package pending

import (
	"context"
	"sync"
	"testing"
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	spacetest "github.com/codeready-toolchain/toolchain-common/pkg/test/space"
	commonsignup "github.com/codeready-toolchain/toolchain-common/pkg/test/usersignup"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func TestGetOldestSignupPendingApproval(t *testing.T) {
	// given
	ctx := context.TODO()
	withoutStateLabel := commonsignup.NewUserSignup()
	notReady := commonsignup.NewUserSignup(commonsignup.WithStateLabel("not-ready"))
	pending := commonsignup.NewUserSignup(commonsignup.WithStateLabel("pending"))
	approved := commonsignup.NewUserSignup(commonsignup.WithStateLabel("approved"))
	deactivated := commonsignup.NewUserSignup(commonsignup.WithStateLabel("deactivated"))
	banned := commonsignup.NewUserSignup(commonsignup.WithStateLabel("banned"))

	cache, cl := newCache(t, &toolchainv1alpha1.UserSignup{}, listPendingUserSignups, withoutStateLabel, notReady, pending, approved, deactivated, banned)

	// when
	foundPending := cache.getOldestPendingObject(ctx, test.HostOperatorNs)

	// then
	assert.Len(t, cache.sortedObjectNames, 1)
	assert.Equal(t, pending.Name, foundPending.GetName())
	approve(t, cl, pending)

	t.Run("won't return any since all are approved", func(t *testing.T) {
		// when
		foundPending := cache.getOldestPendingObject(ctx, test.HostOperatorNs)

		// then
		assert.Empty(t, cache.sortedObjectNames)
		assert.Nil(t, foundPending)
	})

	t.Run("will pick the newly added (reactivated) pending UserSignup", func(t *testing.T) {
		// given
		commonsignup.WithStateLabel("pending")(deactivated)
		err := cl.Update(context.TODO(), deactivated)
		require.NoError(t, err)

		// when
		foundPending := cache.getOldestPendingObject(ctx, test.HostOperatorNs)

		// then
		assert.Len(t, cache.sortedObjectNames, 1)
		assert.Equal(t, deactivated.Name, foundPending.GetName())

		t.Run("should keep unapproved resource", func(t *testing.T) {
			// when
			foundPending := cache.getOldestPendingObject(ctx, test.HostOperatorNs)

			// then
			assert.Len(t, cache.sortedObjectNames, 1)
			assert.Equal(t, deactivated.Name, foundPending.GetName())
		})
	})
}

func TestGetOldestSpacePendingTargetCluster(t *testing.T) {
	// given
	ctx := context.TODO()
	withoutStateLabel := spacetest.NewSpace(test.HostOperatorNs, "without-state")
	pending := spacetest.NewSpace(test.HostOperatorNs, "pending", spacetest.WithStateLabel("pending"))
	clusterAssigned := spacetest.NewSpace(test.HostOperatorNs, "cluster-assigned", spacetest.WithStateLabel("cluster-assigned"))

	cache, cl := newCache(t, &toolchainv1alpha1.Space{}, listPendingSpaces, withoutStateLabel, pending, clusterAssigned)

	// when
	foundPending := cache.getOldestPendingObject(ctx, test.HostOperatorNs)

	// then
	require.Len(t, cache.sortedObjectNames, 1)
	assert.Equal(t, pending.Name, foundPending.GetName())
	assignCluster(t, cl, pending)

	t.Run("won't return any since all have cluster assigned", func(t *testing.T) {
		// when
		foundPending := cache.getOldestPendingObject(ctx, test.HostOperatorNs)

		// then
		assert.Empty(t, cache.sortedObjectNames)
		assert.Nil(t, foundPending)
	})

	t.Run("will pick the newly added pending Space", func(t *testing.T) {
		// given
		spacetest.WithStateLabel("pending")(clusterAssigned)
		err := cl.Update(context.TODO(), clusterAssigned)
		require.NoError(t, err)

		// when
		foundPending := cache.getOldestPendingObject(ctx, test.HostOperatorNs)

		// then
		assert.Len(t, cache.sortedObjectNames, 1)
		assert.Equal(t, clusterAssigned.Name, foundPending.GetName())

		t.Run("should keep pending resource", func(t *testing.T) {
			// when
			foundPending := cache.getOldestPendingObject(ctx, test.HostOperatorNs)

			// then
			assert.Len(t, cache.sortedObjectNames, 1)
			assert.Equal(t, clusterAssigned.Name, foundPending.GetName())
		})
	})
}

func approve(t *testing.T, cl *test.FakeClient, signup *toolchainv1alpha1.UserSignup) {
	commonsignup.WithStateLabel("approved")(signup)
	err := cl.Update(context.TODO(), signup)
	assert.NoError(t, err) //  approve contains assertions that must only be used in the goroutine running the test function (testifylint)
}

func assignCluster(t *testing.T, cl *test.FakeClient, sp *toolchainv1alpha1.Space) {
	spacetest.WithStateLabel("cluster-assigned")(sp)
	err := cl.Update(context.TODO(), sp)
	require.NoError(t, err)
}

func TestGetOldestPendingApprovalWithMultipleUserSignups(t *testing.T) {
	// given
	ctx := context.TODO()
	withoutStateLabel := commonsignup.NewUserSignup()
	// we need to create the UserSignups with different timestamp in the range of seconds because
	// the k8s resource keeps the time in RFC3339 format where the smallest unit are seconds.
	pending1 := commonsignup.NewUserSignup(commonsignup.WithStateLabel("pending"), commonsignup.CreatedBefore(5*time.Second), commonsignup.WithName("1-oldest"))
	pending2 := commonsignup.NewUserSignup(commonsignup.WithStateLabel("pending"), commonsignup.CreatedBefore(3*time.Second), commonsignup.WithName("2-oldest"))
	pending3 := commonsignup.NewUserSignup(commonsignup.WithStateLabel("pending"), commonsignup.CreatedBefore(2*time.Second), commonsignup.WithName("3-oldest"))
	cache, cl := newCache(t, &toolchainv1alpha1.UserSignup{}, listPendingUserSignups, withoutStateLabel, pending2, pending3, pending1)

	// when
	foundPending := cache.getOldestPendingObject(ctx, test.HostOperatorNs)

	// then
	assert.Len(t, cache.sortedObjectNames, 3)
	assert.Equal(t, pending1.Name, foundPending.GetName())
	approve(t, cl, pending1)

	t.Run("should keep two UserSignup since the pending4 hasn't been loaded yet", func(t *testing.T) {
		// given
		pending4 := commonsignup.NewUserSignup(commonsignup.WithStateLabel("pending"))
		err := cl.Create(context.TODO(), pending4)
		require.NoError(t, err)

		// when
		foundPending := cache.getOldestPendingObject(ctx, test.HostOperatorNs)

		// then
		assert.Len(t, cache.sortedObjectNames, 2)
		assert.Equal(t, pending2.Name, foundPending.GetName())
		approve(t, cl, pending2)

		t.Run("should keep one UserSignup since the pending4 hasn't been loaded yet and previous ones were removed", func(t *testing.T) {
			// when
			foundPending := cache.getOldestPendingObject(ctx, test.HostOperatorNs)

			// then
			assert.Len(t, cache.sortedObjectNames, 1)
			assert.Equal(t, pending3.Name, foundPending.GetName())
			approve(t, cl, pending3)

			t.Run("should load the pending4 resource", func(t *testing.T) {
				// when
				foundPending := cache.getOldestPendingObject(ctx, test.HostOperatorNs)

				// then
				assert.Len(t, cache.sortedObjectNames, 1)
				assert.Equal(t, pending4.Name, foundPending.GetName())
			})
		})
	})
}

func TestGetOldestPendingApprovalWithMultipleSpaces(t *testing.T) {
	// given
	ctx := context.TODO()
	withoutStateLabel := spacetest.NewSpace(test.HostOperatorNs, "without-state")
	// we need to create the Spaces with different timestamp in the range of seconds because
	// the k8s resource keeps the time in RFC3339 format where the smallest unit are seconds.
	pending1 := spacetest.NewSpace(test.HostOperatorNs, "1-oldest", spacetest.WithStateLabel("pending"), spacetest.CreatedBefore(5*time.Second))
	pending2 := spacetest.NewSpace(test.HostOperatorNs, "2-oldest", spacetest.WithStateLabel("pending"), spacetest.CreatedBefore(3*time.Second))
	pending3 := spacetest.NewSpace(test.HostOperatorNs, "3-oldest", spacetest.WithStateLabel("pending"), spacetest.CreatedBefore(2*time.Second))
	cache, cl := newCache(t, &toolchainv1alpha1.Space{}, listPendingSpaces, withoutStateLabel, pending2, pending3, pending1)

	// when
	foundPending := cache.getOldestPendingObject(ctx, test.HostOperatorNs)

	// then
	assert.Len(t, cache.sortedObjectNames, 3)
	assert.Equal(t, pending1.Name, foundPending.GetName())
	assignCluster(t, cl, pending1)

	t.Run("should keep two Spaces since the pending4 hasn't been loaded yet", func(t *testing.T) {
		// given
		pending4 := spacetest.NewSpace(test.HostOperatorNs, "pending-4", spacetest.WithStateLabel("pending"))
		err := cl.Create(context.TODO(), pending4)
		require.NoError(t, err)

		// when
		foundPending := cache.getOldestPendingObject(ctx, test.HostOperatorNs)

		// then
		assert.Len(t, cache.sortedObjectNames, 2)
		assert.Equal(t, pending2.Name, foundPending.GetName())
		assignCluster(t, cl, pending2)

		t.Run("should keep one Space since the pending4 hasn't been loaded yet and previous ones were removed", func(t *testing.T) {
			// when
			foundPending := cache.getOldestPendingObject(ctx, test.HostOperatorNs)

			// then
			assert.Len(t, cache.sortedObjectNames, 1)
			assert.Equal(t, pending3.Name, foundPending.GetName())
			assignCluster(t, cl, pending3)

			t.Run("should load the pending4 resource", func(t *testing.T) {
				// when
				foundPending := cache.getOldestPendingObject(ctx, test.HostOperatorNs)

				// then
				assert.Len(t, cache.sortedObjectNames, 1)
				assert.Equal(t, pending4.Name, foundPending.GetName())
			})
		})
	})
}

func TestGetOldestPendingApprovalWithMultipleUserSignupsInParallel(t *testing.T) {
	// given
	ctx := context.TODO()
	cache, cl := newCache(t, &toolchainv1alpha1.UserSignup{}, listPendingUserSignups)

	var latch sync.WaitGroup
	latch.Add(1)
	var waitForFinished sync.WaitGroup

	waitForFinished.Add(3000)
	for i := 0; i < 1000; i++ {
		go func() {
			defer waitForFinished.Done()
			latch.Wait()
			pending1 := commonsignup.NewUserSignup(commonsignup.WithStateLabel("pending"))
			pending2 := commonsignup.NewUserSignup(commonsignup.WithStateLabel("pending"))
			allSingups := []*toolchainv1alpha1.UserSignup{
				commonsignup.NewUserSignup(commonsignup.WithStateLabel("not-ready")),
				commonsignup.NewUserSignup(commonsignup.WithStateLabel("deactivated")),
				commonsignup.NewUserSignup(commonsignup.WithStateLabel("approved")),
				pending1,
				pending2,
			}
			for _, signup := range allSingups {
				err := cl.Create(ctx, signup)
				assert.NoError(t, err) // require must only be used in the goroutine running the test function (testifylint)
			}

			for _, pending := range []*toolchainv1alpha1.UserSignup{pending1, pending2} {
				go func(toApprove *toolchainv1alpha1.UserSignup) {
					defer waitForFinished.Done()
					oldestPendingApproval := cache.getOldestPendingObject(ctx, test.HostOperatorNs)
					assert.NotNil(t, oldestPendingApproval) // require must only be used in the goroutine running the test function (testifylint)
					approve(t, cl, toApprove)
				}(pending)
			}
		}()
	}

	// when
	latch.Done()
	waitForFinished.Wait()

	// when
	foundPending := cache.getOldestPendingObject(ctx, test.HostOperatorNs)

	// then
	assert.Nil(t, foundPending)
}

func newCache(t *testing.T, objectType runtimeclient.Object, listPendingObjects ListPendingObjects, initObjects ...runtime.Object) (*cache, *test.FakeClient) {
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
