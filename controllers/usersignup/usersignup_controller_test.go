package usersignup

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	. "github.com/codeready-toolchain/host-operator/pkg/space"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/controllers/toolchainconfig"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	"github.com/codeready-toolchain/host-operator/pkg/capacity"
	"github.com/codeready-toolchain/host-operator/pkg/counter"
	"github.com/codeready-toolchain/host-operator/pkg/metrics"
	"github.com/codeready-toolchain/host-operator/pkg/segment"
	. "github.com/codeready-toolchain/host-operator/test"
	ntest "github.com/codeready-toolchain/host-operator/test/notification"
	tiertest "github.com/codeready-toolchain/host-operator/test/nstemplatetier"
	segmenttest "github.com/codeready-toolchain/host-operator/test/segment"
	spacebindingtest "github.com/codeready-toolchain/host-operator/test/spacebinding"
	hspc "github.com/codeready-toolchain/host-operator/test/spaceprovisionerconfig"
	"github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	"github.com/codeready-toolchain/toolchain-common/pkg/condition"
	commonconfig "github.com/codeready-toolchain/toolchain-common/pkg/configuration"
	commonsocialevent "github.com/codeready-toolchain/toolchain-common/pkg/socialevent"
	"github.com/codeready-toolchain/toolchain-common/pkg/states"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	testconfig "github.com/codeready-toolchain/toolchain-common/pkg/test/config"
	murtest "github.com/codeready-toolchain/toolchain-common/pkg/test/masteruserrecord"
	metricstest "github.com/codeready-toolchain/toolchain-common/pkg/test/metrics"
	testsocialevent "github.com/codeready-toolchain/toolchain-common/pkg/test/socialevent"
	spacetest "github.com/codeready-toolchain/toolchain-common/pkg/test/space"
	spc "github.com/codeready-toolchain/toolchain-common/pkg/test/spaceprovisionerconfig"
	commontier "github.com/codeready-toolchain/toolchain-common/pkg/test/tier"
	commonsignup "github.com/codeready-toolchain/toolchain-common/pkg/test/usersignup"

	"github.com/gofrs/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var (
	baseNSTemplateTier  = tiertest.NewNSTemplateTier("base", "dev", "stage")
	base2NSTemplateTier = tiertest.NewNSTemplateTier("base2", "dev", "stage")
	deactivate30Tier    = commontier.NewUserTier(commontier.WithName("deactivate30"), commontier.WithDeactivationTimeoutDays(30))
	deactivate80Tier    = commontier.NewUserTier(commontier.WithName("deactivate80"), commontier.WithDeactivationTimeoutDays(80))
	event               = testsocialevent.NewSocialEvent(test.HostOperatorNs, commonsocialevent.NewName(),
		testsocialevent.WithUserTier(deactivate80Tier.Name),
		testsocialevent.WithSpaceTier(base2NSTemplateTier.Name))
)

func TestUserSignupCreateMUROk(t *testing.T) {
	spaceProvisionerConfig := hspc.NewEnabledValidTenantSPC("member1")
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	for testname, userSignup := range map[string]*toolchainv1alpha1.UserSignup{
		"manually approved with valid activation annotation": commonsignup.NewUserSignup(
			commonsignup.ApprovedManually(),
			commonsignup.WithTargetCluster("member1"),
			commonsignup.WithStateLabel(toolchainv1alpha1.UserSignupStateLabelValueNotReady),
			commonsignup.WithAnnotation(toolchainv1alpha1.UserSignupActivationCounterAnnotationKey, "2"), // this is a returning user
			commonsignup.WithUserID("198573"),
			commonsignup.WithAccountID("387832"),
			commonsignup.WithOriginalSub("original-sub-value:1234")),
		"automatically approved with valid activation annotation": commonsignup.NewUserSignup(
			commonsignup.ApprovedManually(),
			commonsignup.WithTargetCluster("member1"),
			commonsignup.WithStateLabel(toolchainv1alpha1.UserSignupStateLabelValueDeactivated),
			commonsignup.WithAnnotation(toolchainv1alpha1.UserSignupActivationCounterAnnotationKey, "2"), // this is a returning user
			commonsignup.WithOriginalSub("original-sub-value:1234")),
		"automatically approved via social event": commonsignup.NewUserSignup(
			commonsignup.ApprovedManually(),
			commonsignup.WithTargetCluster("member1"),
			commonsignup.WithStateLabel(toolchainv1alpha1.UserSignupStateLabelValueDeactivated),
			commonsignup.WithAnnotation(toolchainv1alpha1.UserSignupActivationCounterAnnotationKey, "2"), // this is a returning user
			commonsignup.WithLabel(toolchainv1alpha1.SocialEventUserSignupLabelKey, event.Name),
			commonsignup.WithOriginalSub("original-sub-value:1234"),
			commonsignup.WithUserID("9834722"),
			commonsignup.WithAccountID("4837262")),
		"automatically approved via unknown social event": commonsignup.NewUserSignup(
			commonsignup.ApprovedManually(),
			commonsignup.WithTargetCluster("member1"),
			commonsignup.WithStateLabel(toolchainv1alpha1.UserSignupStateLabelValueDeactivated),
			commonsignup.WithAnnotation(toolchainv1alpha1.UserSignupActivationCounterAnnotationKey, "2"), // this is a returning user
			commonsignup.WithLabel(toolchainv1alpha1.SocialEventUserSignupLabelKey, "unknown"),
			commonsignup.WithOriginalSub("original-sub-value:1234")),
		"manually approved with invalid activation annotation": commonsignup.NewUserSignup(
			commonsignup.ApprovedManually(),
			commonsignup.WithTargetCluster("member1"),
			commonsignup.WithStateLabel(toolchainv1alpha1.UserSignupStateLabelValueNotReady),
			commonsignup.WithAnnotation(toolchainv1alpha1.UserSignupActivationCounterAnnotationKey, "?")), // annotation value is not an 'int'
		"manually approved without activation annotation": commonsignup.NewUserSignup(
			commonsignup.ApprovedManually(),
			commonsignup.WithTargetCluster("member1"),
			commonsignup.WithStateLabel(toolchainv1alpha1.UserSignupStateLabelValueNotReady),
			commonsignup.WithoutAnnotation(toolchainv1alpha1.UserSignupActivationCounterAnnotationKey)), // no annotation on this user, so the value will not be incremented
	} {
		t.Run(testname, func(t *testing.T) {
			// given
			defer counter.Reset()
			config := commonconfig.NewToolchainConfigObjWithReset(t,
				testconfig.AutomaticApproval().Enabled(true),
				testconfig.Metrics().ForceSynchronization(false))
			r, req, _ := prepareReconcile(t, userSignup.Name, config, spaceProvisionerConfig, userSignup, baseNSTemplateTier, deactivate30Tier, deactivate80Tier, event)
			InitializeCountersWithToolchainConfig(t, config, NewToolchainStatus(
				WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
					"1,internal": 0,
					"2,internal": 1,
					"3,internal": 0,
				}),
				WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
					string(metrics.External): 1,
				})))

			// when
			res, err := r.Reconcile(context.TODO(), req)

			// then verify that the MUR exists and is complete
			require.NoError(t, err)
			require.Equal(t, reconcile.Result{}, res)

			murtest.AssertThatMasterUserRecords(t, r.Client).HaveCount(1)
			mur := murtest.AssertThatMasterUserRecord(t, userSignup.Name, r.Client).
				HasLabelWithValue(toolchainv1alpha1.MasterUserRecordOwnerLabelKey, userSignup.Name).
				HasUserAccounts(1).Get()
			switch testname {
			case "automatically approved via social event":
				murtest.AssertThatMasterUserRecord(t, userSignup.Name, r.Client).HasTier(*deactivate80Tier)
			default:
				murtest.AssertThatMasterUserRecord(t, userSignup.Name, r.Client).HasTier(*deactivate30Tier)
			}
			metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupUniqueTotal) // zero because we started with a not-ready state instead of empty as per usual
			metricstest.AssertMetricsCounterEquals(t, 1, metrics.UserSignupApprovedTotal)
			segmenttest.AssertMessageQueuedForProvisionedMur(t, r.SegmentClient, userSignup, mur.Name)

			AssertThatCountersAndMetrics(t).
				HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
					string(metrics.Internal): 1, // new user with an `@redhat.com` email address
					string(metrics.External): 1, // existing metric (from the counter init)
				}) //
			actualUserSignup := &toolchainv1alpha1.UserSignup{}
			err = r.Client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, actualUserSignup)
			require.NoError(t, err)
			assert.Equal(t, "approved", actualUserSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey])
			switch testname {
			case "manually approved with valid activation annotation",
				"automatically approved with valid activation annotation",
				"automatically approved via social event",
				"automatically approved via unknown social event":
				assert.Equal(t, "3", actualUserSignup.Annotations[toolchainv1alpha1.UserSignupActivationCounterAnnotationKey]) // annotation value is incremented
				AssertThatCountersAndMetrics(t).
					HaveUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
						"1,internal": 0, // unchanged
						"2,internal": 0, // decreased
						"3,internal": 1, // increased
					})
			case "manually approved without activation annotation",
				"manually approved with invalid activation annotation":
				assert.Equal(t, "1", actualUserSignup.Annotations[toolchainv1alpha1.UserSignupActivationCounterAnnotationKey]) // annotation was set to "1" since it was missing
				AssertThatCountersAndMetrics(t).
					HaveUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
						"1,internal": 1, // increased
						"2,internal": 1, // unchanged
						"3,internal": 0, // unchanged
					})
			default:
				assert.Fail(t, "unknown testcase")
			}
			// UserSignup not marked as ready yet
			metricstest.AssertAllHistogramBucketsAreEmpty(t, metrics.UserSignupProvisionTimeHistogram)
		})
	}
}

func TestUserSignupCreateSpaceAndSpaceBindingOk(t *testing.T) {
	spaceProvisionerConfig := hspc.NewEnabledValidTenantSPC("member1")
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	for testname, userSignup := range map[string]*toolchainv1alpha1.UserSignup{
		"without skip space creation annotation": commonsignup.NewUserSignup(
			commonsignup.ApprovedManually(),
			commonsignup.WithTargetCluster("member1"),
			commonsignup.WithStateLabel(toolchainv1alpha1.UserSignupStateLabelValueNotReady),
			commonsignup.WithoutAnnotation(toolchainv1alpha1.SkipAutoCreateSpaceAnnotationKey),
			commonsignup.WithRequestReceivedTimeAnnotation(time.Now())),
		"with social event": commonsignup.NewUserSignup(
			commonsignup.ApprovedManually(),
			commonsignup.WithTargetCluster("member1"),
			commonsignup.WithStateLabel(toolchainv1alpha1.UserSignupStateLabelValueNotReady),
			commonsignup.WithoutAnnotation(toolchainv1alpha1.SkipAutoCreateSpaceAnnotationKey),
			commonsignup.WithLabel(toolchainv1alpha1.SocialEventUserSignupLabelKey, event.Name),
			commonsignup.WithRequestReceivedTimeAnnotation(time.Now()),
		),
		"with skip space creation annotation set to false": commonsignup.NewUserSignup(
			commonsignup.ApprovedManually(),
			commonsignup.WithTargetCluster("member1"),
			commonsignup.WithStateLabel(toolchainv1alpha1.UserSignupStateLabelValueNotReady),
			commonsignup.WithAnnotation(toolchainv1alpha1.SkipAutoCreateSpaceAnnotationKey, "false"),
			commonsignup.WithRequestReceivedTimeAnnotation(time.Now())),
		"with skip space creation annotation set to true": commonsignup.NewUserSignup(
			commonsignup.ApprovedManually(),
			commonsignup.WithTargetCluster("member1"),
			commonsignup.WithStateLabel(toolchainv1alpha1.UserSignupStateLabelValueNotReady),
			commonsignup.WithAnnotation(toolchainv1alpha1.SkipAutoCreateSpaceAnnotationKey, "true"),
			commonsignup.WithRequestReceivedTimeAnnotation(time.Now())),
		"with feature toggles": commonsignup.NewUserSignup(
			commonsignup.ApprovedManually(),
			commonsignup.WithTargetCluster("member1"),
			commonsignup.WithStateLabel(toolchainv1alpha1.UserSignupStateLabelValueNotReady),
			commonsignup.WithoutAnnotation(toolchainv1alpha1.SkipAutoCreateSpaceAnnotationKey),
			commonsignup.WithRequestReceivedTimeAnnotation(time.Now())),
	} {
		t.Run(testname, func(t *testing.T) {
			// given
			defer counter.Reset()

			mur := newMasterUserRecord(userSignup, "member1", deactivate30Tier.Name, "foo")
			mur.Labels = map[string]string{toolchainv1alpha1.MasterUserRecordOwnerLabelKey: userSignup.Name}

			config := &toolchainv1alpha1.ToolchainConfig{}
			switch testname {
			case "with feature toggles":
				weight100 := uint(100)
				weight0 := uint(0)
				config = commonconfig.NewToolchainConfigObjWithReset(t, testconfig.Metrics().ForceSynchronization(false), testconfig.Tiers().FeatureToggle("feature-on", &weight100).FeatureToggle("feature-off", &weight0))
			}
			r, req, _ := prepareReconcile(t, userSignup.Name, config, spaceProvisionerConfig, userSignup, mur, baseNSTemplateTier, base2NSTemplateTier, deactivate30Tier, deactivate80Tier, event)

			// when
			res, err := r.Reconcile(context.TODO(), req)

			// then verify that the Space exists or not depending on the skip space creation annotation
			require.NoError(t, err)
			require.Equal(t, reconcile.Result{}, res)

			AssertThatUserSignup(t, req.Namespace, userSignup.Name, r.Client).HasLabel(toolchainv1alpha1.UserSignupStateLabelKey, "approved")
			switch testname {
			case "without skip space creation annotation", "with skip space creation annotation set to false":
				spacetest.AssertThatSpace(t, test.HostOperatorNs, "foo", r.Client).
					Exists().
					HasLabelWithValue(toolchainv1alpha1.SpaceCreatorLabelKey, userSignup.Name).
					HasSpecTargetCluster("member1").
					HasSpecTargetClusterRoles([]string{cluster.RoleLabel(cluster.Tenant)}).
					HasTier("base").
					DoesNotHaveAnnotation(toolchainv1alpha1.FeatureToggleNameAnnotationKey)
				AssertThatUserSignup(t, req.Namespace, userSignup.Name, r.Client).HasHomeSpace("foo")
			case "with feature toggles":
				spacetest.AssertThatSpace(t, test.HostOperatorNs, "foo", r.Client).
					Exists().
					HasLabelWithValue(toolchainv1alpha1.SpaceCreatorLabelKey, userSignup.Name).
					HasSpecTargetCluster("member1").
					HasSpecTargetClusterRoles([]string{cluster.RoleLabel(cluster.Tenant)}).
					HasTier("base").
					HasAnnotationWithValue(toolchainv1alpha1.FeatureToggleNameAnnotationKey, "feature-on")
				AssertThatUserSignup(t, req.Namespace, userSignup.Name, r.Client).HasHomeSpace("foo")
			case "with social event":
				spacetest.AssertThatSpace(t, test.HostOperatorNs, "foo", r.Client).
					Exists().
					HasLabelWithValue(toolchainv1alpha1.SpaceCreatorLabelKey, userSignup.Name).
					HasSpecTargetCluster("member1").
					HasSpecTargetClusterRoles([]string{cluster.RoleLabel(cluster.Tenant)}).
					HasTier("base2").
					DoesNotHaveAnnotation(toolchainv1alpha1.FeatureToggleNameAnnotationKey)
				AssertThatUserSignup(t, req.Namespace, userSignup.Name, r.Client).HasHomeSpace("foo")
			case "with skip space creation annotation set to true":
				spacetest.AssertThatSpace(t, test.HostOperatorNs, "foo", r.Client).
					DoesNotExist()
				AssertThatUserSignup(t, req.Namespace, userSignup.Name, r.Client).HasHomeSpace("")
			default:
				assert.Fail(t, "unknown testcase")
			}

			t.Run("second reconcile", func(t *testing.T) {
				// when
				res, err := r.Reconcile(context.TODO(), req)
				require.NoError(t, err)
				// then
				require.Equal(t, reconcile.Result{}, res)
				switch testname {
				case "without skip space creation annotation", "with skip space creation annotation set to false", "with social event":
					spacebindingtest.AssertThatSpaceBinding(t, test.HostOperatorNs, "foo", "foo", r.Client).
						Exists().
						HasLabelWithValue(toolchainv1alpha1.SpaceCreatorLabelKey, userSignup.Name).
						HasLabelWithValue(toolchainv1alpha1.SpaceBindingMasterUserRecordLabelKey, "foo").
						HasLabelWithValue(toolchainv1alpha1.SpaceBindingSpaceLabelKey, "foo").
						HasSpec("foo", "foo", "admin")
					AssertThatUserSignup(t, req.Namespace, userSignup.Name, r.Client).HasHomeSpace("foo")
					spacetest.AssertThatSpace(t, test.HostOperatorNs, "foo", r.Client).
						Exists().
						DoesNotHaveAnnotation(toolchainv1alpha1.FeatureToggleNameAnnotationKey)
				case "with feature toggles":
					spacebindingtest.AssertThatSpaceBinding(t, test.HostOperatorNs, "foo", "foo", r.Client).
						Exists().
						HasLabelWithValue(toolchainv1alpha1.SpaceCreatorLabelKey, userSignup.Name).
						HasLabelWithValue(toolchainv1alpha1.SpaceBindingMasterUserRecordLabelKey, "foo").
						HasLabelWithValue(toolchainv1alpha1.SpaceBindingSpaceLabelKey, "foo").
						HasSpec("foo", "foo", "admin")
					AssertThatUserSignup(t, req.Namespace, userSignup.Name, r.Client).HasHomeSpace("foo")
					spacetest.AssertThatSpace(t, test.HostOperatorNs, "foo", r.Client).
						Exists().
						HasAnnotationWithValue(toolchainv1alpha1.FeatureToggleNameAnnotationKey, "feature-on")
				case "with skip space creation annotation set to true":
					spacebindingtest.AssertThatSpaceBinding(t, test.HostOperatorNs, "foo", "foo", r.Client).
						DoesNotExist()
					AssertThatUserSignup(t, req.Namespace, userSignup.Name, r.Client).HasHomeSpace("")
				default:
					assert.Fail(t, "unknown testcase")
				}
				// UserSignup not marked as ready yet
				metricstest.AssertAllHistogramBucketsAreEmpty(t, metrics.UserSignupProvisionTimeHistogram)
			})
		})
	}
}

func TestDeletingUserSignupShouldNotUpdateMetrics(t *testing.T) {
	// given
	spaceProvisionerConfig := hspc.NewEnabledValidTenantSPC("member1")
	defer counter.Reset()
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	userSignup := commonsignup.NewUserSignup(
		commonsignup.ApprovedManually(),
		commonsignup.BeingDeleted(),
		commonsignup.WithStateLabel(toolchainv1alpha1.UserSignupStateLabelValueNotReady),
		commonsignup.WithAnnotation(toolchainv1alpha1.UserSignupActivationCounterAnnotationKey, "2"),
		commonsignup.WithRequestReceivedTimeAnnotation(time.Now()))
	controllerutil.AddFinalizer(userSignup, toolchainv1alpha1.FinalizerName)
	r, req, _ := prepareReconcile(t, userSignup.Name, nil, spaceProvisionerConfig, userSignup, baseNSTemplateTier)
	InitializeCountersWithMetricsSyncDisabled(t, NewToolchainStatus(
		WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
			"1,internal": 1,
			"2,internal": 1,
			"2,external": 9,
			"3,internal": 1,
		}),
		WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
			string(metrics.External): 12,
		})))

	// when
	_, err := r.Reconcile(context.TODO(), req)

	// then
	require.NoError(t, err)

	// Verify the counters
	AssertThatCountersAndMetrics(t).
		HaveUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
			"1,internal": 1,
			"2,internal": 1,
			"2,external": 9,
			"3,internal": 1,
		}).
		HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
			string(metrics.External): 12,
		})
	metricstest.AssertAllHistogramBucketsAreEmpty(t, metrics.UserSignupProvisionTimeHistogram)
}

func TestUserSignupVerificationRequiredMetric(t *testing.T) {
	// given
	spaceProvisionerConfig := hspc.NewEnabledValidTenantSPC("member1")
	defer counter.Reset()
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	userSignup := commonsignup.NewUserSignup(
		commonsignup.ApprovedManually(),
	)
	// set verification required to true in spec only, status will be added during reconcile
	states.SetVerificationRequired(userSignup, true)
	r, req, _ := prepareReconcile(t, userSignup.Name, nil, spaceProvisionerConfig, userSignup, baseNSTemplateTier)
	metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupVerificationRequiredTotal) // nothing yet since not reconciled yet

	// when
	_, err := r.Reconcile(context.TODO(), req)

	// then
	require.NoError(t, err)

	// assert that usersignup status has verification required condition
	updatedUserSignup := &toolchainv1alpha1.UserSignup{}
	err = r.Client.Get(context.TODO(), types.NamespacedName{
		Namespace: test.HostOperatorNs,
		Name:      userSignup.Name,
	}, updatedUserSignup)
	require.NoError(t, err)

	// verify that the status has the verification required condition
	test.AssertContainsCondition(t, updatedUserSignup.Status.Conditions, toolchainv1alpha1.Condition{
		Type:   toolchainv1alpha1.UserSignupComplete,
		Status: corev1.ConditionFalse,
		Reason: toolchainv1alpha1.UserSignupVerificationRequiredReason,
	})

	// Verify the metric
	metricstest.AssertMetricsCounterEquals(t, 1, metrics.UserSignupVerificationRequiredTotal) // should be 1 since verification required status was set

	t.Run("second reconcile - metrics counter still equals 1", func(t *testing.T) {
		// when
		_, err := r.Reconcile(context.TODO(), req)

		// then
		require.NoError(t, err)

		// Verify the metric
		metricstest.AssertMetricsCounterEquals(t, 1, metrics.UserSignupVerificationRequiredTotal) // should still be 1 since verification required status was already set
	})
}

