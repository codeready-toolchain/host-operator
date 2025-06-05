package templates

import (
	"embed"
	"io/fs"
)

// BundledWithHostOperatorAnnotationValue is meant to be the value of the toolchainv1alpha1.BundledLabelKey that marks
// the objects as bundled with the host operator and therefore managed by it.
const BundledWithHostOperatorAnnotationValue = "host-operator"

func GetAllFileNames(TemplateTierFS *embed.FS, root string) (files []string, err error) {
	if err := fs.WalkDir(TemplateTierFS, root, func(path string, d fs.DirEntry, err error) error {
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
