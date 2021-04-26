package usersignup

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	"github.com/codeready-toolchain/host-operator/pkg/configuration"
	"github.com/codeready-toolchain/host-operator/pkg/counter"
	"github.com/codeready-toolchain/host-operator/pkg/metrics"
	"github.com/codeready-toolchain/host-operator/pkg/templates/nstemplatetiers"
	. "github.com/codeready-toolchain/host-operator/test"
	ntest "github.com/codeready-toolchain/host-operator/test/notification"
	"github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	murtest "github.com/codeready-toolchain/toolchain-common/pkg/test/masteruserrecord"

	uuid "github.com/satori/go.uuid"
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

func newNsTemplateTier(tierName string, nsTypes ...string) *v1alpha1.NSTemplateTier {
	namespaces := make([]v1alpha1.NSTemplateTierNamespace, len(nsTypes))
	for i, nsType := range nsTypes {
		revision := fmt.Sprintf("123abc%d", i+1)
		namespaces[i] = v1alpha1.NSTemplateTierNamespace{
			TemplateRef: nstemplatetiers.NewTierTemplateName(tierName, nsType, revision),
		}
	}

	return &v1alpha1.NSTemplateTier{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: test.HostOperatorNs,
			Name:      tierName,
		},
		Spec: v1alpha1.NSTemplateTierSpec{
			Namespaces: namespaces,
			ClusterResources: &v1alpha1.NSTemplateTierClusterResources{
				TemplateRef: nstemplatetiers.NewTierTemplateName(tierName, "clusterresources", "654321b"),
			},
		},
	}
}

var baseNSTemplateTier = newNsTemplateTier("base", "dev", "stage")

func TestUserSignupCreateMUROk(t *testing.T) {

	logf.SetLogger(zap.Logger(true))
	for testname, userSignup := range map[string]*v1alpha1.UserSignup{
		"with valid activation annotation":   NewUserSignup(Approved(), WithTargetCluster("east"), WithStateLabel("not-ready"), WithAnnotation(v1alpha1.UserSignupActivationCounterAnnotationKey, "2")), // this is a returning user
		"with invalid activation annotation": NewUserSignup(Approved(), WithTargetCluster("east"), WithStateLabel("not-ready"), WithAnnotation(v1alpha1.UserSignupActivationCounterAnnotationKey, "?")), // annotation value is not an 'int'
		"without activation annotation":      NewUserSignup(Approved(), WithTargetCluster("east"), WithStateLabel("not-ready"), WithoutAnnotation(v1alpha1.UserSignupActivationCounterAnnotationKey)),   // no annotation on this user, so the value will not be incremented
	} {
		t.Run(testname, func(t *testing.T) {
			// given
			defer counter.Reset()
			r, req, _ := prepareReconcile(t, userSignup.Name, NewGetMemberClusters(), userSignup, baseNSTemplateTier)
			InitializeCounters(t, NewToolchainStatus(
				WithHost(WithMasterUserRecordCount(1)),
				WithMetric(v1alpha1.UsersPerActivationMetricKey, v1alpha1.Metric{
					"1": 0,
					"2": 1,
					"3": 0,
				})))

			// when
			res, err := r.Reconcile(req)

			// then verify that the MUR exists and is complete
			require.NoError(t, err)
			require.Equal(t, reconcile.Result{}, res)
			murs := &v1alpha1.MasterUserRecordList{}
			err = r.client.List(context.TODO(), murs)
			require.NoError(t, err)
			require.Len(t, murs.Items, 1)
			mur := murs.Items[0]
			require.Equal(t, test.HostOperatorNs, mur.Namespace)
			require.Equal(t, userSignup.Name, mur.Labels[v1alpha1.MasterUserRecordOwnerLabelKey])
			require.Len(t, mur.Spec.UserAccounts, 1)
			assert.Equal(t, "base", mur.Spec.UserAccounts[0].Spec.NSTemplateSet.TierName)
			assert.Equal(t, []v1alpha1.NSTemplateSetNamespace{
				{
					TemplateRef: "base-dev-123abc1",
				},
				{
					TemplateRef: "base-stage-123abc2",
				},
			}, mur.Spec.UserAccounts[0].Spec.NSTemplateSet.Namespaces)
			require.NotNil(t, mur.Spec.UserAccounts[0].Spec.NSTemplateSet.ClusterResources)
			assert.Equal(t, "base-clusterresources-654321b", mur.Spec.UserAccounts[0].Spec.NSTemplateSet.ClusterResources.TemplateRef)

			AssertMetricsCounterEquals(t, 0, metrics.UserSignupUniqueTotal) // zero because we started with a not-ready state instead of empty as per usual
			AssertMetricsCounterEquals(t, 1, metrics.UserSignupApprovedTotal)

			AssertThatCounters(t).HaveMasterUserRecords(2)
			actualUserSignup := &v1alpha1.UserSignup{}
			err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, actualUserSignup)
			require.NoError(t, err)
			assert.Equal(t, "approved", actualUserSignup.Labels[v1alpha1.UserSignupStateLabelKey])
			switch testname {
			case "with valid activation annotation":
				assert.Equal(t, "3", actualUserSignup.Annotations[v1alpha1.UserSignupActivationCounterAnnotationKey]) // annotation value is incremented
				AssertMetricsGaugeEquals(t, 0, metrics.UsersPerActivationGaugeVec.WithLabelValues("1"))               // unchanged
				AssertMetricsGaugeEquals(t, 0, metrics.UsersPerActivationGaugeVec.WithLabelValues("2"))               // decreased
				AssertMetricsGaugeEquals(t, 1, metrics.UsersPerActivationGaugeVec.WithLabelValues("3"))               // incresaed
			case "without activation annotation", "with invalid activation annotation":
				assert.Equal(t, "1", actualUserSignup.Annotations[v1alpha1.UserSignupActivationCounterAnnotationKey]) // annotation was set to "1" since it was missing
				AssertMetricsGaugeEquals(t, 1, metrics.UsersPerActivationGaugeVec.WithLabelValues("1"))               // incresaed
				AssertMetricsGaugeEquals(t, 1, metrics.UsersPerActivationGaugeVec.WithLabelValues("2"))               // unchanged
				AssertMetricsGaugeEquals(t, 0, metrics.UsersPerActivationGaugeVec.WithLabelValues("3"))               // unchanged
			default:
				assert.Fail(t, "unknown testcase")
			}
		})
	}
}

func TestUserSignupMigration(t *testing.T) {

	// check that the "toolchain.dev.openshift.com/activation-counter" annotation
	// is set to "1" on usersignups which are complete/true and for whom
	// the annotation was not already set

	// given
	logf.SetLogger(zap.Logger(true))

	for _, state := range []string{
		v1alpha1.UserSignupStateLabelValueApproved,
		v1alpha1.UserSignupStateLabelValueDeactivated,
		v1alpha1.UserSignupStateLabelValueBanned,
	} {
		t.Run(fmt.Sprintf("when user is %s", state), func(t *testing.T) {

			t.Run("when annotation is missing", func(t *testing.T) {
				defer counter.Reset()
				userSignup := NewUserSignup(SignupComplete(""), WithStateLabel(state))
				r, req, _ := prepareReconcile(t, userSignup.Name, NewGetMemberClusters(), userSignup, baseNSTemplateTier)
				InitializeCounters(t, NewToolchainStatus(
					WithHost(WithMasterUserRecordCount(1)),
					WithMetric(v1alpha1.UsersPerActivationMetricKey, v1alpha1.Metric{
						"1": 0,
						"2": 0,
						"3": 0,
					})))
				// when
				res, err := r.Reconcile(req)

				// then verify that the MUR exists and is complete
				require.NoError(t, err)
				require.Equal(t, reconcile.Result{}, res)
				actualUserSignup := &v1alpha1.UserSignup{}
				err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, actualUserSignup)
				require.NoError(t, err)
				assert.Equal(t, "1", actualUserSignup.Annotations[v1alpha1.UserSignupActivationCounterAnnotationKey]) // set
				AssertMetricsGaugeEquals(t, 1, metrics.UsersPerActivationGaugeVec.WithLabelValues("1"))               // set
				AssertMetricsGaugeEquals(t, 0, metrics.UsersPerActivationGaugeVec.WithLabelValues("2"))               // unchanged
				AssertMetricsGaugeEquals(t, 0, metrics.UsersPerActivationGaugeVec.WithLabelValues("3"))               // unchanged
			})

			t.Run("when annotation already exists", func(t *testing.T) {
				defer counter.Reset()
				userSignup := NewUserSignup(SignupComplete(""), WithStateLabel(state), WithAnnotation(v1alpha1.UserSignupActivationCounterAnnotationKey, "2"))
				r, req, _ := prepareReconcile(t, userSignup.Name, NewGetMemberClusters(), userSignup, baseNSTemplateTier)
				InitializeCounters(t, NewToolchainStatus(
					WithHost(WithMasterUserRecordCount(1)),
					WithMetric(v1alpha1.UsersPerActivationMetricKey, v1alpha1.Metric{
						"1": 0,
						"2": 1,
						"3": 0,
					})))
				// when
				res, err := r.Reconcile(req)

				// then verify that the MUR exists and is complete
				require.NoError(t, err)
				require.Equal(t, reconcile.Result{}, res)
				actualUserSignup := &v1alpha1.UserSignup{}
				err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, actualUserSignup)
				require.NoError(t, err)
				assert.Equal(t, "2", actualUserSignup.Annotations[v1alpha1.UserSignupActivationCounterAnnotationKey]) // unchanged
				AssertMetricsGaugeEquals(t, 0, metrics.UsersPerActivationGaugeVec.WithLabelValues("1"))               // unchanged
				AssertMetricsGaugeEquals(t, 1, metrics.UsersPerActivationGaugeVec.WithLabelValues("2"))               // unchanged
				AssertMetricsGaugeEquals(t, 0, metrics.UsersPerActivationGaugeVec.WithLabelValues("3"))               // unchanged

			})
		})
	}

	for _, state := range []string{
		v1alpha1.UserSignupStateLabelValueNotReady,
		v1alpha1.UserSignupStateLabelValuePending,
	} {
		t.Run(fmt.Sprintf("when user is %s", state), func(t *testing.T) {
			t.Run("when annotation is missing", func(t *testing.T) {
				defer counter.Reset()
				userSignup := NewUserSignup(VerificationRequired(), WithStateLabel(state))
				r, req, _ := prepareReconcile(t, userSignup.Name, NewGetMemberClusters(), userSignup, baseNSTemplateTier)
				InitializeCounters(t, NewToolchainStatus(
					WithHost(WithMasterUserRecordCount(1)),
					WithMetric(v1alpha1.UsersPerActivationMetricKey, v1alpha1.Metric{
						"1": 0,
						"2": 0,
						"3": 0,
					})))
				// when
				res, err := r.Reconcile(req)

				// then verify that the MUR exists and is complete
				require.NoError(t, err)
				require.Equal(t, reconcile.Result{}, res)
				actualUserSignup := &v1alpha1.UserSignup{}
				err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, actualUserSignup)
				require.NoError(t, err)
				assert.Empty(t, actualUserSignup.Annotations[v1alpha1.UserSignupActivationCounterAnnotationKey]) // unset
				AssertMetricsGaugeEquals(t, 0, metrics.UsersPerActivationGaugeVec.WithLabelValues("1"))          // unchanged
				AssertMetricsGaugeEquals(t, 0, metrics.UsersPerActivationGaugeVec.WithLabelValues("2"))          // unchanged
				AssertMetricsGaugeEquals(t, 0, metrics.UsersPerActivationGaugeVec.WithLabelValues("3"))          // unchanged
			})
		})
	}
}
func TestDeletingUserSignupShouldNotUpdateMetrics(t *testing.T) {
	// given
	logf.SetLogger(zap.Logger(true))
	userSignup := NewUserSignup(BeingDeleted(), WithTargetCluster("east"),
		WithStateLabel("not-ready"),
		WithAnnotation(v1alpha1.UserSignupActivationCounterAnnotationKey, "2"))
	r, req, _ := prepareReconcile(t, userSignup.Name, NewGetMemberClusters(), userSignup, baseNSTemplateTier)
	InitializeCounters(t, NewToolchainStatus(
		WithHost(WithMasterUserRecordCount(12)),
		WithMetric(v1alpha1.UsersPerActivationMetricKey, v1alpha1.Metric{
			"1": 1,
			"2": 10,
			"3": 1,
		})))

	// when
	_, err := r.Reconcile(req)

	// then
	require.NoError(t, err)

	// Verify the counters
	AssertThatCounters(t).HaveMasterUserRecords(12). // unchanged at this point
								HaveUsersPerActivations(v1alpha1.Metric{
			"1": 1,
			"2": 10, // unchanged
			"3": 1,
		})
	AssertMetricsGaugeEquals(t, 1, metrics.UsersPerActivationGaugeVec.WithLabelValues("1"))
	AssertMetricsGaugeEquals(t, 10, metrics.UsersPerActivationGaugeVec.WithLabelValues("2")) // unchanged
	AssertMetricsGaugeEquals(t, 1, metrics.UsersPerActivationGaugeVec.WithLabelValues("3"))
}