func TestUserSignupWithAutoApprovalWithoutTargetCluster(t *testing.T) {
	// given
	userSignup := commonsignup.NewUserSignup(commonsignup.WithRequestReceivedTimeAnnotation(time.Now()))
	spaceProvisionerConfig := hspc.NewEnabledValidTenantSPC("member1")

	config := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true), testconfig.Metrics().ForceSynchronization(false))
	r, req, _ := prepareReconcile(t, userSignup.Name, config, spaceProvisionerConfig, userSignup, baseNSTemplateTier, deactivate30Tier)
	InitializeCountersWithToolchainConfig(t, config, NewToolchainStatus(
		WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
			"1,external": 1,
		}),
		WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
			string(metrics.External): 1,
		}),
	))

	// when - The first reconcile creates the MasterUserRecord
	res, err := r.Reconcile(context.TODO(), req)

	// then
	require.NoError(t, err)
	require.Equal(t, reconcile.Result{}, res)

	// Lookup the user signup again
	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)
	assert.Equal(t, "approved", userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey])
	metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupAutoDeactivatedTotal)
	metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupBannedTotal)
	metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupDeactivatedTotal)
	metricstest.AssertMetricsCounterEquals(t, 1, metrics.UserSignupApprovedTotal)
	metricstest.AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)
	murtest.AssertThatMasterUserRecords(t, r.Client).HaveCount(1)
	mur := murtest.AssertThatMasterUserRecord(t, userSignup.Spec.IdentityClaims.PreferredUsername, r.Client).
		HasLabelWithValue(toolchainv1alpha1.MasterUserRecordOwnerLabelKey, userSignup.Name).
		HasUserAccounts(1).
		HasTier(*deactivate30Tier).
		Get()
	segmenttest.AssertMessageQueuedForProvisionedMur(t, r.SegmentClient, userSignup, mur.Name)

	// space and spacebinding should be created after the next reconcile
	spacetest.AssertThatSpace(t, test.HostOperatorNs, userSignup.Spec.IdentityClaims.PreferredUsername, r.Client).DoesNotExist()
	spacebindingtest.AssertThatSpaceBinding(t, test.HostOperatorNs, userSignup.Spec.IdentityClaims.PreferredUsername,
		userSignup.Spec.IdentityClaims.PreferredUsername, r.Client).DoesNotExist()

	test.AssertConditionsMatch(t, userSignup.Status.Conditions,
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupApproved,
			Status: corev1.ConditionTrue,
			Reason: "ApprovedAutomatically",
		},
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupUserDeactivatingNotificationCreated,
			Status: corev1.ConditionFalse,
			Reason: "UserNotInPreDeactivation",
		},
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupUserDeactivatedNotificationCreated,
			Status: corev1.ConditionFalse,
			Reason: "UserIsActive",
		})

	AssertThatCountersAndMetrics(t).HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
		string(metrics.External): 1,
		string(metrics.Internal): 1,
	})

	t.Run("second reconcile", func(t *testing.T) {
		// when
		res, err = r.Reconcile(context.TODO(), req)

		// then
		require.NoError(t, err)
		require.Equal(t, reconcile.Result{}, res)

		// space should now be created
		spacetest.AssertThatSpace(t, test.HostOperatorNs, userSignup.Spec.IdentityClaims.PreferredUsername, r.Client).
			HasLabelWithValue(toolchainv1alpha1.SpaceCreatorLabelKey, userSignup.Name).
			Exists().
			HasSpecTargetCluster("member1").
			HasSpecTargetClusterRoles([]string{cluster.RoleLabel(cluster.Tenant)}).
			HasTier(baseNSTemplateTier.Name)
		spacebindingtest.AssertThatSpaceBinding(t, test.HostOperatorNs, "foo", "foo", r.Client).
			DoesNotExist()
		t.Run("third reconcile", func(t *testing.T) {
			// set the space & mur to ready
			setSpaceToReady(t, r.Client, userSignup.Spec.IdentityClaims.PreferredUsername)
			setMURToReady(t, r.Client, userSignup.Spec.IdentityClaims.PreferredUsername)

			// when
			res, err = r.Reconcile(context.TODO(), req)

			// then
			require.NoError(t, err)
			require.Equal(t, reconcile.Result{}, res)

			// spacebinding should be created
			spacebindingtest.AssertThatSpaceBinding(t, test.HostOperatorNs, userSignup.Spec.IdentityClaims.PreferredUsername,
				userSignup.Spec.IdentityClaims.PreferredUsername, r.Client).
				Exists().
				HasLabelWithValue(toolchainv1alpha1.SpaceCreatorLabelKey, userSignup.Name).
				HasLabelWithValue(toolchainv1alpha1.SpaceBindingMasterUserRecordLabelKey, userSignup.Spec.IdentityClaims.PreferredUsername).
				HasLabelWithValue(toolchainv1alpha1.SpaceBindingSpaceLabelKey, userSignup.Spec.IdentityClaims.PreferredUsername).
				HasSpec(mur.Name, userSignup.Spec.IdentityClaims.PreferredUsername, "admin")

			// Lookup the userSignup one more time and check the conditions are updated
			err = r.Client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
			require.NoError(t, err)
			require.Equal(t, userSignup.Status.CompliantUsername, mur.Name)
			assert.Equal(t, "approved", userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey])
			AssertThatUserSignup(t, req.Namespace, userSignup.Name, r.Client).HasHomeSpace(userSignup.Name)

			test.AssertConditionsMatch(t, userSignup.Status.Conditions,
				toolchainv1alpha1.Condition{
					Type:   toolchainv1alpha1.UserSignupApproved,
					Status: corev1.ConditionTrue,
					Reason: "ApprovedAutomatically",
				},
				toolchainv1alpha1.Condition{
					Type:   toolchainv1alpha1.UserSignupComplete,
					Status: corev1.ConditionTrue,
				},
				toolchainv1alpha1.Condition{
					Type:   toolchainv1alpha1.UserSignupUserDeactivatingNotificationCreated,
					Status: corev1.ConditionFalse,
					Reason: "UserNotInPreDeactivation",
				},
				toolchainv1alpha1.Condition{
					Type:   toolchainv1alpha1.UserSignupUserDeactivatedNotificationCreated,
					Status: corev1.ConditionFalse,
					Reason: "UserIsActive",
				})
		})
	})
	metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupAutoDeactivatedTotal)
	metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupBannedTotal)
	metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupDeactivatedTotal)
	metricstest.AssertMetricsCounterEquals(t, 1, metrics.UserSignupApprovedTotal)
	metricstest.AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)
	metricstest.AssertHistogramBucketEquals(t, 1, 1, metrics.UserSignupProvisionTimeHistogram) // could fail in debug mode
	segmenttest.AssertMessageQueuedForProvisionedMur(t, r.SegmentClient, userSignup, mur.Name)
}

func TestUserSignupWithMissingEmailAddressFails(t *testing.T) {
	// given
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	userSignup := commonsignup.NewUserSignup()
	userSignup.Spec.IdentityClaims.Email = ""

	spaceProvisionerConfig := hspc.NewEnabledValidTenantSPC("member1")
	config := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true), testconfig.Metrics().ForceSynchronization(false))
	r, req, _ := prepareReconcile(t, userSignup.Name, config, spaceProvisionerConfig, userSignup, baseNSTemplateTier)
	InitializeCountersWithToolchainConfig(t, config, NewToolchainStatus(
		WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
			"1,external": 1,
		}),
		WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
			string(metrics.External): 1,
		}),
	))

	// when
	_, err := r.Reconcile(context.TODO(), req)

	// then
	require.Error(t, err)

	// Lookup the user signup again
	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)
	assert.Equal(t, toolchainv1alpha1.UserSignupStateLabelValueNotReady, userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey])
	metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
	metricstest.AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)
	test.AssertConditionsMatch(t, userSignup.Status.Conditions,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupComplete,
			Status:  corev1.ConditionFalse,
			Reason:  "MissingUserEmail",
			Message: "missing email at usersignup",
		})
	AssertThatCountersAndMetrics(t).
		HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
			string(metrics.External): 1,
		}).
		HaveUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
			"1,external": 1,
		})
}

func TestUserSignupWithInvalidEmailHashLabelFails(t *testing.T) {
	// given
	defer counter.Reset()
	userSignup := commonsignup.NewUserSignup(
		commonsignup.WithLabel(toolchainv1alpha1.UserSignupUserEmailHashLabelKey, "abcdef0123456789"),
		commonsignup.WithLabel("toolchain.dev.openshift.com/approved", "false"),
	)

	userSignup.Spec.IdentityClaims.Email = "foo@redhat.com"

	spaceProvisionerConfig := hspc.NewEnabledValidTenantSPC("member1")
	config := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true), testconfig.Metrics().ForceSynchronization(false))
	r, req, _ := prepareReconcile(t, userSignup.Name, config, spaceProvisionerConfig, userSignup, baseNSTemplateTier)
	InitializeCountersWithToolchainConfig(t, config, NewToolchainStatus(
		WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
			"1,external": 1,
		}),
		WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
			string(metrics.External): 1,
		}),
	))

	// when
	_, err := r.Reconcile(context.TODO(), req)

	// then
	require.Error(t, err)

	// Lookup the user signup again
	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)
	assert.Equal(t, toolchainv1alpha1.UserSignupStateLabelValueNotReady, userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey])
	metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
	metricstest.AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)
	test.AssertConditionsMatch(t, userSignup.Status.Conditions,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupComplete,
			Status:  corev1.ConditionFalse,
			Reason:  "InvalidEmailHashLabel",
			Message: "hash is invalid",
		})
	AssertThatCountersAndMetrics(t).
		HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
			string(metrics.External): 1,
		}).
		HaveUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
			"1,external": 1,
		})
}

func TestUpdateOfApprovedLabelFails(t *testing.T) {
	// given
	userSignup := commonsignup.NewUserSignup()

	spaceProvisionerConfig := hspc.NewEnabledValidTenantSPC("member1")
	config := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true), testconfig.Metrics().ForceSynchronization(false))
	r, req, fakeClient := prepareReconcile(t, userSignup.Name, config, spaceProvisionerConfig, userSignup, baseNSTemplateTier)
	fakeClient.MockUpdate = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.UpdateOption) error {
		return fmt.Errorf("some error")
	}
	InitializeCountersWithToolchainConfig(t, config, NewToolchainStatus(
		WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
			"1,internal": 0, // no user approved yet
		}),
		WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
			string(metrics.External): 1,
		}),
	))

	// when
	_, err := r.Reconcile(context.TODO(), req)

	// then
	require.Error(t, err)
	require.EqualError(t, err, "unable to update state label at UserSignup resource: some error")

	// Lookup the user signup again
	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)
	test.AssertConditionsMatch(t, userSignup.Status.Conditions,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupComplete,
			Status:  corev1.ConditionFalse,
			Reason:  "UnableToUpdateStateLabel",
			Message: "some error",
		})
	AssertThatCountersAndMetrics(t).
		HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
			string(metrics.External): 1,
		}).
		HaveUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
			"1,internal": 0, // unchanged
		})
	metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
	metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupUniqueTotal)
}

func TestUserSignupWithMissingEmailHashLabelFails(t *testing.T) {
	// given
	userSignup := commonsignup.NewUserSignup()
	userSignup.Spec.IdentityClaims.Email = "foo@redhat.com"
	userSignup.Labels = map[string]string{"toolchain.dev.openshift.com/approved": "false"}

	spaceProvisionerConfig := hspc.NewEnabledValidTenantSPC("member1")
	config := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true), testconfig.Metrics().ForceSynchronization(false))
	r, req, _ := prepareReconcile(t, userSignup.Name, config, spaceProvisionerConfig, userSignup, baseNSTemplateTier)
	InitializeCountersWithToolchainConfig(t, config, NewToolchainStatus(
		WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
			"1,external": 1,
		}),
		WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
			string(metrics.External): 1,
		}),
	))

	// when
	_, err := r.Reconcile(context.TODO(), req)

	// then
	require.Error(t, err)

	// Lookup the user signup again
	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)
	assert.Equal(t, toolchainv1alpha1.UserSignupStateLabelValueNotReady, userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey])
	metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
	metricstest.AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)
	test.AssertConditionsMatch(t, userSignup.Status.Conditions,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupComplete,
			Status:  corev1.ConditionFalse,
			Reason:  "MissingEmailHashLabel",
			Message: "missing label at usersignup",
		})
	AssertThatCountersAndMetrics(t).
		HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
			string(metrics.External): 1,
		}).
		HaveUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
			"1,external": 1,
		})
}

func TestNonDefaultNSTemplateTier(t *testing.T) {
	// given
	customNSTemplateTier := tiertest.NewNSTemplateTier("custom", "dev", "stage")
	customUserTier := commontier.NewUserTier(commontier.WithName("custom"), commontier.WithDeactivationTimeoutDays(120))
	config := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true), testconfig.Tiers().DefaultUserTier("custom"), testconfig.Tiers().DefaultSpaceTier("custom"), testconfig.Metrics().ForceSynchronization(false))
	userSignup := commonsignup.NewUserSignup()
	spaceProvisionerConfig := hspc.NewEnabledValidTenantSPC("member1")
	r, req, _ := prepareReconcile(t, userSignup.Name, config, spaceProvisionerConfig, userSignup, customNSTemplateTier, customUserTier) // use custom tier

	commonconfig.ResetCache() // reset the config cache so that the update config is picked up
	InitializeCountersWithToolchainConfig(t, config, NewToolchainStatus(
		WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
			string(metrics.External): 1,
		}),
		WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
			"1,external": 1,
		}),
	))

	// when
	res, err := r.Reconcile(context.TODO(), req)

	// then
	require.NoError(t, err)
	require.Equal(t, reconcile.Result{}, res)

	// Lookup the user signup again
	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)
	assert.Equal(t, "approved", userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey])
	metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupAutoDeactivatedTotal)
	metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupBannedTotal)
	metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupDeactivatedTotal)
	metricstest.AssertMetricsCounterEquals(t, 1, metrics.UserSignupApprovedTotal)
	metricstest.AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)
	mur := murtest.AssertThatMasterUserRecord(t, userSignup.Name, r.Client).Get()
	segmenttest.AssertMessageQueuedForProvisionedMur(t, r.SegmentClient, userSignup, mur.Name)

	murtest.AssertThatMasterUserRecords(t, r.Client).HaveCount(1)
	murtest.AssertThatMasterUserRecord(t, userSignup.Name, r.Client).
		HasLabelWithValue(toolchainv1alpha1.MasterUserRecordOwnerLabelKey, userSignup.Name).
		HasUserAccounts(1).
		HasTier(*customUserTier).
		Get()

	// space should only be created after the next reconcile
	spacetest.AssertThatSpace(t, test.HostOperatorNs, userSignup.Name, r.Client).DoesNotExist()
	spacebindingtest.AssertThatSpaceBinding(t, test.HostOperatorNs, mur.Name, userSignup.Name, r.Client).DoesNotExist()

	test.AssertConditionsMatch(t, userSignup.Status.Conditions,
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupApproved,
			Status: corev1.ConditionTrue,
			Reason: "ApprovedAutomatically",
		},
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupUserDeactivatingNotificationCreated,
			Status: corev1.ConditionFalse,
			Reason: "UserNotInPreDeactivation",
		},
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupUserDeactivatedNotificationCreated,
			Status: corev1.ConditionFalse,
			Reason: "UserIsActive",
		})

	AssertThatCountersAndMetrics(t).HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
		string(metrics.External): 1,
		string(metrics.Internal): 1,
	})

	t.Run("second reconcile", func(t *testing.T) {
		// when
		res, err = r.Reconcile(context.TODO(), req)

		// then
		require.NoError(t, err)
		require.Equal(t, reconcile.Result{}, res)

		// space should be created on the second reconcile
		spacetest.AssertThatSpace(t, test.HostOperatorNs, userSignup.Name, r.Client).
			Exists().
			HasSpecTargetCluster("member1").
			HasTier(customUserTier.Name)
	})
}

func TestUserSignupFailedMissingTier(t *testing.T) {
	type variation struct {
		description    string
		config         *toolchainv1alpha1.ToolchainConfig
		expectedReason string
		expectedMsg    string
	}

	variations := []variation{
		{
			description:    "default spacetier",
			config:         commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true), testconfig.Metrics().ForceSynchronization(false)),
			expectedReason: "NoTemplateTierAvailable",
			expectedMsg:    "nstemplatetiers.toolchain.dev.openshift.com \"base\" not found",
		},
		{
			description:    "non-default spacetier",
			config:         commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true), testconfig.Tiers().DefaultSpaceTier("nonexistent"), testconfig.Metrics().ForceSynchronization(false)),
			expectedReason: "NoTemplateTierAvailable",
			expectedMsg:    "nstemplatetiers.toolchain.dev.openshift.com \"nonexistent\" not found",
		},
		{
			description:    "default usertier",
			config:         commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true), testconfig.Metrics().ForceSynchronization(false)),
			expectedReason: "NoUserTierAvailable",
			expectedMsg:    "usertiers.toolchain.dev.openshift.com \"deactivate30\" not found",
		},
		{
			description:    "non-default usertier",
			config:         commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true), testconfig.Tiers().DefaultUserTier("nonexistent"), testconfig.Metrics().ForceSynchronization(false)),
			expectedReason: "NoUserTierAvailable",
			expectedMsg:    "usertiers.toolchain.dev.openshift.com \"nonexistent\" not found",
		},
	}

	for _, v := range variations {
		t.Run(v.description, func(t *testing.T) {
			// given
			userSignup := commonsignup.NewUserSignup()
			userSignup.Status.Conditions = append(userSignup.Status.Conditions,
				toolchainv1alpha1.Condition{
					Type:   toolchainv1alpha1.UserSignupApproved,
					Status: corev1.ConditionTrue,
					Reason: "ApprovedAutomatically",
				})
			spaceProvisionerConfig := hspc.NewEnabledValidTenantSPC("member1")

			objs := []runtimeclient.Object{userSignup, spaceProvisionerConfig}
			if strings.Contains(v.description, "spacetier") { // when testing missing spacetier then create mur and usertier so that the error is about space tier
				objs = append(objs, newMasterUserRecord(userSignup, "member-1", deactivate30Tier.Name, "foo"))
				objs = append(objs, deactivate30Tier)
			}
			r, req, _ := prepareReconcile(t, userSignup.Name, v.config, objs...) // the tier does not exist

			commonconfig.ResetCache() // reset the config cache so that the update config is picked up
			InitializeCountersWithToolchainConfig(t, v.config, NewToolchainStatus(
				WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
					string(metrics.External): 1,
				}),
				WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
					"1,external": 1,
				}),
			))

			// when
			_, err := r.Reconcile(context.TODO(), req)

			// then
			// error reported, and request is requeued and userSignup status was updated
			require.Error(t, err)
			err = r.Client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
			require.NoError(t, err)
			t.Logf("usersignup status: %+v", userSignup.Status)
			test.AssertConditionsMatch(t, userSignup.Status.Conditions,
				toolchainv1alpha1.Condition{
					Type:   toolchainv1alpha1.UserSignupApproved,
					Status: corev1.ConditionTrue,
					Reason: "ApprovedAutomatically",
				},
				toolchainv1alpha1.Condition{
					Type:    toolchainv1alpha1.UserSignupComplete,
					Status:  corev1.ConditionFalse,
					Reason:  v.expectedReason,
					Message: v.expectedMsg,
				},
				toolchainv1alpha1.Condition{
					Type:   toolchainv1alpha1.UserSignupUserDeactivatingNotificationCreated,
					Status: corev1.ConditionFalse,
					Reason: "UserNotInPreDeactivation",
				},
				toolchainv1alpha1.Condition{
					Type:   toolchainv1alpha1.UserSignupUserDeactivatedNotificationCreated,
					Status: corev1.ConditionFalse,
					Reason: "UserIsActive",
				})
			assert.Equal(t, "approved", userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey])
			metricstest.AssertMetricsCounterEquals(t, 1, metrics.UserSignupApprovedTotal) // incremented, even though the provisioning failed due to missing NSTemplateTier
			metricstest.AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)   // incremented, even though the provisioning failed due to missing NSTemplateTier
			segmenttest.AssertNoMessageQueued(t, r.SegmentClient)
			AssertThatCountersAndMetrics(t).
				HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
					string(metrics.External): 1,
				}).
				HaveUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
					"1,external": 1,
					"1,internal": 1,
				})
		})
	}
}

func TestUnapprovedUserSignupWhenNoClusterReady(t *testing.T) {
	// given
	userSignup := commonsignup.NewUserSignup()

	spc1 := spc.NewSpaceProvisionerConfig("member1Spc", test.HostOperatorNs,
		spc.ReferencingToolchainCluster("member1"), spc.Enabled(true), spc.MaxNumberOfSpaces(1), spc.WithReadyConditionInvalid("intentionally invalid"))
	spc2 := spc.NewSpaceProvisionerConfig("member2Spc", test.HostOperatorNs,
		spc.ReferencingToolchainCluster("member2"), spc.Enabled(true), spc.WithReadyConditionInvalid("intentionally invalid"))

	config := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true), testconfig.Metrics().ForceSynchronization(false))
	r, req, _ := prepareReconcile(t, userSignup.Name, config, spc1, spc2, userSignup, baseNSTemplateTier)
	InitializeCountersWithToolchainConfig(t, config, NewToolchainStatus(
		WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
			"1,external": 2,
		}),
		WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
			string(metrics.External): 2,
		}),
	))

	// when
	res, err := r.Reconcile(context.TODO(), req)

	// then
	// it should not return an error but just wait for another reconcile triggered by updated ToolchainStatus
	require.NoError(t, err)
	assert.Empty(t, res.RequeueAfter)
	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)
	t.Logf("usersignup status: %+v", userSignup.Status)
	test.AssertConditionsMatch(t, userSignup.Status.Conditions,
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupApproved,
			Status: corev1.ConditionFalse,
			Reason: "PendingApproval",
		},
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupComplete,
			Status: corev1.ConditionFalse,
			Reason: "NoClusterAvailable",
		},
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupUserDeactivatingNotificationCreated,
			Status: corev1.ConditionFalse,
			Reason: "UserNotInPreDeactivation",
		},
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupUserDeactivatedNotificationCreated,
			Status: corev1.ConditionFalse,
			Reason: "UserIsActive",
		})

	assert.Equal(t, "pending", userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey])
	metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
	metricstest.AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)

	AssertThatCountersAndMetrics(t).
		HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
			string(metrics.External): 2,
		}).
		HaveUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
			"1,external": 2,
		})
}

func TestUserSignupFailedNoClusterWithCapacityAvailable(t *testing.T) {
	// given
	userSignup := commonsignup.NewUserSignup()

	spc1 := hspc.NewEnabledTenantSPC("member1")
	spc2 := hspc.NewEnabledTenantSPC("member2")
	config := commonconfig.NewToolchainConfigObjWithReset(t,
		testconfig.AutomaticApproval().Enabled(true), testconfig.Metrics().ForceSynchronization(false))
	r, req, _ := prepareReconcile(t, userSignup.Name, config, spc1, spc2, userSignup, baseNSTemplateTier)
	InitializeCountersWithToolchainConfig(t, config, NewToolchainStatus(
		WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
			string(metrics.External): 1,
		}),
		WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
			"1,external": 1,
		}),
	))

	// when
	res, err := r.Reconcile(context.TODO(), req)

	// then
	// it should not return an error but just wait for another reconcile triggered by updated ToolchainStatus
	require.NoError(t, err)
	assert.Empty(t, res.RequeueAfter)
	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)
	t.Logf("usersignup status: %+v", userSignup.Status)
	test.AssertConditionsMatch(t, userSignup.Status.Conditions,
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupApproved,
			Status: corev1.ConditionFalse,
			Reason: "PendingApproval",
		},
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupComplete,
			Status: corev1.ConditionFalse,
			Reason: "NoClusterAvailable",
		},
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupUserDeactivatingNotificationCreated,
			Status: corev1.ConditionFalse,
			Reason: "UserNotInPreDeactivation",
		},
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupUserDeactivatedNotificationCreated,
			Status: corev1.ConditionFalse,
			Reason: "UserIsActive",
		})

	assert.Equal(t, "pending", userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey])
	metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
	metricstest.AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)

	AssertThatCountersAndMetrics(t).
		HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
			string(metrics.External): 1,
		}).
		HaveUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
			"1,external": 1,
		})
}

func TestUserSignupWithManualApprovalApproved(t *testing.T) {
	// given
	userSignup := commonsignup.NewUserSignup(
		commonsignup.ApprovedManuallyAgo(time.Minute),
		commonsignup.WithRequestReceivedTimeAnnotation(time.Now()))
	spc1 := hspc.NewEnabledValidTenantSPC("member1")

	config := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true), testconfig.Metrics().ForceSynchronization(false))
	r, req, _ := prepareReconcile(t, userSignup.Name, config, spc1, userSignup, baseNSTemplateTier, deactivate30Tier)
	InitializeCountersWithToolchainConfig(t, config, NewToolchainStatus(
		WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
			string(metrics.External): 1,
		}),
		WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
			"1,external": 1,
		}),
	))

	// when
	res, err := r.Reconcile(context.TODO(), req)

	// then
	require.NoError(t, err)
	require.Equal(t, reconcile.Result{}, res)

	// Lookup the userSignup again
	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)
	assert.Equal(t, "approved", userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey])
	metricstest.AssertMetricsCounterEquals(t, 1, metrics.UserSignupApprovedTotal)
	metricstest.AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)
	murtest.AssertThatMasterUserRecords(t, r.Client).HaveCount(1)
	mur := murtest.AssertThatMasterUserRecord(t, userSignup.Name, r.Client).
		HasLabelWithValue(toolchainv1alpha1.MasterUserRecordOwnerLabelKey, userSignup.Name).
		HasUserAccounts(1).
		Get()
	segmenttest.AssertMessageQueuedForProvisionedMur(t, r.SegmentClient, userSignup, mur.Name)
	test.AssertConditionsMatch(t, userSignup.Status.Conditions,
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupApproved,
			Status: corev1.ConditionTrue,
			Reason: "ApprovedByAdmin",
		},
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupUserDeactivatingNotificationCreated,
			Status: corev1.ConditionFalse,
			Reason: "UserNotInPreDeactivation",
		},
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupUserDeactivatedNotificationCreated,
			Status: corev1.ConditionFalse,
			Reason: "UserIsActive",
		})
	AssertThatCountersAndMetrics(t).
		HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
			string(metrics.External): 1,
			string(metrics.Internal): 1,
		}).
		HaveUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
			"1,external": 1,
			"1,internal": 1,
		})

	t.Run("second reconcile - space should be created", func(t *testing.T) {
		// when
		res, err = r.Reconcile(context.TODO(), req)

		// then
		require.NoError(t, err)
		require.Equal(t, reconcile.Result{}, res)

		// space should be created on the second reconcile
		spacetest.AssertThatSpace(t, test.HostOperatorNs, userSignup.Name, r.Client).
			Exists().
			HasSpecTargetCluster("member1").
			HasTier(baseNSTemplateTier.Name)

		t.Run("third reconcile - spacebinding created and usersignup completed", func(t *testing.T) {
			// given
			setSpaceToReady(t, r.Client, mur.Name)
			setMURToReady(t, r.Client, userSignup.Spec.IdentityClaims.PreferredUsername)

			// when
			res, err = r.Reconcile(context.TODO(), req)

			// then
			require.NoError(t, err)
			require.Equal(t, reconcile.Result{}, res)
			spacebindingtest.AssertThatSpaceBinding(t, test.HostOperatorNs, mur.Name, userSignup.Name, r.Client).
				Exists().
				HasLabelWithValue(toolchainv1alpha1.SpaceCreatorLabelKey, userSignup.Name).
				HasLabelWithValue(toolchainv1alpha1.SpaceBindingMasterUserRecordLabelKey, userSignup.Name).
				HasLabelWithValue(toolchainv1alpha1.SpaceBindingSpaceLabelKey, userSignup.Name).
				HasSpec(mur.Name, userSignup.Name, "admin")

			// Lookup the userSignup one more time and check the conditions are updated
			err = r.Client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
			require.NoError(t, err)
			require.Equal(t, userSignup.Status.CompliantUsername, mur.Name)
			assert.Equal(t, "approved", userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey])
			metricstest.AssertMetricsCounterEquals(t, 1, metrics.UserSignupApprovedTotal)
			metricstest.AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)
			segmenttest.AssertMessageQueuedForProvisionedMur(t, r.SegmentClient, userSignup, mur.Name)
			test.AssertConditionsMatch(t, userSignup.Status.Conditions,
				toolchainv1alpha1.Condition{
					Type:   toolchainv1alpha1.UserSignupApproved,
					Status: corev1.ConditionTrue,
					Reason: "ApprovedByAdmin",
				},
				toolchainv1alpha1.Condition{
					Type:   toolchainv1alpha1.UserSignupComplete,
					Status: corev1.ConditionTrue,
				},
				toolchainv1alpha1.Condition{
					Type:   toolchainv1alpha1.UserSignupUserDeactivatingNotificationCreated,
					Status: corev1.ConditionFalse,
					Reason: "UserNotInPreDeactivation",
				},
				toolchainv1alpha1.Condition{
					Type:   toolchainv1alpha1.UserSignupUserDeactivatedNotificationCreated,
					Status: corev1.ConditionFalse,
					Reason: "UserIsActive",
				})
			AssertThatCountersAndMetrics(t).
				HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
					string(metrics.External): 1,
					string(metrics.Internal): 1,
				}).
				HaveUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
					"1,external": 1,
					"1,internal": 1,
				})
		})
	})

	metricstest.AssertAllHistogramBucketsAreEmpty(t, metrics.UserSignupProvisionTimeHistogram)
}

