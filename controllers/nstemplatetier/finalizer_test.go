package nstemplatetier

import (
	"context"
	"fmt"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/hash"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestNSTemplateTierUsedFinalizer(t *testing.T) {
	t.Run("succeeds when tier not used", func(t *testing.T) {
		// given
		tier := &toolchainv1alpha1.NSTemplateTier{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "tier",
				Namespace: test.HostOperatorNs,
			},
		}
		cl := test.NewFakeClient(t, tier)
		f := &nsTemplateTierUsedFinalizer{Client: cl}

		// when
		_, err := f.Finalize(context.TODO(), tier)

		// then
		require.NoError(t, err)
	})
	t.Run("fails when tier used", func(t *testing.T) {
		// given
		tier := &toolchainv1alpha1.NSTemplateTier{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "tier",
				Namespace: test.HostOperatorNs,
			},
		}
		space := &toolchainv1alpha1.Space{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-space",
				Namespace: tier.Namespace,
				Labels:    map[string]string{hash.TemplateTierHashLabelKey(tier.Name): "1234"},
			},
		}
		cl := test.NewFakeClient(t, tier, space)
		f := &nsTemplateTierUsedFinalizer{Client: cl}

		// when
		_, err := f.Finalize(context.TODO(), tier)

		// then
		require.Error(t, err)
	})
	t.Run("fails on list error", func(t *testing.T) {
		// given
		tier := &toolchainv1alpha1.NSTemplateTier{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "tier",
				Namespace: test.HostOperatorNs,
			},
		}
		cl := test.NewFakeClient(t, tier)
		cl.MockList = func(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
			return fmt.Errorf("fake error")
		}
		f := &nsTemplateTierUsedFinalizer{Client: cl}

		// when
		_, err := f.Finalize(context.TODO(), tier)

		// then
		require.Error(t, err)
	})
}
