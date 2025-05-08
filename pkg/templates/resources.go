package templates

import (
	"embed"
	"io/fs"
)

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