func TestUserSignupWithNoApprovalPolicyTreatedAsManualApproved(t *testing.T) {
	// given
	userSignup := commonsignup.NewUserSignup(commonsignup.ApprovedManuallyAgo(time.Minute))

	config := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.Metrics().ForceSynchronization(false))

	spc1 := hspc.NewEnabledValidTenantSPC("member1")

	r, req, _ := prepareReconcile(t, userSignup.Name, config, spc1, userSignup, baseNSTemplateTier, deactivate30Tier)
	InitializeCountersWithToolchainConfig(t, config, NewToolchainStatus(
		WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
			string(metrics.External): 1,
		}),
		WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
			"1,external": 1,
		}),
	))

	// when
	res, err := r.Reconcile(context.TODO(), req)

	// then
	require.NoError(t, err)
	require.Equal(t, reconcile.Result{}, res)

	// Lookup the userSignup again
	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)
	assert.Equal(t, "approved", userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey])
	metricstest.AssertMetricsCounterEquals(t, 1, metrics.UserSignupApprovedTotal)
	metricstest.AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)

	murtest.AssertThatMasterUserRecords(t, r.Client).HaveCount(1)
	mur := murtest.AssertThatMasterUserRecord(t, userSignup.Name, r.Client).
		HasLabelWithValue(toolchainv1alpha1.MasterUserRecordOwnerLabelKey, userSignup.Name).
		HasUserAccounts(1).
		Get()
	segmenttest.AssertMessageQueuedForProvisionedMur(t, r.SegmentClient, userSignup, mur.Name)

	test.AssertConditionsMatch(t, userSignup.Status.Conditions,
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupApproved,
			Status: corev1.ConditionTrue,
			Reason: "ApprovedByAdmin",
		},
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupUserDeactivatingNotificationCreated,
			Status: corev1.ConditionFalse,
			Reason: "UserNotInPreDeactivation",
		},
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupUserDeactivatedNotificationCreated,
			Status: corev1.ConditionFalse,
			Reason: "UserIsActive",
		})
	AssertThatCountersAndMetrics(t).
		HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
			string(metrics.External): 1,
			string(metrics.Internal): 1,
		}).
		HaveUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
			"1,external": 1,
			"1,internal": 1,
		})

	t.Run("second reconcile", func(t *testing.T) {
		// when
		res, err = r.Reconcile(context.TODO(), req)

		// then
		require.NoError(t, err)
		require.Equal(t, reconcile.Result{}, res)

		// space should be created on the second reconcile
		spacetest.AssertThatSpace(t, test.HostOperatorNs, mur.Name, r.Client).
			Exists().
			HasSpecTargetCluster("member1").
			HasTier(baseNSTemplateTier.Name)

		t.Run("third reconcile", func(t *testing.T) {
			// given
			setSpaceToReady(t, r.Client, mur.Name)
			setMURToReady(t, r.Client, userSignup.Spec.IdentityClaims.PreferredUsername)

			// when
			res, err = r.Reconcile(context.TODO(), req)

			// then
			require.NoError(t, err)
			require.Equal(t, reconcile.Result{}, res)

			spacebindingtest.AssertThatSpaceBinding(t, test.HostOperatorNs, mur.Name, userSignup.Name, r.Client).
				Exists().
				HasLabelWithValue(toolchainv1alpha1.SpaceCreatorLabelKey, userSignup.Name).
				HasLabelWithValue(toolchainv1alpha1.SpaceBindingMasterUserRecordLabelKey, mur.Name).
				HasLabelWithValue(toolchainv1alpha1.SpaceBindingSpaceLabelKey, userSignup.Name).
				HasSpec(mur.Name, userSignup.Name, "admin")

			// Lookup the userSignup one more and check the conditions are updated
			err = r.Client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
			require.NoError(t, err)
			require.Equal(t, userSignup.Status.CompliantUsername, mur.Name)

			assert.Equal(t, "approved", userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey])
			metricstest.AssertMetricsCounterEquals(t, 1, metrics.UserSignupApprovedTotal)
			metricstest.AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)
			segmenttest.AssertMessageQueuedForProvisionedMur(t, r.SegmentClient, userSignup, mur.Name)

			test.AssertConditionsMatch(t, userSignup.Status.Conditions,
				toolchainv1alpha1.Condition{
					Type:   toolchainv1alpha1.UserSignupApproved,
					Status: corev1.ConditionTrue,
					Reason: "ApprovedByAdmin",
				},
				toolchainv1alpha1.Condition{
					Type:   toolchainv1alpha1.UserSignupComplete,
					Status: corev1.ConditionTrue,
				},
				toolchainv1alpha1.Condition{
					Type:   toolchainv1alpha1.UserSignupUserDeactivatingNotificationCreated,
					Status: corev1.ConditionFalse,
					Reason: "UserNotInPreDeactivation",
				},
				toolchainv1alpha1.Condition{
					Type:   toolchainv1alpha1.UserSignupUserDeactivatedNotificationCreated,
					Status: corev1.ConditionFalse,
					Reason: "UserIsActive",
				})
			AssertThatCountersAndMetrics(t).
				HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
					string(metrics.External): 1,
					string(metrics.Internal): 1,
				}).
				HaveUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
					"1,external": 1,
					"1,internal": 1,
				})
		})
	})
}

func TestUserSignupWithManualApprovalNotApproved(t *testing.T) {
	// given
	userSignup := commonsignup.NewUserSignup()
	spc1 := hspc.NewEnabledValidTenantSPC("member1")

	config := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.Metrics().ForceSynchronization(false))
	r, req, _ := prepareReconcile(t, userSignup.Name, config, spc1, userSignup, baseNSTemplateTier)
	InitializeCountersWithToolchainConfig(t, config, NewToolchainStatus(
		WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
			string(metrics.External): 1,
		}),
		WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
			"1,external": 1,
		}),
	))

	// when
	res, err := r.Reconcile(context.TODO(), req)

	// then
	require.NoError(t, err)
	require.Equal(t, reconcile.Result{}, res)

	// Lookup the userSignup again
	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)
	assert.Equal(t, "pending", userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey])
	metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
	metricstest.AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)

	// There should be no MasterUserRecords
	murtest.AssertThatMasterUserRecords(t, r.Client).HaveCount(0)

	test.AssertConditionsMatch(t, userSignup.Status.Conditions,
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupApproved,
			Status: corev1.ConditionFalse,
			Reason: "PendingApproval",
		},
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupComplete,
			Status: corev1.ConditionFalse,
			Reason: "PendingApproval",
		},
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupUserDeactivatingNotificationCreated,
			Status: corev1.ConditionFalse,
			Reason: "UserNotInPreDeactivation",
		},
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupUserDeactivatedNotificationCreated,
			Status: corev1.ConditionFalse,
			Reason: "UserIsActive",
		})
	AssertThatCountersAndMetrics(t).
		HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
			string(metrics.External): 1,
		}).
		HaveUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
			"1,external": 1,
		})
}

func TestUserSignupWithAutoApprovalWithTargetCluster(t *testing.T) {
	// given
	userSignup := commonsignup.NewUserSignup(commonsignup.WithTargetCluster("east"))

	spc1 := hspc.NewEnabledValidTenantSPC("member1")
	config := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true), testconfig.Metrics().ForceSynchronization(false))
	r, req, _ := prepareReconcile(t, userSignup.Name, config, spc1, userSignup, baseNSTemplateTier, deactivate30Tier)
	InitializeCountersWithToolchainConfig(t, config, NewToolchainStatus(
		WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
			string(metrics.External): 1,
		}),
		WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
			"1,external": 1,
		}),
	))

	// when
	res, err := r.Reconcile(context.TODO(), req)

	// then
	require.NoError(t, err)
	require.Equal(t, reconcile.Result{}, res)

	// Lookup the userSignup again
	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)
	assert.Equal(t, "approved", userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey])
	metricstest.AssertMetricsCounterEquals(t, 1, metrics.UserSignupApprovedTotal)
	metricstest.AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)

	murtest.AssertThatMasterUserRecords(t, r.Client).HaveCount(1)
	mur := murtest.AssertThatMasterUserRecord(t, userSignup.Name, r.Client).
		HasLabelWithValue(toolchainv1alpha1.MasterUserRecordOwnerLabelKey, userSignup.Name).
		HasUserAccounts(1).
		HasTier(*deactivate30Tier).
		Get()
	segmenttest.AssertMessageQueuedForProvisionedMur(t, r.SegmentClient, userSignup, mur.Name)

	test.AssertConditionsMatch(t, userSignup.Status.Conditions,
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupApproved,
			Status: corev1.ConditionTrue,
			Reason: "ApprovedAutomatically",
		},
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupUserDeactivatingNotificationCreated,
			Status: corev1.ConditionFalse,
			Reason: "UserNotInPreDeactivation",
		},
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupUserDeactivatedNotificationCreated,
			Status: corev1.ConditionFalse,
			Reason: "UserIsActive",
		})
	AssertThatCountersAndMetrics(t).
		HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
			string(metrics.External): 1,
			string(metrics.Internal): 1,
		}).
		HaveUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
			"1,external": 1,
			"1,internal": 1,
		})

	t.Run("second reconcile", func(t *testing.T) {
		// when
		res, err = r.Reconcile(context.TODO(), req)

		// then
		require.NoError(t, err)
		require.Equal(t, reconcile.Result{}, res)

		// space should be created on the second reconcile
		spacetest.AssertThatSpace(t, test.HostOperatorNs, userSignup.Name, r.Client).
			Exists().
			HasSpecTargetCluster("east").
			HasTier(baseNSTemplateTier.Name)

		t.Run("third reconcile", func(t *testing.T) {
			// given
			setSpaceToReady(t, r.Client, userSignup.Name)
			setMURToReady(t, r.Client, userSignup.Spec.IdentityClaims.PreferredUsername)

			// when
			res, err = r.Reconcile(context.TODO(), req)

			// then
			require.NoError(t, err)
			require.Equal(t, reconcile.Result{}, res)

			spacebindingtest.AssertThatSpaceBinding(t, test.HostOperatorNs, mur.Name, userSignup.Name, r.Client).
				Exists().
				HasLabelWithValue(toolchainv1alpha1.SpaceCreatorLabelKey, userSignup.Name).
				HasLabelWithValue(toolchainv1alpha1.SpaceBindingMasterUserRecordLabelKey, mur.Name).
				HasLabelWithValue(toolchainv1alpha1.SpaceBindingSpaceLabelKey, userSignup.Name).
				HasSpec(mur.Name, userSignup.Name, "admin")

			// Lookup the userSignup one more and check the conditions are updated
			err = r.Client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
			require.NoError(t, err)
			require.Equal(t, userSignup.Status.CompliantUsername, mur.Name)

			assert.Equal(t, "approved", userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey])
			metricstest.AssertMetricsCounterEquals(t, 1, metrics.UserSignupApprovedTotal)
			metricstest.AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)
			segmenttest.AssertMessageQueuedForProvisionedMur(t, r.SegmentClient, userSignup, mur.Name)

			test.AssertConditionsMatch(t, userSignup.Status.Conditions,
				toolchainv1alpha1.Condition{
					Type:   toolchainv1alpha1.UserSignupApproved,
					Status: corev1.ConditionTrue,
					Reason: "ApprovedAutomatically",
				},
				toolchainv1alpha1.Condition{
					Type:   toolchainv1alpha1.UserSignupComplete,
					Status: corev1.ConditionTrue,
				},
				toolchainv1alpha1.Condition{
					Type:   toolchainv1alpha1.UserSignupUserDeactivatingNotificationCreated,
					Status: corev1.ConditionFalse,
					Reason: "UserNotInPreDeactivation",
				},
				toolchainv1alpha1.Condition{
					Type:   toolchainv1alpha1.UserSignupUserDeactivatedNotificationCreated,
					Status: corev1.ConditionFalse,
					Reason: "UserIsActive",
				})
			AssertThatCountersAndMetrics(t).
				HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
					string(metrics.External): 1,
					string(metrics.Internal): 1,
				}).
				HaveUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
					"1,external": 1,
					"1,internal": 1,
				})
		})
	})
}

func TestUserSignupWithMissingApprovalPolicyTreatedAsManual(t *testing.T) {
	// given
	userSignup := commonsignup.NewUserSignup(commonsignup.WithTargetCluster("east"))

	r, req, _ := prepareReconcile(t, userSignup.Name, nil, userSignup, baseNSTemplateTier)
	InitializeCountersWithMetricsSyncDisabled(t, NewToolchainStatus(
		WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
			string(metrics.External): 1,
		}),
		WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
			"1,external": 1,
		}),
	))

	// when
	res, err := r.Reconcile(context.TODO(), req)

	// then
	require.NoError(t, err)
	require.Equal(t, reconcile.Result{}, res)

	// Lookup the userSignup again
	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)
	assert.Equal(t, "pending", userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey])
	metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
	metricstest.AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)

	test.AssertConditionsMatch(t, userSignup.Status.Conditions,
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupApproved,
			Status: corev1.ConditionFalse,
			Reason: "PendingApproval",
		},
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupComplete,
			Status: corev1.ConditionFalse,
			Reason: "PendingApproval",
		},
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupUserDeactivatingNotificationCreated,
			Status: corev1.ConditionFalse,
			Reason: "UserNotInPreDeactivation",
		},
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupUserDeactivatedNotificationCreated,
			Status: corev1.ConditionFalse,
			Reason: "UserIsActive",
		})
	AssertThatCountersAndMetrics(t).
		HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
			string(metrics.External): 1,
		}).
		HaveUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
			"1,external": 1,
		})
}

type MurOrSpaceCreateFails struct {
	testName      string
	expectedError string
}

func TestUserSignupMUROrSpaceOrSpaceBindingCreateFails(t *testing.T) {
	for _, testcase := range []MurOrSpaceCreateFails{
		{
			testName:      "create mur error",
			expectedError: "error creating MasterUserRecord: create mur error",
		},
		{
			testName:      "create space error",
			expectedError: "error creating Space: create space error",
		},
		{
			testName:      "create spacebinding error",
			expectedError: "error creating SpaceBinding: create spacebinding error",
		},
	} {
		t.Run(testcase.testName, func(t *testing.T) {
			// given
			userSignup := commonsignup.NewUserSignup(commonsignup.ApprovedManually())

			mur := newMasterUserRecord(userSignup, "member1", deactivate30Tier.Name, "foo")
			mur.Labels = map[string]string{toolchainv1alpha1.MasterUserRecordOwnerLabelKey: userSignup.Name}

			space := NewSpace(userSignup, "member1", "foo", "base")

			spc1 := hspc.NewEnabledValidTenantSPC("member1")
			initObjs := []runtimeclient.Object{userSignup, baseNSTemplateTier, deactivate30Tier, spc1}
			switch testcase.testName {
			case "create space error":
				// mur must exist first, space is created on the reconcile after the mur is created
				initObjs = append(initObjs, mur)
			case "create spacebinding error":
				// mur and space must exist first, spacebinding is created on the reconcile after the space is created
				initObjs = append(initObjs, mur, space)
			}
			r, req, fakeClient := prepareReconcile(t, userSignup.Name, nil, initObjs...)
			InitializeCountersWithMetricsSyncDisabled(t, NewToolchainStatus(
				WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
					string(metrics.External): 1,
				}),
				WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
					"1,external": 1,
				}),
			))

			fakeClient.MockCreate = func(ctx context.Context, obj runtimeclient.Object, _ ...runtimeclient.CreateOption) error {
				switch obj.(type) {
				case *toolchainv1alpha1.MasterUserRecord:
					if testcase.testName == "create mur error" {
						return errors.New("create mur error")
					}
					return fakeClient.Client.Create(ctx, obj)
				case *toolchainv1alpha1.Space:
					if testcase.testName == "create space error" {
						return errors.New("create space error")
					}
					return fakeClient.Client.Create(ctx, obj)
				case *toolchainv1alpha1.SpaceBinding:
					if testcase.testName == "create spacebinding error" {
						return errors.New("create spacebinding error")
					}
					return fakeClient.Client.Create(ctx, obj)
				default:
					return fakeClient.Client.Create(ctx, obj)
				}
			}

			// when
			res, err := r.Reconcile(context.TODO(), req)

			// then
			require.EqualError(t, err, testcase.expectedError)
			require.Equal(t, reconcile.Result{}, res)
			AssertThatCountersAndMetrics(t).
				HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
					string(metrics.External): 1,
				}).
				HaveUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
					"1,external": 1,
					"1,internal": 1,
				})

			err = r.Client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
			require.NoError(t, err)
			assert.Equal(t, "approved", userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey])
			metricstest.AssertMetricsCounterEquals(t, 1, metrics.UserSignupApprovedTotal)
			metricstest.AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)
			segmenttest.AssertNoMessageQueued(t, r.SegmentClient)
		})
	}
}

func TestUserSignupMURReadFails(t *testing.T) {
	// given
	userSignup := commonsignup.NewUserSignup(commonsignup.ApprovedManually())

	spc1 := hspc.NewEnabledValidTenantSPC("member1")
	r, req, fakeClient := prepareReconcile(t, userSignup.Name, nil, spc1, userSignup)
	InitializeCountersWithMetricsSyncDisabled(t, NewToolchainStatus(
		WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
			string(metrics.External): 1,
		}),
		WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
			"1,external": 1,
		}),
	))

	fakeClient.MockGet = func(ctx context.Context, key runtimeclient.ObjectKey, obj runtimeclient.Object, opts ...runtimeclient.GetOption) error {
		switch obj.(type) {
		case *toolchainv1alpha1.MasterUserRecord:
			return errors.New("failed to lookup MUR")
		default:
			return fakeClient.Client.Get(ctx, key, obj, opts...)
		}
	}

	// when
	_, err := r.Reconcile(context.TODO(), req)

	// then
	require.Error(t, err)
	AssertThatCountersAndMetrics(t).
		HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
			string(metrics.External): 1,
		}).
		HaveUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
			"1,external": 1,
			"1,internal": 1, // incremented
		})

	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)
	assert.Equal(t, "approved", userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey])
	metricstest.AssertMetricsCounterEquals(t, 1, metrics.UserSignupApprovedTotal)
	metricstest.AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)
}

func TestUserSignupSetStatusApprovedByAdminFails(t *testing.T) {
	// given
	userSignup := commonsignup.NewUserSignup(commonsignup.ApprovedManually())
	userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey] = "approved"

	spc1 := hspc.NewEnabledValidTenantSPC("member1")
	r, req, fakeClient := prepareReconcile(t, userSignup.Name, nil, spc1, userSignup)
	InitializeCountersWithMetricsSyncDisabled(t, NewToolchainStatus(
		WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
			string(metrics.External): 1,
		}),
		WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
			"1,external": 1,
		}),
	))

	fakeClient.MockStatusUpdate = func(ctx context.Context, obj runtimeclient.Object, _ ...runtimeclient.SubResourceUpdateOption) error {
		switch obj.(type) {
		case *toolchainv1alpha1.UserSignup:
			return errors.New("failed to update UserSignup status")
		default:
			return fakeClient.Client.Update(ctx, obj)
		}
	}

	// when
	res, err := r.Reconcile(context.TODO(), req)

	// then
	require.Error(t, err)
	require.Equal(t, reconcile.Result{}, res)
	AssertThatCountersAndMetrics(t).
		HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
			string(metrics.External): 1,
		}).
		HaveUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
			"1,external": 1,
		})

	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)
	assert.Equal(t, "approved", userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey])
	metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal) // zero since starting state was approved
	metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupUniqueTotal)   // zero since starting state was approved
	assert.Empty(t, userSignup.Status.Conditions)
}

func TestUserSignupSetStatusApprovedAutomaticallyFails(t *testing.T) {
	// given
	userSignup := commonsignup.NewUserSignup()

	spc1 := hspc.NewEnabledValidTenantSPC("member1")
	config := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true), testconfig.Metrics().ForceSynchronization(false))
	r, req, fakeClient := prepareReconcile(t, userSignup.Name, config, spc1, userSignup)
	InitializeCountersWithToolchainConfig(t, config, NewToolchainStatus(
		WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
			string(metrics.External): 1,
		}),
		WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
			"1,external": 1,
		}),
	))

	fakeClient.MockStatusUpdate = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.SubResourceUpdateOption) error {
		switch obj.(type) {
		case *toolchainv1alpha1.UserSignup:
			return errors.New("failed to update UserSignup status")
		default:
			return fakeClient.Client.Update(ctx, obj)
		}
	}

	// when
	res, err := r.Reconcile(context.TODO(), req)

	// then
	require.Error(t, err)
	require.Equal(t, reconcile.Result{}, res)
	AssertThatCountersAndMetrics(t).
		HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
			string(metrics.External): 1,
		}).
		HaveUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
			"1,external": 1,
		})

	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)
	assert.Equal(t, toolchainv1alpha1.UserSignupStateLabelValueNotReady, userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey])
	metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
	metricstest.AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)
	assert.Empty(t, userSignup.Status.Conditions)
}

func TestUserSignupSetStatusNoClustersAvailableFails(t *testing.T) {
	// given
	userSignup := commonsignup.NewUserSignup()

	config := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true), testconfig.Metrics().ForceSynchronization(false))
	r, req, fakeClient := prepareReconcile(t, userSignup.Name, config, userSignup)
	InitializeCountersWithToolchainConfig(t, config, NewToolchainStatus(
		WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
			string(metrics.External): 1,
		}),
		WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
			"1,external": 1,
		}),
	))

	fakeClient.MockStatusUpdate = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.SubResourceUpdateOption) error {
		switch obj := obj.(type) {
		case *toolchainv1alpha1.UserSignup:
			for _, cond := range obj.Status.Conditions {
				if cond.Reason == "NoClusterAvailable" {
					return errors.New("failed to update UserSignup status")
				}
			}
			return fakeClient.Client.Update(ctx, obj)
		default:
			return fakeClient.Client.Update(ctx, obj)
		}
	}

	// when
	res, err := r.Reconcile(context.TODO(), req)

	// then
	require.Error(t, err)
	require.Equal(t, reconcile.Result{}, res)
	AssertThatCountersAndMetrics(t).
		HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
			string(metrics.External): 1,
		}).
		HaveUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
			"1,external": 1,
		})

	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)
	assert.Equal(t, "pending", userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey])
	metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
	metricstest.AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)
}

