package usersignup

import (
	"context"
	"errors"
	"fmt"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/controllers/toolchainconfig"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	"github.com/codeready-toolchain/host-operator/pkg/counter"
	"github.com/codeready-toolchain/host-operator/pkg/metrics"
	"github.com/codeready-toolchain/host-operator/pkg/templates/nstemplatetiers"
	. "github.com/codeready-toolchain/host-operator/test"
	ntest "github.com/codeready-toolchain/host-operator/test/notification"
	"github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	commonconfig "github.com/codeready-toolchain/toolchain-common/pkg/configuration"
	"github.com/codeready-toolchain/toolchain-common/pkg/states"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	testconfig "github.com/codeready-toolchain/toolchain-common/pkg/test/config"
	murtest "github.com/codeready-toolchain/toolchain-common/pkg/test/masteruserrecord"
	"github.com/gofrs/uuid"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func newNsTemplateTier(tierName string, nsTypes ...string) *toolchainv1alpha1.NSTemplateTier {
	namespaces := make([]toolchainv1alpha1.NSTemplateTierNamespace, len(nsTypes))
	for i, nsType := range nsTypes {
		revision := fmt.Sprintf("123abc%d", i+1)
		namespaces[i] = toolchainv1alpha1.NSTemplateTierNamespace{
			TemplateRef: nstemplatetiers.NewTierTemplateName(tierName, nsType, revision),
		}
	}

	return &toolchainv1alpha1.NSTemplateTier{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: test.HostOperatorNs,
			Name:      tierName,
		},
		Spec: toolchainv1alpha1.NSTemplateTierSpec{
			Namespaces: namespaces,
			ClusterResources: &toolchainv1alpha1.NSTemplateTierClusterResources{
				TemplateRef: nstemplatetiers.NewTierTemplateName(tierName, "clusterresources", "654321b"),
			},
		},
	}
}

var baseNSTemplateTier = newNsTemplateTier("base", "dev", "stage")

func TestUserSignupCreateMUROk(t *testing.T) {
	member := NewMemberCluster(t, "member1", v1.ConditionTrue)
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	for testname, userSignup := range map[string]*toolchainv1alpha1.UserSignup{
		"with valid activation annotation": NewUserSignup(
			Approved(),
			WithTargetCluster("member1"),
			WithStateLabel("not-ready"),
			WithAnnotation(toolchainv1alpha1.UserSignupActivationCounterAnnotationKey, "2"),
			WithOriginalSub("original-sub-value:1234")), // this is a returning user
		"with invalid activation annotation": NewUserSignup(
			Approved(),
			WithTargetCluster("member1"),
			WithStateLabel("not-ready"),
			WithAnnotation(toolchainv1alpha1.UserSignupActivationCounterAnnotationKey, "?")), // annotation value is not an 'int'
		"without activation annotation": NewUserSignup(
			Approved(),
			WithTargetCluster("member1"),
			WithStateLabel("not-ready"),
			WithoutAnnotation(toolchainv1alpha1.UserSignupActivationCounterAnnotationKey)), // no annotation on this user, so the value will not be incremented
	} {
		t.Run(testname, func(t *testing.T) {
			// given
			defer counter.Reset()
			r, req, _ := prepareReconcile(t, userSignup.Name, NewGetMemberClusters(member), userSignup, baseNSTemplateTier)
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
			murtest.AssertThatMasterUserRecords(t, r.Client).HaveCount(1)
			murtest.AssertThatMasterUserRecord(t, "foo", r.Client).
				HasLabelWithValue(toolchainv1alpha1.MasterUserRecordOwnerLabelKey, userSignup.Name).
				HasOriginalSub(userSignup.Spec.OriginalSub).
				HasUserAccounts(1).
				HasTier(*baseNSTemplateTier)
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupUniqueTotal) // zero because we started with a not-ready state instead of empty as per usual
			AssertMetricsCounterEquals(t, 1, metrics.UserSignupApprovedTotal)

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
			case "with valid activation annotation":
				assert.Equal(t, "3", actualUserSignup.Annotations[toolchainv1alpha1.UserSignupActivationCounterAnnotationKey]) // annotation value is incremented
				AssertThatCountersAndMetrics(t).
					HaveUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
						"1,internal": 0, // unchanged
						"2,internal": 0, // decreased
						"3,internal": 1, // increased
					})
			case "without activation annotation", "with invalid activation annotation":
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

func TestDeletingUserSignupShouldNotUpdateMetrics(t *testing.T) {
	// given
	member := NewMemberCluster(t, "member1", v1.ConditionTrue)
	defer counter.Reset()
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	userSignup := NewUserSignup(BeingDeleted(), Approved(),
		WithStateLabel("not-ready"),
		WithAnnotation(toolchainv1alpha1.UserSignupActivationCounterAnnotationKey, "2"))
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

func TestUserSignupWithAutoApprovalWithoutTargetCluster(t *testing.T) {
	// given
	userSignup := NewUserSignup()

	ready := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue))
	r, req, _ := prepareReconcile(t, userSignup.Name, ready, userSignup, commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true)), baseNSTemplateTier)
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
	mur := murtest.AssertThatMasterUserRecord(t, "foo", r.Client).
		HasLabelWithValue(toolchainv1alpha1.MasterUserRecordOwnerLabelKey, userSignup.Name).
		HasOriginalSub(userSignup.Spec.OriginalSub).
		HasUserAccounts(1).
		HasTier(*baseNSTemplateTier).
		Get()

	test.AssertConditionsMatch(t, userSignup.Status.Conditions,
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupApproved,
			Status: v1.ConditionTrue,
			Reason: "ApprovedAutomatically",
		},
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupUserDeactivatingNotificationCreated,
			Status: v1.ConditionFalse,
			Reason: "UserNotInPreDeactivation",
		},
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupUserDeactivatedNotificationCreated,
			Status: v1.ConditionFalse,
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

		// Lookup the userSignup one more and check the conditions are updated
		err = r.Client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
		require.NoError(t, err)
		require.Equal(t, userSignup.Status.CompliantUsername, mur.Name)
		assert.Equal(t, "approved", userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey])

		test.AssertConditionsMatch(t, userSignup.Status.Conditions,
			toolchainv1alpha1.Condition{
				Type:   toolchainv1alpha1.UserSignupApproved,
				Status: v1.ConditionTrue,
				Reason: "ApprovedAutomatically",
			},
			toolchainv1alpha1.Condition{
				Type:   toolchainv1alpha1.UserSignupComplete,
				Status: v1.ConditionTrue,
			},
			toolchainv1alpha1.Condition{
				Type:   toolchainv1alpha1.UserSignupUserDeactivatingNotificationCreated,
				Status: v1.ConditionFalse,
				Reason: "UserNotInPreDeactivation",
			},
			toolchainv1alpha1.Condition{
				Type:   toolchainv1alpha1.UserSignupUserDeactivatedNotificationCreated,
				Status: v1.ConditionFalse,
				Reason: "UserIsActive",
			})
	})
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupAutoDeactivatedTotal)
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupBannedTotal)
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupDeactivatedTotal)
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupApprovedTotal)
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)
}

