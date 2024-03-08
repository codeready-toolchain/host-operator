package spacebindingcleanup

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	"github.com/codeready-toolchain/host-operator/pkg/cluster"
	. "github.com/codeready-toolchain/host-operator/test"
	sb "github.com/codeready-toolchain/host-operator/test/spacebinding"
	commoncluster "github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	commonconfig "github.com/codeready-toolchain/toolchain-common/pkg/configuration"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	testconfig "github.com/codeready-toolchain/toolchain-common/pkg/test/config"
	"github.com/codeready-toolchain/toolchain-common/pkg/test/masteruserrecord"
	spacetest "github.com/codeready-toolchain/toolchain-common/pkg/test/space"
	sbrtestcommon "github.com/codeready-toolchain/toolchain-common/pkg/test/spacebindingrequest"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/stretchr/testify/require"
	"k8s.io/client-go/kubernetes/scheme"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestDeleteSpaceBinding(t *testing.T) {
	// given
	sbLaraRedhatAdmin := sb.NewSpaceBinding("lara", "redhat", "admin", "signupA")
	sbJoeRedhatView := sb.NewSpaceBinding("joe", "redhat", "view", "signupB")
	sbLaraIbmEdit := sb.NewSpaceBinding("lara", "ibm", "edit", "signupC")

	redhatSpace := spacetest.NewSpace(test.HostOperatorNs, "redhat")
	ibmSpace := spacetest.NewSpace(test.HostOperatorNs, "ibm")

	laraMur := masteruserrecord.NewMasterUserRecord(t, "lara")
	joeMur := masteruserrecord.NewMasterUserRecord(t, "joe")

	t.Run("lara-redhat SpaceBinding removed when redhat space is missing", func(t *testing.T) {

		fakeClient := test.NewFakeClient(t, sbLaraRedhatAdmin, sbJoeRedhatView, sbLaraIbmEdit, laraMur, joeMur, ibmSpace)
		reconciler := prepareReconciler(t, fakeClient)

		// when
		res, err := reconciler.Reconcile(context.TODO(), requestFor(sbLaraRedhatAdmin))

		// then
		require.Equal(t, res.RequeueAfter, time.Duration(0)) // no requeue
		require.NoError(t, err)
		sb.AssertThatSpaceBinding(t, test.HostOperatorNs, "lara", "redhat", fakeClient).DoesNotExist()
		sb.AssertThatSpaceBinding(t, test.HostOperatorNs, "joe", "redhat", fakeClient).Exists()
		sb.AssertThatSpaceBinding(t, test.HostOperatorNs, "lara", "ibm", fakeClient).Exists()
	})

	t.Run("joe-redhat SpaceBinding removed when joe MUR is missing", func(t *testing.T) {

		fakeClient := test.NewFakeClient(t, sbJoeRedhatView, sbLaraRedhatAdmin, sbLaraIbmEdit, laraMur, ibmSpace, redhatSpace)
		reconciler := prepareReconciler(t, fakeClient)

		// when
		res, err := reconciler.Reconcile(context.TODO(), requestFor(sbJoeRedhatView))

		// then
		require.Equal(t, res.RequeueAfter, time.Duration(0)) // no requeue
		require.NoError(t, err)
		sb.AssertThatSpaceBinding(t, test.HostOperatorNs, "lara", "redhat", fakeClient).Exists()
		sb.AssertThatSpaceBinding(t, test.HostOperatorNs, "joe", "redhat", fakeClient).DoesNotExist()
		sb.AssertThatSpaceBinding(t, test.HostOperatorNs, "lara", "ibm", fakeClient).Exists()
	})

	t.Run("lara-redhat SpaceBinding is being deleted, so no action needed", func(t *testing.T) {
		sbLaraRedhatAdmin := sbLaraRedhatAdmin.DeepCopy()
		now := metav1.Now()
		sbLaraRedhatAdmin.DeletionTimestamp = &now
		fakeClient := test.NewFakeClient(t, sbLaraRedhatAdmin, sbJoeRedhatView, sbLaraIbmEdit, laraMur, joeMur, ibmSpace)
		reconciler := prepareReconciler(t, fakeClient)

		// when
		res, err := reconciler.Reconcile(context.TODO(), requestFor(sbLaraRedhatAdmin))

		// then
		require.Equal(t, res.RequeueAfter, time.Duration(0)) // no requeue
		require.NoError(t, err)
		sb.AssertThatSpaceBinding(t, test.HostOperatorNs, "lara", "redhat", fakeClient).Exists()
		sb.AssertThatSpaceBinding(t, test.HostOperatorNs, "joe", "redhat", fakeClient).Exists()
		sb.AssertThatSpaceBinding(t, test.HostOperatorNs, "lara", "ibm", fakeClient).Exists()
	})

	t.Run("no SpaceBinding removed when MUR and Space are present", func(t *testing.T) {

		fakeClient := test.NewFakeClient(t, sbLaraRedhatAdmin, sbJoeRedhatView, sbLaraIbmEdit, laraMur, joeMur, ibmSpace, redhatSpace)
		reconciler := prepareReconciler(t, fakeClient)

		// when
		res, err := reconciler.Reconcile(context.TODO(), requestFor(sbLaraRedhatAdmin))

		// then
		require.Equal(t, res.RequeueAfter, time.Duration(0)) // no requeue
		require.NoError(t, err)
		sb.AssertThatSpaceBinding(t, test.HostOperatorNs, "lara", "redhat", fakeClient).Exists()
		sb.AssertThatSpaceBinding(t, test.HostOperatorNs, "joe", "redhat", fakeClient).Exists()
		sb.AssertThatSpaceBinding(t, test.HostOperatorNs, "lara", "ibm", fakeClient).Exists()
	})

	t.Run("fails while getting the bound resource", func(t *testing.T) {

		for _, boundResourceName := range []string{"lara", "redhat"} {
			fakeClient := test.NewFakeClient(t, sbLaraRedhatAdmin, redhatSpace, laraMur)
			reconciler := prepareReconciler(t, fakeClient)
			fakeClient.MockGet = func(ctx context.Context, key runtimeclient.ObjectKey, obj runtimeclient.Object, opts ...runtimeclient.GetOption) error {
				if key.Name == boundResourceName {
					return fmt.Errorf("some error")
				}
				return fakeClient.Client.Get(ctx, key, obj, opts...)
			}

			// when
			_, err := reconciler.Reconcile(context.TODO(), requestFor(sbLaraRedhatAdmin))

			// then
			require.Error(t, err)
			sb.AssertThatSpaceBinding(t, test.HostOperatorNs, "lara", "redhat", fakeClient).Exists()
		}
	})

	t.Run("fails while deleting the SpaceBinding", func(t *testing.T) {

		fakeClient := test.NewFakeClient(t, sbLaraRedhatAdmin, redhatSpace)
		reconciler := prepareReconciler(t, fakeClient)
		fakeClient.MockDelete = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.DeleteOption) error {
			return fmt.Errorf("some error")
		}

		// when
		_, err := reconciler.Reconcile(context.TODO(), requestFor(sbLaraRedhatAdmin))

		// then
		require.Error(t, err)
		sb.AssertThatSpaceBinding(t, test.HostOperatorNs, "lara", "redhat", fakeClient).Exists()
	})
}

