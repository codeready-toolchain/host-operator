package counter_test

import (
	"context"
	"sync"
	"testing"

	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/counter"
	"github.com/codeready-toolchain/host-operator/pkg/metrics"
	. "github.com/codeready-toolchain/host-operator/test"
	commonconfig "github.com/codeready-toolchain/toolchain-common/pkg/configuration"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	testconfig "github.com/codeready-toolchain/toolchain-common/pkg/test/config"
	"github.com/codeready-toolchain/toolchain-common/pkg/test/masteruserrecord"
	spacetest "github.com/codeready-toolchain/toolchain-common/pkg/test/space"

	"github.com/stretchr/testify/assert"
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
	counter.IncrementMasterUserRecordCount(logger, metrics.Internal)

	// then
	AssertThatCountersAndMetrics(t).
		HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{string(metrics.Internal): 1})
}

func TestRemoveMurFromCounter(t *testing.T) {
	// given
	InitializeCounters(t, NewToolchainStatus(
		WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
			"1,external": 1,
			"1,internal": 1,
		}),
		WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
			string(metrics.Internal): 1,
			string(metrics.External): 1,
		})))
	defer counter.Reset()

	// when
	counter.DecrementMasterUserRecordCount(logger, metrics.Internal)

	// then
	AssertThatCountersAndMetrics(t).
		HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
			string(metrics.Internal): 0,
			string(metrics.External): 1,
		})
}

func TestRemoveMurFromCounterWhenIsAlreadyZero(t *testing.T) {
	// given
	InitializeCounters(t, NewToolchainStatus())
	defer counter.Reset()

	// when
	counter.DecrementMasterUserRecordCount(logger, metrics.Internal)

	// then
	AssertThatCountersAndMetrics(t).
		HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{})
}

func TestRemoveMurFromCounterWhenIsAlreadyZeroAndNotInitialized(t *testing.T) {
	// given
	defer counter.Reset()

	// when
	counter.DecrementMasterUserRecordCount(logger, metrics.Internal)

	// then
	AssertThatUninitializedCounters(t).
		HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{string(metrics.Internal): -1})
}

// spaces tests ----------

func TestAddSpaceToCounter(t *testing.T) {
	// given
	InitializeCounters(t, NewToolchainStatus(
		WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
			"1,internal": 1,
		}),
		WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
			string(metrics.Internal): 1,
		}),
	))
	defer counter.Reset()

	// when
	counter.IncrementSpaceCount(logger, "member-1")

	// then
	AssertThatCountersAndMetrics(t).
		HaveSpacesForCluster("member-1", 1).
		HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
			string(metrics.Internal): 1,
		})
}

func TestRemoveSpaceFromCounter(t *testing.T) {
	// given
	InitializeCounters(t,
		NewToolchainStatus(
			WithMember("member-1", WithSpaceCount(2)),
			WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
				"1,internal": 1,
			}),
			WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
				string(metrics.Internal): 1,
			}),
		))
	defer counter.Reset()

	// when
	counter.DecrementSpaceCount(logger, "member-1")

	// then
	AssertThatCountersAndMetrics(t).
		HaveSpacesForCluster("member-1", 1)
}

func TestRemoveSpaceFromCounterWhenIsAlreadyZero(t *testing.T) {
	// given
	InitializeCounters(t,
		NewToolchainStatus(
			WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
				"1,internal": 1,
				"1,external": 1,
			}),
			WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
				string(metrics.Internal): 1,
				string(metrics.External): 1,
			}),
			WithMember("member-1", WithSpaceCount(0)),
			WithMember("member-2", WithSpaceCount(2))))
	defer counter.Reset()

	// when
	counter.DecrementSpaceCount(logger, "member-1")

	// then
	AssertThatCountersAndMetrics(t).
		HaveSpacesForCluster("member-1", 0).
		HaveSpacesForCluster("member-2", 2)
}

func TestRemoveSpaceFromCounterWhenIsAlreadyZeroAndNotInitialized(t *testing.T) {
	// given
	defer counter.Reset()

	// when
	counter.DecrementSpaceCount(logger, "member-1")

	// then
	AssertThatUninitializedCounters(t).
		HaveSpacesForCluster("member-1", -1)
}

// end spaces tests ------

