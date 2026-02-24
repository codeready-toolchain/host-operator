package metrics_test

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/metrics"
	metricstest "github.com/codeready-toolchain/host-operator/test/metrics"
	toolchainstatustest "github.com/codeready-toolchain/host-operator/test/toolchainstatus"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/codeready-toolchain/toolchain-common/pkg/test/masteruserrecord"
	metricscommontest "github.com/codeready-toolchain/toolchain-common/pkg/test/metrics"
	"github.com/codeready-toolchain/toolchain-common/pkg/test/space"
	"github.com/codeready-toolchain/toolchain-common/pkg/test/usersignup"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var logger = logf.Log.WithName("metrics_test")

func TestRegisterCustomMetrics(t *testing.T) {
	// when
	collectors := metrics.RegisterCustomMetrics()

	// then
	// verify that all metrics were registered successfully
	assert.Len(t, collectors, 14)
}

func TestResetMetrics(t *testing.T) {

	// when
	metrics.UserSignupUniqueTotal.Inc()
	metrics.SpaceGaugeVec.WithLabelValues("member-1").Set(20)
	metrics.UserSignupProvisionTimeHistogram.Observe(1)

	metrics.Reset()

	// then
	metricscommontest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupUniqueTotal)
	metricscommontest.AssertMetricsGaugeEquals(t, 0, metrics.SpaceGaugeVec.WithLabelValues("member-1"))
	metricscommontest.AssertAllHistogramBucketsAreEmpty(t, metrics.UserSignupProvisionTimeHistogram)
}

func TestAddMurToCounter(t *testing.T) {
	// given
	metricstest.InitializeCountersWithMetricsSyncDisabled(t, toolchainstatustest.NewToolchainStatus())
	defer metrics.Reset()

	// when
	metrics.IncrementMasterUserRecordCount(logger, metrics.Internal)

	// then
	metricstest.AssertThatCountersAndMetrics(t).
		HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{string(metrics.Internal): 1})
}

func TestRemoveMurFromCounter(t *testing.T) {
	// given
	metricstest.InitializeCountersWithMetricsSyncDisabled(t, toolchainstatustest.NewToolchainStatus(
		toolchainstatustest.WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
			"1,external": 1,
			"1,internal": 1,
		}),
		toolchainstatustest.WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
			string(metrics.Internal): 1,
			string(metrics.External): 1,
		})))
	defer metrics.Reset()

	// when
	metrics.DecrementMasterUserRecordCount(logger, metrics.Internal)

	// then
	metricstest.AssertThatCountersAndMetrics(t).
		HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
			string(metrics.Internal): 0,
			string(metrics.External): 1,
		})
}

func TestRemoveMurFromCounterWhenIsAlreadyZero(t *testing.T) {
	// given
	metricstest.InitializeCountersWithMetricsSyncDisabled(t, toolchainstatustest.NewToolchainStatus())
	defer metrics.Reset()

	// when
	metrics.DecrementMasterUserRecordCount(logger, metrics.Internal)

	// then
	metricstest.AssertThatCountersAndMetrics(t).
		HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{})
}

func TestRemoveMurFromCounterWhenIsAlreadyZeroAndNotInitialized(t *testing.T) {
	// given
	defer metrics.Reset()

	// when
	metrics.DecrementMasterUserRecordCount(logger, metrics.Internal)

	// then
	metricstest.AssertThatUninitializedCounters(t).
		HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{string(metrics.Internal): -1})
}

// spaces tests ----------

func TestAddSpaceToCounter(t *testing.T) {
	// given
	metricstest.InitializeCountersWithMetricsSyncDisabled(t, toolchainstatustest.NewToolchainStatus(
		toolchainstatustest.WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
			"1,internal": 1,
		}),
		toolchainstatustest.WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
			string(metrics.Internal): 1,
		}),
	))
	defer metrics.Reset()

	// when
	metrics.IncrementSpaceCount(logger, "member-1")

	// then
	metricstest.AssertThatCountersAndMetrics(t).
		HaveSpacesForCluster("member-1", 1).
		HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
			string(metrics.Internal): 1,
		})
}

func TestRemoveSpaceFromCounter(t *testing.T) {
	// given
	metricstest.InitializeCountersWithMetricsSyncDisabled(t,
		toolchainstatustest.NewToolchainStatus(
			toolchainstatustest.WithMember("member-1", toolchainstatustest.WithSpaceCount(2)),
			toolchainstatustest.WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
				"1,internal": 1,
			}),
			toolchainstatustest.WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
				string(metrics.Internal): 1,
			}),
		))
	defer metrics.Reset()

	// when
	metrics.DecrementSpaceCount(logger, "member-1")

	// then
	metricstest.AssertThatCountersAndMetrics(t).
		HaveSpacesForCluster("member-1", 1)
}