func TestUserSignupWithMissingEmailAnnotationFails(t *testing.T) {
	// given
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	userSignup := NewUserSignup(WithoutAnnotations())

	ready := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue))
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
	assert.Equal(t, "not-ready", userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey])
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)
	test.AssertConditionsMatch(t, userSignup.Status.Conditions,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupComplete,
			Status:  v1.ConditionFalse,
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
	userSignup := NewUserSignup(
		WithLabel(toolchainv1alpha1.UserSignupUserEmailHashLabelKey, "abcdef0123456789"),
		WithLabel("toolchain.dev.openshift.com/approved", "false"),
		WithAnnotation(toolchainv1alpha1.UserSignupUserEmailAnnotationKey, "foo@redhat.com"),
	)

	ready := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue))
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
	assert.Equal(t, "not-ready", userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey])
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)
	test.AssertConditionsMatch(t, userSignup.Status.Conditions,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupComplete,
			Status:  v1.ConditionFalse,
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
	userSignup := NewUserSignup()

	ready := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue))
	r, req, fakeClient := prepareReconcile(t, userSignup.Name, ready, userSignup, commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true)), baseNSTemplateTier)
	fakeClient.MockUpdate = func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
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
			Status:  v1.ConditionFalse,
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
	userSignup := NewUserSignup()
	userSignup.Annotations = map[string]string{
		toolchainv1alpha1.UserSignupUserEmailAnnotationKey: "foo@redhat.com",
	}
	userSignup.Labels = map[string]string{"toolchain.dev.openshift.com/approved": "false"}

	ready := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue))
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
	assert.Equal(t, "not-ready", userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey])
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)
	test.AssertConditionsMatch(t, userSignup.Status.Conditions,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupComplete,
			Status:  v1.ConditionFalse,
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
	customTier := newNsTemplateTier("custom", "dev", "stage")
	config := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true), testconfig.Tiers().DefaultTier("custom"))
	userSignup := NewUserSignup()
	ready := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue))
	r, req, _ := prepareReconcile(t, userSignup.Name, ready, userSignup, config, customTier) // use custom tier

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

	murtest.AssertThatMasterUserRecords(t, r.Client).HaveCount(1)
	murtest.AssertThatMasterUserRecord(t, "foo", r.Client).
		HasLabelWithValue(toolchainv1alpha1.MasterUserRecordOwnerLabelKey, userSignup.Name).
		HasOriginalSub(userSignup.Spec.OriginalSub).
		HasUserAccounts(1).
		HasTier(*customTier)

	test.AssertConditionsMatch(t, userSignup.Status.Conditions,
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupApproved,
			Status: v1.ConditionTrue,
			Reason: "ApprovedAutomatically",
		},
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupUserDeactivatingNotificationCreated,
			Status: v1.ConditionFalse,
			Reason: "UserNotInPreDeactivation",
		},
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupUserDeactivatedNotificationCreated,
			Status: v1.ConditionFalse,
			Reason: "UserIsActive",
		})

	AssertThatCountersAndMetrics(t).HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
		string(metrics.External): 1,
		string(metrics.Internal): 1,
	})
}

func TestUserSignupFailedMissingNSTemplateTier(t *testing.T) {

	type variation struct {
		description string
		tierName    string
		config      *toolchainv1alpha1.ToolchainConfig
	}

	variations := []variation{
		{
			description: "default tier",
			tierName:    "base",
			config:      commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true)),
		},
		{
			description: "non-default tier",
			tierName:    "nonexistent",
			config:      commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true), testconfig.Tiers().DefaultTier("nonexistent")),
		},
	}

	for _, v := range variations {
		t.Run(v.description, func(t *testing.T) {
			// given
			userSignup := NewUserSignup()
			ready := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue))
			r, req, _ := prepareReconcile(t, userSignup.Name, ready, userSignup, v.config) // baseNSTemplateTier and non-default tier do not exist

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
					Status: v1.ConditionTrue,
					Reason: "ApprovedAutomatically",
				},
				toolchainv1alpha1.Condition{
					Type:    toolchainv1alpha1.UserSignupComplete,
					Status:  v1.ConditionFalse,
					Reason:  "NoTemplateTierAvailable",
					Message: fmt.Sprintf("nstemplatetiers.toolchain.dev.openshift.com \"%s\" not found", v.tierName),
				},
				toolchainv1alpha1.Condition{
					Type:   toolchainv1alpha1.UserSignupUserDeactivatingNotificationCreated,
					Status: v1.ConditionFalse,
					Reason: "UserNotInPreDeactivation",
				},
				toolchainv1alpha1.Condition{
					Type:   toolchainv1alpha1.UserSignupUserDeactivatedNotificationCreated,
					Status: v1.ConditionFalse,
					Reason: "UserIsActive",
				})
			assert.Equal(t, "approved", userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey])
			AssertMetricsCounterEquals(t, 1, metrics.UserSignupApprovedTotal) // incremented, even though the provisioning failed due to missing NSTemplateTier
			AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)   // incremented, even though the provisioning failed due to missing NSTemplateTier
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
	userSignup := NewUserSignup()

	notReady := NewGetMemberClusters(
		NewMemberCluster(t, "member1", v1.ConditionFalse),
		NewMemberCluster(t, "member2", v1.ConditionFalse))
	config := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true).MaxNumberOfUsers(1))
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
			Status: v1.ConditionFalse,
			Reason: "PendingApproval",
		},
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupComplete,
			Status: v1.ConditionFalse,
			Reason: "NoClusterAvailable",
		},
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupUserDeactivatingNotificationCreated,
			Status: v1.ConditionFalse,
			Reason: "UserNotInPreDeactivation",
		},
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupUserDeactivatedNotificationCreated,
			Status: v1.ConditionFalse,
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
	userSignup := NewUserSignup()
	noCapacity := NewGetMemberClusters(
		NewMemberCluster(t, "member1", v1.ConditionTrue),
		NewMemberCluster(t, "member2", v1.ConditionTrue))
	config := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true).ResourceCapacityThreshold(60))
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
			Status: v1.ConditionFalse,
			Reason: "PendingApproval",
		},
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupComplete,
			Status: v1.ConditionFalse,
			Reason: "NoClusterAvailable",
		},
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupUserDeactivatingNotificationCreated,
			Status: v1.ConditionFalse,
			Reason: "UserNotInPreDeactivation",
		},
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupUserDeactivatedNotificationCreated,
			Status: v1.ConditionFalse,
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
	userSignup := NewUserSignup(Approved())

	ready := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue))
	r, req, _ := prepareReconcile(t, userSignup.Name, ready, userSignup, commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true)), baseNSTemplateTier)
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
	mur := murtest.AssertThatMasterUserRecord(t, "foo", r.Client).
		HasLabelWithValue(toolchainv1alpha1.MasterUserRecordOwnerLabelKey, userSignup.Name).
		HasUserAccounts(1).
		Get()
	test.AssertConditionsMatch(t, userSignup.Status.Conditions,
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupApproved,
			Status: v1.ConditionTrue,
			Reason: "ApprovedByAdmin",
		},
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupUserDeactivatingNotificationCreated,
			Status: v1.ConditionFalse,
			Reason: "UserNotInPreDeactivation",
		},
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupUserDeactivatedNotificationCreated,
			Status: v1.ConditionFalse,
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

		// Lookup the userSignup one more time and check the conditions are updated
		err = r.Client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
		require.NoError(t, err)
		require.Equal(t, userSignup.Status.CompliantUsername, mur.Name)
		assert.Equal(t, "approved", userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey])
		AssertMetricsCounterEquals(t, 1, metrics.UserSignupApprovedTotal)
		AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)
		test.AssertConditionsMatch(t, userSignup.Status.Conditions,
			toolchainv1alpha1.Condition{
				Type:   toolchainv1alpha1.UserSignupApproved,
				Status: v1.ConditionTrue,
				Reason: "ApprovedByAdmin",
			},
			toolchainv1alpha1.Condition{
				Type:   toolchainv1alpha1.UserSignupComplete,
				Status: v1.ConditionTrue,
			},
			toolchainv1alpha1.Condition{
				Type:   toolchainv1alpha1.UserSignupUserDeactivatingNotificationCreated,
				Status: v1.ConditionFalse,
				Reason: "UserNotInPreDeactivation",
			},
			toolchainv1alpha1.Condition{
				Type:   toolchainv1alpha1.UserSignupUserDeactivatedNotificationCreated,
				Status: v1.ConditionFalse,
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
}

func TestUserSignupWithNoApprovalPolicyTreatedAsManualApproved(t *testing.T) {
	// given
	userSignup := NewUserSignup(Approved())

	config := commonconfig.NewToolchainConfigObjWithReset(t)

	ready := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue))

	r, req, _ := prepareReconcile(t, userSignup.Name, ready, userSignup, baseNSTemplateTier, config)
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
	mur := murtest.AssertThatMasterUserRecord(t, "foo", r.Client).
		HasLabelWithValue(toolchainv1alpha1.MasterUserRecordOwnerLabelKey, userSignup.Name).
		HasUserAccounts(1).
		Get()

	test.AssertConditionsMatch(t, userSignup.Status.Conditions,
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupApproved,
			Status: v1.ConditionTrue,
			Reason: "ApprovedByAdmin",
		},
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupUserDeactivatingNotificationCreated,
			Status: v1.ConditionFalse,
			Reason: "UserNotInPreDeactivation",
		},
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupUserDeactivatedNotificationCreated,
			Status: v1.ConditionFalse,
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

		// Lookup the userSignup one more and check the conditions are updated
		err = r.Client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
		require.NoError(t, err)
		require.Equal(t, userSignup.Status.CompliantUsername, mur.Name)

		assert.Equal(t, "approved", userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey])
		AssertMetricsCounterEquals(t, 1, metrics.UserSignupApprovedTotal)
		AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)

		test.AssertConditionsMatch(t, userSignup.Status.Conditions,
			toolchainv1alpha1.Condition{
				Type:   toolchainv1alpha1.UserSignupApproved,
				Status: v1.ConditionTrue,
				Reason: "ApprovedByAdmin",
			},
			toolchainv1alpha1.Condition{
				Type:   toolchainv1alpha1.UserSignupComplete,
				Status: v1.ConditionTrue,
			},
			toolchainv1alpha1.Condition{
				Type:   toolchainv1alpha1.UserSignupUserDeactivatingNotificationCreated,
				Status: v1.ConditionFalse,
				Reason: "UserNotInPreDeactivation",
			},
			toolchainv1alpha1.Condition{
				Type:   toolchainv1alpha1.UserSignupUserDeactivatedNotificationCreated,
				Status: v1.ConditionFalse,
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
}

func TestUserSignupWithManualApprovalNotApproved(t *testing.T) {
	// given
	userSignup := NewUserSignup()

	ready := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue))
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
			Status: v1.ConditionFalse,
			Reason: "PendingApproval",
		},
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupComplete,
			Status: v1.ConditionFalse,
			Reason: "PendingApproval",
		},
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupUserDeactivatingNotificationCreated,
			Status: v1.ConditionFalse,
			Reason: "UserNotInPreDeactivation",
		},
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupUserDeactivatedNotificationCreated,
			Status: v1.ConditionFalse,
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
	userSignup := NewUserSignup(WithTargetCluster("east"))

	ready := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue))
	r, req, _ := prepareReconcile(t, userSignup.Name, ready, userSignup, commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true)), baseNSTemplateTier)
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
	mur := murtest.AssertThatMasterUserRecord(t, "foo", r.Client).
		HasLabelWithValue(toolchainv1alpha1.MasterUserRecordOwnerLabelKey, userSignup.Name).
		HasOriginalSub(userSignup.Spec.OriginalSub).
		HasUserAccounts(1).
		HasTier(*baseNSTemplateTier).
		Get()

	test.AssertConditionsMatch(t, userSignup.Status.Conditions,
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupApproved,
			Status: v1.ConditionTrue,
			Reason: "ApprovedAutomatically",
		},
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupUserDeactivatingNotificationCreated,
			Status: v1.ConditionFalse,
			Reason: "UserNotInPreDeactivation",
		},
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupUserDeactivatedNotificationCreated,
			Status: v1.ConditionFalse,
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

		// Lookup the userSignup one more and check the conditions are updated
		err = r.Client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
		require.NoError(t, err)
		require.Equal(t, userSignup.Status.CompliantUsername, mur.Name)

		assert.Equal(t, "approved", userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey])
		AssertMetricsCounterEquals(t, 1, metrics.UserSignupApprovedTotal)
		AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)

		test.AssertConditionsMatch(t, userSignup.Status.Conditions,
			toolchainv1alpha1.Condition{
				Type:   toolchainv1alpha1.UserSignupApproved,
				Status: v1.ConditionTrue,
				Reason: "ApprovedAutomatically",
			},
			toolchainv1alpha1.Condition{
				Type:   toolchainv1alpha1.UserSignupComplete,
				Status: v1.ConditionTrue,
			},
			toolchainv1alpha1.Condition{
				Type:   toolchainv1alpha1.UserSignupUserDeactivatingNotificationCreated,
				Status: v1.ConditionFalse,
				Reason: "UserNotInPreDeactivation",
			},
			toolchainv1alpha1.Condition{
				Type:   toolchainv1alpha1.UserSignupUserDeactivatedNotificationCreated,
				Status: v1.ConditionFalse,
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
}

func TestUserSignupWithMissingApprovalPolicyTreatedAsManual(t *testing.T) {
	// given
	userSignup := NewUserSignup(WithTargetCluster("east"))

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
			Status: v1.ConditionFalse,
			Reason: "PendingApproval",
		},
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupComplete,
			Status: v1.ConditionFalse,
			Reason: "PendingApproval",
		},
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupUserDeactivatingNotificationCreated,
			Status: v1.ConditionFalse,
			Reason: "UserNotInPreDeactivation",
		},
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupUserDeactivatedNotificationCreated,
			Status: v1.ConditionFalse,
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

