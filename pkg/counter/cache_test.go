package counter_test

import (
	"context"
	"sync"
	"testing"

	"github.com/codeready-toolchain/host-operator/pkg/counter"
	. "github.com/codeready-toolchain/host-operator/test"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/codeready-toolchain/toolchain-common/pkg/test/masteruserrecord"

	"github.com/stretchr/testify/require"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var logger = logf.Log.WithName("cache_test")

func TestAddMurToCounter(t *testing.T) {
	// given
	InitializeCounters(t, NewToolchainStatus())
	defer counter.Reset()

	// when
	counter.IncrementMasterUserRecordCount()

	// then
	AssertThatCounters(t).HaveMasterUserRecords(1)
}

func TestRemoveMurFromCounter(t *testing.T) {
	// given
	InitializeCounters(t, NewToolchainStatus(WithHost(WithMasterUserRecordCount(2))))
	defer counter.Reset()

	// when
	counter.DecrementMasterUserRecordCount(logger)

	// then
	AssertThatCounters(t).HaveMasterUserRecords(1)
}

func TestRemoveMurFromCounterWhenIsAlreadyZero(t *testing.T) {
	// given
	InitializeCounters(t, NewToolchainStatus())
	defer counter.Reset()

	// when
	counter.DecrementMasterUserRecordCount(logger)

	// then
	AssertThatCounters(t).HaveMasterUserRecords(0)
}

func TestRemoveMurFromCounterWhenIsAlreadyZeroAndNotInitialized(t *testing.T) {
	// given
	defer counter.Reset()

	// when
	counter.DecrementMasterUserRecordCount(logger)

	// then
	AssertThatUninitializedCounters(t).HaveMasterUserRecords(-1)
}

func TestAddUserAccountToCounter(t *testing.T) {
	// given
	InitializeCounters(t, NewToolchainStatus(WithHost(WithMasterUserRecordCount(1))))
	defer counter.Reset()

	// when
	counter.IncrementUserAccountCount("member-1")

	// then
	AssertThatCounters(t).HaveMasterUserRecords(1).HaveUserAccountsForCluster("member-1", 1)
}

func TestRemoveUserAccountFromCounter(t *testing.T) {
	// given
	InitializeCounters(t,
		NewToolchainStatus(
			WithHost(WithMasterUserRecordCount(1)),
			WithMember("member-1", WithUserAccountCount(2))))
	defer counter.Reset()

	// when
	counter.DecrementUserAccountCount(logger, "member-1")

	// then
	AssertThatCounters(t).HaveMasterUserRecords(1).HaveUserAccountsForCluster("member-1", 1)
}

func TestRemoveUserAccountFromCounterWhenIsAlreadyZero(t *testing.T) {
	// given
	InitializeCounters(t,
		NewToolchainStatus(
			WithHost(WithMasterUserRecordCount(2)),
			WithMember("member-1", WithUserAccountCount(0)),
			WithMember("member-2", WithUserAccountCount(2))))
	defer counter.Reset()

	// when
	counter.DecrementUserAccountCount(logger, "member-1")

	// then
	AssertThatCounters(t).HaveMasterUserRecords(2).
		HaveUserAccountsForCluster("member-1", 0).
		HaveUserAccountsForCluster("member-2", 2)
}

func TestRemoveUserAccountFromCounterWhenIsAlreadyZeroAndNotInitialized(t *testing.T) {
	// given
	defer counter.Reset()

	// when
	counter.DecrementUserAccountCount(logger, "member-1")

	// then
	AssertThatUninitializedCounters(t).HaveMasterUserRecords(0).
		HaveUserAccountsForCluster("member-1", -1)
}

func TestInitializeCounterFromToolchainCluster(t *testing.T) {
	// given
	toolchainStatus := NewToolchainStatus(
		WithHost(WithMasterUserRecordCount(13)),
		WithMember("member-1", WithUserAccountCount(10)),
		WithMember("member-2", WithUserAccountCount(3)))

	// when
	InitializeCounters(t, toolchainStatus)

	// then
	AssertThatCounters(t).HaveMasterUserRecords(13).
		HaveUserAccountsForCluster("member-1", 10).
		HaveUserAccountsForCluster("member-2", 3)
	AssertThatGivenToolchainStatus(t, toolchainStatus).
		HasMurCount(13).
		HasUserAccountCount("member-1", 10).
		HasUserAccountCount("member-2", 3)
}

func TestInitializeCounterFromToolchainClusterWithNegativeNumbersInCache(t *testing.T) {
	// given
	counter.DecrementUserAccountCount(logger, "member-1")
	counter.DecrementMasterUserRecordCount(logger)
	toolchainStatus := NewToolchainStatus(
		WithHost(WithMasterUserRecordCount(13)),
		WithMember("member-1", WithUserAccountCount(10)),
		WithMember("member-2", WithUserAccountCount(3)))

	// when
	InitializeCountersWithoutReset(t, toolchainStatus)

	// then
	AssertThatCounters(t).HaveMasterUserRecords(12).
		HaveUserAccountsForCluster("member-1", 9).
		HaveUserAccountsForCluster("member-2", 3)
	AssertThatGivenToolchainStatus(t, toolchainStatus).
		HasMurCount(12).
		HasUserAccountCount("member-1", 9).
		HasUserAccountCount("member-2", 3)
}

func TestInitializeCounterByLoadingExistingResources(t *testing.T) {
	// given
	logf.SetLogger(zap.Logger(true))
	//this will be ignored by resetting when initializing counters
	counter.IncrementMasterUserRecordCount()
	counter.IncrementUserAccountCount("member-1")

	murs := CreateMultipleMurs(t, "user-", 10, "member-1")
	toolchainStatus := NewToolchainStatus(
		WithHost(WithMasterUserRecordCount(0)),
		WithMember("member-1", WithUserAccountCount(0)))

	// when
	InitializeCounters(t, toolchainStatus, murs...)

	// then
	AssertThatCounters(t).HaveMasterUserRecords(10).
		HaveUserAccountsForCluster("member-1", 10)
	AssertThatGivenToolchainStatus(t, toolchainStatus).
		HasMurCount(10).
		HasUserAccountCount("member-1", 10)
}

func TestShouldNotInitializeAgain(t *testing.T) {
	// given
	//this will be ignored by resetting when loading existing MURs
	counter.IncrementMasterUserRecordCount()
	counter.IncrementUserAccountCount("member-1")

	murs := CreateMultipleMurs(t, "user-", 10, "member-1")
	toolchainStatus := NewToolchainStatus(
		WithHost(WithMasterUserRecordCount(0)),
		WithMember("member-1", WithUserAccountCount(0)))
	InitializeCounters(t, toolchainStatus, murs...)
	fakeClient := test.NewFakeClient(t, murs...)
	err := fakeClient.Create(context.TODO(), masteruserrecord.NewMasterUserRecord(t, "ignored", masteruserrecord.TargetCluster("member-1")))
	require.NoError(t, err)

	// when
	err = counter.Synchronize(fakeClient, toolchainStatus)

	// then
	require.NoError(t, err)
	AssertThatCounters(t).HaveMasterUserRecords(10).
		HaveUserAccountsForCluster("member-1", 10)
	AssertThatGivenToolchainStatus(t, toolchainStatus).
		HasMurCount(10).
		HasUserAccountCount("member-1", 10)
}

func TestMultipleExecutionsInParallel(t *testing.T) {
	// given
	murs := CreateMultipleMurs(t, "user-", 10, "member-1")
	toolchainStatus := NewToolchainStatus(
		WithHost(WithMasterUserRecordCount(0)),
		WithMember("member-1", WithUserAccountCount(0)),
		WithMember("member-2", WithUserAccountCount(0)))
	InitializeCounters(t, toolchainStatus, murs...)
	fakeClient := test.NewFakeClient(t, murs...)
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
			counter.IncrementUserAccountCount("member-2")
			if index < 1000 {
				go func() {
					defer waitForFinished.Done()
					counter.DecrementUserAccountCount(logger, "member-2")
				}()
			}
		}(i)
		go func(index int) {
			defer waitForFinished.Done()
			latch.Wait()
			counter.IncrementUserAccountCount("member-1")
			if index < 1000 {
				go func() {
					defer waitForFinished.Done()
					counter.DecrementUserAccountCount(logger, "member-1")
				}()
			}
		}(i)
	}

	for i := 0; i < 102; i++ {
		waitForFinished.Add(1)
		go func() {
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
	AssertThatCounters(t).HaveMasterUserRecords(12).
		HaveUserAccountsForCluster("member-1", 12).
		HaveUserAccountsForCluster("member-2", 2)
	AssertThatGivenToolchainStatus(t, toolchainStatus).
		HasMurCount(12).
		HasUserAccountCount("member-1", 12).
		HasUserAccountCount("member-2", 2)
}
