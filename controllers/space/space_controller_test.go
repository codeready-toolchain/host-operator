package space_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	tierutil "github.com/codeready-toolchain/host-operator/controllers/nstemplatetier/util"
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

func TestCreateSpace(t *testing.T) {

	// given
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)
	basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates)
	t.Run("success", func(t *testing.T) {
		// given
		s := spacetest.NewSpace("oddity", spacetest.WithTargetCluster("member-1"))
		hostClient := test.NewFakeClient(t, s, basicTier)
		member1 := NewMemberCluster(t, "member-1", corev1.ConditionTrue)
		member2 := NewMemberCluster(t, "member-2", corev1.ConditionTrue)
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
			HasFinalizer(toolchainv1alpha1.FinalizerName)
		nsTmplSet := nstemplatetsettest.AssertThatNSTemplateSet(t, test.MemberOperatorNs, "oddity", member1.Client).
			Exists().
			HasClusterResourcesTemplateRef("basic-clusterresources-123456new").
			HasNamespaceTemplateRefs("basic-code-123456new", "basic-dev-123456new", "basic-stage-123456new").
			Get()

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
				HasFinalizer(toolchainv1alpha1.FinalizerName)
			nsTmplSet := nstemplatetsettest.AssertThatNSTemplateSet(t, test.MemberOperatorNs, "oddity", member1.Client).
				Exists().
				HasClusterResourcesTemplateRef("basic-clusterresources-123456new").
				HasNamespaceTemplateRefs("basic-code-123456new", "basic-dev-123456new", "basic-stage-123456new").
				Get()

			t.Run("done when NSTemplateSet is ready", func(t *testing.T) {
				// given another round of requeue without with NSTemplateSet now *ready*
				nsTmplSet.Status.Conditions = []toolchainv1alpha1.Condition{
					nstemplatetsettest.Provisioned(),
				}
				err := member1.Client.Update(context.TODO(), nsTmplSet)
				require.NoError(t, err)
				ctrl := newReconciler(hostClient, member1, member2)

				// when
				res, err := ctrl.Reconcile(context.TODO(), requestFor(s))

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
			res, err := ctrl.Reconcile(context.TODO(), requestFor(nil))

			// then
			require.NoError(t, err) // not an error, space simply doesn't exist :shrug:
			assert.False(t, res.Requeue)
		})

		t.Run("error while getting space", func(t *testing.T) {
			// given
			s := spacetest.NewSpace("oddity")
			hostClient := test.NewFakeClient(t, s)
			hostClient.MockGet = func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
				if _, ok := obj.(*toolchainv1alpha1.Space); ok {
					return fmt.Errorf("mock error")
				}
				return hostClient.Client.Get(ctx, key, obj)
			}
			member1 := NewMemberCluster(t, "member-1", corev1.ConditionTrue)
			member2 := NewMemberCluster(t, "member-2", corev1.ConditionTrue)
			ctrl := newReconciler(hostClient, member1, member2)

			// when
			res, err := ctrl.Reconcile(context.TODO(), requestFor(s))

			// then
			require.EqualError(t, err, "unable to get the current Space: mock error")
			assert.False(t, res.Requeue)
		})

		t.Run("error while adding finalizer", func(t *testing.T) {
			// given
			s := spacetest.NewSpace("oddity")
			hostClient := test.NewFakeClient(t, s)
			hostClient.MockUpdate = func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
				if _, ok := obj.(*toolchainv1alpha1.Space); ok {
					return fmt.Errorf("mock error")
				}
				return hostClient.Client.Update(ctx, obj, opts...)
			}
			member1 := NewMemberCluster(t, "member-1", corev1.ConditionTrue)
			member2 := NewMemberCluster(t, "member-2", corev1.ConditionTrue)
			ctrl := newReconciler(hostClient, member1, member2)

			// when
			res, err := ctrl.Reconcile(context.TODO(), requestFor(s))

			// then
			require.EqualError(t, err, "mock error")
			assert.False(t, res.Requeue)
		})

		t.Run("unspecified target member cluster", func(t *testing.T) {
			// given
			s := spacetest.NewSpace("oddity")
			hostClient := test.NewFakeClient(t, s)
			member1 := NewMemberCluster(t, "member-1", corev1.ConditionTrue)
			member2 := NewMemberCluster(t, "member-2", corev1.ConditionTrue)
			ctrl := newReconciler(hostClient, member1, member2)

			// when
			res, err := ctrl.Reconcile(context.TODO(), requestFor(s))

			// then
			require.EqualError(t, err, "unspecified target member cluster")
			assert.False(t, res.Requeue)
			spacetest.AssertThatSpace(t, s.Namespace, s.Name, hostClient).
				HasNoStatusTargetCluster().
				HasConditions(spacetest.ProvisioningPending("unspecified target member cluster"))
		})

		t.Run("unknown target member cluster", func(t *testing.T) {
			// given
			s := spacetest.NewSpace("oddity", spacetest.WithTargetCluster("unknown"))
			hostClient := test.NewFakeClient(t, s)
			member1 := NewMemberCluster(t, "member-1", corev1.ConditionTrue)
			member2 := NewMemberCluster(t, "member-2", corev1.ConditionTrue)
			ctrl := newReconciler(hostClient, member1, member2)

			// when
			res, err := ctrl.Reconcile(context.TODO(), requestFor(s))

			// then
			require.EqualError(t, err, "unknown target member cluster 'unknown'")
			assert.False(t, res.Requeue)
			spacetest.AssertThatSpace(t, s.Namespace, s.Name, hostClient).
				HasStatusTargetCluster("unknown").
				HasConditions(spacetest.ProvisioningFailed("unknown target member cluster 'unknown'"))
		})

		t.Run("error while getting NSTemplateTier on host cluster", func(t *testing.T) {
			// given
			s := spacetest.NewSpace("oddity",
				spacetest.WithTargetCluster("member-1"),
				spacetest.WithFinalizer())
			hostClient := test.NewFakeClient(t, s)
			hostClient.MockGet = func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
				if _, ok := obj.(*toolchainv1alpha1.NSTemplateTier); ok {
					return fmt.Errorf("mock error")
				}
				return hostClient.Client.Get(ctx, key, obj)
			}
			member1 := NewMemberCluster(t, "member-1", corev1.ConditionTrue)
			member2 := NewMemberCluster(t, "member-2", corev1.ConditionTrue)
			ctrl := newReconciler(hostClient, member1, member2)

			// when
			res, err := ctrl.Reconcile(context.TODO(), requestFor(s))

			// then
			require.EqualError(t, err, "mock error")
			assert.False(t, res.Requeue)
			spacetest.AssertThatSpace(t, s.Namespace, s.Name, hostClient).
				HasTargetCluster("member-1").
				HasStatusTargetCluster("member-1").
				HasConditions(spacetest.ProvisioningFailed("mock error"))
		})

		t.Run("error while getting NSTemplateSet on member cluster", func(t *testing.T) {
			// given
			s := spacetest.NewSpace("oddity",
				spacetest.WithTargetCluster("member-1"),
				spacetest.WithFinalizer())
			hostClient := test.NewFakeClient(t, s, basicTier)
			member1Client := test.NewFakeClient(t)
			member1Client.MockGet = func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
				if _, ok := obj.(*toolchainv1alpha1.NSTemplateSet); ok {
					return fmt.Errorf("mock error")
				}
				return member1Client.Client.Get(ctx, key, obj)
			}
			member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
			member2 := NewMemberCluster(t, "member-2", corev1.ConditionTrue)
			ctrl := newReconciler(hostClient, member1, member2)

			// when
			res, err := ctrl.Reconcile(context.TODO(), requestFor(s))

			// then
			require.EqualError(t, err, "mock error")
			assert.False(t, res.Requeue)
			spacetest.AssertThatSpace(t, s.Namespace, s.Name, hostClient).
				HasStatusTargetCluster("member-1").
				HasConditions(spacetest.UnableToCreateNSTemplateSet("mock error"))
		})

		t.Run("error while creating NSTemplateSet on member cluster", func(t *testing.T) {
			// given
			s := spacetest.NewSpace("oddity",
				spacetest.WithTargetCluster("member-1"),
				spacetest.WithFinalizer())
			hostClient := test.NewFakeClient(t, s, basicTier)
			member1Client := test.NewFakeClient(t)
			member1Client.MockCreate = func(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
				if _, ok := obj.(*toolchainv1alpha1.NSTemplateSet); ok {
					return fmt.Errorf("mock error")
				}
				return member1Client.Client.Create(ctx, obj, opts...)
			}
			member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
			member2 := NewMemberCluster(t, "member-2", corev1.ConditionTrue)
			ctrl := newReconciler(hostClient, member1, member2)

			// when
			res, err := ctrl.Reconcile(context.TODO(), requestFor(s))

			// then
			require.EqualError(t, err, "mock error")
			assert.Equal(t, reconcile.Result{Requeue: false}, res)
			spacetest.AssertThatSpace(t, s.Namespace, s.Name, hostClient).
				HasStatusTargetCluster("member-1").
				HasConditions(spacetest.UnableToCreateNSTemplateSet("mock error"))
		})

		t.Run("error while updating status after creating NSTemplateSet", func(t *testing.T) {
			// given
			s := spacetest.NewSpace("oddity",
				spacetest.WithTargetCluster("member-1"),
				spacetest.WithFinalizer())
			hostClient := test.NewFakeClient(t, s, basicTier)
			hostClient.MockStatusUpdate = func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
				if _, ok := obj.(*toolchainv1alpha1.Space); ok {
					return fmt.Errorf("mock error")
				}
				return hostClient.Client.Status().Update(ctx, obj, opts...)
			}
			member1 := NewMemberCluster(t, "member-1", corev1.ConditionTrue)
			member2 := NewMemberCluster(t, "member-2", corev1.ConditionTrue)
			ctrl := newReconciler(hostClient, member1, member2)

			// when
			res, err := ctrl.Reconcile(context.TODO(), requestFor(s))

			// then
			require.EqualError(t, err, "mock error")
			assert.Equal(t, reconcile.Result{Requeue: false}, res)
			spacetest.AssertThatSpace(t, s.Namespace, s.Name, hostClient).
				HasStatusTargetCluster(""). // not set
				HasNoConditions()           // not set
		})
	})
}