func TestUserSignupMURCreateFails(t *testing.T) {
	// given
	userSignup := NewUserSignup(Approved())

	ready := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue))
	r, req, fakeClient := prepareReconcile(t, userSignup.Name, ready, userSignup, baseNSTemplateTier)
	InitializeCounters(t, NewToolchainStatus(
		WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
			string(metrics.External): 1,
		}),
		WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
			"1,external": 1,
		}),
	))

	fakeClient.MockCreate = func(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
		switch obj.(type) {
		case *toolchainv1alpha1.MasterUserRecord:
			return errors.New("unable to create mur")
		default:
			return fakeClient.Create(ctx, obj)
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
			"1,internal": 1,
		})

	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)
	assert.Equal(t, "approved", userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey])
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupApprovedTotal)
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)

}

func TestUserSignupMURReadFails(t *testing.T) {
	// given
	userSignup := NewUserSignup(Approved())

	ready := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue))
	r, req, fakeClient := prepareReconcile(t, userSignup.Name, ready, userSignup)
	InitializeCounters(t, NewToolchainStatus(
		WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
			string(metrics.External): 1,
		}),
		WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
			"1,external": 1,
		}),
	))

	fakeClient.MockGet = func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
		switch obj.(type) {
		case *toolchainv1alpha1.MasterUserRecord:
			return errors.New("failed to lookup MUR")
		default:
			return fakeClient.Client.Get(ctx, key, obj)
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
			"1,internal": 1,
		})

	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)
	assert.Equal(t, "approved", userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey])
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupApprovedTotal)
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)

}

func TestUserSignupSetStatusApprovedByAdminFails(t *testing.T) {
	// given
	userSignup := NewUserSignup(Approved())
	userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey] = "approved"

	ready := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue))
	r, req, fakeClient := prepareReconcile(t, userSignup.Name, ready, userSignup)
	InitializeCounters(t, NewToolchainStatus(
		WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
			string(metrics.External): 1,
		}),
		WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
			"1,external": 1,
		}),
	))

	fakeClient.MockStatusUpdate = func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
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
	userSignup := NewUserSignup()

	ready := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue))
	r, req, fakeClient := prepareReconcile(t, userSignup.Name, ready, userSignup, commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true)))
	InitializeCounters(t, NewToolchainStatus(
		WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
			string(metrics.External): 1,
		}),
		WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
			"1,external": 1,
		}),
	))

	fakeClient.MockStatusUpdate = func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
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
	assert.Equal(t, "not-ready", userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey])
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)
	assert.Empty(t, userSignup.Status.Conditions)

}