func TestUserSignupWithAutoApprovalWithoutTargetCluster(t *testing.T) {
	// given
	userSignup := NewUserSignup()

	ready := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue))
	r, req, _ := prepareReconcile(t, userSignup.Name, ready, userSignup, NewHostOperatorConfigWithReset(t, test.AutomaticApproval().Enabled()), baseNSTemplateTier)
	InitializeCounters(t, NewToolchainStatus(WithHost(WithMasterUserRecordCount(1))))

	// when - The first reconcile creates the MasterUserRecord
	res, err := r.Reconcile(req)

	// then
	require.NoError(t, err)
	require.Equal(t, reconcile.Result{}, res)

	// Lookup the user signup again
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)
	assert.Equal(t, "approved", userSignup.Labels[v1alpha1.UserSignupStateLabelKey])
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupAutoDeactivatedTotal)
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupBannedTotal)
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupDeactivatedTotal)
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupApprovedTotal)
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)

	murs := &v1alpha1.MasterUserRecordList{}
	err = r.client.List(context.TODO(), murs)
	require.NoError(t, err)
	require.Len(t, murs.Items, 1)

	mur := murs.Items[0]
	require.Equal(t, test.HostOperatorNs, mur.Namespace)
	require.Equal(t, userSignup.Name, mur.Labels[v1alpha1.MasterUserRecordOwnerLabelKey])
	require.Len(t, mur.Spec.UserAccounts, 1)
	assert.Equal(t, "base", mur.Spec.UserAccounts[0].Spec.NSTemplateSet.TierName)
	require.Len(t, mur.Spec.UserAccounts[0].Spec.NSTemplateSet.Namespaces, 2)
	assert.Contains(t, mur.Spec.UserAccounts[0].Spec.NSTemplateSet.Namespaces,
		v1alpha1.NSTemplateSetNamespace{
			TemplateRef: "base-dev-123abc1",
			Template:    "",
		})
	assert.Contains(t, mur.Spec.UserAccounts[0].Spec.NSTemplateSet.Namespaces,
		v1alpha1.NSTemplateSetNamespace{
			TemplateRef: "base-stage-123abc2",
			Template:    "",
		})
	require.NotNil(t, mur.Spec.UserAccounts[0].Spec.NSTemplateSet.ClusterResources)
	assert.Equal(t, "base-clusterresources-654321b", mur.Spec.UserAccounts[0].Spec.NSTemplateSet.ClusterResources.TemplateRef)

	test.AssertConditionsMatch(t, userSignup.Status.Conditions,
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupApproved,
			Status: v1.ConditionTrue,
			Reason: "ApprovedAutomatically",
		},
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupUserDeactivatedNotificationCreated,
			Status: v1.ConditionFalse,
			Reason: "UserIsActive",
		})

	AssertThatCounters(t).HaveMasterUserRecords(2)

	t.Run("second reconcile", func(t *testing.T) {
		// when
		res, err = r.Reconcile(req)

		// then
		require.NoError(t, err)
		require.Equal(t, reconcile.Result{}, res)

		// Lookup the userSignup one more and check the conditions are updated
		err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
		require.NoError(t, err)
		require.Equal(t, userSignup.Status.CompliantUsername, mur.Name)
		assert.Equal(t, "approved", userSignup.Labels[v1alpha1.UserSignupStateLabelKey])

		test.AssertConditionsMatch(t, userSignup.Status.Conditions,
			v1alpha1.Condition{
				Type:   v1alpha1.UserSignupApproved,
				Status: v1.ConditionTrue,
				Reason: "ApprovedAutomatically",
			},
			v1alpha1.Condition{
				Type:   v1alpha1.UserSignupComplete,
				Status: v1.ConditionTrue,
			},
			v1alpha1.Condition{
				Type:   v1alpha1.UserSignupUserDeactivatedNotificationCreated,
				Status: v1.ConditionFalse,
				Reason: "UserIsActive",
			})
	})
	AssertThatCounters(t).HaveMasterUserRecords(2)
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupAutoDeactivatedTotal)
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupBannedTotal)
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupDeactivatedTotal)
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupApprovedTotal)
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)
}

func TestUserSignupWithMissingEmailAnnotationFails(t *testing.T) {
	// given
	logf.SetLogger(zap.Logger(true))
	userSignup := NewUserSignup(WithoutAnnotations())

	ready := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue))
	r, req, _ := prepareReconcile(t, userSignup.Name, ready, userSignup,
		NewHostOperatorConfigWithReset(t, test.AutomaticApproval().Enabled()), baseNSTemplateTier)
	InitializeCounters(t, NewToolchainStatus(WithHost(WithMasterUserRecordCount(1))))

	// when
	_, err := r.Reconcile(req)

	// then
	require.Error(t, err)

	// Lookup the user signup again
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)
	assert.Equal(t, "not-ready", userSignup.Labels[v1alpha1.UserSignupStateLabelKey])
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)
	test.AssertConditionsMatch(t, userSignup.Status.Conditions,
		v1alpha1.Condition{
			Type:    v1alpha1.UserSignupComplete,
			Status:  v1.ConditionFalse,
			Reason:  "MissingUserEmailAnnotation",
			Message: "missing annotation at usersignup",
		})
	AssertThatCounters(t).HaveMasterUserRecords(1)
}

func TestUserSignupWithInvalidEmailHashLabelFails(t *testing.T) {
	// given
	defer counter.Reset()
	userSignup := NewUserSignup(
		WithLabel(v1alpha1.UserSignupUserEmailHashLabelKey, "abcdef0123456789"),
		WithLabel("toolchain.dev.openshift.com/approved", "false"),
		WithAnnotation(v1alpha1.UserSignupUserEmailAnnotationKey, "foo@redhat.com"),
	)

	ready := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue))
	r, req, _ := prepareReconcile(t, userSignup.Name, ready, userSignup, NewHostOperatorConfigWithReset(t, test.AutomaticApproval().Enabled()), baseNSTemplateTier)
	InitializeCounters(t, NewToolchainStatus(WithHost(WithMasterUserRecordCount(1))))

	// when
	_, err := r.Reconcile(req)

	// then
	require.Error(t, err)

	// Lookup the user signup again
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)
	assert.Equal(t, "not-ready", userSignup.Labels[v1alpha1.UserSignupStateLabelKey])
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)
	test.AssertConditionsMatch(t, userSignup.Status.Conditions,
		v1alpha1.Condition{
			Type:    v1alpha1.UserSignupComplete,
			Status:  v1.ConditionFalse,
			Reason:  "InvalidEmailHashLabel",
			Message: "hash is invalid",
		})
	AssertThatCounters(t).HaveMasterUserRecords(1)
}

func TestUpdateOfApprovedLabelFails(t *testing.T) {
	// given
	userSignup := NewUserSignup()

	ready := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue))
	r, req, fakeClient := prepareReconcile(t, userSignup.Name, ready, userSignup, NewHostOperatorConfigWithReset(t, test.AutomaticApproval().Enabled()), baseNSTemplateTier)
	fakeClient.MockUpdate = func(ctx context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
		return fmt.Errorf("some error")
	}
	InitializeCounters(t, NewToolchainStatus(
		WithHost(WithMasterUserRecordCount(1)),
		WithMetric(v1alpha1.UsersPerActivationMetricKey, v1alpha1.Metric{
			"1": 0, // no user approved yet
		})))

	// when
	_, err := r.Reconcile(req)

	// then
	require.Error(t, err)

	// Lookup the user signup again
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)
	test.AssertConditionsMatch(t, userSignup.Status.Conditions,
		v1alpha1.Condition{
			Type:    v1alpha1.UserSignupComplete,
			Status:  v1.ConditionFalse,
			Reason:  "UnableToUpdateStateLabel",
			Message: "some error",
		})
	AssertThatCounters(t).HaveMasterUserRecords(1).
		HaveUsersPerActivations(v1alpha1.Metric{
			"1": 0, // unchanged
		})
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupUniqueTotal)
	AssertMetricsGaugeEquals(t, 0, metrics.UsersPerActivationGaugeVec.WithLabelValues("1")) // unchanged
}

func TestUserSignupWithMissingEmailHashLabelFails(t *testing.T) {
	// given
	userSignup := NewUserSignup()
	userSignup.Annotations = map[string]string{
		v1alpha1.UserSignupUserEmailAnnotationKey: "foo@redhat.com",
	}
	userSignup.Labels = map[string]string{"toolchain.dev.openshift.com/approved": "false"}

	ready := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue))
	r, req, _ := prepareReconcile(t, userSignup.Name, ready, userSignup, NewHostOperatorConfigWithReset(t, test.AutomaticApproval().Enabled()), baseNSTemplateTier)
	InitializeCounters(t, NewToolchainStatus(WithHost(WithMasterUserRecordCount(1))))

	// when
	_, err := r.Reconcile(req)

	// then
	require.Error(t, err)

	// Lookup the user signup again
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)
	assert.Equal(t, "not-ready", userSignup.Labels[v1alpha1.UserSignupStateLabelKey])
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)
	test.AssertConditionsMatch(t, userSignup.Status.Conditions,
		v1alpha1.Condition{
			Type:    v1alpha1.UserSignupComplete,
			Status:  v1.ConditionFalse,
			Reason:  "MissingEmailHashLabel",
			Message: "missing label at usersignup",
		})
	AssertThatCounters(t).HaveMasterUserRecords(1)
}

func TestUserSignupFailedMissingNSTemplateTier(t *testing.T) {
	// given
	userSignup := NewUserSignup()
	ready := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue))
	r, req, _ := prepareReconcile(t, userSignup.Name, ready, userSignup, NewHostOperatorConfigWithReset(t, test.AutomaticApproval().Enabled())) // baseNSTemplateTier does not exist
	InitializeCounters(t, NewToolchainStatus(WithHost(WithMasterUserRecordCount(1))))

	// when
	_, err := r.Reconcile(req)

	// then
	// error reported, and request is requeued and userSignup status was updated
	require.Error(t, err)
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)
	t.Logf("usersignup status: %+v", userSignup.Status)
	test.AssertConditionsMatch(t, userSignup.Status.Conditions,
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupApproved,
			Status: v1.ConditionTrue,
			Reason: "ApprovedAutomatically",
		},
		v1alpha1.Condition{
			Type:    v1alpha1.UserSignupComplete,
			Status:  v1.ConditionFalse,
			Reason:  "NoTemplateTierAvailable",
			Message: "nstemplatetiers.toolchain.dev.openshift.com \"base\" not found",
		},
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupUserDeactivatedNotificationCreated,
			Status: v1.ConditionFalse,
			Reason: "UserIsActive",
		})
	assert.Equal(t, "approved", userSignup.Labels[v1alpha1.UserSignupStateLabelKey])
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupApprovedTotal)
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)
	AssertThatCounters(t).HaveMasterUserRecords(1)
}