func TestUserSignupWithExistingMUROK(t *testing.T) {
	// given
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	userSignup := commonsignup.NewUserSignup()
	userSignup.Spec.IdentityClaims.Email = "foo@redhat.com"
	userSignup.Labels = map[string]string{
		toolchainv1alpha1.UserSignupUserEmailHashLabelKey: "fd2addbd8d82f0d2dc088fa122377eaa",
		"toolchain.dev.openshift.com/approved":            "true",
	}
	userSignup.Spec.IdentityClaims.OriginalSub = "original-sub:foo"

	// Create a MUR with the same UserID but don't set the OriginalSub property
	mur := &toolchainv1alpha1.MasterUserRecord{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: test.HostOperatorNs,
			Labels: map[string]string{
				toolchainv1alpha1.MasterUserRecordOwnerLabelKey: userSignup.Name,
			},
		},
	}

	space := NewSpace(userSignup, "member1", "foo", "base")

	spacebinding := spacebindingtest.NewSpaceBinding("foo", "foo", "admin", userSignup.Name)

	spc1 := hspc.NewEnabledValidTenantSPC("member1")
	config := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true), testconfig.Metrics().ForceSynchronization(false))
	r, req, _ := prepareReconcile(t, userSignup.Name, config, spc1, userSignup, mur, space, spacebinding, baseNSTemplateTier, deactivate30Tier)
	InitializeCountersWithToolchainConfig(t, config, NewToolchainStatus(
		WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
			string(metrics.External): 1,
		}),
		WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
			"1,external": 1,
		}),
	))

	// when
	_, err := r.Reconcile(context.TODO(), req)

	// then
	require.NoError(t, err)

	// The first reconciliation will result in a change in the MUR.  We will load it here and confirm that the
	// OriginalSub property has now been set
	murInstance := &toolchainv1alpha1.MasterUserRecord{}
	err = r.Client.Get(context.TODO(), types.NamespacedName{
		Namespace: test.HostOperatorNs,
		Name:      mur.Name,
	}, murInstance)
	require.NoError(t, err)

	t.Run("reconcile a second time to update UserSignup.Status", func(t *testing.T) {
		// given the space is ready
		setSpaceToReady(t, r.Client, "foo")
		setMURToReady(t, r.Client, "foo")

		// Reconcile again so that the userSignup status is now updated
		_, err = r.Reconcile(context.TODO(), req)

		// then
		require.NoError(t, err)

		instance := &toolchainv1alpha1.UserSignup{}
		err = r.Client.Get(context.TODO(), types.NamespacedName{
			Namespace: test.HostOperatorNs,
			Name:      userSignup.Name,
		}, instance)
		require.NoError(t, err)
		assert.Equal(t, "approved", instance.Labels[toolchainv1alpha1.UserSignupStateLabelKey])
		metricstest.AssertMetricsCounterEquals(t, 1, metrics.UserSignupApprovedTotal)
		metricstest.AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)

		require.Equal(t, mur.Name, instance.Status.CompliantUsername)
		test.AssertContainsCondition(t, instance.Status.Conditions, toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupComplete,
			Status: corev1.ConditionTrue,
		})
		AssertThatCountersAndMetrics(t).
			HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
				string(metrics.External): 1,
			}).
			HaveUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
				"1,external": 1,
				"1,internal": 1,
			})
	})
}

func TestUserSignupWithExistingMURDifferentUserIDOK(t *testing.T) {
	// given
	userSignup := commonsignup.NewUserSignup(commonsignup.ApprovedManually())
	// Create a MUR with a different UserID
	mur := &toolchainv1alpha1.MasterUserRecord{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: test.HostOperatorNs,
			Labels: map[string]string{
				toolchainv1alpha1.MasterUserRecordOwnerLabelKey: uuid.Must(uuid.NewV4()).String(),
				"toolchain.dev.openshift.com/approved":          "true",
			},
		},
	}

	spc1 := hspc.NewEnabledValidTenantSPC("member1")
	config := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true), testconfig.Metrics().ForceSynchronization(false))
	r, req, _ := prepareReconcile(t, userSignup.Name, config, spc1, userSignup, mur, baseNSTemplateTier, deactivate30Tier)
	InitializeCountersWithToolchainConfig(t, config, NewToolchainStatus(
		WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
			string(metrics.External): 1,
		}),
		WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
			"1,external": 1,
		}),
	))

	// when
	_, err := r.Reconcile(context.TODO(), req)

	// then
	require.NoError(t, err)

	// We should now have 2 MURs
	murtest.AssertThatMasterUserRecords(t, r.Client).HaveCount(2)
	AssertThatCountersAndMetrics(t).
		HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
			string(metrics.External): 1,
			string(metrics.Internal): 1,
		}).
		HaveUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
			"1,external": 1,
			"1,internal": 1,
		})

	key := types.NamespacedName{
		Namespace: test.HostOperatorNs,
		Name:      userSignup.Name,
	}
	instance := &toolchainv1alpha1.UserSignup{}
	err = r.Client.Get(context.TODO(), key, instance)
	require.NoError(t, err)
	assert.Equal(t, "approved", instance.Labels[toolchainv1alpha1.UserSignupStateLabelKey])
	metricstest.AssertMetricsCounterEquals(t, 1, metrics.UserSignupApprovedTotal)
	metricstest.AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)
	usMur := murtest.AssertThatMasterUserRecord(t, userSignup.Name, r.Client).Get()
	segmenttest.AssertMessageQueuedForProvisionedMur(t, r.SegmentClient, userSignup, usMur.Name)

	t.Run("second reconcile", func(t *testing.T) {
		// when
		_, err = r.Reconcile(context.TODO(), req)

		// then
		require.NoError(t, err)

		err = r.Client.Get(context.TODO(), key, instance)
		require.NoError(t, err)

		assert.Equal(t, "approved", instance.Labels[toolchainv1alpha1.UserSignupStateLabelKey])
		metricstest.AssertMetricsCounterEquals(t, 1, metrics.UserSignupApprovedTotal)
		metricstest.AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)
		usMur := murtest.AssertThatMasterUserRecord(t, userSignup.Name, r.Client).Get()
		segmenttest.AssertMessageQueuedForProvisionedMur(t, r.SegmentClient, userSignup, usMur.Name)

		t.Run("verify usersignup on third reconcile", func(t *testing.T) {
			// given space is ready
			setSpaceToReady(t, r.Client, userSignup.Name)
			setMURToReady(t, r.Client, userSignup.Spec.IdentityClaims.PreferredUsername)

			// when
			res, err := r.Reconcile(context.TODO(), req)

			// then
			require.NoError(t, err)
			require.Equal(t, reconcile.Result{}, res)

			err = r.Client.Get(context.TODO(), key, instance)
			require.NoError(t, err)

			require.Equal(t, userSignup.Name, instance.Status.CompliantUsername)

			// Confirm that the mur exists
			mur = &toolchainv1alpha1.MasterUserRecord{}
			err = r.Client.Get(context.TODO(), types.NamespacedName{Namespace: test.HostOperatorNs, Name: instance.Status.CompliantUsername}, mur)
			require.NoError(t, err)
			require.Equal(t, instance.Name, mur.Labels[toolchainv1alpha1.MasterUserRecordOwnerLabelKey])
			require.Equal(t, mur.Name, instance.Status.CompliantUsername)
			cond, found := condition.FindConditionByType(instance.Status.Conditions, toolchainv1alpha1.UserSignupComplete)
			require.True(t, found)
			require.Equal(t, corev1.ConditionTrue, cond.Status)
			AssertThatCountersAndMetrics(t).
				HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
					string(metrics.External): 1,
					string(metrics.Internal): 1,
				}).
				HaveUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
					"1,external": 1,
					"1,internal": 1,
				})
		})
	})
}

func TestUserSignupPropagatedClaimsSynchronizedToMURWhenModified(t *testing.T) {
	// given
	userSignup := commonsignup.NewUserSignup()
	spc1 := hspc.NewEnabledValidTenantSPC("member1")

	config := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true), testconfig.Metrics().ForceSynchronization(false))
	r, req, _ := prepareReconcile(t, userSignup.Name, config, spc1, userSignup,
		baseNSTemplateTier, deactivate30Tier)

	// when - The first reconcile creates the MasterUserRecord
	res, err := r.Reconcile(context.TODO(), req)

	// then
	require.NoError(t, err)
	require.Equal(t, reconcile.Result{}, res)

	// Lookup the user signup again
	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)
	assert.Equal(t, "approved", userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey])

	// We should now have a MUR
	murtest.AssertThatMasterUserRecords(t, r.Client).HaveCount(1)

	key := types.NamespacedName{
		Namespace: test.HostOperatorNs,
		Name:      userSignup.Name,
	}
	instance := &toolchainv1alpha1.UserSignup{}
	err = r.Client.Get(context.TODO(), key, instance)
	require.NoError(t, err)
	assert.Equal(t, "approved", instance.Labels[toolchainv1alpha1.UserSignupStateLabelKey])

	mur := toolchainv1alpha1.MasterUserRecord{}
	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, &mur)
	require.NoError(t, err)

	require.Equal(t, mur.Spec.PropagatedClaims, userSignup.Spec.IdentityClaims.PropagatedClaims)

	// Modify one of the propagated claims
	userSignup.Spec.IdentityClaims.UserID = "314159265358979"

	// Reconcile the UserSignup again
	r, req, _ = prepareReconcile(t, userSignup.Name, nil, spc1, userSignup, deactivate30Tier)
	res, err = r.Reconcile(context.TODO(), req)
	require.NoError(t, err)
	require.Equal(t, reconcile.Result{}, res)

	// Reload the MUR
	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, &mur)
	require.NoError(t, err)

	// Confirm the propagated claim has been updated in the MUR
	require.Equal(t, "314159265358979", mur.Spec.PropagatedClaims.UserID)
}

func TestUserSignupWithSpecialCharOK(t *testing.T) {
	// given
	userSignup := commonsignup.NewUserSignup(commonsignup.WithUsername("foo#$%^bar@redhat.com"))

	spc1 := hspc.NewEnabledValidTenantSPC("member1")
	config := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true), testconfig.Metrics().ForceSynchronization(false))
	r, req, _ := prepareReconcile(t, userSignup.Name, config, spc1, userSignup, baseNSTemplateTier, deactivate30Tier)
	InitializeCountersWithToolchainConfig(t, config, NewToolchainStatus(
		WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
			string(metrics.External): 1,
		}),
		WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
			"1,external": 1,
		}),
	))

	// when
	_, err := r.Reconcile(context.TODO(), req)

	// then
	require.NoError(t, err)

	murtest.AssertThatMasterUserRecords(t, r.Client).HaveCount(1)
	murtest.AssertThatMasterUserRecord(t, "foo-bar", r.Client).HasNoConditions()
	AssertThatCountersAndMetrics(t).
		HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
			string(metrics.External): 1,
			string(metrics.Internal): 1,
		}).
		HaveUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
			"1,external": 1,
			"1,internal": 1,
		})

	metricstest.AssertMetricsCounterEquals(t, 1, metrics.UserSignupApprovedTotal)
	metricstest.AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)
	mur := murtest.AssertThatMasterUserRecord(t, "foo-bar", r.Client).Get()
	segmenttest.AssertMessageQueuedForProvisionedMur(t, r.SegmentClient, userSignup, mur.Name)
}

func TestUserSignupDeactivatedAfterMURCreated(t *testing.T) {
	// given
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	userSignup := commonsignup.NewUserSignup(commonsignup.Deactivated())
	userSignup.Status = toolchainv1alpha1.UserSignupStatus{
		Conditions: []toolchainv1alpha1.Condition{
			{
				Type:   toolchainv1alpha1.UserSignupComplete,
				Status: corev1.ConditionTrue,
			},
			{
				Type:   toolchainv1alpha1.UserSignupApproved,
				Status: corev1.ConditionTrue,
				Reason: "ApprovedAutomatically",
			},
		},
		CompliantUsername: "john-doe",
	}

	userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey] = toolchainv1alpha1.UserSignupStateLabelValueApproved
	userSignup.Labels["toolchain.dev.openshift.com/approved"] = "true"
	key := test.NamespacedName(test.HostOperatorNs, userSignup.Name)

	t.Run("when MUR exists, then it should be deleted", func(t *testing.T) {
		// given
		mur := murtest.NewMasterUserRecord(t, "john-doe", murtest.MetaNamespace(test.HostOperatorNs))
		mur.Labels = map[string]string{toolchainv1alpha1.MasterUserRecordOwnerLabelKey: userSignup.Name}

		space := spacetest.NewSpace(test.HostOperatorNs, mur.Name,
			spacetest.WithCreatorLabel(userSignup.Name),
			spacetest.WithSpecTargetCluster("member-1"),
			spacetest.WithStatusTargetCluster("member-1"), // already provisioned on a target cluster
			spacetest.WithFinalizer())

		spacebinding := spacebindingtest.NewSpaceBinding("john-doe", "john-doe", "admin", userSignup.Name)

		config := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true), testconfig.Metrics().ForceSynchronization(false))
		r, req, _ := prepareReconcile(t, userSignup.Name, config, userSignup, mur, space, spacebinding, baseNSTemplateTier)
		InitializeCountersWithToolchainConfig(t, config, NewToolchainStatus(
			WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
				string(metrics.External): 1,
			}),
			WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
				"1,external": 1,
			}),
		))

		// when
		_, err := r.Reconcile(context.TODO(), req)

		// then
		require.NoError(t, err)
		err = r.Client.Get(context.TODO(), key, userSignup)
		require.NoError(t, err)
		// The state label should still be set to approved until the controller reconciles the deactivation
		assert.Equal(t, "approved", userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey])
		metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupDeactivatedTotal) // 0 because usersignup has not reconciled the deactivation
		metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)    // 0 because usersignup was originally deactivated
		metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupUniqueTotal)      // 0 because state was initially set to approved

		// Confirm the status is now set to Deactivating
		test.AssertConditionsMatch(t, userSignup.Status.Conditions,
			toolchainv1alpha1.Condition{
				Type:   toolchainv1alpha1.UserSignupApproved,
				Status: corev1.ConditionTrue,
				Reason: "ApprovedAutomatically",
			},
			toolchainv1alpha1.Condition{
				Type:   toolchainv1alpha1.UserSignupComplete,
				Status: corev1.ConditionFalse,
				Reason: "DeactivationInProgress",
			})

		// The Space and SpaceBinding should still exist because cleanup would be handled by the space cleanup controller
		spacetest.AssertThatSpace(t, test.HostOperatorNs, "john-doe", r.Client).Exists()
		spacebindingtest.AssertThatSpaceBinding(t, test.HostOperatorNs, "john-doe", "john-doe", r.Client).Exists()

		// The MUR should have now been deleted
		murtest.AssertThatMasterUserRecords(t, r.Client).HaveCount(0)

		AssertThatCountersAndMetrics(t).
			HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
				string(metrics.External): 1, // unchanged for now (see above)
			}).
			HaveUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
				"1,external": 1,
			})

		// There should not be a notification created yet, only the next reconcile (with deleted mur) would create the notification
		ntest.AssertNoNotificationsExist(t, r.Client)
	})

	t.Run("when MUR doesn't exist, then the condition should be set to Deactivated", func(t *testing.T) {
		// given
		config := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true), testconfig.Metrics().ForceSynchronization(false))
		r, req, _ := prepareReconcile(t, userSignup.Name, config, userSignup, baseNSTemplateTier)
		InitializeCountersWithToolchainConfig(t, config, NewToolchainStatus(
			WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
				string(metrics.External): 2,
			}),
			WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
				"1,external": 2,
			}),
		))

		// when
		_, err := r.Reconcile(context.TODO(), req)

		// then
		require.NoError(t, err)

		// Lookup the UserSignup
		err = r.Client.Get(context.TODO(), key, userSignup)
		require.NoError(t, err)
		assert.Equal(t, "deactivated", userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey])
		metricstest.AssertMetricsCounterEquals(t, 1, metrics.UserSignupDeactivatedTotal) // one because the deactivation was reconciled
		metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
		metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupUniqueTotal)

		// Confirm the status has been set to Deactivated
		test.AssertConditionsMatch(t, userSignup.Status.Conditions,
			toolchainv1alpha1.Condition{
				Type:   toolchainv1alpha1.UserSignupApproved,
				Status: corev1.ConditionFalse,
				Reason: "Deactivated",
			},
			toolchainv1alpha1.Condition{
				Type:   toolchainv1alpha1.UserSignupComplete,
				Status: corev1.ConditionTrue,
				Reason: "Deactivated",
			},
			toolchainv1alpha1.Condition{
				Type:   toolchainv1alpha1.UserSignupUserDeactivatedNotificationCreated,
				Status: corev1.ConditionTrue,
				Reason: "NotificationCRCreated",
			})
		AssertThatCountersAndMetrics(t).
			HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
				string(metrics.External): 2, // unchanged
			}).
			HaveUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
				"1,external": 2,
			})

		// Confirm that the scheduled deactivation time has been set to nil
		require.Nil(t, userSignup.Status.ScheduledDeactivationTimestamp)

		// A deactivated notification should have been created
		notifications := &toolchainv1alpha1.NotificationList{}
		err = r.Client.List(context.TODO(), notifications)
		require.NoError(t, err)
		require.Len(t, notifications.Items, 1)
		notification := notifications.Items[0]
		require.Contains(t, notification.Name, "john-doe-deactivated-")
		assert.Greater(t, len(notification.Name), len("john-doe-deactivated-"))
		require.Equal(t, userSignup.Spec.IdentityClaims.Sub, notification.Spec.Context["Sub"])
		require.Equal(t, "https://registration.crt-placeholder.com", notification.Spec.Context["RegistrationURL"])
		assert.Equal(t, "userdeactivated", notification.Spec.Template)
	})

	t.Run("second deactivated notification not created when it already exists", func(t *testing.T) {
		// given
		userSignup2 := commonsignup.NewUserSignup(commonsignup.Deactivated())
		userSignup2.Status = toolchainv1alpha1.UserSignupStatus{
			Conditions: []toolchainv1alpha1.Condition{
				{
					Type:   toolchainv1alpha1.UserSignupComplete,
					Status: corev1.ConditionTrue,
				},
				{
					Type:   toolchainv1alpha1.UserSignupApproved,
					Status: corev1.ConditionTrue,
					Reason: "ApprovedAutomatically",
				},
			},
			CompliantUsername: "john-smythe",
		}
		key2 := test.NamespacedName(test.HostOperatorNs, userSignup2.Name)

		existingNotification := &toolchainv1alpha1.Notification{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: test.HostOperatorNs,
				Name:      "john-smythe-deactivated-123",
				Labels: map[string]string{
					toolchainv1alpha1.NotificationUserNameLabelKey: userSignup2.Status.CompliantUsername,
					toolchainv1alpha1.NotificationTypeLabelKey:     toolchainv1alpha1.NotificationTypeDeactivated,
				},
			},
		}

		config := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true), testconfig.Metrics().ForceSynchronization(false))
		r, req, _ := prepareReconcile(t, userSignup2.Name, config, userSignup2, baseNSTemplateTier, existingNotification)

		// when
		_, err := r.Reconcile(context.TODO(), req)

		// then
		require.NoError(t, err)

		// Lookup the UserSignup
		err = r.Client.Get(context.TODO(), key2, userSignup2)
		require.NoError(t, err)
		assert.Equal(t, "deactivated", userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey])

		// Confirm the status has been set to Deactivated
		test.AssertConditionsMatch(t, userSignup.Status.Conditions,
			toolchainv1alpha1.Condition{
				Type:   toolchainv1alpha1.UserSignupApproved,
				Status: corev1.ConditionFalse,
				Reason: "Deactivated",
			},
			toolchainv1alpha1.Condition{
				Type:   toolchainv1alpha1.UserSignupComplete,
				Status: corev1.ConditionTrue,
				Reason: "Deactivated",
			},
			toolchainv1alpha1.Condition{
				Type:   toolchainv1alpha1.UserSignupUserDeactivatedNotificationCreated,
				Status: corev1.ConditionTrue,
				Reason: "NotificationCRCreated",
			})

		// A deactivated notification should have been created
		notifications := &toolchainv1alpha1.NotificationList{}
		err = r.Client.List(context.TODO(), notifications)
		require.NoError(t, err)
		require.Len(t, notifications.Items, 1)
		notification := notifications.Items[0]
		require.Equal(t, "john-smythe-deactivated-123", notification.Name)
	})
}

func TestUserSignupFailedToCreateDeactivationNotification(t *testing.T) {
	// given
	meta := commonsignup.NewUserSignupObjectMeta("", "john.doe@redhat.com")
	userSignup := &toolchainv1alpha1.UserSignup{
		ObjectMeta: meta,
		Spec: toolchainv1alpha1.UserSignupSpec{
			IdentityClaims: toolchainv1alpha1.IdentityClaimsEmbedded{
				PropagatedClaims: toolchainv1alpha1.PropagatedClaims{
					Sub:   "UserID123",
					Email: "john.doe@redhat.com",
				},
				PreferredUsername: meta.Name,
			},
			States: []toolchainv1alpha1.UserSignupState{toolchainv1alpha1.UserSignupStateDeactivated},
		},
		Status: toolchainv1alpha1.UserSignupStatus{
			Conditions: []toolchainv1alpha1.Condition{
				{
					Type:   toolchainv1alpha1.UserSignupComplete,
					Status: corev1.ConditionTrue,
				},
				{
					Type:   toolchainv1alpha1.UserSignupApproved,
					Status: corev1.ConditionTrue,
					Reason: "ApprovedAutomatically",
				},
			},
			CompliantUsername: "john-doe",
		},
	}
	userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey] = toolchainv1alpha1.UserSignupStateLabelValueApproved
	userSignup.Labels["toolchain.dev.openshift.com/approved"] = "true"
	// NotificationUserNameLabelKey is only used for easy lookup for debugging and e2e tests
	userSignup.Labels[toolchainv1alpha1.NotificationUserNameLabelKey] = "john-doe"
	// NotificationTypeLabelKey is only used for easy lookup for debugging and e2e tests
	userSignup.Labels[toolchainv1alpha1.NotificationTypeLabelKey] = toolchainv1alpha1.NotificationTypeDeactivated
	key := test.NamespacedName(test.HostOperatorNs, userSignup.Name)

	t.Run("when the deactivation notification cannot be created", func(t *testing.T) {
		// given
		config := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true), testconfig.Metrics().ForceSynchronization(false))
		r, req, fakeClient := prepareReconcile(t, userSignup.Name, config, userSignup, baseNSTemplateTier)
		InitializeCountersWithToolchainConfig(t, config, NewToolchainStatus(
			WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
				string(metrics.External): 2,
			}),
			WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
				"1,external": 2,
			}),
		))

		fakeClient.MockCreate = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.CreateOption) error {
			switch obj.(type) {
			case *toolchainv1alpha1.Notification:
				return errors.New("unable to create deactivation notification")
			default:
				return test.Create(ctx, fakeClient, obj, opts...)
			}
		}

		// when
		_, err := r.Reconcile(context.TODO(), req)

		// then
		require.Error(t, err)
		require.Equal(t, "Failed to create user deactivation notification: unable to create deactivation notification", err.Error())

		// Lookup the UserSignup
		err = r.Client.Get(context.TODO(), key, userSignup)
		require.NoError(t, err)

		// Confirm the status shows the deactivation notification failure
		test.AssertConditionsMatch(t, userSignup.Status.Conditions,
			toolchainv1alpha1.Condition{
				Type:   toolchainv1alpha1.UserSignupApproved,
				Status: corev1.ConditionTrue,
				Reason: "ApprovedAutomatically",
			},
			toolchainv1alpha1.Condition{
				Type:   toolchainv1alpha1.UserSignupComplete,
				Status: corev1.ConditionTrue,
			},
			toolchainv1alpha1.Condition{
				Type:    toolchainv1alpha1.UserSignupUserDeactivatedNotificationCreated,
				Status:  corev1.ConditionFalse,
				Reason:  "NotificationCRCreationFailed",
				Message: "unable to create deactivation notification",
			})
		AssertThatCountersAndMetrics(t).
			HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
				string(metrics.External): 2, // unchanged
			}).
			HaveUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
				"1,external": 2,
			})
		assert.Equal(t, "approved", userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey])
		metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupDeactivatedTotal)
		metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
		metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupUniqueTotal)

		// A deactivated notification should not have been created
		notificationList := &toolchainv1alpha1.NotificationList{}
		err = r.Client.List(context.TODO(), notificationList)
		require.NoError(t, err)
		require.Empty(t, notificationList.Items)
	})
}

