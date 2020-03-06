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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var log = logf.Log.WithName("templates")

// CreateOrUpdateResources generates the NSTemplateTier resources from the namespace templates,
// then uses the manager's client to create or update the resources on the cluster.
func CreateOrUpdateResources(s *runtime.Scheme, client client.Client, namespace string, namespaceAssets, clusterResourceQuotaAssets assetFunc) error {
	g, err := newNSTemplateTierGenerator(s, namespaceAssets, clusterResourceQuotaAssets)
	if err != nil {
		return errors.Wrap(err, "unable to create or update NSTemplateTiers")
	}
	tierObjs, err := g.newNSTemplateTiers(namespace)
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
		if err := client.Create(context.TODO(), tierObj); err != nil {
			if !apierrors.IsAlreadyExists(err) {
				return errors.Wrapf(err, "unable to create the '%s' NSTemplateTiers in namespace '%s'", tierObj.Name, tierObj.Namespace)
			}
			log.Info("NSTemplateTier resource already exists", "namespace", tierObj.Namespace, "name", tierObj.Name)
			// get the existing NSTemplateTier
			existing := &toolchainv1alpha1.NSTemplateTier{}
			err = client.Get(context.TODO(), types.NamespacedName{
				Namespace: tierObj.Namespace,
				Name:      tierObj.Name,
			}, existing)
			if err != nil {
				return errors.Wrapf(err, "unable to get the '%s' NSTemplateTiers in namespace '%s'", tierObj.Name, tierObj.Namespace)
			}
			// retrieve the current 'resourceVersion' to set it in the resource passed to the `client.Update()`
			// otherwise we would get an error with the following message:
			// "nstemplatetiers.toolchain.dev.openshift.com \"basic\" is invalid: metadata.resourceVersion: Invalid value: 0x0: must be specified for an update"
			tierObj.ObjectMeta.ResourceVersion = existing.ObjectMeta.ResourceVersion
			if err := client.Update(context.TODO(), tierObj); err != nil {
				return errors.Wrapf(err, "unable to update the '%s' NSTemplateTiers in namespace '%s'", tierObj.Name, tierObj.Namespace)
			}
			log.Info("NSTemplateTier resource updated", "namespace", tierObj.Namespace, "name", tierObj.Name, "ResourceVersion", tierObj.ResourceVersion)
		} else {
			log.Info("NSTemplateTier resource created", "namespace", tierObj.Namespace, "name", tierObj.Name)
		}
	}
	return nil
}

// an alias for the func to get an asset
type assetFunc func(name string) ([]byte, error)

// nstemplatetierGenerator the NSTemplateTier manifest generator
type nstemplatetierGenerator struct {
	namespaces       map[string]map[string]asset // namespace template+revision indexed by their tier ('advanced', 'basic') and type ('code', 'dev', 'stage')
	clusterResources map[string]asset            // cluster resource quota template+revision indexed by their tier ('advanced', 'basic')
	decoder          runtime.Decoder
}

type asset struct {
	revision string
	content  []byte
}

// newNSTemplateTierGenerator returns a new nstemplatetierGenerator
func newNSTemplateTierGenerator(s *runtime.Scheme, namespaceAssets, clusterResourceQuotaAssets assetFunc) (*nstemplatetierGenerator, error) {
	namespaces, err := loadNamespaceAssets(namespaceAssets)
	if err != nil {
		return nil, err
	}
	clusterResources, err := loadClusterResourceQuotaAssets(clusterResourceQuotaAssets)
	if err != nil {
		return nil, err
	}
	return &nstemplatetierGenerator{
		namespaces:       namespaces,
		clusterResources: clusterResources,
		decoder:          serializer.NewCodecFactory(s).UniversalDeserializer(),
	}, nil
}

// loadNamespaceAssets returns a "supermap" in which:
// - each key is a tier (eg: "basic", "advanced", etc.)
// - each value is map of assets indexed by their type, i.e., template contents ([]byte) and their revision (string).
// Eg:
//   {
//		"code": asset{
//			revision:"123456",
//			content: []byte{...},
//		},
//		"dev": asset {
//			revision: "abcdef",
//			content: []byte{...},
//		},
//		...
// 	}
func loadNamespaceAssets(get assetFunc) (map[string]map[string]asset, error) {
	metadata, err := get("metadata.yaml")
	if err != nil {
		return nil, errors.Wrapf(err, "unable to load namespace templates")
	}
	data := make(map[string]string)
	err = yaml.Unmarshal([]byte(metadata), &data)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to load namespace templates")
	}
	// reads all entries in the YAML, splitting the keys
	// as we expect the following format: '<tier_kind>-<namespace_kind>' (eg: 'advanced-code')
	revisionsByTier := make(map[string]map[string]string)
	for filename, revision := range data {
		data := strings.Split(filename, "-")
		if len(data) != 2 {
			return nil, errors.Errorf("invalid namespace template filename. Expected format: '<tier_kind>-<namespace_kind>', got %s", filename)
		}
		tier := data[0] // 'advanced', 'basic', etc.
		typ := data[1]  // 'code', 'stage', etc.
		if _, ok := revisionsByTier[tier]; !ok {
			revisionsByTier[tier] = make(map[string]string, 3) // expect 3 entries: 'code', 'dev' and 'stage', but map size will increase if needed
		}
		revisionsByTier[tier][typ] = revision
	}
	// now, load the associated template contents
	result := make(map[string]map[string]asset, len(revisionsByTier))
	for tier, revisionsByKind := range revisionsByTier {
		// iterate on each type (eg: 'code', 'stage', etc.)
		for typ, revision := range revisionsByKind {
			// get the associated template
			content, err := get(fmt.Sprintf("%s-%s.yaml", tier, typ))
			if err != nil {
				return nil, errors.Wrapf(err, "unable to load namespace templates")
			}
			if _, exists := result[tier]; !exists {
				result[tier] = make(map[string]asset, len(revisionsByKind))
			}
			result[tier][typ] = asset{
				revision: revision,
				content:  content,
			}
		}
	}
	return result, nil
}

