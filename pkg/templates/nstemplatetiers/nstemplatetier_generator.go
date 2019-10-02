package nstemplatetiers

import (
	"context"
	"fmt"
	"sort"
	"strings"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"

	templatev1 "github.com/openshift/api/template/v1"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var log = logf.Log.WithName("templates")

// CreateOrUpdateResources generates the NSTemplateTier resources from the namespace templates,
// then uses the manager's client to create or update the resources on the cluster.
func CreateOrUpdateResources(mgr manager.Manager, namespace string, asset func(name string) ([]byte, error)) error {
	g, err := NewNSTemplateTierGenerator(mgr.GetScheme(), asset)
	if err != nil {
		return errors.Wrap(err, "unable to create or update NSTemplateTiers")
	}
	tiers, err := g.NewNSTemplateTiers(namespace)
	if err != nil {
		return errors.Wrap(err, "unable to create or update NSTemplateTiers")
	}
	for _, tier := range tiers {
		err := mgr.GetClient().Create(context.TODO(), tier)
		if err != nil {
			return errors.Wrapf(err, "unable to create NSTemplateTier %s", tier.Name)
		}
	}
	return nil
}

// NSTemplateTierGenerator the NSTemplateTier manifest generator
type NSTemplateTierGenerator struct {
	asset     func(name string) ([]byte, error) // the func which gives access to the
	revisions map[string]map[string]string
	decoder   runtime.Decoder
}

// NewNSTemplateTierGenerator returns a new NSTemplateTierGenerator
func NewNSTemplateTierGenerator(s *runtime.Scheme, asset func(name string) ([]byte, error)) (*NSTemplateTierGenerator, error) {
	metadata, err := asset("metadata.yaml")
	if err != nil {
		return nil, errors.Wrapf(err, "unable to initialize the NSTemplateTierGenerator")
	}
	revisions, err := parseAllRevisions(metadata)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to initialize the NSTemplateTierGenerator")
	}
	return &NSTemplateTierGenerator{
		asset:     asset,
		revisions: revisions,
		decoder:   serializer.NewCodecFactory(s).UniversalDeserializer(),
	}, nil
}

// parseRevisions returns a "supermap" in which:
// - each key is a tier kind (eg: "basic", "advanced", etc.)
// - each value is a map of revisions indexed by their associated template kind
//   (eg: {"code":"123456", "dev":"abcdef", "stage":"cafe01"})
func parseAllRevisions(metadata []byte) (map[string]map[string]string, error) {
	data := make(map[string]interface{})
	err := yaml.Unmarshal([]byte(metadata), &data)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to parse all template revisions")
	}
	// reads all entries in the YAML, splitting the keys
	// as we expect the following format: '<tier_kind>-<namespace_kind>' (eg: 'advanced-code')
	revisions := make(map[string]map[string]string)
	for filename, revision := range data {
		data := strings.Split(filename, "-")
		if len(data) != 2 {
			return nil, errors.Errorf("invalid namespace template filename. Expected format: '<tier_kind>-<namespace_kind>', got %s", filename)
		}
		tierKind := data[0]
		nsKind := data[1]
		// create a new entry if needed
		r, ok := revision.(string)
		if !ok {
			return nil, errors.Errorf("invalid namespace template filename revision for '%[1]s'. Expected a string, got a %[2]T ('%[2]v')", filename, revision)
		}
		if _, ok := revisions[tierKind]; !ok {
			revisions[tierKind] = make(map[string]string, 3) // expect 3 entries: 'code', 'dev' and 'stage'
		}
		revisions[tierKind][nsKind] = r
	}
	log.Info("templates revisions loaded", "revisions", revisions)
	return revisions, nil
}

// NewNSTemplateTiers generates all manifests, indexed by their associated tier kind and by their namespace kind
// eg:
// - advanced:
//   - code: <[]byte>
//   - dev: <[]byte>
//   - stage: <[]byte>
// - basic:
//   - code: <[]byte>
//   - dev: <[]byte>
//   - stage: <[]byte>
func (g NSTemplateTierGenerator) NewNSTemplateTiers(namespace string) (map[string]*toolchainv1alpha1.NSTemplateTier, error) {
	tiers := make(map[string]*toolchainv1alpha1.NSTemplateTier, len(g.revisions))
	for tier := range g.revisions {
		tmpl, err := g.NewNSTemplateTier(tier, namespace)
		if err != nil {
			return nil, errors.Wrapf(err, "unable to generate all NSTemplateTier manifests")
		}
		tiers[tier] = tmpl
	}
	return tiers, nil
}

// NewNSTemplateTier initializes a complete NSTemplateTier object
// by embedding the `<tier>-code.yml`, `<tier>-dev.yml` and `<tier>-stage.yml`
// file along with each one's git (short) commit as the revision associated with
// the template.
//
// Something like:
// ------
// kind: NSTemplateTier
//   metadata:
//     name: basic
//   spec:
//     namespaces:
//     - type: code
//       revision: y8f907f6
//       template: >
//         <yaml-ns-template>
//     - type: dev
//       revision: f8q907f4
//       template: >
//         <yaml-ns-template>
//     - type: stage
//       revision: 907fy8f6
//       template: >
//         <yaml-ns-template>
// ------
func (g NSTemplateTierGenerator) NewNSTemplateTier(tier, namespace string) (*toolchainv1alpha1.NSTemplateTier, error) {
	revisions, ok := g.revisions[tier]
	if !ok {
		return nil, errors.Errorf("tier '%s' does not exist", tier)
	}
	obj := &toolchainv1alpha1.NSTemplateTier{
		ObjectMeta: metav1.ObjectMeta{
			Name:      tier,
			Namespace: namespace,
		},
		Spec: toolchainv1alpha1.NSTemplateTierSpec{},
	}
	// retrieve the namespace types and order them, so we can order them, and compare
	// with the expected templates during the tests
	nsTypes := make([]string, 0, len(revisions))
	for nsType := range revisions {
		nsTypes = append(nsTypes, nsType)
	}
	sort.Strings(nsTypes)
	for _, nsType := range nsTypes {
		// get the content of the `-<t>.yaml` file
		tmpl, err := g.asset(fmt.Sprintf("%s-%s.yaml", tier, nsType))
		if err != nil {
			return nil, errors.Wrapf(err, "unable to generate '%s' NSTemplateTier manifest", tier)
		}
		// parse the content as
		tmplObj, err := decodeTemplate(g.decoder, tmpl)
		if err != nil {
			return nil, errors.Wrapf(err, "unable to generate '%s' NSTemplateTier manifest", tier)
		}
		// add it to the NSTemplateTier obj
		obj.Spec.Namespaces = append(obj.Spec.Namespaces, toolchainv1alpha1.NSTemplateTierNamespace{
			Type:     nsType,
			Revision: revisions[nsType],
			Template: *tmplObj,
		})
	}
	return obj, nil
}

func decodeTemplate(decoder runtime.Decoder, tmplContent []byte) (*templatev1.Template, error) {
	tmpl := &templatev1.Template{}
	_, _, err := decoder.Decode(tmplContent, nil, tmpl)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to decode template")
	}
	return tmpl, nil
}
