package tiertemplate

import (
	"testing"
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/apis"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	templatev1 "github.com/openshift/api/template/v1"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/kubectl/pkg/scheme"
)

type TierTemplateOption func(*toolchainv1alpha1.TierTemplate)

func NewTierTemplate(t *testing.T, name, ns string, opts ...TierTemplateOption) *toolchainv1alpha1.TierTemplate {
	t.Helper()
	tt := &toolchainv1alpha1.TierTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
	}
	for _, apply := range opts {
		apply(tt)
	}
	return tt
}

func WithFinalizer() TierTemplateOption {
	return func(tt *toolchainv1alpha1.TierTemplate) {
		tt.Finalizers = []string{toolchainv1alpha1.FinalizerName}
	}
}

func WithDeletionTimestamp() TierTemplateOption {
	return func(tt *toolchainv1alpha1.TierTemplate) {
		tt.DeletionTimestamp = &metav1.Time{Time: time.Now()}
	}
}

func WithTierName(tierName string) TierTemplateOption {
	return func(tt *toolchainv1alpha1.TierTemplate) {
		tt.Spec.TierName = tierName
	}
}

func WithTypeName(typeName string) TierTemplateOption {
	return func(tt *toolchainv1alpha1.TierTemplate) {
		tt.Spec.Type = typeName
	}
}

func WithRevision(revision string) TierTemplateOption {
	return func(tt *toolchainv1alpha1.TierTemplate) {
		tt.Spec.Revision = revision
	}
}

func WithTemplate(tmpl templatev1.Template) TierTemplateOption {
	return func(tt *toolchainv1alpha1.TierTemplate) {
		tt.Spec.Template = tmpl
	}
}

func WithTestTierTemplate(t *testing.T) TierTemplateOption {
	t.Helper()
	return func(tt *toolchainv1alpha1.TierTemplate) {
		t.Helper()
		var (
			ns test.TemplateObject = `
- apiVersion: v1
  kind: Namespace
  metadata:
    name: ${SPACE_NAME}
`
			spacename test.TemplateParam = `
- name: SPACE_NAME
  value: johnsmith`
		)
		s := scheme.Scheme
		err := apis.AddToScheme(s)
		require.NoError(t, err)
		codecFactory := serializer.NewCodecFactory(s)
		decoder := codecFactory.UniversalDeserializer()
		tmpl := templatev1.Template{}
		_, _, err = decoder.Decode([]byte(test.CreateTemplate(test.WithObjects(ns), test.WithParams(spacename))), nil, &tmpl)
		require.NoError(t, err)

		WithTemplate(tmpl)(tt)
	}
}

func WithTemplateObjects(objs ...unstructured.Unstructured) TierTemplateOption {
	return func(tt *toolchainv1alpha1.TierTemplate) {
		if len(objs) == 0 {
			tt.Spec.TemplateObjects = nil
			return
		}

		var templateObjects []runtime.RawExtension
		for i := range objs {
			templateObjects = append(templateObjects, runtime.RawExtension{Object: &objs[i]})
		}
		tt.Spec.TemplateObjects = templateObjects
	}
}
