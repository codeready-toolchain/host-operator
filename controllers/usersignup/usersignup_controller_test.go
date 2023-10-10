package usersignup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

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
	testusertier "github.com/codeready-toolchain/host-operator/test/usertier"
	"github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	"github.com/codeready-toolchain/toolchain-common/pkg/condition"
	commonconfig "github.com/codeready-toolchain/toolchain-common/pkg/configuration"
	commonsocialevent "github.com/codeready-toolchain/toolchain-common/pkg/socialevent"
	"github.com/codeready-toolchain/toolchain-common/pkg/states"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	testconfig "github.com/codeready-toolchain/toolchain-common/pkg/test/config"
	murtest "github.com/codeready-toolchain/toolchain-common/pkg/test/masteruserrecord"
	testsocialevent "github.com/codeready-toolchain/toolchain-common/pkg/test/socialevent"
	spacetest "github.com/codeready-toolchain/toolchain-common/pkg/test/space"
	commonsignup "github.com/codeready-toolchain/toolchain-common/pkg/test/usersignup"

	"github.com/gofrs/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var baseNSTemplateTier = tiertest.NewNSTemplateTier("base", "dev", "stage")
var base2NSTemplateTier = tiertest.NewNSTemplateTier("base2", "dev", "stage")
var deactivate30Tier = testusertier.NewUserTier("deactivate30", 30)
var deactivate80Tier = testusertier.NewUserTier("deactivate80", 80)
var event = testsocialevent.NewSocialEvent(test.HostOperatorNs, commonsocialevent.NewName(),
	testsocialevent.WithUserTier(deactivate80Tier.Name),
	testsocialevent.WithSpaceTier(base2NSTemplateTier.Name))

func TestUserSignupCreateMUROk(t *testing.T) {
	member := NewMemberClusterWithTenantRole(t, "member1", corev1.ConditionTrue)
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	for testname, userSignup := range map[string]*toolchainv1alpha1.UserSignup{
		"manually approved with valid activation annotation": commonsignup.NewUserSignup(
			commonsignup.ApprovedManually(),
			commonsignup.WithTargetCluster("member1"),
			commonsignup.WithStateLabel(toolchainv1alpha1.UserSignupStateLabelValueNotReady),
			commonsignup.WithAnnotation(toolchainv1alpha1.UserSignupActivationCounterAnnotationKey, "2"), // this is a returning user
			commonsignup.WithAnnotation(toolchainv1alpha1.SSOUserIDAnnotationKey, "198573"),
			commonsignup.WithAnnotation(toolchainv1alpha1.SSOAccountIDAnnotationKey, "387832"),
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
			commonsignup.WithAnnotation(toolchainv1alpha1.SSOUserIDAnnotationKey, "9834722"),
			commonsignup.WithAnnotation(toolchainv1alpha1.SSOAccountIDAnnotationKey, "4837262"),
			commonsignup.WithOriginalSub("original-sub-value:1234")),
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
			config := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true))
			r, req, _ := prepareReconcile(t, userSignup.Name, NewGetMemberClusters(member), config, userSignup, baseNSTemplateTier, deactivate30Tier, deactivate80Tier, event)
			InitializeCounters(t, NewToolchainStatus(
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

			val, ok := userSignup.Annotations[toolchainv1alpha1.SSOUserIDAnnotationKey]
			if ok {
				murtest.AssertThatMasterUserRecord(t, userSignup.Name, r.Client).
					HasAnnotationWithValue(toolchainv1alpha1.SSOUserIDAnnotationKey, val)
			}

			val, ok = userSignup.Annotations[toolchainv1alpha1.SSOAccountIDAnnotationKey]
			if ok {
				murtest.AssertThatMasterUserRecord(t, userSignup.Name, r.Client).
					HasAnnotationWithValue(toolchainv1alpha1.SSOAccountIDAnnotationKey, val)
			}

			murtest.AssertThatMasterUserRecords(t, r.Client).HaveCount(1)
			mur := murtest.AssertThatMasterUserRecord(t, userSignup.Name, r.Client).
				HasLabelWithValue(toolchainv1alpha1.MasterUserRecordOwnerLabelKey, userSignup.Name).
				HasOriginalSub(userSignup.Spec.OriginalSub).
				HasUserAccounts(1).Get()
			switch testname {
			case "automatically approved via social event":
				murtest.AssertThatMasterUserRecord(t, userSignup.Name, r.Client).HasTier(*deactivate80Tier)
			default:
				murtest.AssertThatMasterUserRecord(t, userSignup.Name, r.Client).HasTier(*deactivate30Tier)
			}
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupUniqueTotal) // zero because we started with a not-ready state instead of empty as per usual
			AssertMetricsCounterEquals(t, 1, metrics.UserSignupApprovedTotal)
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
		})
	}
}

func TestUserSignupCreateSpaceAndSpaceBindingOk(t *testing.T) {
	member := NewMemberClusterWithTenantRole(t, "member1", corev1.ConditionTrue)
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	for testname, userSignup := range map[string]*toolchainv1alpha1.UserSignup{
		"without skip space creation annotation": commonsignup.NewUserSignup(
			commonsignup.ApprovedManually(),
			commonsignup.WithTargetCluster("member1"),
			commonsignup.WithStateLabel(toolchainv1alpha1.UserSignupStateLabelValueNotReady),
			commonsignup.WithoutAnnotation(toolchainv1alpha1.SkipAutoCreateSpaceAnnotationKey)),
		"with social event": commonsignup.NewUserSignup(
			commonsignup.ApprovedManually(),
			commonsignup.WithTargetCluster("member1"),
			commonsignup.WithStateLabel(toolchainv1alpha1.UserSignupStateLabelValueNotReady),
			commonsignup.WithoutAnnotation(toolchainv1alpha1.SkipAutoCreateSpaceAnnotationKey),
			commonsignup.WithLabel(toolchainv1alpha1.SocialEventUserSignupLabelKey, event.Name),
		),
		"with skip space creation annotation set to false": commonsignup.NewUserSignup(
			commonsignup.ApprovedManually(),
			commonsignup.WithTargetCluster("member1"),
			commonsignup.WithStateLabel(toolchainv1alpha1.UserSignupStateLabelValueNotReady),
			commonsignup.WithAnnotation(toolchainv1alpha1.SkipAutoCreateSpaceAnnotationKey, "false")),
		"with skip space creation annotation set to true": commonsignup.NewUserSignup(
			commonsignup.ApprovedManually(),
			commonsignup.WithTargetCluster("member1"),
			commonsignup.WithStateLabel(toolchainv1alpha1.UserSignupStateLabelValueNotReady),
			commonsignup.WithAnnotation(toolchainv1alpha1.SkipAutoCreateSpaceAnnotationKey, "true")),
	} {
		t.Run(testname, func(t *testing.T) {
			// given
			defer counter.Reset()

			mur := newMasterUserRecord(userSignup, "member1", deactivate30Tier.Name, "foo")
			mur.Labels = map[string]string{toolchainv1alpha1.MasterUserRecordOwnerLabelKey: userSignup.Name}
			r, req, _ := prepareReconcile(t, userSignup.Name, NewGetMemberClusters(member), userSignup, mur, baseNSTemplateTier, base2NSTemplateTier, deactivate30Tier, deactivate80Tier, event)

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
					HasTier("base")
			case "with social event":
				spacetest.AssertThatSpace(t, test.HostOperatorNs, "foo", r.Client).
					Exists().
					HasLabelWithValue(toolchainv1alpha1.SpaceCreatorLabelKey, userSignup.Name).
					HasSpecTargetCluster("member1").
					HasSpecTargetClusterRoles([]string{cluster.RoleLabel(cluster.Tenant)}).
					HasTier("base2")
			case "with skip space creation annotation set to true":
				spacetest.AssertThatSpace(t, test.HostOperatorNs, "foo", r.Client).
					DoesNotExist()
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
				case "with skip space creation annotation set to true":
					spacebindingtest.AssertThatSpaceBinding(t, test.HostOperatorNs, "foo", "foo", r.Client).
						DoesNotExist()
				default:
					assert.Fail(t, "unknown testcase")
				}
			})
		})
	}
}