func TestUserSignupReactivateAfterDeactivated(t *testing.T) {
	// given
	meta := commonsignup.NewUserSignupObjectMeta("", "john.doe@redhat.com")
	userSignup := &toolchainv1alpha1.UserSignup{
		ObjectMeta: meta,
		Spec: toolchainv1alpha1.UserSignupSpec{
			IdentityClaims: toolchainv1alpha1.IdentityClaimsEmbedded{
				PropagatedClaims: toolchainv1alpha1.PropagatedClaims{
					Email:  "john.doe@redhat.com",
					UserID: "UserID123",
					Sub:    "UserID123",
				},
				PreferredUsername: meta.Name,
			},
		},
		Status: toolchainv1alpha1.UserSignupStatus{
			CompliantUsername: "john-doe",
		},
	}
	key := test.NamespacedName(test.HostOperatorNs, userSignup.Name)

	t.Run("when reactivating the usersignup successfully", func(t *testing.T) {
		// given
		// start with a usersignup that has the Notification Created status set to "true" but Spec.Deactivated is set to "false" which signals a user which has been just reactivated.
		userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey] = "deactivated"
		userSignup.Labels["toolchain.dev.openshift.com/approved"] = "true"
		userSignup.Annotations[toolchainv1alpha1.UserSignupActivationCounterAnnotationKey] = "2" // the user signed up twice
		userSignup.Status.Conditions = []toolchainv1alpha1.Condition{
			{
				Type:   toolchainv1alpha1.UserSignupComplete,
				Status: corev1.ConditionTrue,
				Reason: "Deactivated",
			},
			{
				Type:   toolchainv1alpha1.UserSignupApproved,
				Status: corev1.ConditionFalse,
				Reason: "Deactivated",
			},
			{
				Type:   toolchainv1alpha1.UserSignupUserDeactivatedNotificationCreated,
				Status: corev1.ConditionTrue,
				Reason: "NotificationCRCreated",
			},
		}
		spc1 := hspc.NewEnabledValidTenantSPC("member1")
		config := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true), testconfig.Metrics().ForceSynchronization(false))
		r, req, _ := prepareReconcile(t, userSignup.Name, config, spc1, userSignup, baseNSTemplateTier, deactivate30Tier)
		InitializeCountersWithToolchainConfig(t, config, NewToolchainStatus(
			WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
				"2,internal": 11, // 11 users signed-up 2 times, including our user above, even though she is not active at the moment
				"3,internal": 10, // 10 users signed-up 3 times
			}),
			WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
				string(metrics.Internal): 21,
			}),
		))

		// when
		_, err := r.Reconcile(context.TODO(), req)

		// then
		require.NoError(t, err)

		// Lookup the UserSignup
		err = r.Client.Get(context.TODO(), key, userSignup)
		require.NoError(t, err)

		// Confirm the status shows the notification created condition is reset to active
		test.AssertConditionsMatch(t, userSignup.Status.Conditions,
			toolchainv1alpha1.Condition{
				Type:   toolchainv1alpha1.UserSignupApproved,
				Status: corev1.ConditionTrue,
				Reason: "ApprovedAutomatically",
			},
			toolchainv1alpha1.Condition{
				Type:   toolchainv1alpha1.UserSignupComplete,
				Status: corev1.ConditionTrue,
				Reason: "Deactivated",
			},
			toolchainv1alpha1.Condition{
				Type:   toolchainv1alpha1.UserSignupUserDeactivatingNotificationCreated,
				Status: corev1.ConditionFalse,
				Reason: "UserNotInPreDeactivation",
			},
			toolchainv1alpha1.Condition{
				Type:   toolchainv1alpha1.UserSignupUserDeactivatedNotificationCreated,
				Status: corev1.ConditionFalse,
				Reason: "UserIsActive",
			})

		// A mur should be created so the counter should be 21
		AssertThatCountersAndMetrics(t).
			HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
				string(metrics.Internal): 22, // one more than before
			}).
			HaveUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
				"2,internal": 10,
				"3,internal": 11,
			})

		assert.Equal(t, "approved", userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey])
		// verify that the annotation was incremented
		assert.Equal(t, "3", userSignup.Annotations[toolchainv1alpha1.UserSignupActivationCounterAnnotationKey])
		metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupDeactivatedTotal)
		metricstest.AssertMetricsCounterEquals(t, 1, metrics.UserSignupApprovedTotal)
		metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupUniqueTotal)
		mur := murtest.AssertThatMasterUserRecord(t, userSignup.Name, r.Client).Get()
		segmenttest.AssertMessageQueuedForProvisionedMur(t, r.SegmentClient, userSignup, mur.Name)

		// There should not be a notification created because the user was reactivated
		ntest.AssertNoNotificationsExist(t, r.Client)
	})

	t.Run("when resetting the usersignup deactivation notification status fails", func(t *testing.T) {
		// given
		// start with a usersignup that has the Notification Created status set to "true" but Spec.Deactivated is set to "false" which signals a user which has been just reactivated.
		userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey] = "deactivated"
		userSignup.Labels["toolchain.dev.openshift.com/approved"] = "true"
		userSignup.Status.Conditions = []toolchainv1alpha1.Condition{
			{
				Type:   toolchainv1alpha1.UserSignupComplete,
				Status: corev1.ConditionTrue,
				Reason: "Deactivated",
			},
			{
				Type:   toolchainv1alpha1.UserSignupApproved,
				Status: corev1.ConditionTrue,
				Reason: "ApprovedAutomatically",
			},
			{
				Type:   toolchainv1alpha1.UserSignupUserDeactivatedNotificationCreated,
				Status: corev1.ConditionTrue,
				Reason: "NotificationCRCreated",
			},
		}
		config := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true), testconfig.Metrics().ForceSynchronization(false))
		r, req, fakeClient := prepareReconcile(t, userSignup.Name, config, userSignup, baseNSTemplateTier)
		InitializeCountersWithToolchainConfig(t, config, NewToolchainStatus(
			WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
				string(metrics.External): 2,
			}),
			WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
				"1,external": 2,
			}),
		))

		fakeClient.MockStatusUpdate = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.SubResourceUpdateOption) error {
			switch obj.(type) {
			case *toolchainv1alpha1.UserSignup:
				return errors.New("failed to update UserSignup status")
			default:
				return fakeClient.Client.Update(ctx, obj)
			}
		}

		// when
		_, err := r.Reconcile(context.TODO(), req)

		// then
		require.Error(t, err)
		require.Equal(t, "failed to update UserSignup status", err.Error())

		// Lookup the UserSignup
		err = r.Client.Get(context.TODO(), key, userSignup)
		require.NoError(t, err)

		// Confirm the status shows the notification is unchanged because the status update failed
		test.AssertConditionsMatch(t, userSignup.Status.Conditions,
			toolchainv1alpha1.Condition{
				Type:   toolchainv1alpha1.UserSignupApproved,
				Status: corev1.ConditionTrue,
				Reason: "ApprovedAutomatically",
			},
			toolchainv1alpha1.Condition{
				Type:   toolchainv1alpha1.UserSignupComplete,
				Status: corev1.ConditionTrue,
				Reason: "Deactivated",
			},
			toolchainv1alpha1.Condition{
				Type:   toolchainv1alpha1.UserSignupUserDeactivatedNotificationCreated,
				Status: corev1.ConditionTrue,
				Reason: "NotificationCRCreated",
			})
		AssertThatCountersAndMetrics(t).
			HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
				string(metrics.External): 2, // unchanged
			}).
			HaveUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
				"1,external": 2,
			})

		// State is still deactivated because the status update failed
		assert.Equal(t, "deactivated", userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey])
		metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupDeactivatedTotal)
		metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
		metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupUniqueTotal)

		// A deactivation notification should not be created because this is the reactivation case
		ntest.AssertNoNotificationsExist(t, r.Client)
	})
}

func TestUserSignupDeactivatedWhenMURAndSpaceAndSpaceBindingExists(t *testing.T) {
	// given
	userSignup := &toolchainv1alpha1.UserSignup{
		ObjectMeta: commonsignup.NewUserSignupObjectMeta("", "edward.jones@redhat.com"),
		Spec: toolchainv1alpha1.UserSignupSpec{
			IdentityClaims: toolchainv1alpha1.IdentityClaimsEmbedded{
				PropagatedClaims: toolchainv1alpha1.PropagatedClaims{
					Sub:   "UserID123",
					Email: "edward.jones@redhat.com",
				},
				PreferredUsername: "edward.jones@redhat.com",
			},
			States: []toolchainv1alpha1.UserSignupState{toolchainv1alpha1.UserSignupStateApproved},
		},
		Status: toolchainv1alpha1.UserSignupStatus{
			Conditions: []toolchainv1alpha1.Condition{
				{
					Type:   toolchainv1alpha1.UserSignupComplete,
					Status: corev1.ConditionTrue,
					Reason: "",
				},
				{
					Type:   toolchainv1alpha1.UserSignupApproved,
					Status: corev1.ConditionTrue,
					Reason: "ApprovedAutomatically",
				},
			},
			CompliantUsername: "edward-jones",
		},
	}
	userSignup.Labels["toolchain.dev.openshift.com/approved"] = "true"
	userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey] = "approved"

	mur := murtest.NewMasterUserRecord(t, "edward-jones", murtest.MetaNamespace(test.HostOperatorNs))
	mur.Labels = map[string]string{
		toolchainv1alpha1.MasterUserRecordOwnerLabelKey: userSignup.Name,
		toolchainv1alpha1.UserSignupStateLabelKey:       "approved",
	}

	space := spacetest.NewSpace(test.HostOperatorNs, "edward-jones",
		spacetest.WithSpecTargetCluster("member-1"),
		spacetest.WithStatusTargetCluster("member-1"), // already provisioned on a target cluster
		spacetest.WithFinalizer())

	spacebinding := spacebindingtest.NewSpaceBinding("edward-jones", "edward-jones", "admin", userSignup.Name)

	key := test.NamespacedName(test.HostOperatorNs, userSignup.Name)

	t.Run("when MUR exists and not deactivated, nothing should happen", func(t *testing.T) {
		config := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true), testconfig.Metrics().ForceSynchronization(false))
		r, req, _ := prepareReconcile(t, userSignup.Name, config, userSignup, mur, space, spacebinding, baseNSTemplateTier, deactivate30Tier)
		// given space & mur are ready
		setSpaceToReady(t, r.Client, mur.Name)
		setMURToReady(t, r.Client, mur.Name)

		_, err := r.Reconcile(context.TODO(), req)

		// then
		require.NoError(t, err)
		err = r.Client.Get(context.TODO(), key, userSignup)
		require.NoError(t, err)
		assert.Equal(t, "approved", userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey])

		// Confirm the status is still set correctly
		test.AssertConditionsMatch(t, userSignup.Status.Conditions,
			toolchainv1alpha1.Condition{
				Type:   toolchainv1alpha1.UserSignupApproved,
				Status: corev1.ConditionTrue,
				Reason: "ApprovedAutomatically",
			},
			toolchainv1alpha1.Condition{
				Type:   toolchainv1alpha1.UserSignupComplete,
				Status: corev1.ConditionTrue,
				Reason: "",
			},
			toolchainv1alpha1.Condition{
				Type:   toolchainv1alpha1.UserSignupUserDeactivatedNotificationCreated,
				Status: corev1.ConditionFalse,
				Reason: "UserIsActive",
			},
			toolchainv1alpha1.Condition{
				Type:   toolchainv1alpha1.UserSignupUserDeactivatingNotificationCreated,
				Status: corev1.ConditionFalse,
				Reason: "UserNotInPreDeactivation",
			})

		// The MUR should have not been deleted
		murtest.AssertThatMasterUserRecords(t, r.Client).HaveCount(1)
		murtest.AssertThatMasterUserRecord(t, "edward-jones", r.Client).Exists()

		// The Space and SpaceBinding should not have been deleted
		spacetest.AssertThatSpace(t, test.HostOperatorNs, "edward-jones", r.Client).Exists()
		spacebindingtest.AssertThatSpaceBinding(t, test.HostOperatorNs, "edward-jones", "edward-jones", r.Client).Exists()
	})

	t.Run("when UserSignup deactivated and MUR and Space and SpaceBinding exists, then they should be deleted", func(t *testing.T) {
		// Given
		states.SetDeactivated(userSignup, true)

		config := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true), testconfig.Metrics().ForceSynchronization(false))
		r, req, _ := prepareReconcile(t, userSignup.Name, config, userSignup, mur, space, spacebinding, baseNSTemplateTier)
		InitializeCountersWithToolchainConfig(t, config, NewToolchainStatus(
			WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
				string(metrics.External): 1,
			}),
			WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
				"1,external": 1,
			}),
		))

		t.Run("first reconcile - status should be deactivating and mur should be deleted", func(t *testing.T) {
			// when
			_, err := r.Reconcile(context.TODO(), req)

			// then
			require.NoError(t, err)
			err = r.Client.Get(context.TODO(), key, userSignup)
			require.NoError(t, err)
			assert.Equal(t, "approved", userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey]) // State should still be approved at this stage
			metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupDeactivatedTotal)
			metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
			metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupUniqueTotal)

			// Confirm the status is still set to Deactivating
			test.AssertConditionsMatch(t, userSignup.Status.Conditions,
				toolchainv1alpha1.Condition{
					Type:   toolchainv1alpha1.UserSignupApproved,
					Status: corev1.ConditionTrue,
					Reason: "ApprovedAutomatically",
				},
				toolchainv1alpha1.Condition{
					Type:   toolchainv1alpha1.UserSignupComplete,
					Status: corev1.ConditionFalse,
					Reason: "DeactivationInProgress",
				},
				toolchainv1alpha1.Condition{
					Type:   toolchainv1alpha1.UserSignupUserDeactivatingNotificationCreated,
					Status: corev1.ConditionFalse,
					Reason: "UserNotInPreDeactivation",
				},
				toolchainv1alpha1.Condition{
					Type:   toolchainv1alpha1.UserSignupUserDeactivatedNotificationCreated,
					Status: corev1.ConditionFalse,
					Reason: "UserIsActive",
				})

			// The MUR should have now been deleted
			murtest.AssertThatMasterUserRecords(t, r.Client).HaveCount(0)

			// The Space and SpaceBinding should still exist because cleanup would be handled by the space cleanup controller
			spacetest.AssertThatSpace(t, test.HostOperatorNs, space.Name, r.Client).Exists()
			spacebindingtest.AssertThatSpaceBinding(t, test.HostOperatorNs, "edward-jones", "edward-jones", r.Client).Exists()

			AssertThatCountersAndMetrics(t).
				HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
					string(metrics.External): 1,
				}).
				HaveUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
					"1,external": 1,
				})

			// There should not be a notification created yet, only the next reconcile (with deleted mur) would create the notification
			ntest.AssertNoNotificationsExist(t, r.Client)
		})

		t.Run("second reconcile - condition should be deactivated and deactivation notification created", func(t *testing.T) {
			res, err := r.Reconcile(context.TODO(), req)
			require.NoError(t, err)
			require.Equal(t, reconcile.Result{}, res)

			// lookup the userSignup and check the conditions
			err = r.Client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
			require.NoError(t, err)

			// Confirm the status has been set to Deactivated and the deactivation notification is created
			test.AssertConditionsMatch(t, userSignup.Status.Conditions,
				toolchainv1alpha1.Condition{
					Type:   toolchainv1alpha1.UserSignupApproved,
					Status: corev1.ConditionFalse,
					Reason: "Deactivated",
				},
				toolchainv1alpha1.Condition{
					Type:   toolchainv1alpha1.UserSignupComplete,
					Status: corev1.ConditionTrue,
					Reason: "Deactivated",
				},
				toolchainv1alpha1.Condition{
					Type:   toolchainv1alpha1.UserSignupUserDeactivatedNotificationCreated,
					Status: corev1.ConditionTrue,
					Reason: "NotificationCRCreated",
				},
				toolchainv1alpha1.Condition{
					Type:   toolchainv1alpha1.UserSignupUserDeactivatingNotificationCreated,
					Status: corev1.ConditionFalse,
					Reason: "UserNotInPreDeactivation",
				})
			// metrics should be the same after the 2nd reconcile
			metricstest.AssertMetricsCounterEquals(t, 1, metrics.UserSignupDeactivatedTotal)
			metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
			metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupUniqueTotal)

			// The Space and SpaceBinding should still exist because cleanup would be handled by the space cleanup controller
			spacetest.AssertThatSpace(t, test.HostOperatorNs, space.Name, r.Client).Exists()
			spacebindingtest.AssertThatSpaceBinding(t, test.HostOperatorNs, "edward-jones", "edward-jones", r.Client).Exists()
		})
	})
}

func TestUserSignupDeactivatingNotificationCreated(t *testing.T) {
	userSignup := &toolchainv1alpha1.UserSignup{
		ObjectMeta: commonsignup.NewUserSignupObjectMeta("", "edward.jones@redhat.com"),
		Spec: toolchainv1alpha1.UserSignupSpec{
			IdentityClaims: toolchainv1alpha1.IdentityClaimsEmbedded{
				PropagatedClaims: toolchainv1alpha1.PropagatedClaims{
					Sub:   "UserID089",
					Email: "edward.jones@redhat.com",
				},
				PreferredUsername: "edward.jones@redhat.com",
			},
			States: []toolchainv1alpha1.UserSignupState{"deactivating"},
		},
		Status: toolchainv1alpha1.UserSignupStatus{
			Conditions: []toolchainv1alpha1.Condition{
				{
					Type:   toolchainv1alpha1.UserSignupComplete,
					Status: corev1.ConditionTrue,
				},
				{
					Type:   toolchainv1alpha1.UserSignupApproved,
					Status: corev1.ConditionTrue,
					Reason: "ApprovedAutomatically",
				},
			},
			CompliantUsername: "edward-jones",
		},
	}

	// given
	mur := murtest.NewMasterUserRecord(t, "edward-jones", murtest.MetaNamespace(test.HostOperatorNs))
	mur.Labels = map[string]string{
		toolchainv1alpha1.MasterUserRecordOwnerLabelKey: userSignup.Name,
		toolchainv1alpha1.UserSignupStateLabelKey:       "approved",
	}

	userSignup.Labels["toolchain.dev.openshift.com/approved"] = "true"
	userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey] = "approved"
	key := test.NamespacedName(test.HostOperatorNs, userSignup.Name)

	config := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true), testconfig.Metrics().ForceSynchronization(false))
	r, req, _ := prepareReconcile(t, userSignup.Name, config, userSignup, mur, baseNSTemplateTier, deactivate30Tier)

	// when
	_, err := r.Reconcile(context.TODO(), req)

	// then
	require.NoError(t, err)
	err = r.Client.Get(context.TODO(), key, userSignup)
	require.NoError(t, err)
	assert.Equal(t, "approved", userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey])

	notifications := &toolchainv1alpha1.NotificationList{}
	err = r.Client.List(context.TODO(), notifications)
	require.NoError(t, err)

	require.Len(t, notifications.Items, 1)

	require.Equal(t, "userdeactivating", notifications.Items[0].Spec.Template)
	require.Equal(t, userSignup.Spec.IdentityClaims.Sub, notifications.Items[0].Spec.Context["Sub"])

	// Confirm the status is correct
	test.AssertConditionsMatch(t, userSignup.Status.Conditions,
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupComplete,
			Status: corev1.ConditionTrue,
		},
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupApproved,
			Status: corev1.ConditionTrue,
			Reason: "ApprovedAutomatically",
		},
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupUserDeactivatedNotificationCreated,
			Status: corev1.ConditionFalse,
			Reason: "UserIsActive",
		},
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupUserDeactivatingNotificationCreated,
			Status: corev1.ConditionTrue,
			Reason: "NotificationCRCreated",
		},
	)

	// Now let's pretend that the notification was created but the status failed to update.  We'll do this by modifying
	// the status back to original values
	userSignup.Status.Conditions = []toolchainv1alpha1.Condition{
		{
			Type:   toolchainv1alpha1.UserSignupComplete,
			Status: corev1.ConditionTrue,
		},
		{
			Type:   toolchainv1alpha1.UserSignupApproved,
			Status: corev1.ConditionTrue,
			Reason: "ApprovedAutomatically",
		},
	}

	// Prepare the reconciliation again, but this time include the notification that was previously created
	r, req, _ = prepareReconcile(t, userSignup.Name, config, userSignup, mur, &notifications.Items[0], baseNSTemplateTier, deactivate30Tier)

	// Reconcile again
	_, err = r.Reconcile(context.TODO(), req)

	// then
	require.NoError(t, err)
	err = r.Client.Get(context.TODO(), key, userSignup)
	require.NoError(t, err)
	assert.Equal(t, "approved", userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey])

	// Reload the notifications again
	notifications = &toolchainv1alpha1.NotificationList{}
	err = r.Client.List(context.TODO(), notifications)
	require.NoError(t, err)

	// There should still only be one notification
	require.Len(t, notifications.Items, 1)

	require.Equal(t, "userdeactivating", notifications.Items[0].Spec.Template)
	require.Equal(t, userSignup.Spec.IdentityClaims.Sub, notifications.Items[0].Spec.Context["Sub"])

	// Confirm the status is still correct
	test.AssertConditionsMatch(t, userSignup.Status.Conditions,
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupComplete,
			Status: corev1.ConditionTrue,
		},
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupApproved,
			Status: corev1.ConditionTrue,
			Reason: "ApprovedAutomatically",
		},
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupUserDeactivatedNotificationCreated,
			Status: corev1.ConditionFalse,
			Reason: "UserIsActive",
		},
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupUserDeactivatingNotificationCreated,
			Status: corev1.ConditionTrue,
			Reason: "NotificationCRCreated",
		},
	)
}

func TestUserSignupBannedWithoutMURAndSpace(t *testing.T) {
	// given
	userSignup := commonsignup.NewUserSignup()
	userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey] = "approved"

	bannedUser := &toolchainv1alpha1.BannedUser{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				toolchainv1alpha1.BannedUserEmailHashLabelKey: "fd2addbd8d82f0d2dc088fa122377eaa",
				toolchainv1alpha1.UserSignupStateLabelKey:     "approved",
			},
		},
		Spec: toolchainv1alpha1.BannedUserSpec{
			Email: "foo@redhat.com",
		},
	}

	config := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true), testconfig.Metrics().ForceSynchronization(false))
	r, req, _ := prepareReconcile(t, userSignup.Name, config, userSignup, bannedUser, baseNSTemplateTier)
	InitializeCountersWithToolchainConfig(t, config, NewToolchainStatus(
		WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
			string(metrics.External): 1,
		}),
		WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
			"1,external": 1,
		}),
	))

	// when
	_, err := r.Reconcile(context.TODO(), req)

	// then
	require.NoError(t, err)
	err = r.Client.Get(context.TODO(), test.NamespacedName(test.HostOperatorNs, userSignup.Name), userSignup)
	require.NoError(t, err)
	assert.Equal(t, "banned", userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey])
	metricstest.AssertMetricsCounterEquals(t, 1, metrics.UserSignupBannedTotal)
	metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupDeactivatedTotal)
	metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
	metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupUniqueTotal)

	// Confirm the status is set to Banned
	test.AssertConditionsMatch(t, userSignup.Status.Conditions,
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupComplete,
			Status: corev1.ConditionTrue,
			Reason: "Banned",
		})

	// Confirm that the MUR has now been deleted
	murtest.AssertThatMasterUserRecords(t, r.Client).HaveCount(0)
	// Confirm that the Space has now been deleted
	spacetest.AssertThatSpaces(t, r.Client).HaveCount(0)
	// Confirm that the SpaceBinding has now been deleted
	spacebindingtest.AssertThatSpaceBindings(t, r.Client).HaveCount(0)

	AssertThatCountersAndMetrics(t).
		HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
			string(metrics.External): 1,
		}).
		HaveUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
			"1,external": 1,
		})
}

func TestUserSignupVerificationRequired(t *testing.T) {
	// given
	userSignup := commonsignup.NewUserSignup(commonsignup.VerificationRequired())

	config := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true), testconfig.Metrics().ForceSynchronization(false))
	r, req, _ := prepareReconcile(t, userSignup.Name, config, userSignup, baseNSTemplateTier)
	InitializeCountersWithToolchainConfig(t, config, NewToolchainStatus(
		WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
			string(metrics.External): 1,
		}),
		WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
			"1,external": 1,
		}),
	))

	// when
	_, err := r.Reconcile(context.TODO(), req)

	// then
	require.NoError(t, err)
	err = r.Client.Get(context.TODO(), test.NamespacedName(test.HostOperatorNs, userSignup.Name), userSignup)
	require.NoError(t, err)
	assert.Equal(t, toolchainv1alpha1.UserSignupStateLabelValueNotReady, userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey])
	metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupBannedTotal)
	metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupDeactivatedTotal)
	metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
	metricstest.AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)

	// Confirm the status is set to VerificationRequired
	test.AssertConditionsMatch(t, userSignup.Status.Conditions,
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupComplete,
			Status: corev1.ConditionFalse,
			Reason: "VerificationRequired",
		},
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupUserDeactivatingNotificationCreated,
			Status: corev1.ConditionFalse,
			Reason: "UserNotInPreDeactivation",
		},
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupUserDeactivatedNotificationCreated,
			Status: corev1.ConditionFalse,
			Reason: "UserIsActive",
		})

	// Confirm that no MUR is created
	murtest.AssertThatMasterUserRecords(t, r.Client).HaveCount(0)
	spacetest.AssertThatSpaces(t, r.Client).HaveCount(0)
	spacebindingtest.AssertThatSpaceBindings(t, r.Client).HaveCount(0)
	AssertThatCountersAndMetrics(t).
		HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
			string(metrics.External): 1,
		}).
		HaveUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
			"1,external": 1,
		})
}