func TestDeleteSpaceBindingRequest(t *testing.T) {
	toolchainconfig := commonconfig.NewToolchainConfigObjWithReset(t,
		testconfig.SpaceConfig().SpaceBindingRequestEnabled(true))
	sbr := sbrtestcommon.NewSpaceBindingRequest("lara", "lara-tenant",
		sbrtestcommon.WithMUR("lara"),
		sbrtestcommon.WithSpaceRole("admin"))
	sbLaraAdmin := sb.NewSpaceBinding("lara", "lara", "admin", sbr.GetName(), sb.WithSpaceBindingRequest(sbr)) // the spacebinding was created from spacebindingrequest
	t.Run("SpaceBindingRequest is deleted", func(t *testing.T) {
		// given
		member1 := NewMemberClusterWithClient(test.NewFakeClient(t, sbr), "member-1", corev1.ConditionTrue)
		hostClient := test.NewFakeClient(t, sbLaraAdmin, toolchainconfig)
		reconciler := prepareReconciler(t, hostClient, member1)

		// when
		res, err := reconciler.Reconcile(context.TODO(), requestFor(sbLaraAdmin))

		// then
		require.Equal(t, 10*time.Second, res.RequeueAfter)
		require.NoError(t, err)
		sbrtestcommon.AssertThatSpaceBindingRequest(t, sbr.GetNamespace(), sbr.GetName(), member1.Client).DoesNotExist() // spacebindingrequest was deleted
	})

	t.Run("spaceBinding is deleted when spaceBindingRequest is missing", func(t *testing.T) {
		// given
		member1 := NewMemberClusterWithClient(test.NewFakeClient(t), "member-1", corev1.ConditionTrue) // for some reason spacebindingrequest is gone from member cluster
		hostClient := test.NewFakeClient(t, sbLaraAdmin, toolchainconfig)
		reconciler := prepareReconciler(t, hostClient, member1)

		// when
		res, err := reconciler.Reconcile(context.TODO(), requestFor(sbLaraAdmin))

		// then
		require.Equal(t, res.RequeueAfter, time.Duration(0)) // no requeue
		require.NoError(t, err)
		sb.AssertThatSpaceBinding(t, test.HostOperatorNs, "lara", "lara", hostClient).DoesNotExist() // the spacebinding is deleted
	})

	t.Run("unable to get SpaceBindingRequest", func(t *testing.T) {
		// given
		member1Client := test.NewFakeClient(t)
		member1Client.MockGet = func(ctx context.Context, key runtimeclient.ObjectKey, obj runtimeclient.Object, opts ...runtimeclient.GetOption) error {
			if _, ok := obj.(*v1alpha1.SpaceBindingRequest); ok {
				return fmt.Errorf("mock error")
			}
			return member1Client.Get(ctx, key, obj, opts...)
		}
		member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue) // for some reason spacebindingrequest is gone from member cluster
		hostClient := test.NewFakeClient(t, sbLaraAdmin, toolchainconfig)
		reconciler := prepareReconciler(t, hostClient, member1)

		// when
		_, err := reconciler.Reconcile(context.TODO(), requestFor(sbLaraAdmin))

		// then
		require.EqualError(t, err, "unable to get the current *v1alpha1.SpaceBindingRequest: mock error")
		sb.AssertThatSpaceBinding(t, test.HostOperatorNs, "lara", "lara", hostClient).Exists() // the spacebinding is not deleted yet
	})

	t.Run("fails while deleting the SpaceBindingRequest", func(t *testing.T) {
		// given
		member1Client := test.NewFakeClient(t, sbr)
		member1Client.MockDelete = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.DeleteOption) error {
			if _, ok := obj.(*v1alpha1.SpaceBindingRequest); ok {
				return fmt.Errorf("mock error")
			}
			return member1Client.Client.Delete(ctx, obj, opts...)
		}
		member1 := NewMemberClusterWithClient(member1Client, "member-1", corev1.ConditionTrue) // for some reason spacebindingrequest is gone from member cluster
		hostClient := test.NewFakeClient(t, sbLaraAdmin, toolchainconfig)
		reconciler := prepareReconciler(t, hostClient, member1)

		// when
		_, err := reconciler.Reconcile(context.TODO(), requestFor(sbLaraAdmin))

		// then
		require.EqualError(t, err, "unable to delete the SpaceBindingRequest: mock error")
		sb.AssertThatSpaceBinding(t, test.HostOperatorNs, "lara", "lara", hostClient).Exists() // the spacebinding is not deleted yet
	})
}

func prepareReconciler(t *testing.T, hostCl runtimeclient.Client, memberClusters ...*commoncluster.CachedToolchainCluster) *Reconciler {
	require.NoError(t, os.Setenv("WATCH_NAMESPACE", test.HostOperatorNs))
	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)

	clusters := map[string]cluster.Cluster{}
	for _, c := range memberClusters {
		clusters[c.Name] = cluster.Cluster{
			Config: &commoncluster.Config{
				Type:              commoncluster.Member,
				OperatorNamespace: c.OperatorNamespace,
				OwnerClusterName:  test.MemberClusterName,
			},
			Client: c.Client,
		}
	}

	reconciler := &Reconciler{
		Namespace:      test.HostOperatorNs,
		Scheme:         s,
		Client:         hostCl,
		MemberClusters: clusters,
	}
	return reconciler
}

func requestFor(s *v1alpha1.SpaceBinding) reconcile.Request {
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
			Namespace: "john-tenant",
			Name:      "unknown",
		},
	}
}