func TestDeletingUserSignupShouldNotUpdateMetrics(t *testing.T) {
	// given
	member := NewMemberClusterWithTenantRole(t, "member1", corev1.ConditionTrue)
	defer counter.Reset()
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	userSignup := commonsignup.NewUserSignup(
		commonsignup.ApprovedManually(),
		commonsignup.BeingDeleted(),
		commonsignup.WithStateLabel(toolchainv1alpha1.UserSignupStateLabelValueNotReady),
		commonsignup.WithAnnotation(toolchainv1alpha1.UserSignupActivationCounterAnnotationKey, "2"))
	r, req, _ := prepareReconcile(t, userSignup.Name, NewGetMemberClusters(member), userSignup, baseNSTemplateTier)
	InitializeCounters(t, NewToolchainStatus(
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

}

func TestUserSignupVerificationRequiredMetric(t *testing.T) {
	// given
	member := NewMemberClusterWithTenantRole(t, "member1", corev1.ConditionTrue)
	defer counter.Reset()
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	userSignup := commonsignup.NewUserSignup(
		commonsignup.ApprovedManually(),
	)
	// set verification required to true in spec only, status will be added during reconcile
	states.SetVerificationRequired(userSignup, true)
	r, req, _ := prepareReconcile(t, userSignup.Name, NewGetMemberClusters(member), userSignup, baseNSTemplateTier)
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupVerificationRequiredTotal) // nothing yet since not reconciled yet

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
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupVerificationRequiredTotal) // should be 1 since verification required status was set

	t.Run("second reconcile - metrics counter still equals 1", func(t *testing.T) {
		// when
		_, err := r.Reconcile(context.TODO(), req)

		// then
		require.NoError(t, err)

		// Verify the metric
		AssertMetricsCounterEquals(t, 1, metrics.UserSignupVerificationRequiredTotal) // should still be 1 since verification required status was already set
	})
}

func TestUserSignupWithAutoApprovalWithoutTargetCluster(t *testing.T) {
	// given
	userSignup := commonsignup.NewUserSignup()
	ready := NewGetMemberClusters(NewMemberClusterWithTenantRole(t, "member1", corev1.ConditionTrue))

	r, req, _ := prepareReconcile(t, userSignup.Name, ready, userSignup, commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true)), baseNSTemplateTier, deactivate30Tier)
	InitializeCounters(t, NewToolchainStatus(
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
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupAutoDeactivatedTotal)
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupBannedTotal)
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupDeactivatedTotal)
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupApprovedTotal)
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)
	murtest.AssertThatMasterUserRecords(t, r.Client).HaveCount(1)
	mur := murtest.AssertThatMasterUserRecord(t, userSignup.Spec.Username, r.Client).
		HasLabelWithValue(toolchainv1alpha1.MasterUserRecordOwnerLabelKey, userSignup.Name).
		HasOriginalSub(userSignup.Spec.OriginalSub).
		HasUserAccounts(1).
		HasTier(*deactivate30Tier).
		Get()
	segmenttest.AssertMessageQueuedForProvisionedMur(t, r.SegmentClient, userSignup, mur.Name)

	// space and spacebinding should be created after the next reconcile
	spacetest.AssertThatSpace(t, test.HostOperatorNs, userSignup.Spec.Username, r.Client).DoesNotExist()
	spacebindingtest.AssertThatSpaceBinding(t, test.HostOperatorNs, userSignup.Spec.Username, userSignup.Spec.Username, r.Client).DoesNotExist()

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
		spacetest.AssertThatSpace(t, test.HostOperatorNs, userSignup.Spec.Username, r.Client).
			HasLabelWithValue(toolchainv1alpha1.SpaceCreatorLabelKey, userSignup.Name).
			Exists().
			HasSpecTargetCluster("member1").
			HasSpecTargetClusterRoles([]string{cluster.RoleLabel(cluster.Tenant)}).
			HasTier(baseNSTemplateTier.Name)
		spacebindingtest.AssertThatSpaceBinding(t, test.HostOperatorNs, "foo", "foo", r.Client).
			DoesNotExist()
		t.Run("third reconcile", func(t *testing.T) {
			// set the space to ready
			err = r.setSpaceToReady(userSignup.Spec.Username)
			require.NoError(t, err)
			// when
			res, err = r.Reconcile(context.TODO(), req)

			// then
			require.NoError(t, err)
			require.Equal(t, reconcile.Result{}, res)

			// spacebinding should be created
			spacebindingtest.AssertThatSpaceBinding(t, test.HostOperatorNs, userSignup.Spec.Username, userSignup.Spec.Username, r.Client).
				Exists().
				HasLabelWithValue(toolchainv1alpha1.SpaceCreatorLabelKey, userSignup.Name).
				HasLabelWithValue(toolchainv1alpha1.SpaceBindingMasterUserRecordLabelKey, userSignup.Spec.Username).
				HasLabelWithValue(toolchainv1alpha1.SpaceBindingSpaceLabelKey, userSignup.Spec.Username).
				HasSpec(mur.Name, userSignup.Spec.Username, "admin")

			// Lookup the userSignup one more time and check the conditions are updated
			err = r.Client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
			require.NoError(t, err)
			require.Equal(t, userSignup.Status.CompliantUsername, mur.Name)
			assert.Equal(t, "approved", userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey])

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
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupAutoDeactivatedTotal)
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupBannedTotal)
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupDeactivatedTotal)
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupApprovedTotal)
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)
	segmenttest.AssertMessageQueuedForProvisionedMur(t, r.SegmentClient, userSignup, mur.Name)
}

func TestUserSignupWithMissingEmailAnnotationFails(t *testing.T) {
	// given
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	userSignup := commonsignup.NewUserSignup(commonsignup.WithoutAnnotations())

	ready := NewGetMemberClusters(NewMemberClusterWithTenantRole(t, "member1", corev1.ConditionTrue))
	r, req, _ := prepareReconcile(t, userSignup.Name, ready, userSignup,
		commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true)), baseNSTemplateTier)
	InitializeCounters(t, NewToolchainStatus(
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
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)
	test.AssertConditionsMatch(t, userSignup.Status.Conditions,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupComplete,
			Status:  corev1.ConditionFalse,
			Reason:  "MissingUserEmailAnnotation",
			Message: "missing annotation at usersignup",
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
		commonsignup.WithAnnotation(toolchainv1alpha1.UserSignupUserEmailAnnotationKey, "foo@redhat.com"),
	)

	ready := NewGetMemberClusters(NewMemberClusterWithTenantRole(t, "member1", corev1.ConditionTrue))
	r, req, _ := prepareReconcile(t, userSignup.Name, ready, userSignup, commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true)), baseNSTemplateTier)
	InitializeCounters(t, NewToolchainStatus(
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
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)
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

	ready := NewGetMemberClusters(NewMemberClusterWithTenantRole(t, "member1", corev1.ConditionTrue))
	r, req, fakeClient := prepareReconcile(t, userSignup.Name, ready, userSignup, commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true)), baseNSTemplateTier)
	fakeClient.MockUpdate = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.UpdateOption) error {
		return fmt.Errorf("some error")
	}
	InitializeCounters(t, NewToolchainStatus(
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
	assert.EqualError(t, err, "unable to update state label at UserSignup resource: some error")

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
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupUniqueTotal)
}

