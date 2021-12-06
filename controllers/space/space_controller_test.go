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
			assert.Equal(t, reconcile.Result{Requeue: false}, res) // no need to requeue when creating the NSTemplateSet
			nsTmplSet := nstemplatetsettest.AssertThatNSTemplateSet(t, test.MemberOperatorNs, "oddity", member1.Client).
				Exists().
				Get()
			spacetest.AssertThatSpace(t, test.HostOperatorNs, "oddity", hostClient).
				Exists().
				HasStatusTargetCluster("member-1").
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
					HasStatusTargetCluster("member-1").
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
						HasStatusTargetCluster("member-1").
						HasConditions(spacetest.Ready()).
						HasFinalizer(toolchainv1alpha1.FinalizerName)
				})
			})
		})

		t.Run("failure", func(t *testing.T) {

			t.Run("space not found", func(t *testing.T) {
				// given
				hostClient := test.NewFakeClient(t)
				member1 := NewMemberCluster(t, "member-1", corev1.ConditionTrue)
				member2 := NewMemberCluster(t, "member-2", corev1.ConditionTrue)
				ctrl := newReconciler(hostClient, member1, member2)

				// when
				res, err := ctrl.Reconcile(context.TODO(), reconcile.Request{
					NamespacedName: types.NamespacedName{
						Namespace: test.HostOperatorNs,
						Name:      "unknown",
					},
				})

				// then
				require.NoError(t, err) // not an error, space simply doesn't exist :shrug:
				assert.False(t, res.Requeue)
			})

			t.Run("error while getting space", func(t *testing.T) {
				// given
				space := spacetest.NewSpace("oddity", basicTier.Name)
				hostClient := test.NewFakeClient(t, space)
				hostClient.MockGet = func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
					if _, ok := obj.(*toolchainv1alpha1.Space); ok {
						return fmt.Errorf("mock error!")
					}
					return hostClient.Client.Get(ctx, key, obj)
				}
				member1 := NewMemberCluster(t, "member-1", corev1.ConditionTrue)
				member2 := NewMemberCluster(t, "member-2", corev1.ConditionTrue)
				ctrl := newReconciler(hostClient, member1, member2)

				// when
				res, err := ctrl.Reconcile(context.TODO(), reconcile.Request{
					NamespacedName: types.NamespacedName{
						Namespace: test.HostOperatorNs,
						Name:      "unknown",
					},
				})

				// then
				require.EqualError(t, err, "unable to get the current Space: mock error!")
				assert.False(t, res.Requeue)
			})

			t.Run("error while adding finalizer", func(t *testing.T) {
				// given
				space := spacetest.NewSpace("oddity", basicTier.Name)
				hostClient := test.NewFakeClient(t, space)
				hostClient.MockUpdate = func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
					fmt.Printf("updating object of type %T\n", obj)
					if _, ok := obj.(*toolchainv1alpha1.Space); ok {
						return fmt.Errorf("mock error!")
					}
					return hostClient.Client.Update(ctx, obj, opts...)
				}
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
				require.EqualError(t, err, "mock error!")
				assert.False(t, res.Requeue)
			})

			t.Run("unspecified target member cluster", func(t *testing.T) {
				// given
				space := spacetest.NewSpace("oddity", basicTier.Name)
				hostClient := test.NewFakeClient(t, space)
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
				require.EqualError(t, err, "unspecified target member cluster")
				assert.False(t, res.Requeue)
				spacetest.AssertThatSpace(t, space.Namespace, space.Name, hostClient).
					HasNoStatusTargetCluster().
					HasConditions(toolchainv1alpha1.Condition{
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
				require.EqualError(t, err, "unknown target member cluster 'unknown'")
				assert.False(t, res.Requeue)
				spacetest.AssertThatSpace(t, space.Namespace, space.Name, hostClient).
					HasStatusTargetCluster("unknown").
					HasConditions(toolchainv1alpha1.Condition{
						Type:    toolchainv1alpha1.ConditionReady,
						Status:  corev1.ConditionFalse,
						Reason:  toolchainv1alpha1.SpaceProvisioningFailedReason,
						Message: "unknown target member cluster 'unknown'",
					})
			})

			t.Run("error while getting NSTemplateTier on host cluster", func(t *testing.T) {
				// given
				space := spacetest.NewSpace("oddity", basicTier.Name,
					spacetest.WithTargetCluster("member-1"),
					spacetest.WithFinalizer())
				hostClient := test.NewFakeClient(t, space)
				hostClient.MockGet = func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
					if _, ok := obj.(*toolchainv1alpha1.NSTemplateTier); ok {
						return fmt.Errorf("mock error!")
					}
					return hostClient.Client.Get(ctx, key, obj)
				}
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
				require.EqualError(t, err, "mock error!")
				assert.False(t, res.Requeue)
				spacetest.AssertThatSpace(t, space.Namespace, space.Name, hostClient).
					HasStatusTargetCluster("member-1").
					HasConditions(toolchainv1alpha1.Condition{
						Type:    toolchainv1alpha1.ConditionReady,
						Status:  corev1.ConditionFalse,
						Reason:  toolchainv1alpha1.SpaceProvisioningFailedReason,
						Message: "mock error!",
					})
			})

			t.Run("error while getting NSTemplateSet on member cluster", func(t *testing.T) {
				// given
				space := spacetest.NewSpace("oddity", basicTier.Name,
					spacetest.WithTargetCluster("member-1"),
					spacetest.WithFinalizer())
				hostClient := test.NewFakeClient(t, space)
				member1Client := test.NewFakeClient(t)
				member1Client.MockGet = func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
					if _, ok := obj.(*toolchainv1alpha1.NSTemplateSet); ok {
						return fmt.Errorf("mock error!")
					}
					return member1Client.Client.Get(ctx, key, obj)
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
				require.EqualError(t, err, "mock error!")
				assert.False(t, res.Requeue)
				spacetest.AssertThatSpace(t, space.Namespace, space.Name, hostClient).
					HasStatusTargetCluster("member-1").
					HasConditions(toolchainv1alpha1.Condition{
						Type:    toolchainv1alpha1.ConditionReady,
						Status:  corev1.ConditionFalse,
						Reason:  toolchainv1alpha1.SpaceUnableToCreateNSTemplateSetReason,
						Message: "mock error!",
					})
			})

			t.Run("error while creating NSTemplateSet on member cluster", func(t *testing.T) {
				// given
				space := spacetest.NewSpace("oddity", basicTier.Name,
					spacetest.WithTargetCluster("member-1"),
					spacetest.WithFinalizer())
				hostClient := test.NewFakeClient(t, space, basicTier)
				member1Client := test.NewFakeClient(t)
				member1Client.MockCreate = func(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
					if _, ok := obj.(*toolchainv1alpha1.NSTemplateSet); ok {
						return fmt.Errorf("mock error!")
					}
					return member1Client.Client.Create(ctx, obj, opts...)
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
				require.EqualError(t, err, "mock error!")
				assert.Equal(t, reconcile.Result{Requeue: false}, res)
				spacetest.AssertThatSpace(t, space.Namespace, space.Name, hostClient).
					HasStatusTargetCluster("member-1").
					HasConditions(toolchainv1alpha1.Condition{
						Type:    toolchainv1alpha1.ConditionReady,
						Status:  corev1.ConditionFalse,
						Reason:  toolchainv1alpha1.SpaceUnableToCreateNSTemplateSetReason,
						Message: "mock error!",
					})
			})

			t.Run("error while updating status after creating NSTemplateSet", func(t *testing.T) {
				// given
				space := spacetest.NewSpace("oddity", basicTier.Name,
					spacetest.WithTargetCluster("member-1"),
					spacetest.WithFinalizer())
				hostClient := test.NewFakeClient(t, space, basicTier)
				hostClient.MockStatusUpdate = func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
					if _, ok := obj.(*toolchainv1alpha1.Space); ok {
						return fmt.Errorf("mock error!")
					}
					return hostClient.Client.Status().Update(ctx, obj, opts...)
				}
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
				require.EqualError(t, err, "mock error!")
				assert.Equal(t, reconcile.Result{Requeue: false}, res)
				spacetest.AssertThatSpace(t, space.Namespace, space.Name, hostClient).
					HasStatusTargetCluster(""). // not set
					HasNoConditions()           // not set
			})
		})
	})

	t.Run("delete", func(t *testing.T) {

		t.Run("success", func(t *testing.T) {

			t.Run("after space was successfully provisioned", func(t *testing.T) {

				t.Run("when space is deleted", func(t *testing.T) {
					// given
					space := spacetest.NewSpace("oddity", basicTier.Name,
						spacetest.WithTargetCluster("member-1"),
						spacetest.WithFinalizer(),
						spacetest.WithDeletionTimestamp())
					hostClient := test.NewFakeClient(t, space, basicTier)
					nstmplSet := nstemplatetsettest.NewNSTemplateSet("oddity", nstemplatetsettest.WithReadyCondition())
					member1Client := test.NewFakeClient(t, nstmplSet)
					member1Client.MockDelete = func(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
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
					assert.Equal(t, reconcile.Result{Requeue: false}, res) // no need to explicitly requeue while the NSTemplate is terminating
					nsTmplSet := nstemplatetsettest.AssertThatNSTemplateSet(t, test.MemberOperatorNs, "oddity", member1.Client).
						Exists().
						HasDeletionTimestamp().
						HasConditions(nstemplatetsettest.Terminating()).
						Get()
					spacetest.AssertThatSpace(t, test.HostOperatorNs, "oddity", hostClient).
						Exists().
						HasStatusTargetCluster("member-1").
						HasConditions(spacetest.Terminating()).
						HasFinalizer(toolchainv1alpha1.FinalizerName) // finalizer is still present while the NSTemplateSet is not fully deleted

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
						assert.Equal(t, reconcile.Result{Requeue: false}, res) // no need to explicitly requeue while the NSTemplate is terminating
						// no changes
						nsTmplSet := nstemplatetsettest.AssertThatNSTemplateSet(t, test.MemberOperatorNs, "oddity", member1.Client).
							Exists().
							HasDeletionTimestamp().
							HasConditions(nstemplatetsettest.Terminating()).
							Get()
						spacetest.AssertThatSpace(t, test.HostOperatorNs, "oddity", hostClient).
							Exists().
							HasStatusTargetCluster("member-1").
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
								HasStatusTargetCluster("member-1").
								HasConditions(spacetest.Terminating()).
								HasNoFinalizers() // space resource can be deleted by the server now that the finalizer has been removed
						})
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
						HasNoStatusTargetCluster().
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
						HasNoStatusTargetCluster().
						HasNoFinalizers() // will allow for deletion of the Space CR
				})
			})

		})

		t.Run("failure", func(t *testing.T) {

			t.Run("error while getting NSTemplateSet on member cluster", func(t *testing.T) {
				// given
				space := spacetest.NewSpace("oddity", basicTier.Name,
					spacetest.WithTargetCluster("member-1"),
					spacetest.WithFinalizer(),
					spacetest.WithDeletionTimestamp())
				hostClient := test.NewFakeClient(t, space)
				member1Client := test.NewFakeClient(t)
				member1Client.MockGet = func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
					if _, ok := obj.(*toolchainv1alpha1.NSTemplateSet); ok {
						return fmt.Errorf("mock error!")
					}
					return member1Client.Client.Get(ctx, key, obj)
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
				require.EqualError(t, err, "mock error!")
				assert.False(t, res.Requeue)
				spacetest.AssertThatSpace(t, test.HostOperatorNs, "oddity", hostClient).
					Exists().
					HasStatusTargetCluster("member-1").
					HasConditions(spacetest.TerminatingFailed("mock error!")).
					HasFinalizer(toolchainv1alpha1.FinalizerName)
			})

			t.Run("error while deleting NSTemplateSet", func(t *testing.T) {
				// given
				space := spacetest.NewSpace("oddity", basicTier.Name,
					spacetest.WithTargetCluster("member-1"),
					spacetest.WithFinalizer(),
					spacetest.WithDeletionTimestamp())
				hostClient := test.NewFakeClient(t, space, basicTier)
				nstmplSet := nstemplatetsettest.NewNSTemplateSet("oddity", nstemplatetsettest.WithReadyCondition())
				member1Client := test.NewFakeClient(t, nstmplSet)
				member1Client.MockDelete = func(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
					if _, ok := obj.(*toolchainv1alpha1.NSTemplateSet); ok {
						return fmt.Errorf("mock error!")
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
				require.EqualError(t, err, "mock error!")
				assert.Equal(t, reconcile.Result{Requeue: false}, res)
				spacetest.AssertThatSpace(t, test.HostOperatorNs, "oddity", hostClient).
					Exists().
					HasStatusTargetCluster("member-1").
					HasConditions(spacetest.TerminatingFailed("mock error!")).
					HasFinalizer(toolchainv1alpha1.FinalizerName)
			})

			t.Run("error while updating status after deleting NSTemplateSet", func(t *testing.T) {
				// given
				space := spacetest.NewSpace("oddity", basicTier.Name,
					spacetest.WithTargetCluster("member-1"),
					spacetest.WithFinalizer(),
					spacetest.WithDeletionTimestamp())
				hostClient := test.NewFakeClient(t, space, basicTier)
				hostClient.MockStatusUpdate = func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
					if _, ok := obj.(*toolchainv1alpha1.Space); ok {
						return fmt.Errorf("mock error!")
					}
					return hostClient.Client.Status().Update(ctx, obj, opts...)
				}
				nstmplSet := nstemplatetsettest.NewNSTemplateSet("oddity", nstemplatetsettest.WithReadyCondition())
				member1Client := test.NewFakeClient(t, nstmplSet)
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
				require.EqualError(t, err, "mock error!")
				assert.Equal(t, reconcile.Result{Requeue: false}, res)
				spacetest.AssertThatSpace(t, test.HostOperatorNs, "oddity", hostClient).
					Exists().
					HasNoConditions(). // no condition set after deleting the NSTemplateSet (note: this test does not cover conditions set while provisioning the Space)
					HasFinalizer(toolchainv1alpha1.FinalizerName)
			})

			t.Run("error while removing finalizer after deleting NSTemplateSet", func(t *testing.T) {
				// given
				space := spacetest.NewSpace("oddity", basicTier.Name,
					spacetest.WithTargetCluster("member-1"),
					spacetest.WithFinalizer(),
					spacetest.WithDeletionTimestamp())
				hostClient := test.NewFakeClient(t, space, basicTier)
				nstmplSet := nstemplatetsettest.NewNSTemplateSet("oddity", nstemplatetsettest.WithReadyCondition())
				member1Client := test.NewFakeClient(t, nstmplSet)
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
				assert.Equal(t, reconcile.Result{Requeue: false}, res) // no need to explicitly requeue while the NSTemplate is terminating
				spacetest.AssertThatSpace(t, test.HostOperatorNs, "oddity", hostClient).
					Exists().
					HasStatusTargetCluster("member-1").
					HasConditions(spacetest.Terminating()).
					HasFinalizer(toolchainv1alpha1.FinalizerName) // finalizer wasn't removed

				t.Run("fail when removing finalizer", func(t *testing.T) {
					// given
					hostClient.MockUpdate = func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
						if _, ok := obj.(*toolchainv1alpha1.Space); ok {
							return fmt.Errorf("mock error!")
						}
						return hostClient.Client.Update(ctx, obj, opts...)
					}
					ctrl := newReconciler(hostClient, member1, member2)
					// when
					res, err := ctrl.Reconcile(context.TODO(), reconcile.Request{
						NamespacedName: types.NamespacedName{
							Namespace: space.Namespace,
							Name:      space.Name,
						},
					})

					// then
					require.EqualError(t, err, "mock error!")
					assert.Equal(t, reconcile.Result{Requeue: false}, res) // no need to explicitly requeue while the NSTemplate is terminating
					spacetest.AssertThatSpace(t, test.HostOperatorNs, "oddity", hostClient).
						Exists().
						HasStatusTargetCluster("member-1").
						HasConditions(spacetest.TerminatingFailed("mock error!"))
					// finalizer should be removed, but FakeClient performs a full update of the resource, so we can't verify that:
					// see https://github.com/kubernetes-sigs/controller-runtime/blob/v0.8.3/pkg/client/fake/client.go#L522-L526

				})
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
		Namespace:      test.HostOperatorNs,
		MemberClusters: clusters,
	}
}
