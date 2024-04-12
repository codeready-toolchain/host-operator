package nstemplatetiers

import (
	"context"

	"github.com/codeready-toolchain/host-operator/pkg/templates/assets"
	commonclient "github.com/codeready-toolchain/toolchain-common/pkg/client"
	"github.com/codeready-toolchain/toolchain-common/pkg/template/nstemplatetiers"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
	"k8s.io/apimachinery/pkg/runtime"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// CreateOrUpdateResources generates the NSTemplateTier resources from the cluster resource template and namespace templates,
// then uses the manager's client to create or update the resources on the cluster.
func CreateOrUpdateResources(ctx context.Context, s *runtime.Scheme, client runtimeclient.Client, namespace string, assets assets.Assets) error {

	// load templates from assets
	metadataContent, err := assets.Asset("metadata.yaml")
	if err != nil {
		return errors.Wrapf(err, "unable to load templates")
	}
	metadata := make(map[string]string)
	err = yaml.Unmarshal(metadataContent, &metadata)
	if err != nil {
		return errors.Wrapf(err, "unable to load templates")
	}
	files := map[string][]byte{}
	for _, name := range assets.Names() {
		if name == "metadata.yaml" {
			continue
		}
		content, err := assets.Asset(name)
		if err != nil {
			return errors.Wrapf(err, "unable to load templates")
		}
		files[name] = content
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
