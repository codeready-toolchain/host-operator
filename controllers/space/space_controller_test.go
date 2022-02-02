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
		s := spacetest.NewSpace("oddity", spacetest.WithSpecTargetCluster("member-1"))
		hostClient := test.NewFakeClient(t, s, basicTier)
		member1 := NewMemberCluster(t, "member-1", corev1.ConditionTrue)
		member2 := NewMemberCluster(t, "member-2", corev1.ConditionTrue)
		ctrl := newReconciler(hostClient, member1, member2)

		// when
		res, err := ctrl.Reconcile(context.TODO(), requestFor(s))

		// then
		require.NoError(t, err)
		assert.True(t, res.Requeue) // requeue requested explictely when NSTemplateSet is created, even though watching the resource is enough to trigger a new reconcile loop
		spacetest.AssertThatSpace(t, test.HostOperatorNs, "oddity", hostClient).
			Exists().
			HasStatusTargetCluster("member-1").
			HasConditions(spacetest.Provisioning()).
			HasStateLabel("cluster-assigned").
			HasFinalizer()
		nsTmplSet := nstemplatetsettest.AssertThatNSTemplateSet(t, test.MemberOperatorNs, "oddity", member1.Client).
			Exists().
			HasTierName(basicTier.Name).
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
				HasStateLabel("cluster-assigned").
				HasFinalizer()
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
					HasStateLabel("cluster-assigned").
					HasFinalizer()
			})
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
			require.NoError(t, err) // the lack of target member cluster is valid, hence no error is returned
			assert.False(t, res.Requeue)
			spacetest.AssertThatSpace(t, s.Namespace, s.Name, hostClient).
				HasNoStatusTargetCluster().
				HasStateLabel("pending").
				HasConditions(spacetest.ProvisioningPending("unspecified target member cluster")) // the Space will remain in `ProvisioningPending` until a target member cluster is set.
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

		t.Run("unknown target member cluster", func(t *testing.T) {
			// given
			s := spacetest.NewSpace("oddity", spacetest.WithSpecTargetCluster("unknown"))
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
				HasStateLabel("cluster-assigned").
				HasConditions(spacetest.ProvisioningFailed("unknown target member cluster 'unknown'"))
		})

		t.Run("error while getting NSTemplateTier on host cluster", func(t *testing.T) {
			// given
			s := spacetest.NewSpace("oddity",
				spacetest.WithSpecTargetCluster("member-1"),
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
				HasSpecTargetCluster("member-1").
				HasStatusTargetCluster("member-1").
				HasConditions(spacetest.ProvisioningFailed("mock error"))
		})

		t.Run("error while getting NSTemplateSet on member cluster", func(t *testing.T) {
			// given
			s := spacetest.NewSpace("oddity",
				spacetest.WithSpecTargetCluster("member-1"),
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
				spacetest.WithSpecTargetCluster("member-1"),
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
				spacetest.WithSpecTargetCluster("member-1"),
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
				HasNoStatusTargetCluster(). // not set
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
					spacetest.WithDeletionTimestamp(), // deletion was requested
					spacetest.WithFinalizer(),
					spacetest.WithSpecTargetCluster("member-1"),
					spacetest.WithStatusTargetCluster("member-1"))
				hostClient := test.NewFakeClient(t, s, basicTier)
				nstmplSet := nstemplatetsettest.NewNSTemplateSet("oddity", nstemplatetsettest.WithReadyCondition())
				member1Client := test.NewFakeClient(t, nstmplSet)
				member1Client.MockDelete = func(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
					if nstmplSet, ok := obj.(*toolchainv1alpha1.NSTemplateSet); ok {
						now := metav1.Now()
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
					HasFinalizer(). // finalizer is still present while the NSTemplateSet is not fully deleted
					HasStatusTargetCluster("member-1").
					HasConditions(spacetest.Terminating())
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
						HasFinalizer().
						HasStatusTargetCluster("member-1").
						HasConditions(spacetest.Terminating())
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
							HasNoFinalizers(). // space resource can be deleted by the server now that the finalizer has been removed
							HasStatusTargetCluster("member-1").
							HasConditions(spacetest.Terminating())
					})
				})
			})

			t.Run("when using status target cluster", func(t *testing.T) {
				// given
				s := spacetest.NewSpace("oddity",
					spacetest.WithoutSpecTargetCluster(),          // targetCluster is not specified in spec ...
					spacetest.WithStatusTargetCluster("member-1"), // ... but is available in status
					spacetest.WithFinalizer(),
					spacetest.WithDeletionTimestamp())
				hostClient := test.NewFakeClient(t, s, basicTier)
				nstmplSet := nstemplatetsettest.NewNSTemplateSet("oddity", nstemplatetsettest.WithReadyCondition())
				member1Client := test.NewFakeClient(t, nstmplSet)
				member1Client.MockDelete = mockDeleteNSTemplateSet(member1Client.Client)
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
					HasFinalizer() // finalizer is still present while the NSTemplateSet is not fully deleted
				nstemplatetsettest.AssertThatNSTemplateSet(t, test.MemberOperatorNs, "oddity", member1.Client).
					Exists().
					HasDeletionTimestamp().
					HasConditions(nstemplatetsettest.Terminating()).
					Get()
				// stop the test here: it verified that the NSTemlateSet deletion was triggered (the rest is already covered above)
			})

			t.Run("without spec and status target cluster", func(t *testing.T) {
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
					spacetest.WithoutSpecTargetCluster(),
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
					spacetest.WithSpecTargetCluster("member-3"),
					spacetest.WithStatusTargetCluster("member-3"), // assume that Space was provisioned on a cluster which is now missing
					spacetest.WithFinalizer(),
					spacetest.WithDeletionTimestamp(),
					spacetest.WithCondition(spacetest.ProvisioningFailed("unknown target member cluster 'member-3'")),
				)
				hostClient := test.NewFakeClient(t, s, basicTier)
				member1 := NewMemberCluster(t, "member-1", corev1.ConditionTrue)
				member2 := NewMemberCluster(t, "member-2", corev1.ConditionTrue)
				ctrl := newReconciler(hostClient, member1, member2)

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
			})
		})

	})

	t.Run("failure", func(t *testing.T) {

		t.Run("error while getting NSTemplateSet on member cluster", func(t *testing.T) {
			// given
			s := spacetest.NewSpace("oddity",
				spacetest.WithDeletionTimestamp(), // deletion was requested
				spacetest.WithSpecTargetCluster("member-1"),
				spacetest.WithStatusTargetCluster("member-1"),
				spacetest.WithFinalizer(),
			)
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
				HasFinalizer().
				HasSpecTargetCluster("member-1").
				HasStatusTargetCluster("member-1").
				HasConditions(spacetest.TerminatingFailed("mock error"))
		})

		t.Run("error when NSTemplateSet is stuck in deletion", func(t *testing.T) {
			// given
			s := spacetest.NewSpace("oddity",
				spacetest.WithDeletionTimestamp(), // deletion was requested
				spacetest.WithSpecTargetCluster("member-1"),
				spacetest.WithStatusTargetCluster("member-1"),
				spacetest.WithFinalizer(),
			)
			hostClient := test.NewFakeClient(t, s)
			nstmplSet := nstemplatetsettest.NewNSTemplateSet("oddity", nstemplatetsettest.WithDeletionTimestamp(time.Now().Add(-2*time.Minute)))
			member1Client := test.NewFakeClient(t, nstmplSet)
			member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
			member2 := NewMemberCluster(t, "member-2", corev1.ConditionTrue)
			ctrl := newReconciler(hostClient, member1, member2)

			// when
			res, err := ctrl.Reconcile(context.TODO(), requestFor(s))

			// then
			require.EqualError(t, err, "NSTemplateSet deletion has not completed in over 1 minute")
			assert.False(t, res.Requeue)
			spacetest.AssertThatSpace(t, s.Namespace, s.Name, hostClient).
				Exists().
				HasFinalizer().
				HasSpecTargetCluster("member-1").
				HasStatusTargetCluster("member-1").
				HasConditions(spacetest.TerminatingFailed("NSTemplateSet deletion has not completed in over 1 minute"))
		})
	})
}