func TestUserSignupWithMissingEmailHashLabelFails(t *testing.T) {
	// given
	userSignup := commonsignup.NewUserSignup()
	userSignup.Annotations = map[string]string{
		toolchainv1alpha1.UserSignupUserEmailAnnotationKey: "foo@redhat.com",
	}
	userSignup.Labels = map[string]string{"toolchain.dev.openshift.com/approved": "false"}

	ready := NewGetMemberClusters(NewMemberClusterWithTenantRole(t, "member1", corev1.ConditionTrue))
	r, req, _ := prepareReconcile(t, userSignup.Name, ready, userSignup, commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true)), baseNSTemplateTier)
	InitializeCounters(t, NewToolchainStatus(
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
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)
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
	customUserTier := testusertier.NewUserTier("custom", 120)
	config := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true), testconfig.Tiers().DefaultUserTier("custom"), testconfig.Tiers().DefaultSpaceTier("custom"))
	userSignup := commonsignup.NewUserSignup()
	ready := NewGetMemberClusters(NewMemberClusterWithTenantRole(t, "member1", corev1.ConditionTrue))
	r, req, _ := prepareReconcile(t, userSignup.Name, ready, userSignup, config, customNSTemplateTier, customUserTier) // use custom tier

	commonconfig.ResetCache() // reset the config cache so that the update config is picked up
	InitializeCounters(t, NewToolchainStatus(
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
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupAutoDeactivatedTotal)
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupBannedTotal)
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupDeactivatedTotal)
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupApprovedTotal)
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)
	mur := murtest.AssertThatMasterUserRecord(t, userSignup.Name, r.Client).Get()
	segmenttest.AssertMessageQueuedForProvisionedMur(t, r.SegmentClient, userSignup, mur.Name)

	murtest.AssertThatMasterUserRecords(t, r.Client).HaveCount(1)
	murtest.AssertThatMasterUserRecord(t, userSignup.Name, r.Client).
		HasLabelWithValue(toolchainv1alpha1.MasterUserRecordOwnerLabelKey, userSignup.Name).
		HasOriginalSub(userSignup.Spec.OriginalSub).
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
			config:         commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true)),
			expectedReason: "NoTemplateTierAvailable",
			expectedMsg:    "nstemplatetiers.toolchain.dev.openshift.com \"base\" not found",
		},
		{
			description:    "non-default spacetier",
			config:         commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true), testconfig.Tiers().DefaultSpaceTier("nonexistent")),
			expectedReason: "NoTemplateTierAvailable",
			expectedMsg:    "nstemplatetiers.toolchain.dev.openshift.com \"nonexistent\" not found",
		},
		{
			description:    "default usertier",
			config:         commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true)),
			expectedReason: "NoUserTierAvailable",
			expectedMsg:    "usertiers.toolchain.dev.openshift.com \"deactivate30\" not found",
		},
		{
			description:    "non-default usertier",
			config:         commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true), testconfig.Tiers().DefaultUserTier("nonexistent")),
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
			ready := NewGetMemberClusters(NewMemberClusterWithTenantRole(t, "member1", corev1.ConditionTrue))

			objs := []runtime.Object{userSignup, v.config}
			if strings.Contains(v.description, "spacetier") { // when testing missing spacetier then create mur and usertier so that the error is about space tier
				objs = append(objs, newMasterUserRecord(userSignup, "member-1", deactivate30Tier.Name, "foo"))
				objs = append(objs, deactivate30Tier)
			}
			r, req, _ := prepareReconcile(t, userSignup.Name, ready, objs...) // the tier does not exist

			commonconfig.ResetCache() // reset the config cache so that the update config is picked up
			InitializeCounters(t, NewToolchainStatus(
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
			AssertMetricsCounterEquals(t, 1, metrics.UserSignupApprovedTotal) // incremented, even though the provisioning failed due to missing NSTemplateTier
			AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)   // incremented, even though the provisioning failed due to missing NSTemplateTier
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

	notReady := NewGetMemberClusters(
		NewMemberClusterWithTenantRole(t, "member1", corev1.ConditionFalse),
		NewMemberClusterWithTenantRole(t, "member2", corev1.ConditionFalse))
	config := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true),
		testconfig.CapacityThresholds().MaxNumberOfSpaces(testconfig.PerMemberCluster("member1", 1)))
	r, req, _ := prepareReconcile(t, userSignup.Name, notReady, userSignup, config, baseNSTemplateTier)
	InitializeCounters(t, NewToolchainStatus(
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
	assert.False(t, res.Requeue)
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
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)

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
	noCapacity := NewGetMemberClusters(
		NewMemberClusterWithTenantRole(t, "member1", corev1.ConditionTrue),
		NewMemberClusterWithTenantRole(t, "member2", corev1.ConditionTrue))
	config := commonconfig.NewToolchainConfigObjWithReset(t,
		testconfig.AutomaticApproval().Enabled(true),
		testconfig.CapacityThresholds().ResourceCapacityThreshold(60))
	r, req, _ := prepareReconcile(t, userSignup.Name, noCapacity, userSignup, config, baseNSTemplateTier)
	InitializeCounters(t, NewToolchainStatus(
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
	assert.False(t, res.Requeue)
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
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)

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
	userSignup := commonsignup.NewUserSignup(commonsignup.ApprovedManuallyAgo(time.Minute))
	ready := NewGetMemberClusters(NewMemberClusterWithTenantRole(t, "member1", corev1.ConditionTrue))

	r, req, _ := prepareReconcile(t, userSignup.Name, ready, userSignup, commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true)), baseNSTemplateTier, deactivate30Tier)
	InitializeCounters(t, NewToolchainStatus(
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
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupApprovedTotal)
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)
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
			err = r.setSpaceToReady(mur.Name)
			require.NoError(t, err)
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
			AssertMetricsCounterEquals(t, 1, metrics.UserSignupApprovedTotal)
			AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)
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

func TestUserSignupWithNoApprovalPolicyTreatedAsManualApproved(t *testing.T) {
	// given
	userSignup := commonsignup.NewUserSignup(commonsignup.ApprovedManuallyAgo(time.Minute))

	config := commonconfig.NewToolchainConfigObjWithReset(t)

	ready := NewGetMemberClusters(NewMemberClusterWithTenantRole(t, "member1", corev1.ConditionTrue))

	r, req, _ := prepareReconcile(t, userSignup.Name, ready, userSignup, baseNSTemplateTier, config, deactivate30Tier)
	InitializeCounters(t, NewToolchainStatus(
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
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupApprovedTotal)
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)

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
			//given
			err = r.setSpaceToReady(mur.Name)
			require.NoError(t, err)
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
			AssertMetricsCounterEquals(t, 1, metrics.UserSignupApprovedTotal)
			AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)
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
	ready := NewGetMemberClusters(NewMemberClusterWithTenantRole(t, "member1", corev1.ConditionTrue))

	r, req, _ := prepareReconcile(t, userSignup.Name, ready, userSignup, commonconfig.NewToolchainConfigObjWithReset(t), baseNSTemplateTier)
	InitializeCounters(t, NewToolchainStatus(
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
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)

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

	ready := NewGetMemberClusters(NewMemberClusterWithTenantRole(t, "member1", corev1.ConditionTrue))
	r, req, _ := prepareReconcile(t, userSignup.Name, ready, userSignup, commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true)), baseNSTemplateTier, deactivate30Tier)
	InitializeCounters(t, NewToolchainStatus(
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
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupApprovedTotal)
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)

	murtest.AssertThatMasterUserRecords(t, r.Client).HaveCount(1)
	mur := murtest.AssertThatMasterUserRecord(t, userSignup.Name, r.Client).
		HasLabelWithValue(toolchainv1alpha1.MasterUserRecordOwnerLabelKey, userSignup.Name).
		HasOriginalSub(userSignup.Spec.OriginalSub).
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
			err = r.setSpaceToReady(userSignup.Name)
			require.NoError(t, err)
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
			AssertMetricsCounterEquals(t, 1, metrics.UserSignupApprovedTotal)
			AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)
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

	r, req, _ := prepareReconcile(t, userSignup.Name, NewGetMemberClusters(), userSignup, baseNSTemplateTier)
	InitializeCounters(t, NewToolchainStatus(
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
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)

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

			ready := NewGetMemberClusters(NewMemberClusterWithTenantRole(t, "member1", corev1.ConditionTrue))
			initObjs := []runtime.Object{userSignup, baseNSTemplateTier, deactivate30Tier}
			if testcase.testName == "create space error" {
				// mur must exist first, space is created on the reconcile after the mur is created
				initObjs = append(initObjs, mur)
			} else if testcase.testName == "create spacebinding error" {
				// mur and space must exist first, spacebinding is created on the reconcile after the space is created
				initObjs = append(initObjs, mur, space)
			}
			r, req, fakeClient := prepareReconcile(t, userSignup.Name, ready, initObjs...)
			InitializeCounters(t, NewToolchainStatus(
				WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
					string(metrics.External): 1,
				}),
				WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
					"1,external": 1,
				}),
			))

			fakeClient.MockCreate = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.CreateOption) error {
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
			AssertMetricsCounterEquals(t, 1, metrics.UserSignupApprovedTotal)
			AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)
			segmenttest.AssertNoMessageQueued(t, r.SegmentClient)
		})
	}
}