func TestUnapprovedUserSignupWhenNoClusterReady(t *testing.T) {
	// given
	userSignup := NewUserSignup()

	notReady := NewGetMemberClusters(
		NewMemberCluster(t, "member1", v1.ConditionFalse),
		NewMemberCluster(t, "member2", v1.ConditionFalse))
	config := NewHostOperatorConfigWithReset(t, test.AutomaticApproval().Enabled().MaxUsersNumber(1))
	r, req, _ := prepareReconcile(t, userSignup.Name, notReady, userSignup, config, baseNSTemplateTier)
	InitializeCounters(t, NewToolchainStatus(WithHost(WithMasterUserRecordCount(2))))

	// when
	res, err := r.Reconcile(req)

	// then
	// it should not return an error but just wait for another reconcile triggered by updated ToolchainStatus
	require.NoError(t, err)
	assert.Equal(t, reconcile.Result{Requeue: false}, res)
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)
	t.Logf("usersignup status: %+v", userSignup.Status)
	test.AssertConditionsMatch(t, userSignup.Status.Conditions,
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupApproved,
			Status: v1.ConditionFalse,
			Reason: "PendingApproval",
		},
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupComplete,
			Status: v1.ConditionFalse,
			Reason: "NoClusterAvailable",
		},
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupUserDeactivatedNotificationCreated,
			Status: v1.ConditionFalse,
			Reason: "UserIsActive",
		})

	assert.Equal(t, "pending", userSignup.Labels[v1alpha1.UserSignupStateLabelKey])
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)

	AssertThatCounters(t).HaveMasterUserRecords(2)
}

func TestUserSignupFailedNoClusterWithCapacityAvailable(t *testing.T) {
	// given
	userSignup := NewUserSignup()
	noCapacity := NewGetMemberClusters(
		NewMemberCluster(t, "member1", v1.ConditionTrue),
		NewMemberCluster(t, "member2", v1.ConditionTrue))
	config := NewHostOperatorConfigWithReset(t, test.AutomaticApproval().Enabled().ResourceCapThreshold(60))
	r, req, _ := prepareReconcile(t, userSignup.Name, noCapacity, userSignup, config, baseNSTemplateTier)
	InitializeCounters(t, NewToolchainStatus(WithHost(WithMasterUserRecordCount(1))))

	// when
	res, err := r.Reconcile(req)

	// then
	// it should not return an error but just wait for another reconcile triggered by updated ToolchainStatus
	require.NoError(t, err)
	assert.Equal(t, reconcile.Result{Requeue: false}, res)
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)
	t.Logf("usersignup status: %+v", userSignup.Status)
	test.AssertConditionsMatch(t, userSignup.Status.Conditions,
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupApproved,
			Status: v1.ConditionFalse,
			Reason: "PendingApproval",
		},
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupComplete,
			Status: v1.ConditionFalse,
			Reason: "NoClusterAvailable",
		},
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupUserDeactivatedNotificationCreated,
			Status: v1.ConditionFalse,
			Reason: "UserIsActive",
		})

	assert.Equal(t, "pending", userSignup.Labels[v1alpha1.UserSignupStateLabelKey])
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)

	AssertThatCounters(t).HaveMasterUserRecords(1)
}

func TestUserSignupWithManualApprovalApproved(t *testing.T) {
	// given
	userSignup := NewUserSignup(Approved())

	ready := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue))
	r, req, _ := prepareReconcile(t, userSignup.Name, ready, userSignup, NewHostOperatorConfigWithReset(t, test.AutomaticApproval().Enabled()), baseNSTemplateTier)
	InitializeCounters(t, NewToolchainStatus(WithHost(WithMasterUserRecordCount(1))))

	// when
	res, err := r.Reconcile(req)

	// then
	require.NoError(t, err)
	require.Equal(t, reconcile.Result{}, res)

	// Lookup the userSignup again
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)
	assert.Equal(t, "approved", userSignup.Labels[v1alpha1.UserSignupStateLabelKey])
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupApprovedTotal)
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)

	murs := &v1alpha1.MasterUserRecordList{}
	err = r.client.List(context.TODO(), murs)
	require.NoError(t, err)
	require.Len(t, murs.Items, 1)

	mur := murs.Items[0]

	require.Equal(t, test.HostOperatorNs, mur.Namespace)
	require.Equal(t, userSignup.Name, mur.Labels[v1alpha1.MasterUserRecordOwnerLabelKey])
	require.Len(t, mur.Spec.UserAccounts, 1)

	test.AssertConditionsMatch(t, userSignup.Status.Conditions,
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupApproved,
			Status: v1.ConditionTrue,
			Reason: "ApprovedByAdmin",
		},
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupUserDeactivatedNotificationCreated,
			Status: v1.ConditionFalse,
			Reason: "UserIsActive",
		})
	AssertThatCounters(t).HaveMasterUserRecords(2)

	t.Run("second reconcile", func(t *testing.T) {
		// when
		res, err = r.Reconcile(req)

		// then
		require.NoError(t, err)
		require.Equal(t, reconcile.Result{}, res)

		// Lookup the userSignup one more time and check the conditions are updated
		err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
		require.NoError(t, err)
		require.Equal(t, userSignup.Status.CompliantUsername, mur.Name)
		assert.Equal(t, "approved", userSignup.Labels[v1alpha1.UserSignupStateLabelKey])
		AssertMetricsCounterEquals(t, 1, metrics.UserSignupApprovedTotal)
		AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)
		test.AssertConditionsMatch(t, userSignup.Status.Conditions,
			v1alpha1.Condition{
				Type:   v1alpha1.UserSignupApproved,
				Status: v1.ConditionTrue,
				Reason: "ApprovedByAdmin",
			},
			v1alpha1.Condition{
				Type:   v1alpha1.UserSignupComplete,
				Status: v1.ConditionTrue,
			},
			v1alpha1.Condition{
				Type:   v1alpha1.UserSignupUserDeactivatedNotificationCreated,
				Status: v1.ConditionFalse,
				Reason: "UserIsActive",
			})
		AssertThatCounters(t).HaveMasterUserRecords(2)
	})
}

func TestUserSignupWithNoApprovalPolicyTreatedAsManualApproved(t *testing.T) {
	// given
	userSignup := NewUserSignup(Approved())

	ready := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue))
	r, req, _ := prepareReconcile(t, userSignup.Name, ready, userSignup, baseNSTemplateTier)
	InitializeCounters(t, NewToolchainStatus(WithHost(WithMasterUserRecordCount(1))))

	// when
	res, err := r.Reconcile(req)

	// then
	require.NoError(t, err)
	require.Equal(t, reconcile.Result{}, res)

	// Lookup the userSignup again
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)
	assert.Equal(t, "approved", userSignup.Labels[v1alpha1.UserSignupStateLabelKey])
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupApprovedTotal)
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)

	murs := &v1alpha1.MasterUserRecordList{}
	err = r.client.List(context.TODO(), murs)
	require.NoError(t, err)
	require.Len(t, murs.Items, 1)

	mur := murs.Items[0]

	require.Equal(t, test.HostOperatorNs, mur.Namespace)
	require.Equal(t, userSignup.Name, mur.Labels[v1alpha1.MasterUserRecordOwnerLabelKey])
	require.Len(t, mur.Spec.UserAccounts, 1)

	test.AssertConditionsMatch(t, userSignup.Status.Conditions,
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupApproved,
			Status: v1.ConditionTrue,
			Reason: "ApprovedByAdmin",
		},
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupUserDeactivatedNotificationCreated,
			Status: v1.ConditionFalse,
			Reason: "UserIsActive",
		})
	AssertThatCounters(t).HaveMasterUserRecords(2)

	t.Run("second reconcile", func(t *testing.T) {
		// when
		res, err = r.Reconcile(req)

		// then
		require.NoError(t, err)
		require.Equal(t, reconcile.Result{}, res)

		// Lookup the userSignup one more and check the conditions are updated
		err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
		require.NoError(t, err)
		require.Equal(t, userSignup.Status.CompliantUsername, mur.Name)

		assert.Equal(t, "approved", userSignup.Labels[v1alpha1.UserSignupStateLabelKey])
		AssertMetricsCounterEquals(t, 1, metrics.UserSignupApprovedTotal)
		AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)

		test.AssertConditionsMatch(t, userSignup.Status.Conditions,
			v1alpha1.Condition{
				Type:   v1alpha1.UserSignupApproved,
				Status: v1.ConditionTrue,
				Reason: "ApprovedByAdmin",
			},
			v1alpha1.Condition{
				Type:   v1alpha1.UserSignupComplete,
				Status: v1.ConditionTrue,
			},
			v1alpha1.Condition{
				Type:   v1alpha1.UserSignupUserDeactivatedNotificationCreated,
				Status: v1.ConditionFalse,
				Reason: "UserIsActive",
			})
		AssertThatCounters(t).HaveMasterUserRecords(2)
	})
}

func TestUserSignupWithManualApprovalNotApproved(t *testing.T) {
	// given
	userSignup := NewUserSignup()

	ready := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue))
	r, req, _ := prepareReconcile(t, userSignup.Name, ready, userSignup, NewHostOperatorConfigWithReset(t), baseNSTemplateTier)
	InitializeCounters(t, NewToolchainStatus(WithHost(WithMasterUserRecordCount(1))))

	// when
	res, err := r.Reconcile(req)

	// then
	require.NoError(t, err)
	require.Equal(t, reconcile.Result{}, res)

	// Lookup the userSignup again
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)
	assert.Equal(t, "pending", userSignup.Labels[v1alpha1.UserSignupStateLabelKey])
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)

	// There should be no MasterUserRecords
	murs := &v1alpha1.MasterUserRecordList{}
	err = r.client.List(context.TODO(), murs)
	require.NoError(t, err)
	require.Len(t, murs.Items, 0)

	test.AssertConditionsMatch(t, userSignup.Status.Conditions,
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupApproved,
			Status: v1.ConditionFalse,
			Reason: "PendingApproval",
		},
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupComplete,
			Status: v1.ConditionFalse,
			Reason: "PendingApproval",
		},
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupUserDeactivatedNotificationCreated,
			Status: v1.ConditionFalse,
			Reason: "UserIsActive",
		})
	AssertThatCounters(t).HaveMasterUserRecords(1)
}

func TestUserSignupWithAutoApprovalWithTargetCluster(t *testing.T) {
	// given
	userSignup := NewUserSignup(WithTargetCluster("east"))

	ready := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue))
	r, req, _ := prepareReconcile(t, userSignup.Name, ready, userSignup, NewHostOperatorConfigWithReset(t, test.AutomaticApproval().Enabled()), baseNSTemplateTier)
	InitializeCounters(t, NewToolchainStatus(WithHost(WithMasterUserRecordCount(1))))

	// when
	res, err := r.Reconcile(req)

	// then
	require.NoError(t, err)
	require.Equal(t, reconcile.Result{}, res)

	// Lookup the userSignup again
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)
	assert.Equal(t, "approved", userSignup.Labels[v1alpha1.UserSignupStateLabelKey])
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupApprovedTotal)
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)

	murs := &v1alpha1.MasterUserRecordList{}
	err = r.client.List(context.TODO(), murs)
	require.NoError(t, err)
	require.Len(t, murs.Items, 1)

	mur := murs.Items[0]
	require.Equal(t, test.HostOperatorNs, mur.Namespace)
	require.Equal(t, userSignup.Name, mur.Labels[v1alpha1.MasterUserRecordOwnerLabelKey])
	require.Len(t, mur.Spec.UserAccounts, 1)
	assert.Equal(t, "base", mur.Spec.UserAccounts[0].Spec.NSTemplateSet.TierName)
	require.Len(t, mur.Spec.UserAccounts[0].Spec.NSTemplateSet.Namespaces, 2)

	test.AssertConditionsMatch(t, userSignup.Status.Conditions,
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupApproved,
			Status: v1.ConditionTrue,
			Reason: "ApprovedAutomatically",
		},
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupUserDeactivatedNotificationCreated,
			Status: v1.ConditionFalse,
			Reason: "UserIsActive",
		})
	AssertThatCounters(t).HaveMasterUserRecords(2)

	t.Run("second reconcile", func(t *testing.T) {
		// when
		res, err = r.Reconcile(req)

		// then
		require.NoError(t, err)
		require.Equal(t, reconcile.Result{}, res)

		// Lookup the userSignup one more and check the conditions are updated
		err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
		require.NoError(t, err)
		require.Equal(t, userSignup.Status.CompliantUsername, mur.Name)

		assert.Equal(t, "approved", userSignup.Labels[v1alpha1.UserSignupStateLabelKey])
		AssertMetricsCounterEquals(t, 1, metrics.UserSignupApprovedTotal)
		AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)

		test.AssertConditionsMatch(t, userSignup.Status.Conditions,
			v1alpha1.Condition{
				Type:   v1alpha1.UserSignupApproved,
				Status: v1.ConditionTrue,
				Reason: "ApprovedAutomatically",
			},
			v1alpha1.Condition{
				Type:   v1alpha1.UserSignupComplete,
				Status: v1.ConditionTrue,
			},
			v1alpha1.Condition{
				Type:   v1alpha1.UserSignupUserDeactivatedNotificationCreated,
				Status: v1.ConditionFalse,
				Reason: "UserIsActive",
			})
		AssertThatCounters(t).HaveMasterUserRecords(2)
	})
}

