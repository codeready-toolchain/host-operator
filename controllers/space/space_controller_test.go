package space_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/controllers/space"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	"github.com/codeready-toolchain/host-operator/pkg/cluster"
	"github.com/codeready-toolchain/host-operator/pkg/metrics"
	. "github.com/codeready-toolchain/host-operator/test"
	tiertest "github.com/codeready-toolchain/host-operator/test/nstemplatetier"
	spacebindingtest "github.com/codeready-toolchain/host-operator/test/spacebinding"
	commoncluster "github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	"github.com/codeready-toolchain/toolchain-common/pkg/hash"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	murtest "github.com/codeready-toolchain/toolchain-common/pkg/test/masteruserrecord"
	nstemplatetsettest "github.com/codeready-toolchain/toolchain-common/pkg/test/nstemplateset"
	spacetest "github.com/codeready-toolchain/toolchain-common/pkg/test/space"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestCreateSpace(t *testing.T) {

	// given
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	err := apis.AddToScheme(scheme.Scheme)
	require.NoError(t, err)
	base1nsTier := tiertest.Base1nsTier(t, tiertest.CurrentBase1nsTemplates)
	t.Run("success", func(t *testing.T) {
		type testRun struct {
			name           string
			featureToggles string
		}

		tests := []testRun{
			{
				name: "without feature toggles",
			},
			{
				name:           "with feature toggles",
				featureToggles: "feature-1,feature-2",
			},
		}
		for _, testRun := range tests {
			t.Run(testRun.name, func(t *testing.T) {
				spaceOptions := []spacetest.Option{spacetest.WithSpecTargetCluster("member-1")}
				if testRun.featureToggles != "" {
					spaceOptions = append(spaceOptions, spacetest.WithAnnotation(toolchainv1alpha1.FeatureToggleNameAnnotationKey, testRun.featureToggles))
				}

				// given
				s := spacetest.NewSpace(test.HostOperatorNs, "oddity", spaceOptions...)
				hostClient := test.NewFakeClient(t, s, base1nsTier)
				member1 := NewMemberClusterWithTenantRole(t, "member-1", corev1.ConditionTrue)
				member2 := NewMemberClusterWithTenantRole(t, "member-2", corev1.ConditionTrue)
				InitializeCounters(t,
					NewToolchainStatus())
				ctrl := newReconciler(hostClient, member1, member2)

				// when
				res, err := ctrl.Reconcile(context.TODO(), requestFor(s))

				// then
				require.NoError(t, err)
				assert.True(t, res.Requeue) // requeue requested explicitly when NSTemplateSet is created, even though watching the resource is enough to trigger a new reconcile loop
				spacetest.AssertThatSpace(t, test.HostOperatorNs, "oddity", hostClient).
					Exists().
					HasStatusTargetCluster("member-1").
					HasConditions(spacetest.Provisioning()).
					HasStateLabel("cluster-assigned").
					HasFinalizer()
				var nsTmplSet *toolchainv1alpha1.NSTemplateSet
				nsTmplSetAssertion := nstemplatetsettest.AssertThatNSTemplateSet(t, test.MemberOperatorNs, "oddity", member1.Client).
					Exists().
					HasTierName(base1nsTier.Name).
					HasClusterResourcesTemplateRef("base1ns-clusterresources-123456new").
					HasNamespaceTemplateRefs("base1ns-code-123456new", "base1ns-dev-123456new", "base1ns-stage-123456new")
				if testRun.featureToggles != "" {
					nsTmplSetAssertion = nsTmplSetAssertion.
						HasAnnotationWithValue(toolchainv1alpha1.FeatureToggleNameAnnotationKey, testRun.featureToggles)
				} else {
					nsTmplSetAssertion = nsTmplSetAssertion.
						DoesNotHaveAnnotation(toolchainv1alpha1.FeatureToggleNameAnnotationKey)
				}
				nsTmplSet = nsTmplSetAssertion.Get()
				AssertThatCountersAndMetrics(t).
					HaveSpacesForCluster("member-1", 1).
					HaveSpacesForCluster("member-2", 0) // check that increment for member-1 was called

				t.Run("requeue while NSTemplateSet is not ready", func(t *testing.T) {
					// given another round of requeue without while NSTemplateSet is *not ready*
					nsTmplSet.Status.Conditions = []toolchainv1alpha1.Condition{
						nstemplatetsettest.Provisioning(),
					}
					err := member1.Client.Update(context.TODO(), nsTmplSet)
					require.NoError(t, err)
					ctrl := newReconciler(hostClient, member1, member2)

					// when
					res, err := ctrl.Reconcile(context.TODO(), requestFor(s))

					// then
					require.NoError(t, err)
					assert.False(t, res.Requeue)
					spacetest.AssertThatSpace(t, test.HostOperatorNs, "oddity", hostClient).
						Exists().
						HasStatusTargetCluster("member-1").
						HasConditions(spacetest.Provisioning()).
						HasStateLabel("cluster-assigned").
						HasFinalizer()
					nsTmplSet := nstemplatetsettest.AssertThatNSTemplateSet(t, test.MemberOperatorNs, "oddity", member1.Client).
						Exists().
						HasClusterResourcesTemplateRef("base1ns-clusterresources-123456new").
						HasNamespaceTemplateRefs("base1ns-code-123456new", "base1ns-dev-123456new", "base1ns-stage-123456new").
						Get()
					AssertThatCountersAndMetrics(t).
						HaveSpacesForCluster("member-1", 1).
						HaveSpacesForCluster("member-2", 0) // nothing has changed since previous increment

					t.Run("when NSTemplateSet is ready update the provisioned namespace list", func(t *testing.T) {
						// given another round of requeue without with NSTemplateSet now *ready*
						nsTmplSet.Status.Conditions = []toolchainv1alpha1.Condition{
							nstemplatetsettest.Provisioned(),
						}
						nsTmplSet.Status.ProvisionedNamespaces = []toolchainv1alpha1.SpaceNamespace{
							{
								Name: "john-dev",
								Type: "default",
							},
							{
								Name: "john-stage",
							},
						}
						err := member1.Client.Update(context.TODO(), nsTmplSet)
						require.NoError(t, err)
						ctrl := newReconciler(hostClient, member1, member2)

						// when
						_, err = ctrl.Reconcile(context.TODO(), requestFor(s))

						// then
						require.NoError(t, err)
						assert.Equal(t, reconcile.Result{Requeue: false}, res) // no requeue and status with new provisioned namespaces was updated
						spacetest.AssertThatSpace(t, test.HostOperatorNs, "oddity", hostClient).
							Exists().
							HasStatusTargetCluster("member-1").
							HasStatusProvisionedNamespaces(nsTmplSet.Status.ProvisionedNamespaces).
							HasConditions(spacetest.Ready()). // space is still provisioning
							HasStateLabel("cluster-assigned").
							HasFinalizer()
						AssertThatCountersAndMetrics(t).
							HaveSpacesForCluster("member-1", 1).
							HaveSpacesForCluster("member-2", 0) // space counter unchanged

						t.Run("done when provisioned namespace list is up to date", func(t *testing.T) {
							// given another round of requeue with updated list of provisioned namespaces in status
							// when
							res, err := ctrl.Reconcile(context.TODO(), requestFor(s))

							// then
							require.NoError(t, err)
							assert.Equal(t, reconcile.Result{Requeue: false}, res) // no more requeue.
							spacetest.AssertThatSpace(t, test.HostOperatorNs, "oddity", hostClient).
								Exists().
								HasStatusTargetCluster("member-1").
								HasStatusProvisionedNamespaces(nsTmplSet.Status.ProvisionedNamespaces).
								HasConditions(spacetest.Ready()). // space is now ready
								HasStateLabel("cluster-assigned").
								HasFinalizer()
							AssertThatCountersAndMetrics(t).
								HaveSpacesForCluster("member-1", 1).
								HaveSpacesForCluster("member-2", 0) // space counter unchanged
						})

					})
				})

				t.Run("unspecified target member cluster", func(t *testing.T) {
					// given
					s := spacetest.NewSpace(test.HostOperatorNs, "oddity")
					hostClient := test.NewFakeClient(t, s)
					member1 := NewMemberClusterWithTenantRole(t, "member-1", corev1.ConditionTrue)
					member2 := NewMemberClusterWithTenantRole(t, "member-2", corev1.ConditionTrue)
					ctrl := newReconciler(hostClient, member1, member2)
					InitializeCounters(t,
						NewToolchainStatus())

					// when
					res, err := ctrl.Reconcile(context.TODO(), requestFor(s))

					// then
					require.NoError(t, err) // the lack of target member cluster is valid, hence no error is returned
					assert.False(t, res.Requeue)
					spacetest.AssertThatSpace(t, test.HostOperatorNs, s.Name, hostClient).
						HasNoStatusTargetCluster().
						HasStateLabel("pending").
						HasConditions(spacetest.ProvisioningPending("unspecified target member cluster")) // the Space will remain in `ProvisioningPending` until a target member cluster is set.
					AssertThatCountersAndMetrics(t).
						HaveSpacesForCluster("member-1", 0).
						HaveSpacesForCluster("member-2", 0) // no counters since `spec.TargetCluster` is not specified
				})

				t.Run("unspecified tierName", func(t *testing.T) {
					// given
					s := spacetest.NewSpace(test.HostOperatorNs, "oddity", spacetest.WithTierName(""))
					hostClient := test.NewFakeClient(t, s)
					member1 := NewMemberClusterWithTenantRole(t, "member-1", corev1.ConditionTrue)
					member2 := NewMemberClusterWithTenantRole(t, "member-2", corev1.ConditionTrue)
					ctrl := newReconciler(hostClient, member1, member2)
					InitializeCounters(t,
						NewToolchainStatus())

					// when
					res, err := ctrl.Reconcile(context.TODO(), requestFor(s))

					// then
					require.NoError(t, err) // the lack of tierName is valid, hence no error is returned
					assert.False(t, res.Requeue)
					spacetest.AssertThatSpace(t, test.HostOperatorNs, s.Name, hostClient).
						HasNoStatusTargetCluster().
						HasStateLabel("pending").
						HasConditions(spacetest.ProvisioningPending("unspecified tier name")) // the Space will remain in `ProvisioningPending` until a tierName is set.
					AssertThatCountersAndMetrics(t).
						HaveSpacesForCluster("member-1", 0).
						HaveSpacesForCluster("member-2", 0) // no counters since `spec.TargetCluster` is not specified
				})
				t.Run("update parent-space label with parentSpace spec field", func(t *testing.T) {
					// given
					subSpace := spacetest.NewSpace(test.HostOperatorNs, "subSpace",
						spacetest.WithTierName(base1nsTier.Name),
						spacetest.WithSpecTargetCluster("member-1"),
						spacetest.WithStatusTargetCluster("member-1"),
						spacetest.WithSpecParentSpace("parentSpace"))

					nsTmplSet := nstemplatetsettest.NewNSTemplateSet("subSpace", nstemplatetsettest.WithReadyCondition(), nstemplatetsettest.WithReferencesFor(base1nsTier))
					memberClient := test.NewFakeClient(t, nsTmplSet)
					member := NewMemberClusterWithClient(memberClient, "member-1", corev1.ConditionTrue)
					hostClient := test.NewFakeClient(t, subSpace, base1nsTier)
					ctrl := newReconciler(hostClient, member)

					// when
					_, err := ctrl.Reconcile(context.TODO(), requestFor(subSpace))

					// then
					require.NoError(t, err)
					spacetest.AssertThatSpace(t, subSpace.Namespace, subSpace.Name, hostClient).
						Exists().
						HasStatusTargetCluster("member-1").
						HasConditions(spacetest.Ready()).
						HasStateLabel("cluster-assigned").
						HasLabelWithValue(toolchainv1alpha1.ParentSpaceLabelKey, "parentSpace"). // check that the parent-space label was set according to spec.ParentSpace field value
						HasFinalizer()
				})

				t.Run("without parentSpace spec field", func(t *testing.T) {
					// given
					subSpace := spacetest.NewSpace(test.HostOperatorNs, "myspace",
						spacetest.WithTierName(base1nsTier.Name),
						spacetest.WithSpecTargetCluster("member-1"),
						spacetest.WithStatusTargetCluster("member-1"))

					nsTmplSet := nstemplatetsettest.NewNSTemplateSet("myspace", nstemplatetsettest.WithReadyCondition(), nstemplatetsettest.WithReferencesFor(base1nsTier))
					memberClient := test.NewFakeClient(t, nsTmplSet)
					member := NewMemberClusterWithClient(memberClient, "member-1", corev1.ConditionTrue)
					hostClient := test.NewFakeClient(t, subSpace, base1nsTier)
					ctrl := newReconciler(hostClient, member)

					// when
					_, err := ctrl.Reconcile(context.TODO(), requestFor(subSpace))

					// then
					require.NoError(t, err)
					spacetest.AssertThatSpace(t, subSpace.Namespace, subSpace.Name, hostClient).
						Exists().
						HasStatusTargetCluster("member-1").
						HasConditions(spacetest.Ready()).
						HasStateLabel("cluster-assigned").
						DoesNotHaveLabel(toolchainv1alpha1.ParentSpaceLabelKey). // check that the parent-space label is not present
						HasFinalizer()
				})
			})
		}
	})

	t.Run("failure", func(t *testing.T) {

		t.Run("space not found", func(t *testing.T) {
			// given
			hostClient := test.NewFakeClient(t)
			member1 := NewMemberClusterWithTenantRole(t, "member-1", corev1.ConditionTrue)
			member2 := NewMemberClusterWithTenantRole(t, "member-2", corev1.ConditionTrue)
			ctrl := newReconciler(hostClient, member1, member2)
			InitializeCounters(t,
				NewToolchainStatus())

			// when
			res, err := ctrl.Reconcile(context.TODO(), requestFor(nil))

			// then
			require.NoError(t, err) // not an error, space simply doesn't exist :shrug:
			assert.False(t, res.Requeue)
			AssertThatCountersAndMetrics(t).
				HaveSpacesForCluster("member-1", 0).
				HaveSpacesForCluster("member-2", 0) // no space is created
		})

		t.Run("error while getting space", func(t *testing.T) {
			// given
			s := spacetest.NewSpace(test.HostOperatorNs, "oddity")
			hostClient := test.NewFakeClient(t, s)
			hostClient.MockGet = func(ctx context.Context, key runtimeclient.ObjectKey, obj runtimeclient.Object, opts ...runtimeclient.GetOption) error {
				if _, ok := obj.(*toolchainv1alpha1.Space); ok {
					return fmt.Errorf("mock error")
				}
				return hostClient.Client.Get(ctx, key, obj, opts...)
			}
			member1 := NewMemberClusterWithTenantRole(t, "member-1", corev1.ConditionTrue)
			member2 := NewMemberClusterWithTenantRole(t, "member-2", corev1.ConditionTrue)
			ctrl := newReconciler(hostClient, member1, member2)
			InitializeCounters(t,
				NewToolchainStatus())

			// when
			res, err := ctrl.Reconcile(context.TODO(), requestFor(s))

			// then
			require.EqualError(t, err, "unable to get the current Space: mock error")
			assert.False(t, res.Requeue)
			AssertThatCountersAndMetrics(t).
				HaveSpacesForCluster("member-1", 0).
				HaveSpacesForCluster("member-2", 0) // no space is created since `spec.TargetCluster` field is not set
		})

		t.Run("error while adding finalizer", func(t *testing.T) {
			// given
			s := spacetest.NewSpace(test.HostOperatorNs, "oddity")
			hostClient := test.NewFakeClient(t, s)
			hostClient.MockUpdate = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.UpdateOption) error {
				if _, ok := obj.(*toolchainv1alpha1.Space); ok {
					return fmt.Errorf("mock error")
				}
				return hostClient.Client.Update(ctx, obj, opts...)
			}
			member1 := NewMemberClusterWithTenantRole(t, "member-1", corev1.ConditionTrue)
			member2 := NewMemberClusterWithTenantRole(t, "member-2", corev1.ConditionTrue)
			ctrl := newReconciler(hostClient, member1, member2)
			InitializeCounters(t,
				NewToolchainStatus())

			// when
			res, err := ctrl.Reconcile(context.TODO(), requestFor(s))

			// then
			require.EqualError(t, err, "mock error")
			assert.False(t, res.Requeue)
			AssertThatCountersAndMetrics(t).
				HaveSpacesForCluster("member-1", 0).
				HaveSpacesForCluster("member-2", 0) // no space is created since `spec.TargetCluster` field is not set
		})

		t.Run("unknown target member cluster", func(t *testing.T) {
			// given
			s := spacetest.NewSpace(test.HostOperatorNs, "oddity", spacetest.WithSpecTargetCluster("unknown"))
			hostClient := test.NewFakeClient(t, s)
			member1 := NewMemberClusterWithTenantRole(t, "member-1", corev1.ConditionTrue)
			member2 := NewMemberClusterWithTenantRole(t, "member-2", corev1.ConditionTrue)
			ctrl := newReconciler(hostClient, member1, member2)
			InitializeCounters(t,
				NewToolchainStatus())

			// when
			res, err := ctrl.Reconcile(context.TODO(), requestFor(s))

			// then
			require.EqualError(t, err, "unknown target member cluster 'unknown'")
			assert.False(t, res.Requeue)
			spacetest.AssertThatSpace(t, test.HostOperatorNs, s.Name, hostClient).
				HasStatusTargetCluster(""). // has empty target cluster since it wasn't provisioned
				HasStateLabel("cluster-assigned").
				HasConditions(spacetest.ProvisioningFailed("unknown target member cluster 'unknown'"))
			AssertThatCountersAndMetrics(t).
				HaveSpacesForCluster("member-1", 0).
				HaveSpacesForCluster("member-2", 0) // no counters are set since target cluster is invalid
		})

		t.Run("error while getting NSTemplateTier on host cluster", func(t *testing.T) {
			// given
			s := spacetest.NewSpace(test.HostOperatorNs, "oddity",
				spacetest.WithSpecTargetCluster("member-1"),
				spacetest.WithFinalizer())
			hostClient := test.NewFakeClient(t, s)
			hostClient.MockGet = func(ctx context.Context, key runtimeclient.ObjectKey, obj runtimeclient.Object, opts ...runtimeclient.GetOption) error {
				if _, ok := obj.(*toolchainv1alpha1.NSTemplateTier); ok {
					return fmt.Errorf("mock error")
				}
				return hostClient.Client.Get(ctx, key, obj, opts...)
			}
			member1 := NewMemberClusterWithTenantRole(t, "member-1", corev1.ConditionTrue)
			member2 := NewMemberClusterWithTenantRole(t, "member-2", corev1.ConditionTrue)
			ctrl := newReconciler(hostClient, member1, member2)
			InitializeCounters(t,
				NewToolchainStatus())

			// when
			res, err := ctrl.Reconcile(context.TODO(), requestFor(s))

			// then
			require.EqualError(t, err, "mock error")
			assert.False(t, res.Requeue)
			spacetest.AssertThatSpace(t, test.HostOperatorNs, s.Name, hostClient).
				HasSpecTargetCluster("member-1").
				HasConditions(spacetest.ProvisioningFailed("mock error"))
			AssertThatCountersAndMetrics(t).
				HaveSpacesForCluster("member-1", 0).
				HaveSpacesForCluster("member-2", 0) // no counters increment when there is an error on NSTemplateTier
		})

		t.Run("error while getting NSTemplateSet on member cluster", func(t *testing.T) {
			// given
			s := spacetest.NewSpace(test.HostOperatorNs, "oddity",
				spacetest.WithSpecTargetCluster("member-1"),
				spacetest.WithFinalizer())
			hostClient := test.NewFakeClient(t, s, base1nsTier)
			member1Client := test.NewFakeClient(t)
			member1Client.MockGet = func(ctx context.Context, key runtimeclient.ObjectKey, obj runtimeclient.Object, opts ...runtimeclient.GetOption) error {
				if _, ok := obj.(*toolchainv1alpha1.NSTemplateSet); ok {
					return fmt.Errorf("mock error")
				}
				return member1Client.Client.Get(ctx, key, obj, opts...)
			}
			member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
			member2 := NewMemberClusterWithTenantRole(t, "member-2", corev1.ConditionTrue)
			ctrl := newReconciler(hostClient, member1, member2)
			InitializeCounters(t,
				NewToolchainStatus())

			// when
			res, err := ctrl.Reconcile(context.TODO(), requestFor(s))

			// then
			require.EqualError(t, err, "mock error")
			assert.False(t, res.Requeue)
			spacetest.AssertThatSpace(t, test.HostOperatorNs, s.Name, hostClient).
				HasStatusTargetCluster("member-1").
				HasConditions(spacetest.UnableToCreateNSTemplateSet("mock error"))
			AssertThatCountersAndMetrics(t).
				HaveSpacesForCluster("member-1", 0).
				HaveSpacesForCluster("member-2", 0) // no counters increment when there is an error on NSTemplateSet
		})

		t.Run("error while creating NSTemplateSet on member cluster", func(t *testing.T) {
			// given
			s := spacetest.NewSpace(test.HostOperatorNs, "oddity",
				spacetest.WithSpecTargetCluster("member-1"),
				spacetest.WithFinalizer())
			hostClient := test.NewFakeClient(t, s, base1nsTier)
			member1Client := test.NewFakeClient(t)
			member1Client.MockCreate = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.CreateOption) error {
				if _, ok := obj.(*toolchainv1alpha1.NSTemplateSet); ok {
					return fmt.Errorf("mock error")
				}
				return member1Client.Client.Create(ctx, obj, opts...)
			}
			member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
			member2 := NewMemberClusterWithTenantRole(t, "member-2", corev1.ConditionTrue)
			ctrl := newReconciler(hostClient, member1, member2)
			InitializeCounters(t,
				NewToolchainStatus())

			// when
			res, err := ctrl.Reconcile(context.TODO(), requestFor(s))

			// then
			require.EqualError(t, err, "mock error")
			assert.Equal(t, reconcile.Result{Requeue: false}, res)
			spacetest.AssertThatSpace(t, test.HostOperatorNs, s.Name, hostClient).
				HasStatusTargetCluster("member-1").
				HasConditions(spacetest.UnableToCreateNSTemplateSet("mock error"))
			AssertThatCountersAndMetrics(t).
				HaveSpacesForCluster("member-1", 0).
				HaveSpacesForCluster("member-2", 0) // no counters increment when there is an error on NSTemplateSet
		})

		t.Run("error while updating status after creating NSTemplateSet", func(t *testing.T) {
			// given
			s := spacetest.NewSpace(test.HostOperatorNs, "oddity",
				spacetest.WithSpecTargetCluster("member-1"),
				spacetest.WithFinalizer())
			hostClient := test.NewFakeClient(t, s, base1nsTier)
			hostClient.MockStatusUpdate = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.UpdateOption) error {
				if _, ok := obj.(*toolchainv1alpha1.Space); ok {
					return fmt.Errorf("mock error")
				}
				return hostClient.Client.Status().Update(ctx, obj, opts...)
			}
			member1 := NewMemberClusterWithTenantRole(t, "member-1", corev1.ConditionTrue)
			member2 := NewMemberClusterWithTenantRole(t, "member-2", corev1.ConditionTrue)
			ctrl := newReconciler(hostClient, member1, member2)
			InitializeCounters(t,
				NewToolchainStatus())

			// when
			res, err := ctrl.Reconcile(context.TODO(), requestFor(s))

			// then
			require.EqualError(t, err, "mock error")
			assert.Equal(t, reconcile.Result{Requeue: false}, res)
			spacetest.AssertThatSpace(t, test.HostOperatorNs, s.Name, hostClient).
				HasNoStatusTargetCluster(). // not set
				HasNoConditions()           // not set
			AssertThatCountersAndMetrics(t).
				HaveSpacesForCluster("member-1", 0).
				HaveSpacesForCluster("member-2", 0) // no counters are incremented
		})

		t.Run("error while setting provisioned namespace list", func(t *testing.T) {
			// given
			s := spacetest.NewSpace(test.HostOperatorNs, "john",
				spacetest.WithSpecTargetCluster("member-1"),
				spacetest.WithFinalizer())
			hostClient := test.NewFakeClient(t, s, base1nsTier)
			nsTmplSet := nstemplatetsettest.NewNSTemplateSet("john", nstemplatetsettest.WithReadyCondition(), nstemplatetsettest.WithReferencesFor(base1nsTier))
			member1Client := test.NewFakeClient(t, nsTmplSet)
			hostClient.MockStatusUpdate = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.UpdateOption) error {
				if space, ok := obj.(*toolchainv1alpha1.Space); ok {
					if len(space.Status.ProvisionedNamespaces) > 0 {
						return fmt.Errorf("update error")
					}
				}
				return hostClient.Client.Status().Update(ctx, obj, opts...)
			}
			member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
			// set list of namespace in NSTemplateSet so that they can be copied to the space.status
			nsTmplSet.Status.ProvisionedNamespaces = []toolchainv1alpha1.SpaceNamespace{
				{
					Name: "john-dev",
					Type: "default",
				},
				{
					Name: "john-stage",
				},
			}
			err := member1.Client.Update(context.TODO(), nsTmplSet)
			require.NoError(t, err)
			ctrl := newReconciler(hostClient, member1)

			// when
			res, err := ctrl.Reconcile(context.TODO(), requestFor(s))

			// then
			require.EqualError(t, err, "error setting provisioned namespaces: update error")
			assert.False(t, res.Requeue)
			spacetest.AssertThatSpace(t, test.HostOperatorNs, s.Name, hostClient).
				HasStatusProvisionedNamespaces([]toolchainv1alpha1.SpaceNamespace(nil)) // unable to set list of namespaces
		})
	})
}