func TestUserSignupMURReadFails(t *testing.T) {
	// given
	userSignup := commonsignup.NewUserSignup(commonsignup.ApprovedManually())

	ready := NewGetMemberClusters(NewMemberClusterWithTenantRole(t, "member1", corev1.ConditionTrue))
	r, req, fakeClient := prepareReconcile(t, userSignup.Name, ready, userSignup)
	InitializeCounters(t, NewToolchainStatus(
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
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupApprovedTotal)
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)
}

func TestUserSignupSetStatusApprovedByAdminFails(t *testing.T) {
	// given
	userSignup := commonsignup.NewUserSignup(commonsignup.ApprovedManually())
	userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey] = "approved"

	ready := NewGetMemberClusters(NewMemberClusterWithTenantRole(t, "member1", corev1.ConditionTrue))
	r, req, fakeClient := prepareReconcile(t, userSignup.Name, ready, userSignup)
	InitializeCounters(t, NewToolchainStatus(
		WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
			string(metrics.External): 1,
		}),
		WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
			"1,external": 1,
		}),
	))

	fakeClient.MockStatusUpdate = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.UpdateOption) error {
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
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal) // zero since starting state was approved
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupUniqueTotal)   // zero since starting state was approved
	assert.Empty(t, userSignup.Status.Conditions)
}

func TestUserSignupSetStatusApprovedAutomaticallyFails(t *testing.T) {
	// given
	userSignup := commonsignup.NewUserSignup()

	ready := NewGetMemberClusters(NewMemberClusterWithTenantRole(t, "member1", corev1.ConditionTrue))
	r, req, fakeClient := prepareReconcile(t, userSignup.Name, ready, userSignup, commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true)))
	InitializeCounters(t, NewToolchainStatus(
		WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
			string(metrics.External): 1,
		}),
		WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
			"1,external": 1,
		}),
	))

	fakeClient.MockStatusUpdate = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.UpdateOption) error {
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
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)
	assert.Empty(t, userSignup.Status.Conditions)

}

func TestUserSignupSetStatusNoClustersAvailableFails(t *testing.T) {
	// given
	userSignup := commonsignup.NewUserSignup()

	r, req, fakeClient := prepareReconcile(t, userSignup.Name, NewGetMemberClusters(), userSignup, commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true)))
	InitializeCounters(t, NewToolchainStatus(
		WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
			string(metrics.External): 1,
		}),
		WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
			"1,external": 1,
		}),
	))

	fakeClient.MockStatusUpdate = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.UpdateOption) error {
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
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)

}

func TestUserSignupWithExistingMUROK(t *testing.T) {
	// given
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	userSignup := commonsignup.NewUserSignup()
	userSignup.Annotations = map[string]string{
		toolchainv1alpha1.UserSignupUserEmailAnnotationKey: "foo@redhat.com",
	}
	userSignup.Labels = map[string]string{
		toolchainv1alpha1.UserSignupUserEmailHashLabelKey: "fd2addbd8d82f0d2dc088fa122377eaa",
		"toolchain.dev.openshift.com/approved":            "true",
	}
	userSignup.Spec.OriginalSub = "original-sub:foo"

	// Create a MUR with the same UserID but don't set the OriginalSub property
	mur := &toolchainv1alpha1.MasterUserRecord{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: test.HostOperatorNs,
			Labels: map[string]string{
				toolchainv1alpha1.MasterUserRecordOwnerLabelKey: userSignup.Name,
			},
			Annotations: map[string]string{
				toolchainv1alpha1.MasterUserRecordEmailAnnotationKey: userSignup.Annotations[toolchainv1alpha1.UserSignupUserEmailAnnotationKey],
			},
		},
	}

	space := NewSpace(userSignup, "member1", "foo", "base")

	spacebinding := spacebindingtest.NewSpaceBinding("foo", "foo", "admin", userSignup.Name)

	ready := NewGetMemberClusters(NewMemberClusterWithTenantRole(t, "member1", corev1.ConditionTrue))
	r, req, _ := prepareReconcile(t, userSignup.Name, ready, userSignup, mur, space, spacebinding, commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true)), baseNSTemplateTier, deactivate30Tier)
	InitializeCounters(t, NewToolchainStatus(
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
	require.Equal(t, userSignup.Spec.OriginalSub, murInstance.Spec.OriginalSub)

	t.Run("reconcile a second time to update UserSignup.Status", func(t *testing.T) {
		// given the space is ready
		err = r.setSpaceToReady("foo")
		require.NoError(t, err)
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
		AssertMetricsCounterEquals(t, 1, metrics.UserSignupApprovedTotal)
		AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)

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

	ready := NewGetMemberClusters(NewMemberClusterWithTenantRole(t, "member1", corev1.ConditionTrue))
	r, req, _ := prepareReconcile(t, userSignup.Name, ready, userSignup, mur, commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true)), baseNSTemplateTier, deactivate30Tier)
	InitializeCounters(t, NewToolchainStatus(
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
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupApprovedTotal)
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)
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
		AssertMetricsCounterEquals(t, 1, metrics.UserSignupApprovedTotal)
		AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)
		usMur := murtest.AssertThatMasterUserRecord(t, userSignup.Name, r.Client).Get()
		segmenttest.AssertMessageQueuedForProvisionedMur(t, r.SegmentClient, userSignup, usMur.Name)

		t.Run("verify usersignup on third reconcile", func(t *testing.T) {
			// given space is ready
			err = r.setSpaceToReady(userSignup.Name)
			require.NoError(t, err)
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
	ready := NewGetMemberClusters(NewMemberClusterWithTenantRole(t, "member1", corev1.ConditionTrue))

	r, req, _ := prepareReconcile(t, userSignup.Name, ready, userSignup,
		commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true)),
		baseNSTemplateTier, deactivate30Tier)
	InitializeCounters(t, NewToolchainStatus(
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
	userSignup.Spec.IdentityClaims.PropagatedClaims.Email = "abc@def.com"

	// Reconcile the UserSignup again
	r, req, _ = prepareReconcile(t, userSignup.Name, ready, userSignup, deactivate30Tier)
	res, err = r.Reconcile(context.TODO(), req)
	require.NoError(t, err)
	require.Equal(t, reconcile.Result{}, res)

	// Reload the MUR
	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, &mur)
	require.NoError(t, err)

	// Confirm the propagated claim has been updated in the MUR
	require.Equal(t, "abc@def.com", mur.Spec.PropagatedClaims.Email)
}