func TestUserSignupBannedMURAndSpaceExists(t *testing.T) {
	// given
	userSignup := commonsignup.NewUserSignup()
	userSignup.Status = toolchainv1alpha1.UserSignupStatus{
		Conditions: []toolchainv1alpha1.Condition{
			{
				Type:   toolchainv1alpha1.UserSignupComplete,
				Status: corev1.ConditionTrue,
			},
			{
				Type:   toolchainv1alpha1.UserSignupApproved,
				Status: corev1.ConditionTrue,
				Reason: "ApprovedAutomatically",
			},
		},
		CompliantUsername: "foo",
	}
	userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey] = "approved"
	userSignup.Labels["toolchain.dev.openshift.com/approved"] = "true"

	bannedUser := &toolchainv1alpha1.BannedUser{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				toolchainv1alpha1.BannedUserEmailHashLabelKey: "fd2addbd8d82f0d2dc088fa122377eaa",
			},
		},
		Spec: toolchainv1alpha1.BannedUserSpec{
			Email: "foo@redhat.com",
		},
	}

	mur := murtest.NewMasterUserRecord(t, "foo", murtest.MetaNamespace(test.HostOperatorNs))
	mur.Labels = map[string]string{toolchainv1alpha1.MasterUserRecordOwnerLabelKey: userSignup.Name}

	space := spacetest.NewSpace(test.HostOperatorNs, mur.Name,
		spacetest.WithCreatorLabel(userSignup.Name),
		spacetest.WithSpecTargetCluster("member-1"),
		spacetest.WithStatusTargetCluster("member-1"), // already provisioned on a target cluster
		spacetest.WithFinalizer())

	spacebinding := spacebindingtest.NewSpaceBinding("foo", "foo", "admin", userSignup.Name)

	config := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true), testconfig.Metrics().ForceSynchronization(false))
	r, req, _ := prepareReconcile(t, userSignup.Name, config, userSignup, mur, space, spacebinding, bannedUser, baseNSTemplateTier)
	InitializeCountersWithToolchainConfig(t, config, NewToolchainStatus(
		WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
			string(metrics.External): 1,
		}),
		WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
			"1,external": 1,
		}),
	))

	// when
	_, err := r.Reconcile(context.TODO(), req)

	// then
	require.NoError(t, err)
	key := test.NamespacedName(test.HostOperatorNs, userSignup.Name)
	err = r.Client.Get(context.TODO(), key, userSignup)
	require.NoError(t, err)
	assert.Equal(t, "banned", userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey])
	metricstest.AssertMetricsCounterEquals(t, 1, metrics.UserSignupBannedTotal)
	metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupDeactivatedTotal)
	metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
	metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupUniqueTotal)

	// Confirm the status is set to Banning
	test.AssertConditionsMatch(t, userSignup.Status.Conditions,
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupComplete,
			Status: corev1.ConditionFalse,
			Reason: "Banning",
		},
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupApproved,
			Status: corev1.ConditionTrue,
			Reason: "ApprovedAutomatically",
		})

	// Confirm that the MUR has now been deleted
	murtest.AssertThatMasterUserRecords(t, r.Client).HaveCount(0)

	// The Space should still exist because cleanup would be handled by the space cleanup controller
	spacetest.AssertThatSpaces(t, r.Client).HaveCount(1)
	// The SpaceBinding should still exist because cleanup would be handled by the space cleanup controller
	spacebindingtest.AssertThatSpaceBindings(t, r.Client).HaveCount(1)

	AssertThatCountersAndMetrics(t).
		HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
			string(metrics.External): 1,
		}).
		HaveUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
			"1,external": 1,
		})

	t.Run("second reconcile", func(t *testing.T) {
		// when
		_, err = r.Reconcile(context.TODO(), req)
		require.NoError(t, err)

		err = r.Client.Get(context.TODO(), key, userSignup)
		require.NoError(t, err)

		assert.Equal(t, "banned", userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey])
		// metrics should be the same after the 2nd reconcile
		metricstest.AssertMetricsCounterEquals(t, 1, metrics.UserSignupBannedTotal)
		metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupDeactivatedTotal)
		metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
		metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupUniqueTotal)

		// Confirm the status is now set to Banned
		test.AssertConditionsMatch(t, userSignup.Status.Conditions,
			toolchainv1alpha1.Condition{
				Type:   toolchainv1alpha1.UserSignupComplete,
				Status: corev1.ConditionTrue,
				Reason: "Banned",
			},
			toolchainv1alpha1.Condition{
				Type:   toolchainv1alpha1.UserSignupApproved,
				Status: corev1.ConditionTrue,
				Reason: "ApprovedAutomatically",
			})

		// Confirm that there is still no MUR
		murtest.AssertThatMasterUserRecords(t, r.Client).HaveCount(0)
		AssertThatCountersAndMetrics(t).
			HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
				string(metrics.External): 1,
			}).
			HaveUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
				"1,external": 1,
			})

		// The Space should still exist because cleanup would be handled by the space cleanup controller
		spacetest.AssertThatSpaces(t, r.Client).HaveCount(1)

		// The SpaceBinding should still exist because cleanup would be handled by the space cleanup controller
		spacebindingtest.AssertThatSpaceBindings(t, r.Client).HaveCount(1)
	})
}

func TestUserSignupListBannedUsersFails(t *testing.T) {
	// given
	userSignup := commonsignup.NewUserSignup()

	config := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true), testconfig.Metrics().ForceSynchronization(false))
	r, req, fakeClient := prepareReconcile(t, userSignup.Name, config, userSignup, baseNSTemplateTier)
	InitializeCountersWithToolchainConfig(t, config, NewToolchainStatus(
		WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
			string(metrics.External): 1,
		}),
		WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
			"1,external": 1,
		}),
	))

	fakeClient.MockList = func(ctx context.Context, list runtimeclient.ObjectList, opts ...runtimeclient.ListOption) error {
		return errors.New("err happened")
	}

	// when
	_, err := r.Reconcile(context.TODO(), req)

	// then
	require.Error(t, err)
	AssertThatCountersAndMetrics(t).
		HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
			string(metrics.External): 1,
		}).
		HaveUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
			"1,external": 1,
		})
}

func TestUserSignupDeactivatedButMURDeleteFails(t *testing.T) {
	t.Run("usersignup deactivated but mur delete failed", func(t *testing.T) {
		// given
		userSignup := commonsignup.NewUserSignup(commonsignup.Deactivated())
		userSignup.Status = toolchainv1alpha1.UserSignupStatus{
			Conditions: []toolchainv1alpha1.Condition{
				{
					Type:   toolchainv1alpha1.UserSignupComplete,
					Status: corev1.ConditionTrue,
				},
				{
					Type:   toolchainv1alpha1.UserSignupApproved,
					Status: corev1.ConditionTrue,
					Reason: "ApprovedAutomatically",
				},
			},
			CompliantUsername: "john-doe",
		}
		userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey] = "approved"
		userSignup.Labels["toolchain.dev.openshift.com/approved"] = "true"

		key := test.NamespacedName(test.HostOperatorNs, userSignup.Name)

		mur := murtest.NewMasterUserRecord(t, "john-doe", murtest.MetaNamespace(test.HostOperatorNs))
		mur.Labels = map[string]string{toolchainv1alpha1.MasterUserRecordOwnerLabelKey: userSignup.Name}

		space := spacetest.NewSpace(test.HostOperatorNs, "john-doe",
			spacetest.WithCreatorLabel(userSignup.Name),
			spacetest.WithSpecTargetCluster("member-1"),
			spacetest.WithStatusTargetCluster("member-1"), // already provisioned on a target cluster
			spacetest.WithFinalizer())

		spacebinding := spacebindingtest.NewSpaceBinding("john-doe", "john-doe", "admin", userSignup.Name)

		config := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true), testconfig.Metrics().ForceSynchronization(false))
		r, req, fakeClient := prepareReconcile(t, userSignup.Name, config, userSignup, mur, space, spacebinding, baseNSTemplateTier)
		InitializeCountersWithToolchainConfig(t, config, NewToolchainStatus(
			WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
				string(metrics.External): 1,
			}),
			WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
				"1,external": 1,
			}),
		))

		fakeClient.MockDelete = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.DeleteOption) error {
			switch obj.(type) {
			case *toolchainv1alpha1.MasterUserRecord:
				return errors.New("unable to delete mur")
			default:
				return fakeClient.Client.Delete(ctx, obj)
			}
		}

		t.Run("first reconcile", func(t *testing.T) {
			// when
			_, err := r.Reconcile(context.TODO(), req)
			require.Error(t, err)

			// then

			// Lookup the UserSignup
			err = r.Client.Get(context.TODO(), key, userSignup)
			require.NoError(t, err)
			assert.Equal(t, "approved", userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey])
			metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupDeactivatedTotal)
			metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
			metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupUniqueTotal)

			// Confirm the status is set to UnableToDeleteMUR
			test.AssertConditionsMatch(t, userSignup.Status.Conditions,
				toolchainv1alpha1.Condition{
					Type:   toolchainv1alpha1.UserSignupApproved,
					Status: corev1.ConditionTrue,
					Reason: "ApprovedAutomatically",
				},
				toolchainv1alpha1.Condition{
					Type:    toolchainv1alpha1.UserSignupComplete,
					Status:  corev1.ConditionFalse,
					Reason:  "UnableToDeleteMUR",
					Message: "unable to delete mur",
				})
			AssertThatCountersAndMetrics(t).
				HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
					string(metrics.External): 1,
				}).
				HaveUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
					"1,external": 1,
				})

			// The Space and SpaceBinding should still exist because cleanup would be handled by the space cleanup controller
			spacetest.AssertThatSpaces(t, r.Client).HaveCount(1)
			spacebindingtest.AssertThatSpaceBindings(t, r.Client).HaveCount(1)

			t.Run("second reconcile - there should not be a notification created since there was a deletion failure even if reconciled again", func(t *testing.T) {
				_, err := r.Reconcile(context.TODO(), req)
				require.Error(t, err)
				ntest.AssertNoNotificationsExist(t, r.Client)
				assert.Equal(t, "approved", userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey]) // UserSignup should still be approved
				// the metrics should be the same, deactivation should only be counted once
				metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupDeactivatedTotal)
				metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
				metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupUniqueTotal)

				// The Space and SpaceBinding should still exist because cleanup would be handled by the space cleanup controller
				spacetest.AssertThatSpaces(t, r.Client).HaveCount(1)
				spacebindingtest.AssertThatSpaceBindings(t, r.Client).HaveCount(1)
			})
		})
	})

	t.Run("usersignup deactivated but mur already deleted", func(t *testing.T) {
		// given
		userSignup := commonsignup.NewUserSignup(commonsignup.Deactivated())
		userSignup.Status = toolchainv1alpha1.UserSignupStatus{
			Conditions: []toolchainv1alpha1.Condition{
				{
					Type:   toolchainv1alpha1.UserSignupComplete,
					Status: corev1.ConditionTrue,
				},
				{
					Type:   toolchainv1alpha1.UserSignupApproved,
					Status: corev1.ConditionTrue,
					Reason: "ApprovedAutomatically",
				},
			},
			CompliantUsername: "john-doe",
		}

		userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey] = "approved"
		userSignup.Labels["toolchain.dev.openshift.com/approved"] = "true"

		mur := murtest.NewMasterUserRecord(t, "john-doe", murtest.MetaNamespace(test.HostOperatorNs))
		mur.Labels = map[string]string{toolchainv1alpha1.MasterUserRecordOwnerLabelKey: userSignup.Name}

		config := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true), testconfig.Metrics().ForceSynchronization(false))
		r, req, fakeClient := prepareReconcile(t, userSignup.Name, config, userSignup, mur, baseNSTemplateTier)

		fakeClient.MockDelete = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.DeleteOption) error {
			switch obj.(type) {
			case *toolchainv1alpha1.MasterUserRecord:
				return k8serr.NewNotFound(schema.GroupResource{}, "john-doe")
			default:
				return fakeClient.Client.Delete(ctx, obj)
			}
		}

		// when
		_, err := r.Reconcile(context.TODO(), req)
		require.NoError(t, err)
	})
}

func TestUserSignupDeactivatedButStatusUpdateFails(t *testing.T) {
	// given
	userSignup := &toolchainv1alpha1.UserSignup{
		ObjectMeta: commonsignup.NewUserSignupObjectMeta("", "alice.mayweather.doe@redhat.com"),
		Spec: toolchainv1alpha1.UserSignupSpec{
			IdentityClaims: toolchainv1alpha1.IdentityClaimsEmbedded{
				PropagatedClaims: toolchainv1alpha1.PropagatedClaims{
					Sub: "UserID123",
				},
				PreferredUsername: "alice.mayweather.doe@redhat.com",
			},
			States: []toolchainv1alpha1.UserSignupState{toolchainv1alpha1.UserSignupStateDeactivated},
		},
		Status: toolchainv1alpha1.UserSignupStatus{
			Conditions: []toolchainv1alpha1.Condition{
				{
					Type:   toolchainv1alpha1.UserSignupComplete,
					Status: corev1.ConditionTrue,
				},
				{
					Type:   toolchainv1alpha1.UserSignupApproved,
					Status: corev1.ConditionTrue,
					Reason: "ApprovedAutomatically",
				},
			},
			CompliantUsername: "alice-mayweather",
		},
	}
	userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey] = "approved"
	userSignup.Labels["toolchain.dev.openshift.com/approved"] = "true"

	key := test.NamespacedName(test.HostOperatorNs, userSignup.Name)

	mur := murtest.NewMasterUserRecord(t, "john-doe", murtest.MetaNamespace(test.HostOperatorNs))
	mur.Labels = map[string]string{toolchainv1alpha1.MasterUserRecordOwnerLabelKey: userSignup.Name}

	config := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true), testconfig.Metrics().ForceSynchronization(false))
	r, req, fakeClient := prepareReconcile(t, userSignup.Name, config, userSignup, mur, baseNSTemplateTier)
	InitializeCountersWithToolchainConfig(t, config, NewToolchainStatus(
		WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
			string(metrics.External): 1,
		}),
		WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
			"1,external": 1,
		}),
	))

	fakeClient.MockStatusUpdate = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.SubResourceUpdateOption) error {
		switch obj.(type) {
		case *toolchainv1alpha1.UserSignup:
			return errors.New("mock error")
		default:
			return fakeClient.Client.Status().Update(ctx, obj)
		}
	}

	// when
	_, err := r.Reconcile(context.TODO(), req)
	require.Error(t, err)

	// then

	// Lookup the UserSignup
	err = r.Client.Get(context.TODO(), key, userSignup)
	require.NoError(t, err)
	assert.Equal(t, "approved", userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey])
	metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupDeactivatedTotal)
	metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
	metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupUniqueTotal)

	// Status unchanged since it could not be updated
	test.AssertConditionsMatch(t, userSignup.Status.Conditions,
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupApproved,
			Status: corev1.ConditionTrue,
			Reason: "ApprovedAutomatically",
		},
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupComplete,
			Status: corev1.ConditionTrue,
		})
	AssertThatCountersAndMetrics(t).
		HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
			string(metrics.External): 1,
		}).
		HaveUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
			"1,external": 1,
		})
}

type testUsername struct {
	username                  string
	compliantUsername         string
	replacedCompliantUsername string
}

// TestDeathBy100Signups tests the logic of generateCompliantUsername() which allows no more than 100 attempts to find a vacant name
func TestDeathBy100Signups(t *testing.T) {
	// given
	for testcase, testusername := range map[string]testUsername{
		"Username less than maxlengthWithSuffix characters":           {username: "foo@redhat.com", compliantUsername: "foo", replacedCompliantUsername: "foo"},
		"Username length greater than maxlengthWithSuffix characters": {username: "longer-user-names@redhat.com", compliantUsername: "longer-user-names", replacedCompliantUsername: "longer-user-name"},
	} {
		t.Run(testcase, func(t *testing.T) {
			logf.SetLogger(zap.New(zap.UseDevMode(true)))
			userSignup := commonsignup.NewUserSignup(
				commonsignup.WithName(testusername.username),
				commonsignup.ApprovedManually())
			spc1 := hspc.NewEnabledValidTenantSPC("member1")
			initObjs := make([]runtimeclient.Object, 0, 110)
			initObjs = append(initObjs,
				userSignup,
				deactivate30Tier,
				baseNSTemplateTier,
				spc1,
			)

			// create 100 MURs and Spaces that follow the naming pattern used by `generateCompliantUsername()`: `foo`, `foo-2`, ..., `foo-100`
			initObjs = append(initObjs, &toolchainv1alpha1.MasterUserRecord{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testusername.compliantUsername,
					Namespace: test.HostOperatorNs,
					Labels:    map[string]string{toolchainv1alpha1.MasterUserRecordOwnerLabelKey: uuid.Must(uuid.NewV4()).String()},
				},
			})

			// stagger the numbering for MURs and Spaces so that one of them will always be missing. eg. MURs will not be found on odd numbers and Spaces not found on even numbers until it makes 100 attempts
			for i := 2; i <= 100; i += 2 {
				initObjs = append(initObjs,
					murtest.NewMasterUserRecord(t, fmt.Sprintf("%s-%d", testusername.replacedCompliantUsername, i), murtest.WithOwnerLabel(uuid.Must(uuid.NewV4()).String())))
			}
			for i := 3; i <= 100; i += 2 {
				initObjs = append(initObjs,
					spacetest.NewSpace(test.HostOperatorNs, fmt.Sprintf("%s-%d", testusername.replacedCompliantUsername, i)))
			}

			config := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true), testconfig.Metrics().ForceSynchronization(false))

			r, req, _ := prepareReconcile(t, userSignup.Name, config, initObjs...)
			InitializeCountersWithToolchainConfig(t, config, NewToolchainStatus(
				WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
					string(metrics.External): 100,
				}),
				WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
					"1,external": 100,
				}),
			))

			// when
			res, err := r.Reconcile(context.TODO(), req)

			// then
			require.Error(t, err)
			require.EqualError(t, err, fmt.Sprintf("Error generating compliant username for %s: unable to transform username [%s] even after 100 attempts", testusername.username, testusername.username))
			require.Equal(t, reconcile.Result{}, res)

			// Lookup the user signup again
			err = r.Client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
			require.NoError(t, err)
			assert.Equal(t, "approved", userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey])
			metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupDeactivatedTotal)
			metricstest.AssertMetricsCounterEquals(t, 1, metrics.UserSignupApprovedTotal)
			metricstest.AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)
			segmenttest.AssertNoMessageQueued(t, r.SegmentClient)

			test.AssertConditionsMatch(t, userSignup.Status.Conditions,
				toolchainv1alpha1.Condition{
					Type:    toolchainv1alpha1.UserSignupComplete,
					Status:  corev1.ConditionFalse,
					Reason:  "UnableToCreateMUR",
					Message: fmt.Sprintf("unable to transform username [%s] even after 100 attempts", testusername.username),
				},
				toolchainv1alpha1.Condition{
					Type:   toolchainv1alpha1.UserSignupApproved,
					Status: corev1.ConditionTrue,
					Reason: "ApprovedByAdmin",
				},
				toolchainv1alpha1.Condition{
					Type:   toolchainv1alpha1.UserSignupUserDeactivatingNotificationCreated,
					Status: corev1.ConditionFalse,
					Reason: "UserNotInPreDeactivation",
				},
				toolchainv1alpha1.Condition{
					Type:   toolchainv1alpha1.UserSignupUserDeactivatedNotificationCreated,
					Status: corev1.ConditionFalse,
					Reason: "UserIsActive",
				},
			)
			AssertThatCountersAndMetrics(t).
				HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
					string(metrics.External): 100, // unchanged
				}).
				HaveUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
					"1,external": 100,
					"1,internal": 1, // was incremented, even though associated MUR could not be created
				})
		})
	}
}

func TestGenerateUniqueCompliantUsername(t *testing.T) {
	// given
	logf.SetLogger(zap.New(zap.UseDevMode(true)))

	mur := murtest.NewMasterUserRecord(t, "cool-user")
	space := spacetest.NewSpace(test.HostOperatorNs, "cool-user")
	spaceInTerminating := spacetest.NewSpace(test.HostOperatorNs, "cool-user",
		spacetest.WithFinalizer(),
		spacetest.WithDeletionTimestamp(),
		spacetest.WithCreatorLabel("cool-user"))

	for testcase, params := range map[string]struct {
		conflictingObject runtimeclient.Object
		skipSpaceCreation string
		expectedUsername  string
	}{
		"with conflicting MasterUserRecord": {
			conflictingObject: mur,
			skipSpaceCreation: "false",
			expectedUsername:  "cool-user-2",
		},
		"with conflicting MasterUserRecord when space creation is skipped": {
			conflictingObject: mur,
			skipSpaceCreation: "true",
			expectedUsername:  "cool-user-2",
		},
		"with conflicting Space": {
			conflictingObject: space,
			skipSpaceCreation: "false",
			expectedUsername:  "cool-user-2",
		},
		"with conflicting Space  when space creation is skipped": {
			conflictingObject: space,
			skipSpaceCreation: "true",
			expectedUsername:  "cool-user",
		},
		"with conflicting Space in terminating state": {
			conflictingObject: spaceInTerminating,
			skipSpaceCreation: "false",
			expectedUsername:  "cool-user-2",
		},
		"with conflicting Space in terminating state when space creation is skipped": {
			conflictingObject: spaceInTerminating,
			skipSpaceCreation: "true",
			expectedUsername:  "cool-user",
		},
	} {
		t.Run(testcase, func(t *testing.T) {
			userSignup := commonsignup.NewUserSignup(
				commonsignup.WithName("cool-user"),
				commonsignup.ApprovedManually())

			userSignup.Annotations[toolchainv1alpha1.SkipAutoCreateSpaceAnnotationKey] = params.skipSpaceCreation

			spc1 := hspc.NewEnabledValidTenantSPC("member1")
			config := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true), testconfig.Metrics().ForceSynchronization(false))
			r, req, _ := prepareReconcile(t, userSignup.Name, config, spc1, userSignup, baseNSTemplateTier,
				deactivate30Tier, params.conflictingObject)

			// when
			res, err := r.Reconcile(context.TODO(), req)

			// then
			require.NoError(t, err)
			require.Equal(t, reconcile.Result{}, res)

			// Lookup the user signup again
			murtest.AssertThatMasterUserRecord(t, params.expectedUsername, r.Client).
				HasLabelWithValue(toolchainv1alpha1.MasterUserRecordOwnerLabelKey, "cool-user")
			userSignup = AssertThatUserSignup(t, test.HostOperatorNs, "cool-user", r.Client).
				HasCompliantUsername("").
				HasLabel(toolchainv1alpha1.UserSignupStateLabelKey, "approved").
				Get()

			test.AssertConditionsMatch(t, userSignup.Status.Conditions,
				toolchainv1alpha1.Condition{
					Type:   toolchainv1alpha1.UserSignupApproved,
					Status: corev1.ConditionTrue,
					Reason: "ApprovedByAdmin",
				},
				toolchainv1alpha1.Condition{
					Type:   toolchainv1alpha1.UserSignupUserDeactivatingNotificationCreated,
					Status: corev1.ConditionFalse,
					Reason: "UserNotInPreDeactivation",
				},
				toolchainv1alpha1.Condition{
					Type:   toolchainv1alpha1.UserSignupUserDeactivatedNotificationCreated,
					Status: corev1.ConditionFalse,
					Reason: "UserIsActive",
				},
			)
		})
	}
}

func TestUserSignupWithMultipleExistingMURNotOK(t *testing.T) {
	// given
	userSignup := commonsignup.NewUserSignup()

	// Create a MUR with the same UserID
	mur := &toolchainv1alpha1.MasterUserRecord{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: test.HostOperatorNs,
			Labels:    map[string]string{toolchainv1alpha1.MasterUserRecordOwnerLabelKey: userSignup.Name},
		},
	}

	// Create another MUR with the same UserID
	mur2 := &toolchainv1alpha1.MasterUserRecord{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bar",
			Namespace: test.HostOperatorNs,
			Labels:    map[string]string{toolchainv1alpha1.MasterUserRecordOwnerLabelKey: userSignup.Name},
		},
	}

	spc1 := hspc.NewEnabledValidTenantSPC("member1")
	config := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true), testconfig.Metrics().ForceSynchronization(false))
	r, req, _ := prepareReconcile(t, userSignup.Name, config, spc1, userSignup, mur, mur2, baseNSTemplateTier)
	InitializeCountersWithToolchainConfig(t, config, NewToolchainStatus(
		WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
			string(metrics.External): 1,
		}),
		WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
			"1,external": 1,
		}),
	))

	// when
	_, err := r.Reconcile(context.TODO(), req)

	// then
	require.EqualError(t, err, "Multiple MasterUserRecords found: multiple matching MasterUserRecord resources found")

	key := types.NamespacedName{
		Namespace: test.HostOperatorNs,
		Name:      userSignup.Name,
	}
	instance := &toolchainv1alpha1.UserSignup{}
	err = r.Client.Get(context.TODO(), key, instance)
	require.NoError(t, err)

	test.AssertConditionsMatch(t, instance.Status.Conditions,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupComplete,
			Status:  corev1.ConditionFalse,
			Reason:  "InvalidMURState",
			Message: "multiple matching MasterUserRecord resources found",
		},
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupUserDeactivatingNotificationCreated,
			Status: corev1.ConditionFalse,
			Reason: "UserNotInPreDeactivation",
		},
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupUserDeactivatedNotificationCreated,
			Status: corev1.ConditionFalse,
			Reason: "UserIsActive",
		},
	)
	AssertThatCountersAndMetrics(t).
		HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
			string(metrics.External): 1,
		}).
		HaveUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
			"1,external": 1,
		})
	metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupDeactivatedTotal)
	metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
	metricstest.AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)
}

