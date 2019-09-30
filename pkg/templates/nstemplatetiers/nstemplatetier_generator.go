package nstemplatetiers

import (
	"bufio"
	"bytes"
	"fmt"
	"sort"
	"strings"
	texttemplate "text/template"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var nstemplatetierTmpl *texttemplate.Template
var initErr error

var log = logf.Log.WithName("templates")

func init() {
	nstemplatetierTmpl, initErr = texttemplate.New("NSTemplateTier").
		Funcs(texttemplate.FuncMap{
			"indent": indent,
		}).
		Parse(`kind: NSTemplateTier
apiVersion: toolchain.dev.openshift.com/v1alpha1
metadata:
  name: {{ .Name }}
spec:
  namespaces:{{ range $index, $ns := .Namespaces }}
  - type: {{ .Type }}
    revision: {{ .Revision }}
    template:
{{ .Template }}{{ end }}`)
	if initErr != nil {
		log.Error(initErr, "failed to initialize NSTemplateTier template")
	}
}

// NSTemplateTierGenerator the NSTemplateTier manifest generator
type NSTemplateTierGenerator struct {
	asset     func(name string) ([]byte, error) // the func which gives access to the
	revisions map[string]map[string]string
}

// NewNSTemplateTierGenerator returns a new NSTemplateTierGenerator
func NewNSTemplateTierGenerator(asset func(name string) ([]byte, error)) (*NSTemplateTierGenerator, error) {
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
		return nil, err
	}
	// reads all entries in the YAML, splitting the keys
	// as we expect the following format: '<tier_kind>-<namespace_kind>' (eg: 'advanced-code')
	revisions := make(map[string]map[string]string)
	for filename, revision := range data {
		data := strings.Split(filename, "-")
		if len(data) != 2 {
			log.Info("invalid namespace template filename. Expected format: '<tier_kind>-<namespace_kind>'", "filename", filename)
			continue
		}
		tierKind := data[0]
		nsKind := data[1]
		// create a new entry if needed
		if _, ok := revisions[tierKind]; !ok {
			revisions[tierKind] = make(map[string]string, 3) // expect 3 entries: 'code', 'dev' and 'stage'
		}
		revision, ok := revision.(string)
		if !ok {
			log.Info("invalid namespace template filename revision. Expected a string", "filename", filename, "revision", revision)
			continue
		}
		revisions[tierKind][nsKind] = revision
	}
	log.Info("templates revisions loaded", "revisions", revisions)
	return revisions, nil
}

// GenerateManifest generates a full NSTemplateTier manifest
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
func (g NSTemplateTierGenerator) GenerateManifest(tier string) ([]byte, error) {
	if nstemplatetierTmpl == nil {
		return nil, errors.Wrapf(initErr, "Cannot generate the '%s' NSTemplateTier manifest", tier)
	}
	revisions, ok := g.revisions[tier]
	if !ok {
		return nil, errors.Errorf("tier '%s' does not exist", tier)
	}
	type namespace struct {
		Type     string
		Revision string
		Template string
	}
	namespaces := make([]namespace, 0, len(revisions))
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
		tmpl, err = indent(tmpl, 6)
		if err != nil {
			return nil, errors.Wrapf(err, "unable to generate '%s' NSTemplateTier manifest", tier)
		}
		namespaces = append(namespaces, namespace{
			Type:     nsType,
			Revision: revisions[nsType],
			Template: string(tmpl),
		})
	}
	result := bytes.NewBuffer(nil)
	err := nstemplatetierTmpl.Execute(result, struct {
		Name       string
		Namespaces []namespace
	}{
		Name:       tier,
		Namespaces: namespaces,
	})
	return result.Bytes(), err
}

func indent(template []byte, indent int) ([]byte, error) {
	result := bytes.NewBuffer(nil)
	scanner := bufio.NewScanner(bytes.NewReader(template))
	for scanner.Scan() {
		if result.Len() > 0 {
			// if previous line(s) were written, insert a line feed, first
			result.WriteRune('\n')
		}
		result.WriteString(strings.Repeat(" ", indent))
		result.Write(scanner.Bytes())
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return result.Bytes(), nil
}