func TestDeleteSpace(t *testing.T) {

	// given
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	err := apis.AddToScheme(scheme.Scheme)
	require.NoError(t, err)
	base1nsTier := tiertest.Base1nsTier(t, tiertest.CurrentBase1nsTemplates)

	t.Run("after space was successfully provisioned", func(t *testing.T) {

		// given a space that is being deleted
		s := spacetest.NewSpace(test.HostOperatorNs, "oddity",
			spacetest.WithDeletionTimestamp(), // deletion was requested
			spacetest.WithFinalizer(),
			spacetest.WithSpecTargetCluster("member-1"),
			spacetest.WithStatusTargetCluster("member-1"))
		nsTmplSet := nstemplatetsettest.NewNSTemplateSet("oddity", nstemplatetsettest.WithReadyCondition())

		t.Run("Space controller deletes NSTemplateSet", func(t *testing.T) {
			// given
			InitializeCounters(t, NewToolchainStatus(WithEmptyMetrics(), WithMember("member-1", WithSpaceCount(1))))
			hostClient := test.NewFakeClient(t, s, base1nsTier)
			memberClient := test.NewFakeClient(t, nsTmplSet)
			member := NewMemberClusterWithClient(memberClient, "member-1", corev1.ConditionTrue)
			ctrl := newReconciler(hostClient, member)

			// when
			res, err := ctrl.Reconcile(context.TODO(), requestFor(s))

			// then
			require.NoError(t, err)
			assert.Equal(t, reconcile.Result{Requeue: false}, res) // no need to explicitly requeue while the NSTemplate is terminating
			spacetest.AssertThatSpace(t, s.Namespace, s.Name, hostClient).
				Exists().
				HasFinalizer(). // finalizer is still present while the NSTemplateSet is not fully deleted
				HasStatusTargetCluster("member-1").
				HasConditions(spacetest.Terminating())
			nstemplatetsettest.AssertThatNSTemplateSet(t, test.MemberOperatorNs, "oddity", member.Client).
				DoesNotExist()
			AssertThatCountersAndMetrics(t).
				HaveSpacesForCluster("member-1", 0) // counter is decremented
		})

		t.Run("when NSTemplateSet is being deleted and in terminating state", func(t *testing.T) {
			// given
			InitializeCounters(t, NewToolchainStatus(WithEmptyMetrics(), WithMember("member-1", WithSpaceCount(1))))
			nsTmplSet := nstemplatetsettest.NewNSTemplateSet("oddity", nstemplatetsettest.WithDeletionTimestamp(time.Now()), func(templateSet *toolchainv1alpha1.NSTemplateSet) {
				templateSet.Status.Conditions = []toolchainv1alpha1.Condition{
					nstemplatetsettest.Terminating(),
				}
				// we need to set the finalizer, otherwise, the FakeClient would delete the object immediately
				templateSet.Finalizers = []string{"kubernetes"}
			})
			hostClient := test.NewFakeClient(t, s, base1nsTier)
			memberClient := test.NewFakeClient(t, nsTmplSet)
			member := NewMemberClusterWithClient(memberClient, "member-1", corev1.ConditionTrue)
			ctrl := newReconciler(hostClient, member)

			// when
			res, err := ctrl.Reconcile(context.TODO(), requestFor(s))

			// then
			require.NoError(t, err)
			assert.Equal(t, reconcile.Result{Requeue: false}, res) // no need to explicitly requeue while the NSTemplate is terminating
			// no changes
			spacetest.AssertThatSpace(t, s.Namespace, s.Name, hostClient).
				Exists().
				HasFinalizer().
				HasStatusTargetCluster("member-1").
				HasConditions(spacetest.Terminating())
			nstemplatetsettest.AssertThatNSTemplateSet(t, test.MemberOperatorNs, "oddity", member.Client).
				Exists().
				HasDeletionTimestamp().
				HasConditions(nstemplatetsettest.Terminating())
			AssertThatCountersAndMetrics(t).
				HaveSpacesForCluster("member-1", 1) // counter for space is not decremented because the NSTemplateSet is already being deleted
		})

		t.Run("when using status target cluster", func(t *testing.T) {
			// given
			s := spacetest.NewSpace(test.HostOperatorNs, "oddity",
				spacetest.WithoutSpecTargetCluster(),          // targetCluster is not specified in spec ...
				spacetest.WithStatusTargetCluster("member-1"), // ... but is available in status
				spacetest.WithFinalizer(),
				spacetest.WithDeletionTimestamp())
			hostClient := test.NewFakeClient(t, s, base1nsTier)
			nstmplSet := nstemplatetsettest.NewNSTemplateSet("oddity", nstemplatetsettest.WithReadyCondition())
			member1Client := test.NewFakeClient(t, nstmplSet)
			member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
			member2 := NewMemberClusterWithTenantRole(t, "member-2", corev1.ConditionTrue)
			ctrl := newReconciler(hostClient, member1, member2)
			InitializeCounters(t, NewToolchainStatus())

			// when
			res, err := ctrl.Reconcile(context.TODO(), requestFor(s))

			// then
			require.NoError(t, err)
			assert.Equal(t, reconcile.Result{Requeue: false}, res) // no need to explicitly requeue while the NSTemplate is terminating
			spacetest.AssertThatSpace(t, s.Namespace, s.Name, hostClient).
				Exists().
				HasStatusTargetCluster("member-1").
				HasConditions(spacetest.Terminating()).
				HasFinalizer() // finalizer is still present while the NSTemplateSet is not fully deleted
			nstemplatetsettest.AssertThatNSTemplateSet(t, test.MemberOperatorNs, "oddity", member1.Client).
				DoesNotExist()
			AssertThatCountersAndMetrics(t).
				HaveSpacesForCluster("member-1", 0) // space counter is not incremented when `.spec.TargetCluster` is not set
			// stop the test here: it verified that the NSTemplateSet deletion was triggered (the rest is already covered above)
		})

		t.Run("without spec and status target cluster", func(t *testing.T) {
			// given
			s := spacetest.NewSpace(test.HostOperatorNs, "oddity",
				spacetest.WithFinalizer(),
				spacetest.WithDeletionTimestamp())
			hostClient := test.NewFakeClient(t, s, base1nsTier)
			ctrl := newReconciler(hostClient)
			InitializeCounters(t, NewToolchainStatus())

			// when
			_, err := ctrl.Reconcile(context.TODO(), reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: s.Namespace,
					Name:      s.Name,
				},
			})

			// then
			require.NoError(t, err)
			// finalizer was removed
			spacetest.AssertThatSpace(t, s.Namespace, s.Name, hostClient).
				DoesNotExist()
			AssertThatCountersAndMetrics(t).
				HaveSpacesForCluster("member-1", 0) // space counter is not incremented when `.spec.TargetCluster` is not set
		})

		t.Run("when nstemplateset is gone, then it should remove the finalizer and the client automatically delete the Space", func(t *testing.T) {
			// given
			s := spacetest.NewSpace(test.HostOperatorNs, "delete-space",
				spacetest.WithoutSpecTargetCluster(),          // targetCluster is not specified in spec ...
				spacetest.WithStatusTargetCluster("member-1"), // ... but is available in status
				spacetest.WithDeletionTimestamp(),
				spacetest.WithFinalizer())
			hostClient := test.NewFakeClient(t, s, base1nsTier)
			member1Client := test.NewFakeClient(t)
			member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
			member2 := NewMemberClusterWithTenantRole(t, "member-2", corev1.ConditionTrue)
			ctrl := newReconciler(hostClient, member1, member2)
			InitializeCounters(t, NewToolchainStatus(WithEmptyMetrics(), WithMember("member-1", WithSpaceCount(1))))

			// when
			_, err := ctrl.Reconcile(context.TODO(), reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: s.Namespace,
					Name:      s.Name,
				},
			})

			// then
			require.NoError(t, err)
			// finalizer was removed
			spacetest.AssertThatSpace(t, s.Namespace, s.Name, hostClient).
				DoesNotExist()
			AssertThatCountersAndMetrics(t).
				HaveSpacesForCluster("member-1", 1) // space counter is not decremented
		})
	})

	t.Run("when space was not successfully provisioned", func(t *testing.T) {

		t.Run("because of missing target member cluster", func(t *testing.T) {
			// given
			s := spacetest.NewSpace(test.HostOperatorNs, "oddity",
				spacetest.WithoutSpecTargetCluster(),
				spacetest.WithFinalizer(),
				spacetest.WithDeletionTimestamp(),
				spacetest.WithCondition(spacetest.ProvisioningFailed("missing target member cluster")),
			)
			hostClient := test.NewFakeClient(t, s, base1nsTier)
			member1 := NewMemberClusterWithTenantRole(t, "member-1", corev1.ConditionTrue)
			member2 := NewMemberClusterWithTenantRole(t, "member-2", corev1.ConditionTrue)
			ctrl := newReconciler(hostClient, member1, member2)
			InitializeCounters(t,
				NewToolchainStatus())

			// when
			res, err := ctrl.Reconcile(context.TODO(), requestFor(s))

			// then
			require.NoError(t, err)
			assert.Equal(t, reconcile.Result{Requeue: false}, res) // no requeue needed
			spacetest.AssertThatSpace(t, s.Namespace, s.Name, hostClient).
				DoesNotExist()
			nstemplatetsettest.AssertThatNSTemplateSet(t, test.MemberOperatorNs, "oddity", member1.Client).
				DoesNotExist()
			AssertThatCountersAndMetrics(t).
				HaveSpacesForCluster("member-1", 0).
				HaveSpacesForCluster("member-2", 0) // space counter is not incremented when `.spec.TargetCluster` is not set
		})

		t.Run("because of unknown target member cluster", func(t *testing.T) {
			// given
			s := spacetest.NewSpace(test.HostOperatorNs, "oddity",
				spacetest.WithSpecTargetCluster("member-3"),
				spacetest.WithStatusTargetCluster("member-3"), // assume that Space was provisioned on a cluster which is now missing
				spacetest.WithFinalizer(),
				spacetest.WithDeletionTimestamp(),
				spacetest.WithCondition(spacetest.ProvisioningFailed("unknown target member cluster 'member-3'")),
			)
			hostClient := test.NewFakeClient(t, s, base1nsTier)
			member1 := NewMemberClusterWithTenantRole(t, "member-1", corev1.ConditionTrue)
			member2 := NewMemberClusterWithTenantRole(t, "member-2", corev1.ConditionTrue)
			ctrl := newReconciler(hostClient, member1, member2)
			InitializeCounters(t,
				NewToolchainStatus())

			// when
			res, err := ctrl.Reconcile(context.TODO(), requestFor(s))

			// then
			require.EqualError(t, err, "cannot delete NSTemplateSet: unknown target member cluster: 'member-3'")
			assert.Equal(t, reconcile.Result{Requeue: false}, res) // no requeue needed
			spacetest.AssertThatSpace(t, s.Namespace, s.Name, hostClient).
				Exists().
				HasFinalizer(). // finalizer is still there, until the error above is fixed
				HasSpecTargetCluster("member-3").
				HasStatusTargetCluster("member-3")
			nstemplatetsettest.AssertThatNSTemplateSet(t, test.MemberOperatorNs, "oddity", member1.Client).
				DoesNotExist()
			AssertThatCountersAndMetrics(t).
				HaveSpacesForCluster("member-1", 0).
				HaveSpacesForCluster("member-2", 0).
				HaveSpacesForCluster("member-3", 0) // unknown target cluster doesn't increment counter
		})
	})

	t.Run("failure", func(t *testing.T) {

		t.Run("error while getting NSTemplateSet on member cluster", func(t *testing.T) {
			// given
			s := spacetest.NewSpace(test.HostOperatorNs, "oddity",
				spacetest.WithDeletionTimestamp(), // deletion was requested
				spacetest.WithSpecTargetCluster("member-1"),
				spacetest.WithStatusTargetCluster("member-1"),
				spacetest.WithFinalizer(),
			)
			hostClient := test.NewFakeClient(t, s)
			member1Client := test.NewFakeClient(t)
			member1Client.MockGet = func(ctx context.Context, key runtimeclient.ObjectKey, obj runtimeclient.Object, opts ...runtimeclient.GetOption) error {
				if _, ok := obj.(*toolchainv1alpha1.NSTemplateSet); ok {
					return fmt.Errorf("mock error")
				}
				return member1Client.Client.Get(ctx, key, obj, opts...)
			}
			member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
			member2 := NewMemberClusterWithTenantRole(t, "member-2", corev1.ConditionTrue)
			ctrl := newReconciler(hostClient, member1, member2)
			InitializeCounters(t,
				NewToolchainStatus())

			// when
			res, err := ctrl.Reconcile(context.TODO(), requestFor(s))

			// then
			require.EqualError(t, err, "mock error")
			assert.False(t, res.Requeue)
			spacetest.AssertThatSpace(t, test.HostOperatorNs, s.Name, hostClient).
				Exists().
				HasFinalizer().
				HasSpecTargetCluster("member-1").
				HasStatusTargetCluster("member-1").
				HasConditions(spacetest.TerminatingFailed("mock error"))
			AssertThatCountersAndMetrics(t).
				HaveSpacesForCluster("member-1", 0).
				HaveSpacesForCluster("member-2", 0) // not increment when there is an error with getting NSTemplateSet
		})

		t.Run("error when NSTemplateSet is stuck in deletion", func(t *testing.T) {
			// given
			s := spacetest.NewSpace(test.HostOperatorNs, "oddity",
				spacetest.WithDeletionTimestamp(), // deletion was requested
				spacetest.WithSpecTargetCluster("member-1"),
				spacetest.WithStatusTargetCluster("member-1"),
				spacetest.WithFinalizer(),
			)
			hostClient := test.NewFakeClient(t, s)
			nstmplSet := nstemplatetsettest.NewNSTemplateSet("oddity", nstemplatetsettest.WithDeletionTimestamp(time.Now().Add(-2*time.Minute)))
			member1Client := test.NewFakeClient(t, nstmplSet)
			member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
			member2 := NewMemberClusterWithTenantRole(t, "member-2", corev1.ConditionTrue)
			ctrl := newReconciler(hostClient, member1, member2)
			InitializeCounters(t,
				NewToolchainStatus(
					WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
						"1,internal": 1,
					}),
					WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
						string(metrics.Internal): 1,
					}),
					WithMember("member-1", WithSpaceCount(1)),
				))

			// when
			res, err := ctrl.Reconcile(context.TODO(), requestFor(s))

			// then
			require.EqualError(t, err, "NSTemplateSet deletion has not completed in over 1 minute")
			assert.False(t, res.Requeue)
			spacetest.AssertThatSpace(t, test.HostOperatorNs, s.Name, hostClient).
				Exists().
				HasFinalizer().
				HasSpecTargetCluster("member-1").
				HasStatusTargetCluster("member-1").
				HasConditions(spacetest.TerminatingFailed("NSTemplateSet deletion has not completed in over 1 minute"))
			AssertThatCountersAndMetrics(t).
				HaveSpacesForCluster("member-1", 1).
				HaveSpacesForCluster("member-2", 0) // space counter is not decremented when there NSTemplateSet deletion is stuck
		})

		t.Run("unable to update status, it should not change the counter", func(t *testing.T) {
			// given
			s := spacetest.NewSpace(test.HostOperatorNs, "oddity",
				spacetest.WithDeletionTimestamp(),
				spacetest.WithFinalizer(),
				spacetest.WithSpecTargetCluster("member-1"),
				spacetest.WithStatusTargetCluster("member-1"))

			hostClient := test.NewFakeClient(t, s, base1nsTier)
			hostClient.MockStatusUpdate = mockUpdateSpaceStatusFail(hostClient.Client)
			nstmplSet := nstemplatetsettest.NewNSTemplateSet("oddity", nstemplatetsettest.WithReadyCondition())
			member1Client := test.NewFakeClient(t, nstmplSet)
			member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
			member2 := NewMemberClusterWithTenantRole(t, "member-2", corev1.ConditionTrue)
			ctrl := newReconciler(hostClient, member1, member2)
			InitializeCounters(t,
				NewToolchainStatus(WithEmptyMetrics(), WithMember("member-1", WithSpaceCount(1))))

			// when
			res, err := ctrl.Reconcile(context.TODO(), requestFor(s))

			// then
			require.EqualError(t, err, "mock error")
			assert.False(t, res.Requeue)
			spacetest.AssertThatSpace(t, test.HostOperatorNs, s.Name, hostClient).
				Exists().
				HasFinalizer().
				HasSpecTargetCluster("member-1").
				HasStatusTargetCluster("member-1").
				HasConditions()
			AssertThatCountersAndMetrics(t).
				HaveSpacesForCluster("member-1", 1). // counter not is decremented because of the failed status updated
				HaveSpacesForCluster("member-2", 0)  // space counter is unchanged
		})
	})
}