func TestDeleteSpace(t *testing.T) {

	// given
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)
	basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates)

	t.Run("success", func(t *testing.T) {

		t.Run("after space was successfully provisioned", func(t *testing.T) {

			t.Run("when space is deleted", func(t *testing.T) {
				// given a space that is being deleted
				s := spacetest.NewSpace("oddity",
					spacetest.WithTargetCluster("member-1"),
					spacetest.WithFinalizer(),
					spacetest.WithDeletionTimestamp())
				hostClient := test.NewFakeClient(t, s, basicTier)
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
				res, err := ctrl.Reconcile(context.TODO(), requestFor(s))

				// then
				require.NoError(t, err)
				assert.Equal(t, reconcile.Result{Requeue: false}, res) // no need to explicitly requeue while the NSTemplate is terminating
				spacetest.AssertThatSpace(t, s.Namespace, s.Name, hostClient).
					Exists().
					HasStatusTargetCluster("member-1").
					HasConditions(spacetest.Terminating()).
					HasFinalizer(toolchainv1alpha1.FinalizerName) // finalizer is still present while the NSTemplateSet is not fully deleted
				nsTmplSet := nstemplatetsettest.AssertThatNSTemplateSet(t, test.MemberOperatorNs, "oddity", member1.Client).
					Exists().
					HasDeletionTimestamp().
					HasConditions(nstemplatetsettest.Terminating()).
					Get()

				t.Run("wait while NSTemplateSet is still terminating", func(t *testing.T) {
					// given another reconcile loop while the NSTemplateSet is *terminating* (ie, user namespaces are being deleted)
					nsTmplSet.Status.Conditions = []toolchainv1alpha1.Condition{
						nstemplatetsettest.Terminating(),
					}
					err := member1.Client.Update(context.TODO(), nsTmplSet)
					require.NoError(t, err)

					// when
					res, err := ctrl.Reconcile(context.TODO(), requestFor(s))

					// then
					require.NoError(t, err)
					assert.Equal(t, reconcile.Result{Requeue: false}, res) // no need to explicitly requeue while the NSTemplate is terminating
					// no changes
					spacetest.AssertThatSpace(t, s.Namespace, s.Name, hostClient).
						Exists().
						HasStatusTargetCluster("member-1").
						HasConditions(spacetest.Terminating()).
						HasFinalizer(toolchainv1alpha1.FinalizerName)
					nsTmplSet := nstemplatetsettest.AssertThatNSTemplateSet(t, test.MemberOperatorNs, "oddity", member1.Client).
						Exists().
						HasDeletionTimestamp().
						HasConditions(nstemplatetsettest.Terminating()).
						Get()

					t.Run("done when NSTemplateSet is deleted", func(t *testing.T) {
						// given another reconcile loop with the NSTemplateSet now *fully deleted*
						member1Client.MockDelete = nil
						err := member1.Client.Delete(context.TODO(), nsTmplSet)
						require.NoError(t, err)
						// when
						res, err := ctrl.Reconcile(context.TODO(), requestFor(s))
						// then
						require.NoError(t, err)
						assert.False(t, res.Requeue)
						// no changes
						spacetest.AssertThatSpace(t, s.Namespace, s.Name, hostClient).
							Exists().
							HasStatusTargetCluster("member-1").
							HasConditions(spacetest.Terminating()).
							HasNoFinalizers() // space resource can be deleted by the server now that the finalizer has been removed
					})
				})
			})

			t.Run("when using status target cluster", func(t *testing.T) {
				// given
				s := spacetest.NewSpace("oddity",
					spacetest.WithoutTargetCluster(),              // targetCluster is not specified in spec ...
					spacetest.WithStatusTargetCluster("member-1"), // ... but is available in status
					spacetest.WithFinalizer(),
					spacetest.WithDeletionTimestamp())
				hostClient := test.NewFakeClient(t, s, basicTier)
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
				res, err := ctrl.Reconcile(context.TODO(), requestFor(s))

				// then
				require.NoError(t, err)
				assert.Equal(t, reconcile.Result{Requeue: false}, res) // no need to explicitly requeue while the NSTemplate is terminating
				spacetest.AssertThatSpace(t, s.Namespace, s.Name, hostClient).
					Exists().
					HasStatusTargetCluster("member-1").
					HasConditions(spacetest.Terminating()).
					HasFinalizer(toolchainv1alpha1.FinalizerName) // finalizer is still present while the NSTemplateSet is not fully deleted
				nstemplatetsettest.AssertThatNSTemplateSet(t, test.MemberOperatorNs, "oddity", member1.Client).
					Exists().
					HasDeletionTimestamp().
					HasConditions(nstemplatetsettest.Terminating()).
					Get()
				// stop the test here: it verified that the NSTemlateSet deletion was triggered (the rest is already covered above)
			})

			t.Run("no target cluster", func(t *testing.T) {
				// given
				s := spacetest.NewSpace("oddity",
					spacetest.WithFinalizer(),
					spacetest.WithDeletionTimestamp())
				hostClient := test.NewFakeClient(t, s, basicTier)
				ctrl := newReconciler(hostClient)

				// when
				_, err := ctrl.Reconcile(context.TODO(), reconcile.Request{
					NamespacedName: types.NamespacedName{
						Namespace: s.Namespace,
						Name:      s.Name,
					},
				})

				// then
				require.NoError(t, err)
				spacetest.AssertThatSpace(t, s.Namespace, s.Name, hostClient).
					Exists().
					HasNoStatusTargetCluster().
					HasNoConditions(). // no need to set any `terminating` condition since the resource can be immediately deleted
					HasNoFinalizers()  // finalizer was directly removed since there was no NSTemplateSet to delete (since no target member cluster where to look it up)

			})
		})

		t.Run("when space was not successfully provisioned", func(t *testing.T) {

			t.Run("because of missing target member cluster", func(t *testing.T) {
				// given
				s := spacetest.NewSpace("oddity",
					spacetest.WithoutTargetCluster(),
					spacetest.WithFinalizer(),
					spacetest.WithDeletionTimestamp(),
					spacetest.WithCondition(spacetest.ProvisioningFailed("missing target member cluster")),
				)
				hostClient := test.NewFakeClient(t, s, basicTier)
				member1 := NewMemberCluster(t, "member-1", corev1.ConditionTrue)
				member2 := NewMemberCluster(t, "member-2", corev1.ConditionTrue)
				ctrl := newReconciler(hostClient, member1, member2)

				// when
				res, err := ctrl.Reconcile(context.TODO(), requestFor(s))

				// then
				require.NoError(t, err)
				assert.Equal(t, reconcile.Result{Requeue: false}, res) // no requeue needed
				spacetest.AssertThatSpace(t, s.Namespace, s.Name, hostClient).
					Exists().
					HasNoStatusTargetCluster().
					HasNoFinalizers() // will allow for deletion of the Space CR
				nstemplatetsettest.AssertThatNSTemplateSet(t, test.MemberOperatorNs, "oddity", member1.Client).
					DoesNotExist()
			})

			t.Run("because of unknown target member cluster", func(t *testing.T) {
				// given
				s := spacetest.NewSpace("oddity",
					spacetest.WithTargetCluster("unknown"),
					spacetest.WithFinalizer(),
					spacetest.WithDeletionTimestamp(),
					spacetest.WithCondition(spacetest.ProvisioningFailed("unknown target member cluster 'unknown'")),
				)
				hostClient := test.NewFakeClient(t, s, basicTier)
				member1 := NewMemberCluster(t, "member-1", corev1.ConditionTrue)
				member2 := NewMemberCluster(t, "member-2", corev1.ConditionTrue)
				ctrl := newReconciler(hostClient, member1, member2)

				// when
				res, err := ctrl.Reconcile(context.TODO(), requestFor(s))

				// then
				require.EqualError(t, err, "cannot delete NSTemplateSet: unknown target member cluster: 'unknown'")
				assert.Equal(t, reconcile.Result{Requeue: false}, res) // no requeue needed
				spacetest.AssertThatSpace(t, s.Namespace, s.Name, hostClient).
					Exists().
					HasStatusTargetCluster("unknown").
					HasFinalizer(toolchainv1alpha1.FinalizerName) // finalizer is still there, until the error above is fixed
				nstemplatetsettest.AssertThatNSTemplateSet(t, test.MemberOperatorNs, "oddity", member1.Client).
					DoesNotExist()
			})
		})

	})

	t.Run("failure", func(t *testing.T) {

		t.Run("error while getting NSTemplateSet on member cluster", func(t *testing.T) {
			// given
			s := spacetest.NewSpace("oddity",
				spacetest.WithTargetCluster("member-1"),
				spacetest.WithFinalizer(),
				spacetest.WithDeletionTimestamp())
			hostClient := test.NewFakeClient(t, s)
			member1Client := test.NewFakeClient(t)
			member1Client.MockGet = func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
				if _, ok := obj.(*toolchainv1alpha1.NSTemplateSet); ok {
					return fmt.Errorf("mock error")
				}
				return member1Client.Client.Get(ctx, key, obj)
			}
			member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
			member2 := NewMemberCluster(t, "member-2", corev1.ConditionTrue)
			ctrl := newReconciler(hostClient, member1, member2)

			// when
			res, err := ctrl.Reconcile(context.TODO(), requestFor(s))

			// then
			require.EqualError(t, err, "mock error")
			assert.False(t, res.Requeue)
			spacetest.AssertThatSpace(t, s.Namespace, s.Name, hostClient).
				Exists().
				HasStatusTargetCluster("member-1").
				HasConditions(spacetest.TerminatingFailed("mock error")).
				HasFinalizer(toolchainv1alpha1.FinalizerName)
		})
	})
}