func TestApprovedManuallyUserSignupWhenNoMembersAvailable(t *testing.T) {
	// given
	userSignup := commonsignup.NewUserSignup(commonsignup.ApprovedManually())

	config := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true), testconfig.Metrics().ForceSynchronization(false))
	r, req, _ := prepareReconcile(t, userSignup.Name, config, userSignup, baseNSTemplateTier)
	InitializeCountersWithToolchainConfig(t, config, NewToolchainStatus(
		WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
			string(metrics.External): 1,
		}),
		WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
			"1,external": 1,
		}),
	))

	// when
	_, err := r.Reconcile(context.TODO(), req)

	// then
	require.EqualError(t, err, "no target clusters available: no suitable member cluster found - capacity was reached")
	AssertThatCountersAndMetrics(t).
		HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
			string(metrics.External): 1,
		}).
		HaveUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
			"1,external": 1,
		})

	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)
	assert.Equal(t, "pending", userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey])
	metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupDeactivatedTotal)
	metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
	metricstest.AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)

	test.AssertConditionsMatch(t, userSignup.Status.Conditions,
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupApproved,
			Status: corev1.ConditionTrue,
			Reason: "ApprovedByAdmin",
		},
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupComplete,
			Status:  corev1.ConditionFalse,
			Reason:  "NoClusterAvailable",
			Message: "no suitable member cluster found - capacity was reached",
		},
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupUserDeactivatingNotificationCreated,
			Status: corev1.ConditionFalse,
			Reason: "UserNotInPreDeactivation",
		},
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupUserDeactivatedNotificationCreated,
			Status: corev1.ConditionFalse,
			Reason: "UserIsActive",
		})
}

func TestCaptchaAnnotatedWhenUserSignupBanned(t *testing.T) {
	assessmentAnnotationFraudulent := "FRAUDULENT"
	assessmentAnnotationLegitimate := "LEGITIMATE"
	for tcName, tc := range map[string]struct {
		captchEnabled                                          bool
		userSignupStateLabelKey                                string
		userSignupCaptchaAssessmentIDAnnotationKey             string
		userSignupCaptchaAnnotatedAssessmentAnnotationKey      string
		isBanned                                               bool
		expectedUserSignupCaptchaAnnotatedAssessmentAnnotation string
	}{
		"captcha disabled and signup without UserSignupCaptchaAssessmentIDAnnotationKey": {
			captchEnabled:                                          false, // captcha disabled
			userSignupStateLabelKey:                                toolchainv1alpha1.UserSignupStateLabelValueApproved,
			userSignupCaptchaAssessmentIDAnnotationKey:             "",
			userSignupCaptchaAnnotatedAssessmentAnnotationKey:      "",
			expectedUserSignupCaptchaAnnotatedAssessmentAnnotation: "",
		},
		"captcha disabled and signup with UserSignupCaptchaAssessmentIDAnnotationKey": {
			captchEnabled:                                          false,
			userSignupStateLabelKey:                                toolchainv1alpha1.UserSignupStateLabelValueApproved,
			userSignupCaptchaAssessmentIDAnnotationKey:             "captcha-annotation-123",
			userSignupCaptchaAnnotatedAssessmentAnnotationKey:      "",
			expectedUserSignupCaptchaAnnotatedAssessmentAnnotation: "", // expect assessment annotation to not be set because captcha is disabled
		},
		"signup without UserSignupCaptchaAssessmentIDAnnotationKey set": {
			captchEnabled:                                          true, // captcha enabled
			userSignupStateLabelKey:                                toolchainv1alpha1.UserSignupStateLabelValueApproved,
			userSignupCaptchaAssessmentIDAnnotationKey:             "",
			userSignupCaptchaAnnotatedAssessmentAnnotationKey:      "",
			expectedUserSignupCaptchaAnnotatedAssessmentAnnotation: "", // expect assessment annotation to not be set because there was no assessment ID
		},
		"signup with UserSignupCaptchaAssessmentIDAnnotationKey set and user is approved": {
			captchEnabled:           true,
			isBanned:                false,
			userSignupStateLabelKey: toolchainv1alpha1.UserSignupStateLabelValueApproved,
			userSignupCaptchaAssessmentIDAnnotationKey:             "captcha-annotation-123",
			userSignupCaptchaAnnotatedAssessmentAnnotationKey:      "",
			expectedUserSignupCaptchaAnnotatedAssessmentAnnotation: "", // assessment ID exists but user is approved, nothing to annotate
		},
		"signup with UserSignupCaptchaAssessmentIDAnnotationKey set and user is not approved": {
			captchEnabled:           true,
			isBanned:                false,
			userSignupStateLabelKey: toolchainv1alpha1.UserSignupStateLabelValueNotReady,
			userSignupCaptchaAssessmentIDAnnotationKey:             "captcha-annotation-123",
			userSignupCaptchaAnnotatedAssessmentAnnotationKey:      "",
			expectedUserSignupCaptchaAnnotatedAssessmentAnnotation: "", // assessment ID exists but user is approved, nothing to annotate
		},
		"signup without UserSignupCaptchaAssessmentIDAnnotationKey set and user is now banned": {
			captchEnabled:           true,
			isBanned:                true,
			userSignupStateLabelKey: toolchainv1alpha1.UserSignupStateLabelValueApproved,
			userSignupCaptchaAssessmentIDAnnotationKey:             "", // no assessment ID
			userSignupCaptchaAnnotatedAssessmentAnnotationKey:      "",
			expectedUserSignupCaptchaAnnotatedAssessmentAnnotation: "", // user is banned but there was no assessment ID
		},
		"signup with UserSignupCaptchaAssessmentIDAnnotationKey set and user is now banned": {
			captchEnabled:           true,
			isBanned:                true,
			userSignupStateLabelKey: toolchainv1alpha1.UserSignupStateLabelValueApproved,
			userSignupCaptchaAssessmentIDAnnotationKey:             "captcha-annotation-123",
			userSignupCaptchaAnnotatedAssessmentAnnotationKey:      "",
			expectedUserSignupCaptchaAnnotatedAssessmentAnnotation: assessmentAnnotationFraudulent, // assessment ID is provided and user is banned
		},
		"signup was already banned and assessment annotated": {
			captchEnabled:           true,
			isBanned:                true,
			userSignupStateLabelKey: toolchainv1alpha1.UserSignupStateLabelValueBanned,
			userSignupCaptchaAssessmentIDAnnotationKey:             "captcha-annotation-123",
			userSignupCaptchaAnnotatedAssessmentAnnotationKey:      assessmentAnnotationFraudulent, // user was already annotated
			expectedUserSignupCaptchaAnnotatedAssessmentAnnotation: assessmentAnnotationFraudulent,
		},
		"signup with UserSignupCaptchaAssessmentIDAnnotationKey set and user was banned but is now approved": {
			captchEnabled:           true,
			isBanned:                false,                                             // user is now approved
			userSignupStateLabelKey: toolchainv1alpha1.UserSignupStateLabelValueBanned, // previous state was banned
			userSignupCaptchaAssessmentIDAnnotationKey:             "captcha-annotation-123",
			userSignupCaptchaAnnotatedAssessmentAnnotationKey:      assessmentAnnotationFraudulent, // user was previously banned
			expectedUserSignupCaptchaAnnotatedAssessmentAnnotation: assessmentAnnotationLegitimate,
		},
	} {
		t.Run(tcName, func(t *testing.T) {
			// given
			userSignup := commonsignup.NewUserSignup(
				commonsignup.ApprovedManually(),
				commonsignup.WithTargetCluster("east"))
			userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey] = tc.userSignupStateLabelKey
			if tc.userSignupCaptchaAssessmentIDAnnotationKey != "" {
				userSignup.Annotations[toolchainv1alpha1.UserSignupCaptchaAssessmentIDAnnotationKey] = tc.userSignupCaptchaAssessmentIDAnnotationKey
			}
			if tc.userSignupCaptchaAnnotatedAssessmentAnnotationKey != "" {
				userSignup.Annotations[toolchainv1alpha1.UserSignupCaptchaAnnotatedAssessmentAnnotationKey] = tc.userSignupCaptchaAnnotatedAssessmentAnnotationKey
			}

			initObjs := []runtimeclient.Object{userSignup, baseNSTemplateTier, deactivate30Tier}
			if tc.isBanned {
				bannedUser := &toolchainv1alpha1.BannedUser{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							toolchainv1alpha1.BannedUserEmailHashLabelKey: "fd2addbd8d82f0d2dc088fa122377eaa",
						},
					},
					Spec: toolchainv1alpha1.BannedUserSpec{
						Email: "foo@redhat.com",
					},
				}
				initObjs = append(initObjs, bannedUser)
			}
			config := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true), testconfig.RegistrationService().Verification().CaptchaEnabled(tc.captchEnabled))
			r, req, _ := prepareReconcile(t, userSignup.Name, config, initObjs...)

			// when
			_, err := r.Reconcile(context.TODO(), req)

			// then
			require.NoError(t, err)
			key := test.NamespacedName(test.HostOperatorNs, userSignup.Name)
			err = r.Client.Get(context.TODO(), key, userSignup)
			require.NoError(t, err)
			assert.Equal(t, tc.expectedUserSignupCaptchaAnnotatedAssessmentAnnotation, userSignup.Annotations[toolchainv1alpha1.UserSignupCaptchaAnnotatedAssessmentAnnotationKey]) // annotated assessment should now be set
		})
	}
}

func prepareReconcile(t *testing.T, name string, toolchainConfig *toolchainv1alpha1.ToolchainConfig, initObjs ...runtimeclient.Object) (*Reconciler, reconcile.Request, *test.FakeClient) {
	metrics.Reset()

	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret",
			Namespace: "test-namespace",
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"token": []byte("mycooltoken"),
		},
	}
	toolchainStatus := NewToolchainStatus(
		WithMember("member1", WithNodeRoleUsage("worker", 68), WithNodeRoleUsage("master", 65)))

	if toolchainConfig == nil {
		toolchainConfig = commonconfig.NewToolchainConfigObjWithReset(t, testconfig.Metrics().ForceSynchronization(false))
	}

	InitializeCountersWithToolchainConfig(t, toolchainConfig, toolchainStatus)

	initObjs = append(initObjs, toolchainConfig, secret, toolchainStatus)

	fakeClient := test.NewFakeClient(t, initObjs...)

	r := &Reconciler{
		StatusUpdater: &StatusUpdater{
			Client: fakeClient,
		},
		Scheme:         s,
		ClusterManager: capacity.NewClusterManager(test.HostOperatorNs, fakeClient),
		SegmentClient:  segment.NewClient(segmenttest.NewClient()),
	}
	return r, newReconcileRequest(name), fakeClient
}

func newReconcileRequest(name string) reconcile.Request {
	return reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      name,
			Namespace: test.HostOperatorNs,
		},
	}
}

func TestUsernameWithForbiddenPrefix(t *testing.T) {
	// given
	fakeClient := test.NewFakeClient(t)
	config, err := toolchainconfig.GetToolchainConfig(fakeClient)
	require.NoError(t, err)

	defer counter.Reset()

	// Confirm we have 5 forbidden prefixes by default
	require.Len(t, config.Users().ForbiddenUsernamePrefixes(), 5)
	names := []string{"-Bob", "-Dave", "Linda", ""}

	testingPrefixes := config.Users().ForbiddenUsernamePrefixes()
	// As 'kube' is already a forbidden prefix, so "kubesaw" is covered
	// but testing it explicitly would prevent future changes from breaking this behavior.
	testingPrefixes = append(testingPrefixes, "kubesaw")

	for _, prefix := range testingPrefixes {
		userSignup := commonsignup.NewUserSignup(
			commonsignup.ApprovedManually(),
			commonsignup.WithTargetCluster("east"))
		userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey] = toolchainv1alpha1.UserSignupStateLabelValueNotReady

		for _, name := range names {
			userSignup.Spec.IdentityClaims.PreferredUsername = fmt.Sprintf("%s%s", prefix, name)

			r, req, _ := prepareReconcile(t, userSignup.Name, nil, userSignup, baseNSTemplateTier, deactivate30Tier)

			// when
			_, err := r.Reconcile(context.TODO(), req)
			require.NoError(t, err)

			// then verify that the username has been prefixed - first lookup the UserSignup again
			err = r.Client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
			require.NoError(t, err)

			// Lookup the MUR
			murtest.AssertThatMasterUserRecords(t, r.Client).HaveCount(1)
			murtest.AssertThatMasterUserRecord(t, fmt.Sprintf("crt-%s%s", prefix, name), r.Client).
				Exists().
				HasLabelWithValue(toolchainv1alpha1.MasterUserRecordOwnerLabelKey, userSignup.Name)
		}

	}
}

func TestUsernameWithForbiddenSuffixes(t *testing.T) {
	// given
	fakeClient := test.NewFakeClient(t)
	config, err := toolchainconfig.GetToolchainConfig(fakeClient)
	require.NoError(t, err)

	defer counter.Reset()

	require.Len(t, config.Users().ForbiddenUsernameSuffixes(), 1)
	names := []string{"dedicated-", "cluster-", "bob", ""}

	for _, suffix := range config.Users().ForbiddenUsernameSuffixes() {
		userSignup := commonsignup.NewUserSignup(
			commonsignup.ApprovedManually(),
			commonsignup.WithTargetCluster("east"))
		userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey] = toolchainv1alpha1.UserSignupStateLabelValueNotReady

		for _, name := range names {
			userSignup.Spec.IdentityClaims.PreferredUsername = fmt.Sprintf("%s%s", name, suffix)

			r, req, _ := prepareReconcile(t, userSignup.Name, nil, userSignup, baseNSTemplateTier, deactivate30Tier)

			// when
			_, err := r.Reconcile(context.TODO(), req)
			require.NoError(t, err)

			// then verify that the username has been suffixed - first lookup the UserSignup again
			err = r.Client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
			require.NoError(t, err)

			// Lookup the MUR
			murtest.AssertThatMasterUserRecords(t, r.Client).HaveCount(1)
			murtest.AssertThatMasterUserRecord(t, fmt.Sprintf("%s%s-crt", name, suffix), r.Client).
				Exists().
				HasLabelWithValue(toolchainv1alpha1.MasterUserRecordOwnerLabelKey, userSignup.Name)
		}

	}
}

// Test the scenario where the existing usersignup is reactivated and the CompliantUsername becomes outdated eg. transformUsername func is changed
func TestChangedCompliantUsername(t *testing.T) {
	// starting with a UserSignup that exists and was just approved again (reactivated) and has the now outdated CompliantUsername
	userSignup := commonsignup.NewUserSignup(
		commonsignup.ApprovedManually(),
		commonsignup.WithTargetCluster("east"))
	userSignup.Status = toolchainv1alpha1.UserSignupStatus{
		Conditions: []toolchainv1alpha1.Condition{
			{
				Type:   toolchainv1alpha1.UserSignupApproved,
				Status: corev1.ConditionTrue,
				Reason: "ApprovedAutomatically",
			},
			{
				Type:   toolchainv1alpha1.UserSignupComplete,
				Status: corev1.ConditionTrue,
				Reason: "Deactivated",
			},
			{
				Type:   toolchainv1alpha1.UserSignupUserDeactivatedNotificationCreated,
				Status: corev1.ConditionTrue,
				Reason: "NotificationCRCreated",
			},
		},
		CompliantUsername: "foo-old", // outdated UserSignup CompliantUsername
	}
	// create the initial resources
	r, req, _ := prepareReconcile(t, userSignup.Name, nil, userSignup, baseNSTemplateTier, deactivate30Tier)

	// 1st reconcile should provision a new MUR
	res, err := r.Reconcile(context.TODO(), req)
	require.NoError(t, err)
	require.Equal(t, reconcile.Result{}, res)

	// verify the new MUR is provisioned
	murtest.AssertThatMasterUserRecords(t, r.Client).HaveCount(1)
	mur := murtest.AssertThatMasterUserRecord(t, userSignup.Name, r.Client).
		Exists().
		HasLabelWithValue(toolchainv1alpha1.MasterUserRecordOwnerLabelKey, userSignup.Name).
		HasUserAccounts(1).
		HasTier(*deactivate30Tier).
		Get()

	// lookup the userSignup and check the conditions are updated but the CompliantUsername is still the old one
	AssertThatUserSignup(t, req.Namespace, userSignup.Name, r.Client).HasCompliantUsername("foo-old")

	// 2nd reconcile should create a new Space for the new MUR
	res, err = r.Reconcile(context.TODO(), req)
	require.NoError(t, err)
	require.Equal(t, reconcile.Result{}, res)

	spacetest.AssertThatSpace(t, test.HostOperatorNs, userSignup.Name, r.Client).
		HasLabelWithValue(toolchainv1alpha1.SpaceCreatorLabelKey, userSignup.Name).
		Exists().
		HasSpecTargetCluster("east").
		HasTier(baseNSTemplateTier.Name)
	// given space & mur are ready
	setSpaceToReady(t, r.Client, userSignup.Name)
	setMURToReady(t, r.Client, userSignup.Name)

	// 3rd reconcile should create a new SpaceBinding for the new MUR
	res, err = r.Reconcile(context.TODO(), req)
	require.NoError(t, err)
	require.Equal(t, reconcile.Result{}, res)

	spacebindingtest.AssertThatSpaceBinding(t, test.HostOperatorNs, mur.Name, userSignup.Name, r.Client).
		Exists().
		HasLabelWithValue(toolchainv1alpha1.SpaceCreatorLabelKey, userSignup.Name).
		HasLabelWithValue(toolchainv1alpha1.SpaceBindingMasterUserRecordLabelKey, mur.Name).
		HasLabelWithValue(toolchainv1alpha1.SpaceBindingSpaceLabelKey, userSignup.Name).
		HasSpec(mur.Name, userSignup.Name, "admin")

	// 4th reconcile should update the CompliantUsername on the UserSignup status
	res, err = r.Reconcile(context.TODO(), req)
	require.NoError(t, err)
	require.Equal(t, reconcile.Result{}, res)

	// lookup the userSignup one more time and verify that the CompliantUsername was updated using the current transformUsername logic
	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)
	// the CompliantUsername and MUR name should now match
	AssertThatUserSignup(t, req.Namespace, userSignup.Name, r.Client).HasCompliantUsername(mur.Name)
}

func TestMigrateMur(t *testing.T) {
	// given
	userSignup := commonsignup.NewUserSignup(
		commonsignup.ApprovedManually(),
		commonsignup.WithTargetCluster("east"))
	expectedMur := newMasterUserRecord(userSignup, "east", deactivate30Tier.Name, "foo")

	oldMur := expectedMur.DeepCopy()

	oldMur.Generation = 1
	oldMur.ResourceVersion = "1000"

	// old MUR does not have TierName set
	oldMur.Spec.TierName = ""

	// old MUR has tier hash label set
	oldMur.Labels = map[string]string{
		toolchainv1alpha1.MasterUserRecordOwnerLabelKey: userSignup.Name,
	}

	t.Run("mur should be migrated", func(t *testing.T) {
		// given
		r, req, _ := prepareReconcile(t, userSignup.Name, nil, userSignup, baseNSTemplateTier, oldMur, deactivate30Tier)

		// when
		_, err := r.Reconcile(context.TODO(), req)
		// then verify that the MUR exists and is complete
		require.NoError(t, err)
		murtest.AssertThatMasterUserRecords(t, r.Client).HaveCount(1)
		murtest.AssertThatMasterUserRecord(t, expectedMur.Name, r.Client).
			Exists().
			HasTier(*deactivate30Tier).                                                         // tier name should be set
			HasLabelWithValue(toolchainv1alpha1.MasterUserRecordOwnerLabelKey, userSignup.Name) // other labels unchanged
	})
}

func TestUpdateMetricsByState(t *testing.T) {
	manuallyApprovedSignup := commonsignup.NewUserSignup(
		commonsignup.ApprovedManually(),
		commonsignup.WithTargetCluster("east"))

	automaticallyApprovedSignup := commonsignup.NewUserSignup()

	t.Run("common state changes", func(t *testing.T) {
		t.Run("empty -> not-ready - increment UserSignupUniqueTotal", func(t *testing.T) {
			// given
			metrics.Reset()
			r := &Reconciler{
				SegmentClient: segment.NewClient(segmenttest.NewClient()),
			}
			// when
			r.updateUserSignupMetricsByState(manuallyApprovedSignup, "", toolchainv1alpha1.UserSignupStateLabelValueNotReady)
			r.updateUserSignupMetricsByState(automaticallyApprovedSignup, "", toolchainv1alpha1.UserSignupStateLabelValueNotReady)
			// then
			metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupAutoDeactivatedTotal)
			metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupBannedTotal)
			metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupDeactivatedTotal)
			metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
			metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedWithMethodTotal.WithLabelValues("manual"))
			metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedWithMethodTotal.WithLabelValues("automatic"))
			metricstest.AssertMetricsCounterEquals(t, 2, metrics.UserSignupUniqueTotal)
			segmenttest.AssertNoMessageQueued(t, r.SegmentClient)
		})

		t.Run("not-ready -> pending - no change", func(t *testing.T) {
			// given
			metrics.Reset()
			r := &Reconciler{
				SegmentClient: segment.NewClient(segmenttest.NewClient()),
			}
			// when
			r.updateUserSignupMetricsByState(manuallyApprovedSignup, toolchainv1alpha1.UserSignupStateLabelValueNotReady, "pending")
			r.updateUserSignupMetricsByState(automaticallyApprovedSignup, toolchainv1alpha1.UserSignupStateLabelValueNotReady, "pending")
			// then
			metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupAutoDeactivatedTotal)
			metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupBannedTotal)
			metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupDeactivatedTotal)
			metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
			metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedWithMethodTotal.WithLabelValues("manual"))
			metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedWithMethodTotal.WithLabelValues("automatic"))
			metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupUniqueTotal)
			segmenttest.AssertNoMessageQueued(t, r.SegmentClient)
		})

		t.Run("pending -> approved - increment UserSignupApprovedTotal", func(t *testing.T) {
			// given
			metrics.Reset()
			r := &Reconciler{
				SegmentClient: segment.NewClient(segmenttest.NewClient()),
			}
			// when
			r.updateUserSignupMetricsByState(manuallyApprovedSignup, "pending", "approved")
			r.updateUserSignupMetricsByState(automaticallyApprovedSignup, "pending", "approved")
			// then
			metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupAutoDeactivatedTotal)
			metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupBannedTotal)
			metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupDeactivatedTotal)
			metricstest.AssertMetricsCounterEquals(t, 2, metrics.UserSignupApprovedTotal)
			metricstest.AssertMetricsCounterEquals(t, 1, metrics.UserSignupApprovedWithMethodTotal.WithLabelValues("manual"))
			metricstest.AssertMetricsCounterEquals(t, 1, metrics.UserSignupApprovedWithMethodTotal.WithLabelValues("automatic"))
			metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupUniqueTotal)
			segmenttest.AssertNoMessageQueued(t, r.SegmentClient)
		})

		t.Run("approved -> deactivated - increment UserSignupDeactivatedTotal", func(t *testing.T) {
			// given
			metrics.Reset()
			r := &Reconciler{
				SegmentClient: segment.NewClient(segmenttest.NewClient()),
			}
			// when
			r.updateUserSignupMetricsByState(manuallyApprovedSignup, "approved", "deactivated")
			r.updateUserSignupMetricsByState(automaticallyApprovedSignup, "approved", "deactivated")
			// then
			metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupAutoDeactivatedTotal)
			metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupBannedTotal)
			metricstest.AssertMetricsCounterEquals(t, 2, metrics.UserSignupDeactivatedTotal)
			metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
			metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedWithMethodTotal.WithLabelValues("manual"))
			metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedWithMethodTotal.WithLabelValues("automatic"))
			metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupUniqueTotal)
			segmenttest.AssertNoMessageQueued(t, r.SegmentClient)
		})

		t.Run("pending -> deactivated - do NOT increment UserSignupDeactivatedTotal", func(t *testing.T) {
			// given
			metrics.Reset()
			r := &Reconciler{
				SegmentClient: segment.NewClient(segmenttest.NewClient()),
			}
			// when
			r.updateUserSignupMetricsByState(manuallyApprovedSignup, "pending", "deactivated")
			r.updateUserSignupMetricsByState(automaticallyApprovedSignup, "pending", "deactivated")
			// then
			metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupAutoDeactivatedTotal)
			metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupBannedTotal)
			metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupDeactivatedTotal)
			metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
			metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedWithMethodTotal.WithLabelValues("manual"))
			metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedWithMethodTotal.WithLabelValues("automatic"))
			metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupUniqueTotal)
			segmenttest.AssertNoMessageQueued(t, r.SegmentClient)
		})

		t.Run("deactivated -> banned - increment UserSignupBannedTotal", func(t *testing.T) {
			// given
			metrics.Reset()
			r := &Reconciler{
				SegmentClient: segment.NewClient(segmenttest.NewClient()),
			}
			// when
			r.updateUserSignupMetricsByState(manuallyApprovedSignup, "deactivated", "banned")
			r.updateUserSignupMetricsByState(automaticallyApprovedSignup, "deactivated", "banned")
			// then
			metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupAutoDeactivatedTotal)
			metricstest.AssertMetricsCounterEquals(t, 2, metrics.UserSignupBannedTotal)
			metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupDeactivatedTotal)
			metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
			metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedWithMethodTotal.WithLabelValues("manual"))
			metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedWithMethodTotal.WithLabelValues("automatic"))
			metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupUniqueTotal)
			segmenttest.AssertNoMessageQueued(t, r.SegmentClient)
		})
	})

	t.Run("uncommon state changes", func(t *testing.T) {
		t.Run("old value is not empty", func(t *testing.T) {
			// given
			metrics.Reset()
			r := &Reconciler{
				SegmentClient: segment.NewClient(segmenttest.NewClient()),
			}
			// when
			r.updateUserSignupMetricsByState(manuallyApprovedSignup, "any-value", "")
			r.updateUserSignupMetricsByState(automaticallyApprovedSignup, "any-value", "")
			// then
			metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupAutoDeactivatedTotal)
			metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupBannedTotal)
			metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupDeactivatedTotal)
			metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
			metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedWithMethodTotal.WithLabelValues("manual"))
			metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedWithMethodTotal.WithLabelValues("automatic"))
			metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupUniqueTotal)
			segmenttest.AssertNoMessageQueued(t, r.SegmentClient)
		})

		t.Run("new value is not-ready - no change", func(t *testing.T) {
			// given
			metrics.Reset()
			r := &Reconciler{
				SegmentClient: segment.NewClient(segmenttest.NewClient()),
			}
			// when
			r.updateUserSignupMetricsByState(manuallyApprovedSignup, "any-value", toolchainv1alpha1.UserSignupStateLabelValueNotReady)
			r.updateUserSignupMetricsByState(automaticallyApprovedSignup, "any-value", toolchainv1alpha1.UserSignupStateLabelValueNotReady)
			// then
			metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupAutoDeactivatedTotal)
			metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupBannedTotal)
			metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupDeactivatedTotal)
			metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
			metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedWithMethodTotal.WithLabelValues("manual"))
			metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedWithMethodTotal.WithLabelValues("automatic"))
			metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupUniqueTotal)
			segmenttest.AssertNoMessageQueued(t, r.SegmentClient)
		})

		t.Run("new value is not a valid state - no change", func(t *testing.T) {
			// given
			metrics.Reset()
			r := &Reconciler{
				SegmentClient: segment.NewClient(segmenttest.NewClient()),
			}
			// when
			r.updateUserSignupMetricsByState(manuallyApprovedSignup, "any-value", "x")
			r.updateUserSignupMetricsByState(automaticallyApprovedSignup, "any-value", "x")
			// then
			metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupAutoDeactivatedTotal)
			metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupBannedTotal)
			metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupDeactivatedTotal)
			metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
			metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedWithMethodTotal.WithLabelValues("manual"))
			metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedWithMethodTotal.WithLabelValues("automatic"))
			metricstest.AssertMetricsCounterEquals(t, 0, metrics.UserSignupUniqueTotal)
			segmenttest.AssertNoMessageQueued(t, r.SegmentClient)
		})
	})
}