func TestUpdateSpaceTier(t *testing.T) {

	// given
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)
	basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates)
	otherTier := tiertest.OtherTier()

	t.Run("tier promotion (update needed due to different tier)", func(t *testing.T) {
		// given that Space is promoted from `basic` to `other` tier and corresponding NSTemplateSet is not up-to-date
		s := spacetest.NewSpace("oddity",
			spacetest.WithTierNameFor(otherTier), // assume that at this point, the `TemplateTierHash` label was already removed by the ChangeTierRequestController
			spacetest.WithSpecTargetCluster("member-1"),
			spacetest.WithStatusTargetCluster("member-1"), // already provisioned on a target cluster
			spacetest.WithFinalizer(),
			spacetest.WithCondition(spacetest.Ready()))
		hostClient := test.NewFakeClient(t, s, basicTier, otherTier)
		nstmplSet := nstemplatetsettest.NewNSTemplateSet("oddity", nstemplatetsettest.WithReferencesFor(basicTier), nstemplatetsettest.WithReadyCondition())
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
			RequeueAfter: 1 * time.Second,
		}, res) // explicitly requeue while the NSTemplate update is triggered by its controller
		spacetest.AssertThatSpace(t, test.HostOperatorNs, "oddity", hostClient).
			Exists().
			HasTier(otherTier.Name).
			HasSpecTargetCluster("member-1").
			HasStatusTargetCluster("member-1").
			HasConditions(spacetest.Updating()).
			DoesNotHaveLabel(tierutil.TemplateTierHashLabelKey(otherTier.Name)) // not set yet, since NSTemplateSet must be updated first
		nsTmplSet := nstemplatetsettest.AssertThatNSTemplateSet(t, test.MemberOperatorNs, "oddity", member1.Client).
			Exists().
			HasTierName(otherTier.Name).
			HasClusterResourcesTemplateRef("other-clusterresources-123456a").
			HasNamespaceTemplateRefs("other-code-123456a", "other-dev-123456a", "other-stage-123456a").
			Get()

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
				DoesNotHaveLabel(tierutil.TemplateTierHashLabelKey(otherTier.Name))

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
					DoesNotHaveLabel(tierutil.TemplateTierHashLabelKey(otherTier.Name)). // not set yet
					HasFinalizer().
					Get()

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
						HasMatchingTierLabelForTier(otherTier).
						HasFinalizer()

				})
			})
		})
	})

	t.Run("tier update for single space (same tier but updated references)", func(t *testing.T) {
		// get an older basic tier (with outdated references) that the nstemplateset can be referenced to for setup
		olderBasicTier := tiertest.BasicTier(t, tiertest.PreviousBasicTemplates)
		// given that Space is set to the same tier that has been updated and the corresponding NSTemplateSet is not up-to-date
		s := spacetest.NewSpace("oddity",
			spacetest.WithTierNameAndHashLabelFor(olderBasicTier),
			spacetest.WithSpecTargetCluster("member-1"),
			spacetest.WithStatusTargetCluster("member-1"), // already provisioned on a target cluster
			spacetest.WithFinalizer(),
			spacetest.WithCondition(spacetest.Ready()))
		hostClient := test.NewFakeClient(t, s, basicTier)
		nsTmplSet := nstemplatetsettest.NewNSTemplateSet("oddity", nstemplatetsettest.WithReferencesFor(olderBasicTier), nstemplatetsettest.WithReadyCondition()) // NSTemplateSet has references to old basic tier
		member1Client := test.NewFakeClient(t, nsTmplSet)
		member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
		member2 := NewMemberCluster(t, "member-2", corev1.ConditionTrue)
		ctrl := newReconciler(hostClient, member1, member2)

		// when
		res, err := ctrl.Reconcile(context.TODO(), requestFor(s))

		// then
		require.NoError(t, err)
		assert.Equal(t, reconcile.Result{
			Requeue:      true,
			RequeueAfter: 1 * time.Second,
		}, res) // explicitly requeue while the NSTemplate update is triggered by its controller
		s = spacetest.AssertThatSpace(t, test.HostOperatorNs, "oddity", hostClient).
			Exists().
			HasTier(basicTier.Name).
			HasSpecTargetCluster("member-1").
			HasStatusTargetCluster("member-1").
			HasConditions(spacetest.Updating()).
			HasMatchingTierLabelForTier(olderBasicTier).
			Get()
		nsTmplSet = nstemplatetsettest.AssertThatNSTemplateSet(t, test.MemberOperatorNs, "oddity", member1.Client).
			Exists().
			HasTierName(basicTier.Name).
			HasClusterResourcesTemplateRef("basic-clusterresources-123456new").
			HasNamespaceTemplateRefs("basic-code-123456new", "basic-dev-123456new", "basic-stage-123456new").
			Get()
		require.True(t, tierutil.TierHashMatches(basicTier, nsTmplSet.Spec))

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
				HasMatchingTierLabelForTier(basicTier). // label updated
				HasFinalizer()
		})
	})

	t.Run("tier update for 3 spaces (same tier but updated references)", func(t *testing.T) {
		// get an older basic tier (with outdated references) that the nstemplateset can be referenced to for setup
		olderBasicTier := tiertest.BasicTier(t, tiertest.PreviousBasicTemplates)
		// given that Space is set to the same tier that has been updated and the corresponding NSTemplateSet is not up-to-date
		s1 := spacetest.NewSpace("oddity1",
			spacetest.WithTierNameAndHashLabelFor(olderBasicTier),
			spacetest.WithSpecTargetCluster("member-1"),
			spacetest.WithStatusTargetCluster("member-1"), // already provisioned on a target cluster
			spacetest.WithFinalizer(),
			spacetest.WithCondition(spacetest.Ready()))
		nsTmplSet1 := nstemplatetsettest.NewNSTemplateSet(s1.Name, nstemplatetsettest.WithReferencesFor(olderBasicTier), nstemplatetsettest.WithReadyCondition()) // NSTemplateSet has references to old basic tier
		s2 := spacetest.NewSpace("oddity2",
			spacetest.WithTierNameAndHashLabelFor(olderBasicTier),
			spacetest.WithSpecTargetCluster("member-1"),
			spacetest.WithStatusTargetCluster("member-1"), // already provisioned on a target cluster
			spacetest.WithFinalizer(),
			spacetest.WithCondition(spacetest.Ready()))
		nsTmplSet2 := nstemplatetsettest.NewNSTemplateSet(s2.Name, nstemplatetsettest.WithReferencesFor(olderBasicTier), nstemplatetsettest.WithReadyCondition()) // NSTemplateSet has references to old basic tier
		s3 := spacetest.NewSpace("oddity3",
			spacetest.WithTierNameAndHashLabelFor(olderBasicTier),
			spacetest.WithSpecTargetCluster("member-1"),
			spacetest.WithStatusTargetCluster("member-1"), // already provisioned on a target cluster
			spacetest.WithFinalizer(),
			spacetest.WithCondition(spacetest.Ready()))
		nsTmplSet3 := nstemplatetsettest.NewNSTemplateSet(s3.Name, nstemplatetsettest.WithReferencesFor(olderBasicTier), nstemplatetsettest.WithReadyCondition()) // NSTemplateSet has references to old basic tier
		hostClient := test.NewFakeClient(t, s1, s2, s3, basicTier)
		member1Client := test.NewFakeClient(t, nsTmplSet1, nsTmplSet2, nsTmplSet3)
		member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
		member2 := NewMemberCluster(t, "member-2", corev1.ConditionTrue)
		ctrl := newReconciler(hostClient, member1, member2)

		// when reconciling space `s1`
		res, err := ctrl.Reconcile(context.TODO(), requestFor(s1))

		// then
		require.NoError(t, err)
		assert.Equal(t, reconcile.Result{
			Requeue:      true,
			RequeueAfter: 1 * time.Second, // wait 1s for NSTemplateSet update to begin
		}, res) // explicitly requeue while the NSTemplate update is triggered by its controller
		spacetest.AssertThatSpace(t, test.HostOperatorNs, s1.Name, hostClient).
			Exists().
			HasTier(basicTier.Name).
			HasSpecTargetCluster("member-1").
			HasStatusTargetCluster("member-1").
			HasConditions(spacetest.Updating()).
			HasMatchingTierLabelForTier(olderBasicTier)
		nsTmplSet1 = nstemplatetsettest.AssertThatNSTemplateSet(t, test.MemberOperatorNs, nsTmplSet1.Name, member1.Client).
			Exists().
			HasTierName(basicTier.Name).
			HasClusterResourcesTemplateRef("basic-clusterresources-123456new").
			HasNamespaceTemplateRefs("basic-code-123456new", "basic-dev-123456new", "basic-stage-123456new"). // updated
			Get()
		require.True(t, tierutil.TierHashMatches(basicTier, nsTmplSet1.Spec))

		// when reconciling space `s2`
		res, err = ctrl.Reconcile(context.TODO(), requestFor(s2))

		// then
		require.NoError(t, err)
		assert.True(t, res.Requeue)
		assert.Greater(t, res.RequeueAfter, 1*time.Second) // explicitly requeued 2 seconds after `now`
		assert.LessOrEqual(t, res.RequeueAfter, 2*time.Second)
		spacetest.AssertThatSpace(t, test.HostOperatorNs, s2.Name, hostClient).
			Exists().
			HasTier(basicTier.Name).
			HasSpecTargetCluster("member-1").
			HasStatusTargetCluster("member-1").
			HasConditions(spacetest.Ready()).           // unchanged
			HasMatchingTierLabelForTier(olderBasicTier) // unchanged
		nsTmplSet2 = nstemplatetsettest.AssertThatNSTemplateSet(t, test.MemberOperatorNs, nsTmplSet2.Name, member1.Client).
			Exists().
			HasTierName(basicTier.Name).
			HasClusterResourcesTemplateRef("basic-clusterresources-123456old").
			HasNamespaceTemplateRefs("basic-code-123456old", "basic-dev-123456old", "basic-stage-123456old"). // not updated yet
			Get()
		require.True(t, tierutil.TierHashMatches(olderBasicTier, nsTmplSet2.Spec)) // unchanged

		// when reconciling space `s3`
		res, err = ctrl.Reconcile(context.TODO(), requestFor(s3))

		// then
		require.NoError(t, err)
		assert.True(t, res.Requeue)
		assert.Greater(t, res.RequeueAfter, 3*time.Second) // explicitly requeued 2 seconds after s2, i.e, almost 4s after `now`
		assert.LessOrEqual(t, res.RequeueAfter, 4*time.Second)
		spacetest.AssertThatSpace(t, test.HostOperatorNs, s3.Name, hostClient).
			Exists().
			HasTier(basicTier.Name).
			HasSpecTargetCluster("member-1").
			HasStatusTargetCluster("member-1").
			HasConditions(spacetest.Ready()).           // unchanged
			HasMatchingTierLabelForTier(olderBasicTier) // unchanged
		nsTmplSet2 = nstemplatetsettest.AssertThatNSTemplateSet(t, test.MemberOperatorNs, nsTmplSet3.Name, member1.Client).
			Exists().
			HasTierName(basicTier.Name).
			HasClusterResourcesTemplateRef("basic-clusterresources-123456old").
			HasNamespaceTemplateRefs("basic-code-123456old", "basic-dev-123456old", "basic-stage-123456old"). // not updated yet
			Get()
		require.True(t, tierutil.TierHashMatches(olderBasicTier, nsTmplSet2.Spec)) // unchanged

	})

	t.Run("update not needed", func(t *testing.T) {
		// given that Space is promoted to `basic` tier and corresponding NSTemplateSet is already up-to-date and ready
		s := spacetest.NewSpace("oddity",
			// assume that at this point, the `TemplateTierHash` label was already removed by the ChangeTierRequestController
			spacetest.WithTierNameFor(basicTier),
			spacetest.WithCondition(spacetest.Ready()),
			spacetest.WithSpecTargetCluster("member-1"),
			spacetest.WithStatusTargetCluster("member-1"), // already provisioned on a target cluster
			spacetest.WithFinalizer())
		hostClient := test.NewFakeClient(t, s, basicTier)
		nstmplSet := nstemplatetsettest.NewNSTemplateSet("oddity", nstemplatetsettest.WithReferencesFor(basicTier), nstemplatetsettest.WithReadyCondition())
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
			HasSpecTargetCluster("member-1").
			HasStatusTargetCluster("member-1").
			HasConditions(spacetest.Ready()).
			HasMatchingTierLabelForTier(basicTier) // label is immediately set since the NSTemplateSet was already up-to-date
	})

	t.Run("failures", func(t *testing.T) {

		t.Run("when updating space with new templatetierhash label", func(t *testing.T) {
			// given that Space is promoted to `basic` tier and corresponding NSTemplateSet is already up-to-date and ready
			s := spacetest.NewSpace("oddity",
				// assume that at this point, the `TemplateTierHash` label was already removed by the ChangeTierRequestController
				spacetest.WithCondition(spacetest.Ready()),
				spacetest.WithSpecTargetCluster("member-1"),
				spacetest.WithStatusTargetCluster("member-1"), // already provisioned on a target cluster
				spacetest.WithFinalizer())
			hostClient := test.NewFakeClient(t, s, basicTier, otherTier)
			hostClient.MockUpdate = func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
				if _, ok := obj.(*toolchainv1alpha1.Space); ok && obj.GetLabels()[tierutil.TemplateTierHashLabelKey(basicTier.Name)] != "" {
					return fmt.Errorf("mock error")
				}
				return hostClient.Client.Update(ctx, obj, opts...)
			}
			nstmplSet := nstemplatetsettest.NewNSTemplateSet("oddity", nstemplatetsettest.WithReferencesFor(basicTier), nstemplatetsettest.WithReadyCondition())
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
				HasFinalizer()
		})

		t.Run("when NSTemplateSet update failed", func(t *testing.T) {
			// given that Space is promoted to `other` tier and corresponding NSTemplateSet is not up-to-date
			s := spacetest.NewSpace("oddity",
				// assume that at this point, the `TemplateTierHash` label was already removed by the ChangeTierRequestController
				spacetest.WithTierNameFor(otherTier),
				spacetest.WithSpecTargetCluster("member-1"),
				spacetest.WithStatusTargetCluster("member-1"), // already provisioned on a target cluster
				spacetest.WithFinalizer(),
				spacetest.WithCondition(spacetest.Ready()))
			hostClient := test.NewFakeClient(t, s, basicTier, otherTier)
			nstmplSet := nstemplatetsettest.NewNSTemplateSet("oddity", nstemplatetsettest.WithReferencesFor(basicTier), nstemplatetsettest.WithReadyCondition())
			member1Client := test.NewFakeClient(t, nstmplSet)
			member1Client.MockUpdate = mockUpdateNSTemplateSetFail(member1Client.Client)
			member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
			member2 := NewMemberCluster(t, "member-2", corev1.ConditionTrue)
			ctrl := newReconciler(hostClient, member1, member2)

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
				DoesNotHaveLabel(tierutil.TemplateTierHashLabelKey(otherTier.Name)) // not set yet, since NSTemplateSet must be updated first
			nstemplatetsettest.AssertThatNSTemplateSet(t, test.MemberOperatorNs, "oddity", member1.Client).
				Exists().
				HasTierName(basicTier.Name).
				HasClusterResourcesTemplateRef("basic-clusterresources-123456new").
				HasNamespaceTemplateRefs("basic-code-123456new", "basic-dev-123456new", "basic-stage-123456new")
		})
	})
}