func TestUpdateSpaceTier(t *testing.T) {

	// given
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)
	base1nsTier := tiertest.Base1nsTier(t, tiertest.CurrentBase1nsTemplates)
	// get an older base1ns tier (with outdated references)
	olderbase1nsTier := tiertest.Base1nsTier(t, tiertest.PreviousBase1nsTemplates)
	otherTier := tiertest.OtherTier()

	t.Run("tier promotion (update needed due to different tier)", func(t *testing.T) {
		// given that Space is promoted from `base1ns` to `other` tier and corresponding NSTemplateSet is not up-to-date
		s := spacetest.NewSpace(test.HostOperatorNs, "oddity",
			spacetest.WithTierName(otherTier.Name),      // tier changed to other tier
			spacetest.WithTierHashLabelFor(base1nsTier), // still has the old tier hash label
			spacetest.WithSpecTargetCluster("member-1"),
			spacetest.WithStatusTargetCluster("member-1"), // already provisioned on a target cluster
			spacetest.WithFinalizer(),
			spacetest.WithCondition(spacetest.Ready()))
		hostClient := test.NewFakeClient(t, s, base1nsTier, otherTier)
		nstmplSet := nstemplatetsettest.NewNSTemplateSet("oddity", nstemplatetsettest.WithReferencesFor(base1nsTier), nstemplatetsettest.WithReadyCondition())
		member1Client := test.NewFakeClient(t, nstmplSet)
		member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
		member2 := NewMemberClusterWithTenantRole(t, "member-2", corev1.ConditionTrue)
		ctrl := newReconciler(hostClient, member1, member2)
		ctrl.LastExecutedUpdate = time.Now().Add(-1 * time.Minute) // assume that last executed update happened a long time ago
		InitializeCounters(t,
			NewToolchainStatus(
				WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
					"1,internal": 1,
				}),
				WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
					string(metrics.Internal): 1,
				}),
				WithMember("member-1", WithSpaceCount(1)),
			))

		// when
		res, err := ctrl.Reconcile(context.TODO(), requestFor(s))

		// then
		require.NoError(t, err)
		assert.Equal(t, reconcile.Result{
			Requeue:      true,
			RequeueAfter: 1 * time.Second,
		}, res) // explicitly requeue while the NSTemplate update is triggered by its controller
		spacetest.AssertThatSpace(t, test.HostOperatorNs, "oddity", hostClient).
			Exists().
			HasTier(otherTier.Name).
			HasSpecTargetCluster("member-1").
			HasStatusTargetCluster("member-1").
			HasConditions(spacetest.Updating()).
			DoesNotHaveLabel(hash.TemplateTierHashLabelKey(otherTier.Name)). // not set yet, since NSTemplateSet must be updated first
			HasMatchingTierLabelForTier(base1nsTier)                         // old label not removed yet, since NSTemplateSet must be updated first
		nsTmplSet := nstemplatetsettest.AssertThatNSTemplateSet(t, test.MemberOperatorNs, "oddity", member1.Client).
			Exists().
			HasTierName(otherTier.Name).
			HasClusterResourcesTemplateRef("other-clusterresources-123456a").
			HasNamespaceTemplateRefs("other-code-123456a", "other-dev-123456a", "other-stage-123456a").
			Get()
		AssertThatCountersAndMetrics(t).
			HaveSpacesForCluster("member-1", 1).
			HaveSpacesForCluster("member-2", 0) // space counter is unchanged

		t.Run("requeue while NSTemplateSet is not ready", func(t *testing.T) {
			// given another round of requeue while NSTemplateSet is *not ready*
			nsTmplSet.Status.Conditions = []toolchainv1alpha1.Condition{
				nstemplatetsettest.Updating(),
			}
			err := member1.Client.Update(context.TODO(), nsTmplSet)
			require.NoError(t, err)

			// when
			res, err := ctrl.Reconcile(context.TODO(), requestFor(s))

			// then
			require.NoError(t, err)
			assert.False(t, res.Requeue)
			spacetest.AssertThatSpace(t, test.HostOperatorNs, "oddity", hostClient).
				Exists().
				HasTier(otherTier.Name).
				HasSpecTargetCluster("member-1").
				HasStatusTargetCluster("member-1").
				HasConditions(spacetest.Updating()).
				DoesNotHaveLabel(hash.TemplateTierHashLabelKey(otherTier.Name)).
				HasMatchingTierLabelForTier(base1nsTier)
			AssertThatCountersAndMetrics(t).
				HaveSpacesForCluster("member-1", 1).
				HaveSpacesForCluster("member-2", 0) // space counter is unchanged

			t.Run("not done when NSTemplateSet is ready within 1s", func(t *testing.T) {
				// given another round of requeue with NSTemplateSet now *ready*
				// but LESS than 1s after the Space Ready condition was set to `Ready=false/Updating`
				nsTmplSet.Status.Conditions = []toolchainv1alpha1.Condition{
					nstemplatetsettest.Provisioned(),
				}
				err := member1.Client.Update(context.TODO(), nsTmplSet)
				require.NoError(t, err)

				// when
				res, err := ctrl.Reconcile(context.TODO(), requestFor(s))

				// then
				require.NoError(t, err)
				assert.Equal(t, reconcile.Result{
					Requeue:      true,
					RequeueAfter: 1 * time.Second}, res) // requeue requested
				s := spacetest.AssertThatSpace(t, test.HostOperatorNs, "oddity", hostClient).
					Exists().
					HasStatusTargetCluster("member-1").
					HasConditions(spacetest.Updating()).
					DoesNotHaveLabel(hash.TemplateTierHashLabelKey(otherTier.Name)). // not set yet
					HasMatchingTierLabelForTier(base1nsTier).                        // old label not removed yet, since NSTemplateSet not ready for more than 1s
					HasFinalizer().
					Get()
				AssertThatCountersAndMetrics(t).
					HaveSpacesForCluster("member-1", 1).
					HaveSpacesForCluster("member-2", 0) // space counter is unchanged

				t.Run("done when NSTemplateSet is ready for more than 1s", func(t *testing.T) {
					// given another round of requeue with NSTemplateSet now *ready*
					// but MORE than 1s after the Space Ready condition was set to `Ready=false/Updating`

					// hack: change Space's condition timestamp
					s.Status.Conditions[0].LastTransitionTime = metav1.NewTime(s.Status.Conditions[0].LastTransitionTime.Time.Add(-1 * time.Second))
					err := hostClient.Status().Update(context.TODO(), s)
					require.NoError(t, err)

					// when
					res, err := ctrl.Reconcile(context.TODO(), requestFor(s))

					// then
					require.NoError(t, err)
					assert.Equal(t, reconcile.Result{Requeue: false}, res) // no more requeue.
					spacetest.AssertThatSpace(t, test.HostOperatorNs, "oddity", hostClient).
						Exists().
						HasStatusTargetCluster("member-1").
						HasConditions(spacetest.Ready()).
						DoesNotHaveLabel(hash.TemplateTierHashLabelKey(base1nsTier.Name)). // old label removed
						HasMatchingTierLabelForTier(otherTier).                            // new label matching updated tier
						HasFinalizer()
					AssertThatCountersAndMetrics(t).
						HaveSpacesForCluster("member-1", 1).
						HaveSpacesForCluster("member-2", 0) // space counter is unchanged

				})
			})
		})
	})

	t.Run("update without postpone", func(t *testing.T) {
		// get an older base1ns tier (with outdated references) that the nstemplateset can be referenced to for setup
		olderbase1nsTier := tiertest.Base1nsTier(t, tiertest.PreviousBase1nsTemplates)
		// given that Space is set to the same tier that has been updated and the corresponding NSTemplateSet is not up-to-date
		s := spacetest.NewSpace(test.HostOperatorNs, "oddity",
			spacetest.WithTierNameAndHashLabelFor(olderbase1nsTier),
			spacetest.WithSpecTargetCluster("member-1"),
			spacetest.WithStatusTargetCluster("member-1"), // already provisioned on a target cluster
			spacetest.WithFinalizer(),
			spacetest.WithCondition(spacetest.Ready()))
		hostClient := test.NewFakeClient(t, s, base1nsTier)
		nsTmplSet := nstemplatetsettest.NewNSTemplateSet("oddity", nstemplatetsettest.WithReferencesFor(olderbase1nsTier), nstemplatetsettest.WithReadyCondition()) // NSTemplateSet has references to old base1ns tier
		member1Client := test.NewFakeClient(t, nsTmplSet)
		member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
		member2 := NewMemberClusterWithTenantRole(t, "member-2", corev1.ConditionTrue)
		ctrl := newReconciler(hostClient, member1, member2)
		ctrl.LastExecutedUpdate = time.Now().Add(-1 * time.Minute) // assume that last executed update happened a long time ago
		InitializeCounters(t,
			NewToolchainStatus(
				WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
					"1,internal": 1,
				}),
				WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
					string(metrics.Internal): 1,
				}),
				WithMember("member-1", WithSpaceCount(1)),
			))

		// when
		res, err := ctrl.Reconcile(context.TODO(), requestFor(s))

		// then
		require.NoError(t, err)
		assert.Equal(t, reconcile.Result{
			Requeue:      true,            // explicitly requeue while the NSTemplateSet update is triggered by its controller
			RequeueAfter: 1 * time.Second, // requeued by 1s (since the last update happened a long time ago enough)
		}, res)
		s = spacetest.AssertThatSpace(t, test.HostOperatorNs, "oddity", hostClient).
			Exists().
			HasTier(base1nsTier.Name).
			HasSpecTargetCluster("member-1").
			HasStatusTargetCluster("member-1").
			HasConditions(spacetest.Updating()).
			HasMatchingTierLabelForTier(olderbase1nsTier).
			Get()
		nsTmplSet = nstemplatetsettest.AssertThatNSTemplateSet(t, test.MemberOperatorNs, "oddity", member1.Client).
			Exists().
			HasTierName(base1nsTier.Name).
			HasClusterResourcesTemplateRef("base1ns-clusterresources-123456new").
			HasNamespaceTemplateRefs("base1ns-code-123456new", "base1ns-dev-123456new", "base1ns-stage-123456new").
			Get()
		require.True(t, hash.TierHashMatches(base1nsTier, nsTmplSet.Spec))
		AssertThatCountersAndMetrics(t).
			HaveSpacesForCluster("member-1", 1) // space counter is unchanged

		t.Run("NSTemplateSet is provisioned", func(t *testing.T) {
			// given another round of requeue with NSTemplateSet now *ready*
			nsTmplSet.Status.Conditions = []toolchainv1alpha1.Condition{
				nstemplatetsettest.Provisioned(),
			}
			err := member1.Client.Update(context.TODO(), nsTmplSet)
			require.NoError(t, err)
			// hack: change Space's updating condition timestamp so that it appears the status is ready for >1 second since it's required before can go to Ready condition
			s.Status.Conditions[0].LastTransitionTime = metav1.NewTime(s.Status.Conditions[0].LastTransitionTime.Time.Add(-1 * time.Second))
			err = hostClient.Status().Update(context.TODO(), s)
			require.NoError(t, err)

			// when
			res, err = ctrl.Reconcile(context.TODO(), requestFor(s))

			// then
			require.NoError(t, err)
			assert.Equal(t, reconcile.Result{Requeue: false}, res) // no more requeue.
			spacetest.AssertThatSpace(t, test.HostOperatorNs, "oddity", hostClient).
				Exists().
				HasStatusTargetCluster("member-1").
				HasConditions(spacetest.Ready()).
				HasMatchingTierLabelForTier(base1nsTier). // label updated
				HasFinalizer()
			AssertThatCountersAndMetrics(t).
				HaveSpacesForCluster("member-1", 1) // space counter is unchanged
		})
	})

	t.Run("update with postpone", func(t *testing.T) {
		// given a space set to the older version of the tier
		s := spacetest.NewSpace(test.HostOperatorNs, "oddity1",
			spacetest.WithTierNameAndHashLabelFor(olderbase1nsTier),
			spacetest.WithSpecTargetCluster("member-1"),
			spacetest.WithStatusTargetCluster("member-1"),
			spacetest.WithFinalizer(),
			spacetest.WithCondition(spacetest.Ready()))
		nsTmplSet := nstemplatetsettest.NewNSTemplateSet(s.Name,
			nstemplatetsettest.WithReferencesFor(olderbase1nsTier), // NSTemplateSet has references to old base1ns tier
			nstemplatetsettest.WithReadyCondition())
		InitializeCounters(t,
			NewToolchainStatus(
				WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
					"1,internal": 1,
				}),
				WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
					string(metrics.Internal): 1,
				}),
				WithMember("member-1", WithSpaceCount(1)),
			))

		t.Run("postponed by two seconds from now", func(t *testing.T) {
			hostClient := test.NewFakeClient(t, s, base1nsTier)
			member1Client := test.NewFakeClient(t, nsTmplSet)
			member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
			ctrl := newReconciler(hostClient, member1)
			ctrl.LastExecutedUpdate = time.Now()

			// when reconciling space
			res, err := ctrl.Reconcile(context.TODO(), requestFor(s))

			// then
			require.NoError(t, err)
			assert.True(t, res.Requeue)
			assert.LessOrEqual(t, res.RequeueAfter, 2*time.Second) // wait 2s for NSTemplateSet update to begin
			assert.LessOrEqual(t, time.Until(ctrl.NextScheduledUpdate), 2*time.Second)
			// check that the NSTemplateSet is not being updated
			spacetest.AssertThatSpace(t, test.HostOperatorNs, s.Name, hostClient).
				Exists().
				HasTier(base1nsTier.Name).
				HasConditions(spacetest.Ready()).
				HasMatchingTierLabelForTier(olderbase1nsTier)
			nsTmplSet = nstemplatetsettest.AssertThatNSTemplateSet(t, test.MemberOperatorNs, nsTmplSet.Name, member1.Client).
				Exists().
				Get()
			require.True(t, hash.TierHashMatches(olderbase1nsTier, nsTmplSet.Spec))
			AssertThatCountersAndMetrics(t).
				HaveSpacesForCluster("member-1", 1) // space counter is unchanged
		})

		t.Run("postponed by two seconds from the NextScheduledUpdate", func(t *testing.T) {
			hostClient := test.NewFakeClient(t, s, base1nsTier)
			member1Client := test.NewFakeClient(t, nsTmplSet)
			member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
			ctrl := newReconciler(hostClient, member1)
			ctrl.NextScheduledUpdate = time.Now().Add(1 * time.Minute)
			ctrl.LastExecutedUpdate = time.Now()

			// when reconciling space
			res, err := ctrl.Reconcile(context.TODO(), requestFor(s))

			// then
			require.NoError(t, err)
			assert.True(t, res.Requeue)
			assert.LessOrEqual(t, res.RequeueAfter, time.Minute+(2*time.Second)) // wait 2s for NSTemplateSet update to begin
			assert.LessOrEqual(t, time.Until(ctrl.NextScheduledUpdate), time.Minute+(2*time.Second))
			// check that the NSTemplateSet is not being updated
			spacetest.AssertThatSpace(t, test.HostOperatorNs, s.Name, hostClient).
				Exists().
				HasTier(base1nsTier.Name).
				HasConditions(spacetest.Ready()).
				HasMatchingTierLabelForTier(olderbase1nsTier)
			nsTmplSet = nstemplatetsettest.AssertThatNSTemplateSet(t, test.MemberOperatorNs, nsTmplSet.Name, member1.Client).
				Exists().
				Get()
			require.True(t, hash.TierHashMatches(olderbase1nsTier, nsTmplSet.Spec))
			AssertThatCountersAndMetrics(t).
				HaveSpacesForCluster("member-1", 1) // space counter is unchanged
		})
	})

	t.Run("update needed due to feature toggle annotation change in space", func(t *testing.T) {
		type testRun struct {
			name             string
			oldSpaceFeatures string
			newSpaceFeatures string
		}

		tests := []testRun{
			{
				name:             "feature enabled",
				oldSpaceFeatures: "",
				newSpaceFeatures: "feature-1,feature-2",
			},
			{
				name:             "feature disabled",
				oldSpaceFeatures: "feature-1,feature-2",
				newSpaceFeatures: "",
			},
			{
				name:             "feature updated",
				oldSpaceFeatures: "feature-1,feature-2",
				newSpaceFeatures: "feature-3",
			},
			{
				name:             "no updates",
				oldSpaceFeatures: "feature-1,feature-2",
				newSpaceFeatures: "feature-1, feature-2",
			},
			{
				name:             "features still are not present",
				oldSpaceFeatures: "",
				newSpaceFeatures: "",
			},
		}
		for _, testRun := range tests {
			t.Run(testRun.name, func(t *testing.T) {
				// given
				spaceOptions := []spacetest.Option{spacetest.WithSpecTargetCluster("member-1")}
				if testRun.newSpaceFeatures != "" {
					spaceOptions = append(spaceOptions, spacetest.WithAnnotation(toolchainv1alpha1.FeatureToggleNameAnnotationKey, testRun.newSpaceFeatures))
				}

				s := spacetest.NewSpace(test.HostOperatorNs, "oddity", spaceOptions...)

				nsTemplateSetOptions := []nstemplatetsettest.Option{nstemplatetsettest.WithReadyCondition(), nstemplatetsettest.WithReferencesFor(base1nsTier)}
				if testRun.oldSpaceFeatures != "" {
					nsTemplateSetOptions = append(nsTemplateSetOptions, nstemplatetsettest.WithAnnotation(toolchainv1alpha1.FeatureToggleNameAnnotationKey, testRun.oldSpaceFeatures))
				}
				nsTemplateSet := nstemplatetsettest.NewNSTemplateSet("oddity", nsTemplateSetOptions...)
				hostClient := test.NewFakeClient(t, s, base1nsTier)
				member1Client := test.NewFakeClient(t, nsTemplateSet)
				if testRun.oldSpaceFeatures == testRun.newSpaceFeatures {
					member1Client.MockUpdate = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.UpdateOption) error {
						return fmt.Errorf("shouldn't be called")
					}
				}
				member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
				ctrl := newReconciler(hostClient, member1)

				// when
				_, err = ctrl.Reconcile(context.TODO(), requestFor(s))
				require.NoError(t, err)

				// then
				// check that NSTemplateSet is updated accordingly
				nsTmplSetAssertion := nstemplatetsettest.AssertThatNSTemplateSet(t, test.MemberOperatorNs, s.Name, member1.Client).
					Exists()
				if testRun.newSpaceFeatures != "" {
					nsTmplSetAssertion.
						HasAnnotationWithValue(toolchainv1alpha1.FeatureToggleNameAnnotationKey, testRun.newSpaceFeatures)
				} else {
					nsTmplSetAssertion.
						DoesNotHaveAnnotation(toolchainv1alpha1.FeatureToggleNameAnnotationKey)
				}
			})
		}
	})

	t.Run("update not needed when already up-to-date", func(t *testing.T) {
		// given that Space is promoted to `base1ns` tier and corresponding NSTemplateSet is already up-to-date and ready
		s := spacetest.NewSpace(test.HostOperatorNs, "oddity",
			spacetest.WithTierName(base1nsTier.Name),
			spacetest.WithTierHashLabelFor(olderbase1nsTier), // tier hash label not updated yet
			spacetest.WithCondition(spacetest.Ready()),
			spacetest.WithSpecTargetCluster("member-1"),
			spacetest.WithStatusTargetCluster("member-1"), // already provisioned on a target cluster
			spacetest.WithFinalizer())
		hostClient := test.NewFakeClient(t, s, base1nsTier)
		nstmplSet := nstemplatetsettest.NewNSTemplateSet("oddity", nstemplatetsettest.WithReferencesFor(base1nsTier), nstemplatetsettest.WithReadyCondition())
		member1Client := test.NewFakeClient(t, nstmplSet)
		member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
		member2 := NewMemberClusterWithTenantRole(t, "member-2", corev1.ConditionTrue)
		ctrl := newReconciler(hostClient, member1, member2)
		InitializeCounters(t,
			NewToolchainStatus(
				WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
					"1,internal": 1,
				}),
				WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
					string(metrics.Internal): 1,
				}),
				WithMember("member-1", WithSpaceCount(1)),
			))

		// when
		res, err := ctrl.Reconcile(context.TODO(), requestFor(s))

		// then
		require.NoError(t, err)
		assert.False(t, res.Requeue) // no need to requeue since the NSTemplate is already up-to-date
		spacetest.AssertThatSpace(t, test.HostOperatorNs, "oddity", hostClient).
			Exists().
			HasTier(base1nsTier.Name).
			HasSpecTargetCluster("member-1").
			HasStatusTargetCluster("member-1").
			HasConditions(spacetest.Ready()).
			HasMatchingTierLabelForTier(base1nsTier) // label is immediately set since the NSTemplateSet was already up-to-date
		AssertThatCountersAndMetrics(t).
			HaveSpacesForCluster("member-1", 1) // space counter is unchanged
	})

	t.Run("update needed even when Space not ready", func(t *testing.T) {
		notReadySpace := spacetest.NewSpace(test.HostOperatorNs, "oddity1",
			spacetest.WithTierNameAndHashLabelFor(base1nsTier),
			spacetest.WithSpecTargetCluster("member-1"),
			spacetest.WithStatusTargetCluster("member-1"),
			spacetest.WithFinalizer(),
			spacetest.WithCondition(spacetest.Updating())) // space is not ready
		notReadyTmplSet := nstemplatetsettest.NewNSTemplateSet(notReadySpace.Name,
			nstemplatetsettest.WithReferencesFor(olderbase1nsTier), // NSTemplateSet has references to old base1ns tier
			nstemplatetsettest.WithReadyCondition())                // NSTemplateSet is ready
		hostClient := test.NewFakeClient(t, notReadySpace, base1nsTier)
		member1Client := test.NewFakeClient(t, notReadyTmplSet)
		member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
		ctrl := newReconciler(hostClient, member1)
		InitializeCounters(t,
			NewToolchainStatus(
				WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
					"1,internal": 1,
				}),
				WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
					string(metrics.Internal): 1,
				}),
				WithMember("member-1", WithSpaceCount(1)),
			))

		// when reconciling space
		res, err := ctrl.Reconcile(context.TODO(), requestFor(notReadySpace))

		// then
		require.NoError(t, err)
		assert.True(t, res.Requeue)
		spacetest.AssertThatSpace(t, test.HostOperatorNs, notReadySpace.Name, hostClient).
			HasConditions(spacetest.Updating())
		nstemplatetsettest.AssertThatNSTemplateSet(t, test.MemberOperatorNs, notReadyTmplSet.Name, member1Client).
			HasClusterResourcesTemplateRef("base1ns-clusterresources-123456new").
			HasNamespaceTemplateRefs("base1ns-code-123456new", "base1ns-dev-123456new", "base1ns-stage-123456new").
			HasConditions(nstemplatetsettest.Provisioned()) // status will be set to `Updating` by NSTemplateSet controller
		AssertThatCountersAndMetrics(t).
			HaveSpacesForCluster("member-1", 1) // space counter is unchanged
	})

	t.Run("update needed even when NStemplateSet not ready", func(t *testing.T) {
		notReadySpace := spacetest.NewSpace(test.HostOperatorNs, "oddity1",
			spacetest.WithTierNameAndHashLabelFor(base1nsTier),
			spacetest.WithSpecTargetCluster("member-1"),
			spacetest.WithStatusTargetCluster("member-1"),
			spacetest.WithFinalizer(),
			spacetest.WithCondition(spacetest.Ready())) // space is ready
		notReadyTmplSet := nstemplatetsettest.NewNSTemplateSet(notReadySpace.Name,
			nstemplatetsettest.WithReferencesFor(olderbase1nsTier),                                      // NSTemplateSet has references to old base1ns tier
			nstemplatetsettest.WithNotReadyCondition(toolchainv1alpha1.NSTemplateSetUpdatingReason, "")) // NSTemplateSet is not ready
		hostClient := test.NewFakeClient(t, notReadySpace, base1nsTier)
		member1Client := test.NewFakeClient(t, notReadyTmplSet)
		member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
		ctrl := newReconciler(hostClient, member1)
		InitializeCounters(t,
			NewToolchainStatus(
				WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
					"1,internal": 1,
				}),
				WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
					string(metrics.Internal): 1,
				}),
				WithMember("member-1", WithSpaceCount(1)),
			))

		// when reconciling space
		res, err := ctrl.Reconcile(context.TODO(), requestFor(notReadySpace))

		// then
		require.NoError(t, err)
		assert.True(t, res.Requeue)
		spacetest.AssertThatSpace(t, test.HostOperatorNs, notReadySpace.Name, hostClient).
			HasConditions(spacetest.Updating()) // changed by controller
		nstemplatetsettest.AssertThatNSTemplateSet(t, test.MemberOperatorNs, notReadyTmplSet.Name, member1Client).
			HasClusterResourcesTemplateRef("base1ns-clusterresources-123456new").
			HasNamespaceTemplateRefs("base1ns-code-123456new", "base1ns-dev-123456new", "base1ns-stage-123456new").
			HasConditions(nstemplatetsettest.Updating()) // was already `Updating`
		AssertThatCountersAndMetrics(t).
			HaveSpacesForCluster("member-1", 1) // space counter is unchanged
	})

	t.Run("failures", func(t *testing.T) {

		t.Run("when updating space with new templatetierhash label", func(t *testing.T) {
			// given that Space is promoted to `base1ns` tier and corresponding NSTemplateSet is already up-to-date and ready
			s := spacetest.NewSpace(test.HostOperatorNs, "oddity",
				spacetest.WithTierHashLabelFor(otherTier), // assume that at this point, the `TemplateTierHash` label still has the old value
				spacetest.WithCondition(spacetest.Ready()),
				spacetest.WithSpecTargetCluster("member-1"),
				spacetest.WithStatusTargetCluster("member-1"), // already provisioned on a target cluster
				spacetest.WithFinalizer())
			hostClient := test.NewFakeClient(t, s, base1nsTier, otherTier)
			hostClient.MockUpdate = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.UpdateOption) error {
				if _, ok := obj.(*toolchainv1alpha1.Space); ok && obj.GetLabels()[hash.TemplateTierHashLabelKey(base1nsTier.Name)] != "" {
					return fmt.Errorf("mock error")
				}
				return hostClient.Client.Update(ctx, obj, opts...)
			}
			nstmplSet := nstemplatetsettest.NewNSTemplateSet("oddity", nstemplatetsettest.WithReferencesFor(base1nsTier), nstemplatetsettest.WithReadyCondition())
			member1Client := test.NewFakeClient(t, nstmplSet)
			member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
			member2 := NewMemberClusterWithTenantRole(t, "member-2", corev1.ConditionTrue)
			ctrl := newReconciler(hostClient, member1, member2)
			InitializeCounters(t,
				NewToolchainStatus(
					WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
						"1,internal": 1,
					}),
					WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
						string(metrics.Internal): 1,
					}),
					WithMember("member-1", WithSpaceCount(1)),
				))

			// when
			res, err := ctrl.Reconcile(context.TODO(), requestFor(s))

			// then
			require.EqualError(t, err, "mock error")
			assert.False(t, res.Requeue)
			spacetest.AssertThatSpace(t, test.HostOperatorNs, s.Name, hostClient).
				Exists().
				HasStatusTargetCluster("member-1").
				HasConditions(spacetest.ProvisioningFailed("mock error")).
				HasFinalizer()
			AssertThatCountersAndMetrics(t).
				HaveSpacesForCluster("member-1", 1) // space counter is unchanged
		})

		t.Run("when NSTemplateSet update failed", func(t *testing.T) {
			// given that Space is promoted to `other` tier and corresponding NSTemplateSet is not up-to-date
			s := spacetest.NewSpace(test.HostOperatorNs, "oddity",
				spacetest.WithTierName(otherTier.Name),
				spacetest.WithTierHashLabelFor(base1nsTier),
				spacetest.WithSpecTargetCluster("member-1"),
				spacetest.WithStatusTargetCluster("member-1"), // already provisioned on a target cluster
				spacetest.WithFinalizer(),
				spacetest.WithCondition(spacetest.Ready()))
			hostClient := test.NewFakeClient(t, s, base1nsTier, otherTier)
			nstmplSet := nstemplatetsettest.NewNSTemplateSet("oddity", nstemplatetsettest.WithReferencesFor(base1nsTier), nstemplatetsettest.WithReadyCondition())
			member1Client := test.NewFakeClient(t, nstmplSet)
			member1Client.MockUpdate = mockUpdateNSTemplateSetFail(member1Client.Client)
			member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
			member2 := NewMemberClusterWithTenantRole(t, "member-2", corev1.ConditionTrue)
			ctrl := newReconciler(hostClient, member1, member2)
			InitializeCounters(t,
				NewToolchainStatus(
					WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
						"1,internal": 1,
					}),
					WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
						string(metrics.Internal): 1,
					}),
					WithMember("member-1", WithSpaceCount(1)),
				))

			// when
			res, err := ctrl.Reconcile(context.TODO(), requestFor(s))

			// then
			require.EqualError(t, err, "mock error")
			assert.False(t, res.Requeue)
			spacetest.AssertThatSpace(t, test.HostOperatorNs, "oddity", hostClient).
				Exists().
				HasTier(otherTier.Name).
				HasSpecTargetCluster("member-1").
				HasStatusTargetCluster("member-1").
				HasConditions(spacetest.UnableToUpdateNSTemplateSet("mock error")).
				DoesNotHaveLabel(hash.TemplateTierHashLabelKey(otherTier.Name)) // not set yet, since NSTemplateSet must be updated first
			nstemplatetsettest.AssertThatNSTemplateSet(t, test.MemberOperatorNs, "oddity", member1.Client).
				Exists().
				HasTierName(base1nsTier.Name).
				HasClusterResourcesTemplateRef("base1ns-clusterresources-123456new").
				HasNamespaceTemplateRefs("base1ns-code-123456new", "base1ns-dev-123456new", "base1ns-stage-123456new")
			AssertThatCountersAndMetrics(t).
				HaveSpacesForCluster("member-1", 1) // space counter is unchanged
		})
	})
}