func TestUserSignupSetStatusNoClustersAvailableFails(t *testing.T) {
	// given
	userSignup := NewUserSignup()

	r, req, fakeClient := prepareReconcile(t, userSignup.Name, NewGetMemberClusters(), userSignup, commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true)))
	InitializeCounters(t, NewToolchainStatus(
		WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
			string(metrics.External): 1,
		}),
		WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
			"1,external": 1,
		}),
	))

	fakeClient.MockStatusUpdate = func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
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
	userSignup := NewUserSignup()
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

	ready := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue))
	r, req, _ := prepareReconcile(t, userSignup.Name, ready, userSignup, mur, commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true)), baseNSTemplateTier)
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
			Status: v1.ConditionTrue,
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
	userSignup := NewUserSignup(Approved())

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

	ready := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue))
	r, req, _ := prepareReconcile(t, userSignup.Name, ready, userSignup, mur, commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true)), baseNSTemplateTier)
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

		require.Equal(t, "foo-2", instance.Status.CompliantUsername)

		// Confirm that the mur exists
		mur = &toolchainv1alpha1.MasterUserRecord{}
		err = r.Client.Get(context.TODO(), types.NamespacedName{Namespace: test.HostOperatorNs, Name: instance.Status.CompliantUsername}, mur)
		require.NoError(t, err)
		require.Equal(t, instance.Name, mur.Labels[toolchainv1alpha1.MasterUserRecordOwnerLabelKey])

		var cond *toolchainv1alpha1.Condition
		for _, condition := range instance.Status.Conditions {
			if condition.Type == toolchainv1alpha1.UserSignupComplete {
				cond = &condition
			}
		}

		require.Equal(t, mur.Name, instance.Status.CompliantUsername)
		require.NotNil(t, cond)
		require.Equal(t, v1.ConditionTrue, cond.Status)
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
}

func TestUserSignupWithSpecialCharOK(t *testing.T) {
	// given
	userSignup := NewUserSignup(WithUsername("foo#$%^bar@redhat.com"))

	ready := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue))
	r, req, _ := prepareReconcile(t, userSignup.Name, ready, userSignup, commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true)), baseNSTemplateTier)
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
}