func TestRemoveSpaceFromCounterWhenIsAlreadyZero(t *testing.T) {
	// given
	metricstest.InitializeCountersWithMetricsSyncDisabled(t,
		toolchainstatustest.NewToolchainStatus(
			toolchainstatustest.WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
				"1,internal": 1,
				"1,external": 1,
			}),
			toolchainstatustest.WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
				string(metrics.Internal): 1,
				string(metrics.External): 1,
			}),
			toolchainstatustest.WithMember("member-1", toolchainstatustest.WithSpaceCount(0)),
			toolchainstatustest.WithMember("member-2", toolchainstatustest.WithSpaceCount(2))))
	defer metrics.Reset()

	// when
	metrics.DecrementSpaceCount(logger, "member-1")

	// then
	metricstest.AssertThatCountersAndMetrics(t).
		HaveSpacesForCluster("member-1", 0).
		HaveSpacesForCluster("member-2", 2)
}

func TestRemoveSpaceFromCounterWhenIsAlreadyZeroAndNotInitialized(t *testing.T) {
	// given
	defer metrics.Reset()

	// when
	metrics.DecrementSpaceCount(logger, "member-1")

	// then
	metricstest.AssertThatUninitializedCounters(t).
		HaveSpacesForCluster("member-1", -1)
}

// end spaces tests ------

func TestUpdateUsersPerActivationMetric(t *testing.T) {
	// given
	toolchainStatus := toolchainstatustest.NewToolchainStatus()
	metricstest.InitializeCountersWithMetricsSyncDisabled(t, toolchainStatus)

	// when
	metrics.UpdateUsersPerActivationCounters(logger, 1, metrics.Internal) // a user signup once
	metrics.UpdateUsersPerActivationCounters(logger, 2, metrics.Internal) // a user signup twice (hence counter for "1" will be decreased)
	err := metrics.Synchronize(context.TODO(), test.NewFakeClient(t), toolchainStatus)

	// then
	require.NoError(t, err)
	metricstest.AssertThatCountersAndMetrics(t).
		HaveUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
			"1,internal": 0,
			"2,internal": 1,
		})
	toolchainstatustest.AssertThatGivenToolchainStatus(t, toolchainStatus).
		HasUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
			"1,internal": 0,
			"2,internal": 1,
		})
}