func TestUserSignupWithMissingApprovalPolicyTreatedAsManual(t *testing.T) {
	// given
	userSignup := NewUserSignup(WithTargetCluster("east"))

	r, req, _ := prepareReconcile(t, userSignup.Name, NewGetMemberClusters(), userSignup, baseNSTemplateTier)
	InitializeCounters(t, NewToolchainStatus(WithHost(WithMasterUserRecordCount(1))))

	// when
	res, err := r.Reconcile(req)

	// then
	require.NoError(t, err)
	require.Equal(t, reconcile.Result{}, res)

	// Lookup the userSignup again
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)
	assert.Equal(t, "pending", userSignup.Labels[v1alpha1.UserSignupStateLabelKey])
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)

	test.AssertConditionsMatch(t, userSignup.Status.Conditions,
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupApproved,
			Status: v1.ConditionFalse,
			Reason: "PendingApproval",
		},
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupComplete,
			Status: v1.ConditionFalse,
			Reason: "PendingApproval",
		},
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupUserDeactivatedNotificationCreated,
			Status: v1.ConditionFalse,
			Reason: "UserIsActive",
		})
	AssertThatCounters(t).HaveMasterUserRecords(1)
}

func TestUserSignupMURCreateFails(t *testing.T) {
	// given
	userSignup := NewUserSignup(Approved())

	ready := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue))
	r, req, fakeClient := prepareReconcile(t, userSignup.Name, ready, userSignup, baseNSTemplateTier)
	InitializeCounters(t, NewToolchainStatus(WithHost(WithMasterUserRecordCount(1))))

	fakeClient.MockCreate = func(ctx context.Context, obj runtime.Object, opts ...client.CreateOption) error {
		switch obj.(type) {
		case *v1alpha1.MasterUserRecord:
			return errors.New("unable to create mur")
		default:
			return fakeClient.Create(ctx, obj)
		}
	}

	// when
	res, err := r.Reconcile(req)

	// then
	require.Error(t, err)
	require.Equal(t, reconcile.Result{}, res)
	AssertThatCounters(t).HaveMasterUserRecords(1)

	err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)
	assert.Equal(t, "approved", userSignup.Labels[v1alpha1.UserSignupStateLabelKey])
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupApprovedTotal)
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)

}

func TestUserSignupMURReadFails(t *testing.T) {
	// given
	userSignup := NewUserSignup(Approved())

	ready := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue))
	r, req, fakeClient := prepareReconcile(t, userSignup.Name, ready, userSignup)
	InitializeCounters(t, NewToolchainStatus(WithHost(WithMasterUserRecordCount(1))))

	fakeClient.MockGet = func(ctx context.Context, key client.ObjectKey, obj runtime.Object) error {
		switch obj.(type) {
		case *v1alpha1.MasterUserRecord:
			return errors.New("failed to lookup MUR")
		default:
			return fakeClient.Client.Get(ctx, key, obj)
		}
	}

	// when
	_, err := r.Reconcile(req)

	// then
	require.Error(t, err)
	AssertThatCounters(t).HaveMasterUserRecords(1)

	err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)
	assert.Equal(t, "approved", userSignup.Labels[v1alpha1.UserSignupStateLabelKey])
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupApprovedTotal)
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)

}

func TestUserSignupSetStatusApprovedByAdminFails(t *testing.T) {
	// given
	userSignup := NewUserSignup(Approved())
	userSignup.Labels[v1alpha1.UserSignupStateLabelKey] = "approved"

	ready := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue))
	r, req, fakeClient := prepareReconcile(t, userSignup.Name, ready, userSignup)
	InitializeCounters(t, NewToolchainStatus(WithHost(WithMasterUserRecordCount(1))))

	fakeClient.MockStatusUpdate = func(ctx context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
		switch obj.(type) {
		case *v1alpha1.UserSignup:
			return errors.New("failed to update UserSignup status")
		default:
			return fakeClient.Client.Update(ctx, obj)
		}
	}

	// when
	res, err := r.Reconcile(req)

	// then
	require.Error(t, err)
	require.Equal(t, reconcile.Result{}, res)
	AssertThatCounters(t).HaveMasterUserRecords(1)

	err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)
	assert.Equal(t, "approved", userSignup.Labels[v1alpha1.UserSignupStateLabelKey])
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal) // zero since starting state was approved
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupUniqueTotal)   // zero since starting state was approved
	assert.Empty(t, userSignup.Status.Conditions)
}

func TestUserSignupSetStatusApprovedAutomaticallyFails(t *testing.T) {
	// given
	userSignup := NewUserSignup()

	ready := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue))
	r, req, fakeClient := prepareReconcile(t, userSignup.Name, ready, userSignup, NewHostOperatorConfigWithReset(t, test.AutomaticApproval().Enabled()))
	InitializeCounters(t, NewToolchainStatus(WithHost(WithMasterUserRecordCount(1))))

	fakeClient.MockStatusUpdate = func(ctx context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
		switch obj.(type) {
		case *v1alpha1.UserSignup:
			return errors.New("failed to update UserSignup status")
		default:
			return fakeClient.Client.Update(ctx, obj)
		}
	}

	// when
	res, err := r.Reconcile(req)

	// then
	require.Error(t, err)
	require.Equal(t, reconcile.Result{}, res)
	AssertThatCounters(t).HaveMasterUserRecords(1)

	err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)
	assert.Equal(t, "not-ready", userSignup.Labels[v1alpha1.UserSignupStateLabelKey])
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)
	assert.Empty(t, userSignup.Status.Conditions)

}

func TestUserSignupSetStatusNoClustersAvailableFails(t *testing.T) {
	// given
	userSignup := NewUserSignup()

	r, req, fakeClient := prepareReconcile(t, userSignup.Name, NewGetMemberClusters(), userSignup, NewHostOperatorConfigWithReset(t, test.AutomaticApproval().Enabled()))
	InitializeCounters(t, NewToolchainStatus(WithHost(WithMasterUserRecordCount(1))))

	fakeClient.MockStatusUpdate = func(ctx context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
		switch obj := obj.(type) {
		case *v1alpha1.UserSignup:
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
	res, err := r.Reconcile(req)

	// then
	require.Error(t, err)
	require.Equal(t, reconcile.Result{}, res)
	AssertThatCounters(t).HaveMasterUserRecords(1)

	err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)
	assert.Equal(t, "pending", userSignup.Labels[v1alpha1.UserSignupStateLabelKey])
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)

}

func TestUserSignupWithExistingMUROK(t *testing.T) {
	// given
	userSignup := NewUserSignup()
	userSignup.Annotations = map[string]string{
		v1alpha1.UserSignupUserEmailAnnotationKey: "foo@redhat.com",
	}
	userSignup.Labels = map[string]string{
		v1alpha1.UserSignupUserEmailHashLabelKey: "fd2addbd8d82f0d2dc088fa122377eaa",
		"toolchain.dev.openshift.com/approved":   "true",
	}

	// Create a MUR with the same UserID
	mur := &v1alpha1.MasterUserRecord{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: test.HostOperatorNs,
			Labels:    map[string]string{v1alpha1.MasterUserRecordOwnerLabelKey: userSignup.Name},
		},
	}

	ready := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue))
	r, req, _ := prepareReconcile(t, userSignup.Name, ready, userSignup, mur, NewHostOperatorConfigWithReset(t, test.AutomaticApproval().Enabled()), baseNSTemplateTier)
	InitializeCounters(t, NewToolchainStatus(WithHost(WithMasterUserRecordCount(1))))

	// when
	_, err := r.Reconcile(req)

	// then
	require.NoError(t, err)

	key := types.NamespacedName{
		Namespace: test.HostOperatorNs,
		Name:      userSignup.Name,
	}
	instance := &v1alpha1.UserSignup{}
	err = r.client.Get(context.TODO(), key, instance)
	require.NoError(t, err)
	assert.Equal(t, "approved", instance.Labels[v1alpha1.UserSignupStateLabelKey])
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupApprovedTotal)
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)

	require.Equal(t, mur.Name, instance.Status.CompliantUsername)
	test.AssertContainsCondition(t, instance.Status.Conditions, v1alpha1.Condition{
		Type:   v1alpha1.UserSignupComplete,
		Status: v1.ConditionTrue,
	})
	AssertThatCounters(t).HaveMasterUserRecords(1)
}

func TestUserSignupWithExistingMURDifferentUserIDOK(t *testing.T) {
	// given
	userSignup := NewUserSignup(Approved())

	// Create a MUR with a different UserID
	mur := &v1alpha1.MasterUserRecord{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: test.HostOperatorNs,
			Labels: map[string]string{
				v1alpha1.MasterUserRecordOwnerLabelKey: uuid.NewV4().String(),
				"toolchain.dev.openshift.com/approved": "true",
			},
		},
	}

	ready := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue))
	r, req, _ := prepareReconcile(t, userSignup.Name, ready, userSignup, mur, NewHostOperatorConfigWithReset(t, test.AutomaticApproval().Enabled()), baseNSTemplateTier)
	InitializeCounters(t, NewToolchainStatus(WithHost(WithMasterUserRecordCount(1))))

	// when
	_, err := r.Reconcile(req)

	// then
	require.NoError(t, err)

	// We should now have 2 MURs
	murs := &v1alpha1.MasterUserRecordList{}
	err = r.client.List(context.TODO(), murs)
	require.NoError(t, err)
	require.Len(t, murs.Items, 2)
	AssertThatCounters(t).HaveMasterUserRecords(2)

	key := types.NamespacedName{
		Namespace: test.HostOperatorNs,
		Name:      userSignup.Name,
	}
	instance := &v1alpha1.UserSignup{}
	err = r.client.Get(context.TODO(), key, instance)
	require.NoError(t, err)
	assert.Equal(t, "approved", instance.Labels[v1alpha1.UserSignupStateLabelKey])
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupApprovedTotal)
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)

	t.Run("second reconcile", func(t *testing.T) {
		// when
		_, err = r.Reconcile(req)

		// then
		require.NoError(t, err)

		err = r.client.Get(context.TODO(), key, instance)
		require.NoError(t, err)
		assert.Equal(t, "approved", instance.Labels[v1alpha1.UserSignupStateLabelKey])
		AssertMetricsCounterEquals(t, 1, metrics.UserSignupApprovedTotal)
		AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)

		require.Equal(t, "foo-2", instance.Status.CompliantUsername)

		// Confirm that the mur exists
		mur = &v1alpha1.MasterUserRecord{}
		err = r.client.Get(context.TODO(), types.NamespacedName{Namespace: test.HostOperatorNs, Name: instance.Status.CompliantUsername}, mur)
		require.NoError(t, err)
		require.Equal(t, instance.Name, mur.Labels[v1alpha1.MasterUserRecordOwnerLabelKey])

		var cond *v1alpha1.Condition
		for _, condition := range instance.Status.Conditions {
			if condition.Type == v1alpha1.UserSignupComplete {
				cond = &condition
			}
		}

		require.Equal(t, mur.Name, instance.Status.CompliantUsername)
		require.NotNil(t, cond)
		require.Equal(t, v1.ConditionTrue, cond.Status)
		AssertThatCounters(t).HaveMasterUserRecords(2)
	})
}

