package nstemplatetierrevisioncleanup

import (
	"fmt"
	"strings"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	templatev1 "github.com/openshift/api/template/v1"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/kubernetes/scheme"
)

const (
	operatorNamespace = "toolchain-host-operator"
)

func TestReconcile(t *testing.T) {

}

func createTierTemplate(t *testing.T, typeName string, withTemplateObjects []runtime.RawExtension, tierName string) *toolchainv1alpha1.TierTemplate {
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

	revision := "123456new"
	// we can set the template field to something empty as it is not relevant for the tests
	tt := &toolchainv1alpha1.TierTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      strings.ToLower(fmt.Sprintf("%s-%s-%s", tierName, typeName, revision)),
			Namespace: test.HostOperatorNs,
		},
		Spec: toolchainv1alpha1.TierTemplateSpec{
			TierName: tierName,
			Type:     typeName,
			Revision: revision,
			Template: tmpl,
		},
	}

	// just copy the raw objects to the templateObjects field
	// TODO this will be removed once we switch on using templateObjects only in the TierTemplates
	if withTemplateObjects != nil {
		tt.Spec.TemplateObjects = withTemplateObjects
	}

	return tt
}

func withTemplateObjects(templates ...unstructured.Unstructured) []runtime.RawExtension {
	var templateObjects []runtime.RawExtension
	for i := range templates {
		templateObjects = append(templateObjects, runtime.RawExtension{Object: &templates[i]})
	}
	return templateObjects
}