func TestInitializeCounterFromToolchainCluster(t *testing.T) {
	// given
	toolchainStatus := toolchainstatustest.NewToolchainStatus(
		toolchainstatustest.WithMember("member-1", toolchainstatustest.WithSpaceCount(10)),
		toolchainstatustest.WithMember("member-2", toolchainstatustest.WithSpaceCount(3)),
		toolchainstatustest.WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
			"1,internal": 7,
			"1,external": 2,
			"2,internal": 1,
			"2,external": 3,
		}),
		toolchainstatustest.WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
			string(metrics.Internal): 8,
			string(metrics.External): 5,
		}),
	)

	// when
	metricstest.InitializeCountersWithMetricsSyncDisabled(t, toolchainStatus)

	// then
	metricstest.AssertThatCountersAndMetrics(t).
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
	toolchainstatustest.AssertThatGivenToolchainStatus(t, toolchainStatus).
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
	metrics.DecrementSpaceCount(logger, "member-1")
	metrics.DecrementMasterUserRecordCount(logger, metrics.Internal)
	metrics.UpdateUsersPerActivationCounters(logger, 1, metrics.Internal) // a user signup once
	metrics.UpdateUsersPerActivationCounters(logger, 2, metrics.Internal) // a user signup twice (hence counter for "1" will be decreased)
	toolchainStatus := toolchainstatustest.NewToolchainStatus(
		toolchainstatustest.WithMember("member-1", toolchainstatustest.WithSpaceCount(10)),
		toolchainstatustest.WithMember("member-2", toolchainstatustest.WithSpaceCount(3)),
		toolchainstatustest.WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
			"1,internal": 10,
			"1,external": 3,
		}),
		toolchainstatustest.WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
			string(metrics.Internal): 8,
			string(metrics.External): 5,
		}))

	// when
	metricstest.InitializeCountersWithoutReset(t, toolchainStatus)

	// then
	metricstest.AssertThatCountersAndMetrics(t).
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
	toolchainstatustest.AssertThatGivenToolchainStatus(t, toolchainStatus).
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
	metrics.IncrementMasterUserRecordCount(logger, metrics.Internal)
	metrics.IncrementSpaceCount(logger, "member-1")

	initObjs := []runtimeclient.Object{}
	for index := range 3 {
		initObjs = append(initObjs, usersignup.NewUserSignup(usersignup.WithName(fmt.Sprintf("user-%d", index)), usersignup.WithAnnotation(toolchainv1alpha1.UserSignupActivationCounterAnnotationKey, strconv.Itoa(index+1))))
		initObjs = append(initObjs, masteruserrecord.NewMasterUserRecord(t, fmt.Sprintf("user-%d", index), masteruserrecord.TargetCluster("member-1")))
		initObjs = append(initObjs, space.NewSpace(test.HostOperatorNs, fmt.Sprintf("user-%d", index), space.WithSpecTargetCluster("member-1")))
	}
	toolchainStatus := toolchainstatustest.NewToolchainStatus(
		toolchainstatustest.WithMember("member-1", toolchainstatustest.WithSpaceCount(0)),
		toolchainstatustest.WithMetric("outdated", toolchainv1alpha1.Metric{
			"cookie":    1,
			"chocolate": 2,
		}),
	)

	// when
	metricstest.InitializeCountersWithMetricsSyncDisabled(t, toolchainStatus, initObjs...)

	// then
	metricstest.AssertThatCountersAndMetrics(t).
		HaveSpacesForCluster("member-1", 3).
		HaveUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
			"1,internal": 1,
			"2,internal": 1,
			"3,internal": 1,
		}).
		HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
			string(metrics.Internal): 3, // all MURs have `@redhat.com` email address
		})
	toolchainstatustest.AssertThatGivenToolchainStatus(t, toolchainStatus).
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
	metrics.IncrementMasterUserRecordCount(logger, metrics.Internal)

	// all expected metrics are set ...
	toolchainStatus := toolchainstatustest.NewToolchainStatus(
		toolchainstatustest.WithMember("member-1", toolchainstatustest.WithSpaceCount(0)),
		toolchainstatustest.WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
			string(metrics.Internal): 1,
			string(metrics.External): 1,
		}),
		toolchainstatustest.WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
			"1,external": 1,
			"1,internal": 1,
		}),
		// outdated metric which should be removed during the initialization
		toolchainstatustest.WithMetric("outdated", toolchainv1alpha1.Metric{
			"cookie":    1,
			"chocolate": 2,
		}),
	)
	// ... but config flag will force synchronization from resources
	initObjs := []runtimeclient.Object{}
	for index := range 3 {
		initObjs = append(initObjs, usersignup.NewUserSignup(usersignup.WithName(fmt.Sprintf("user-%d", index)), usersignup.WithAnnotation(toolchainv1alpha1.UserSignupActivationCounterAnnotationKey, strconv.Itoa(index+1))))
		initObjs = append(initObjs, masteruserrecord.NewMasterUserRecord(t, fmt.Sprintf("user-%d", index), masteruserrecord.TargetCluster("member-1")))
		initObjs = append(initObjs, space.NewSpace(test.HostOperatorNs, fmt.Sprintf("user-%d", index), space.WithSpecTargetCluster("member-1")))
	}

	// when
	metricstest.InitializeCounters(t, toolchainStatus, initObjs...)

	// then
	metricstest.AssertThatCountersAndMetrics(t).
		HaveSpacesForCluster("member-1", 3).
		HaveUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
			"1,internal": 1,
			"2,internal": 1,
			"3,internal": 1,
		}).
		HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
			string(metrics.Internal): 3, // all MURs have `@redhat.com` email address
		})
	toolchainstatustest.AssertThatGivenToolchainStatus(t, toolchainStatus).
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
	metrics.IncrementMasterUserRecordCount(logger, metrics.Internal)
	metrics.IncrementSpaceCount(logger, "member-1")

	toolchainStatus := toolchainstatustest.NewToolchainStatus(toolchainstatustest.WithMember("member-1", toolchainstatustest.WithSpaceCount(0)))
	initObjs := []runtimeclient.Object{}
	for index := range 10 {
		initObjs = append(initObjs, usersignup.NewUserSignup(usersignup.WithName(fmt.Sprintf("user-%d", index)), usersignup.WithAnnotation(toolchainv1alpha1.UserSignupActivationCounterAnnotationKey, strconv.Itoa(index+1))))
		initObjs = append(initObjs, masteruserrecord.NewMasterUserRecord(t, fmt.Sprintf("user-%d", index), masteruserrecord.TargetCluster("member-1")))
		initObjs = append(initObjs, space.NewSpace(test.HostOperatorNs, fmt.Sprintf("user-%d", index), space.WithSpecTargetCluster("member-1")))
	}
	metricstest.InitializeCountersWithMetricsSyncDisabled(t, toolchainStatus, initObjs...)
	fakeClient := test.NewFakeClient(t, initObjs...)
	err := fakeClient.Create(context.TODO(), masteruserrecord.NewMasterUserRecord(t, "ignored", masteruserrecord.TargetCluster("member-1")))
	require.NoError(t, err)
	err = fakeClient.Create(context.TODO(), space.NewSpace(test.HostOperatorNs, "ignored", space.WithSpecTargetCluster("member-1")))
	require.NoError(t, err)

	// when
	err = metrics.Synchronize(context.TODO(), fakeClient, toolchainStatus)

	// then
	require.NoError(t, err)
	metricstest.AssertThatCountersAndMetrics(t).
		HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
			string(metrics.Internal): 10, // all MURs have `@redhat.com` email address
		}).
		HaveSpacesForCluster("member-1", 10)
	toolchainstatustest.AssertThatGivenToolchainStatus(t, toolchainStatus).
		HasMasterUserRecordsPerDomain(map[string]int{
			string(metrics.Internal): 10, // all MURs have `@redhat.com` email address
		}).
		HasSpaceCount("member-1", 10)
}