func TestUserSignupWithSpecialCharOK(t *testing.T) {
	// given
	userSignup := commonsignup.NewUserSignup(commonsignup.WithUsername("foo#$%^bar@redhat.com"))

	ready := NewGetMemberClusters(NewMemberClusterWithTenantRole(t, "member1", corev1.ConditionTrue))
	r, req, _ := prepareReconcile(t, userSignup.Name, ready, userSignup, commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true)), baseNSTemplateTier, deactivate30Tier)
	InitializeCounters(t, NewToolchainStatus(
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

	AssertMetricsCounterEquals(t, 1, metrics.UserSignupApprovedTotal)
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)
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

		r, req, _ := prepareReconcile(t, userSignup.Name, NewGetMemberClusters(), userSignup, mur, space, spacebinding,
			commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true)), baseNSTemplateTier)
		InitializeCounters(t, NewToolchainStatus(
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
		AssertMetricsCounterEquals(t, 0, metrics.UserSignupDeactivatedTotal) // 0 because usersignup has not reconciled the deactivation
		AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)    // 0 because usersignup was originally deactivated
		AssertMetricsCounterEquals(t, 0, metrics.UserSignupUniqueTotal)      // 0 because state was initially set to approved

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
		r, req, _ := prepareReconcile(t, userSignup.Name, NewGetMemberClusters(), userSignup, commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true)), baseNSTemplateTier)
		InitializeCounters(t, NewToolchainStatus(
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
		AssertMetricsCounterEquals(t, 1, metrics.UserSignupDeactivatedTotal) // one because the deactivation was reconciled
		AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
		AssertMetricsCounterEquals(t, 0, metrics.UserSignupUniqueTotal)

		// Confirm the status has been set to Deactivated
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

		// A deactivated notification should have been created
		notifications := &toolchainv1alpha1.NotificationList{}
		err = r.Client.List(context.TODO(), notifications)
		require.NoError(t, err)
		require.Len(t, notifications.Items, 1)
		notification := notifications.Items[0]
		require.Contains(t, notification.Name, "john-doe-deactivated-")
		assert.True(t, len(notification.Name) > len("john-doe-deactivated-"))
		require.Equal(t, userSignup.Spec.Userid, notification.Spec.Context["UserID"])
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

		r, req, _ := prepareReconcile(t, userSignup2.Name, NewGetMemberClusters(), userSignup2,
			commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true)),
			baseNSTemplateTier, existingNotification)

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
			Userid:   "UserID123",
			Username: meta.Name,
			States:   []toolchainv1alpha1.UserSignupState{toolchainv1alpha1.UserSignupStateDeactivated},
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
		r, req, fakeClient := prepareReconcile(t, userSignup.Name, NewGetMemberClusters(), userSignup,
			commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true)), baseNSTemplateTier)
		InitializeCounters(t, NewToolchainStatus(
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
		AssertMetricsCounterEquals(t, 0, metrics.UserSignupDeactivatedTotal)
		AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
		AssertMetricsCounterEquals(t, 0, metrics.UserSignupUniqueTotal)

		// A deactivated notification should not have been created
		notificationList := &toolchainv1alpha1.NotificationList{}
		err = r.Client.List(context.TODO(), notificationList)
		require.NoError(t, err)
		require.Equal(t, 0, len(notificationList.Items))
	})
}

func TestUserSignupReactivateAfterDeactivated(t *testing.T) {
	// given
	meta := commonsignup.NewUserSignupObjectMeta("", "john.doe@redhat.com")
	userSignup := &toolchainv1alpha1.UserSignup{
		ObjectMeta: meta,
		Spec: toolchainv1alpha1.UserSignupSpec{
			Userid:   "UserID123",
			Username: meta.Name,
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
				Status: corev1.ConditionTrue,
				Reason: "ApprovedAutomatically",
			},
			{
				Type:   toolchainv1alpha1.UserSignupUserDeactivatedNotificationCreated,
				Status: corev1.ConditionTrue,
				Reason: "NotificationCRCreated",
			},
		}
		ready := NewGetMemberClusters(NewMemberClusterWithTenantRole(t, "member1", corev1.ConditionTrue))
		r, req, _ := prepareReconcile(t, userSignup.Name, ready, userSignup, commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true)), baseNSTemplateTier, deactivate30Tier)
		InitializeCounters(t, NewToolchainStatus(
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
		AssertMetricsCounterEquals(t, 0, metrics.UserSignupDeactivatedTotal)
		AssertMetricsCounterEquals(t, 1, metrics.UserSignupApprovedTotal)
		AssertMetricsCounterEquals(t, 0, metrics.UserSignupUniqueTotal)
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
		r, req, fakeClient := prepareReconcile(t, userSignup.Name, NewGetMemberClusters(), userSignup, commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true)), baseNSTemplateTier)
		InitializeCounters(t, NewToolchainStatus(
			WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
				string(metrics.External): 2,
			}),
			WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
				"1,external": 2,
			}),
		))

		fakeClient.MockStatusUpdate = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.UpdateOption) error {
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
		AssertMetricsCounterEquals(t, 0, metrics.UserSignupDeactivatedTotal)
		AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
		AssertMetricsCounterEquals(t, 0, metrics.UserSignupUniqueTotal)

		// A deactivation notification should not be created because this is the reactivation case
		ntest.AssertNoNotificationsExist(t, r.Client)
	})
}

