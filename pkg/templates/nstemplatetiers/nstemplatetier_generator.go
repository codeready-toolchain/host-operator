package nstemplatetiers

import (
	"sort"
	"strings"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	commonclient "github.com/codeready-toolchain/toolchain-common/pkg/client"

	templatev1 "github.com/openshift/api/template/v1"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var log = logf.Log.WithName("templates")

// CreateOrUpdateResources generates the NSTemplateTier resources from the namespace templates,
// then uses the manager's client to create or update the resources on the cluster.
func CreateOrUpdateResources(s *runtime.Scheme, client client.Client, namespace string, assets Assets) error {
	templatesByTier, err := loadTemplatesByTiers(assets)
	if err != nil {
		return errors.Wrap(err, "unable to create or update NSTemplateTiers")
	}
	tierObjs, err := newNSTemplateTiers(s, namespace, templatesByTier)
	if err != nil {
		return errors.Wrap(err, "unable to create or update NSTemplateTiers")
	}
	// sort the tiers by name so we have a predictive order of creation/update, which also avoids test flakiness
	tiers := make([]string, 0, len(tierObjs))
	for tier := range tierObjs {
		tiers = append(tiers, tier)
	}
	sort.Strings(tiers)
	for _, tier := range tiers {
		tierObj := tierObjs[tier]
		log.Info("creating or updating NSTemplateTier", "namespace", tierObj.Namespace, "name", tierObj.Name)
		cl := commonclient.NewApplyClient(client, s)
		created, err := cl.CreateOrUpdateObject(tierObj, true, nil)
		if err != nil {
			return errors.Wrapf(err, "unable to create or update the '%s' NSTemplateTiers in namespace '%s'", tierObj.Name, tierObj.Namespace)
		}
		if created {
			log.Info("NSTemplateTier resource created", "namespace", tierObj.Namespace, "name", tierObj.Name)
		} else {
			log.Info("NSTemplateTier resource updated", "namespace", tierObj.Namespace, "name", tierObj.Name, "ResourceVersion", tierObj.ResourceVersion)
		}
	}
	return nil
}

// templates: namespaces and other cluster-scoped resources belonging to a given tier ("advanced", "basic", "team", etc.)
type templates struct {
	namespaceTemplates map[string]template // namespace templates (including roles, etc.) indexed by type ("dev", "code", "stage")
	clusterTemplate    *template           // other cluster-scoped resources, in a single template file
}

// template: a template content and its latest git revision
type template struct {
	revision string
	content  []byte
}

// loadAssetsByTiers loads the assets and dispatches them by tiers, assuming the given `assets` has the following structure:
//
// metadata.yaml
// advanced/
//   cluster.yaml
//   ns_code.yaml
//   ns_xyz.yaml
// basic/
//   cluster.yaml
//   ns_code.yaml
//   ns_xyz.yaml
// team/
//   ns_code.yaml
//   ns_xyz.yaml
func loadTemplatesByTiers(assets Assets) (map[string]*templates, error) {
	metadataContent, err := assets.Asset("metadata.yaml")
	if err != nil {
		return nil, errors.Wrapf(err, "unable to load templates")
	}
	metadata := make(map[string]string)
	err = yaml.Unmarshal([]byte(metadataContent), &metadata)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to load templates")
	}

	results := make(map[string]*templates)
	for _, name := range assets.Names() {
		if name == "metadata.yaml" {
			continue
		}
		// split the name using the `/` separator
		parts := strings.Split(name, "/")
		// skip any name that does not have 3 parts
		if len(parts) != 2 {
			return nil, errors.Wrapf(err, "unable to load templates: invalid name format for file '%s'", name)
		}
		tier := parts[0]
		filename := parts[1]
		if _, exists := results[tier]; !exists {
			results[tier] = &templates{
				namespaceTemplates: map[string]template{},
			}
		}
		content, err := assets.Asset(name)
		if err != nil {
			return nil, errors.Wrapf(err, "unable to load templates")
		}
		tmpl := template{
			revision: metadata[strings.TrimSuffix(name, ".yaml")],
			content:  content,
		}
		switch {
		case strings.HasPrefix(filename, "ns_"):
			kind := strings.TrimSuffix(strings.TrimPrefix(filename, "ns_"), ".yaml")
			results[tier].namespaceTemplates[kind] = tmpl
		case filename == "cluster.yaml":
			results[tier].clusterTemplate = &tmpl
		default:
			return nil, errors.Wrapf(err, "unable to load templates: unknown scope for file '%s'", name)
		}
	}
	return results, nil
}

// NewNSTemplateTiers generates all manifests, indexed by their associated tier
func newNSTemplateTiers(s *runtime.Scheme, namespace string, templatesByTier map[string]*templates) (map[string]*toolchainv1alpha1.NSTemplateTier, error) {
	decoder := serializer.NewCodecFactory(s).UniversalDeserializer()
	tiers := make(map[string]*toolchainv1alpha1.NSTemplateTier, len(templatesByTier))
	for tier, tmpls := range templatesByTier {
		tmpl, err := newNSTemplateTier(decoder, namespace, tier, *tmpls)
		if err != nil {
			return nil, errors.Wrapf(err, "unable to generate NSTemplateTiers")
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
//       revision: "y8f907f6"
//       template: >
//         <yaml-ns-template>
//     - type: dev
//       revision: "f8q907f4"
//       template: >
//         <yaml-ns-template>
//     - type: stage
//       revision: "907fy8f6"
//       template: >
//         <yaml-ns-template>
//     clusterResources:
//       revision: "907fy8f6"
//       template: >
//         <yaml-ns-template>
// ------
func newNSTemplateTier(decoder runtime.Decoder, namespace, tier string, tmpls templates) (*toolchainv1alpha1.NSTemplateTier, error) {
	// retrieve the namespace types and order them, so we can compare
	// with the expected templates during the tests
	namespaceKinds := make([]string, 0, len(tmpls.namespaceTemplates))
	for kind := range tmpls.namespaceTemplates {
		namespaceKinds = append(namespaceKinds, kind)
	}
	sort.Strings(namespaceKinds)
	result := &toolchainv1alpha1.NSTemplateTier{
		ObjectMeta: metav1.ObjectMeta{
			Name:      tier,
			Namespace: namespace,
		},
		Spec: toolchainv1alpha1.NSTemplateTierSpec{},
	}
	for _, kind := range namespaceKinds {
		// convert the content into a templatev1.Template
		tmplObj := &templatev1.Template{}
		_, _, err := decoder.Decode(tmpls.namespaceTemplates[kind].content, nil, tmplObj)
		if err != nil {
			return nil, errors.Wrapf(err, "unable to generate '%s' NSTemplateTier manifest", tier)
		}
		// add it to the NSTemplateTier obj
		result.Spec.Namespaces = append(result.Spec.Namespaces, toolchainv1alpha1.NSTemplateTierNamespace{
			Type:     kind,
			Revision: tmpls.namespaceTemplates[kind].revision,
			Template: *tmplObj,
		})
	}
	// also, add the cluster resource template+revision if it exists
	if tmpls.clusterTemplate != nil {
		// convert the content into a templatev1.Template
		tmplObj := &templatev1.Template{}
		_, _, err := decoder.Decode(tmpls.clusterTemplate.content, nil, tmplObj)
		if err != nil {
			return nil, errors.Wrapf(err, "unable to generate '%s' NSTemplateTier manifest", tier)
		}
		result.Spec.ClusterResources = &toolchainv1alpha1.NSTemplateTierClusterResources{
			Revision: tmpls.clusterTemplate.revision,
			Template: *tmplObj,
		}
	}
	return result, nil
}