func TestUserSignupWithSpecialCharOK(t *testing.T) {
	// given
	userSignup := NewUserSignup(WithUsername("foo#$%^bar@redhat.com"))

	ready := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue))
	r, req, _ := prepareReconcile(t, userSignup.Name, ready, userSignup, NewHostOperatorConfigWithReset(t, test.AutomaticApproval().Enabled()), baseNSTemplateTier)
	InitializeCounters(t, NewToolchainStatus(WithHost(WithMasterUserRecordCount(1))))

	// when
	_, err := r.Reconcile(req)

	// then
	require.NoError(t, err)

	murtest.AssertThatMasterUserRecord(t, "foo-bar", r.client).HasNoConditions()
	AssertThatCounters(t).HaveMasterUserRecords(2)
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupApprovedTotal)
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)
}

func TestUserSignupDeactivatedAfterMURCreated(t *testing.T) {
	// given
	userSignup := NewUserSignup(Deactivated())
	userSignup.Status = v1alpha1.UserSignupStatus{
		Conditions: []v1alpha1.Condition{
			{
				Type:   v1alpha1.UserSignupComplete,
				Status: v1.ConditionTrue,
			},
			{
				Type:   v1alpha1.UserSignupApproved,
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
		mur.Labels = map[string]string{v1alpha1.MasterUserRecordOwnerLabelKey: userSignup.Name}

		r, req, _ := prepareReconcile(t, userSignup.Name, NewGetMemberClusters(), userSignup, mur, NewHostOperatorConfigWithReset(t, test.AutomaticApproval().Enabled()), baseNSTemplateTier)
		InitializeCounters(t, NewToolchainStatus(WithHost(WithMasterUserRecordCount(1))))

		// when
		_, err := r.Reconcile(req)

		// then
		require.NoError(t, err)
		err = r.client.Get(context.TODO(), key, userSignup)
		require.NoError(t, err)
		assert.Equal(t, "deactivated", userSignup.Labels[v1alpha1.UserSignupStateLabelKey])
		AssertMetricsCounterEquals(t, 1, metrics.UserSignupDeactivatedTotal)
		AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal) // 0 because usersignup was originally deactivated
		AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)   // 1 because state was initially empty

		// Confirm the status is now set to Deactivating
		test.AssertConditionsMatch(t, userSignup.Status.Conditions,
			v1alpha1.Condition{
				Type:   v1alpha1.UserSignupApproved,
				Status: v1.ConditionTrue,
				Reason: "ApprovedAutomatically",
			},
			v1alpha1.Condition{
				Type:   v1alpha1.UserSignupComplete,
				Status: v1.ConditionFalse,
				Reason: "Deactivating",
			})

		murs := &v1alpha1.MasterUserRecordList{}

		// The MUR should have now been deleted
		err = r.client.List(context.TODO(), murs)
		require.NoError(t, err)
		require.Len(t, murs.Items, 0)
		AssertThatCounters(t).HaveMasterUserRecords(1)

		// There should not be a notification created yet, only the next reconcile (with deleted mur) would create the notification
		ntest.AssertNoNotificationsExist(t, r.client)
	})

	t.Run("when MUR doesn't exist, then the condition should be set to Deactivated", func(t *testing.T) {
		// given
		r, req, _ := prepareReconcile(t, userSignup.Name, NewGetMemberClusters(), userSignup, NewHostOperatorConfigWithReset(t, test.AutomaticApproval().Enabled()), baseNSTemplateTier)
		InitializeCounters(t, NewToolchainStatus(WithHost(WithMasterUserRecordCount(2))))

		// when
		_, err := r.Reconcile(req)

		// then
		require.NoError(t, err)

		// Lookup the UserSignup
		err = r.client.Get(context.TODO(), key, userSignup)
		require.NoError(t, err)
		assert.Equal(t, "deactivated", userSignup.Labels[v1alpha1.UserSignupStateLabelKey])
		AssertMetricsCounterEquals(t, 0, metrics.UserSignupDeactivatedTotal) // zero because state didn't change
		AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
		AssertMetricsCounterEquals(t, 0, metrics.UserSignupUniqueTotal)

		// Confirm the status has been set to Deactivated
		test.AssertConditionsMatch(t, userSignup.Status.Conditions,
			v1alpha1.Condition{
				Type:   v1alpha1.UserSignupApproved,
				Status: v1.ConditionTrue,
				Reason: "ApprovedAutomatically",
			},
			v1alpha1.Condition{
				Type:   v1alpha1.UserSignupComplete,
				Status: v1.ConditionTrue,
				Reason: "Deactivated",
			},
			v1alpha1.Condition{
				Type:   v1alpha1.UserSignupUserDeactivatedNotificationCreated,
				Status: v1.ConditionTrue,
				Reason: "NotificationCRCreated",
			})
		AssertThatCounters(t).HaveMasterUserRecords(2)

		// A deactivated notification should have been created
		notifications := &v1alpha1.NotificationList{}
		err = r.client.List(context.TODO(), notifications)
		require.NoError(t, err)
		require.Len(t, notifications.Items, 1)
		notification := notifications.Items[0]
		require.Contains(t, notification.Name, "john-doe-deactivated-")
		assert.True(t, len(notification.Name) > len("john-doe-deactivated-"))
		require.Equal(t, userSignup.Name, notification.Spec.UserID)
		assert.Equal(t, "userdeactivated", notification.Spec.Template)
	})

}

func TestUserSignupFailedToCreateDeactivationNotification(t *testing.T) {
	// given
	userSignup := &v1alpha1.UserSignup{
		ObjectMeta: NewUserSignupObjectMeta("", "john.doe@redhat.com"),
		Spec: v1alpha1.UserSignupSpec{
			UserID:      "UserID123",
			Username:    "john.doe@redhat.com",
			Deactivated: true,
		},
		Status: v1alpha1.UserSignupStatus{
			Conditions: []v1alpha1.Condition{
				{
					Type:   v1alpha1.UserSignupComplete,
					Status: v1.ConditionTrue,
				},
				{
					Type:   v1alpha1.UserSignupApproved,
					Status: v1.ConditionTrue,
					Reason: "ApprovedAutomatically",
				},
			},
			CompliantUsername: "john-doe",
		},
	}
	userSignup.Labels[v1alpha1.UserSignupStateLabelKey] = v1alpha1.NotificationTypeDeactivated
	userSignup.Labels["toolchain.dev.openshift.com/approved"] = "true"
	// NotificationUserNameLabelKey is only used for easy lookup for debugging and e2e tests
	userSignup.Labels[v1alpha1.NotificationUserNameLabelKey] = "john-doe"
	// NotificationTypeLabelKey is only used for easy lookup for debugging and e2e tests
	userSignup.Labels[v1alpha1.NotificationTypeLabelKey] = v1alpha1.NotificationTypeDeactivated
	key := test.NamespacedName(test.HostOperatorNs, userSignup.Name)

	t.Run("when the deactivation notification cannot be created", func(t *testing.T) {
		// given
		r, req, fakeClient := prepareReconcile(t, userSignup.Name, NewGetMemberClusters(), userSignup,
			NewHostOperatorConfigWithReset(t, test.AutomaticApproval().Enabled()), baseNSTemplateTier)
		InitializeCounters(t, NewToolchainStatus(WithHost(WithMasterUserRecordCount(2))))

		fakeClient.MockCreate = func(ctx context.Context, obj runtime.Object, opts ...client.CreateOption) error {
			switch obj.(type) {
			case *v1alpha1.Notification:
				return errors.New("unable to create deactivation notification")
			default:
				return fakeClient.Create(ctx, obj)
			}
		}

		// when
		_, err := r.Reconcile(req)

		// then
		require.Error(t, err)
		require.Equal(t, "Failed to create user deactivation notification: unable to create deactivation notification", err.Error())

		// Lookup the UserSignup
		err = r.client.Get(context.TODO(), key, userSignup)
		require.NoError(t, err)

		// Confirm the status shows the deactivation notification failure
		test.AssertConditionsMatch(t, userSignup.Status.Conditions,
			v1alpha1.Condition{
				Type:   v1alpha1.UserSignupApproved,
				Status: v1.ConditionTrue,
				Reason: "ApprovedAutomatically",
			},
			v1alpha1.Condition{
				Type:   v1alpha1.UserSignupComplete,
				Status: v1.ConditionTrue,
			},
			v1alpha1.Condition{
				Type:    v1alpha1.UserSignupUserDeactivatedNotificationCreated,
				Status:  v1.ConditionFalse,
				Reason:  "NotificationCRCreationFailed",
				Message: "unable to create deactivation notification",
			})
		AssertThatCounters(t).HaveMasterUserRecords(2)
		assert.Equal(t, "deactivated", userSignup.Labels[v1alpha1.UserSignupStateLabelKey])
		AssertMetricsCounterEquals(t, 0, metrics.UserSignupDeactivatedTotal) // zero because state didn't change
		AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
		AssertMetricsCounterEquals(t, 0, metrics.UserSignupUniqueTotal)

		// A deactivated notification should not have been created
		notificationList := &v1alpha1.NotificationList{}
		err = r.client.List(context.TODO(), notificationList)
		require.NoError(t, err)
		require.Equal(t, 0, len(notificationList.Items))
	})
}