func TestUserSignupDeactivatedAfterMURCreated(t *testing.T) {
	// given
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	userSignup := NewUserSignup(Deactivated())
	userSignup.Status = toolchainv1alpha1.UserSignupStatus{
		Conditions: []toolchainv1alpha1.Condition{
			{
				Type:   toolchainv1alpha1.UserSignupComplete,
				Status: v1.ConditionTrue,
			},
			{
				Type:   toolchainv1alpha1.UserSignupApproved,
				Status: v1.ConditionTrue,
				Reason: "ApprovedAutomatically",
			},
		},
		CompliantUsername: "john-doe",
	}

	userSignup.Labels["toolchain.dev.openshift.com/approved"] = "true"
	key := test.NamespacedName(test.HostOperatorNs, userSignup.Name)

	t.Run("when MUR exists, then it should be deleted", func(t *testing.T) {
		// given
		mur := murtest.NewMasterUserRecord(t, "john-doe", murtest.MetaNamespace(test.HostOperatorNs))
		mur.Labels = map[string]string{toolchainv1alpha1.MasterUserRecordOwnerLabelKey: userSignup.Name}

		r, req, _ := prepareReconcile(t, userSignup.Name, NewGetMemberClusters(), userSignup, mur,
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
		assert.Equal(t, "deactivated", userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey])
		AssertMetricsCounterEquals(t, 1, metrics.UserSignupDeactivatedTotal)
		AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal) // 0 because usersignup was originally deactivated
		AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)   // 1 because state was initially empty

		// Confirm the status is now set to Deactivating
		test.AssertConditionsMatch(t, userSignup.Status.Conditions,
			toolchainv1alpha1.Condition{
				Type:   toolchainv1alpha1.UserSignupApproved,
				Status: v1.ConditionTrue,
				Reason: "ApprovedAutomatically",
			},
			toolchainv1alpha1.Condition{
				Type:   toolchainv1alpha1.UserSignupComplete,
				Status: v1.ConditionFalse,
				Reason: "DeactivationInProgress",
			})

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
		AssertMetricsCounterEquals(t, 0, metrics.UserSignupDeactivatedTotal) // zero because state didn't change
		AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
		AssertMetricsCounterEquals(t, 0, metrics.UserSignupUniqueTotal)

		// Confirm the status has been set to Deactivated
		test.AssertConditionsMatch(t, userSignup.Status.Conditions,
			toolchainv1alpha1.Condition{
				Type:   toolchainv1alpha1.UserSignupApproved,
				Status: v1.ConditionTrue,
				Reason: "ApprovedAutomatically",
			},
			toolchainv1alpha1.Condition{
				Type:   toolchainv1alpha1.UserSignupComplete,
				Status: v1.ConditionTrue,
				Reason: "Deactivated",
			},
			toolchainv1alpha1.Condition{
				Type:   toolchainv1alpha1.UserSignupUserDeactivatedNotificationCreated,
				Status: v1.ConditionTrue,
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
		userSignup2 := NewUserSignup(Deactivated())
		userSignup2.Status = toolchainv1alpha1.UserSignupStatus{
			Conditions: []toolchainv1alpha1.Condition{
				{
					Type:   toolchainv1alpha1.UserSignupComplete,
					Status: v1.ConditionTrue,
				},
				{
					Type:   toolchainv1alpha1.UserSignupApproved,
					Status: v1.ConditionTrue,
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
				Status: v1.ConditionTrue,
				Reason: "ApprovedAutomatically",
			},
			toolchainv1alpha1.Condition{
				Type:   toolchainv1alpha1.UserSignupComplete,
				Status: v1.ConditionTrue,
				Reason: "Deactivated",
			},
			toolchainv1alpha1.Condition{
				Type:   toolchainv1alpha1.UserSignupUserDeactivatedNotificationCreated,
				Status: v1.ConditionTrue,
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
	userSignup := &toolchainv1alpha1.UserSignup{
		ObjectMeta: NewUserSignupObjectMeta("", "john.doe@redhat.com"),
		Spec: toolchainv1alpha1.UserSignupSpec{
			Userid:   "UserID123",
			Username: "john.doe@redhat.com",
			States:   []toolchainv1alpha1.UserSignupState{toolchainv1alpha1.UserSignupStateDeactivated},
		},
		Status: toolchainv1alpha1.UserSignupStatus{
			Conditions: []toolchainv1alpha1.Condition{
				{
					Type:   toolchainv1alpha1.UserSignupComplete,
					Status: v1.ConditionTrue,
				},
				{
					Type:   toolchainv1alpha1.UserSignupApproved,
					Status: v1.ConditionTrue,
					Reason: "ApprovedAutomatically",
				},
			},
			CompliantUsername: "john-doe",
		},
	}
	userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey] = toolchainv1alpha1.NotificationTypeDeactivated
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

		fakeClient.MockCreate = func(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
			switch obj.(type) {
			case *toolchainv1alpha1.Notification:
				return errors.New("unable to create deactivation notification")
			default:
				return fakeClient.Create(ctx, obj)
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
				Status: v1.ConditionTrue,
				Reason: "ApprovedAutomatically",
			},
			toolchainv1alpha1.Condition{
				Type:   toolchainv1alpha1.UserSignupComplete,
				Status: v1.ConditionTrue,
			},
			toolchainv1alpha1.Condition{
				Type:    toolchainv1alpha1.UserSignupUserDeactivatedNotificationCreated,
				Status:  v1.ConditionFalse,
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
		assert.Equal(t, "deactivated", userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey])
		AssertMetricsCounterEquals(t, 0, metrics.UserSignupDeactivatedTotal) // zero because state didn't change
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
	userSignup := &toolchainv1alpha1.UserSignup{
		ObjectMeta: NewUserSignupObjectMeta("", "john.doe@redhat.com"),
		Spec: toolchainv1alpha1.UserSignupSpec{
			Userid:   "UserID123",
			Username: "john.doe@redhat.com",
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
				Status: v1.ConditionTrue,
				Reason: "Deactivated",
			},
			{
				Type:   toolchainv1alpha1.UserSignupApproved,
				Status: v1.ConditionTrue,
				Reason: "ApprovedAutomatically",
			},
			{
				Type:   toolchainv1alpha1.UserSignupUserDeactivatedNotificationCreated,
				Status: v1.ConditionTrue,
				Reason: "NotificationCRCreated",
			},
		}
		ready := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue))
		r, req, _ := prepareReconcile(t, userSignup.Name, ready, userSignup, commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true)), baseNSTemplateTier)
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
				Status: v1.ConditionTrue,
				Reason: "ApprovedAutomatically",
			},
			toolchainv1alpha1.Condition{
				Type:   toolchainv1alpha1.UserSignupComplete,
				Status: v1.ConditionTrue,
				Reason: "Deactivated",
			},
			toolchainv1alpha1.Condition{
				Type:   toolchainv1alpha1.UserSignupUserDeactivatingNotificationCreated,
				Status: v1.ConditionFalse,
				Reason: "UserNotInPreDeactivation",
			},
			toolchainv1alpha1.Condition{
				Type:   toolchainv1alpha1.UserSignupUserDeactivatedNotificationCreated,
				Status: v1.ConditionFalse,
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
				Status: v1.ConditionTrue,
				Reason: "Deactivated",
			},
			{
				Type:   toolchainv1alpha1.UserSignupApproved,
				Status: v1.ConditionTrue,
				Reason: "ApprovedAutomatically",
			},
			{
				Type:   toolchainv1alpha1.UserSignupUserDeactivatedNotificationCreated,
				Status: v1.ConditionTrue,
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

		fakeClient.MockStatusUpdate = func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
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
				Status: v1.ConditionTrue,
				Reason: "ApprovedAutomatically",
			},
			toolchainv1alpha1.Condition{
				Type:   toolchainv1alpha1.UserSignupComplete,
				Status: v1.ConditionTrue,
				Reason: "Deactivated",
			},
			toolchainv1alpha1.Condition{
				Type:   toolchainv1alpha1.UserSignupUserDeactivatedNotificationCreated,
				Status: v1.ConditionTrue,
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

func TestUserSignupDeactivatedWhenMURExists(t *testing.T) {
	// given
	userSignup := &toolchainv1alpha1.UserSignup{
		ObjectMeta: NewUserSignupObjectMeta("", "edward.jones@redhat.com"),
		Spec: toolchainv1alpha1.UserSignupSpec{
			Userid:   "UserID123",
			Username: "edward.jones@redhat.com",
			States:   []toolchainv1alpha1.UserSignupState{toolchainv1alpha1.UserSignupStateApproved},
		},
		Status: toolchainv1alpha1.UserSignupStatus{
			Conditions: []toolchainv1alpha1.Condition{
				{
					Type:   toolchainv1alpha1.UserSignupComplete,
					Status: v1.ConditionTrue,
					Reason: "",
				},
				{
					Type:   toolchainv1alpha1.UserSignupApproved,
					Status: v1.ConditionTrue,
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

	key := test.NamespacedName(test.HostOperatorNs, userSignup.Name)

	t.Run("when MUR exists and not deactivated, nothing should happen", func(t *testing.T) {
		r, req, _ := prepareReconcile(t, userSignup.Name, NewGetMemberClusters(), userSignup, mur, commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true)), baseNSTemplateTier)
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
				Status: v1.ConditionTrue,
				Reason: "ApprovedAutomatically",
			},
			toolchainv1alpha1.Condition{
				Type:   toolchainv1alpha1.UserSignupComplete,
				Status: v1.ConditionTrue,
				Reason: "",
			},
			toolchainv1alpha1.Condition{
				Type:   toolchainv1alpha1.UserSignupUserDeactivatedNotificationCreated,
				Status: v1.ConditionFalse,
				Reason: "UserIsActive",
			},
			toolchainv1alpha1.Condition{
				Type:   toolchainv1alpha1.UserSignupUserDeactivatingNotificationCreated,
				Status: v1.ConditionFalse,
				Reason: "UserNotInPreDeactivation",
			})

		// The MUR should have not been deleted
		murtest.AssertThatMasterUserRecords(t, r.Client).HaveCount(1)
		murtest.AssertThatMasterUserRecord(t, "edward-jones", r.Client).Exists()
	})

	t.Run("when UserSignup deactivated and MUR exists, then it should be deleted", func(t *testing.T) {

		// Given
		states.SetDeactivated(userSignup, true)

		r, req, _ := prepareReconcile(t, userSignup.Name, NewGetMemberClusters(), userSignup, mur, commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true)), baseNSTemplateTier)
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
			assert.Equal(t, "deactivated", userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey])
			AssertMetricsCounterEquals(t, 1, metrics.UserSignupDeactivatedTotal)
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupUniqueTotal)

			// Confirm the status is still set to Deactivating
			test.AssertConditionsMatch(t, userSignup.Status.Conditions,
				toolchainv1alpha1.Condition{
					Type:   toolchainv1alpha1.UserSignupApproved,
					Status: v1.ConditionTrue,
					Reason: "ApprovedAutomatically",
				},
				toolchainv1alpha1.Condition{
					Type:   toolchainv1alpha1.UserSignupComplete,
					Status: v1.ConditionFalse,
					Reason: "DeactivationInProgress",
				},
				toolchainv1alpha1.Condition{
					Type:   toolchainv1alpha1.UserSignupUserDeactivatingNotificationCreated,
					Status: v1.ConditionFalse,
					Reason: "UserNotInPreDeactivation",
				},
				toolchainv1alpha1.Condition{
					Type:   toolchainv1alpha1.UserSignupUserDeactivatedNotificationCreated,
					Status: v1.ConditionFalse,
					Reason: "UserIsActive",
				})

			// The MUR should have now been deleted
			murtest.AssertThatMasterUserRecords(t, r.Client).HaveCount(0)

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
					Status: v1.ConditionTrue,
					Reason: "ApprovedAutomatically",
				},
				toolchainv1alpha1.Condition{
					Type:   toolchainv1alpha1.UserSignupComplete,
					Status: v1.ConditionTrue,
					Reason: "Deactivated",
				},
				toolchainv1alpha1.Condition{
					Type:   toolchainv1alpha1.UserSignupUserDeactivatedNotificationCreated,
					Status: v1.ConditionTrue,
					Reason: "NotificationCRCreated",
				},
				toolchainv1alpha1.Condition{
					Type:   toolchainv1alpha1.UserSignupUserDeactivatingNotificationCreated,
					Status: v1.ConditionFalse,
					Reason: "UserNotInPreDeactivation",
				})
			// metrics should be the same after the 2nd reconcile
			AssertMetricsCounterEquals(t, 1, metrics.UserSignupDeactivatedTotal)
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupUniqueTotal)
		})
	})
}

func TestUserSignupDeactivatingNotificationCreated(t *testing.T) {
	userSignup := &toolchainv1alpha1.UserSignup{
		ObjectMeta: NewUserSignupObjectMeta("", "edward.jones@redhat.com"),
		Spec: toolchainv1alpha1.UserSignupSpec{
			Userid:   "UserID089",
			Username: "edward.jones@redhat.com",
			States:   []toolchainv1alpha1.UserSignupState{"deactivating"},
		},
		Status: toolchainv1alpha1.UserSignupStatus{
			Conditions: []toolchainv1alpha1.Condition{
				{
					Type:   toolchainv1alpha1.UserSignupComplete,
					Status: v1.ConditionTrue,
				},
				{
					Type:   toolchainv1alpha1.UserSignupApproved,
					Status: v1.ConditionTrue,
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
		commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true)), baseNSTemplateTier)
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
			Status: v1.ConditionTrue,
		},
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupApproved,
			Status: v1.ConditionTrue,
			Reason: "ApprovedAutomatically",
		},
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupUserDeactivatedNotificationCreated,
			Status: v1.ConditionFalse,
			Reason: "UserIsActive",
		},
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupUserDeactivatingNotificationCreated,
			Status: v1.ConditionTrue,
			Reason: "NotificationCRCreated",
		},
	)

	// Now let's pretend that the notification was created but the status failed to update.  We'll do this by modifying
	// the status back to original values
	userSignup.Status.Conditions = []toolchainv1alpha1.Condition{
		{
			Type:   toolchainv1alpha1.UserSignupComplete,
			Status: v1.ConditionTrue,
		},
		{
			Type:   toolchainv1alpha1.UserSignupApproved,
			Status: v1.ConditionTrue,
			Reason: "ApprovedAutomatically",
		},
	}

	// Prepare the reconciliation again, but this time include the notification that was previously created
	r, req, _ = prepareReconcile(t, userSignup.Name, NewGetMemberClusters(), userSignup, mur, &notifications.Items[0],
		commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true)), baseNSTemplateTier)

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
			Status: v1.ConditionTrue,
		},
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupApproved,
			Status: v1.ConditionTrue,
			Reason: "ApprovedAutomatically",
		},
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupUserDeactivatedNotificationCreated,
			Status: v1.ConditionFalse,
			Reason: "UserIsActive",
		},
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupUserDeactivatingNotificationCreated,
			Status: v1.ConditionTrue,
			Reason: "NotificationCRCreated",
		},
	)
}

