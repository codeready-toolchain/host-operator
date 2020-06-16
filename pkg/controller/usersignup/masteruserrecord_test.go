package usersignup

import (
	"testing"

	"github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/controller/nstemplatetier"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNewMasterUserRecord(t *testing.T) {
	t.Run("when clusterResources template is specified", func(t *testing.T) {
		// given
		nsTemplateTier := newNsTemplateTier("advanced", "654321b", "dev", "stage", "extra")

		// when
		mur, err := newMasterUserRecord(nsTemplateTier, "johny", operatorNamespace, test.MemberClusterName, "123456789")

		// then
		require.NoError(t, err)
		assert.Equal(t, newExpectedMur(nsTemplateTier), *mur)
	})

	t.Run("when clusterResources template is NOT specified", func(t *testing.T) {
		// given
		nsTemplateTier := newNsTemplateTier("advanced", "654321b", "dev", "stage", "extra")
		nsTemplateTier.Spec.ClusterResources = nil

		// when
		mur, err := newMasterUserRecord(nsTemplateTier, "johny", operatorNamespace, test.MemberClusterName, "123456789")

		// then
		require.NoError(t, err)
		withoutClusterRes := newExpectedMur(nsTemplateTier)
		withoutClusterRes.Spec.UserAccounts[0].Spec.NSTemplateSet.ClusterResources = nil
		assert.EqualValues(t, withoutClusterRes, *mur)
	})
}

func TestNewNsTemplateSetSpec(t *testing.T) {
	t.Run("when clusterResources template is specified", func(t *testing.T) {
		// given
		nsTemplateTier := newNsTemplateTier("advanced", "654321b", "dev", "stage", "extra")

		// when
		setSpec := NewNSTemplateSetSpec(nsTemplateTier)

		// then
		assert.Equal(t, newExpectedNsTemplateSetSpec(), setSpec)
	})

	t.Run("when clusterResources template is NOT specified", func(t *testing.T) {
		// given
		nsTemplateTier := newNsTemplateTier("advanced", "654321b", "dev", "stage", "extra")
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
	t.Run("when mur is the same", func(t *testing.T) {
		// given
		nsTemplateTier := newNsTemplateTier("advanced", "654321b", "dev", "stage", "extra")
		mur, err := newMasterUserRecord(nsTemplateTier, "johny", operatorNamespace, test.MemberClusterName, "123456789")
		require.NoError(t, err)

		// when
		changed, changedMur := migrateOrFixMurIfNecessary(mur, nsTemplateTier)

		// then
		assert.False(t, changed)
		assert.Equal(t, newExpectedMur(nsTemplateTier), *changedMur)
	})

	t.Run("when mur is missing NsLimit", func(t *testing.T) {
		// given
		nsTemplateTier := newNsTemplateTier("advanced", "654321b", "dev", "stage", "extra")
		mur, err := newMasterUserRecord(nsTemplateTier, "johny", operatorNamespace, test.MemberClusterName, "123456789")
		require.NoError(t, err)
		mur.Spec.UserAccounts[0].Spec.NSLimit = ""

		// when
		changed, changedMur := migrateOrFixMurIfNecessary(mur, nsTemplateTier)

		// then
		assert.True(t, changed)
		assert.Equal(t, newExpectedMur(nsTemplateTier), *changedMur)
	})

	t.Run("when whole NSTemplateSet is missing", func(t *testing.T) {
		// given
		nsTemplateTier := newNsTemplateTier("advanced", "654321b", "dev", "stage", "extra")
		mur, err := newMasterUserRecord(nsTemplateTier, "johny", operatorNamespace, test.MemberClusterName, "123456789")
		require.NoError(t, err)
		mur.Spec.UserAccounts[0].Spec.NSTemplateSet = v1alpha1.NSTemplateSetSpec{}

		// when
		changed, changedMur := migrateOrFixMurIfNecessary(mur, nsTemplateTier)

		// then
		assert.True(t, changed)
		assert.Equal(t, newExpectedMur(nsTemplateTier), *changedMur)
	})

	t.Run("when one namespace is missing and one is extra, but rest is fine, then doesn't change", func(t *testing.T) {
		// given
		nsTemplateTier := newNsTemplateTier("advanced", "654321b", "dev", "stage", "extra")
		mur, err := newMasterUserRecord(nsTemplateTier, "johny", operatorNamespace, test.MemberClusterName, "123456789")
		require.NoError(t, err)
		mur.Spec.UserAccounts[0].Spec.NSTemplateSet.Namespaces[0].TemplateRef = "advanced-cicd-123abc1"
		providedMur := mur.DeepCopy()

		// when
		changed, changedMur := migrateOrFixMurIfNecessary(mur, nsTemplateTier)

		// then
		assert.False(t, changed)
		assert.Equal(t, *providedMur, *changedMur)
	})
}

func newExpectedNsTemplateSetSpec() v1alpha1.NSTemplateSetSpec {
	return v1alpha1.NSTemplateSetSpec{
		TierName: "advanced",
		Namespaces: []v1alpha1.NSTemplateSetNamespace{
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
		ClusterResources: &v1alpha1.NSTemplateSetClusterResources{
			TemplateRef: "advanced-clusterresources-654321b",
		},
	}
}

func newExpectedMur(tier *v1alpha1.NSTemplateTier) v1alpha1.MasterUserRecord {
	hash, _ := nstemplatetier.ComputeTemplateRefsHash(tier)
	return v1alpha1.MasterUserRecord{
		ObjectMeta: v1.ObjectMeta{
			Name:      "johny",
			Namespace: operatorNamespace,
			Labels: map[string]string{
				"toolchain.dev.openshift.com/user-id":              "123456789",
				nstemplatetier.TemplateTierHashLabelKey(tier.Name): hash,
			},
		},
		Spec: v1alpha1.MasterUserRecordSpec{
			UserID:        "123456789",
			Banned:        false,
			Disabled:      false,
			Deprovisioned: false,
			UserAccounts: []v1alpha1.UserAccountEmbedded{
				{
					SyncIndex:     "",
					TargetCluster: test.MemberClusterName,
					Spec: v1alpha1.UserAccountSpecEmbedded{
						UserAccountSpecBase: v1alpha1.UserAccountSpecBase{
							NSLimit:       "default",
							NSTemplateSet: newExpectedNsTemplateSetSpec(),
						},
					},
				},
			},
		},
	}
}
