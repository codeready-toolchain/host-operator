package nstemplatetiers

import (
	"fmt"
	"reflect"
	"strings"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/templates/assets"
	commonclient "github.com/codeready-toolchain/toolchain-common/pkg/client"
	commonTemplate "github.com/codeready-toolchain/toolchain-common/pkg/template"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes/scheme"

	templatev1 "github.com/openshift/api/template/v1"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var log = logf.Log.WithName("usertiers")

// CreateOrUpdateResources generates the UserTier resources,
// then uses the manager's client to create or update the resources on the cluster.
func CreateOrUpdateResources(s *runtime.Scheme, client client.Client, namespace string, assets assets.Assets) error {

	// initialize tier generator, loads templates from assets
	generator, err := newUserTierGenerator(s, client, namespace, assets)
	if err != nil {
		return errors.Wrap(err, "unable to create UserTier generator")
	}

	// create the UserTier resources
	err = generator.createUserTiers()
	if err != nil {
		return errors.Wrap(err, "unable to create UserTiers")
	}

	return nil
}

type tierGenerator struct {
	client          client.Client
	namespace       string
	scheme          *runtime.Scheme
	templatesByTier map[string]*tierData
}

type tierData struct {
	name    string
	objects []client.Object
}

// templates: namespaces and other cluster-scoped resources belonging to a given tier ("advanced", "base", "team", etc.) and the NSTemplateTier that combines them
type templates struct {
	nsTemplateTier     *template           // NSTemplateTier resource with tier-scoped configuration and references to namespace and cluster templates in its spec, in a single template file
	clusterTemplate    *template           // other cluster-scoped resources, in a single template file
	namespaceTemplates map[string]template // namespace templates (including roles, limits, etc.) indexed by type ("dev", "stage")
	spaceroleTemplates map[string]template // spacerole templates (including rolebindings, etc.) indexed by role ("admin", "viewer", etc.)
	basedOnTier        *template           // a special config defining which tier should be reused and which parameters should be overridden
}

// template: a template's content and its latest git revision
type template struct {
	revision string
	content  []byte
}

// newUserTierGenerator loads templates from the provided assets and processes the UserTiers
func newUserTierGenerator(s *runtime.Scheme, client client.Client, namespace string, assets assets.Assets) (*tierGenerator, error) {
	// load templates from assets
	templatesByTier, err := loadTemplatesByTiers(assets)
	if err != nil {
		return nil, err
	}

	c := &tierGenerator{
		client:          client,
		namespace:       namespace,
		scheme:          s,
		templatesByTier: templatesByTier,
	}

	// process UserTiers
	if err := c.initUserTiers(); err != nil {
		return nil, err
	}

	return c, nil
}

// loadTemplatesByTiers loads the assets and dispatches them by tiers, assuming the given `assets` has the following structure:
//
// metadata.yaml
// advanced/
//   based_on_tier.yaml
// basic/
//   cluster.yaml
//   ns_dev.yaml
//   ns_stage.yaml
//   spacerole_admin.yaml
//   tier.yaml
// team/
//   based_on_tier.yaml
//
// The output is a map of `tierData` indexed by tier.
// Each `tierData` object contains itself a map of `template` objects indexed by the namespace type (`namespaceTemplates`);
// an optional `template` for the cluster resources (`clusterTemplate`) and the NSTemplateTier resource object.
// Each `template` object contains a `revision` (`string`) and the `content` of the template to apply (`[]byte`)
func loadTemplatesByTiers(assets assets.Assets) (map[string]*tierData, error) {
	metadataContent, err := assets.Asset("metadata.yaml")
	if err != nil {
		return nil, errors.Wrapf(err, "unable to load templates")
	}
	metadata := make(map[string]string)
	err = yaml.Unmarshal([]byte(metadataContent), &metadata)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to load templates")
	}

	results := make(map[string]*tierData)
	for _, name := range assets.Names() {
		if name == "metadata.yaml" {
			continue
		}
		// split the name using the `/` separator
		parts := strings.Split(name, "/")
		// skip any name that does not have 2 parts
		if len(parts) != 2 {
			return nil, fmt.Errorf("unable to load templates: invalid name format for file '%s'", name)
		}
		tier := parts[0]
		filename := parts[1]
		if _, exists := results[tier]; !exists {
			results[tier] = &tierData{
				name: tier,
				rawTemplates: &templates{
					namespaceTemplates: map[string]template{},
					spaceroleTemplates: map[string]template{},
				},
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
		case filename == "tier.yaml":
			results[tier].rawTemplates.nsTemplateTier = &tmpl
		case filename == "cluster.yaml":
			results[tier].rawTemplates.clusterTemplate = &tmpl
		case strings.HasPrefix(filename, "ns_"):
			kind := strings.TrimSuffix(strings.TrimPrefix(filename, "ns_"), ".yaml")
			results[tier].rawTemplates.namespaceTemplates[kind] = tmpl
		case strings.HasPrefix(filename, "spacerole_"):
			role := strings.TrimSuffix(strings.TrimPrefix(filename, "spacerole_"), ".yaml")
			results[tier].rawTemplates.spaceroleTemplates[role] = tmpl
		case filename == "based_on_tier.yaml":
			basedOnTier := &BasedOnTier{}
			if err := yaml.Unmarshal(content, basedOnTier); err != nil {
				return nil, errors.Wrapf(err, "unable to unmarshal '%s'", name)
			}
			results[tier].rawTemplates.basedOnTier = &tmpl
			results[tier].basedOnTier = basedOnTier
		default:
			return nil, errors.Errorf("unable to load templates: unknown scope for file '%s'", name)
		}
	}

	// check that none of the tiers uses combination of based_on_tier.yaml file together with any template file
	for tier, tierData := range results {
		if tierData.rawTemplates.basedOnTier != nil &&
			(tierData.rawTemplates.clusterTemplate != nil ||
				len(tierData.rawTemplates.namespaceTemplates) > 0 ||
				tierData.rawTemplates.nsTemplateTier != nil) {
			return nil, fmt.Errorf("the tier %s contains a mix of based_on_tier.yaml file together with a regular template file", tier)
		}
	}
	return results, nil
}

// setParams sets the value for each of the keys in the given parameter set to the template, but only if the key exists there
func setParams(parametersToSet []templatev1.Parameter, tmpl *templatev1.Template) {
	for _, paramToSet := range parametersToSet {
		for i, param := range tmpl.Parameters {
			if param.Name == paramToSet.Name {
				tmpl.Parameters[i].Value = paramToSet.Value
				break
			}
		}
	}
}

// NewTierTemplateName a utility func to generate a TierTemplate name, based on the given tier, kind and revision.
// note: the resource name must consist of lower case alphanumeric characters, '-' or '.', and must start and end with an alphanumeric character
func NewTierTemplateName(tier, kind, revision string) string {
	return strings.ToLower(fmt.Sprintf("%s-%s-%s", tier, kind, revision))
}

// newNSTemplateTiers generates all NSTemplateTier resources and adds them to the tier map
func (t *tierGenerator) initNSTemplateTiers() error {

	for tierName, tierData := range t.templatesByTier {
		nsTemplateTier := tierData.rawTemplates.nsTemplateTier
		tierTemplates := tierData.tierTemplates
		sourceTierName := tierName
		var parameters []templatev1.Parameter
		if tierData.basedOnTier != nil {
			parameters = tierData.basedOnTier.Parameters
			fromData := t.templatesByTier[tierData.basedOnTier.From]
			nsTemplateTier = fromData.rawTemplates.nsTemplateTier
			sourceTierName = fromData.name
		}
		objs, err := t.newNSTemplateTier(sourceTierName, tierName, *nsTemplateTier, tierTemplates, parameters)
		if err != nil {
			return err
		}
		t.templatesByTier[tierName].objects = objs
	}

	return nil
}

// createUserTiers creates the UserTier resources from the tier map
func (t *tierGenerator) createUserTiers() error {
	applyCl := commonclient.NewApplyClient(t.client, t.scheme)

	for tierName, tierData := range t.templatesByTier {
		if len(tierData.objects) != 1 {
			return fmt.Errorf("there is an unexpected number of UserTier object to be applied for tier name '%s'; expected: 1; actual: %d", tierName, len(tierData.objects))
		}

		unstructuredObj, ok := tierData.objects[0].(*unstructured.Unstructured)
		if !ok {
			return fmt.Errorf("unable to cast UserTier '%s' to Unstructured object '%+v'", tierName, tierData.objects[0])
		}
		tier := &toolchainv1alpha1.UserTier{}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredObj.Object, tier); err != nil {
			return err
		}

		labels := tier.GetLabels()
		if labels == nil {
			labels = make(map[string]string)
		}
		labels[toolchainv1alpha1.ProviderLabelKey] = toolchainv1alpha1.ProviderLabelValue

		updated, err := applyCl.ApplyObject(tier, commonclient.ForceUpdate(true))
		if err != nil {
			return errors.Wrapf(err, "unable to create or update the '%s' UserTier", tierName)
		}
		tierLog := log.WithValues("name", tierName)
		for i, nsTemplate := range tier.Spec.Namespaces {
			tierLog = tierLog.WithValues(fmt.Sprintf("namespaceTemplate-%d", i), nsTemplate.TemplateRef)
		}
		if tier.Spec.ClusterResources != nil {
			tierLog = tierLog.WithValues("clusterResourcesTemplate", tier.Spec.ClusterResources.TemplateRef)
		}
		if updated {
			tierLog.Info("UserTier was either updated or created")
		} else {
			tierLog.Info("UserTier wasn't updated nor created: the spec was already set as expected")
		}
	}
	return nil
}