func TestUserSignupBanned(t *testing.T) {
	// given
	userSignup := NewUserSignup()
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
			Status: v1.ConditionTrue,
			Reason: "Banned",
		})

	// Confirm that the MUR has now been deleted
	murtest.AssertThatMasterUserRecords(t, r.Client).HaveCount(0)

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
	userSignup := NewUserSignup(VerificationRequired(0))

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
	assert.Equal(t, "not-ready", userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey])
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupBannedTotal)
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupDeactivatedTotal)
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)

	// Confirm the status is set to VerificationRequired
	test.AssertConditionsMatch(t, userSignup.Status.Conditions,
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupComplete,
			Status: v1.ConditionFalse,
			Reason: "VerificationRequired",
		},
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupUserDeactivatingNotificationCreated,
			Status: v1.ConditionFalse,
			Reason: "UserNotInPreDeactivation",
		},
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupUserDeactivatedNotificationCreated,
			Status: v1.ConditionFalse,
			Reason: "UserIsActive",
		})

	// Confirm that no MUR is created
	murtest.AssertThatMasterUserRecords(t, r.Client).HaveCount(0)
	AssertThatCountersAndMetrics(t).
		HaveMasterUserRecordsPerDomain(toolchainv1alpha1.Metric{
			string(metrics.External): 1,
		}).
		HaveUsersPerActivationsAndDomain(toolchainv1alpha1.Metric{
			"1,external": 1,
		})
}

func TestUserSignupBannedMURExists(t *testing.T) {
	// given
	userSignup := NewUserSignup()
	userSignup.Status = toolchainv1alpha1.UserSignupStatus{
		Conditions: []toolchainv1alpha1.Condition{
			{
				Type:   toolchainv1alpha1.UserSignupComplete,
				Status: v1.ConditionTrue,
			},
			{
				Type:   toolchainv1alpha1.UserSignupApproved,
				Status: v1.ConditionTrue,
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

	r, req, _ := prepareReconcile(t, userSignup.Name, NewGetMemberClusters(), userSignup, mur, bannedUser, commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true)), baseNSTemplateTier)
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
			Status: v1.ConditionFalse,
			Reason: "Banning",
		},
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupApproved,
			Status: v1.ConditionTrue,
			Reason: "ApprovedAutomatically",
		})

	// Confirm that the MUR has now been deleted
	murtest.AssertThatMasterUserRecords(t, r.Client).HaveCount(0)

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
				Status: v1.ConditionTrue,
				Reason: "Banned",
			},
			toolchainv1alpha1.Condition{
				Type:   toolchainv1alpha1.UserSignupApproved,
				Status: v1.ConditionTrue,
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
	})
}

func TestUserSignupListBannedUsersFails(t *testing.T) {
	// given
	userSignup := NewUserSignup()

	r, req, fakeClient := prepareReconcile(t, userSignup.Name, NewGetMemberClusters(), userSignup, commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true)), baseNSTemplateTier)
	InitializeCounters(t, NewToolchainStatus(
		WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
			string(metrics.External): 1,
		}),
		WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
			"1,external": 1,
		}),
	))

	fakeClient.MockList = func(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
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
	// given
	userSignup := &toolchainv1alpha1.UserSignup{
		ObjectMeta: NewUserSignupObjectMeta("", "alice.mayweather.doe@redhat.com"),
		Spec: toolchainv1alpha1.UserSignupSpec{
			Userid:   "UserID123",
			Username: "alice.mayweather.doe@redhat.com",
			States:   []toolchainv1alpha1.UserSignupState{toolchainv1alpha1.UserSignupStateDeactivated},
		},
		Status: toolchainv1alpha1.UserSignupStatus{
			Conditions: []toolchainv1alpha1.Condition{
				{
					Type:   toolchainv1alpha1.UserSignupComplete,
					Status: v1.ConditionTrue,
				},
				{
					Type:   toolchainv1alpha1.UserSignupApproved,
					Status: v1.ConditionTrue,
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

	fakeClient.MockDelete = func(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
		switch obj.(type) {
		case *toolchainv1alpha1.MasterUserRecord:
			return errors.New("unable to delete mur")
		default:
			return fakeClient.Delete(ctx, obj)
		}
	}

	t.Run("first reconcile - status should show message about mur deletion failure", func(t *testing.T) {
		// when
		_, err := r.Reconcile(context.TODO(), req)
		require.Error(t, err)

		// then

		// Lookup the UserSignup
		err = r.Client.Get(context.TODO(), key, userSignup)
		require.NoError(t, err)
		assert.Equal(t, "deactivated", userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey])
		AssertMetricsCounterEquals(t, 1, metrics.UserSignupDeactivatedTotal)
		AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
		AssertMetricsCounterEquals(t, 0, metrics.UserSignupUniqueTotal)

		// Confirm the status is set to UnableToDeleteMUR
		test.AssertConditionsMatch(t, userSignup.Status.Conditions,
			toolchainv1alpha1.Condition{
				Type:   toolchainv1alpha1.UserSignupApproved,
				Status: v1.ConditionTrue,
				Reason: "ApprovedAutomatically",
			},
			toolchainv1alpha1.Condition{
				Type:    toolchainv1alpha1.UserSignupComplete,
				Status:  v1.ConditionFalse,
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

		t.Run("second reconcile - there should not be a notification created since the mur deletion failed even if reconciled again", func(t *testing.T) {
			_, err := r.Reconcile(context.TODO(), req)
			require.Error(t, err)
			ntest.AssertNoNotificationsExist(t, r.Client)
			// the metrics should be the same, deactivation should only be counted once
			AssertMetricsCounterEquals(t, 1, metrics.UserSignupDeactivatedTotal)
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupUniqueTotal)
		})
	})
}

// TestDeathBy100Signups tests the logic of generateCompliantUsername() which allows no more than 100 attempts to find a vacant name
func TestDeathBy100Signups(t *testing.T) {
	// given
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	userSignup := NewUserSignup(Approved())

	initObjs := make([]runtime.Object, 0, 110)
	initObjs = append(initObjs, userSignup)
	initObjs = append(initObjs, commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true)))

	// create 100 MURs that follow the naming pattern used by `generateCompliantUsername()`: `foo`, `foo-2`, ..., `foo-100`
	initObjs = append(initObjs, &toolchainv1alpha1.MasterUserRecord{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: test.HostOperatorNs,
			Labels:    map[string]string{toolchainv1alpha1.MasterUserRecordOwnerLabelKey: uuid.Must(uuid.NewV4()).String()},
		},
	})

	for i := 2; i <= 100; i++ {
		initObjs = append(initObjs, &toolchainv1alpha1.MasterUserRecord{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("foo-%d", i),
				Namespace: test.HostOperatorNs,
				Labels:    map[string]string{toolchainv1alpha1.MasterUserRecordOwnerLabelKey: uuid.Must(uuid.NewV4()).String()},
			},
		})
	}

	initObjs = append(initObjs, baseNSTemplateTier)

	ready := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue))
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
	assert.EqualError(t, err, "Error generating compliant username for foo@redhat.com: unable to transform username [foo@redhat.com] even after 100 attempts")
	require.Equal(t, reconcile.Result{}, res)

	// Lookup the user signup again
	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)
	assert.Equal(t, "approved", userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey])
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupDeactivatedTotal)
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupApprovedTotal)
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)

	test.AssertConditionsMatch(t, userSignup.Status.Conditions,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupComplete,
			Status:  v1.ConditionFalse,
			Reason:  "UnableToCreateMUR",
			Message: "unable to transform username [foo@redhat.com] even after 100 attempts",
		},
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupApproved,
			Status: v1.ConditionTrue,
			Reason: "ApprovedByAdmin",
		},
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupUserDeactivatingNotificationCreated,
			Status: v1.ConditionFalse,
			Reason: "UserNotInPreDeactivation",
		},
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupUserDeactivatedNotificationCreated,
			Status: v1.ConditionFalse,
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
}