// TestUpdateSpaceRoles covers the cases where SpaceBindings are created/updated/deleted,
// and how this should affect the Space and its NSTemplateSet
func TestUpdateSpaceRoles(t *testing.T) {

	// given
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)
	base1nsTier := tiertest.Base1nsTier(t, tiertest.CurrentBase1nsTemplates)
	adminMUR := murtest.NewMasterUserRecord(t, "jack")
	viewerMUR := murtest.NewMasterUserRecord(t, "jeff")
	johnMUR := murtest.NewMasterUserRecord(t, "john")

	t.Run("add user with admin role", func(t *testing.T) {
		// given a MUR, a Space and its NSTemplateSet resource...
		s := spacetest.NewSpace(test.HostOperatorNs, "oddity",
			spacetest.WithTierName(base1nsTier.Name),
			spacetest.WithSpecTargetCluster("member-1"),
			spacetest.WithStatusTargetCluster("member-1"), // already provisioned on a target cluster
			spacetest.WithFinalizer())
		nstmplSet := nstemplatetsettest.NewNSTemplateSet("oddity",
			nstemplatetsettest.WithReferencesFor(base1nsTier,
				// include pre-existing users with role...
				nstemplatetsettest.WithSpaceRole("admin", adminMUR.Name),
				nstemplatetsettest.WithSpaceRole("viewer", viewerMUR.Name),
			),
			nstemplatetsettest.WithReadyCondition(),
		)
		// ...and their corresponding space bindings
		sb1 := spacebindingtest.NewSpaceBinding(adminMUR.Name, s.Name, "admin", "signupAdmin")
		sb2 := spacebindingtest.NewSpaceBinding(viewerMUR.Name, s.Name, "viewer", "signupViewer")

		// and a SpaceBinding for John as an Admin on the Space
		sb3 := spacebindingtest.NewSpaceBinding(johnMUR.Name, s.Name, "admin", "signupJohn")

		hostClient := test.NewFakeClient(t, s, johnMUR, adminMUR, sb1, viewerMUR, sb2, sb3, base1nsTier)
		member1Client := test.NewFakeClient(t, nstmplSet)
		member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
		member2 := NewMemberClusterWithTenantRole(t, "member-2", corev1.ConditionTrue)

		ctrl := newReconciler(hostClient, member1, member2)
		InitializeCounters(t,
			NewToolchainStatus(
				WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
					"1,internal": 1,
				}),
				WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
					string(metrics.Internal): 1,
				}),
				WithMember("member-1", WithSpaceCount(1)),
			))

		// when
		res, err := ctrl.Reconcile(context.TODO(), requestFor(s))

		// then
		require.NoError(t, err)
		assert.True(t, res.Requeue) // expect a requeue since the NSTemplateSet was updated
		// Space should be in "updating" state while the NSTemplateSet is being updated
		spacetest.AssertThatSpace(t, test.HostOperatorNs, s.Name, hostClient).
			HasConditions(spacetest.Updating())
		// NSTemplateSet should have an spaceRoles entry for the `mur`
		nstemplatetsettest.AssertThatNSTemplateSet(t, test.MemberOperatorNs, nstmplSet.Name, member1Client).
			HasSpaceRoles(
				nstemplatetsettest.SpaceRole("base1ns-admin-123456new", adminMUR.Name, johnMUR.Name), // entry added for user 'john' as admin
				nstemplatetsettest.SpaceRole("base1ns-viewer-123456new", viewerMUR.Name),             // unchanged
			).
			HasConditions(nstemplatetsettest.Provisioned()) // not changed by the SpaceController, but will be by the NSTemplateSetController
		AssertThatCountersAndMetrics(t).
			HaveSpacesForCluster("member-1", 1) // space counter is unchanged
	})

	t.Run("add duplicate user with admin role", func(t *testing.T) {
		// NOTE: should not happen because SpaceBinding names are based on space+mur names and should be unique,
		// except if a SpaceBinding resource is not created by the UserSignupController  ¯\_(ツ)_/¯

		// given a MUR, a Space and its NSTemplateSet resource...
		s := spacetest.NewSpace(test.HostOperatorNs, "oddity",
			spacetest.WithTierName(base1nsTier.Name),
			spacetest.WithSpecTargetCluster("member-1"),
			spacetest.WithStatusTargetCluster("member-1"), // already provisioned on a target cluster
			spacetest.WithFinalizer())
		nstmplSet := nstemplatetsettest.NewNSTemplateSet("oddity",
			nstemplatetsettest.WithReferencesFor(base1nsTier,
				// include pre-existing users with role...
				nstemplatetsettest.WithSpaceRole("admin", adminMUR.Name),
				nstemplatetsettest.WithSpaceRole("viewer", viewerMUR.Name),
			),
			nstemplatetsettest.WithReadyCondition(),
		)
		// ...and their corresponding space bindings
		sb1 := spacebindingtest.NewSpaceBinding(adminMUR.Name, s.Name, "admin", "signupAdmin")
		sb2 := spacebindingtest.NewSpaceBinding(viewerMUR.Name, s.Name, "viewer", "signupViewer")

		// and a SpaceBinding for John as an Admin on the Space
		sb3 := spacebindingtest.NewSpaceBinding(adminMUR.Name, s.Name, "admin", "signupAdmin") // duplicate of sb1
		sb3.Name = "something-else"                                                            // make sure that the sb3's name does not collide with existing sb1's name

		hostClient := test.NewFakeClient(t, s, johnMUR, adminMUR, sb1, viewerMUR, sb2, sb3, base1nsTier)
		member1Client := test.NewFakeClient(t, nstmplSet)
		member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
		member2 := NewMemberClusterWithTenantRole(t, "member-2", corev1.ConditionTrue)

		ctrl := newReconciler(hostClient, member1, member2)
		InitializeCounters(t,
			NewToolchainStatus(
				WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
					"1,internal": 1,
				}),
				WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
					string(metrics.Internal): 1,
				}),
				WithMember("member-1", WithSpaceCount(1)),
			))

		// when
		res, err := ctrl.Reconcile(context.TODO(), requestFor(s))

		// then
		require.NoError(t, err)
		assert.False(t, res.Requeue) // no requeue since the NSTemplateSet was not updated
		spacetest.AssertThatSpace(t, test.HostOperatorNs, s.Name, hostClient).
			HasConditions(spacetest.Ready())
		// NSTemplateSet should have an spaceRoles entry for the `mur`
		nstemplatetsettest.AssertThatNSTemplateSet(t, test.MemberOperatorNs, nstmplSet.Name, member1Client).
			HasSpaceRoles(
				nstemplatetsettest.SpaceRole("base1ns-admin-123456new", adminMUR.Name),   // NO duplicate entry for `signupAdmin` user
				nstemplatetsettest.SpaceRole("base1ns-viewer-123456new", viewerMUR.Name), // unchanged
			).
			HasConditions(nstemplatetsettest.Provisioned())
		AssertThatCountersAndMetrics(t).
			HaveSpacesForCluster("member-1", 1) // space counter is unchanged
	})

	t.Run("remove user with admin role", func(t *testing.T) {
		// given a MUR, a Space and its NSTemplateSet resource...
		s := spacetest.NewSpace(test.HostOperatorNs, "oddity",
			spacetest.WithTierName(base1nsTier.Name),
			spacetest.WithSpecTargetCluster("member-1"),
			spacetest.WithStatusTargetCluster("member-1"), // already provisioned on a target cluster
			spacetest.WithFinalizer())
		nstmplSet := nstemplatetsettest.NewNSTemplateSet("oddity",
			nstemplatetsettest.WithReferencesFor(base1nsTier,
				// include pre-existing users with role...
				nstemplatetsettest.WithSpaceRole("admin", adminMUR.Name),
				nstemplatetsettest.WithSpaceRole("viewer", viewerMUR.Name),
				// and an entry for john as an admin
				nstemplatetsettest.WithSpaceRole("admin", johnMUR.Name),
			),
			nstemplatetsettest.WithReadyCondition())
		// ...and their corresponding space bindings
		sb1 := spacebindingtest.NewSpaceBinding(adminMUR.Name, s.Name, "admin", "signupAdmin")
		sb2 := spacebindingtest.NewSpaceBinding(viewerMUR.Name, s.Name, "viewer", "signupViewer")

		hostClient := test.NewFakeClient(t, s, adminMUR, sb1, viewerMUR, sb2, johnMUR, base1nsTier)
		member1Client := test.NewFakeClient(t, nstmplSet)
		member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
		member2 := NewMemberClusterWithTenantRole(t, "member-2", corev1.ConditionTrue)

		ctrl := newReconciler(hostClient, member1, member2)
		InitializeCounters(t,
			NewToolchainStatus(
				WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
					"1,internal": 1,
				}),
				WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
					string(metrics.Internal): 1,
				}),
				WithMember("member-1", WithSpaceCount(1)),
			))

		// when
		res, err := ctrl.Reconcile(context.TODO(), requestFor(s))

		// then
		require.NoError(t, err)
		assert.True(t, res.Requeue) // expect a requeue since the NSTemplateSet was updated
		// Space should be in "updating" state while the NSTemplateSet is being updated
		spacetest.AssertThatSpace(t, test.HostOperatorNs, s.Name, hostClient).
			HasConditions(spacetest.Updating())
		// NSTemplateSet should have an spaceRoles entry for the `mur`
		nstemplatetsettest.AssertThatNSTemplateSet(t, test.MemberOperatorNs, nstmplSet.Name, member1Client).
			HasSpaceRoles(
				nstemplatetsettest.SpaceRole("base1ns-admin-123456new", adminMUR.Name),   // entry removed for user 'john'
				nstemplatetsettest.SpaceRole("base1ns-viewer-123456new", viewerMUR.Name), // unchanged
			).
			HasConditions(nstemplatetsettest.Provisioned()) // not changed by the SpaceController, but will be by the NSTemplateSetController
		AssertThatCountersAndMetrics(t).
			HaveSpacesForCluster("member-1", 1) // space counter is unchanged
	})

	t.Run("update user from viewer to admin role", func(t *testing.T) {
		// given a MUR, a Space and its NSTemplateSet resource...
		s := spacetest.NewSpace(test.HostOperatorNs, "oddity",
			spacetest.WithTierName(base1nsTier.Name),
			spacetest.WithSpecTargetCluster("member-1"),
			spacetest.WithStatusTargetCluster("member-1"), // already provisioned on a target cluster
			spacetest.WithFinalizer())
		nstmplSet := nstemplatetsettest.NewNSTemplateSet("oddity",
			nstemplatetsettest.WithReferencesFor(base1nsTier,
				// include pre-existing users with role...
				nstemplatetsettest.WithSpaceRole("admin", adminMUR.Name),
				nstemplatetsettest.WithSpaceRole("viewer", viewerMUR.Name),
				// and user john as a viewer
				nstemplatetsettest.WithSpaceRole("viewer", johnMUR.Name),
			),
			nstemplatetsettest.WithReadyCondition())

		// corresponding space bindings for adminMUR and viewerMUR
		sb1 := spacebindingtest.NewSpaceBinding(adminMUR.Name, s.Name, "admin", "signupAdmin")
		sb2 := spacebindingtest.NewSpaceBinding(viewerMUR.Name, s.Name, "viewer", "signupViewer")
		// but SpaceBinding for John as an _admin_ on the Space
		sb3 := spacebindingtest.NewSpaceBinding(johnMUR.Name, s.Name, "admin", "signupJohn")
		hostClient := test.NewFakeClient(t, s, adminMUR, sb1, viewerMUR, sb2, johnMUR, sb3, base1nsTier)
		member1Client := test.NewFakeClient(t, nstmplSet)
		member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
		member2 := NewMemberClusterWithTenantRole(t, "member-2", corev1.ConditionTrue)

		ctrl := newReconciler(hostClient, member1, member2)
		InitializeCounters(t,
			NewToolchainStatus(
				WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
					"1,internal": 1,
				}),
				WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
					string(metrics.Internal): 1,
				}),
				WithMember("member-1", WithSpaceCount(1)),
			))

		// when
		res, err := ctrl.Reconcile(context.TODO(), requestFor(s))

		// then
		require.NoError(t, err)
		assert.True(t, res.Requeue) // expect a requeue since the NSTemplateSet was updated
		// Space should be in "updating" state while the NSTemplateSet is being updated
		spacetest.AssertThatSpace(t, test.HostOperatorNs, s.Name, hostClient).
			HasConditions(spacetest.Updating())
		// NSTemplateSet should have an spaceRoles entry for the `mur`
		nstemplatetsettest.AssertThatNSTemplateSet(t, test.MemberOperatorNs, nstmplSet.Name, member1Client).
			HasSpaceRoles(
				nstemplatetsettest.SpaceRole("base1ns-admin-123456new", adminMUR.Name, johnMUR.Name), // entry added for user 'john'
				nstemplatetsettest.SpaceRole("base1ns-viewer-123456new", viewerMUR.Name),             // entry removed for user 'john'
			).
			HasConditions(nstemplatetsettest.Provisioned()) // not changed by the SpaceController, but will be by the NSTemplateSetController
		AssertThatCountersAndMetrics(t).
			HaveSpacesForCluster("member-1", 1) // space counter is unchanged
	})
}
func TestRetargetSpace(t *testing.T) {

	// given
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)
	base1nsTier := tiertest.Base1nsTier(t, tiertest.CurrentBase1nsTemplates)

	t.Run("to empty target cluster", func(t *testing.T) {
		// given
		s := spacetest.NewSpace(test.HostOperatorNs, "oddity",
			spacetest.WithFinalizer(),
			spacetest.WithoutSpecTargetCluster(), // assume that field was reset by a client (admin, appstudio console, etc.)
			spacetest.WithStatusTargetCluster("member-1"))
		hostClient := test.NewFakeClient(t, s, base1nsTier)
		nstmplSet := nstemplatetsettest.NewNSTemplateSet("oddity", nstemplatetsettest.WithReadyCondition())
		member1Client := test.NewFakeClient(t, nstmplSet)
		member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
		member2 := NewMemberClusterWithTenantRole(t, "member-2", corev1.ConditionTrue)
		ctrl := newReconciler(hostClient, member1, member2)
		InitializeCounters(t,
			NewToolchainStatus(WithEmptyMetrics(),
				WithMember("member-1", WithSpaceCount(1)),
				WithMember("member-2", WithSpaceCount(0))))

		// when
		res, err := ctrl.Reconcile(context.TODO(), requestFor(s))

		// then
		require.NoError(t, err)
		assert.False(t, res.Requeue)
		spacetest.AssertThatSpace(t, test.HostOperatorNs, s.Name, hostClient).
			HasFinalizer().
			HasNoSpecTargetCluster().
			HasConditions(spacetest.Retargeting()).
			HasStatusTargetCluster("member-1") // not reset yet
		AssertThatCountersAndMetrics(t).
			HaveSpacesForCluster("member-1", 0). // space counter is decremented
			HaveSpacesForCluster("member-2", 0)  // space counter is unchanged

		t.Run("status target cluster reset when NSTemplateSet is deleted", func(t *testing.T) {
			// once NSTemplateSet resource is fully deleted, the SpaceController is triggered again
			// and it doesn't create any NSTemplateSet since the targetCluster is empty

			// when
			res, err := ctrl.Reconcile(context.TODO(), requestFor(s))
			// then
			require.NoError(t, err)
			assert.False(t, res.Requeue)
			spacetest.AssertThatSpace(t, test.HostOperatorNs, s.Name, hostClient).
				HasFinalizer().
				HasNoSpecTargetCluster().
				HasConditions(spacetest.ProvisioningPending("unspecified target member cluster")).
				HasNoStatusTargetCluster() // reset
			nstemplatetsettest.AssertThatNSTemplateSet(t, test.MemberOperatorNs, "oddity", member1.Client).
				DoesNotExist()
			AssertThatCountersAndMetrics(t).
				HaveSpacesForCluster("member-1", 0). // space counter is unchanged
				HaveSpacesForCluster("member-2", 0)  // space counter is unchanged
		})
	})

	t.Run("to another target cluster", func(t *testing.T) {
		// given
		s := spacetest.NewSpace(test.HostOperatorNs, "oddity",
			spacetest.WithFinalizer(),
			spacetest.WithSpecTargetCluster("member-2"), // assume that field was changed by a client (admin, appstudio console, etc.)
			spacetest.WithStatusTargetCluster("member-1"))
		hostClient := test.NewFakeClient(t, s, base1nsTier)
		nstmplSet := nstemplatetsettest.NewNSTemplateSet("oddity", nstemplatetsettest.WithReadyCondition())
		member1Client := test.NewFakeClient(t, nstmplSet)
		member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
		member2Client := test.NewFakeClient(t)
		member2 := NewMemberClusterWithClient(member2Client, "member-2", corev1.ConditionTrue)
		ctrl := newReconciler(hostClient, member1, member2)
		InitializeCounters(t,
			NewToolchainStatus(WithEmptyMetrics(),
				WithMember("member-1", WithSpaceCount(1)),
				WithMember("member-2", WithSpaceCount(0))))

		// when
		res, err := ctrl.Reconcile(context.TODO(), requestFor(s))

		// then
		require.NoError(t, err)
		assert.False(t, res.Requeue)
		spacetest.AssertThatSpace(t, test.HostOperatorNs, s.Name, hostClient).
			HasFinalizer().
			HasSpecTargetCluster("member-2").
			HasConditions(spacetest.Retargeting()).
			HasStatusTargetCluster("member-1") // not reset yet
		AssertThatCountersAndMetrics(t).
			HaveSpacesForCluster("member-1", 0). // space counter is decremented
			HaveSpacesForCluster("member-2", 0)  // space counter is unchanged

		t.Run("status target cluster is reset when NSTemplateSet is deleted on member-1", func(t *testing.T) {
			// when
			res, err := ctrl.Reconcile(context.TODO(), requestFor(s))
			// then
			require.NoError(t, err)
			assert.True(t, res.Requeue) // requeue requested explicitly when NSTemplateSet is created, even though watching the resource is enough to trigger a new reconcile loop
			spacetest.AssertThatSpace(t, s.Namespace, s.Name, hostClient).
				HasFinalizer().
				HasSpecTargetCluster("member-2").
				HasConditions(spacetest.Provisioning()).
				HasStatusTargetCluster("member-2") // updated
			nstemplatetsettest.AssertThatNSTemplateSet(t, test.MemberOperatorNs, "oddity", member1.Client).
				DoesNotExist()
			nstemplatetsettest.AssertThatNSTemplateSet(t, test.MemberOperatorNs, "oddity", member2.Client).
				Exists().
				HasTierName(base1nsTier.Name)
			AssertThatCountersAndMetrics(t).
				HaveSpacesForCluster("member-1", 0).
				HaveSpacesForCluster("member-2", 1) // increment space counter on member-2
		})
	})

	t.Run("failures", func(t *testing.T) {

		t.Run("unable to delete NSTemplateSet", func(t *testing.T) {
			// given
			s := spacetest.NewSpace(test.HostOperatorNs, "oddity",
				spacetest.WithFinalizer(),
				spacetest.WithSpecTargetCluster("member-2"), // assume that field was changed by a client (admin, appstudio console, etc.)
				spacetest.WithStatusTargetCluster("member-1"))
			hostClient := test.NewFakeClient(t, s, base1nsTier)
			nstmplSet := nstemplatetsettest.NewNSTemplateSet("oddity", nstemplatetsettest.WithReadyCondition())
			member1Client := test.NewFakeClient(t, nstmplSet)
			member1Client.MockDelete = mockDeleteNSTemplateSetFail(member1Client.Client)
			member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
			member2Client := test.NewFakeClient(t)
			member2 := NewMemberClusterWithClient(member2Client, "member-2", corev1.ConditionTrue)
			ctrl := newReconciler(hostClient, member1, member2)
			InitializeCounters(t,
				NewToolchainStatus(
					WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
						"1,internal": 1,
					}),
					WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
						string(metrics.Internal): 1,
					}),
					WithMember("member-1", WithSpaceCount(1)),
				))

			// when
			res, err := ctrl.Reconcile(context.TODO(), requestFor(s))

			// then
			require.EqualError(t, err, "mock error")
			assert.False(t, res.Requeue)
			spacetest.AssertThatSpace(t, test.HostOperatorNs, s.Name, hostClient).
				HasFinalizer().
				HasSpecTargetCluster("member-2").
				HasConditions(spacetest.RetargetingFailed("mock error")).
				HasStatusTargetCluster("member-1") // NOT updated
			AssertThatCountersAndMetrics(t).
				HaveSpacesForCluster("member-1", 1).
				HaveSpacesForCluster("member-2", 0) // space counter is unchanged when there is an error while deleting NSTemplateSet
		})

		t.Run("unable to update status, it should not change the counter", func(t *testing.T) {
			// given
			s := spacetest.NewSpace(test.HostOperatorNs, "oddity",
				spacetest.WithFinalizer(),
				spacetest.WithSpecTargetCluster("member-2"), // assume that field was changed by a client (admin, appstudio console, etc.)
				spacetest.WithStatusTargetCluster("member-1"))
			hostClient := test.NewFakeClient(t, s, base1nsTier)
			nstmplSet := nstemplatetsettest.NewNSTemplateSet("oddity", nstemplatetsettest.WithReadyCondition())
			hostClient.MockStatusUpdate = mockUpdateSpaceStatusFail(hostClient.Client)
			member1Client := test.NewFakeClient(t, nstmplSet)
			member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
			member2Client := test.NewFakeClient(t)
			member2 := NewMemberClusterWithClient(member2Client, "member-2", corev1.ConditionTrue)
			ctrl := newReconciler(hostClient, member1, member2)
			InitializeCounters(t,
				NewToolchainStatus(WithEmptyMetrics(), WithMember("member-1", WithSpaceCount(1))))

			// when
			res, err := ctrl.Reconcile(context.TODO(), requestFor(s))

			// then
			require.EqualError(t, err, "mock error")
			assert.False(t, res.Requeue)
			spacetest.AssertThatSpace(t, test.HostOperatorNs, s.Name, hostClient).
				HasFinalizer().
				HasSpecTargetCluster("member-2").
				HasNoConditions().
				HasStatusTargetCluster("member-1") // NOT updated
			AssertThatCountersAndMetrics(t).
				HaveSpacesForCluster("member-1", 1). // counter not is decremented because of the failed status updated
				HaveSpacesForCluster("member-2", 0)  // space counter is unchanged
		})

	})
}