// newUserTier generates a complete UserTier object via Openshift Template based on the contents of tier.yaml.
//
// After processing the Openshift Template the UserTier should look something like:
// ------
// kind: UserTier
//   metadata:
//     name: base
//   spec:
//     deactivationTimeoutDays: 30
// ------
func (t *tierGenerator) newUserTier(sourceTierName, tierName string, userTierTemplate template, parameters []templatev1.Parameter) ([]client.Object, error) {
	decoder := serializer.NewCodecFactory(scheme.Scheme).UniversalDeserializer()
	if reflect.DeepEqual(userTierTemplate, template{}) {
		return nil, fmt.Errorf("tier %s is missing a tier.yaml file", tierName)
	}

	tmplObj := &templatev1.Template{}
	_, _, err := decoder.Decode(userTierTemplate.content, nil, tmplObj)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to generate '%s' UserTier manifest", tierName)
	}

	tmplProcessor := commonTemplate.NewProcessor(t.scheme)
	params := map[string]string{"NAMESPACE": t.namespace}

	setParams(parameters, tmplObj)
	toolchainObjects, err := tmplProcessor.Process(tmplObj.DeepCopy(), params)
	if err != nil {
		return nil, err
	}
	for i := range toolchainObjects {
		toolchainObjects[i].SetName(strings.Replace(toolchainObjects[i].GetName(), sourceTierName, tierName, 1))
	}
	return toolchainObjects, nil
}