func TestUserSignupReactivateAfterDeactivated(t *testing.T) {
	// given
	userSignup := &v1alpha1.UserSignup{
		ObjectMeta: NewUserSignupObjectMeta("", "john.doe@redhat.com"),
		Spec: v1alpha1.UserSignupSpec{
			UserID:      "UserID123",
			Username:    "john.doe@redhat.com",
			Deactivated: false,
		},
		Status: v1alpha1.UserSignupStatus{
			CompliantUsername: "john-doe",
		},
	}
	key := test.NamespacedName(test.HostOperatorNs, userSignup.Name)

	t.Run("when reactivating the usersignup successfully", func(t *testing.T) {
		// given
		// start with a usersignup that has the Notification Created status set to "true" but Spec.Deactivated is set to "false" which signals a user which has been just reactivated.
		userSignup.Labels[v1alpha1.UserSignupStateLabelKey] = "deactivated"
		userSignup.Labels["toolchain.dev.openshift.com/approved"] = "true"
		userSignup.Annotations[v1alpha1.UserSignupActivationCounterAnnotationKey] = "2" // the user signed up twice
		userSignup.Status.Conditions = []v1alpha1.Condition{
			{
				Type:   v1alpha1.UserSignupComplete,
				Status: v1.ConditionTrue,
				Reason: "Deactivated",
			},
			{
				Type:   v1alpha1.UserSignupApproved,
				Status: v1.ConditionTrue,
				Reason: "ApprovedAutomatically",
			},
			{
				Type:   v1alpha1.UserSignupUserDeactivatedNotificationCreated,
				Status: v1.ConditionTrue,
				Reason: "NotificationCRCreated",
			},
		}
		ready := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue))
		r, req, _ := prepareReconcile(t, userSignup.Name, ready, userSignup, NewHostOperatorConfigWithReset(t, test.AutomaticApproval().Enabled()), baseNSTemplateTier)
		InitializeCounters(t, NewToolchainStatus(
			WithHost(WithMasterUserRecordCount(20)),
			WithMetric(v1alpha1.UsersPerActivationMetricKey, v1alpha1.Metric{
				"2": 11, // 11 users signed-up 2 times, including our user above, even though she is not active at the moment
				"3": 10, // 10 users signed-up 3 times
			})))

		// when
		_, err := r.Reconcile(req)

		// then
		require.NoError(t, err)

		// Lookup the UserSignup
		err = r.client.Get(context.TODO(), key, userSignup)
		require.NoError(t, err)

		// Confirm the status shows the notification created condition is reset to active
		test.AssertConditionsMatch(t, userSignup.Status.Conditions,
			v1alpha1.Condition{
				Type:   v1alpha1.UserSignupApproved,
				Status: v1.ConditionTrue,
				Reason: "ApprovedAutomatically",
			},
			v1alpha1.Condition{
				Type:   v1alpha1.UserSignupComplete,
				Status: v1.ConditionTrue,
				Reason: "Deactivated",
			},
			v1alpha1.Condition{
				Type:   v1alpha1.UserSignupUserDeactivatedNotificationCreated,
				Status: v1.ConditionFalse,
				Reason: "UserIsActive",
			})

		// A mur should be created so the counter should be 21
		AssertThatCounters(t).HaveMasterUserRecords(21) // one more than before

		assert.Equal(t, "approved", userSignup.Labels[v1alpha1.UserSignupStateLabelKey])
		// verify that the annotation was incremented
		assert.Equal(t, "3", userSignup.Annotations[v1alpha1.UserSignupActivationCounterAnnotationKey])
		AssertMetricsCounterEquals(t, 0, metrics.UserSignupDeactivatedTotal)
		AssertMetricsCounterEquals(t, 1, metrics.UserSignupApprovedTotal)
		AssertMetricsCounterEquals(t, 0, metrics.UserSignupUniqueTotal)
		AssertMetricsGaugeEquals(t, 10, metrics.UsersPerActivationGaugeVec.WithLabelValues("2")) // user signed up 3 times now, so the counter for `2` activations was decreased by 1
		AssertMetricsGaugeEquals(t, 11, metrics.UsersPerActivationGaugeVec.WithLabelValues("3")) // user signed up 3 times now, so the counter for `3` activations was increased by 1

		// There should not be a notification created because the user was reactivated
		ntest.AssertNoNotificationsExist(t, r.client)
	})

	t.Run("when resetting the usersignup deactivation notification status fails", func(t *testing.T) {
		// given
		// start with a usersignup that has the Notification Created status set to "true" but Spec.Deactivated is set to "false" which signals a user which has been just reactivated.
		userSignup.Labels[v1alpha1.UserSignupStateLabelKey] = "deactivated"
		userSignup.Labels["toolchain.dev.openshift.com/approved"] = "true"
		userSignup.Status.Conditions = []v1alpha1.Condition{
			{
				Type:   v1alpha1.UserSignupComplete,
				Status: v1.ConditionTrue,
				Reason: "Deactivated",
			},
			{
				Type:   v1alpha1.UserSignupApproved,
				Status: v1.ConditionTrue,
				Reason: "ApprovedAutomatically",
			},
			{
				Type:   v1alpha1.UserSignupUserDeactivatedNotificationCreated,
				Status: v1.ConditionTrue,
				Reason: "NotificationCRCreated",
			},
		}
		r, req, fakeClient := prepareReconcile(t, userSignup.Name, NewGetMemberClusters(), userSignup, NewHostOperatorConfigWithReset(t, test.AutomaticApproval().Enabled()), baseNSTemplateTier)
		InitializeCounters(t, NewToolchainStatus(WithHost(WithMasterUserRecordCount(2))))

		fakeClient.MockStatusUpdate = func(ctx context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
			switch obj.(type) {
			case *v1alpha1.UserSignup:
				return errors.New("failed to update UserSignup status")
			default:
				return fakeClient.Client.Update(ctx, obj)
			}
		}

		// when
		_, err := r.Reconcile(req)

		// then
		require.Error(t, err)
		require.Equal(t, "failed to update UserSignup status", err.Error())

		// Lookup the UserSignup
		err = r.client.Get(context.TODO(), key, userSignup)
		require.NoError(t, err)

		// Confirm the status shows the notification is unchanged because the status update failed
		test.AssertConditionsMatch(t, userSignup.Status.Conditions,
			v1alpha1.Condition{
				Type:   v1alpha1.UserSignupApproved,
				Status: v1.ConditionTrue,
				Reason: "ApprovedAutomatically",
			},
			v1alpha1.Condition{
				Type:   v1alpha1.UserSignupComplete,
				Status: v1.ConditionTrue,
				Reason: "Deactivated",
			},
			v1alpha1.Condition{
				Type:   v1alpha1.UserSignupUserDeactivatedNotificationCreated,
				Status: v1.ConditionTrue,
				Reason: "NotificationCRCreated",
			})
		AssertThatCounters(t).HaveMasterUserRecords(2)

		// State is still deactivated because the status update failed
		assert.Equal(t, "deactivated", userSignup.Labels[v1alpha1.UserSignupStateLabelKey])
		AssertMetricsCounterEquals(t, 0, metrics.UserSignupDeactivatedTotal)
		AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
		AssertMetricsCounterEquals(t, 0, metrics.UserSignupUniqueTotal)

		// A deactivation notification should not be created because this is the reactivation case
		ntest.AssertNoNotificationsExist(t, r.client)
	})
}

func TestUserSignupDeactivatingWhenMURExists(t *testing.T) {
	userSignup := &v1alpha1.UserSignup{
		ObjectMeta: NewUserSignupObjectMeta("", "edward.jones@redhat.com"),
		Spec: v1alpha1.UserSignupSpec{
			UserID:      "UserID123",
			Username:    "edward.jones@redhat.com",
			Deactivated: true,
		},
		Status: v1alpha1.UserSignupStatus{
			Conditions: []v1alpha1.Condition{
				{
					Type:   v1alpha1.UserSignupComplete,
					Status: v1.ConditionFalse,
					Reason: "Deactivating",
				},
				{
					Type:   v1alpha1.UserSignupApproved,
					Status: v1.ConditionTrue,
					Reason: "ApprovedAutomatically",
				},
			},
			CompliantUsername: "edward-jones",
		},
	}
	userSignup.Labels["toolchain.dev.openshift.com/approved"] = "true"
	userSignup.Labels[v1alpha1.UserSignupStateLabelKey] = "approved"
	key := test.NamespacedName(test.HostOperatorNs, userSignup.Name)

	t.Run("when MUR exists, then it should be deleted", func(t *testing.T) {
		// given
		mur := murtest.NewMasterUserRecord(t, "edward-jones", murtest.MetaNamespace(test.HostOperatorNs))
		mur.Labels = map[string]string{
			v1alpha1.MasterUserRecordOwnerLabelKey: userSignup.Name,
			v1alpha1.UserSignupStateLabelKey:       "approved",
		}

		r, req, _ := prepareReconcile(t, userSignup.Name, NewGetMemberClusters(), userSignup, mur, NewHostOperatorConfigWithReset(t, test.AutomaticApproval().Enabled()), baseNSTemplateTier)
		InitializeCounters(t, NewToolchainStatus(WithHost(WithMasterUserRecordCount(1))))

		t.Run("first reconcile - status should be deactivating and mur should be deleted", func(t *testing.T) {
			// when
			_, err := r.Reconcile(req)

			// then
			require.NoError(t, err)
			err = r.client.Get(context.TODO(), key, userSignup)
			require.NoError(t, err)
			assert.Equal(t, "deactivated", userSignup.Labels[v1alpha1.UserSignupStateLabelKey])
			AssertMetricsCounterEquals(t, 1, metrics.UserSignupDeactivatedTotal)
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupUniqueTotal)

			// Confirm the status is still set to Deactivating
			test.AssertConditionsMatch(t, userSignup.Status.Conditions,
				v1alpha1.Condition{
					Type:   v1alpha1.UserSignupApproved,
					Status: v1.ConditionTrue,
					Reason: "ApprovedAutomatically",
				},
				v1alpha1.Condition{
					Type:   v1alpha1.UserSignupComplete,
					Status: v1.ConditionFalse,
					Reason: "Deactivating",
				})

			murs := &v1alpha1.MasterUserRecordList{}

			// The MUR should have now been deleted
			err = r.client.List(context.TODO(), murs)
			require.NoError(t, err)
			require.Len(t, murs.Items, 0)
			AssertThatCounters(t).HaveMasterUserRecords(1)

			// There should not be a notification created yet, only the next reconcile (with deleted mur) would create the notification
			ntest.AssertNoNotificationsExist(t, r.client)
		})

		t.Run("second reconcile - condition should be deactivated and deactivation notification created", func(t *testing.T) {
			res, err := r.Reconcile(req)
			require.NoError(t, err)
			require.Equal(t, reconcile.Result{}, res)

			// lookup the userSignup and check the conditions
			err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
			require.NoError(t, err)

			// Confirm the status has been set to Deactivated and the deactivation notification is created
			test.AssertConditionsMatch(t, userSignup.Status.Conditions,
				v1alpha1.Condition{
					Type:   v1alpha1.UserSignupApproved,
					Status: v1.ConditionTrue,
					Reason: "ApprovedAutomatically",
				},
				v1alpha1.Condition{
					Type:   v1alpha1.UserSignupComplete,
					Status: v1.ConditionTrue,
					Reason: "Deactivated",
				},
				v1alpha1.Condition{
					Type:   v1alpha1.UserSignupUserDeactivatedNotificationCreated,
					Status: v1.ConditionTrue,
					Reason: "NotificationCRCreated",
				})
			// metrics should be the same after the 2nd reconcile
			AssertMetricsCounterEquals(t, 1, metrics.UserSignupDeactivatedTotal)
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
			AssertMetricsCounterEquals(t, 0, metrics.UserSignupUniqueTotal)
		})
	})
}

func TestUserSignupBanned(t *testing.T) {
	// given
	userSignup := NewUserSignup()
	userSignup.Labels[v1alpha1.UserSignupStateLabelKey] = "approved"

	bannedUser := &v1alpha1.BannedUser{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				v1alpha1.BannedUserEmailHashLabelKey: "fd2addbd8d82f0d2dc088fa122377eaa",
				v1alpha1.UserSignupStateLabelKey:     "approved",
			},
		},
		Spec: v1alpha1.BannedUserSpec{
			Email: "foo@redhat.com",
		},
	}

	r, req, _ := prepareReconcile(t, userSignup.Name, NewGetMemberClusters(), userSignup, bannedUser, NewHostOperatorConfigWithReset(t, test.AutomaticApproval().Enabled()), baseNSTemplateTier)
	InitializeCounters(t, NewToolchainStatus(WithHost(WithMasterUserRecordCount(1))))

	// when
	_, err := r.Reconcile(req)

	// then
	require.NoError(t, err)
	err = r.client.Get(context.TODO(), test.NamespacedName(test.HostOperatorNs, userSignup.Name), userSignup)
	require.NoError(t, err)
	assert.Equal(t, "banned", userSignup.Labels[v1alpha1.UserSignupStateLabelKey])
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupBannedTotal)
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupDeactivatedTotal)
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupUniqueTotal)

	// Confirm the status is set to Banned
	test.AssertConditionsMatch(t, userSignup.Status.Conditions,
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupComplete,
			Status: v1.ConditionTrue,
			Reason: "Banned",
		})

	// Confirm that no MUR is created
	murs := &v1alpha1.MasterUserRecordList{}

	// Confirm that the MUR has now been deleted
	err = r.client.List(context.TODO(), murs)
	require.NoError(t, err)
	require.Len(t, murs.Items, 0)
	AssertThatCounters(t).HaveMasterUserRecords(1)
}

func TestUserSignupVerificationRequired(t *testing.T) {
	// given
	userSignup := NewUserSignup(VerificationRequired())

	r, req, _ := prepareReconcile(t, userSignup.Name, NewGetMemberClusters(), userSignup, NewHostOperatorConfigWithReset(t, test.AutomaticApproval().Enabled()), baseNSTemplateTier)
	InitializeCounters(t, NewToolchainStatus(WithHost(WithMasterUserRecordCount(1))))

	// when
	_, err := r.Reconcile(req)

	// then
	require.NoError(t, err)
	err = r.client.Get(context.TODO(), test.NamespacedName(test.HostOperatorNs, userSignup.Name), userSignup)
	require.NoError(t, err)
	assert.Equal(t, "not-ready", userSignup.Labels[v1alpha1.UserSignupStateLabelKey])
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupBannedTotal)
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupDeactivatedTotal)
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)

	// Confirm the status is set to VerificationRequired
	test.AssertConditionsMatch(t, userSignup.Status.Conditions,
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupComplete,
			Status: v1.ConditionFalse,
			Reason: "VerificationRequired",
		},
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupUserDeactivatedNotificationCreated,
			Status: v1.ConditionFalse,
			Reason: "UserIsActive",
		})

	// Confirm that no MUR is created
	murs := &v1alpha1.MasterUserRecordList{}
	err = r.client.List(context.TODO(), murs)
	require.NoError(t, err)
	require.Len(t, murs.Items, 0)
	AssertThatCounters(t).HaveMasterUserRecords(1)
}