func TestUserSignupDeactivatedWhenMURAndSpaceAndSpaceBindingExists(t *testing.T) {
	// given
	userSignup := &toolchainv1alpha1.UserSignup{
		ObjectMeta: commonsignup.NewUserSignupObjectMeta("", "edward.jones@redhat.com"),
		Spec: toolchainv1alpha1.UserSignupSpec{
			Userid:   "UserID123",
			Username: "edward.jones@redhat.com",
			States:   []toolchainv1alpha1.UserSignupState{toolchainv1alpha1.UserSignupStateApproved},
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
		r, req, _ := prepareReconcile(t, userSignup.Name, NewGetMemberClusters(), userSignup, mur, space, spacebinding, commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true)), baseNSTemplateTier, deactivate30Tier)
		err := r.setSpaceToReady(mur.Name) // given space is ready
		require.NoError(t, err)
		_, err = r.Reconcile(context.TODO(), req)

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

		r, req, _ := prepareReconcile(t, userSignup.Name, NewGetMemberClusters(), userSignup, mur, space, spacebinding, commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true)), baseNSTemplateTier)
		InitializeCounters(t, NewToolchainStatus(
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
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupDeactivatedTotal)
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupUniqueTotal)

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
				},
				toolchainv1alpha1.Condition{
					Type:   toolchainv1alpha1.UserSignupUserDeactivatingNotificationCreated,
					Status: corev1.ConditionFalse,
					Reason: "UserNotInPreDeactivation",
				})
			// metrics should be the same after the 2nd reconcile
			AssertMetricsCounterEquals(t, 1, metrics.UserSignupDeactivatedTotal)
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupUniqueTotal)

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
			Userid:   "UserID089",
			Username: "edward.jones@redhat.com",
			States:   []toolchainv1alpha1.UserSignupState{"deactivating"},
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

	r, req, _ := prepareReconcile(t, userSignup.Name, NewGetMemberClusters(), userSignup, mur,
		commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true)), baseNSTemplateTier, deactivate30Tier)
	InitializeCounters(t, NewToolchainStatus(
		WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
			string(metrics.External): 1,
		}),
	))

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
	require.Equal(t, userSignup.Spec.Userid, notifications.Items[0].Spec.Context["UserID"])

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
	r, req, _ = prepareReconcile(t, userSignup.Name, NewGetMemberClusters(), userSignup, mur, &notifications.Items[0],
		commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true)), baseNSTemplateTier, deactivate30Tier)

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
	require.Equal(t, userSignup.Spec.Userid, notifications.Items[0].Spec.Context["UserID"])

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

	r, req, _ := prepareReconcile(t, userSignup.Name, NewGetMemberClusters(), userSignup, bannedUser, commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true)), baseNSTemplateTier)
	InitializeCounters(t, NewToolchainStatus(
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
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupBannedTotal)
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupDeactivatedTotal)
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupUniqueTotal)

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
	userSignup := commonsignup.NewUserSignup(commonsignup.VerificationRequired(0))

	r, req, _ := prepareReconcile(t, userSignup.Name, NewGetMemberClusters(), userSignup, commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true)), baseNSTemplateTier)
	InitializeCounters(t, NewToolchainStatus(
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
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupBannedTotal)
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupDeactivatedTotal)
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)

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

	r, req, _ := prepareReconcile(t, userSignup.Name, NewGetMemberClusters(), userSignup, mur, space, spacebinding, bannedUser, commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true)), baseNSTemplateTier)
	InitializeCounters(t, NewToolchainStatus(
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
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupBannedTotal)
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupDeactivatedTotal)
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupUniqueTotal)

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
		AssertMetricsCounterEquals(t, 1, metrics.UserSignupBannedTotal)
		AssertMetricsCounterEquals(t, 0, metrics.UserSignupDeactivatedTotal)
		AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
		AssertMetricsCounterEquals(t, 0, metrics.UserSignupUniqueTotal)

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

	r, req, fakeClient := prepareReconcile(t, userSignup.Name, NewGetMemberClusters(), userSignup, commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true)), baseNSTemplateTier)
	InitializeCounters(t, NewToolchainStatus(
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
		userSignup := &toolchainv1alpha1.UserSignup{
			ObjectMeta: commonsignup.NewUserSignupObjectMeta("", "alice.mayweather.doe@redhat.com"),
			Spec: toolchainv1alpha1.UserSignupSpec{
				Userid:   "UserID123",
				Username: "alice.mayweather.doe@redhat.com",
				States:   []toolchainv1alpha1.UserSignupState{toolchainv1alpha1.UserSignupStateDeactivated},
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

		mur := murtest.NewMasterUserRecord(t, "alice-mayweather", murtest.MetaNamespace(test.HostOperatorNs))
		mur.Labels = map[string]string{toolchainv1alpha1.MasterUserRecordOwnerLabelKey: userSignup.Name}

		space := spacetest.NewSpace(test.HostOperatorNs, "alice-mayweather",
			spacetest.WithCreatorLabel(userSignup.Name),
			spacetest.WithSpecTargetCluster("member-1"),
			spacetest.WithStatusTargetCluster("member-1"), // already provisioned on a target cluster
			spacetest.WithFinalizer())

		spacebinding := spacebindingtest.NewSpaceBinding("alice-mayweather", "alice-mayweather", "admin", userSignup.Name)

		r, req, fakeClient := prepareReconcile(t, userSignup.Name, NewGetMemberClusters(), userSignup, mur, space, spacebinding, commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true)), baseNSTemplateTier)
		InitializeCounters(t, NewToolchainStatus(
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
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupDeactivatedTotal)
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupUniqueTotal)

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
				AssertMetricsCounterEquals(t, 0, metrics.UserSignupDeactivatedTotal)
				AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
				AssertMetricsCounterEquals(t, 0, metrics.UserSignupUniqueTotal)

				// The Space and SpaceBinding should still exist because cleanup would be handled by the space cleanup controller
				spacetest.AssertThatSpaces(t, r.Client).HaveCount(1)
				spacebindingtest.AssertThatSpaceBindings(t, r.Client).HaveCount(1)
			})
		})
	})
}

func TestUserSignupDeactivatedButStatusUpdateFails(t *testing.T) {
	// given
	userSignup := &toolchainv1alpha1.UserSignup{
		ObjectMeta: commonsignup.NewUserSignupObjectMeta("", "alice.mayweather.doe@redhat.com"),
		Spec: toolchainv1alpha1.UserSignupSpec{
			Userid:   "UserID123",
			Username: "alice.mayweather.doe@redhat.com",
			States:   []toolchainv1alpha1.UserSignupState{toolchainv1alpha1.UserSignupStateDeactivated},
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

	r, req, fakeClient := prepareReconcile(t, userSignup.Name, NewGetMemberClusters(), userSignup, mur, commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true)), baseNSTemplateTier)
	InitializeCounters(t, NewToolchainStatus(
		WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
			string(metrics.External): 1,
		}),
		WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
			"1,external": 1,
		}),
	))

	fakeClient.MockStatusUpdate = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.UpdateOption) error {
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
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupDeactivatedTotal)
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupUniqueTotal)

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
			initObjs := make([]runtime.Object, 0, 110)
			initObjs = append(initObjs, userSignup, deactivate30Tier)
			initObjs = append(initObjs, commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true)))

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

			initObjs = append(initObjs, baseNSTemplateTier)

			ready := NewGetMemberClusters(NewMemberClusterWithTenantRole(t, "member1", corev1.ConditionTrue))
			r, req, _ := prepareReconcile(t, userSignup.Name, ready, initObjs...)
			InitializeCounters(t, NewToolchainStatus(
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
			assert.EqualError(t, err, fmt.Sprintf("Error generating compliant username for %s: unable to transform username [%s] even after 100 attempts", testusername.username, testusername.username))
			require.Equal(t, reconcile.Result{}, res)

			// Lookup the user signup again
			err = r.Client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
			require.NoError(t, err)
			assert.Equal(t, "approved", userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey])
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupDeactivatedTotal)
			AssertMetricsCounterEquals(t, 1, metrics.UserSignupApprovedTotal)
			AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)
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

			ready := NewGetMemberClusters(NewMemberClusterWithTenantRole(t, "member1", corev1.ConditionTrue))
			r, req, _ := prepareReconcile(t, userSignup.Name, ready, userSignup, baseNSTemplateTier,
				deactivate30Tier, params.conflictingObject, commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true)))

			InitializeCounters(t, NewToolchainStatus())

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

	ready := NewGetMemberClusters(NewMemberClusterWithTenantRole(t, "member1", corev1.ConditionTrue))
	r, req, _ := prepareReconcile(t, userSignup.Name, ready, userSignup, mur, mur2, commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true)), baseNSTemplateTier)
	InitializeCounters(t, NewToolchainStatus(
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
	assert.EqualError(t, err, "Multiple MasterUserRecords found: multiple matching MasterUserRecord resources found")

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
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupDeactivatedTotal)
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)
}

func TestApprovedManuallyUserSignupWhenNoMembersAvailable(t *testing.T) {
	// given
	userSignup := commonsignup.NewUserSignup(commonsignup.ApprovedManually())

	r, req, _ := prepareReconcile(t, userSignup.Name, NewGetMemberClusters(), userSignup, commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true)), baseNSTemplateTier)
	InitializeCounters(t, NewToolchainStatus(
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
	assert.EqualError(t, err, "no target clusters available: no suitable member cluster found - capacity was reached")
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
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupDeactivatedTotal)
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)

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

