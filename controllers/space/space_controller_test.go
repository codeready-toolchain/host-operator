package space_test

import (
	"context"
	"os"
	"testing"
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/controllers/space"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	"github.com/codeready-toolchain/host-operator/pkg/cluster"
	. "github.com/codeready-toolchain/host-operator/test"
	tiertest "github.com/codeready-toolchain/host-operator/test/nstemplatetier"
	spacetest "github.com/codeready-toolchain/host-operator/test/space"
	commoncluster "github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	nstemplatetsettest "github.com/codeready-toolchain/toolchain-common/pkg/test/nstemplateset"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestReconciler(t *testing.T) {

	// given
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)
	basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates)

	t.Run("create", func(t *testing.T) {

		t.Run("success", func(t *testing.T) {
			// given
			space := spacetest.NewSpace("oddity", basicTier.Name, spacetest.WithTargetCluster("member-1"))
			hostClient := test.NewFakeClient(t, space, basicTier)
			member1 := NewMemberCluster(t, "member-1", corev1.ConditionTrue)
			member2 := NewMemberCluster(t, "member-2", corev1.ConditionTrue)
			ctrl := newReconciler(hostClient, member1, member2)

			// when
			res, err := ctrl.Reconcile(context.TODO(), reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: space.Namespace,
					Name:      space.Name,
				},
			})

			// then
			require.NoError(t, err)
			assert.Equal(t, reconcile.Result{Requeue: true}, res)
			nsTmplSet := nstemplatetsettest.AssertThatNSTemplateSet(t, test.MemberOperatorNs, "oddity", member1.Client).
				Exists().
				Get()
			spacetest.AssertThatSpace(t, test.HostOperatorNs, "oddity", hostClient).
				Exists().
				HasConditions(spacetest.Provisioning()).
				HasFinalizer(toolchainv1alpha1.FinalizerName)

			t.Run("requeue while NSTemplateSet is not ready", func(t *testing.T) {
				// given another round of requeue without while NSTemplateSet is *not ready*
				nsTmplSet.Status.Conditions = []toolchainv1alpha1.Condition{
					{
						Type:   toolchainv1alpha1.ConditionReady,
						Status: corev1.ConditionFalse,
						Reason: toolchainv1alpha1.NSTemplateSetProvisioningReason,
					},
				}
				err := member1.Client.Update(context.TODO(), nsTmplSet)
				require.NoError(t, err)
				ctrl := newReconciler(hostClient, member1, member2)

				// when
				res, err := ctrl.Reconcile(context.TODO(), reconcile.Request{
					NamespacedName: types.NamespacedName{
						Namespace: space.Namespace,
						Name:      space.Name,
					},
				})

				// then
				require.NoError(t, err)
				assert.Equal(t, reconcile.Result{Requeue: true}, res)
				spacetest.AssertThatSpace(t, test.HostOperatorNs, "oddity", hostClient).
					Exists().
					HasConditions(spacetest.Provisioning()).
					HasFinalizer(toolchainv1alpha1.FinalizerName)

				t.Run("done when NSTemplateSet is ready", func(t *testing.T) {
					// given another round of requeue without with NSTemplateSet now *ready*
					nsTmplSet.Status.Conditions = []toolchainv1alpha1.Condition{
						{
							Type:   toolchainv1alpha1.ConditionReady,
							Status: corev1.ConditionTrue,
						},
					}
					err := member1.Client.Update(context.TODO(), nsTmplSet)
					require.NoError(t, err)
					ctrl := newReconciler(hostClient, member1, member2)

					// when
					res, err := ctrl.Reconcile(context.TODO(), reconcile.Request{
						NamespacedName: types.NamespacedName{
							Namespace: space.Namespace,
							Name:      space.Name,
						},
					})

					// then
					require.NoError(t, err)
					assert.Equal(t, reconcile.Result{Requeue: false}, res) // no more requeue.
					spacetest.AssertThatSpace(t, test.HostOperatorNs, "oddity", hostClient).
						Exists().
						HasConditions(spacetest.Ready()).
						HasFinalizer(toolchainv1alpha1.FinalizerName)
				})
			})
		})

		t.Run("failure", func(t *testing.T) {

			t.Run("unspecified target member cluster", func(t *testing.T) {
				// given
				space := spacetest.NewSpace("oddity", basicTier.Name)
				hostClient := test.NewFakeClient(t, space)
				member1 := NewMemberCluster(t, "member1", corev1.ConditionTrue)
				member2 := NewMemberCluster(t, "member2", corev1.ConditionTrue)
				ctrl := newReconciler(hostClient, member1, member2)

				// when
				res, err := ctrl.Reconcile(context.TODO(), reconcile.Request{
					NamespacedName: types.NamespacedName{
						Namespace: space.Namespace,
						Name:      space.Name,
					},
				})

				// then
				require.NoError(t, err)
				assert.False(t, res.Requeue)
				spacetest.AssertThatSpace(t, space.Namespace, space.Name, hostClient).HasConditions(toolchainv1alpha1.Condition{
					Type:    toolchainv1alpha1.ConditionReady,
					Status:  corev1.ConditionFalse,
					Reason:  toolchainv1alpha1.SpaceProvisioningFailedReason,
					Message: "unspecified target member cluster",
				})
			})

			t.Run("unknown target member cluster", func(t *testing.T) {
				// given
				space := spacetest.NewSpace("oddity", basicTier.Name, spacetest.WithTargetCluster("unknown"))
				hostClient := test.NewFakeClient(t, space)
				member1 := NewMemberCluster(t, "member1", corev1.ConditionTrue)
				member2 := NewMemberCluster(t, "member2", corev1.ConditionTrue)
				ctrl := newReconciler(hostClient, member1, member2)

				// when
				res, err := ctrl.Reconcile(context.TODO(), reconcile.Request{
					NamespacedName: types.NamespacedName{
						Namespace: space.Namespace,
						Name:      space.Name,
					},
				})

				// then
				require.NoError(t, err)
				assert.False(t, res.Requeue)
				spacetest.AssertThatSpace(t, space.Namespace, space.Name, hostClient).HasConditions(toolchainv1alpha1.Condition{
					Type:    toolchainv1alpha1.ConditionReady,
					Status:  corev1.ConditionFalse,
					Reason:  toolchainv1alpha1.SpaceProvisioningFailedReason,
					Message: "unknown target member cluster 'unknown'",
				})
			})
		})
	})

	t.Run("delete", func(t *testing.T) {

		t.Run("when space was successfully provisioned", func(t *testing.T) {
			// given
			space := spacetest.NewSpace("oddity", basicTier.Name,
				spacetest.WithTargetCluster("member-1"),
				spacetest.WithFinalizer(),
				spacetest.WithDeletionTimestamp())
			hostClient := test.NewFakeClient(t, space, basicTier)
			nstmplSet := nstemplatetsettest.NewNSTemplateSet("oddity", nstemplatetsettest.WithReadyCondition())
			member1Client := test.NewFakeClient(t, nstmplSet)
			member1Client.MockDelete = func(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
				t.Logf("deleting resource of type '%T'", obj)
				if nstmplSet, ok := obj.(*toolchainv1alpha1.NSTemplateSet); ok {
					now := metav1.NewTime(time.Now())
					nstmplSet.DeletionTimestamp = &now
					nstmplSet.Status.Conditions = []toolchainv1alpha1.Condition{
						nstemplatetsettest.Terminating(),
					}
					// instead of deleting the resource in the FakeClient,
					// we update it with a `DeletionTimestamp`
					return member1Client.Client.Update(ctx, obj)
				}
				return member1Client.Client.Delete(ctx, obj, opts...)
			}
			member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
			member2 := NewMemberCluster(t, "member-2", corev1.ConditionTrue)
			ctrl := newReconciler(hostClient, member1, member2)

			// when
			res, err := ctrl.Reconcile(context.TODO(), reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: space.Namespace,
					Name:      space.Name,
				},
			})

			// then
			require.NoError(t, err)
			assert.Equal(t, reconcile.Result{Requeue: true}, res)
			nsTmplSet := nstemplatetsettest.AssertThatNSTemplateSet(t, test.MemberOperatorNs, "oddity", member1.Client).
				Exists().
				HasDeletionTimestamp().
				HasConditions(nstemplatetsettest.Terminating()).
				Get()
			spacetest.AssertThatSpace(t, test.HostOperatorNs, "oddity", hostClient).
				Exists().
				HasConditions(spacetest.Terminating()).
				HasFinalizer(toolchainv1alpha1.FinalizerName)

			t.Run("requeue while NSTemplateSet is terminating", func(t *testing.T) {
				// given another round of requeue without while NSTemplateSet is *not ready*
				nsTmplSet.Status.Conditions = []toolchainv1alpha1.Condition{
					{
						Type:   toolchainv1alpha1.ConditionReady,
						Status: corev1.ConditionFalse,
						Reason: toolchainv1alpha1.NSTemplateSetTerminatingReason,
					},
				}
				err := member1.Client.Update(context.TODO(), nsTmplSet)
				require.NoError(t, err)
				ctrl := newReconciler(hostClient, member1, member2)

				// when
				res, err := ctrl.Reconcile(context.TODO(), reconcile.Request{
					NamespacedName: types.NamespacedName{
						Namespace: space.Namespace,
						Name:      space.Name,
					},
				})

				// then
				require.NoError(t, err)
				assert.Equal(t, reconcile.Result{Requeue: true}, res)
				// no changes
				nsTmplSet := nstemplatetsettest.AssertThatNSTemplateSet(t, test.MemberOperatorNs, "oddity", member1.Client).
					Exists().
					HasDeletionTimestamp().
					HasConditions(nstemplatetsettest.Terminating()).
					Get()
				spacetest.AssertThatSpace(t, test.HostOperatorNs, "oddity", hostClient).
					Exists().
					HasConditions(spacetest.Terminating()).
					HasFinalizer(toolchainv1alpha1.FinalizerName)

				t.Run("done when NSTemplateSet is deleted", func(t *testing.T) {
					// given another round of requeue without with NSTemplateSet now *fully deleted*
					member1Client.MockDelete = nil
					err := member1.Client.Delete(context.TODO(), nsTmplSet)
					require.NoError(t, err)
					ctrl := newReconciler(hostClient, member1, member2)
					// when
					res, err := ctrl.Reconcile(context.TODO(), reconcile.Request{
						NamespacedName: types.NamespacedName{
							Namespace: space.Namespace,
							Name:      space.Name,
						},
					})
					// then
					require.NoError(t, err)
					assert.Equal(t, reconcile.Result{Requeue: false}, res) // no more requeue.
					spacetest.AssertThatSpace(t, test.HostOperatorNs, "oddity", hostClient).
						Exists().
						HasConditions(spacetest.Terminating()).
						HasNoFinalizers() // space resource can be deleted by the server now that the finalizer has been removed
				})
			})
		})

		t.Run("when space was not successfully provisioned", func(t *testing.T) {

			t.Run("because of missing target member cluster", func(t *testing.T) {
				// given
				space := spacetest.NewSpace("oddity", basicTier.Name,
					spacetest.WithoutTargetCluster(),
					spacetest.WithFinalizer(),
					spacetest.WithDeletionTimestamp(),
					spacetest.WithCondition(toolchainv1alpha1.Condition{
						Type:    toolchainv1alpha1.ConditionReady,
						Status:  corev1.ConditionFalse,
						Reason:  toolchainv1alpha1.SpaceProvisioningFailedReason,
						Message: "missing target member cluster",
					}),
				)
				hostClient := test.NewFakeClient(t, space, basicTier)
				member1 := NewMemberCluster(t, "member-1", corev1.ConditionTrue)
				member2 := NewMemberCluster(t, "member-2", corev1.ConditionTrue)
				ctrl := newReconciler(hostClient, member1, member2)

				// when
				res, err := ctrl.Reconcile(context.TODO(), reconcile.Request{
					NamespacedName: types.NamespacedName{
						Namespace: space.Namespace,
						Name:      space.Name,
					},
				})

				// then
				require.NoError(t, err)
				assert.Equal(t, reconcile.Result{Requeue: false}, res) // no requeue neede
				spacetest.AssertThatSpace(t, test.HostOperatorNs, "oddity", hostClient).
					Exists().
					HasNoFinalizers() // will allow for deletion of the Space CR
			})

			t.Run("because of unknown target member cluster", func(t *testing.T) {
				// given
				space := spacetest.NewSpace("oddity", basicTier.Name,
					spacetest.WithTargetCluster("unknown"),
					spacetest.WithFinalizer(),
					spacetest.WithDeletionTimestamp(),
					spacetest.WithCondition(toolchainv1alpha1.Condition{
						Type:    toolchainv1alpha1.ConditionReady,
						Status:  corev1.ConditionFalse,
						Reason:  toolchainv1alpha1.SpaceProvisioningFailedReason,
						Message: "unknown target member cluster 'unknown'",
					}),
				)
				hostClient := test.NewFakeClient(t, space, basicTier)
				member1 := NewMemberCluster(t, "member-1", corev1.ConditionTrue)
				member2 := NewMemberCluster(t, "member-2", corev1.ConditionTrue)
				ctrl := newReconciler(hostClient, member1, member2)

				// when
				res, err := ctrl.Reconcile(context.TODO(), reconcile.Request{
					NamespacedName: types.NamespacedName{
						Namespace: space.Namespace,
						Name:      space.Name,
					},
				})

				// then
				require.NoError(t, err)
				assert.Equal(t, reconcile.Result{Requeue: false}, res) // no requeue neede
				spacetest.AssertThatSpace(t, test.HostOperatorNs, "oddity", hostClient).
					Exists().
					HasNoFinalizers() // will allow for deletion of the Space CR
			})
		})
	})
}

func newReconciler(hostCl client.Client, memberClusters ...*commoncluster.CachedToolchainCluster) *space.Reconciler {
	os.Setenv("WATCH_NAMESPACE", test.HostOperatorNs)
	clusters := map[string]cluster.Cluster{}
	for _, c := range memberClusters {
		clusters[c.Name] = cluster.Cluster{
			OperatorNamespace: c.OperatorNamespace,
			Client:            c.Client,
		}
	}
	return &space.Reconciler{
		Client:         hostCl,
		MemberClusters: clusters,
	}
}
