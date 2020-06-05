package nstemplatetiers

import (
	"context"
	"fmt"
	"sort"
	"strings"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/templates/assets"
	commonclient "github.com/codeready-toolchain/toolchain-common/pkg/client"

	templatev1 "github.com/openshift/api/template/v1"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var log = logf.Log.WithName("templates")

// CreateOrUpdateResources generates the NSTemplateTier resources from the namespace templates,
// then uses the manager's client to create or update the resources on the cluster.
func CreateOrUpdateResources(s *runtime.Scheme, client client.Client, namespace string, assets assets.Assets) error {

	// create the TierTemplates
	tierTmplsByTier, err := newTierTemplates(s, namespace, assets)
	if err != nil {
		return errors.Wrap(err, "unable to create TierTemplates")
	}
	for _, tierTmpls := range tierTmplsByTier {
		for _, tierTmpl := range tierTmpls {
			// using the "standard" client since we don't need to support updates on such resources, they should be immutable
			if err := client.Create(context.TODO(), tierTmpl); err != nil && !apierrors.IsAlreadyExists(err) {
				return errors.Wrapf(err, "unable to create the '%s' TierTemplate in namespace '%s'", tierTmpl.Name, tierTmpl.Namespace)
			}
			log.Info("TierTemplate resource created", "namespace", tierTmpl.Namespace, "name", tierTmpl.Name)
		}
	}
	// create the NSTemplateTiers
	nstmplTiersByTier, err := newNSTemplateTiers(namespace, tierTmplsByTier)
	if err != nil {
		return errors.Wrap(err, "unable to create or update NSTemplateTiers")
	}
	cl := commonclient.NewApplyClient(client, s)

	for _, nstmplTier := range nstmplTiersByTier {
		createdOrUpdated, err := cl.CreateOrUpdateObject(nstmplTier, true, nil)
		if err != nil {
			return errors.Wrapf(err, "unable to create or update the '%s' NSTemplateTiers in namespace '%s'", nstmplTier.Name, nstmplTier.Namespace)
		}
		if createdOrUpdated {
			log.Info("NSTemplateTier resource created/updated", "namespace", nstmplTier.Namespace, "name", nstmplTier.Name)
		} else {
			log.Info("NSTemplateTier resource was already up-to-date", "namespace", nstmplTier.Namespace, "name", nstmplTier.Name, "ResourceVersion", nstmplTier.ResourceVersion)
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
//
// The output is a map of `templates` indexed by tier.
// Each `templates` object contains itself a map of `template` objects indexed by the namespace type (`namespaceTemplates`)
// and an optional `template` for the cluster resources (`clusterTemplate`).
// Each `template` object contains a `revision` (`string`) and the `content` of the template to apply (`[]byte`)
func loadTemplatesByTiers(assets assets.Assets) (map[string]*templates, error) {
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
		// skip any name that does not have 2 parts
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
			return nil, errors.Errorf("unable to load templates: unknown scope for file '%s'", name)
		}
	}
	return results, nil
}

// newTierTemplates generates all TierTemplate resources, indexed by their tier
func newTierTemplates(s *runtime.Scheme, namespace string, assets assets.Assets) (map[string][]*toolchainv1alpha1.TierTemplate, error) {
	templatesByTier, err := loadTemplatesByTiers(assets)
	if err != nil {
		return nil, err
	}
	decoder := serializer.NewCodecFactory(s).UniversalDeserializer()
	result := map[string][]*toolchainv1alpha1.TierTemplate{}
	// proceed tiers by alphabetical order
	tiers := make([]string, 0, len(templatesByTier))
	for tier := range templatesByTier {
		tiers = append(tiers, tier)
	}
	sort.Strings(tiers)
	for _, tier := range tiers {
		tmpls := templatesByTier[tier]
		kinds := make([]string, 0, len(tmpls.namespaceTemplates))
		for kind := range tmpls.namespaceTemplates {
			kinds = append(kinds, kind)
		}
		sort.Strings(kinds)
		// namespace templates
		for _, kind := range kinds {
			tmpl := tmpls.namespaceTemplates[kind]
			tierTmpl, err := newTierTemplate(decoder, namespace, tier, kind, tmpl)
			if err != nil {
				return nil, err
			}
			if _, exists := result[tierTmpl.Spec.TierName]; !exists {
				result[tierTmpl.Spec.TierName] = []*toolchainv1alpha1.TierTemplate{}
			}
			result[tierTmpl.Spec.TierName] = append(result[tierTmpl.Spec.TierName], tierTmpl)
		}
		// cluster resources templates
		if tmpls.clusterTemplate != nil {
			tierTmpl, err := newTierTemplate(decoder, namespace, tier, toolchainv1alpha1.ClusterResourcesTemplateType, *tmpls.clusterTemplate)
			if err != nil {
				return nil, err
			}
			result[tierTmpl.Spec.TierName] = append(result[tierTmpl.Spec.TierName], tierTmpl)
		}
	}
	return result, nil
}

// newTierTemplate generates a TierTemplate resource for a given tier and kind
func newTierTemplate(decoder runtime.Decoder, namespace, tier, kind string, tmpl template) (*toolchainv1alpha1.TierTemplate, error) {
	name := NewTierTemplateName(tier, kind, tmpl.revision)
	tmplObj := &templatev1.Template{}
	_, _, err := decoder.Decode(tmpl.content, nil, tmplObj)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to generate '%s' TierTemplate manifest", name)
	}
	return &toolchainv1alpha1.TierTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name, // link to the TierTemplate resource, whose name is: `<tierName>-<nsType>-<revision>`,
		},
		Spec: toolchainv1alpha1.TierTemplateSpec{
			Revision: tmpl.revision,
			TierName: tier,
			Type:     kind,
			Template: *tmplObj,
		},
	}, nil
}

