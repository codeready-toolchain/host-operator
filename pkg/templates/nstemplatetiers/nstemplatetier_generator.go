package nstemplatetiers

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/templates/assets"
	commonclient "github.com/codeready-toolchain/toolchain-common/pkg/client"
	commonTemplate "github.com/codeready-toolchain/toolchain-common/pkg/template"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/kubectl/pkg/scheme"

	templatev1 "github.com/openshift/api/template/v1"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var log = logf.Log.WithName("templates")

// CreateOrUpdateResources generates the NSTemplateTier resources from the cluster resource template and namespace templates,
// then uses the manager's client to create or update the resources on the cluster.
func CreateOrUpdateResources(s *runtime.Scheme, client client.Client, namespace string, assets assets.Assets) error {

	// initialize tier generator, loads templates from assets
	generator, err := newTierGenerator(s, client, namespace, assets)
	if err != nil {
		return errors.Wrap(err, "unable to create TierTemplates")
	}

	// create the TierTemplate resources
	err = generator.createTierTemplates()
	if err != nil {
		return errors.Wrap(err, "unable to create TierTemplates")
	}

	// create the NSTemplateTier resources
	err = generator.createNSTemplateTiers()
	if err != nil {
		return errors.Wrap(err, "unable to create NSTemplateTiers")
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
	name           string
	rawTemplates   *templates
	tierTemplates  []*toolchainv1alpha1.TierTemplate
	nstmplTierObjs []commonclient.ToolchainObject
	tierConfig     *TierConfig
}

// templates: namespaces and other cluster-scoped resources belonging to a given tier ("advanced", "basic", "team", etc.) and the NSTemplateTier that combines them
type templates struct {
	namespaceTemplates map[string]template // namespace templates (including roles, etc.) indexed by type ("dev", "code", "stage")
	clusterTemplate    *template           // other cluster-scoped resources, in a single template file
	nsTemplateTier     template            // NSTemplateTier resource with tier-scoped configuration and references to namespace and cluster templates in its spec, in a single template file
	tierConfig         template
}

// template: a template's content and its latest git revision
type template struct {
	revision string
	content  []byte
}

// newTierGenerator loads templates from the provided assets and processes the tierTemplates and NSTemplateTiers
func newTierGenerator(s *runtime.Scheme, client client.Client, namespace string, assets assets.Assets) (*tierGenerator, error) {
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

	// process tierTemplates
	if err := c.initTierTemplates(); err != nil {
		return nil, err
	}

	// process NSTemplateTiers
	if err := c.initNSTemplateTiers(); err != nil {
		return nil, err
	}

	return c, nil
}

//from:
//  name: base
//to:
//  name: baseextendedidling
//  replace:
//    idlerTimeoutSeconds: 43200
type TierConfig struct {
	Revision string
	From     From `json:"from"`
	To       To   `json:"to"`
}

type From struct {
	Name string `json:"name"`
}

type To struct {
	Name       string                 `json:"name"`
	Parameters []templatev1.Parameter `json:"parameters,omitempty" protobuf:"bytes,4,rep,name=parameters"`
}

// loadTemplatesByTiers loads the assets and dispatches them by tiers, assuming the given `assets` has the following structure:
//
// metadata.yaml
// advanced/
//   cluster.yaml
//   ns_code.yaml
//   ns_xyz.yaml
//   tier.yaml
// basic/
//   cluster.yaml
//   ns_code.yaml
//   ns_xyz.yaml
//   tier.yaml
// team/
//   cluster.yaml
//   ns_xyz.yaml
//   tier.yaml
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
		case strings.HasPrefix(filename, "ns_"):
			kind := strings.TrimSuffix(strings.TrimPrefix(filename, "ns_"), ".yaml")
			results[tier].rawTemplates.namespaceTemplates[kind] = tmpl
		case filename == "cluster.yaml":
			results[tier].rawTemplates.clusterTemplate = &tmpl
		case filename == "tier.yaml":
			results[tier].rawTemplates.nsTemplateTier = tmpl
		case filename == "tier-config.yaml":
			tierConfig := &TierConfig{}
			if err := yaml.Unmarshal(content, tierConfig); err != nil {
				return nil, err
			}
			results[tier].rawTemplates.tierConfig = tmpl
			results[tier].tierConfig = tierConfig
		default:
			return nil, errors.Errorf("unable to load templates: unknown scope for file '%s'", name)
		}
	}

	return results, nil
}

