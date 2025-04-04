package nstemplatetiers

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"strings"

	commonclient "github.com/codeready-toolchain/toolchain-common/pkg/client"
	"github.com/codeready-toolchain/toolchain-common/pkg/template/nstemplatetiers"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
	"k8s.io/apimachinery/pkg/runtime"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const NSTemplateTierRootDir = "templates/nstemplatetiers"

// CreateOrUpdateResources generates the NSTemplateTier resources from the cluster resource template and namespace templates,
// then uses the manager's client to create or update the resources on the cluster.
func CreateOrUpdateResources(ctx context.Context, s *runtime.Scheme, client runtimeclient.Client, namespace string, nstemplatetierFS embed.FS, root string) error {

	// load templates from assets
	metadataContent, err := nstemplatetierFS.ReadFile(root + "/metadata.yaml")
	if err != nil {
		return errors.Wrapf(err, "unable to load templates")
	}
	metadata := make(map[string]string)
	err = yaml.Unmarshal(metadataContent, &metadata)
	if err != nil {
		return errors.Wrapf(err, "unable to load templates")
	}

	paths, err := getAllFileNames(&nstemplatetierFS, root)
	if err != nil {
		return err
	}
	if len(paths) == 0 {
		return fmt.Errorf("could not find any user templates")
	}

	files := map[string][]byte{}
	for _, name := range paths {
		if name == "templates/nstemplatetiers/metadata.yaml" {
			continue
		}

		parts := strings.Split(name, "/")
		// skip any name that does not have 4 parts
		if len(parts) != 4 {
			return fmt.Errorf("unable to load templates: invalid name format for file '%s'", name)
		}

		fileName := parts[2] + "/" + parts[3]
		content, err := nstemplatetierFS.ReadFile(name)
		if err != nil {
			return errors.Wrapf(err, "unable to load templates")
		}
		files[fileName] = content
	}

	// initialize tier generator, loads templates from assets
	return nstemplatetiers.GenerateTiers(s, func(toEnsure runtimeclient.Object, canUpdate bool, _ string) (bool, error) {
		if !canUpdate {
			if err := client.Create(ctx, toEnsure); err != nil && !apierrors.IsAlreadyExists(err) {
				return false, err
			}
			return true, nil
		}
		applyCl := commonclient.NewApplyClient(client)
		return applyCl.ApplyObject(ctx, toEnsure, commonclient.ForceUpdate(true))
	}, namespace, metadata, files)
}

func getAllFileNames(nsTemplateTierFS *embed.FS, root string) (files []string, err error) {

	if err := fs.WalkDir(nsTemplateTierFS, root, func(path string, d fs.DirEntry, err error) error {
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