func TestUpdate(t *testing.T) {

	// given
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)
	basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates)
	otherTier := tiertest.OtherTier()

	t.Run("update needed", func(t *testing.T) {
		// given that Space is promoted to `other` tier and corresponding NSTemplateSet is not up-to-date
		s := spacetest.NewSpace("oddity",
			spacetest.WithTierNameFor(otherTier), // assume that at this point, the `TemplateTierHash` label was already removed by the ChangeTierRequestController
			spacetest.WithTargetCluster("member-1"),
			spacetest.WithStatusTargetCluster("member-1"), // already provisioned on a target cluster
			spacetest.WithFinalizer())
		hostClient := test.NewFakeClient(t, s, basicTier, otherTier)
		nstmplSet := nstemplatetsettest.NewNSTemplateSet("oddity", nstemplatetsettest.WithReadyCondition())
		member1Client := test.NewFakeClient(t, nstmplSet)
		member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
		member2 := NewMemberCluster(t, "member-2", corev1.ConditionTrue)
		ctrl := newReconciler(hostClient, member1, member2)

		// when
		res, err := ctrl.Reconcile(context.TODO(), requestFor(s))

		// then
		require.NoError(t, err)
		assert.Equal(t, reconcile.Result{
			Requeue:      true,
			RequeueAfter: 3 * time.Second,
		}, res) // explicitly requeue while the NSTemplate update is triggered by its controller
		spacetest.AssertThatSpace(t, test.HostOperatorNs, "oddity", hostClient).
			Exists().
			HasTier(otherTier.Name).
			HasTargetCluster("member-1").
			HasStatusTargetCluster("member-1").
			HasConditions(spacetest.Provisioning()).
			DoesNotHaveLabel(tierutil.TemplateTierHashLabelKey(otherTier.Name)) // not set yet, since NSTemplateSet must be updated first
		nsTmplSet := nstemplatetsettest.AssertThatNSTemplateSet(t, test.MemberOperatorNs, "oddity", member1.Client).
			Exists().
			HasTierName(otherTier.Name).
			Get()

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
				HasTier(otherTier.Name).
				HasTargetCluster("member-1").
				HasStatusTargetCluster("member-1").
				HasConditions(spacetest.Provisioning()).
				DoesNotHaveLabel(tierutil.TemplateTierHashLabelKey(otherTier.Name))

			t.Run("done when NSTemplateSet is ready", func(t *testing.T) {
				// given another round of requeue without with NSTemplateSet now *ready*
				nsTmplSet.Status.Conditions = []toolchainv1alpha1.Condition{
					nstemplatetsettest.Provisioned(),
				}
				err := member1.Client.Update(context.TODO(), nsTmplSet)
				require.NoError(t, err)
				ctrl := newReconciler(hostClient, member1, member2)

				// when
				res, err := ctrl.Reconcile(context.TODO(), requestFor(s))

				// then
				require.NoError(t, err)
				assert.Equal(t, reconcile.Result{Requeue: false}, res) // no more requeue.
				spacetest.AssertThatSpace(t, test.HostOperatorNs, "oddity", hostClient).
					Exists().
					HasStatusTargetCluster("member-1").
					HasConditions(spacetest.Ready()).
					HasLabel(tierutil.TemplateTierHashLabelKey(otherTier.Name)).
					HasFinalizer(toolchainv1alpha1.FinalizerName)
			})
		})
	})

	t.Run("update not needed", func(t *testing.T) {
		// given that Space is promoted to `basic` tier and corresponding NSTemplateSet is already up-to-date and ready
		s := spacetest.NewSpace("oddity",
			// assume that at this point, the `TemplateTierHash` label was already removed by the ChangeTierRequestController
			spacetest.WithCondition(spacetest.Ready()),
			spacetest.WithTargetCluster("member-1"),
			spacetest.WithStatusTargetCluster("member-1"), // already provisioned on a target cluster
			spacetest.WithFinalizer())
		hostClient := test.NewFakeClient(t, s, basicTier, otherTier)
		nstmplSet := nstemplatetsettest.NewNSTemplateSet("oddity", nstemplatetsettest.WithReadyCondition())
		member1Client := test.NewFakeClient(t, nstmplSet)
		member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
		member2 := NewMemberCluster(t, "member-2", corev1.ConditionTrue)
		ctrl := newReconciler(hostClient, member1, member2)

		// when
		res, err := ctrl.Reconcile(context.TODO(), requestFor(s))

		// then
		require.NoError(t, err)
		assert.False(t, res.Requeue) // no need to requeue since the NSTemplate is already up-to-date
		spacetest.AssertThatSpace(t, test.HostOperatorNs, "oddity", hostClient).
			Exists().
			HasTier(basicTier.Name).
			HasTargetCluster("member-1").
			HasStatusTargetCluster("member-1").
			HasConditions(spacetest.Ready()).
			HasLabel(tierutil.TemplateTierHashLabelKey(basicTier.Name)) // label is immediately set since the NSTemplateSet was already up-to-date
	})

	t.Run("failures", func(t *testing.T) {

		t.Run("when updating space with new templatetierhash label", func(t *testing.T) {
			// given that Space is promoted to `basic` tier and corresponding NSTemplateSet is already up-to-date and ready
			s := spacetest.NewSpace("oddity",
				// assume that at this point, the `TemplateTierHash` label was already removed by the ChangeTierRequestController
				spacetest.WithCondition(spacetest.Ready()),
				spacetest.WithTargetCluster("member-1"),
				spacetest.WithStatusTargetCluster("member-1"), // already provisioned on a target cluster
				spacetest.WithFinalizer())
			hostClient := test.NewFakeClient(t, s, basicTier, otherTier)
			hostClient.MockUpdate = func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
				if _, ok := obj.(*toolchainv1alpha1.Space); ok {
					return fmt.Errorf("mock error")
				}
				return hostClient.Client.Update(ctx, obj, opts...)
			}
			nstmplSet := nstemplatetsettest.NewNSTemplateSet("oddity", nstemplatetsettest.WithReadyCondition())
			member1Client := test.NewFakeClient(t, nstmplSet)
			member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
			member2 := NewMemberCluster(t, "member-2", corev1.ConditionTrue)
			ctrl := newReconciler(hostClient, member1, member2)

			// when
			res, err := ctrl.Reconcile(context.TODO(), requestFor(s))

			// then
			require.EqualError(t, err, "mock error")
			assert.False(t, res.Requeue)
			spacetest.AssertThatSpace(t, s.Namespace, s.Name, hostClient).
				Exists().
				HasStatusTargetCluster("member-1").
				HasConditions(spacetest.ProvisioningFailed("mock error")).
				HasFinalizer(toolchainv1alpha1.FinalizerName)
		})

		t.Run("when NSTemplateSet updated failed", func(t *testing.T) {
			// given that Space is promoted to `other` tier and corresponding NSTemplateSet is not up-to-date
			s := spacetest.NewSpace("oddity",
				// assume that at this point, the `TemplateTierHash` label was already removed by the ChangeTierRequestController
				spacetest.WithTierNameFor(otherTier),
				spacetest.WithTargetCluster("member-1"),
				spacetest.WithStatusTargetCluster("member-1"), // already provisioned on a target cluster
				spacetest.WithFinalizer())
			hostClient := test.NewFakeClient(t, s, basicTier, otherTier)
			nstmplSet := nstemplatetsettest.NewNSTemplateSet("oddity", nstemplatetsettest.WithReadyCondition())
			member1Client := test.NewFakeClient(t, nstmplSet)
			member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
			member2 := NewMemberCluster(t, "member-2", corev1.ConditionTrue)
			ctrl := newReconciler(hostClient, member1, member2)

			// when
			res, err := ctrl.Reconcile(context.TODO(), requestFor(s))

			// then
			require.NoError(t, err)
			assert.Equal(t, reconcile.Result{
				Requeue:      true,
				RequeueAfter: 3 * time.Second,
			}, res) // explicitly requeue while the NSTemplate update is triggered by its controller
			spacetest.AssertThatSpace(t, test.HostOperatorNs, "oddity", hostClient).
				Exists().
				HasTier(otherTier.Name).
				HasTargetCluster("member-1").
				HasStatusTargetCluster("member-1").
				HasConditions(spacetest.Provisioning()).
				DoesNotHaveLabel(tierutil.TemplateTierHashLabelKey(otherTier.Name)) // not set yet, since NSTemplateSet must be updated first
			nsTmplSet := nstemplatetsettest.AssertThatNSTemplateSet(t, test.MemberOperatorNs, "oddity", member1.Client).
				Exists().
				HasTierName(otherTier.Name).
				Get()

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
					HasTier(otherTier.Name).
					HasTargetCluster("member-1").
					HasStatusTargetCluster("member-1").
					HasConditions(spacetest.Provisioning()).
					DoesNotHaveLabel(tierutil.TemplateTierHashLabelKey(otherTier.Name))

				t.Run("failed when namespace failed to provision", func(t *testing.T) {
					// given another round of requeue without with NSTemplateSet now *ready*
					nsTmplSet.Status.Conditions = []toolchainv1alpha1.Condition{
						nstemplatetsettest.UnableToProvisionNamespace("oops, something went wrong"),
					}
					err := member1.Client.Update(context.TODO(), nsTmplSet)
					require.NoError(t, err)
					ctrl := newReconciler(hostClient, member1, member2)

					// when
					res, err := ctrl.Reconcile(context.TODO(), requestFor(s))

					// then
					require.EqualError(t, err, "oops, something went wrong")
					assert.Equal(t, reconcile.Result{Requeue: false}, res) // no more requeue.
					spacetest.AssertThatSpace(t, test.HostOperatorNs, "oddity", hostClient).
						Exists().
						HasStatusTargetCluster("member-1").
						HasConditions(spacetest.ProvisioningFailed("oops, something went wrong")). // NSTemplateSet error message is copied into Space status
						HasLabel(tierutil.TemplateTierHashLabelKey(otherTier.Name)).
						HasFinalizer(toolchainv1alpha1.FinalizerName)
				})

				t.Run("failed when clusterresources failed to provision", func(t *testing.T) {
					// given another round of requeue without with NSTemplateSet now *ready*
					nsTmplSet.Status.Conditions = []toolchainv1alpha1.Condition{
						nstemplatetsettest.UnableToProvisionClusterResources("oops, something went wrong"),
					}
					err := member1.Client.Update(context.TODO(), nsTmplSet)
					require.NoError(t, err)
					ctrl := newReconciler(hostClient, member1, member2)

					// when
					res, err := ctrl.Reconcile(context.TODO(), requestFor(s))

					// then
					require.EqualError(t, err, "oops, something went wrong")
					assert.Equal(t, reconcile.Result{Requeue: false}, res) // no more requeue.
					spacetest.AssertThatSpace(t, test.HostOperatorNs, "oddity", hostClient).
						Exists().
						HasStatusTargetCluster("member-1").
						HasConditions(spacetest.ProvisioningFailed("oops, something went wrong")). // NSTemplateSet error message is copied into Space status
						HasLabel(tierutil.TemplateTierHashLabelKey(otherTier.Name)).
						HasFinalizer(toolchainv1alpha1.FinalizerName)
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

func requestFor(space *toolchainv1alpha1.Space) reconcile.Request {
	if space != nil {
		return reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: space.Namespace,
				Name:      space.Name,
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