// initTierTemplates generates all TierTemplate resources, and adds them to the tier map indexed by tier name
func (t *tierGenerator) initTierTemplates() error {

	// process tiers in alphabetical order
	tiers := make([]string, 0, len(t.templatesByTier))
	for tier := range t.templatesByTier {
		tiers = append(tiers, tier)
	}
	sort.Strings(tiers)
	for _, tier := range tiers {

		tierData := t.templatesByTier[tier]
		tierConfigRevision := ""
		var parameters []templatev1.Parameter
		if tierData.tierConfig != nil {
			parameters = tierData.tierConfig.To.Parameters
			tierData = t.templatesByTier[tierData.tierConfig.From.Name]
			tierConfigRevision = tierData.rawTemplates.tierConfig.revision
		}
		tierTemplates, err := t.newTierTemplates(tierConfigRevision, tierData, tier, parameters)
		if err != nil {
			return err
		}
		t.templatesByTier[tier].tierTemplates = tierTemplates
	}

	return nil
}

func (t *tierGenerator) newTierTemplates(tierConfigRevision string, tierData *tierData, tier string, parameters []templatev1.Parameter) ([]*toolchainv1alpha1.TierTemplate, error) {
	decoder := serializer.NewCodecFactory(t.scheme).UniversalDeserializer()

	// namespace templates
	kinds := make([]string, 0, len(tierData.rawTemplates.namespaceTemplates))
	for kind := range tierData.rawTemplates.namespaceTemplates {
		kinds = append(kinds, kind)
	}

	tierTmpls := []*toolchainv1alpha1.TierTemplate{}
	sort.Strings(kinds)
	for _, kind := range kinds {
		tmpl := tierData.rawTemplates.namespaceTemplates[kind]
		tierTmpl, err := t.newTierTemplate(decoder, tierConfigRevision, tier, kind, tmpl, parameters)
		if err != nil {
			return nil, err
		}
		//tierTmpl.Name = strings.ReplaceAll(tierTmpl.Name, tierData.name, tier)
		tierTmpls = append(tierTmpls, tierTmpl)
	}
	// cluster resources templates
	if tierData.rawTemplates.clusterTemplate != nil {
		tierTmpl, err := t.newTierTemplate(decoder, tierConfigRevision, tier, toolchainv1alpha1.ClusterResourcesTemplateType, *tierData.rawTemplates.clusterTemplate, parameters)
		if err != nil {
			return nil, err
		}
		//tierTmpl.Name = strings.ReplaceAll(tierTmpl.Name, tierData.name, tier)
		tierTmpls = append(tierTmpls, tierTmpl)
	}
	return tierTmpls, nil
}

// createTierTemplates creates all TierTemplate resources from the tier map
func (t *tierGenerator) createTierTemplates() error {

	// create the templates
	for _, tierTmpls := range t.templatesByTier {
		for _, tierTmpl := range tierTmpls.tierTemplates {
			// using the "standard" client since we don't need to support updates on such resources, they should be immutable
			if err := t.client.Create(context.TODO(), tierTmpl); err != nil && !apierrors.IsAlreadyExists(err) {
				return errors.Wrapf(err, "unable to create the '%s' TierTemplate in namespace '%s'", tierTmpl.Name, tierTmpl.Namespace)
			}
			log.Info("TierTemplate resource created", "namespace", tierTmpl.Namespace, "name", tierTmpl.Name)
		}
	}
	return nil
}

// newTierTemplate generates a TierTemplate resource for a given tier and kind
func (t *tierGenerator) newTierTemplate(decoder runtime.Decoder, tierConfigRevision, tier, kind string, tmpl template, parameters []templatev1.Parameter) (*toolchainv1alpha1.TierTemplate, error) {
	if tierConfigRevision == "" {
		tierConfigRevision = tmpl.revision
	}
	revision := fmt.Sprintf("%s-%s", tmpl.revision, tierConfigRevision)
	name := NewTierTemplateName(tier, kind, revision)
	tmplObj := &templatev1.Template{}
	_, _, err := decoder.Decode(tmpl.content, nil, tmplObj)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to generate '%s' TierTemplate manifest", name)
	}
	setParams(parameters, tmplObj)

	return &toolchainv1alpha1.TierTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: t.namespace,
			Name:      name, // link to the TierTemplate resource, whose name is: `<tierName>-<nsType>-<revision>`,
		},
		Spec: toolchainv1alpha1.TierTemplateSpec{
			Revision: revision,
			TierName: tier,
			Type:     kind,
			Template: *tmplObj,
		},
	}, nil
}