func TestUpdateUsersPerActivationMetric(t *testing.T) {
	// given
	toolchainStatus := NewToolchainStatus()
	InitializeCounters(t, toolchainStatus)

	// when
	counter.UpdateUsersPerActivationCounters(logger, 1, metrics.Internal) // a user signup once
	counter.UpdateUsersPerActivationCounters(logger, 2, metrics.Internal) // a user signup twice (hence counter for "1" will be decreased)

	// then
	AssertThatCountersAndMetrics(t).
		HaveUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
			"1,internal": 0,
			"2,internal": 1,
		})
	AssertThatGivenToolchainStatus(t, toolchainStatus).
		HasUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
			"1,internal": 0,
			"2,internal": 1,
		})
}

func TestInitializeCounterFromToolchainCluster(t *testing.T) {
	// given
	toolchainStatus := NewToolchainStatus(
		WithMember("member-1", WithSpaceCount(10)),
		WithMember("member-2", WithSpaceCount(3)),
		WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
			"1,internal": 7,
			"1,external": 2,
			"2,internal": 1,
			"2,external": 3,
		}),
		WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
			string(metrics.Internal): 8,
			string(metrics.External): 5,
		}),
	)

	// when
	InitializeCounters(t, toolchainStatus)

	// then
	AssertThatCountersAndMetrics(t).
		HaveSpacesForCluster("member-1", 10).
		HaveSpacesForCluster("member-2", 3).
		HaveUsersPerActivationsAndDomain(map[string]int{
			"1,internal": 7,
			"1,external": 2,
			"2,internal": 1,
			"2,external": 3,
		}).
		HaveMasterUserRecordsPerDomain(map[string]int{
			string(metrics.Internal): 8,
			string(metrics.External): 5,
		})
	AssertThatGivenToolchainStatus(t, toolchainStatus).
		HasSpaceCount("member-1", 10).
		HasSpaceCount("member-2", 3).
		HasUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
			"1,internal": 7,
			"1,external": 2,
			"2,internal": 1,
			"2,external": 3,
		}).
		HasMasterUserRecordsPerDomain(map[string]int{
			string(metrics.Internal): 8,
			string(metrics.External): 5,
		})
}

func TestInitializeCounterFromToolchainClusterWithoutReset(t *testing.T) {
	// given
	counter.DecrementSpaceCount(logger, "member-1")
	counter.DecrementMasterUserRecordCount(logger, metrics.Internal)
	counter.UpdateUsersPerActivationCounters(logger, 1, metrics.Internal) // a user signup once
	counter.UpdateUsersPerActivationCounters(logger, 2, metrics.Internal) // a user signup twice (hence counter for "1" will be decreased)
	toolchainStatus := NewToolchainStatus(
		WithMember("member-1", WithSpaceCount(10)),
		WithMember("member-2", WithSpaceCount(3)),
		WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
			"1,internal": 10,
			"1,external": 3,
		}),
		WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
			string(metrics.Internal): 8,
			string(metrics.External): 5,
		}))

	// when
	InitializeCountersWithoutReset(t, toolchainStatus)

	// then
	AssertThatCountersAndMetrics(t).
		HaveSpacesForCluster("member-1", 9).
		HaveSpacesForCluster("member-2", 3).
		HaveUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
			"1,internal": 10,
			"1,external": 3,
			"2,internal": 1,
		}).
		HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
			string(metrics.Internal): 7, // decremented
			string(metrics.External): 5,
		})
	AssertThatGivenToolchainStatus(t, toolchainStatus).
		HasSpaceCount("member-1", 9).
		HasSpaceCount("member-2", 3).
		HasUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
			"1,internal": 10,
			"1,external": 3,
			"2,internal": 1,
		}).
		HasMasterUserRecordsPerDomain(map[string]int{
			string(metrics.Internal): 7, // decremented
			string(metrics.External): 5,
		})
}

