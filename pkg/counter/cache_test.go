package counter_test

import (
	"context"
	"sync"
	"testing"

	"github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/counter"
	. "github.com/codeready-toolchain/host-operator/test"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/codeready-toolchain/toolchain-common/pkg/test/masteruserrecord"
	"github.com/stretchr/testify/require"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var logger = logf.Log.WithName("cache_test")

func TestAddMurToCounter(t *testing.T) {
	// given
	InitializeCounter(t, 0)
	defer counter.Reset()

	// when
	counter.IncrementMasterUserRecordCount()

	// then
	AssertThatCounterHas(t, 1)
}

func TestRemoveMurFromCounter(t *testing.T) {
	// given
	InitializeCounter(t, 2)
	defer counter.Reset()

	// when
	counter.DecrementMasterUserRecordCount(logger)

	// then
	AssertThatCounterHas(t, 1)
}

func TestRemoveMurFromCounterWhenIsAlreadyZero(t *testing.T) {
	// given
	InitializeCounter(t, 0)
	defer counter.Reset()

	// when
	counter.DecrementMasterUserRecordCount(logger)

	// then
	AssertThatCounterHas(t, 0)
}

func TestRemoveMurFromCounterWhenIsAlreadyZeroAndNotInitialized(t *testing.T) {
	// given
	defer counter.Reset()

	// when
	counter.DecrementMasterUserRecordCount(logger)

	// then
	AssertThatUninitializedCounterHas(t, -1)
}

func TestAddUserAccountToCounter(t *testing.T) {
	// given
	InitializeCounter(t, 1)
	defer counter.Reset()

	// when
	counter.IncrementUserAccountCount("member")

	// then
	AssertThatCounterHas(t, 1, UserAccountsForCluster("member", 1))
}

func TestRemoveUserAccountFromCounter(t *testing.T) {
	// given
	InitializeCounter(t, 1, UserAccountsForCluster("member", 2))
	defer counter.Reset()

	// when
	counter.DecrementUserAccountCount(logger, "member")

	// then
	AssertThatCounterHas(t, 1, UserAccountsForCluster("member", 1))
}

func TestRemoveUserAccountFromCounterWhenIsAlreadyZero(t *testing.T) {
	// given
	InitializeCounter(t, 2,
		UserAccountsForCluster("member", 0),
		UserAccountsForCluster("member2", 2))
	defer counter.Reset()

	// when
	counter.DecrementUserAccountCount(logger, "member")

	// then
	AssertThatCounterHas(t, 2,
		UserAccountsForCluster("member", 0),
		UserAccountsForCluster("member2", 2))
}

func TestRemoveUserAccountFromCounterWhenIsAlreadyZeroAndNotInitialized(t *testing.T) {
	// given
	defer counter.Reset()

	// when
	counter.DecrementUserAccountCount(logger, "member")

	// then
	AssertThatUninitializedCounterHas(t, 0,
		UserAccountsForCluster("member", -1))
}

func TestInitializeCounterFromToolchainCluster(t *testing.T) {
	// given
	defer counter.Reset()

	// when
	toolchainStatus := InitializeCounter(t, 13, UserAccountsForCluster("member", 10), UserAccountsForCluster("member2", 3))

	// then
	AssertThatCounterHas(t, 13, UserAccountsForCluster("member", 10), UserAccountsForCluster("member2", 3))
	AssertThatGivenToolchainStatus(t, toolchainStatus).
		HasMurCount(13).
		HasUserAccountCount("member", 10).
		HasUserAccountCount("member2", 3)
}

func TestInitializeCounterFromToolchainClusterWithNegativeNumbersInCache(t *testing.T) {
	// given
	defer counter.Reset()
	counter.DecrementUserAccountCount(logger, "member")
	counter.DecrementMasterUserRecordCount(logger)

	// when
	toolchainStatus := InitializeCounterWithoutReset(t, 13, UserAccountsForCluster("member", 10), UserAccountsForCluster("member2", 3))

	// then
	AssertThatCounterHas(t, 12, UserAccountsForCluster("member", 9), UserAccountsForCluster("member2", 3))
	AssertThatGivenToolchainStatus(t, toolchainStatus).
		HasMurCount(12).
		HasUserAccountCount("member", 9).
		HasUserAccountCount("member2", 3)
}

func TestInitializeCounterByLoadingExistingMurs(t *testing.T) {
	// given
	defer counter.Reset()
	//this will be ignored by resetting when loading existing MURs
	counter.IncrementMasterUserRecordCount()
	counter.IncrementUserAccountCount("member")

	murs := CreateMultipleMurs(t, 10)
	fakeClient := test.NewFakeClient(t, murs...)

	// when
	toolchainStatus := InitializeCounterWithClient(t, fakeClient, 0)

	// then
	AssertThatCounterHas(t, 10, UserAccountsForCluster("member-cluster", 10))
	AssertThatGivenToolchainStatus(t, toolchainStatus).
		HasMurCount(10).
		HasUserAccountCount("member-cluster", 10)
}

func TestShouldNotInitializeAgain(t *testing.T) {
	// given
	defer counter.Reset()
	//this will be ignored by resetting when loading existing MURs
	counter.IncrementMasterUserRecordCount()
	counter.IncrementUserAccountCount("member")

	murs := CreateMultipleMurs(t, 10)
	fakeClient := test.NewFakeClient(t, murs...)
	toolchainStatus := InitializeCounterWithClient(t, fakeClient, 0)
	err := fakeClient.Create(context.TODO(), masteruserrecord.NewMasterUserRecord(t, "ignored", masteruserrecord.TargetCluster("member-cluster")))
	require.NoError(t, err)
	toolchainStatus = &v1alpha1.ToolchainStatus{}

	// when
	err = counter.Synchronize(fakeClient, toolchainStatus)

	// then
	AssertThatCounterHas(t, 10, UserAccountsForCluster("member-cluster", 10))
	AssertThatGivenToolchainStatus(t, toolchainStatus).
		HasMurCount(10).
		HasUserAccountCount("member-cluster", 10)
}

func TestMultipleExecutionsInParallel(t *testing.T) {
	// given
	defer counter.Reset()
	murs := CreateMultipleMurs(t, 10)
	fakeClient := test.NewFakeClient(t, murs...)
	toolchainStatus := InitializeCounterWithClient(t, fakeClient, 0)
	var latch sync.WaitGroup
	latch.Add(1)
	var waitForFinished sync.WaitGroup

	for i := 0; i < 1002; i++ {
		waitForFinished.Add(3)
		if i < 1000 {
			waitForFinished.Add(3)
		}
		go func(index int) {
			defer waitForFinished.Done()
			latch.Wait()
			counter.IncrementMasterUserRecordCount()
			if index < 1000 {
				go func() {
					defer waitForFinished.Done()
					counter.DecrementMasterUserRecordCount(logger)
				}()
			}
		}(i)
		go func(index int) {
			defer waitForFinished.Done()
			latch.Wait()
			counter.IncrementUserAccountCount("member")
			if index < 1000 {
				go func() {
					defer waitForFinished.Done()
					counter.DecrementUserAccountCount(logger, "member")
				}()
			}
		}(i)
		go func(index int) {
			defer waitForFinished.Done()
			latch.Wait()
			counter.IncrementUserAccountCount("member-cluster")
			if index < 1000 {
				go func() {
					defer waitForFinished.Done()
					counter.DecrementUserAccountCount(logger, "member-cluster")
				}()
			}
		}(i)
	}

	for i := 0; i < 102; i++ {
		go func() {
			waitForFinished.Add(1)
			defer waitForFinished.Done()
			latch.Wait()
			err := counter.Synchronize(fakeClient, toolchainStatus)
			require.NoError(t, err)
		}()
	}

	// when
	latch.Done()
	waitForFinished.Wait()
	err := counter.Synchronize(fakeClient, toolchainStatus)

	// then
	require.NoError(t, err)
	AssertThatCounterHas(t, 12, UserAccountsForCluster("member-cluster", 12), UserAccountsForCluster("member", 2))
	AssertThatGivenToolchainStatus(t, toolchainStatus).
		HasMurCount(12).
		HasUserAccountCount("member-cluster", 12).
		HasUserAccountCount("member", 2)
}