// TestSubSpace covers the cases where there is a relationship between a parentSpace and a subSpace
// and all the consequences and expectations of this link between two different spaces.
func TestSubSpace(t *testing.T) {

	// given
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)
	base1nsTier := tiertest.Base1nsTier(t, tiertest.CurrentBase1nsTemplates)

	// test SpaceBindings of a parentSpace are created/updated/deleted ,
	// and how this should affect the subSpace and its NSTemplateSet
	t.Run("SpaceBindings inheritance ", func(t *testing.T) {

		t.Run("create parentSpace with admin and viewer roles, and expect subSpace will have same usernames and roles", func(t *testing.T) {
			// given a parentSpace...
			parentSpace := spacetest.NewSpace(test.HostOperatorNs, "parentSpace")
			// ...and their corresponding space bindings
			sb1 := spacebindingtest.NewSpaceBinding("parentSpaceAdmin", parentSpace.Name, "admin", "signupAdmin")
			sb2 := spacebindingtest.NewSpaceBinding("parentSpaceViewer", parentSpace.Name, "viewer", "signupViewer")

			// ...now create a subSpace
			subSpace := spacetest.NewSpace(test.HostOperatorNs, "subSpace",
				spacetest.WithTierName(base1nsTier.Name),
				spacetest.WithSpecTargetCluster("member-1"),     // already provisioned on a target cluster
				spacetest.WithStatusTargetCluster("member-1"),   // already provisioned on a target cluster
				spacetest.WithSpecParentSpace(parentSpace.Name), // set the parentSpace spec
				spacetest.WithFinalizer())
			subNStmplSet := nstemplatetsettest.NewNSTemplateSet(subSpace.Name,
				nstemplatetsettest.WithReadyCondition(), // create empty NSTemplate set for subSpace
			)

			hostClient := test.NewFakeClient(t, parentSpace, sb1, sb2, base1nsTier, subSpace)
			member1Client := test.NewFakeClient(t, subNStmplSet)
			member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
			ctrl := newReconciler(hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(subSpace))

			// then
			require.NoError(t, err)
			// subNSTemplateSet should have same usernames and roles as parentNSTemplateSet
			nstemplatetsettest.AssertThatNSTemplateSet(t, test.MemberOperatorNs, subNStmplSet.Name, member1Client).
				HasSpaceRoles(
					nstemplatetsettest.SpaceRole("base1ns-admin-123456new", "parentSpaceAdmin"),
					nstemplatetsettest.SpaceRole("base1ns-viewer-123456new", "parentSpaceViewer"),
				).
				HasConditions(nstemplatetsettest.Provisioned())
		})

		t.Run("create SpaceBindings for both parentSpace and subSpace, and expect subSpace will have merged roles and usernames", func(t *testing.T) {
			// given a parentSpace...
			parentSpace := spacetest.NewSpace(test.HostOperatorNs, "parentSpace")
			// ...and their corresponding space bindings
			sb1 := spacebindingtest.NewSpaceBinding("parentSpaceAdmin", parentSpace.Name, "admin", "signupAdmin")

			// ...now create a subSpace
			subSpace := spacetest.NewSpace(test.HostOperatorNs, "subSpace",
				spacetest.WithTierName(base1nsTier.Name),
				spacetest.WithSpecTargetCluster("member-1"),     // already provisioned on a target cluster
				spacetest.WithStatusTargetCluster("member-1"),   // already provisioned on a target cluster
				spacetest.WithSpecParentSpace(parentSpace.Name), // set the parentSpace spec
				spacetest.WithFinalizer())
			subNStmplSet := nstemplatetsettest.NewNSTemplateSet(subSpace.Name,
				nstemplatetsettest.WithReadyCondition(),
				nstemplatetsettest.WithReferencesFor(base1nsTier,
					// add an already existing user with a different name which is supposed to be dropped
					nstemplatetsettest.WithSpaceRole("viewer", "subViewer"),
				),
			)
			// ...and the corresponding space bindings for the subSpace
			sb2 := spacebindingtest.NewSpaceBinding("subSpaceViewer", subSpace.Name, "viewer", "signupViewer")

			hostClient := test.NewFakeClient(t, parentSpace, sb1, sb2, base1nsTier, subSpace)
			member1Client := test.NewFakeClient(t, subNStmplSet)
			member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
			ctrl := newReconciler(hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(subSpace))

			// then
			require.NoError(t, err)
			// subNSTemplateSet should have same merged roles and usernames
			nstemplatetsettest.AssertThatNSTemplateSet(t, test.MemberOperatorNs, subNStmplSet.Name, member1Client).
				HasSpaceRoles(
					nstemplatetsettest.SpaceRole("base1ns-admin-123456new", "parentSpaceAdmin"),
					nstemplatetsettest.SpaceRole("base1ns-viewer-123456new", "subSpaceViewer"),
				).
				HasConditions(nstemplatetsettest.Provisioned())
		})

		t.Run("create SpaceBindings for both parentSpace and subSpace with same username, and expect user role from subSpace will override role from parentSpace", func(t *testing.T) {
			// given a parentSpace...
			parentSpace := spacetest.NewSpace(test.HostOperatorNs, "parentSpace")
			// ...and their corresponding space bindings
			sb1 := spacebindingtest.NewSpaceBinding("john", parentSpace.Name, "admin", "signupJohn")
			sb2 := spacebindingtest.NewSpaceBinding("jane", parentSpace.Name, "viewer", "signupJane")

			// ...now create a subSpace
			subSpace := spacetest.NewSpace(test.HostOperatorNs, "subSpace",
				spacetest.WithTierName(base1nsTier.Name),
				spacetest.WithSpecTargetCluster("member-1"),
				spacetest.WithStatusTargetCluster("member-1"),   // already provisioned on a target cluster
				spacetest.WithSpecParentSpace(parentSpace.Name), // already provisioned on a target cluster
				spacetest.WithFinalizer())
			subNStmplSet := nstemplatetsettest.NewNSTemplateSet(subSpace.Name,
				nstemplatetsettest.WithReadyCondition(),
				nstemplatetsettest.WithReferencesFor(base1nsTier,
					// upgrade jane to admin for this sub-space...
					nstemplatetsettest.WithSpaceRole("admin", "jane"), // jane is admin in subSpace
				),
			)
			// ...and the corresponding space bindings for the subSpace
			sb3 := spacebindingtest.NewSpaceBinding("jane", subSpace.Name, "admin", "janeSignup")

			hostClient := test.NewFakeClient(t, parentSpace, sb1, sb2, sb3, base1nsTier, subSpace)
			member1Client := test.NewFakeClient(t, subNStmplSet)
			member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
			ctrl := newReconciler(hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(subSpace))

			// then
			require.NoError(t, err)
			// subNSTemplateSet should have both users as admin
			nstemplatetsettest.AssertThatNSTemplateSet(t, test.MemberOperatorNs, subNStmplSet.Name, member1Client).
				HasSpaceRoles(
					nstemplatetsettest.SpaceRole("base1ns-admin-123456new", "jane", "john"), // jane remains admin for subSpace
				).
				HasConditions(nstemplatetsettest.Provisioned())
		})

		t.Run("create multi level bindings tree", func(t *testing.T) {
			// given a parentSpace...
			parentSpace := spacetest.NewSpace(test.HostOperatorNs, "parentSpace")
			// ...and their corresponding space bindings
			sb1 := spacebindingtest.NewSpaceBinding("john", parentSpace.Name, "admin", "signupJohn")
			sb2 := spacebindingtest.NewSpaceBinding("jane", parentSpace.Name, "edit", "signupJane")
			sb3 := spacebindingtest.NewSpaceBinding("bob", parentSpace.Name, "view", "signupBob")
			sb4 := spacebindingtest.NewSpaceBinding("lara", parentSpace.Name, "view", "signupLara")

			// ...now create a subSpace
			subSpace := spacetest.NewSpace(test.HostOperatorNs, "subSpace",
				spacetest.WithTierName(base1nsTier.Name),
				spacetest.WithSpecTargetCluster("member-1"),
				spacetest.WithStatusTargetCluster("member-1"), // already provisioned on a target cluster
				spacetest.WithSpecParentSpace(parentSpace.Name),
				spacetest.WithFinalizer())
			subNStmplSet := nstemplatetsettest.NewNSTemplateSet(subSpace.Name,
				nstemplatetsettest.WithReadyCondition(),
			)
			sb5 := spacebindingtest.NewSpaceBinding("jane", subSpace.Name, "admin", "signupJane")
			sb6 := spacebindingtest.NewSpaceBinding("lara", subSpace.Name, "edit", "signupLara")

			// ... and now create a subSubSpace
			subSubSpace := spacetest.NewSpace(test.HostOperatorNs, "subSubSpace",
				spacetest.WithTierName(base1nsTier.Name),
				spacetest.WithSpecTargetCluster("member-1"),
				spacetest.WithStatusTargetCluster("member-1"), // already provisioned on a target cluster
				spacetest.WithSpecParentSpace(subSpace.Name),
				spacetest.WithFinalizer())
			subSubNStmplSet := nstemplatetsettest.NewNSTemplateSet(subSubSpace.Name,
				nstemplatetsettest.WithReadyCondition(),
			)
			sb7 := spacebindingtest.NewSpaceBinding("bob", subSubSpace.Name, "edit", "signupBob")
			sb8 := spacebindingtest.NewSpaceBinding("lara", subSubSpace.Name, "admin", "signupLara")

			hostClient := test.NewFakeClient(t, parentSpace, sb1, sb2, sb3, sb4, sb5, sb6, sb7, sb8, base1nsTier, subSpace, subSubSpace)
			member1Client := test.NewFakeClient(t, subNStmplSet, subSubNStmplSet)
			member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
			ctrl := newReconciler(hostClient, member1)

			// when
			_, err := ctrl.Reconcile(context.TODO(), requestFor(subSubSpace))

			// then
			require.NoError(t, err)
			// user is admin of the subSubSpace
			nstemplatetsettest.AssertThatNSTemplateSet(t, test.MemberOperatorNs, subSubNStmplSet.Name, member1Client).
				HasSpaceRoles(
					nstemplatetsettest.SpaceRole("base1ns-admin-123456new", "jane", "john", "lara"),
					nstemplatetsettest.SpaceRole("base1ns-edit-123456new", "bob"),
				).
				HasConditions(nstemplatetsettest.Provisioned())
		})

	})
}