func TestUserSignupLastTargetClusterAnnotation(t *testing.T) {
	t.Run("last target cluster annotation is not initially set but added when mur is created", func(t *testing.T) {
		// given
		userSignup := commonsignup.NewUserSignup()
		spc1 := hspc.NewEnabledValidTenantSPC("member1")
		spc2 := hspc.NewEnabledValidTenantSPC("member2")
		config := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true), testconfig.Metrics().ForceSynchronization(false))
		r, req, _ := prepareReconcile(t, userSignup.Name, config, spc1, spc2, userSignup, baseNSTemplateTier, deactivate30Tier)

		// when
		res, err := r.Reconcile(context.TODO(), req)

		// then
		require.NoError(t, err)
		assert.Empty(t, res.RequeueAfter)
		AssertThatUserSignup(t, req.Namespace, userSignup.Name, r.Client).
			HasAnnotation(toolchainv1alpha1.UserSignupLastTargetClusterAnnotationKey, "member1")
		murtest.AssertThatMasterUserRecords(t, r.Client).HaveCount(1)
		murtest.AssertThatMasterUserRecord(t, userSignup.Name, r.Client).
			HasTargetCluster("member1")
	})

	t.Run("last target cluster annotation is set but cluster is not ready", func(t *testing.T) {
		// given
		userSignup := commonsignup.NewUserSignup()
		userSignup.Annotations[toolchainv1alpha1.UserSignupLastTargetClusterAnnotationKey] = "member2"
		spc1 := hspc.NewEnabledValidTenantSPC("member1")
		spc2 := hspc.NewEnabledTenantSPC("member2")
		config := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true), testconfig.Metrics().ForceSynchronization(false))
		r, req, _ := prepareReconcile(t, userSignup.Name, config, spc1, spc2, userSignup, baseNSTemplateTier, deactivate30Tier)

		// when
		res, err := r.Reconcile(context.TODO(), req)

		// then
		require.NoError(t, err)
		assert.Empty(t, res.RequeueAfter)
		AssertThatUserSignup(t, req.Namespace, userSignup.Name, r.Client).HasAnnotation(toolchainv1alpha1.UserSignupLastTargetClusterAnnotationKey, "member1")
		murtest.AssertThatMasterUserRecords(t, r.Client).HaveCount(1)
		murtest.AssertThatMasterUserRecord(t, userSignup.Name, r.Client).HasTargetCluster("member1")
	})

	t.Run("last target cluster annotation is set and cluster is ready", func(t *testing.T) {
		// given
		userSignup := commonsignup.NewUserSignup()
		userSignup.Annotations[toolchainv1alpha1.UserSignupLastTargetClusterAnnotationKey] = "member2"
		spc1 := hspc.NewEnabledValidTenantSPC("member1")
		spc2 := hspc.NewEnabledValidTenantSPC("member2")
		config := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true), testconfig.Metrics().ForceSynchronization(false))
		r, req, _ := prepareReconcile(t, userSignup.Name, config, spc1, spc2, userSignup, baseNSTemplateTier, deactivate30Tier)

		// make the member2 SPC valid so that we simulate that it now has enough capacity
		member2Spc := &toolchainv1alpha1.SpaceProvisionerConfig{}
		require.NoError(t, r.Client.Get(context.TODO(), runtimeclient.ObjectKeyFromObject(spc2), member2Spc))
		spc.ModifySpaceProvisionerConfig(member2Spc, spc.WithReadyConditionValid())
		require.NoError(t, r.Client.Status().Update(context.TODO(), member2Spc))

		// when
		res, err := r.Reconcile(context.TODO(), req)

		// then
		require.NoError(t, err)
		assert.Empty(t, res.RequeueAfter)
		AssertThatUserSignup(t, req.Namespace, userSignup.Name, r.Client).
			HasAnnotation(toolchainv1alpha1.UserSignupLastTargetClusterAnnotationKey, "member2")
		murtest.AssertThatMasterUserRecords(t, r.Client).HaveCount(1)
		murtest.AssertThatMasterUserRecord(t, userSignup.Name, r.Client).
			HasTargetCluster("member2")
	})

	t.Run("last target cluster annotation is set but cluster does not exist", func(t *testing.T) {
		// given
		userSignup := commonsignup.NewUserSignup()
		userSignup.Annotations[toolchainv1alpha1.UserSignupLastTargetClusterAnnotationKey] = "member2"
		spc1 := hspc.NewEnabledValidTenantSPC("member1")
		config := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true), testconfig.Metrics().ForceSynchronization(false))
		r, req, _ := prepareReconcile(t, userSignup.Name, config, spc1, userSignup, baseNSTemplateTier, deactivate30Tier)

		// when
		res, err := r.Reconcile(context.TODO(), req)

		// then
		require.NoError(t, err)
		assert.Empty(t, res.RequeueAfter)
		AssertThatUserSignup(t, req.Namespace, userSignup.Name, r.Client).
			HasAnnotation(toolchainv1alpha1.UserSignupLastTargetClusterAnnotationKey, "member1")
		murtest.AssertThatMasterUserRecords(t, r.Client).HaveCount(1)
		murtest.AssertThatMasterUserRecord(t, userSignup.Name, r.Client).
			HasTargetCluster("member1")
	})

	t.Run("last target cluster annotation is not set initially and setting the annotation fails", func(t *testing.T) {
		// given
		userSignup := commonsignup.NewUserSignup()
		userSignupName := userSignup.Name
		spc1 := hspc.NewEnabledValidTenantSPC("member1")
		config := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true), testconfig.Metrics().ForceSynchronization(false))
		r, req, cl := prepareReconcile(t, userSignup.Name, config, spc1, userSignup, baseNSTemplateTier, deactivate30Tier)
		cl.MockUpdate = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.UpdateOption) error {
			s, ok := obj.(*toolchainv1alpha1.UserSignup)
			if ok && s.Annotations[toolchainv1alpha1.UserSignupLastTargetClusterAnnotationKey] == "member1" {
				return fmt.Errorf("error")
			}
			return nil
		}
		cl.MockStatusUpdate = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.SubResourceUpdateOption) error {
			s, ok := obj.(*toolchainv1alpha1.UserSignup)
			if ok && s.Annotations[toolchainv1alpha1.UserSignupLastTargetClusterAnnotationKey] == "member1" {
				return fmt.Errorf("some error")
			}
			return nil
		}

		// when
		res, err := r.Reconcile(context.TODO(), req)

		// then
		require.EqualError(t, err, "unable to update last target cluster annotation on UserSignup resource: error")
		assert.Empty(t, res.RequeueAfter)
		AssertThatUserSignup(t, req.Namespace, userSignupName, cl).
			DoesNotHaveAnnotation(toolchainv1alpha1.UserSignupLastTargetClusterAnnotationKey)
		murtest.AssertThatMasterUserRecords(t, r.Client).HaveCount(0)
	})
}

func TestUserSignupStatusNotReady(t *testing.T) {
	spc1 := hspc.NewEnabledValidTenantSPC("member1")
	logf.SetLogger(zap.New(zap.UseDevMode(true)))

	setup := func() (*toolchainv1alpha1.UserSignup, *toolchainv1alpha1.MasterUserRecord, *toolchainv1alpha1.Space, *toolchainv1alpha1.SpaceBinding) {
		userSignup := commonsignup.NewUserSignup(
			commonsignup.ApprovedManually(),
			commonsignup.WithTargetCluster("member1"),
			commonsignup.WithStateLabel(toolchainv1alpha1.UserSignupStateLabelValueNotReady),
			commonsignup.WithoutAnnotation(toolchainv1alpha1.SkipAutoCreateSpaceAnnotationKey))

		mur := newMasterUserRecord(userSignup, "member1", deactivate30Tier.Name, "foo")
		mur.Labels = map[string]string{toolchainv1alpha1.MasterUserRecordOwnerLabelKey: userSignup.Name}
		space := spacetest.NewSpace(test.HostOperatorNs, "foo",
			spacetest.WithSpecTargetCluster("member-1"),
			spacetest.WithStatusTargetCluster("member-1"), // already provisioned on a target cluster
			spacetest.WithFinalizer())

		spacebinding := spacebindingtest.NewSpaceBinding("foo", "foo", "admin", userSignup.Name)
		return userSignup, mur, space, spacebinding
	}

	signupIncomplete := func(object string) []toolchainv1alpha1.Condition {
		return []toolchainv1alpha1.Condition{
			{
				Type:    toolchainv1alpha1.UserSignupComplete,
				Status:  corev1.ConditionFalse,
				Reason:  toolchainv1alpha1.UserSignupProvisioningSpaceReason,
				Message: object + " foo was not ready",
			},
			{
				Type:   toolchainv1alpha1.UserSignupUserDeactivatingNotificationCreated,
				Status: corev1.ConditionFalse,
				Reason: "UserNotInPreDeactivation",
			},
			{
				Type:   toolchainv1alpha1.UserSignupUserDeactivatedNotificationCreated,
				Status: corev1.ConditionFalse,
				Reason: "UserIsActive",
			},
		}
	}
	signupComplete := []toolchainv1alpha1.Condition{
		{
			Type:   toolchainv1alpha1.UserSignupComplete,
			Status: corev1.ConditionTrue,
			Reason: "",
		},
		{
			Type:   toolchainv1alpha1.UserSignupUserDeactivatingNotificationCreated,
			Status: corev1.ConditionFalse,
			Reason: "UserNotInPreDeactivation",
		},
		{
			Type:   toolchainv1alpha1.UserSignupUserDeactivatedNotificationCreated,
			Status: corev1.ConditionFalse,
			Reason: "UserIsActive",
		},
	}

	initObjects := []runtimeclient.Object{
		baseNSTemplateTier, deactivate30Tier, spc1,
	}

	config := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true), testconfig.Metrics().ForceSynchronization(false))

	t.Run("until Space is provisioned", func(t *testing.T) {
		// given
		userSignup, mur, space, spacebinding := setup()
		r, req, _ := prepareReconcile(t, userSignup.Name, config, append(initObjects, userSignup, mur, space, spacebinding)...)

		// when
		res, err := r.Reconcile(context.TODO(), req)

		// then
		require.NoError(t, err)
		require.Equal(t, reconcile.Result{}, res)
		err = r.Client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
		require.NoError(t, err)
		test.AssertConditionsMatch(t, userSignup.Status.Conditions, signupIncomplete("space")...)

		t.Run("when space is provisioned, but not mur", func(t *testing.T) {
			// given
			setSpaceToReady(t, r.Client, space.Name)

			// when
			res, err := r.Reconcile(context.TODO(), req)

			// then
			require.NoError(t, err)
			require.Equal(t, reconcile.Result{}, res)
			err = r.Client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
			require.NoError(t, err)
			test.AssertConditionsMatch(t, userSignup.Status.Conditions, signupIncomplete("MUR")...)

			t.Run("when space & mur are both provisioned", func(t *testing.T) {
				// given
				setMURToReady(t, r.Client, mur.Name)

				// when
				res, err := r.Reconcile(context.TODO(), req)

				// then
				require.NoError(t, err)
				require.Equal(t, reconcile.Result{}, res)
				err = r.Client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
				require.NoError(t, err)
				test.AssertConditionsMatch(t, userSignup.Status.Conditions, signupComplete...)
			})
		})
	})

	// If a space is updating or mur is not ready, keep usersignups that have already completed
	t.Run("keep usersignups while space is updating", func(t *testing.T) {
		// given
		userSignup, mur, space, spacebinding := setup()
		userSignup.Status.Conditions = signupComplete
		space.Status.Conditions = []toolchainv1alpha1.Condition{{
			Type:   toolchainv1alpha1.ConditionReady,
			Status: corev1.ConditionFalse,
			Reason: toolchainv1alpha1.SpaceUpdatingReason,
		}}
		config := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true), testconfig.Metrics().ForceSynchronization(false))
		r, req, _ := prepareReconcile(t, userSignup.Name, config, append(initObjects, userSignup, mur, space, spacebinding)...)

		// when
		res, err := r.Reconcile(context.TODO(), req)

		// then
		require.NoError(t, err)
		require.Equal(t, reconcile.Result{}, res)
		err = r.Client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
		require.NoError(t, err)
		test.AssertConditionsMatch(t, userSignup.Status.Conditions, signupComplete...)

		t.Run("the same for mur", func(t *testing.T) {
			// given
			setSpaceToReady(t, r.Client, space.Name)
			mur.Status.Conditions = []toolchainv1alpha1.Condition{{
				Type:   toolchainv1alpha1.ConditionReady,
				Status: corev1.ConditionFalse,
				Reason: toolchainv1alpha1.MasterUserRecordProvisioningReason,
			}}
			require.NoError(t, r.Client.Update(context.TODO(), mur))

			// when
			res, err := r.Reconcile(context.TODO(), req)

			// then
			require.NoError(t, err)
			require.Equal(t, reconcile.Result{}, res)
			err = r.Client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
			require.NoError(t, err)
			test.AssertConditionsMatch(t, userSignup.Status.Conditions, signupComplete...)

		})
	})
}

// Test the scenario where the user trying to reactivate but their old space still exists
func TestUserReactivatingWhileOldSpaceExists(t *testing.T) {
	// given
	meta := commonsignup.NewUserSignupObjectMeta("", "john.doe@redhat.com")
	userSignup := &toolchainv1alpha1.UserSignup{
		ObjectMeta: meta,
		Spec: toolchainv1alpha1.UserSignupSpec{
			IdentityClaims: toolchainv1alpha1.IdentityClaimsEmbedded{
				PropagatedClaims: toolchainv1alpha1.PropagatedClaims{
					Sub:         "UserID123",
					UserID:      "135246",
					AccountID:   "357468",
					OriginalSub: "11223344",
					Email:       "john.doe@redhat.com",
				},
				PreferredUsername: meta.Name,
			},
		},
		Status: toolchainv1alpha1.UserSignupStatus{
			CompliantUsername: "john-doe",
		},
	}
	mur := murtest.NewMasterUserRecord(t, "john-doe", murtest.MetaNamespace(test.HostOperatorNs))
	mur.Spec.PropagatedClaims.Email = "john.doe@redhat.com"
	mur.Labels = map[string]string{
		toolchainv1alpha1.MasterUserRecordOwnerLabelKey: userSignup.Name,
		toolchainv1alpha1.UserSignupStateLabelKey:       "approved",
	}

	space := spacetest.NewSpace(test.HostOperatorNs, "john-doe",
		spacetest.WithSpecTargetCluster("member-1"),
		spacetest.WithStatusTargetCluster("member-1"), // already provisioned on a target cluster
		spacetest.WithFinalizer(),
		spacetest.WithCondition(spacetest.Terminating()),
		spacetest.WithDeletionTimestamp())

	key := test.NamespacedName(test.HostOperatorNs, userSignup.Name)

	t.Run("when reactivating usersignup has space in terminating state", func(t *testing.T) {
		// given
		// start with a usersignup that has been just reactivated, and MUR has been created
		userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey] = "approved"
		userSignup.Labels["toolchain.dev.openshift.com/approved"] = "true"
		userSignup.Annotations[toolchainv1alpha1.UserSignupActivationCounterAnnotationKey] = "2" // the user signed up twice
		userSignup.Status.Conditions = []toolchainv1alpha1.Condition{
			{
				Type:   toolchainv1alpha1.UserSignupComplete,
				Status: corev1.ConditionTrue,
				Reason: "Deactivated",
			},
			{
				Type:   toolchainv1alpha1.UserSignupApproved,
				Status: corev1.ConditionTrue,
				Reason: "ApprovedAutomatically",
			},
			{
				Type:   toolchainv1alpha1.UserSignupUserDeactivatingNotificationCreated,
				Status: corev1.ConditionFalse,
				Reason: "UserNotInPreDeactivation",
			},
			{
				Type:   toolchainv1alpha1.UserSignupUserDeactivatedNotificationCreated,
				Status: corev1.ConditionFalse,
				Reason: "UserIsActive",
			},
		}
		spc1 := hspc.NewEnabledValidTenantSPC("member1")
		config := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true), testconfig.Metrics().ForceSynchronization(false))
		r, req, _ := prepareReconcile(t, userSignup.Name, config, spc1, userSignup,
			baseNSTemplateTier, deactivate30Tier, mur, space)

		// when
		_, err := r.Reconcile(context.TODO(), req)

		// then
		require.Error(t, err)

		// Lookup the UserSignup
		err = r.Client.Get(context.TODO(), key, userSignup)
		require.NoError(t, err)

		// Confirm the status shows UserSignup Complete as false and the reason as unable to create space.
		test.AssertContainsCondition(t, userSignup.Status.Conditions, toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupComplete,
			Status:  corev1.ConditionFalse,
			Reason:  "UnableToCreateSpace",
			Message: "cannot create space because it is currently being deleted",
		})
	})
}

func TestRecordProvisionTime(t *testing.T) {
	t.Run("should be in histogram", func(t *testing.T) {
		// given
		t.Cleanup(metrics.Reset)
		for i := 0; i < 4000; i++ {
			userSignup := commonsignup.NewUserSignup(
				commonsignup.WithRequestReceivedTimeAnnotation(time.Now().Add(-time.Duration(i) * time.Second)))
			client := test.NewFakeClient(t, userSignup)

			// when
			err := recordProvisionTime(context.TODO(), client, userSignup)

			// then
			require.NoError(t, err)
			AssertThatUserSignup(t, test.HostOperatorNs, userSignup.Name, client).DoesNotHaveAnnotation(toolchainv1alpha1.UserSignupRequestReceivedTimeAnnotationKey)
		}
		for _, value := range metrics.UserSignupProvisionTimeHistogramBuckets {
			metricstest.AssertHistogramBucketEquals(t, value, value, metrics.UserSignupProvisionTimeHistogram) // could fail when debugging
		}
		metricstest.AssertHistogramSampleCountEquals(t, 4000, metrics.UserSignupProvisionTimeHistogram)
	})

	t.Run("should not be in histogram", func(t *testing.T) {
		verify := func(userSignup *toolchainv1alpha1.UserSignup) {
			// given
			client := test.NewFakeClient(t, userSignup)

			// when
			err := recordProvisionTime(context.TODO(), client, userSignup)

			// then
			require.NoError(t, err)
			AssertThatUserSignup(t, test.HostOperatorNs, userSignup.Name, client).DoesNotHaveAnnotation(toolchainv1alpha1.UserSignupRequestReceivedTimeAnnotationKey)
			metricstest.AssertAllHistogramBucketsAreEmpty(t, metrics.UserSignupProvisionTimeHistogram)
		}

		t.Run("with manual approval", func(t *testing.T) {
			t.Cleanup(metrics.Reset)
			verify(commonsignup.NewUserSignup(
				commonsignup.WithRequestReceivedTimeAnnotation(time.Now()),
				commonsignup.ApprovedManually()))
		})

		t.Run("with passed phone verification", func(t *testing.T) {
			t.Cleanup(metrics.Reset)
			verify(commonsignup.NewUserSignup(
				commonsignup.WithRequestReceivedTimeAnnotation(time.Now()),
				commonsignup.WithLabel(toolchainv1alpha1.UserSignupUserPhoneHashLabelKey, "123")))
		})

		t.Run("request-received-time annotation is missing", func(t *testing.T) {
			t.Cleanup(metrics.Reset)
			verify(commonsignup.NewUserSignup())
		})
	})

	t.Run("failure", func(t *testing.T) {
		t.Run("when annotation removal fails", func(t *testing.T) {
			// given
			t.Cleanup(metrics.Reset)
			userSignup := commonsignup.NewUserSignup(commonsignup.WithRequestReceivedTimeAnnotation(time.Now()))
			client := test.NewFakeClient(t, userSignup)
			client.MockUpdate = func(_ context.Context, _ runtimeclient.Object, _ ...runtimeclient.UpdateOption) error {
				return fmt.Errorf("some error")
			}

			// when
			err := recordProvisionTime(context.TODO(), client, userSignup)

			// then
			require.EqualError(t, err, "some error")
			metricstest.AssertAllHistogramBucketsAreEmpty(t, metrics.UserSignupProvisionTimeHistogram)
		})

		t.Run("when parsing annotation value fails", func(t *testing.T) {
			// given
			t.Cleanup(metrics.Reset)
			userSignup := commonsignup.NewUserSignup(commonsignup.WithRequestReceivedTimeAnnotation(time.Now()))
			userSignup.Annotations[toolchainv1alpha1.UserSignupRequestReceivedTimeAnnotationKey] = "broken"
			client := test.NewFakeClient(t, userSignup)

			// when
			err := recordProvisionTime(context.TODO(), client, userSignup)

			// then
			require.NoError(t, err)
			metricstest.AssertAllHistogramBucketsAreEmpty(t, metrics.UserSignupProvisionTimeHistogram)
		})
	})
}

func setSpaceToReady(t *testing.T, cl runtimeclient.Client, name string) {
	space := toolchainv1alpha1.Space{}
	err := cl.Get(context.TODO(), types.NamespacedName{
		Name:      name,
		Namespace: test.HostOperatorNs,
	}, &space)
	require.NoError(t, err)
	space.Status.Conditions, _ = condition.AddOrUpdateStatusConditions(space.Status.Conditions, toolchainv1alpha1.Condition{
		Type:   toolchainv1alpha1.ConditionReady,
		Status: corev1.ConditionTrue,
		Reason: toolchainv1alpha1.SpaceProvisionedReason,
	})
	err = cl.Status().Update(context.TODO(), &space)
	require.NoError(t, err)
}

func setMURToReady(t *testing.T, cl runtimeclient.Client, name string) {
	mur := toolchainv1alpha1.MasterUserRecord{}
	err := cl.Get(context.TODO(), types.NamespacedName{
		Name:      name,
		Namespace: test.HostOperatorNs,
	}, &mur)
	require.NoError(t, err)
	mur.Status.Conditions, _ = condition.AddOrUpdateStatusConditions(mur.Status.Conditions, toolchainv1alpha1.Condition{
		Type:   toolchainv1alpha1.ConditionReady,
		Status: corev1.ConditionTrue,
		Reason: toolchainv1alpha1.MasterUserRecordProvisionedReason,
	})
	err = cl.Status().Update(context.TODO(), &mur)
	require.NoError(t, err)
}