func TestInitializeCounterByLoadingExistingResources(t *testing.T) {
	// given
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	//this will be ignored by resetting when initializing counters
	counter.IncrementMasterUserRecordCount(logger, metrics.Internal)
	counter.IncrementSpaceCount(logger, "member-1")

	usersignups := CreateMultipleUserSignups("user-", 3) // all users have an `@redhat.com` email address
	murs := CreateMultipleMurs(t, "user-", 3, "member-1")
	spaces := CreateMultipleSpaces("user-", 3, "member-1")
	toolchainStatus := NewToolchainStatus(
		WithMember("member-1", WithSpaceCount(0)),
		WithMetric("outdated", toolchainv1alpha1.Metric{
			"cookie":    1,
			"chocolate": 2,
		}),
	)
	initObjs := append([]runtimeclient.Object{}, murs...)
	initObjs = append(initObjs, usersignups...)
	initObjs = append(initObjs, spaces...)

	// when
	InitializeCounters(t, toolchainStatus, initObjs...)

	// then
	AssertThatCountersAndMetrics(t).
		HaveSpacesForCluster("member-1", 3).
		HaveUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
			"1,internal": 1,
			"2,internal": 1,
			"3,internal": 1,
		}).
		HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
			string(metrics.Internal): 3, // all MURs have `@redhat.com` email address
		})
	AssertThatGivenToolchainStatus(t, toolchainStatus).
		HasSpaceCount("member-1", 3).
		HasUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
			"1,internal": 1,
			"2,internal": 1,
			"3,internal": 1,
		}).
		HasMasterUserRecordsPerDomain(map[string]int{
			string(metrics.Internal): 3, // all MURs have `@redhat.com` email address
		}).
		HasNoMetric("outdated")
}

func TestForceInitializeCounterByLoadingExistingResources(t *testing.T) {
	// given
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	//this will be ignored by resetting when initializing counters
	counter.IncrementMasterUserRecordCount(logger, metrics.Internal)

	usersignups := CreateMultipleUserSignups("user-", 3) // all users have an `@redhat.com` email address
	murs := CreateMultipleMurs(t, "user-", 3, "member-1")
	spaces := CreateMultipleSpaces("user-", 3, "member-1")
	// all expected metrics are set ...
	toolchainStatus := NewToolchainStatus(
		WithMember("member-1", WithSpaceCount(0)),
		WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
			string(metrics.Internal): 1,
			string(metrics.External): 1,
		}),
		WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
			"1,external": 1,
			"1,internal": 1,
		}),
		// outdated metric which should be removed during the initialization
		WithMetric("outdated", toolchainv1alpha1.Metric{
			"cookie":    1,
			"chocolate": 2,
		}),
	)
	// ... but config flag will force synchronization from resources
	toolchainConfig := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.Metrics().ForceSynchronization(true))
	initObjs := append([]runtimeclient.Object{}, murs...)
	initObjs = append(initObjs, usersignups...)
	initObjs = append(initObjs, toolchainConfig)
	initObjs = append(initObjs, spaces...)

	// when
	InitializeCounters(t, toolchainStatus, initObjs...)

	// then
	AssertThatCountersAndMetrics(t).
		HaveSpacesForCluster("member-1", 3).
		HaveUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
			"1,internal": 1,
			"2,internal": 1,
			"3,internal": 1,
		}).
		HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
			string(metrics.Internal): 3, // all MURs have `@redhat.com` email address
		})
	AssertThatGivenToolchainStatus(t, toolchainStatus).
		HasSpaceCount("member-1", 3).
		HasUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
			"1,internal": 1,
			"2,internal": 1,
			"3,internal": 1,
		}).
		HasMasterUserRecordsPerDomain(map[string]int{
			string(metrics.Internal): 3, // all MURs have `@redhat.com` email address
		}).
		HasNoMetric("outdated")
}

func TestShouldNotInitializeAgain(t *testing.T) {
	// given
	//this will be ignored by resetting when loading existing MURs
	counter.IncrementMasterUserRecordCount(logger, metrics.Internal)
	counter.IncrementSpaceCount(logger, "member-1")

	murs := CreateMultipleMurs(t, "user-", 10, "member-1")
	spaces := CreateMultipleSpaces("user-", 10, "member-1")
	toolchainStatus := NewToolchainStatus(
		WithMember("member-1", WithSpaceCount(0)))
	initObjs := append([]runtimeclient.Object{}, murs...)
	initObjs = append(initObjs, spaces...)
	InitializeCounters(t, toolchainStatus, initObjs...)
	fakeClient := test.NewFakeClient(t, initObjs...)
	err := fakeClient.Create(context.TODO(), masteruserrecord.NewMasterUserRecord(t, "ignored", masteruserrecord.TargetCluster("member-1")))
	require.NoError(t, err)
	err = fakeClient.Create(context.TODO(), spacetest.NewSpace(test.HostOperatorNs, "ignored", spacetest.WithSpecTargetCluster("member-1")))
	require.NoError(t, err)

	// when
	err = counter.Synchronize(context.TODO(), fakeClient, toolchainStatus)

	// then
	require.NoError(t, err)
	AssertThatCountersAndMetrics(t).
		HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
			string(metrics.Internal): 10, // all MURs have `@redhat.com` email address
		}).
		HaveSpacesForCluster("member-1", 10)
	AssertThatGivenToolchainStatus(t, toolchainStatus).
		HasMasterUserRecordsPerDomain(map[string]int{
			string(metrics.Internal): 10, // all MURs have `@redhat.com` email address
		}).
		HasSpaceCount("member-1", 10)
}