func prepareReconcile(t *testing.T, name string, getMemberClusters cluster.GetMemberClustersFunc, initObjs ...runtime.Object) (*Reconciler, reconcile.Request, *test.FakeClient) {
	os.Setenv("WATCH_NAMESPACE", test.HostOperatorNs)
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

	initObjs = append(initObjs, secret, toolchainStatus)

	fakeClient := test.NewFakeClient(t, initObjs...)

	r := &Reconciler{
		StatusUpdater: &StatusUpdater{
			Client: fakeClient,
		},
		Scheme:         s,
		ClusterManager: capacity.NewClusterManager(getMemberClusters, fakeClient),
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

	for _, prefix := range config.Users().ForbiddenUsernamePrefixes() {
		userSignup := commonsignup.NewUserSignup(
			commonsignup.ApprovedManually(),
			commonsignup.WithTargetCluster("east"))
		userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey] = toolchainv1alpha1.UserSignupStateLabelValueNotReady

		for _, name := range names {
			userSignup.Spec.Username = fmt.Sprintf("%s%s", prefix, name)

			r, req, _ := prepareReconcile(t, userSignup.Name, NewGetMemberClusters(), userSignup, baseNSTemplateTier, deactivate30Tier)
			InitializeCounters(t, NewToolchainStatus(
				WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
					string(metrics.External): 1,
				}),
			))

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
			userSignup.Spec.Username = fmt.Sprintf("%s%s", name, suffix)

			r, req, _ := prepareReconcile(t, userSignup.Name, NewGetMemberClusters(), userSignup, baseNSTemplateTier, deactivate30Tier)
			InitializeCounters(t, NewToolchainStatus(
				WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
					string(metrics.External): 1,
				}),
			))

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
	r, req, _ := prepareReconcile(t, userSignup.Name, NewGetMemberClusters(), userSignup, baseNSTemplateTier, deactivate30Tier)
	InitializeCounters(t, NewToolchainStatus(
		WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
			string(metrics.External): 1,
		}),
	))

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
	// given space is ready
	err = r.setSpaceToReady(userSignup.Name)
	require.NoError(t, err)
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
		r, req, _ := prepareReconcile(t, userSignup.Name, NewGetMemberClusters(), userSignup, baseNSTemplateTier, oldMur, deactivate30Tier)
		InitializeCounters(t, NewToolchainStatus())

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
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupAutoDeactivatedTotal)
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupBannedTotal)
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupDeactivatedTotal)
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedWithMethodTotal.WithLabelValues("manual"))
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedWithMethodTotal.WithLabelValues("automatic"))
			AssertMetricsCounterEquals(t, 2, metrics.UserSignupUniqueTotal)
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
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupAutoDeactivatedTotal)
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupBannedTotal)
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupDeactivatedTotal)
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedWithMethodTotal.WithLabelValues("manual"))
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedWithMethodTotal.WithLabelValues("automatic"))
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupUniqueTotal)
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
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupAutoDeactivatedTotal)
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupBannedTotal)
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupDeactivatedTotal)
			AssertMetricsCounterEquals(t, 2, metrics.UserSignupApprovedTotal)
			AssertMetricsCounterEquals(t, 1, metrics.UserSignupApprovedWithMethodTotal.WithLabelValues("manual"))
			AssertMetricsCounterEquals(t, 1, metrics.UserSignupApprovedWithMethodTotal.WithLabelValues("automatic"))
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupUniqueTotal)
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
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupAutoDeactivatedTotal)
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupBannedTotal)
			AssertMetricsCounterEquals(t, 2, metrics.UserSignupDeactivatedTotal)
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedWithMethodTotal.WithLabelValues("manual"))
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedWithMethodTotal.WithLabelValues("automatic"))
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupUniqueTotal)
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
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupAutoDeactivatedTotal)
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupBannedTotal)
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupDeactivatedTotal)
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedWithMethodTotal.WithLabelValues("manual"))
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedWithMethodTotal.WithLabelValues("automatic"))
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupUniqueTotal)
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
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupAutoDeactivatedTotal)
			AssertMetricsCounterEquals(t, 2, metrics.UserSignupBannedTotal)
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupDeactivatedTotal)
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedWithMethodTotal.WithLabelValues("manual"))
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedWithMethodTotal.WithLabelValues("automatic"))
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupUniqueTotal)
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
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupAutoDeactivatedTotal)
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupBannedTotal)
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupDeactivatedTotal)
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedWithMethodTotal.WithLabelValues("manual"))
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedWithMethodTotal.WithLabelValues("automatic"))
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupUniqueTotal)
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
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupAutoDeactivatedTotal)
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupBannedTotal)
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupDeactivatedTotal)
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedWithMethodTotal.WithLabelValues("manual"))
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedWithMethodTotal.WithLabelValues("automatic"))
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupUniqueTotal)
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
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupAutoDeactivatedTotal)
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupBannedTotal)
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupDeactivatedTotal)
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedWithMethodTotal.WithLabelValues("manual"))
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedWithMethodTotal.WithLabelValues("automatic"))
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupUniqueTotal)
			segmenttest.AssertNoMessageQueued(t, r.SegmentClient)
		})
	})
}

func TestUserSignupLastTargetClusterAnnotation(t *testing.T) {

	t.Run("last target cluster annotation is not initially set but added when mur is created", func(t *testing.T) {
		// given
		userSignup := commonsignup.NewUserSignup()
		members := NewGetMemberClusters(
			NewMemberClusterWithTenantRole(t, "member1", corev1.ConditionTrue),
			NewMemberClusterWithTenantRole(t, "member2", corev1.ConditionTrue))
		r, req, _ := prepareReconcile(t, userSignup.Name, members, userSignup, baseNSTemplateTier, deactivate30Tier, commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true)))
		InitializeCounters(t, NewToolchainStatus())

		// when
		res, err := r.Reconcile(context.TODO(), req)

		// then
		require.NoError(t, err)
		assert.False(t, res.Requeue)
		AssertThatUserSignup(t, req.Namespace, userSignup.Name, r.Client).
			HasAnnotation(toolchainv1alpha1.UserSignupLastTargetClusterAnnotationKey, "member1")
		murtest.AssertThatMasterUserRecords(t, r.Client).HaveCount(1)
		murtest.AssertThatMasterUserRecord(t, userSignup.Name, r.Client).
			HasTargetCluster("member1")
	})

	t.Run("last target cluster annotation is set but cluster lacks capacity", func(t *testing.T) {
		// given
		userSignup := commonsignup.NewUserSignup()
		userSignup.Annotations[toolchainv1alpha1.UserSignupLastTargetClusterAnnotationKey] = "member2"
		members := NewGetMemberClusters(
			NewMemberClusterWithTenantRole(t, "member1", corev1.ConditionTrue),
			// member2 cluster lacks capacity because the prepareReconcile only sets up the resource consumption for member1 so member2 is automatically excluded
			NewMemberClusterWithTenantRole(t, "member2", corev1.ConditionTrue))
		r, req, _ := prepareReconcile(t, userSignup.Name, members, userSignup, baseNSTemplateTier, deactivate30Tier, commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true)))
		InitializeCounters(t, NewToolchainStatus())

		// when
		res, err := r.Reconcile(context.TODO(), req)

		// then
		require.NoError(t, err)
		assert.False(t, res.Requeue)
		AssertThatUserSignup(t, req.Namespace, userSignup.Name, r.Client).HasAnnotation(toolchainv1alpha1.UserSignupLastTargetClusterAnnotationKey, "member1")
		murtest.AssertThatMasterUserRecords(t, r.Client).HaveCount(1)
		murtest.AssertThatMasterUserRecord(t, userSignup.Name, r.Client).HasTargetCluster("member1")
	})

	t.Run("last target cluster annotation is set and cluster has capacity", func(t *testing.T) {
		// given
		userSignup := commonsignup.NewUserSignup()
		userSignup.Annotations[toolchainv1alpha1.UserSignupLastTargetClusterAnnotationKey] = "member2"
		members := NewGetMemberClusters(
			NewMemberClusterWithTenantRole(t, "member1", corev1.ConditionTrue),
			NewMemberClusterWithTenantRole(t, "member2", corev1.ConditionTrue))
		r, req, _ := prepareReconcile(t, userSignup.Name, members, userSignup, baseNSTemplateTier, deactivate30Tier, commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true)))
		InitializeCounters(t, NewToolchainStatus())

		// set acceptable capacity for member2 cluster
		toolchainStatus := &toolchainv1alpha1.ToolchainStatus{}
		err := r.Client.Get(context.TODO(), types.NamespacedName{Name: toolchainconfig.ToolchainStatusName, Namespace: req.Namespace}, toolchainStatus)
		require.NoError(t, err)
		WithMember("member2", WithNodeRoleUsage("worker", 68), WithNodeRoleUsage("master", 65))(toolchainStatus)
		err = r.Client.Status().Update(context.TODO(), toolchainStatus)
		require.NoError(t, err)

		// when
		res, err := r.Reconcile(context.TODO(), req)

		// then
		require.NoError(t, err)
		assert.False(t, res.Requeue)
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
		members := NewGetMemberClusters(NewMemberClusterWithTenantRole(t, "member1", corev1.ConditionTrue))
		r, req, _ := prepareReconcile(t, userSignup.Name, members, userSignup, baseNSTemplateTier, deactivate30Tier, commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true)))
		InitializeCounters(t, NewToolchainStatus())

		// when
		res, err := r.Reconcile(context.TODO(), req)

		// then
		require.NoError(t, err)
		assert.False(t, res.Requeue)
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
		members := NewGetMemberClusters(NewMemberClusterWithTenantRole(t, "member1", corev1.ConditionTrue))
		r, req, cl := prepareReconcile(t, userSignup.Name, members, userSignup, baseNSTemplateTier, deactivate30Tier, commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true)))
		cl.MockUpdate = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.UpdateOption) error {
			s, ok := obj.(*toolchainv1alpha1.UserSignup)
			if ok && s.Annotations[toolchainv1alpha1.UserSignupLastTargetClusterAnnotationKey] == "member1" {
				return fmt.Errorf("error")
			}
			return nil
		}
		cl.MockStatusUpdate = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.UpdateOption) error {
			s, ok := obj.(*toolchainv1alpha1.UserSignup)
			if ok && s.Annotations[toolchainv1alpha1.UserSignupLastTargetClusterAnnotationKey] == "member1" {
				return fmt.Errorf("some error")
			}
			return nil
		}
		InitializeCounters(t, NewToolchainStatus())

		// when
		res, err := r.Reconcile(context.TODO(), req)

		// then
		require.EqualError(t, err, "unable to update last target cluster annotation on UserSignup resource: error")
		assert.False(t, res.Requeue)
		AssertThatUserSignup(t, req.Namespace, userSignupName, cl).
			HasNoAnnotation(toolchainv1alpha1.UserSignupLastTargetClusterAnnotationKey)
		murtest.AssertThatMasterUserRecords(t, r.Client).HaveCount(0)
	})
}

