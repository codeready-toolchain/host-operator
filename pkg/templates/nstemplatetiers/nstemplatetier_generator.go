package nstemplatetiers

import (
	"context"
	"embed"
	"fmt"
	"strings"

	"github.com/codeready-toolchain/host-operator/deploy"
	"github.com/codeready-toolchain/host-operator/pkg/templates"
	commonclient "github.com/codeready-toolchain/toolchain-common/pkg/client"
	"github.com/codeready-toolchain/toolchain-common/pkg/template/nstemplatetiers"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const nsTemplateTierRootDir = "templates/nstemplatetiers"

// CreateOrUpdateResources generates the NSTemplateTier resources from the cluster resource template and namespace templates,
// then uses the manager's client to create or update the resources on the cluster.
func CreateOrUpdateResources(ctx context.Context, s *runtime.Scheme, client runtimeclient.Client, namespace string) error {

	metadata, files, err := LoadFiles(deploy.NSTemplateTiersFS, nsTemplateTierRootDir)
	if err != nil {
		return err
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

// LoadFiles takes the file from deploy/nstemplatetiers/<tiername>/<yaml file name> . the folder structure should be 4 steps .
// as the cologic here is written accordingly
func LoadFiles(nsTemplateTiers embed.FS, root string) (metadata map[string]string, files map[string][]byte, err error) {
	// load templates from assets
	metadataContent, err := nsTemplateTiers.ReadFile(root + "/metadata.yaml")
	if err != nil {
		return nil, nil, errors.Wrapf(err, "unable to load templates")
	}

	metadata = make(map[string]string)
	err = yaml.Unmarshal(metadataContent, &metadata)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "unable to load templates")
	}

	paths, err := templates.GetAllFileNames(&nsTemplateTiers, root)
	if err != nil {
		return nil, nil, err
	}
	if len(paths) == 0 {
		return nil, nil, fmt.Errorf("could not find any ns templates")
	}
	files = make(map[string][]byte)
	for _, name := range paths {
		if name == root+"/metadata.yaml" {
			continue
		}

		parts := strings.Split(name, "/")
		// skip any name that does not have 4 parts
		if len(parts) != 4 {
			return nil, nil, fmt.Errorf("unable to load templates: invalid name format for file '%s'", name)
		}

		fileName := parts[2] + "/" + parts[3]
		content, err := nsTemplateTiers.ReadFile(name)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "unable to load templates")
		}
		files[fileName] = content
	}
	return metadata, files, nil
}
