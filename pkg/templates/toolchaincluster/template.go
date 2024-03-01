package toolchaincluster

import (
	"bytes"
	"embed"
	"io"
	"io/fs"
	"text/template"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes/scheme"
)

// TemplateVariables contains all the available variables that are supported by the templates
type TemplateVariables struct {
	Namespace string
}

// LoadObjectsFromTemplates loads all the kubernetes objects from an embedded filesystem and returns a list of Unstructured objects that can be applied in the cluster.
// The function will return all the objects it finds starting from the root of the embedded filesystem.
func LoadObjectsFromTemplates(toolchainclusterFS *embed.FS, variables *TemplateVariables) ([]*unstructured.Unstructured, error) {
	var objects []*unstructured.Unstructured
	entries, err := getAllTemplateNames(toolchainclusterFS)
	if err != nil {
		return objects, err
	}
	for _, templatePath := range entries {
		templateContent, err := toolchainclusterFS.ReadFile(templatePath)
		if err != nil {
			return objects, err
		}
		buf, err := replaceTemplateVariables(templatePath, templateContent, variables)
		if err != nil {
			return objects, err
		}
		decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(buf.Bytes()), 100)
		for {
			var rawExt runtime.RawExtension
			if err := decoder.Decode(&rawExt); err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				return objects, err
			}
			rawExt.Raw = bytes.TrimSpace(rawExt.Raw)
			if len(rawExt.Raw) == 0 || bytes.Equal(rawExt.Raw, []byte("null")) {
				continue
			}
			unstructuredObj := &unstructured.Unstructured{}
			_, _, err = scheme.Codecs.UniversalDeserializer().Decode(rawExt.Raw, nil, unstructuredObj)
			if err != nil {
				return objects, err
			}
			objects = append(objects, unstructuredObj)
		}
	}
	return objects, nil
}

// replaceTemplateVariables replaces all the variables in the given template and returns a buffer with the evaluated content
func replaceTemplateVariables(templateName string, templateContent []byte, variables *TemplateVariables) (bytes.Buffer, error) {
	var buf bytes.Buffer
	tmpl, err := template.New(templateName).Parse(string(templateContent))
	if err != nil {
		return buf, err
	}
	err = tmpl.Execute(&buf, variables)
	return buf, err
}

// getAllTemplateNames reads the embedded filesystem and returns a list with all the filenames
func getAllTemplateNames(toolchainclusterFS *embed.FS) (files []string, err error) {
	err = fs.WalkDir(toolchainclusterFS, ".", func(path string, d fs.DirEntry, err error) error {
		if d.IsDir() {
			return nil
		}

		files = append(files, path)
		return nil
	})
	return files, err
}