// loadClusterResourceQuotaAssets returns a map in which:
// - each key is a tier type (eg: "basic", "advanced", etc.)
// - each value is an "asset", i.e., a template content ([]byte) and its revision (string)
// Eg:
// asset {
//		revision: "abcdef",
//		content: []byte{...},
//	}
func loadClusterResourceQuotaAssets(get assetFunc) (map[string]asset, error) {
	// get the revisions
	metadata, err := get("metadata.yaml")
	if err != nil {
		return nil, errors.Wrapf(err, "unable to load cluster resource quota templates")
	}
	revisions := make(map[string]string)
	err = yaml.Unmarshal([]byte(metadata), &revisions)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to load cluster resource quota templates")
	}
	result := make(map[string]asset, len(revisions))
	// now, get the templates associated with the revisions
	for tier, revision := range revisions {
		content, err := get(fmt.Sprintf("%s.yaml", tier))
		if err != nil {
			return nil, errors.Wrapf(err, "unable to load cluster resource quota templates")
		}
		result[tier] = asset{
			revision: revision,
			content:  content,
		}
	}
	log.Info("cluster resource quota templates loaded")
	return result, nil
}

// NewNSTemplateTiers generates all manifests, indexed by their associated tier
func (g nstemplatetierGenerator) newNSTemplateTiers(namespace string) (map[string]*toolchainv1alpha1.NSTemplateTier, error) {
	tiers := make(map[string]*toolchainv1alpha1.NSTemplateTier, len(g.namespaces))
	for tier := range g.namespaces {
		tmpl, err := g.newNSTemplateTier(tier, namespace)
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
// ------
func (g nstemplatetierGenerator) newNSTemplateTier(tier, namespace string) (*toolchainv1alpha1.NSTemplateTier, error) {
	namespaceAssetsByType, ok := g.namespaces[tier]
	if !ok {
		return nil, errors.Errorf("tier '%s' does not exist", tier)
	}
	// retrieve the namespace types and order them, so we can order them, and compare
	// with the expected templates during the tests
	namespaceTypes := make([]string, 0, len(namespaceAssetsByType))
	for namespaceType := range namespaceAssetsByType {
		namespaceTypes = append(namespaceTypes, namespaceType)
	}
	sort.Strings(namespaceTypes)
	result := &toolchainv1alpha1.NSTemplateTier{
		ObjectMeta: metav1.ObjectMeta{
			Name:      tier,
			Namespace: namespace,
		},
		Spec: toolchainv1alpha1.NSTemplateTierSpec{},
	}
	for _, typ := range namespaceTypes {
		// get the content of the `-<t>.yaml` file
		tmpl := g.namespaces[tier][typ].content
		// convert the content into a templatev1.Template
		tmplObj := &templatev1.Template{}
		_, _, err := g.decoder.Decode(tmpl, nil, tmplObj)
		if err != nil {
			return nil, errors.Wrapf(err, "unable to generate '%s' NSTemplateTier manifest", tier)
		}
		// add it to the NSTemplateTier obj
		result.Spec.Namespaces = append(result.Spec.Namespaces, toolchainv1alpha1.NSTemplateTierNamespace{
			Type:     typ,
			Revision: g.namespaces[tier][typ].revision,
			Template: *tmplObj,
		})
	}
	// also, add the cluster resource quota template+revision if it exists
	if crqAsset, exists := g.clusterResources[tier]; exists {
		// get the content of the `-<t>.yaml` file
		tmpl := crqAsset.content
		// convert the content into a templatev1.Template
		tmplObj := &templatev1.Template{}
		_, _, err := g.decoder.Decode(tmpl, nil, tmplObj)
		if err != nil {
			return nil, errors.Wrapf(err, "unable to generate '%s' NSTemplateTier manifest", tier)
		}
		result.Spec.ClusterResources = toolchainv1alpha1.NSTemplateTierClusterResources{
			Revision: crqAsset.revision,
			Template: *tmplObj,
		}
	}
	return result, nil
}
