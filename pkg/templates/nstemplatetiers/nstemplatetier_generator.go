package nstemplatetiers

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/deploy"
	"github.com/codeready-toolchain/host-operator/pkg/constants"
	"github.com/codeready-toolchain/host-operator/pkg/templates"
	commonclient "github.com/codeready-toolchain/toolchain-common/pkg/client"
	"github.com/codeready-toolchain/toolchain-common/pkg/template/nstemplatetiers"
	"gopkg.in/yaml.v2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const NsTemplateTierRootDir = "templates/nstemplatetiers"

// BundledTierKeys is the list of NSTemplateTiers that are bundled with the operator binary.
// This list is initialized during the SyncResources call. DO NOT MODIFY it unless you're in
// tests and need to setup a custom list of bundled tiers.
var BundledTierKeys []runtimeclient.ObjectKey

// SyncResources generates the NSTemplateTier resources from the cluster resource template and namespace templates,
// then uses the manager's client to create or update the resources on the cluster. It also deletes all the tiers
// that used to be bundled but are not anymore. Note that these tiers have finalizers ensuring that the deletion
// actually concludes only when such tiers are not used by any space.
func SyncResources(ctx context.Context, s *runtime.Scheme, client runtimeclient.Client, namespace string) error {
	metadata, files, err := LoadFiles(deploy.NSTemplateTiersFS, NsTemplateTierRootDir)
	if err != nil {
		return err
	}

	// re-initialize in case this function got called multiple times, even though it really shouldn't
	BundledTierKeys = nil

	// initialize tier generator, loads templates from assets
	err = nstemplatetiers.GenerateTiers(s, func(toEnsure runtimeclient.Object, canUpdate bool, _ string) (bool, error) {
		controllerutil.AddFinalizer(toEnsure, constants.BundledNSTemplateTierFinalizerName)

		BundledTierKeys = append(BundledTierKeys, runtimeclient.ObjectKeyFromObject(toEnsure))

		if !canUpdate {
			if err := client.Create(ctx, toEnsure); err != nil && !apierrors.IsAlreadyExists(err) {
				return false, err
			}
			return true, nil
		}
		applyCl := commonclient.NewApplyClient(client)
		return applyCl.ApplyObject(ctx, toEnsure, commonclient.ForceUpdate(true))
	}, namespace, metadata, files)
	if err != nil {
		return err
	}

	return removeNoLongerBundledTiers(ctx, client, namespace, BundledTierKeys)
}

// IsBundled tells whether the provided tier is bundled with the binary or not.
// Returns an error if called before SyncResources.
func IsBundled(tier runtimeclient.ObjectKey) (bool, error) {
	if BundledTierKeys == nil {
		return false, fmt.Errorf("SyncResources not called yet")
	}

	return slices.Contains(BundledTierKeys, tier), nil
}

func removeNoLongerBundledTiers(ctx context.Context, client runtimeclient.Client, namespace string, bundledTierKeys []runtimeclient.ObjectKey) error {
	allTiers := &toolchainv1alpha1.NSTemplateTierList{}
	if err := client.List(ctx, allTiers, runtimeclient.InNamespace(namespace)); err != nil {
		return err
	}

	var allErrors []error
	for _, tier := range allTiers.Items {
		if controllerutil.ContainsFinalizer(&tier, constants.BundledNSTemplateTierFinalizerName) &&
			!slices.Contains(bundledTierKeys, runtimeclient.ObjectKeyFromObject(&tier)) {
			if err := client.Delete(ctx, &tier); err != nil {
				allErrors = append(allErrors, err)
			}
		}
	}

	err := errors.Join(allErrors...)
	if err != nil {
		err = fmt.Errorf("failed to delete some of the no-longer-bundled NSTemplateTiers: %w", err)
	}

	return err
}

// LoadFiles takes the file from deploy/nstemplatetiers/<tiername>/<yaml file name> . the folder structure should be 4 steps .
// as the cologic here is written accordingly
func LoadFiles(nsTemplateTiers embed.FS, root string) (metadata map[string]string, files map[string][]byte, err error) {
	// load templates from assets
	metadataContent, err := nsTemplateTiers.ReadFile(filepath.Join(root, "metadata.yaml"))
	if err != nil {
		return nil, nil, fmt.Errorf("unable to load templates: %w", err)
	}

	metadata = make(map[string]string)
	err = yaml.Unmarshal(metadataContent, &metadata)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to load templates: %w", err)
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
		if name == filepath.Join(root, "metadata.yaml") {
			continue
		}

		parts := strings.Split(name, "/")
		// skip any name that does not have 4 parts
		if len(parts) != 4 {
			return nil, nil, fmt.Errorf("unable to load templates: invalid name format for file '%s'", name)
		}

		fileName := filepath.Join(parts[2], parts[3])
		content, err := nsTemplateTiers.ReadFile(name)
		if err != nil {
			return nil, nil, fmt.Errorf("unable to load templates: %w", err)
		}
		files[fileName] = content
	}
	return metadata, files, nil
}