func TestMultipleExecutionsInParallel(t *testing.T) {
	// given
	murs := CreateMultipleMurs(t, "user-", 10, "member-1")
	spaces := CreateMultipleSpaces("user-", 10, "member-1")
	toolchainStatus := NewToolchainStatus(
		WithMember("member-1", WithSpaceCount(0)),
		WithMember("member-2", WithSpaceCount(0)))
	initObjs := append([]runtimeclient.Object{}, murs...)
	initObjs = append(initObjs, spaces...)
	InitializeCounters(t, toolchainStatus, initObjs...)
	fakeClient := test.NewFakeClient(t, initObjs...)
	var latch sync.WaitGroup
	latch.Add(1)
	var waitForFinished sync.WaitGroup

	for i := 0; i < 1002; i++ {
		waitForFinished.Add(4) // 4 routines to increment counters
		if i < 1000 {
			waitForFinished.Add(4) // 4 routines to decrement counters until 1000th iteration
		}
		go func(index int) {
			defer waitForFinished.Done()
			latch.Wait()
			counter.IncrementMasterUserRecordCount(logger, metrics.Internal)
			if index < 1000 {
				go func() {
					defer waitForFinished.Done()
					counter.DecrementMasterUserRecordCount(logger, metrics.Internal)
				}()
			}
		}(i)
		go func(index int) {
			defer waitForFinished.Done()
			latch.Wait()
			counter.IncrementSpaceCount(logger, "member-2")
			if index < 1000 {
				go func() {
					defer waitForFinished.Done()
					counter.DecrementSpaceCount(logger, "member-2")
				}()
			}
		}(i)
		go func(index int) {
			defer waitForFinished.Done()
			latch.Wait()
			counter.IncrementSpaceCount(logger, "member-1")
			if index < 1000 {
				go func() {
					defer waitForFinished.Done()
					counter.DecrementSpaceCount(logger, "member-1")
				}()
			}
		}(i)
		go func(index int) {
			defer waitForFinished.Done()
			latch.Wait()
			counter.UpdateUsersPerActivationCounters(logger, 1, metrics.Internal) // increment metric for internal users with 1 activation
			if index < 1000 {
				go func() {
					defer waitForFinished.Done()
					counter.UpdateUsersPerActivationCounters(logger, 2, metrics.Internal) // increment metric for internal users with 2 activations and decrement metric for internal users with 1 activation
				}()
			}
		}(i)
	}

	for i := 0; i < 102; i++ {
		waitForFinished.Add(1)
		go func() {
			defer waitForFinished.Done()
			latch.Wait()
			err := counter.Synchronize(context.TODO(), fakeClient, toolchainStatus)
			assert.NoError(t, err) // require must only be used in the goroutine running the test function (testifylint)
		}()
	}

	// when
	latch.Done()
	waitForFinished.Wait()
	err := counter.Synchronize(context.TODO(), fakeClient, toolchainStatus)

	// then
	require.NoError(t, err)
	AssertThatCountersAndMetrics(t).
		HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
			string(metrics.Internal): 12, // all MURs have `@redhat.com` email address
		}).
		HaveSpacesForCluster("member-1", 12).
		HaveSpacesForCluster("member-2", 2).
		HaveUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
			"1,internal": 2,
			"2,internal": 1000,
		})
	AssertThatGivenToolchainStatus(t, toolchainStatus).
		HasMasterUserRecordsPerDomain(map[string]int{
			string(metrics.Internal): 12, // all MURs have `@redhat.com` email address
		}).
		HasSpaceCount("member-1", 12).
		HasSpaceCount("member-2", 2).
		HasUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
			"1,internal": 2,
			"2,internal": 1000,
		})
}
