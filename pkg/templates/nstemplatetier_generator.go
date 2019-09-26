package templates

import (
	"bufio"
	"bytes"
	"fmt"
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
metadata:
  name: {{ .Name }}
spec:
  namespaces:{{ range $index, $ns := .Namespaces }}
  - type: {{ .Type }}
    revision: {{ .Revision }}
    template: >
{{ .Template }}{{ end }}`)
	if initErr != nil {
		log.Error(initErr, "failed to initialize NSTemplateTier template")
	}

}

// GenerateNSTemplateTierManifest generates a full NSTemplateTier manifest
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
func GenerateNSTemplateTierManifest(tier string) ([]byte, error) {
	if nstemplatetierTmpl == nil {
		return nil, errors.Wrapf(initErr, "Cannot generate the '%s' NSTemplateTier manifest", tier)
	}
	revisions, err := getRevisions(tier)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to generate '%s' NSTemplateTier manifest", tier)
	}
	type namespace struct {
		Type     string
		Revision string
		Template string
	}
	namespaces := make([]namespace, 3)

	for i, kind := range []string{"code", "dev", "stage"} {
		// get the content of the `-<t>.yaml` file
		tmpl, err := Asset(fmt.Sprintf("%s-%s.yaml", tier, kind))
		if err != nil {
			return nil, errors.Wrapf(err, "unable to generate '%s' NSTemplateTier manifest", tier)
		}
		tmpl, err = indent(tmpl, 6)
		if err != nil {
			return nil, errors.Wrapf(err, "unable to generate '%s' NSTemplateTier manifest", tier)
		}
		namespaces[i] = namespace{
			Type:     kind,
			Revision: revisions[kind],
			Template: string(tmpl),
		}
	}
	result := bytes.NewBuffer(nil)
	err = nstemplatetierTmpl.Execute(result, struct {
		Name       string
		Namespaces []namespace
	}{
		Name:       tier,
		Namespaces: namespaces,
	})
	return result.Bytes(), err
}

// GetRevisions returns a map of revisions indexed by their associated template kind
// (eg: {"code":"123456", "dev":"abcdef", "stage":"cafe01"})
func getRevisions(tier string) (map[string]string, error) {
	metadata, err := Asset("metadata.yaml")
	if err != nil {
		return nil, err
	}
	return parseRevisions(metadata, tier)
}

// parseRevisions returns a map of revisions indexed by their associated template kind
// (eg: {"code":"123456", "dev":"abcdef", "stage":"cafe01"})
func parseRevisions(metadata []byte, tier string) (map[string]string, error) {
	revisions := make(map[string]interface{})
	err := yaml.Unmarshal([]byte(metadata), &revisions)
	if err != nil {
		return nil, err
	}
	result := make(map[string]string, len(revisions))
	for tierkind, revision := range revisions {
		if strings.HasPrefix(tierkind, tier+"-") {
			if revision, ok := revision.(string); ok {
				result[tierkind[len(tier)+1:]] = revision
			}
		}
	}
	return result, nil
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