func TestUserSignupStatusNotReady(t *testing.T) {
	member := NewMemberClusterWithTenantRole(t, "member1", corev1.ConditionTrue)
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

	signupIncomplete := []toolchainv1alpha1.Condition{
		{
			Type:    toolchainv1alpha1.UserSignupComplete,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.UserSignupProvisioningSpaceReason,
			Message: "space foo was not ready",
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

	t.Run("until Space is provisioned", func(t *testing.T) {
		//given
		userSignup, mur, space, spacebinding := setup()
		r, req, _ := prepareReconcile(t, userSignup.Name, NewGetMemberClusters(member), userSignup, mur, space, spacebinding, commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true)), baseNSTemplateTier, deactivate30Tier)
		// when
		res, err := r.Reconcile(context.TODO(), req)
		require.NoError(t, err)
		require.Equal(t, reconcile.Result{}, res)
		// and
		err = r.Client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
		require.NoError(t, err)
		test.AssertConditionsMatch(t, userSignup.Status.Conditions, signupIncomplete...)

	})

	t.Run("when space is provisioned", func(t *testing.T) {
		// given
		userSignup, mur, space, spacebinding := setup()
		space.Status.Conditions = append(space.Status.Conditions, toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.ConditionReady,
			Status: corev1.ConditionTrue,
			Reason: toolchainv1alpha1.SpaceProvisionedReason,
		})
		r, req, _ := prepareReconcile(t, userSignup.Name, NewGetMemberClusters(member), userSignup, mur, space, spacebinding, commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true)), baseNSTemplateTier, deactivate30Tier)
		// when
		res, err := r.Reconcile(context.TODO(), req)
		// then
		require.NoError(t, err)
		require.Equal(t, reconcile.Result{}, res)
		// and
		err = r.Client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
		require.NoError(t, err)
		test.AssertConditionsMatch(t, userSignup.Status.Conditions, signupComplete...)
	})

	// If a space is updating, keep usersignups that have already completed
	t.Run("keep usersignups while space is updating", func(t *testing.T) {
		// given
		userSignup, mur, space, spacebinding := setup()
		userSignup.Status.Conditions = signupComplete
		space.Status.Conditions = []toolchainv1alpha1.Condition{{
			Type:   toolchainv1alpha1.ConditionReady,
			Status: corev1.ConditionFalse,
			Reason: toolchainv1alpha1.SpaceUpdatingReason,
		}}
		r, req, _ := prepareReconcile(t, userSignup.Name, NewGetMemberClusters(member), userSignup, mur, space, spacebinding, commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true)), baseNSTemplateTier, deactivate30Tier)
		// when
		res, err := r.Reconcile(context.TODO(), req)
		require.NoError(t, err)
		require.Equal(t, reconcile.Result{}, res)
		// then
		err = r.Client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
		require.NoError(t, err)
		test.AssertConditionsMatch(t, userSignup.Status.Conditions, signupComplete...)
	})

	// If a space is updating, usersignups that haven't already completed should be marked as incomplete
	t.Run("mark in-progress signups as incomplete when space is updating", func(t *testing.T) {
		// given
		userSignup, mur, space, spacebinding := setup()
		space.Status.Conditions = signupIncomplete
		userSignup.Status.Conditions = []toolchainv1alpha1.Condition{}
		r, req, _ := prepareReconcile(t, userSignup.Name, NewGetMemberClusters(member), userSignup, mur, space, spacebinding, commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true)), baseNSTemplateTier, deactivate30Tier)
		// when
		res, err := r.Reconcile(context.TODO(), req)
		require.NoError(t, err)
		require.Equal(t, reconcile.Result{}, res)
		// then
		err = r.Client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
		require.NoError(t, err)
		test.AssertConditionsMatch(t, userSignup.Status.Conditions, signupIncomplete...)
	})
}

// Test the scenario where the user trying to reactivate but their old space still exists
func TestUserReactivatingWhileOldSpaceExists(t *testing.T) {
	// given
	meta := commonsignup.NewUserSignupObjectMeta("", "john.doe@redhat.com")
	userSignup := &toolchainv1alpha1.UserSignup{
		ObjectMeta: meta,
		Spec: toolchainv1alpha1.UserSignupSpec{
			Userid:   "UserID123",
			Username: meta.Name,
			IdentityClaims: toolchainv1alpha1.IdentityClaimsEmbedded{
				PropagatedClaims: toolchainv1alpha1.PropagatedClaims{
					Sub:         "44332211",
					UserID:      "135246",
					AccountID:   "357468",
					OriginalSub: "11223344",
					Email:       "joe@redhat.com",
				},
			},
		},
		Status: toolchainv1alpha1.UserSignupStatus{
			CompliantUsername: "john-doe",
		},
	}
	mur := murtest.NewMasterUserRecord(t, "john-doe", murtest.MetaNamespace(test.HostOperatorNs))
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
		ready := NewGetMemberClusters(NewMemberClusterWithTenantRole(t, "member1", corev1.ConditionTrue))
		r, req, _ := prepareReconcile(t, userSignup.Name, ready, userSignup,
			commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true)),
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

func (r *Reconciler) setSpaceToReady(name string) error {
	space := toolchainv1alpha1.Space{}
	err := r.Client.Get(context.TODO(), types.NamespacedName{
		Name:      name,
		Namespace: test.HostOperatorNs,
	}, &space)
	if err != nil {
		return err
	}
	space.Status.Conditions = append(space.Status.Conditions, toolchainv1alpha1.Condition{
		Type:   toolchainv1alpha1.ConditionReady,
		Status: corev1.ConditionTrue,
		Reason: toolchainv1alpha1.SpaceProvisionedReason,
	})
	err = r.Client.Update(context.TODO(), &space)
	return err
}