func TestRetargetSpace(t *testing.T) {

	// given
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)
	basicTier := tiertest.BasicTier(t, tiertest.CurrentBasicTemplates)

	t.Run("to empty target cluster", func(t *testing.T) {
		// given
		s := spacetest.NewSpace("oddity",
			spacetest.WithFinalizer(),
			spacetest.WithoutSpecTargetCluster(), // assume that field was reset by a client (admin, appstudio console, etc.)
			spacetest.WithStatusTargetCluster("member-1"))
		hostClient := test.NewFakeClient(t, s, basicTier)
		nstmplSet := nstemplatetsettest.NewNSTemplateSet("oddity", nstemplatetsettest.WithReadyCondition())
		member1Client := test.NewFakeClient(t, nstmplSet)
		member1Client.MockDelete = mockDeleteNSTemplateSet(member1Client.Client)
		member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
		member2 := NewMemberCluster(t, "member-2", corev1.ConditionTrue)
		ctrl := newReconciler(hostClient, member1, member2)

		// when
		res, err := ctrl.Reconcile(context.TODO(), requestFor(s))

		// then
		require.NoError(t, err)
		assert.False(t, res.Requeue)
		spacetest.AssertThatSpace(t, s.Namespace, s.Name, hostClient).
			HasFinalizer().
			HasNoSpecTargetCluster().
			HasConditions(spacetest.Retargeting()).
			HasStatusTargetCluster("member-1") // not reset yet

		t.Run("status target cluster is not reset while NSTemplateSet is still being deleted on member-1", func(t *testing.T) {
			// once NSTemplateSet resource is fully deleted, the SpaceController is triggered again
			// and it can create the NSTemplateSet on member-2 cluster

			// when
			res, err := ctrl.Reconcile(context.TODO(), requestFor(s))
			// then
			require.NoError(t, err)
			assert.False(t, res.Requeue)
			spacetest.AssertThatSpace(t, s.Namespace, s.Name, hostClient).
				HasFinalizer().
				HasNoSpecTargetCluster().
				HasConditions(spacetest.Retargeting()).
				HasStatusTargetCluster("member-1") // not updated yet
			nstemplatetsettest.AssertThatNSTemplateSet(t, test.MemberOperatorNs, "oddity", member1.Client).
				HasDeletionTimestamp()

			t.Run("status target cluster reset when NSTemplateSet is deleted", func(t *testing.T) {
				// once NSTemplateSet resource is fully deleted, the SpaceController is triggered again
				err := member1Client.Client.Delete(context.TODO(), nstmplSet)
				require.NoError(t, err)
				// when
				res, err := ctrl.Reconcile(context.TODO(), requestFor(s))
				// then
				require.NoError(t, err)
				assert.False(t, res.Requeue)
				spacetest.AssertThatSpace(t, s.Namespace, s.Name, hostClient).
					HasFinalizer().
					HasNoSpecTargetCluster().
					HasConditions(spacetest.ProvisioningPending("unspecified target member cluster")).
					HasNoStatusTargetCluster() // reset
			})
		})
	})

	t.Run("to another target cluster", func(t *testing.T) {
		// given
		s := spacetest.NewSpace("oddity",
			spacetest.WithFinalizer(),
			spacetest.WithSpecTargetCluster("member-2"), // assume that field was changed by a client (admin, appstudio console, etc.)
			spacetest.WithStatusTargetCluster("member-1"))
		hostClient := test.NewFakeClient(t, s, basicTier)
		nstmplSet := nstemplatetsettest.NewNSTemplateSet("oddity", nstemplatetsettest.WithReadyCondition())
		member1Client := test.NewFakeClient(t, nstmplSet)
		member1Client.MockDelete = mockDeleteNSTemplateSet(member1Client.Client)
		member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
		member2Client := test.NewFakeClient(t)
		member2 := NewMemberClusterWithClient(member2Client, "member-2", corev1.ConditionTrue)
		ctrl := newReconciler(hostClient, member1, member2)

		// when
		res, err := ctrl.Reconcile(context.TODO(), requestFor(s))

		// then
		require.NoError(t, err)
		assert.False(t, res.Requeue)
		spacetest.AssertThatSpace(t, s.Namespace, s.Name, hostClient).
			HasFinalizer().
			HasSpecTargetCluster("member-2").
			HasConditions(spacetest.Retargeting()).
			HasStatusTargetCluster("member-1") // not reset yet

		t.Run("status target cluster is not reset while NSTemplateSet is still being deleted on member-1", func(t *testing.T) {
			// once NSTemplateSet resource is fully deleted, the SpaceController is triggered again
			// and it can create the NSTemplateSet on member-2 cluster

			// when
			res, err := ctrl.Reconcile(context.TODO(), requestFor(s))
			// then
			require.NoError(t, err)
			assert.False(t, res.Requeue)
			spacetest.AssertThatSpace(t, s.Namespace, s.Name, hostClient).
				HasFinalizer().
				HasSpecTargetCluster("member-2").
				HasConditions(spacetest.Retargeting()).
				HasStatusTargetCluster("member-1") // not updated yet
			nstemplatetsettest.AssertThatNSTemplateSet(t, test.MemberOperatorNs, "oddity", member1.Client).
				HasDeletionTimestamp()
			nstemplatetsettest.AssertThatNSTemplateSet(t, test.MemberOperatorNs, "oddity", member2.Client).
				DoesNotExist()

			t.Run("status target cluster is reset when NSTemplateSet is deleted on member-1", func(t *testing.T) {
				// once NSTemplateSet resource is fully deleted, the SpaceController is triggered again
				// and it can create the NSTemplateSet on member-2 cluster
				err := member1Client.Client.Delete(context.TODO(), nstmplSet) // bypass the mock func
				require.NoError(t, err)
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
					HasTierName(basicTier.Name)
			})
		})
	})

	t.Run("failures", func(t *testing.T) {

		t.Run("unable to delete NSTemplateSet", func(t *testing.T) {
			// given
			s := spacetest.NewSpace("oddity",
				spacetest.WithFinalizer(),
				spacetest.WithSpecTargetCluster("member-2"), // assume that field was changed by a client (admin, appstudio console, etc.)
				spacetest.WithStatusTargetCluster("member-1"))
			hostClient := test.NewFakeClient(t, s, basicTier)
			nstmplSet := nstemplatetsettest.NewNSTemplateSet("oddity", nstemplatetsettest.WithReadyCondition())
			member1Client := test.NewFakeClient(t, nstmplSet)
			member1Client.MockDelete = mockDeleteNSTemplateSetFail(member1Client.Client)
			member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
			member2Client := test.NewFakeClient(t)
			member2 := NewMemberClusterWithClient(member2Client, "member-2", corev1.ConditionTrue)
			ctrl := newReconciler(hostClient, member1, member2)

			// when
			res, err := ctrl.Reconcile(context.TODO(), requestFor(s))

			// then
			require.EqualError(t, err, "mock error")
			assert.False(t, res.Requeue)
			spacetest.AssertThatSpace(t, s.Namespace, s.Name, hostClient).
				HasFinalizer().
				HasSpecTargetCluster("member-2").
				HasConditions(spacetest.RetargetingFailed("mock error")).
				HasStatusTargetCluster("member-1") // NOT updated
		})

		t.Run("unable to update status", func(t *testing.T) {
			// given
			s := spacetest.NewSpace("oddity",
				spacetest.WithFinalizer(),
				spacetest.WithSpecTargetCluster("member-2"), // assume that field was changed by a client (admin, appstudio console, etc.)
				spacetest.WithStatusTargetCluster("member-1"))
			hostClient := test.NewFakeClient(t, s, basicTier)
			nstmplSet := nstemplatetsettest.NewNSTemplateSet("oddity", nstemplatetsettest.WithReadyCondition())
			hostClient.MockStatusUpdate = mockUpdateSpaceStatusFail(hostClient.Client)
			member1Client := test.NewFakeClient(t, nstmplSet)
			member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue)
			member2Client := test.NewFakeClient(t)
			member2 := NewMemberClusterWithClient(member2Client, "member-2", corev1.ConditionTrue)
			ctrl := newReconciler(hostClient, member1, member2)

			// when
			res, err := ctrl.Reconcile(context.TODO(), requestFor(s))

			// then
			require.EqualError(t, err, "mock error")
			assert.False(t, res.Requeue)
			spacetest.AssertThatSpace(t, s.Namespace, s.Name, hostClient).
				HasFinalizer().
				HasSpecTargetCluster("member-2").
				HasNoConditions().
				HasStatusTargetCluster("member-1") // NOT updated
		})

	})
}