func TestUserSignupBannedMURExists(t *testing.T) {
	// given
	userSignup := NewUserSignup()
	userSignup.Status = v1alpha1.UserSignupStatus{
		Conditions: []v1alpha1.Condition{
			{
				Type:   v1alpha1.UserSignupComplete,
				Status: v1.ConditionTrue,
			},
			{
				Type:   v1alpha1.UserSignupApproved,
				Status: v1.ConditionTrue,
				Reason: "ApprovedAutomatically",
			},
		},
		CompliantUsername: "foo",
	}
	userSignup.Labels[v1alpha1.UserSignupStateLabelKey] = "approved"
	userSignup.Labels["toolchain.dev.openshift.com/approved"] = "true"

	bannedUser := &v1alpha1.BannedUser{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				v1alpha1.BannedUserEmailHashLabelKey: "fd2addbd8d82f0d2dc088fa122377eaa",
			},
		},
		Spec: v1alpha1.BannedUserSpec{
			Email: "foo@redhat.com",
		},
	}

	mur := murtest.NewMasterUserRecord(t, "foo", murtest.MetaNamespace(test.HostOperatorNs))
	mur.Labels = map[string]string{v1alpha1.MasterUserRecordOwnerLabelKey: userSignup.Name}

	r, req, _ := prepareReconcile(t, userSignup.Name, NewGetMemberClusters(), userSignup, mur, bannedUser, NewHostOperatorConfigWithReset(t, test.AutomaticApproval().Enabled()), baseNSTemplateTier)
	InitializeCounters(t, NewToolchainStatus(WithHost(WithMasterUserRecordCount(1))))

	// when
	_, err := r.Reconcile(req)

	// then
	require.NoError(t, err)
	key := test.NamespacedName(test.HostOperatorNs, userSignup.Name)
	err = r.client.Get(context.TODO(), key, userSignup)
	require.NoError(t, err)
	assert.Equal(t, "banned", userSignup.Labels[v1alpha1.UserSignupStateLabelKey])
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupBannedTotal)
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupDeactivatedTotal)
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupUniqueTotal)

	// Confirm the status is set to Banning
	test.AssertConditionsMatch(t, userSignup.Status.Conditions,
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupComplete,
			Status: v1.ConditionFalse,
			Reason: "Banning",
		},
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupApproved,
			Status: v1.ConditionTrue,
			Reason: "ApprovedAutomatically",
		})

	murs := &v1alpha1.MasterUserRecordList{}

	// Confirm that the MUR has now been deleted
	err = r.client.List(context.TODO(), murs)
	require.NoError(t, err)
	require.Len(t, murs.Items, 0)
	AssertThatCounters(t).HaveMasterUserRecords(1)

	t.Run("second reconcile", func(t *testing.T) {
		// when
		_, err = r.Reconcile(req)
		require.NoError(t, err)

		err = r.client.Get(context.TODO(), key, userSignup)
		require.NoError(t, err)

		assert.Equal(t, "banned", userSignup.Labels[v1alpha1.UserSignupStateLabelKey])
		// metrics should be the same after the 2nd reconcile
		AssertMetricsCounterEquals(t, 1, metrics.UserSignupBannedTotal)
		AssertMetricsCounterEquals(t, 0, metrics.UserSignupDeactivatedTotal)
		AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
		AssertMetricsCounterEquals(t, 0, metrics.UserSignupUniqueTotal)

		// Confirm the status is now set to Banned
		test.AssertConditionsMatch(t, userSignup.Status.Conditions,
			v1alpha1.Condition{
				Type:   v1alpha1.UserSignupComplete,
				Status: v1.ConditionTrue,
				Reason: "Banned",
			},
			v1alpha1.Condition{
				Type:   v1alpha1.UserSignupApproved,
				Status: v1.ConditionTrue,
				Reason: "ApprovedAutomatically",
			})

		// Confirm that there is still no MUR
		err = r.client.List(context.TODO(), murs)
		require.NoError(t, err)
		require.Len(t, murs.Items, 0)
		AssertThatCounters(t).HaveMasterUserRecords(1)
	})
}

func TestUserSignupListBannedUsersFails(t *testing.T) {
	// given
	userSignup := NewUserSignup()

	r, req, fakeClient := prepareReconcile(t, userSignup.Name, NewGetMemberClusters(), userSignup, NewHostOperatorConfigWithReset(t, test.AutomaticApproval().Enabled()), baseNSTemplateTier)
	InitializeCounters(t, NewToolchainStatus(WithHost(WithMasterUserRecordCount(1))))

	fakeClient.MockList = func(ctx context.Context, list runtime.Object, opts ...client.ListOption) error {
		return errors.New("err happened")
	}

	// when
	_, err := r.Reconcile(req)

	// then
	require.Error(t, err)
	AssertThatCounters(t).HaveMasterUserRecords(1)
}

func TestUserSignupDeactivatedButMURDeleteFails(t *testing.T) {
	// given
	userSignup := &v1alpha1.UserSignup{
		ObjectMeta: NewUserSignupObjectMeta("", "alice.mayweather.doe@redhat.com"),
		Spec: v1alpha1.UserSignupSpec{
			UserID:      "UserID123",
			Username:    "alice.mayweather.doe@redhat.com",
			Deactivated: true,
		},
		Status: v1alpha1.UserSignupStatus{
			Conditions: []v1alpha1.Condition{
				{
					Type:   v1alpha1.UserSignupComplete,
					Status: v1.ConditionTrue,
				},
				{
					Type:   v1alpha1.UserSignupApproved,
					Status: v1.ConditionTrue,
					Reason: "ApprovedAutomatically",
				},
			},
			CompliantUsername: "alice-mayweather",
		},
	}
	userSignup.Labels[v1alpha1.UserSignupStateLabelKey] = "approved"
	userSignup.Labels["toolchain.dev.openshift.com/approved"] = "true"

	key := test.NamespacedName(test.HostOperatorNs, userSignup.Name)

	mur := murtest.NewMasterUserRecord(t, "john-doe", murtest.MetaNamespace(test.HostOperatorNs))
	mur.Labels = map[string]string{v1alpha1.MasterUserRecordOwnerLabelKey: userSignup.Name}

	r, req, fakeClient := prepareReconcile(t, userSignup.Name, NewGetMemberClusters(), userSignup, mur, NewHostOperatorConfigWithReset(t, test.AutomaticApproval().Enabled()), baseNSTemplateTier)
	InitializeCounters(t, NewToolchainStatus(WithHost(WithMasterUserRecordCount(1))))

	fakeClient.MockDelete = func(ctx context.Context, obj runtime.Object, opts ...client.DeleteOption) error {
		switch obj.(type) {
		case *v1alpha1.MasterUserRecord:
			return errors.New("unable to delete mur")
		default:
			return fakeClient.Delete(ctx, obj)
		}
	}

	t.Run("first reconcile - status should show message about mur deletion failure", func(t *testing.T) {
		// when
		_, err := r.Reconcile(req)
		require.Error(t, err)

		// then

		// Lookup the UserSignup
		err = r.client.Get(context.TODO(), key, userSignup)
		require.NoError(t, err)
		assert.Equal(t, "deactivated", userSignup.Labels[v1alpha1.UserSignupStateLabelKey])
		AssertMetricsCounterEquals(t, 1, metrics.UserSignupDeactivatedTotal)
		AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
		AssertMetricsCounterEquals(t, 0, metrics.UserSignupUniqueTotal)

		// Confirm the status is set to UnableToDeleteMUR
		test.AssertConditionsMatch(t, userSignup.Status.Conditions,
			v1alpha1.Condition{
				Type:   v1alpha1.UserSignupApproved,
				Status: v1.ConditionTrue,
				Reason: "ApprovedAutomatically",
			},
			v1alpha1.Condition{
				Type:    v1alpha1.UserSignupComplete,
				Status:  v1.ConditionFalse,
				Reason:  "UnableToDeleteMUR",
				Message: "unable to delete mur",
			})
		AssertThatCounters(t).HaveMasterUserRecords(1)

		t.Run("second reconcile - there should not be a notification created since the mur deletion failed even if reconciled again", func(t *testing.T) {
			_, err := r.Reconcile(req)
			require.Error(t, err)
			ntest.AssertNoNotificationsExist(t, r.client)
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
	userSignup := NewUserSignup(Approved())

	args := make([]runtime.Object, 0)
	args = append(args, userSignup)
	args = append(args, NewHostOperatorConfigWithReset(t, test.AutomaticApproval().Enabled()))

	// create 100 MURs that follow the naming pattern used by generateCompliantUsername()
	args = append(args, &v1alpha1.MasterUserRecord{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: test.HostOperatorNs,
			Labels:    map[string]string{v1alpha1.MasterUserRecordOwnerLabelKey: uuid.NewV4().String()},
		},
	})

	for i := 2; i < 101; i++ {
		args = append(args, &v1alpha1.MasterUserRecord{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("foo-%d", i),
				Namespace: test.HostOperatorNs,
				Labels:    map[string]string{v1alpha1.MasterUserRecordOwnerLabelKey: uuid.NewV4().String()},
			},
		})
	}

	args = append(args, baseNSTemplateTier)

	ready := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue))
	r, req, _ := prepareReconcile(t, userSignup.Name, ready, args...)
	InitializeCounters(t, NewToolchainStatus(WithHost(WithMasterUserRecordCount(100))))

	// when
	res, err := r.Reconcile(req)

	// then
	require.Error(t, err)
	assert.EqualError(t, err, "Error generating compliant username for foo@redhat.com: unable to transform username [foo@redhat.com] even after 100 attempts")
	require.Equal(t, reconcile.Result{}, res)

	// Lookup the user signup again
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)
	assert.Equal(t, "approved", userSignup.Labels[v1alpha1.UserSignupStateLabelKey])
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupDeactivatedTotal)
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupApprovedTotal)
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)

	test.AssertConditionsMatch(t, userSignup.Status.Conditions,
		v1alpha1.Condition{
			Type:    v1alpha1.UserSignupComplete,
			Status:  v1.ConditionFalse,
			Reason:  "UnableToCreateMUR",
			Message: "unable to transform username [foo@redhat.com] even after 100 attempts",
		},
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupApproved,
			Status: v1.ConditionTrue,
			Reason: "ApprovedByAdmin",
		},
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupUserDeactivatedNotificationCreated,
			Status: v1.ConditionFalse,
			Reason: "UserIsActive",
		},
	)
	AssertThatCounters(t).HaveMasterUserRecords(100)
}

func TestUserSignupWithMultipleExistingMURNotOK(t *testing.T) {
	// given
	userSignup := NewUserSignup()

	// Create a MUR with the same UserID
	mur := &v1alpha1.MasterUserRecord{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: test.HostOperatorNs,
			Labels:    map[string]string{v1alpha1.MasterUserRecordOwnerLabelKey: userSignup.Name},
		},
	}

	// Create another MUR with the same UserID
	mur2 := &v1alpha1.MasterUserRecord{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bar",
			Namespace: test.HostOperatorNs,
			Labels:    map[string]string{v1alpha1.MasterUserRecordOwnerLabelKey: userSignup.Name},
		},
	}

	ready := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue))
	r, req, _ := prepareReconcile(t, userSignup.Name, ready, userSignup, mur, mur2, NewHostOperatorConfigWithReset(t, test.AutomaticApproval().Enabled()), baseNSTemplateTier)
	InitializeCounters(t, NewToolchainStatus(WithHost(WithMasterUserRecordCount(1))))

	// when
	_, err := r.Reconcile(req)

	// then
	assert.EqualError(t, err, "Multiple MasterUserRecords found: multiple matching MasterUserRecord resources found")

	key := types.NamespacedName{
		Namespace: test.HostOperatorNs,
		Name:      userSignup.Name,
	}
	instance := &v1alpha1.UserSignup{}
	err = r.client.Get(context.TODO(), key, instance)
	require.NoError(t, err)

	test.AssertConditionsMatch(t, instance.Status.Conditions,
		v1alpha1.Condition{
			Type:    v1alpha1.UserSignupComplete,
			Status:  v1.ConditionFalse,
			Reason:  "InvalidMURState",
			Message: "multiple matching MasterUserRecord resources found",
		},
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupUserDeactivatedNotificationCreated,
			Status: v1.ConditionFalse,
			Reason: "UserIsActive",
		},
	)
	AssertThatCounters(t).HaveMasterUserRecords(1)
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupDeactivatedTotal)
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)
}