// NewTierTemplateName a utility func to generate a TierTemplate name, based on the given tier, kind and revision.
// note: the resource name must consist of lower case alphanumeric characters, '-' or '.', and must start and end with an alphanumeric character
func NewTierTemplateName(tier, kind, revision string) string {
	return strings.ToLower(fmt.Sprintf("%s-%s-%s", tier, kind, revision))
}

// newNSTemplateTiers generates all NSTemplateTier resources, indexed by their associated tier
func newNSTemplateTiers(namespace string, tierTmplsByTier map[string][]*toolchainv1alpha1.TierTemplate) (map[string]*toolchainv1alpha1.NSTemplateTier, error) {
	tiers := make(map[string]*toolchainv1alpha1.NSTemplateTier, len(tierTmplsByTier))
	for tier, tmpls := range tierTmplsByTier {
		tmpl, err := newNSTemplateTier(namespace, tier, tmpls)
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
func newNSTemplateTier(namespace, tier string, tierTmpls []*toolchainv1alpha1.TierTemplate) (*toolchainv1alpha1.NSTemplateTier, error) {
	// retrieve the namespace types and order them, so we can compare
	// with the expected templates during the tests
	result := &toolchainv1alpha1.NSTemplateTier{
		ObjectMeta: metav1.ObjectMeta{
			Name:      tier,
			Namespace: namespace,
		},
		Spec: toolchainv1alpha1.NSTemplateTierSpec{},
	}
	for _, tierTmpl := range tierTmpls {
		switch tierTmpl.Spec.Type {
		case toolchainv1alpha1.ClusterResourcesTemplateType:
			result.Spec.ClusterResources = &toolchainv1alpha1.NSTemplateTierClusterResources{
				TemplateRef: tierTmpl.Name,
			}
		default:
			// add it to the NSTemplateTier obj
			result.Spec.Namespaces = append(result.Spec.Namespaces, toolchainv1alpha1.NSTemplateTierNamespace{
				TemplateRef: tierTmpl.Name,
			})
		}
	}
	return result, nil
}