func mockDeleteNSTemplateSet(cl client.Client) func(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	return func(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
		if nstmplSet, ok := obj.(*toolchainv1alpha1.NSTemplateSet); ok {
			now := metav1.Now()
			nstmplSet.DeletionTimestamp = &now
			nstmplSet.Status.Conditions = []toolchainv1alpha1.Condition{
				nstemplatetsettest.Terminating(),
			}
			// instead of deleting the resource in the FakeClient,
			// we update it with a `DeletionTimestamp`
			return cl.Update(ctx, obj)
		}
		return cl.Delete(ctx, obj, opts...)
	}
}

func mockDeleteNSTemplateSetFail(cl client.Client) func(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	return func(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
		if _, ok := obj.(*toolchainv1alpha1.NSTemplateSet); ok {
			return fmt.Errorf("mock error")
		}
		return cl.Delete(ctx, obj, opts...)
	}
}

func mockUpdateSpaceStatusFail(cl client.Client) func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	return func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
		if _, ok := obj.(*toolchainv1alpha1.Space); ok {
			return fmt.Errorf("mock error")
		}
		return cl.Status().Update(ctx, obj, opts...)
	}
}

func mockUpdateNSTemplateSetFail(cl client.Client) func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	return func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
		if _, ok := obj.(*toolchainv1alpha1.NSTemplateSet); ok {
			return fmt.Errorf("mock error")
		}
		return cl.Update(ctx, obj, opts...)
	}
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
