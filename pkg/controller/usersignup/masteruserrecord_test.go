package usersignup

import (
	"testing"

	"github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNewMasterUserRecord(t *testing.T) {
	t.Run("when clusterResources template is specified", func(t *testing.T) {
		// given
		nSTemplateTier := newNsTemplateTier("advanced", "654321b", "dev", "stage", "extra")

		// when
		mur := newMasterUserRecord(nSTemplateTier, "johny", operatorNamespace, test.MemberClusterName, "123456789")

		// then
		assert.Equal(t, newExpectedMur(), *mur)
	})

	t.Run("when clusterResources template is NOT specified", func(t *testing.T) {
		// given
		nSTemplateTier := newNsTemplateTier("advanced", "654321b", "dev", "stage", "extra")
		nSTemplateTier.Spec.ClusterResources = nil

		// when
		mur := newMasterUserRecord(nSTemplateTier, "johny", operatorNamespace, test.MemberClusterName, "123456789")

		// then
		withoutClusterRes := newExpectedMur()
		withoutClusterRes.Spec.UserAccounts[0].Spec.NSTemplateSet.ClusterResources = nil
		assert.EqualValues(t, withoutClusterRes, *mur)
	})
}

func TestNewNsTemplateSetSpec(t *testing.T) {
	t.Run("when clusterResources template is specified", func(t *testing.T) {
		// given
		nSTemplateTier := newNsTemplateTier("advanced", "654321b", "dev", "stage", "extra")

		// when
		setSpec := NewNSTemplateSetSpec(nSTemplateTier)

		// then
		assert.Equal(t, newExpectedNsTemplateSetSpec(), setSpec)
	})

	t.Run("when clusterResources template is NOT specified", func(t *testing.T) {
		// given
		nSTemplateTier := newNsTemplateTier("advanced", "654321b", "dev", "stage", "extra")
		nSTemplateTier.Spec.ClusterResources = nil

		// when
		setSpec := NewNSTemplateSetSpec(nSTemplateTier)

		// then
		withoutClusterRes := newExpectedNsTemplateSetSpec()
		withoutClusterRes.ClusterResources = nil
		assert.Equal(t, withoutClusterRes, setSpec)
	})
}

func TestMigrateMurIfNecessary(t *testing.T) {
	t.Run("when mur is the same", func(t *testing.T) {
		// given
		nSTemplateTier := newNsTemplateTier("advanced", "654321b", "dev", "stage", "extra")
		mur := newMasterUserRecord(nSTemplateTier, "johny", operatorNamespace, test.MemberClusterName, "123456789")

		// when
		changed, changedMur := migrateMurIfNecessary(mur, nSTemplateTier)

		// then
		assert.False(t, changed)
		assert.Equal(t, newExpectedMur(), *changedMur)
	})

	t.Run("when mur is missing NsLimit", func(t *testing.T) {
		// given
		nSTemplateTier := newNsTemplateTier("advanced", "654321b", "dev", "stage", "extra")
		mur := newMasterUserRecord(nSTemplateTier, "johny", operatorNamespace, test.MemberClusterName, "123456789")
		mur.Spec.UserAccounts[0].Spec.NSLimit = ""

		// when
		changed, changedMur := migrateMurIfNecessary(mur, nSTemplateTier)

		// then
		assert.True(t, changed)
		assert.Equal(t, newExpectedMur(), *changedMur)
	})

	t.Run("when whole NSTemplateSet is missing", func(t *testing.T) {
		// given
		nSTemplateTier := newNsTemplateTier("advanced", "654321b", "dev", "stage", "extra")
		mur := newMasterUserRecord(nSTemplateTier, "johny", operatorNamespace, test.MemberClusterName, "123456789")
		mur.Spec.UserAccounts[0].Spec.NSTemplateSet = v1alpha1.NSTemplateSetSpec{}

		// when
		changed, changedMur := migrateMurIfNecessary(mur, nSTemplateTier)

		// then
		assert.True(t, changed)
		assert.Equal(t, newExpectedMur(), *changedMur)
	})

	t.Run("when NSTemplateSet is missing templateRefs", func(t *testing.T) {
		// given
		nSTemplateTier := newNsTemplateTier("advanced", "654321b", "dev", "stage", "extra")
		mur := newMasterUserRecord(nSTemplateTier, "johny", operatorNamespace, test.MemberClusterName, "123456789")
		mur.Spec.UserAccounts[0].Spec.NSTemplateSet.Namespaces = []v1alpha1.NSTemplateSetNamespace{
			{
				Type:        "dev",
				Revision:    "123abc1",
				TemplateRef: "",
			},
			{
				Type:        "stage",
				Revision:    "123abc2",
				TemplateRef: "",
			},
			{
				Type:        "extra",
				Revision:    "123abc3",
				TemplateRef: "",
			},
		}
		mur.Spec.UserAccounts[0].Spec.NSTemplateSet.ClusterResources = &v1alpha1.NSTemplateSetClusterResources{
			Revision:    "654321b",
			TemplateRef: "",
		}

		// when
		changed, changedMur := migrateMurIfNecessary(mur, nSTemplateTier)

		// then
		assert.True(t, changed)
		assert.Equal(t, newExpectedMur(), *changedMur)
	})

	t.Run("when one namespace is missing and one is extra, but rest is fine, then doesn't change", func(t *testing.T) {
		// given
		nSTemplateTier := newNsTemplateTier("advanced", "654321b", "dev", "stage", "extra")
		mur := newMasterUserRecord(nSTemplateTier, "johny", operatorNamespace, test.MemberClusterName, "123456789")
		mur.Spec.UserAccounts[0].Spec.NSTemplateSet.Namespaces[0].Type = "cicd"
		mur.Spec.UserAccounts[0].Spec.NSTemplateSet.Namespaces[0].TemplateRef = "advanced-cicd-123abc1"
		providedMur := mur.DeepCopy()

		// when
		changed, changedMur := migrateMurIfNecessary(mur, nSTemplateTier)

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

func newExpectedMur() v1alpha1.MasterUserRecord {
	return v1alpha1.MasterUserRecord{
		ObjectMeta: v1.ObjectMeta{
			Name:      "johny",
			Namespace: operatorNamespace,
			Labels: map[string]string{
				"toolchain.dev.openshift.com/user-id": "123456789",
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