func TestUserSignupWithMultipleExistingMURNotOK(t *testing.T) {
	// given
	userSignup := NewUserSignup()

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

	ready := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue))
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
			Status:  v1.ConditionFalse,
			Reason:  "InvalidMURState",
			Message: "multiple matching MasterUserRecord resources found",
		},
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupUserDeactivatingNotificationCreated,
			Status: v1.ConditionFalse,
			Reason: "UserNotInPreDeactivation",
		},
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupUserDeactivatedNotificationCreated,
			Status: v1.ConditionFalse,
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

func TestManuallyApprovedUserSignupWhenNoMembersAvailable(t *testing.T) {
	// given
	userSignup := NewUserSignup(Approved())

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
			Status: v1.ConditionTrue,
			Reason: "ApprovedByAdmin",
		},
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupComplete,
			Status:  v1.ConditionFalse,
			Reason:  "NoClusterAvailable",
			Message: "no suitable member cluster found - capacity was reached",
		},
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupUserDeactivatingNotificationCreated,
			Status: v1.ConditionFalse,
			Reason: "UserNotInPreDeactivation",
		},
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupUserDeactivatedNotificationCreated,
			Status: v1.ConditionFalse,
			Reason: "UserIsActive",
		})

}

func prepareReconcile(t *testing.T, name string, getMemberClusters cluster.GetMemberClustersFunc, initObjs ...runtime.Object) (*Reconciler, reconcile.Request, *test.FakeClient) {
	metrics.Reset()

	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)

	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret",
			Namespace: "test-namespace",
		},
		Type: v1.SecretTypeOpaque,
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
		Scheme:            s,
		GetMemberClusters: getMemberClusters,
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
		userSignup := NewUserSignup(Approved(), WithTargetCluster("east"))
		userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey] = "not-ready"

		for _, name := range names {
			userSignup.Spec.Username = fmt.Sprintf("%s%s", prefix, name)

			r, req, _ := prepareReconcile(t, userSignup.Name, NewGetMemberClusters(), userSignup, baseNSTemplateTier)
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
		userSignup := NewUserSignup(Approved(), WithTargetCluster("east"))
		userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey] = "not-ready"

		for _, name := range names {
			userSignup.Spec.Username = fmt.Sprintf("%s%s", name, suffix)

			r, req, _ := prepareReconcile(t, userSignup.Name, NewGetMemberClusters(), userSignup, baseNSTemplateTier)
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

// Test the scenario where the existing usersignup CompliantUsername becomes outdated eg. transformUsername func is changed
func TestChangedCompliantUsername(t *testing.T) {
	// starting with a UserSignup that exists and was approved and has the now outdated CompliantUsername
	userSignup := NewUserSignup(Approved(), WithTargetCluster("east"))
	userSignup.Status = toolchainv1alpha1.UserSignupStatus{
		Conditions: []toolchainv1alpha1.Condition{
			{
				Type:   toolchainv1alpha1.UserSignupApproved,
				Status: v1.ConditionTrue,
				Reason: toolchainv1alpha1.UserSignupApprovedByAdminReason,
			},
			{
				Status: v1.ConditionTrue,
				Type:   toolchainv1alpha1.UserSignupComplete,
			},
		},
		CompliantUsername: "foo-old",
	}

	// also starting with the old MUR whose name matches the outdated UserSignup CompliantUsername
	oldMur := &toolchainv1alpha1.MasterUserRecord{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo-old",
			Namespace: test.HostOperatorNs,
			Labels:    map[string]string{toolchainv1alpha1.MasterUserRecordOwnerLabelKey: userSignup.Name},
		},
	}

	// create the initial resources
	r, req, _ := prepareReconcile(t, userSignup.Name, NewGetMemberClusters(), userSignup, oldMur, baseNSTemplateTier)
	InitializeCounters(t, NewToolchainStatus(
		WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
			string(metrics.External): 1,
		}),
	))

	// 1st reconcile should effectively be a no op because the MUR name and UserSignup CompliantUsername match and status is all good
	res, err := r.Reconcile(context.TODO(), req)
	require.NoError(t, err)
	require.Equal(t, reconcile.Result{}, res)

	// after the 1st reconcile verify that the MUR still exists and its name still matches the initial UserSignup CompliantUsername
	murtest.AssertThatMasterUserRecords(t, r.Client).HaveCount(1)
	murtest.AssertThatMasterUserRecord(t, "foo-old", r.Client).
		Exists().
		HasLabelWithValue(toolchainv1alpha1.MasterUserRecordOwnerLabelKey, userSignup.Name)
	require.Equal(t, userSignup.Status.CompliantUsername, "foo-old")

	// delete the old MUR to trigger creation of a new MUR using the new username
	err = r.Client.Delete(context.TODO(), oldMur)
	require.NoError(t, err)

	// 2nd reconcile should handle the deleted MUR and provision a new one
	res, err = r.Reconcile(context.TODO(), req)
	require.NoError(t, err)
	require.Equal(t, reconcile.Result{}, res)

	// verify the new MUR is provisioned
	murtest.AssertThatMasterUserRecords(t, r.Client).HaveCount(1)
	mur := murtest.AssertThatMasterUserRecord(t, "foo", r.Client).
		Exists().
		HasLabelWithValue(toolchainv1alpha1.MasterUserRecordOwnerLabelKey, userSignup.Name).
		HasUserAccounts(1).
		HasTier(*baseNSTemplateTier).
		Get()

	// lookup the userSignup and check the conditions are updated but the CompliantUsername is still the old one
	AssertThatUserSignup(t, req.Namespace, userSignup.Name, r.Client).HasCompliantUsername("foo-old")

	// 3rd reconcile should update the CompliantUsername on the UserSignup status
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
	userSignup := NewUserSignup(Approved(), WithTargetCluster("east"))
	mur := newMasterUserRecord(userSignup, "east", baseNSTemplateTier, "foo")

	// set NSLimit and NSTemplateSet to be empty
	mur.Spec.UserAccounts[0].Spec.NSTemplateSet = &toolchainv1alpha1.NSTemplateSetSpec{}
	mur.Spec.UserAccounts[0].Spec.NSLimit = ""

	expectedMur := mur.DeepCopy()
	expectedMur.Generation = 1
	expectedMur.ResourceVersion = "1000"
	expectedMur.Spec.TierName = "base"
	expectedMur.Spec.UserAccounts[0].Spec.NSTemplateSet = &toolchainv1alpha1.NSTemplateSetSpec{}
	expectedMur.Spec.UserAccounts[0].Spec.NSTemplateSet.TierName = "base"
	expectedMur.Spec.UserAccounts[0].Spec.NSLimit = "default"
	expectedMur.Spec.UserAccounts[0].Spec.NSTemplateSet.Namespaces = []toolchainv1alpha1.NSTemplateSetNamespace{
		{
			TemplateRef: "base-dev-123abc1",
		},
		{
			TemplateRef: "base-stage-123abc2",
		},
	}
	expectedMur.Spec.UserAccounts[0].Spec.NSTemplateSet.ClusterResources = &toolchainv1alpha1.NSTemplateSetClusterResources{
		TemplateRef: "base-clusterresources-654321b",
	}

	t.Run("add missing tierName and nsLimit fields", func(t *testing.T) {
		// given
		r, req, _ := prepareReconcile(t, userSignup.Name, NewGetMemberClusters(), userSignup, baseNSTemplateTier, mur)
		InitializeCounters(t, NewToolchainStatus())

		// when
		_, err := r.Reconcile(context.TODO(), req)
		// then verify that the MUR exists and is complete
		require.NoError(t, err)
		murtest.AssertThatMasterUserRecords(t, r.Client).HaveCount(1)
		murtest.AssertThatMasterUserRecord(t, expectedMur.Name, r.Client).Exists()
	})
}

