package usertiers

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"reflect"
	"strings"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	commonclient "github.com/codeready-toolchain/toolchain-common/pkg/client"
	commonTemplate "github.com/codeready-toolchain/toolchain-common/pkg/template"

	templatev1 "github.com/openshift/api/template/v1"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/kubernetes/scheme"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const UserTierRootDir = "templates/usertiers"

var log = logf.Log.WithName("usertiers")

// CreateOrUpdateResources generates the UserTier resources,
// then uses the manager's client to create or update the resources on the cluster.
func CreateOrUpdateResources(ctx context.Context, s *runtime.Scheme, client runtimeclient.Client, namespace string, userTiersFS embed.FS, root string) error {

	// initialize tier generator, loads templates from assets
	generator, err := newUserTierGenerator(s, client, namespace, userTiersFS, root)
	if err != nil {
		return errors.Wrap(err, "unable to create UserTier generator")
	}

	// create the UserTier resources
	err = generator.createUserTiers(ctx)
	if err != nil {
		return errors.Wrap(err, "unable to create UserTiers")
	}

	return nil
}

type tierGenerator struct {
	client          runtimeclient.Client
	namespace       string
	scheme          *runtime.Scheme
	templatesByTier map[string]*tierData
}

type tierData struct {
	name         string
	rawTemplates *templates
	objects      []runtimeclient.Object
}

// templates: any resources belonging to a given user tier ("advanced", "base", "team", etc.), currently only a single template
type templates struct {
	userTier *template // UserTier resource with tier-scoped configuration
}

// template: a template's content and its latest git revision
type template struct {
	content []byte
}

// newUserTierGenerator loads templates from the provided assets and processes the UserTiers
func newUserTierGenerator(s *runtime.Scheme, client runtimeclient.Client, namespace string, userTiersFS embed.FS, root string) (*tierGenerator, error) {
	// load templates
	templatesByTier, err := loadTemplatesByTiers(userTiersFS, root)
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
// nodeactivation/
//
//	tier.yaml
//
// deactivate30/
//
//	tier.yaml
//
// deactivate80/
//
//	tier.yaml
//
// deactivate90/
//
//	tier.yaml
//
// deactivate180/
//
//	tier.yaml
//
// The output is a map of `tierData` indexed by tier.
// Each `tierData` object contains template objects for the tier.
// Each `template` object contains a `revision` (`string`) and the `content` of the template to apply (`[]byte`)
func loadTemplatesByTiers(userTierFS embed.FS, root string) (map[string]*tierData, error) {
	paths, err := getAllFileNames(&userTierFS, root)
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("could not find any user templates")
	}

	results := make(map[string]*tierData)
	for _, name := range paths {
		// split the name using the `/` separator
		parts := strings.Split(name, "/")
		// skip any name that does not have 2 parts
		if len(parts) != 4 {
			return nil, fmt.Errorf("unable to load templates: invalid name format for file '%s'", name)
		}
		tier := parts[2]
		filename := parts[3]
		if _, exists := results[tier]; !exists {
			results[tier] = &tierData{
				name:         tier,
				rawTemplates: &templates{},
			}
		}
		content, err := userTierFS.ReadFile(name)
		if err != nil {
			return nil, errors.Wrapf(err, "unable to load templates")
		}
		tmpl := template{
			content: content,
		}
		switch filename {
		case "tier.yaml":
			results[tier].rawTemplates.userTier = &tmpl
		default:
			return nil, errors.Errorf("unable to load templates: unknown scope for file '%s'", name)
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

// initUserTiers generates all UserTier resources and adds them to the tier map
func (t *tierGenerator) initUserTiers() error {

	for tierName, tierData := range t.templatesByTier {
		userTier := tierData.rawTemplates.userTier
		sourceTierName := tierName
		var parameters []templatev1.Parameter
		objs, err := t.newUserTier(sourceTierName, tierName, *userTier, parameters)
		if err != nil {
			return err
		}
		t.templatesByTier[tierName].objects = objs
	}

	return nil
}

// createUserTiers creates the UserTier resources from the tier map
func (t *tierGenerator) createUserTiers(ctx context.Context) error {
	applyCl := commonclient.NewApplyClient(t.client)

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

		updated, err := applyCl.ApplyObject(ctx, tier, commonclient.ForceUpdate(true))
		if err != nil {
			return errors.Wrapf(err, "unable to create or update the '%s' UserTier", tierName)
		}
		tierLog := log.WithValues("name", tierName)
		tierLog = tierLog.WithValues("DeactivationTimeoutDays", tier.Spec.DeactivationTimeoutDays)
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
//
//	metadata:
//	  name: base
//	spec:
//	  deactivationTimeoutDays: 30
//
// ------
func (t *tierGenerator) newUserTier(sourceTierName, tierName string, userTierTemplate template, parameters []templatev1.Parameter) ([]runtimeclient.Object, error) {
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

func getAllFileNames(notificationFS *embed.FS, root string) (files []string, err error) {

	if err := fs.WalkDir(notificationFS, root, func(path string, d fs.DirEntry, err error) error {
		if d.IsDir() {
			return nil
		}
		files = append(files, path)
		return nil
	}); err != nil {
		return nil, err
	}

	return files, nil
}
