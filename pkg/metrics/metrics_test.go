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

func TestResetMetrics(t *testing.T) {
	// given
	metrics.Reset()
	defer metrics.Reset()

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

func TestIncrementMasterUserRecordCount(t *testing.T) {
	// given
	metrics.Reset()
	defer metrics.Reset()

	// when
	metrics.IncrementMasterUserRecordCount(logger, metrics.Internal)
	metrics.IncrementMasterUserRecordCount(logger, metrics.Internal)
	metrics.IncrementMasterUserRecordCount(logger, metrics.External)

	// then
	metricstest.AssertThatCountersAndMetrics(t).
		HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{string(metrics.Internal): 2}).
		HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{string(metrics.External): 1})
}

func TestDecrementMasterUserRecordCount(t *testing.T) {
	// given
	metrics.Reset()
	defer metrics.Reset()

	// when
	metrics.IncrementMasterUserRecordCount(logger, metrics.Internal)
	metrics.IncrementMasterUserRecordCount(logger, metrics.Internal)
	metrics.DecrementMasterUserRecordCount(logger, metrics.Internal)
	metrics.IncrementMasterUserRecordCount(logger, metrics.External)

	// then
	metricstest.AssertThatCountersAndMetrics(t).
		HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
			string(metrics.Internal): 1,
			string(metrics.External): 1,
		})
}

// spaces tests ----------

func TestIncrementSpaceCount(t *testing.T) {
	// given
	metrics.Reset()
	defer metrics.Reset()

	// when
	metrics.IncrementSpaceCount(logger, "member-1")
	metrics.IncrementSpaceCount(logger, "member-2")
	metrics.IncrementSpaceCount(logger, "member-2")

	// then
	metricstest.AssertThatCountersAndMetrics(t).
		HaveSpacesForCluster("member-1", 1).
		HaveSpacesForCluster("member-2", 2)
}

func TestDecrementSpaceCount(t *testing.T) {
	// given
	metrics.Reset()
	defer metrics.Reset()

	// when
	metrics.IncrementSpaceCount(logger, "member-1")
	metrics.DecrementSpaceCount(logger, "member-1")
	metrics.IncrementSpaceCount(logger, "member-2")
	metrics.IncrementSpaceCount(logger, "member-2")
	metrics.DecrementSpaceCount(logger, "member-2")

	// then
	metricstest.AssertThatCountersAndMetrics(t).
		HaveSpacesForCluster("member-1", 0).
		HaveSpacesForCluster("member-2", 1)
}

// end spaces tests ------

func TestInitializeCountersFromExistingResources(t *testing.T) {
	// given
	// given
	metrics.Reset()
	defer metrics.Reset()
	logf.SetLogger(zap.New(zap.UseDevMode(true)))

	initObjs := []runtimeclient.Object{}
	for index := range 3 {
		initObjs = append(initObjs, usersignup.NewUserSignup(usersignup.WithName(fmt.Sprintf("user-%d", index)), usersignup.WithAnnotation(toolchainv1alpha1.UserSignupActivationCounterAnnotationKey, strconv.Itoa(index+1))))
		initObjs = append(initObjs, masteruserrecord.NewMasterUserRecord(t, fmt.Sprintf("user-%d", index), masteruserrecord.TargetCluster("member-1")))
		initObjs = append(initObjs, space.NewSpace(test.HostOperatorNs, fmt.Sprintf("user-%d", index), space.WithSpecTargetCluster("member-1")))
	}
	fakeClient := test.NewFakeClient(t, initObjs...)

	// when
	err := metrics.Synchronize(context.TODO(), fakeClient, test.HostOperatorNs)
	require.NoError(t, err)

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
}

func TestShouldNotInitializeAgain(t *testing.T) {
	// given
	metrics.Reset()
	defer metrics.Reset()

	initObjs := []runtimeclient.Object{}
	for index := range 10 {
		initObjs = append(initObjs, usersignup.NewUserSignup(usersignup.WithName(fmt.Sprintf("user-%d", index)), usersignup.WithAnnotation(toolchainv1alpha1.UserSignupActivationCounterAnnotationKey, strconv.Itoa(index+1))))
		initObjs = append(initObjs, masteruserrecord.NewMasterUserRecord(t, fmt.Sprintf("user-%d", index), masteruserrecord.TargetCluster("member-1")))
		initObjs = append(initObjs, space.NewSpace(test.HostOperatorNs, fmt.Sprintf("user-%d", index), space.WithSpecTargetCluster("member-1")))
	}
	fakeClient := test.NewFakeClient(t, initObjs...)
	err := metrics.Synchronize(context.TODO(), fakeClient, test.HostOperatorNs)
	require.NoError(t, err)

	// when
	err = fakeClient.Create(context.TODO(), masteruserrecord.NewMasterUserRecord(t, "ignored", masteruserrecord.TargetCluster("member-1")))
	require.NoError(t, err)
	err = fakeClient.Create(context.TODO(), space.NewSpace(test.HostOperatorNs, "ignored", space.WithSpecTargetCluster("member-1")))
	require.NoError(t, err)
	err = metrics.Synchronize(context.TODO(), fakeClient, test.HostOperatorNs)

	// then
	require.NoError(t, err)
	metricstest.AssertThatCountersAndMetrics(t).
		HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
			string(metrics.Internal): 10, // same value
		}).
		HaveSpacesForCluster("member-1", 10)
}

func TestMultipleExecutionsInParallel(t *testing.T) {
	// given
	initObjs := []runtimeclient.Object{}
	for index := range 10 {
		initObjs = append(initObjs, masteruserrecord.NewMasterUserRecord(t, fmt.Sprintf("user-%d", index), masteruserrecord.TargetCluster("member-1")))
		initObjs = append(initObjs, space.NewSpace(test.HostOperatorNs, fmt.Sprintf("user-%d", index), space.WithSpecTargetCluster("member-1")))
	}
	fakeClient := test.NewFakeClient(t, initObjs...)
	metricstest.ResetCounters(t, initObjs...)
	latch := new(sync.WaitGroup)
	latch.Add(1)
	waitForFinished := new(sync.WaitGroup)

	for i := range 1002 {
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
			metrics.IncrementUsersPerActivationCounters(logger, 1, metrics.Internal) // increment metric for internal users with 1 activation
			if index < 1000 {
				go func() {
					defer waitForFinished.Done()
					metrics.IncrementUsersPerActivationCounters(logger, 2, metrics.Internal) // increment metric for internal users with 2 activations and decrement metric for internal users with 1 activation
				}()
			}
		}(i)
	}

	for range 102 {
		waitForFinished.Add(1)
		go func() {
			defer waitForFinished.Done()
			latch.Wait()
			err := metrics.Synchronize(context.TODO(), fakeClient, test.HostOperatorNs)
			assert.NoError(t, err) // require must only be used in the goroutine running the test function (testifylint)
		}()
	}

	// when
	latch.Done()
	waitForFinished.Wait()
	err := metrics.Synchronize(context.TODO(), fakeClient, test.HostOperatorNs)

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
}