func mockDeleteNSTemplateSetFail(cl runtimeclient.Client) func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.DeleteOption) error {
	return func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.DeleteOption) error {
		if _, ok := obj.(*toolchainv1alpha1.NSTemplateSet); ok {
			return fmt.Errorf("mock error")
		}
		return cl.Delete(ctx, obj, opts...)
	}
}

func mockUpdateSpaceStatusFail(cl runtimeclient.Client) func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.UpdateOption) error {
	return func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.UpdateOption) error {
		if _, ok := obj.(*toolchainv1alpha1.Space); ok {
			return fmt.Errorf("mock error")
		}
		return cl.Status().Update(ctx, obj, opts...)
	}
}

func mockUpdateNSTemplateSetFail(cl runtimeclient.Client) func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.UpdateOption) error {
	return func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.UpdateOption) error {
		if _, ok := obj.(*toolchainv1alpha1.NSTemplateSet); ok {
			return fmt.Errorf("mock error")
		}
		return cl.Update(ctx, obj, opts...)
	}
}

func newReconciler(hostCl runtimeclient.Client, memberClusters ...*commoncluster.CachedToolchainCluster) *space.Reconciler {
	os.Setenv("WATCH_NAMESPACE", test.HostOperatorNs)
	clusters := map[string]cluster.Cluster{}
	for _, c := range memberClusters {
		clusters[c.Name] = cluster.Cluster{
			Config: &commoncluster.Config{
				OperatorNamespace: c.OperatorNamespace,
				OwnerClusterName:  test.MemberClusterName,
			},
			Client: c.Client,
		}
	}
	return &space.Reconciler{
		Client:         hostCl,
		Namespace:      test.HostOperatorNs,
		MemberClusters: clusters,
	}
}