func TestMultipleExecutionsInParallel(t *testing.T) {
	// given
	toolchainStatus := toolchainstatustest.NewToolchainStatus(
		toolchainstatustest.WithMember("member-1", toolchainstatustest.WithSpaceCount(0)),
		toolchainstatustest.WithMember("member-2", toolchainstatustest.WithSpaceCount(0)))
	initObjs := []runtimeclient.Object{}
	for index := range 10 {
		initObjs = append(initObjs, masteruserrecord.NewMasterUserRecord(t, fmt.Sprintf("user-%d", index), masteruserrecord.TargetCluster("member-1")))
		initObjs = append(initObjs, space.NewSpace(test.HostOperatorNs, fmt.Sprintf("user-%d", index), space.WithSpecTargetCluster("member-1")))
	}
	metricstest.InitializeCountersWithMetricsSyncDisabled(t, toolchainStatus, initObjs...)
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
			metrics.IncrementMasterUserRecordCount(logger, metrics.Internal)
			if index < 1000 {
				go func() {
					defer waitForFinished.Done()
					metrics.DecrementMasterUserRecordCount(logger, metrics.Internal)
				}()
			}
		}(i)
		go func(index int) {
			defer waitForFinished.Done()
			latch.Wait()
			metrics.IncrementSpaceCount(logger, "member-2")
			if index < 1000 {
				go func() {
					defer waitForFinished.Done()
					metrics.DecrementSpaceCount(logger, "member-2")
				}()
			}
		}(i)
		go func(index int) {
			defer waitForFinished.Done()
			latch.Wait()
			metrics.IncrementSpaceCount(logger, "member-1")
			if index < 1000 {
				go func() {
					defer waitForFinished.Done()
					metrics.DecrementSpaceCount(logger, "member-1")
				}()
			}
		}(i)
		go func(index int) {
			defer waitForFinished.Done()
			latch.Wait()
			metrics.UpdateUsersPerActivationCounters(logger, 1, metrics.Internal) // increment metric for internal users with 1 activation
			if index < 1000 {
				go func() {
					defer waitForFinished.Done()
					metrics.UpdateUsersPerActivationCounters(logger, 2, metrics.Internal) // increment metric for internal users with 2 activations and decrement metric for internal users with 1 activation
				}()
			}
		}(i)
	}

	for i := 0; i < 102; i++ {
		waitForFinished.Add(1)
		go func() {
			defer waitForFinished.Done()
			latch.Wait()
			err := metrics.Synchronize(context.TODO(), fakeClient, toolchainStatus)
			assert.NoError(t, err) // require must only be used in the goroutine running the test function (testifylint)
		}()
	}

	// when
	latch.Done()
	waitForFinished.Wait()
	err := metrics.Synchronize(context.TODO(), fakeClient, toolchainStatus)

	// then
	require.NoError(t, err)
	metricstest.AssertThatCountersAndMetrics(t).
		HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
			string(metrics.Internal): 12, // all MURs have `@redhat.com` email address
		}).
		HaveSpacesForCluster("member-1", 12).
		HaveSpacesForCluster("member-2", 2).
		HaveUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
			"1,internal": 2,
			"2,internal": 1000,
		})
	toolchainstatustest.AssertThatGivenToolchainStatus(t, toolchainStatus).
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