func TestManuallyApprovedUserSignupWhenNoMembersAvailable(t *testing.T) {
	// given
	userSignup := NewUserSignup(Approved())

	r, req, _ := prepareReconcile(t, userSignup.Name, NewGetMemberClusters(), userSignup, NewHostOperatorConfigWithReset(t, test.AutomaticApproval().Enabled()), baseNSTemplateTier)
	InitializeCounters(t, NewToolchainStatus(WithHost(WithMasterUserRecordCount(1))))

	// when
	_, err := r.Reconcile(req)

	// then
	assert.EqualError(t, err, "no target clusters available: no suitable member cluster found - capacity was reached")
	AssertThatCounters(t).HaveMasterUserRecords(1)

	err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)
	assert.Equal(t, "pending", userSignup.Labels[v1alpha1.UserSignupStateLabelKey])
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupDeactivatedTotal)
	AssertMetricsCounterEquals(t, 0, metrics.UserSignupApprovedTotal)
	AssertMetricsCounterEquals(t, 1, metrics.UserSignupUniqueTotal)

	test.AssertConditionsMatch(t, userSignup.Status.Conditions,
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupApproved,
			Status: v1.ConditionTrue,
			Reason: "ApprovedByAdmin",
		},
		v1alpha1.Condition{
			Type:    v1alpha1.UserSignupComplete,
			Status:  v1.ConditionFalse,
			Reason:  "NoClusterAvailable",
			Message: "no suitable member cluster found - capacity was reached",
		},
		v1alpha1.Condition{
			Type:   v1alpha1.UserSignupUserDeactivatedNotificationCreated,
			Status: v1.ConditionFalse,
			Reason: "UserIsActive",
		})

}

func prepareReconcile(t *testing.T, name string, getMemberClusters cluster.GetMemberClustersFunc, initObjs ...runtime.Object) (*ReconcileUserSignup, reconcile.Request, *test.FakeClient) {
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
	config, err := configuration.LoadConfig(fakeClient)
	require.NoError(t, err)

	r := &ReconcileUserSignup{
		statusUpdater: &statusUpdater{
			client: fakeClient,
		},
		scheme:            s,
		crtConfig:         config,
		getMemberClusters: getMemberClusters,
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
	config, err := configuration.LoadConfig(fakeClient)
	require.NoError(t, err)

	defer counter.Reset()

	// Confirm we have 5 forbidden prefixes by default
	require.Len(t, config.GetForbiddenUsernamePrefixes(), 5)
	names := []string{"-Bob", "-Dave", "Linda", ""}

	for _, prefix := range config.GetForbiddenUsernamePrefixes() {
		userSignup := NewUserSignup(Approved(), WithTargetCluster("east"))
		userSignup.Labels[v1alpha1.UserSignupStateLabelKey] = "not-ready"

		for _, name := range names {
			userSignup.Spec.Username = fmt.Sprintf("%s%s", prefix, name)

			r, req, _ := prepareReconcile(t, userSignup.Name, NewGetMemberClusters(), userSignup, baseNSTemplateTier)
			InitializeCounters(t, NewToolchainStatus(WithHost(WithMasterUserRecordCount(1))))

			// when
			_, err := r.Reconcile(req)
			require.NoError(t, err)

			// then verify that the username has been prefixed - first lookup the UserSignup again
			err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
			require.NoError(t, err)

			// Lookup the MUR
			murs := &v1alpha1.MasterUserRecordList{}
			err = r.client.List(context.TODO(), murs, client.InNamespace(test.HostOperatorNs))
			require.NoError(t, err)
			require.Len(t, murs.Items, 1)
			mur := murs.Items[0]
			require.Equal(t, userSignup.Name, mur.Labels[v1alpha1.MasterUserRecordOwnerLabelKey])
			require.Equal(t, fmt.Sprintf("crt-%s%s", prefix, name), mur.Name)
		}

	}
}

func TestUsernameWithForbiddenSuffixes(t *testing.T) {
	// given
	fakeClient := test.NewFakeClient(t)
	config, err := configuration.LoadConfig(fakeClient)
	require.NoError(t, err)

	defer counter.Reset()

	require.Len(t, config.GetForbiddenUsernameSuffixes(), 1)
	names := []string{"dedicated-", "cluster-", "bob", ""}

	for _, suffix := range config.GetForbiddenUsernameSuffixes() {
		userSignup := NewUserSignup(Approved(), WithTargetCluster("east"))
		userSignup.Labels[v1alpha1.UserSignupStateLabelKey] = "not-ready"

		for _, name := range names {
			userSignup.Spec.Username = fmt.Sprintf("%s%s", name, suffix)

			r, req, _ := prepareReconcile(t, userSignup.Name, NewGetMemberClusters(), userSignup, baseNSTemplateTier)
			InitializeCounters(t, NewToolchainStatus(WithHost(WithMasterUserRecordCount(1))))

			// when
			_, err := r.Reconcile(req)
			require.NoError(t, err)

			// then verify that the username has been suffixed - first lookup the UserSignup again
			err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
			require.NoError(t, err)

			// Lookup the MUR
			murs := &v1alpha1.MasterUserRecordList{}
			err = r.client.List(context.TODO(), murs, client.InNamespace(test.HostOperatorNs))
			require.NoError(t, err)
			require.Len(t, murs.Items, 1)
			mur := murs.Items[0]
			require.Equal(t, userSignup.Name, mur.Labels[v1alpha1.MasterUserRecordOwnerLabelKey])
			require.Equal(t, fmt.Sprintf("%s%s-crt", name, suffix), mur.Name)
		}

	}
}

// Test the scenario where the existing usersignup CompliantUsername becomes outdated eg. transformUsername func is changed
func TestChangedCompliantUsername(t *testing.T) {
	// starting with a UserSignup that exists and was approved and has the now outdated CompliantUsername
	userSignup := NewUserSignup(Approved(), WithTargetCluster("east"))
	userSignup.Status = v1alpha1.UserSignupStatus{
		Conditions: []v1alpha1.Condition{
			{
				Type:   v1alpha1.UserSignupApproved,
				Status: v1.ConditionTrue,
				Reason: v1alpha1.UserSignupApprovedByAdminReason,
			},
			{
				Status: v1.ConditionTrue,
				Type:   v1alpha1.UserSignupComplete,
			},
		},
		CompliantUsername: "foo-old",
	}

	// also starting with the old MUR whose name matches the outdated UserSignup CompliantUsername
	oldMur := &v1alpha1.MasterUserRecord{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo-old",
			Namespace: test.HostOperatorNs,
			Labels:    map[string]string{v1alpha1.MasterUserRecordOwnerLabelKey: userSignup.Name},
		},
	}

	// create the initial resources
	r, req, _ := prepareReconcile(t, userSignup.Name, NewGetMemberClusters(), userSignup, oldMur, baseNSTemplateTier)
	InitializeCounters(t, NewToolchainStatus(WithHost(WithMasterUserRecordCount(1))))

	// 1st reconcile should effectively be a no op because the MUR name and UserSignup CompliantUsername match and status is all good
	res, err := r.Reconcile(req)
	require.NoError(t, err)
	require.Equal(t, reconcile.Result{}, res)

	// after the 1st reconcile verify that the MUR still exists and its name still matches the initial UserSignup CompliantUsername
	murs := &v1alpha1.MasterUserRecordList{}
	err = r.client.List(context.TODO(), murs, client.InNamespace(test.HostOperatorNs))
	require.NoError(t, err)
	require.Len(t, murs.Items, 1)
	mur := murs.Items[0]
	require.Equal(t, userSignup.Name, mur.Labels[v1alpha1.MasterUserRecordOwnerLabelKey])
	require.Equal(t, mur.Name, "foo-old")
	require.Equal(t, userSignup.Status.CompliantUsername, "foo-old")

	// delete the old MUR to trigger creation of a new MUR using the new username
	err = r.client.Delete(context.TODO(), oldMur)
	require.NoError(t, err)

	// 2nd reconcile should handle the deleted MUR and provision a new one
	res, err = r.Reconcile(req)
	require.NoError(t, err)
	require.Equal(t, reconcile.Result{}, res)

	// verify the new MUR is provisioned
	murs = &v1alpha1.MasterUserRecordList{}
	err = r.client.List(context.TODO(), murs)
	require.NoError(t, err)
	require.Len(t, murs.Items, 1)
	mur = murs.Items[0]

	// the MUR name should match the new CompliantUserName
	assert.Equal(t, "foo", mur.Name)
	require.Equal(t, test.HostOperatorNs, mur.Namespace)
	require.Equal(t, userSignup.Name, mur.Labels[v1alpha1.MasterUserRecordOwnerLabelKey])
	require.Len(t, mur.Spec.UserAccounts, 1)
	assert.Equal(t, "base", mur.Spec.UserAccounts[0].Spec.NSTemplateSet.TierName)
	require.Len(t, mur.Spec.UserAccounts[0].Spec.NSTemplateSet.Namespaces, 2)

	// lookup the userSignup and check the conditions are updated but the CompliantUsername is still the old one
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)
	require.Equal(t, userSignup.Status.CompliantUsername, "foo-old")

	// 3rd reconcile should update the CompliantUsername on the UserSignup status
	res, err = r.Reconcile(req)
	require.NoError(t, err)
	require.Equal(t, reconcile.Result{}, res)

	// lookup the userSignup one more time and verify that the CompliantUsername was updated using the current transformUsername logic
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: userSignup.Name, Namespace: req.Namespace}, userSignup)
	require.NoError(t, err)

	// the CompliantUsername and MUR name should now match
	require.Equal(t, userSignup.Status.CompliantUsername, mur.Name)
}

func TestMigrateMur(t *testing.T) {
	// given
	userSignup := NewUserSignup(Approved(), WithTargetCluster("east"))
	mur, err := newMasterUserRecord(baseNSTemplateTier, "foo", test.HostOperatorNs, "east",
		userSignup.Name, userSignup.Spec.UserID)
	require.NoError(t, err)

	// set NSLimit and NSTemplateSet to be empty
	mur.Spec.UserAccounts[0].Spec.NSTemplateSet = v1alpha1.NSTemplateSetSpec{}
	mur.Spec.UserAccounts[0].Spec.NSLimit = ""

	expectedMur := mur.DeepCopy()
	expectedMur.Generation = 1
	expectedMur.ResourceVersion = "1"
	expectedMur.Spec.UserAccounts[0].Spec.NSTemplateSet.TierName = "base"
	expectedMur.Spec.UserAccounts[0].Spec.NSLimit = "default"
	expectedMur.Spec.UserAccounts[0].Spec.NSTemplateSet.Namespaces = []v1alpha1.NSTemplateSetNamespace{
		{
			TemplateRef: "base-dev-123abc1",
		},
		{
			TemplateRef: "base-stage-123abc2",
		},
	}
	expectedMur.Spec.UserAccounts[0].Spec.NSTemplateSet.ClusterResources = &v1alpha1.NSTemplateSetClusterResources{
		TemplateRef: "base-clusterresources-654321b",
	}

	t.Run("add missing tierName and nsLimit fields", func(t *testing.T) {
		// given
		r, req, _ := prepareReconcile(t, userSignup.Name, NewGetMemberClusters(), userSignup, baseNSTemplateTier, mur)
		InitializeCounters(t, NewToolchainStatus(WithHost(WithMasterUserRecordCount(0))))

		// when
		_, err := r.Reconcile(req)
		// then verify that the MUR exists and is complete
		require.NoError(t, err)
		murs := &v1alpha1.MasterUserRecordList{}
		err = r.client.List(context.TODO(), murs)
		require.NoError(t, err)
		require.Len(t, murs.Items, 1)
		assert.Equal(t, *expectedMur, murs.Items[0])
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