func requestFor(s *toolchainv1alpha1.Space) reconcile.Request {
	if s != nil {
		return reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: s.Namespace,
				Name:      s.Name,
			},
		}
	}
	return reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: test.HostOperatorNs,
			Name:      "unknown",
		},
	}
}

func TestNewNSTemplateSetSpec(t *testing.T) {

	// given
	nsTemplateTier := tiertest.NewNSTemplateTier("advanced", "dev", "stage")
	s := spacetest.NewSpace(test.HostOperatorNs, "spacejohn",
		spacetest.WithTierName(nsTemplateTier.Name),
		spacetest.WithSpecTargetCluster("member-1"))
	bindings := []toolchainv1alpha1.SpaceBinding{
		{
			Spec: toolchainv1alpha1.SpaceBindingSpec{
				MasterUserRecord: "john",
				Space:            "spacejohn",
				SpaceRole:        "admin",
			},
		},
		{
			Spec: toolchainv1alpha1.SpaceBindingSpec{
				MasterUserRecord: "joe",
				Space:            "spacejohn",
				SpaceRole:        "viewer",
			},
		},
		{
			Spec: toolchainv1alpha1.SpaceBindingSpec{
				MasterUserRecord: "jack",
				Space:            "spacejohn",
				SpaceRole:        "viewer",
			},
		},
	}

	// when
	setSpec := space.NewNSTemplateSetSpec(s, bindings, nsTemplateTier)

	// then
	assert.Equal(t, toolchainv1alpha1.NSTemplateSetSpec{
		TierName: "advanced",
		Namespaces: []toolchainv1alpha1.NSTemplateSetNamespace{
			{
				TemplateRef: "advanced-dev-123abc1",
			},
			{
				TemplateRef: "advanced-stage-123abc2",
			},
		},
		ClusterResources: &toolchainv1alpha1.NSTemplateSetClusterResources{
			TemplateRef: "advanced-clusterresources-654321b",
		},
		SpaceRoles: []toolchainv1alpha1.NSTemplateSetSpaceRole{
			{
				TemplateRef: "advanced-admin-123abc1",
				Usernames: []string{
					"john",
				},
			},
			{
				TemplateRef: "advanced-viewer-123abc2",
				Usernames: []string{
					"jack", "joe", // sorted
				},
			},
		},
	}, setSpec)
}