func TestUpdateMetricsByState(t *testing.T) {
	t.Run("common state changes", func(t *testing.T) {
		t.Run("empty -> not-ready - increment UserSignupUniqueTotal", func(t *testing.T) {
			metrics.Reset()
			updateUserSignupMetricsByState("", "not-ready")
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupAutoDeactivatedTotal)
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupBannedTotal)
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupDeactivatedTotal)
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
			AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)
		})

		t.Run("not-ready -> pending - no change", func(t *testing.T) {
			metrics.Reset()
			updateUserSignupMetricsByState("not-ready", "pending")
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupAutoDeactivatedTotal)
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupBannedTotal)
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupDeactivatedTotal)
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupUniqueTotal)
		})

		t.Run("pending -> approved - increment UserSignupApprovedTotal", func(t *testing.T) {
			metrics.Reset()
			updateUserSignupMetricsByState("pending", "approved")
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupAutoDeactivatedTotal)
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupBannedTotal)
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupDeactivatedTotal)
			AssertMetricsCounterEquals(t, 1, metrics.UserSignupApprovedTotal)
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupUniqueTotal)
		})

		t.Run("approved -> deactivated - increment UserSignupDeactivatedTotal", func(t *testing.T) {
			metrics.Reset()
			updateUserSignupMetricsByState("approved", "deactivated")
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupAutoDeactivatedTotal)
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupBannedTotal)
			AssertMetricsCounterEquals(t, 1, metrics.UserSignupDeactivatedTotal)
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupUniqueTotal)
		})

		t.Run("deactivated -> banned - increment UserSignupBannedTotal", func(t *testing.T) {
			metrics.Reset()
			updateUserSignupMetricsByState("deactivated", "banned")
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupAutoDeactivatedTotal)
			AssertMetricsCounterEquals(t, 1, metrics.UserSignupBannedTotal)
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupDeactivatedTotal)
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupUniqueTotal)
		})
	})

	t.Run("uncommon state changes", func(t *testing.T) {
		t.Run("old value is not empty", func(t *testing.T) {
			metrics.Reset()
			updateUserSignupMetricsByState("any-value", "")
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupAutoDeactivatedTotal)
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupBannedTotal)
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupDeactivatedTotal)
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupUniqueTotal)
		})

		t.Run("new value is not-ready - no change", func(t *testing.T) {
			metrics.Reset()
			updateUserSignupMetricsByState("any-value", "not-ready")
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupAutoDeactivatedTotal)
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupBannedTotal)
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupDeactivatedTotal)
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupUniqueTotal)
		})

		t.Run("new value is not a valid state - no change", func(t *testing.T) {
			metrics.Reset()
			updateUserSignupMetricsByState("any-value", "x")
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupAutoDeactivatedTotal)
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupBannedTotal)
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupDeactivatedTotal)
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupUniqueTotal)
		})
	})
}

func TestUserSignupLastTargetClusterAnnotation(t *testing.T) {

	t.Run("last target cluster annotation is not initially set but added when mur is created", func(t *testing.T) {
		// given
		userSignup := NewUserSignup()
		members := NewGetMemberClusters(
			NewMemberCluster(t, "member1", v1.ConditionTrue),
			NewMemberCluster(t, "member2", v1.ConditionTrue))
		r, req, _ := prepareReconcile(t, userSignup.Name, members, userSignup, baseNSTemplateTier, commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true)))
		InitializeCounters(t, NewToolchainStatus())

		// when
		res, err := r.Reconcile(context.TODO(), req)

		// then
		require.NoError(t, err)
		assert.False(t, res.Requeue)
		AssertThatUserSignup(t, req.Namespace, userSignup.Name, r.Client).
			HasAnnotation(toolchainv1alpha1.UserSignupLastTargetClusterAnnotationKey, "member1")
		murtest.AssertThatMasterUserRecords(t, r.Client).HaveCount(1)
		murtest.AssertThatMasterUserRecord(t, "foo", r.Client).
			HasTargetCluster("member1")
	})

	t.Run("last target cluster annotation is set but cluster lacks capacity", func(t *testing.T) {
		// given
		userSignup := NewUserSignup()
		userSignup.Annotations[toolchainv1alpha1.UserSignupLastTargetClusterAnnotationKey] = "member2"
		members := NewGetMemberClusters(
			NewMemberCluster(t, "member1", v1.ConditionTrue),
			// member2 cluster lacks capacity because the prepareReconcile only sets up the resource consumption for member1 so member2 is automatically excluded
			NewMemberCluster(t, "member2", v1.ConditionTrue))
		r, req, _ := prepareReconcile(t, userSignup.Name, members, userSignup, baseNSTemplateTier, commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true)))
		InitializeCounters(t, NewToolchainStatus())

		// when
		res, err := r.Reconcile(context.TODO(), req)

		// then
		require.NoError(t, err)
		assert.False(t, res.Requeue)
		AssertThatUserSignup(t, req.Namespace, userSignup.Name, r.Client).HasAnnotation(toolchainv1alpha1.UserSignupLastTargetClusterAnnotationKey, "member1")
		murtest.AssertThatMasterUserRecords(t, r.Client).HaveCount(1)
		murtest.AssertThatMasterUserRecord(t, "foo", r.Client).HasTargetCluster("member1")
	})

	t.Run("last target cluster annotation is set and cluster has capacity", func(t *testing.T) {
		// given
		userSignup := NewUserSignup()
		userSignup.Annotations[toolchainv1alpha1.UserSignupLastTargetClusterAnnotationKey] = "member2"
		members := NewGetMemberClusters(
			NewMemberCluster(t, "member1", v1.ConditionTrue),
			NewMemberCluster(t, "member2", v1.ConditionTrue))
		r, req, _ := prepareReconcile(t, userSignup.Name, members, userSignup, baseNSTemplateTier, commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true)))
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
		murtest.AssertThatMasterUserRecord(t, "foo", r.Client).
			HasTargetCluster("member2")
	})

	t.Run("last target cluster annotation is set but cluster does not exist", func(t *testing.T) {
		// given
		userSignup := NewUserSignup()
		userSignup.Annotations[toolchainv1alpha1.UserSignupLastTargetClusterAnnotationKey] = "member2"
		members := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue))
		r, req, _ := prepareReconcile(t, userSignup.Name, members, userSignup, baseNSTemplateTier, commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true)))
		InitializeCounters(t, NewToolchainStatus())

		// when
		res, err := r.Reconcile(context.TODO(), req)

		// then
		require.NoError(t, err)
		assert.False(t, res.Requeue)
		AssertThatUserSignup(t, req.Namespace, userSignup.Name, r.Client).
			HasAnnotation(toolchainv1alpha1.UserSignupLastTargetClusterAnnotationKey, "member1")
		murtest.AssertThatMasterUserRecords(t, r.Client).HaveCount(1)
		murtest.AssertThatMasterUserRecord(t, "foo", r.Client).
			HasTargetCluster("member1")
	})

	t.Run("last target cluster annotation is not set initially and setting the annotation fails", func(t *testing.T) {
		// given
		userSignup := NewUserSignup()
		userSignupName := userSignup.Name
		members := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue))
		r, req, cl := prepareReconcile(t, userSignup.Name, members, userSignup, baseNSTemplateTier, commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true)))
		cl.MockUpdate = func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
			s, ok := obj.(*toolchainv1alpha1.UserSignup)
			if ok && s.Annotations[toolchainv1alpha1.UserSignupLastTargetClusterAnnotationKey] == "member1" {
				return fmt.Errorf("error")
			}
			return nil
		}
		cl.MockStatusUpdate = func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
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
