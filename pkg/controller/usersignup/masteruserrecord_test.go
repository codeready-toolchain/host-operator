package usersignup

import (
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/controller/nstemplatetier"
	. "github.com/codeready-toolchain/host-operator/test"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNewMasterUserRecord(t *testing.T) {

	t.Run("when clusterResources template is specified", func(t *testing.T) {
		// given
		userSignup := NewUserSignup()
		nsTemplateTier := newNsTemplateTier("advanced", "dev", "stage", "extra")

		// when
		mur, err := newMasterUserRecord(userSignup, test.MemberClusterName, nsTemplateTier, "johny")

		// then
		require.NoError(t, err)
		assert.Equal(t, newExpectedMur(nsTemplateTier, userSignup), mur)
	})

	t.Run("when clusterResources template is NOT specified", func(t *testing.T) {
		// given
		userSignup := NewUserSignup()
		nsTemplateTier := newNsTemplateTier("advanced", "dev", "stage", "extra")
		nsTemplateTier.Spec.ClusterResources = nil

		// when
		mur, err := newMasterUserRecord(userSignup, test.MemberClusterName, nsTemplateTier, "johny")

		// then
		require.NoError(t, err)
		withoutClusterRes := newExpectedMur(nsTemplateTier, userSignup)
		withoutClusterRes.Spec.UserAccounts[0].Spec.NSTemplateSet.ClusterResources = nil
		assert.EqualValues(t, withoutClusterRes, mur)
	})
}

func TestNewNsTemplateSetSpec(t *testing.T) {
	t.Run("when clusterResources template is specified", func(t *testing.T) {
		// given
		nsTemplateTier := newNsTemplateTier("advanced", "dev", "stage", "extra")

		// when
		setSpec := NewNSTemplateSetSpec(nsTemplateTier)

		// then
		assert.Equal(t, newExpectedNsTemplateSetSpec(), setSpec)
	})

	t.Run("when clusterResources template is NOT specified", func(t *testing.T) {
		// given
		nsTemplateTier := newNsTemplateTier("advanced", "dev", "stage", "extra")
		nsTemplateTier.Spec.ClusterResources = nil

		// when
		setSpec := NewNSTemplateSetSpec(nsTemplateTier)

		// then
		withoutClusterRes := newExpectedNsTemplateSetSpec()
		withoutClusterRes.ClusterResources = nil
		assert.Equal(t, withoutClusterRes, setSpec)
	})
}

func TestMigrateMurIfNecessary(t *testing.T) {

	t.Run("no update needed", func(t *testing.T) {

		t.Run("when mur is the same", func(t *testing.T) {
			// given
			userSignup := NewUserSignup()
			nsTemplateTier := newNsTemplateTier("advanced", "dev", "stage", "extra")
			mur, err := newMasterUserRecord(userSignup, test.MemberClusterName, nsTemplateTier, "johny")
			require.NoError(t, err)

			// when
			changed, err := migrateOrFixMurIfNecessary(mur, nsTemplateTier, userSignup)

			// then
			require.NoError(t, err)
			assert.False(t, changed)
			assert.Equal(t, newExpectedMur(nsTemplateTier, userSignup), mur)
		})

		t.Run("when one namespace is missing and one is extra, but rest is fine, then doesn't change", func(t *testing.T) {
			// given
			userSignup := NewUserSignup()
			nsTemplateTier := newNsTemplateTier("advanced", "dev", "stage", "extra")
			mur, err := newMasterUserRecord(userSignup, test.MemberClusterName, nsTemplateTier, "johny")
			require.NoError(t, err)
			mur.Spec.UserAccounts[0].Spec.NSTemplateSet.Namespaces[0].TemplateRef = "advanced-cicd-123abc1"
			providedMur := mur.DeepCopy()

			// when
			changed, err := migrateOrFixMurIfNecessary(mur, nsTemplateTier, userSignup)

			// then
			require.NoError(t, err)
			assert.False(t, changed)
			assert.Equal(t, *providedMur, *mur)
		})

		// TODO: to be removed once CRT-1075 is done
		t.Run("when email annotation is set", func(t *testing.T) {
			// given
			logf.SetLogger(zap.New(zap.UseDevMode(true)))
			userSignup := NewUserSignup() // email is `foo@redhat.com`
			nsTemplateTier := newNsTemplateTier("advanced", "dev", "stage", "extra")
			mur, err := newMasterUserRecord(userSignup, test.MemberClusterName, nsTemplateTier, "johny")
			require.NoError(t, err)
			providedMur := mur.DeepCopy()

			// when
			changed, err := migrateOrFixMurIfNecessary(mur, nsTemplateTier, userSignup)

			// then
			require.NoError(t, err)
			assert.False(t, changed)
			assert.Equal(t, *providedMur, *mur)
		})
	})

	t.Run("update needed", func(t *testing.T) {

		t.Run("when mur is missing NsLimit", func(t *testing.T) {
			// given
			userSignup := NewUserSignup()
			nsTemplateTier := newNsTemplateTier("advanced", "dev", "stage", "extra")
			mur, err := newMasterUserRecord(userSignup, test.MemberClusterName, nsTemplateTier, "johny")
			require.NoError(t, err)
			mur.Spec.UserAccounts[0].Spec.NSLimit = ""

			// when
			changed, err := migrateOrFixMurIfNecessary(mur, nsTemplateTier, userSignup)

			// then
			require.NoError(t, err)
			assert.True(t, changed)
			assert.Equal(t, newExpectedMur(nsTemplateTier, userSignup), mur)
		})

		t.Run("when whole NSTemplateSet is missing", func(t *testing.T) {
			// given
			userSignup := NewUserSignup()
			nsTemplateTier := newNsTemplateTier("advanced", "dev", "stage", "extra")
			mur, err := newMasterUserRecord(userSignup, test.MemberClusterName, nsTemplateTier, "johny")
			require.NoError(t, err)
			mur.Spec.UserAccounts[0].Spec.NSTemplateSet = toolchainv1alpha1.NSTemplateSetSpec{}

			// when
			changed, err := migrateOrFixMurIfNecessary(mur, nsTemplateTier, userSignup)

			// then
			require.NoError(t, err)
			assert.True(t, changed)
			assert.Equal(t, newExpectedMur(nsTemplateTier, userSignup), mur)
		})

		t.Run("when tier labels are missing", func(t *testing.T) {
			// given
			userSignup := NewUserSignup()
			nsTemplateTier := newNsTemplateTier("advanced", "dev", "stage", "extra")
			mur, err := newMasterUserRecord(userSignup, test.MemberClusterName, nsTemplateTier, "johny")
			delete(mur.Labels, "toolchain.dev.openshift.com/advanced-tier-hash") // removed for the purpose of this test
			require.NoError(t, err)

			// when
			changed, err := migrateOrFixMurIfNecessary(mur, nsTemplateTier, userSignup)

			// then
			require.NoError(t, err)
			assert.True(t, changed)
			assert.Equal(t, newExpectedMur(nsTemplateTier, userSignup), mur)
		})

		// TODO: to be removed once CRT-1075 is done
		t.Run("when email annotation is missing", func(t *testing.T) {
			// given
			logf.SetLogger(zap.New(zap.UseDevMode(true)))
			userSignup := NewUserSignup() // email is `foo@redhat.com`
			nsTemplateTier := newNsTemplateTier("advanced", "dev", "stage", "extra")
			mur, err := newMasterUserRecord(userSignup, test.MemberClusterName, nsTemplateTier, "johny")
			require.NoError(t, err)
			delete(mur.Annotations, toolchainv1alpha1.MasterUserRecordEmailAnnotationKey)

			// when
			changed, err := migrateOrFixMurIfNecessary(mur, nsTemplateTier, userSignup)

			// then
			require.NoError(t, err)
			assert.True(t, changed)
			assert.Equal(t, newExpectedMur(nsTemplateTier, userSignup), mur)
		})
	})

}

func newExpectedNsTemplateSetSpec() toolchainv1alpha1.NSTemplateSetSpec {
	return toolchainv1alpha1.NSTemplateSetSpec{
		TierName: "advanced",
		Namespaces: []toolchainv1alpha1.NSTemplateSetNamespace{
			{
				TemplateRef: "advanced-dev-123abc1",
			},
			{
				TemplateRef: "advanced-stage-123abc2",
			},
			{
				TemplateRef: "advanced-extra-123abc3",
			},
		},
		ClusterResources: &toolchainv1alpha1.NSTemplateSetClusterResources{
			TemplateRef: "advanced-clusterresources-654321b",
		},
	}
}

func newExpectedMur(tier *toolchainv1alpha1.NSTemplateTier, userSignup *toolchainv1alpha1.UserSignup) *toolchainv1alpha1.MasterUserRecord {
	hash, _ := nstemplatetier.ComputeHashForNSTemplateTier(tier)
	return &toolchainv1alpha1.MasterUserRecord{
		ObjectMeta: v1.ObjectMeta{
			Name:      "johny",
			Namespace: test.HostOperatorNs,
			Labels: map[string]string{
				"toolchain.dev.openshift.com/owner":                userSignup.Name,
				nstemplatetier.TemplateTierHashLabelKey(tier.Name): hash,
			},
			Annotations: map[string]string{
				"toolchain.dev.openshift.com/user-email": "foo@redhat.com",
			},
		},
		Spec: toolchainv1alpha1.MasterUserRecordSpec{
			UserID:        userSignup.Spec.Userid,
			Banned:        false,
			Disabled:      false,
			Deprovisioned: false,
			UserAccounts: []toolchainv1alpha1.UserAccountEmbedded{
				{
					SyncIndex:     "",
					TargetCluster: test.MemberClusterName,
					Spec: toolchainv1alpha1.UserAccountSpecEmbedded{
						UserAccountSpecBase: toolchainv1alpha1.UserAccountSpecBase{
							NSLimit:       "default",
							NSTemplateSet: newExpectedNsTemplateSetSpec(),
						},
					},
				},
			},
		},
	}
}