func setParams(parametersToSet []templatev1.Parameter, tmpl *templatev1.Template) {
	for _, paramToSet := range parametersToSet {
		for i, param := range tmpl.Parameters {
			if param.Name == paramToSet.Name {
				tmpl.Parameters[i].Value = paramToSet.Value
				break
			}
			tmpl.Parameters = append(tmpl.Parameters, paramToSet)
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
		if tierData.tierConfig != nil {
			parameters = tierData.tierConfig.To.Parameters
			fromData := t.templatesByTier[tierData.tierConfig.From.Name]
			nsTemplateTier = fromData.rawTemplates.nsTemplateTier
			sourceTierName = fromData.name
		}
		tmpl, err := t.newNSTemplateTier(sourceTierName, tierName, nsTemplateTier, tierTemplates, parameters)
		if err != nil {
			return err
		}
		t.templatesByTier[tierName].nstmplTierObjs = tmpl
	}

	return nil
}

// createNSTemplateTiers creates the NSTemplateTier resources from the tier map
func (t *tierGenerator) createNSTemplateTiers() error {
	applyCl := commonclient.NewApplyClient(t.client, t.scheme)

	for tierName, tierData := range t.templatesByTier {
		if len(tierData.nstmplTierObjs) != 1 {
			return fmt.Errorf("there is an unexpected number of NSTemplateTier object to be applied for tier name '%s'; expected: 1; actual: %d", tierName, len(tierData.nstmplTierObjs))
		}

		unstructuredObj, ok := tierData.nstmplTierObjs[0].GetClientObject().(*unstructured.Unstructured)
		if !ok {
			return fmt.Errorf("unable to cast NSTemplateTier '%s' to Unstructured object '%+v'", tierName, tierData.nstmplTierObjs[0].GetClientObject())
		}
		tier := &toolchainv1alpha1.NSTemplateTier{}
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
			return errors.Wrapf(err, "unable to create or update the '%s' NSTemplateTier", tierName)
		}
		tierLog := log.WithValues("name", tierName)
		for i, nsTemplate := range tier.Spec.Namespaces {
			tierLog = tierLog.WithValues(fmt.Sprintf("namespaceTemplate-%d", i), nsTemplate.TemplateRef)
		}
		if tier.Spec.ClusterResources != nil {
			tierLog = tierLog.WithValues("clusterResourcesTemplate", tier.Spec.ClusterResources.TemplateRef)
		}
		if updated {
			tierLog.Info("NSTemplateTier was either updated or created")
		} else {
			tierLog.Info("NSTemplateTier wasn't updated nor created - the expected spec was already there")
		}
	}
	return nil
}

// NewNSTemplateTier generates a complete NSTemplateTier object via Openshift Template based on the contents of tier.yaml and
// by embedding the `<tier>-code.yaml`, `<tier>-dev.yaml` and `<tier>-stage.yaml` and cluster.yaml references.
//
// After processing the Openshift Template the NSTemplateTier should look something like:
// ------
// kind: NSTemplateTier
//   metadata:
//     name: basic
//   spec:
//     clusterResources:
//       templateRef: basic-clusterresources-07cac69
//     namespaces:
//     - templateRef: basic-code-cb6fbd2
//     - templateRef: basic-dev-4d49fe0
//     - templateRef: basic-stage-4d49fe0
// ------
func (t *tierGenerator) newNSTemplateTier(sourceTierName, tierName string, nsTemplateTier template, tierTemplates []*toolchainv1alpha1.TierTemplate, parameters []templatev1.Parameter) ([]commonclient.ToolchainObject, error) {
	decoder := serializer.NewCodecFactory(scheme.Scheme).UniversalDeserializer()
	if reflect.DeepEqual(nsTemplateTier, template{}) {
		return nil, fmt.Errorf("tier %s is missing a tier.yaml file", tierName)
	}

	tmplObj := &templatev1.Template{}
	_, _, err := decoder.Decode(nsTemplateTier.content, nil, tmplObj)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to generate '%s' NSTemplateTier manifest", tierName)
	}

	tmplProcessor := commonTemplate.NewProcessor(t.scheme)
	params := map[string]string{"NAMESPACE": t.namespace}

	for _, tierTmpl := range tierTemplates {
		switch tierTmpl.Spec.Type {
		// ClusterResources
		case toolchainv1alpha1.ClusterResourcesTemplateType:
			params["CLUSTER_TEMPL_REF"] = tierTmpl.Name
		// Namespaces
		default:
			tmplType := strings.ToUpper(tierTmpl.Spec.Type) // code, dev, stage
			key := tmplType + "_TEMPL_REF"                  // eg. CODE_TEMPL_REF
			params[key] = tierTmpl.Name
		}
	}
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
