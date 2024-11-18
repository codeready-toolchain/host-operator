package spaceprovisionerconfig

import (
	"context"
	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func MapToolchainClusterToSpaceProvisionerConfigs(ctx context.Context, cl runtimeclient.Client) func(context.Context, runtimeclient.Object) []reconcile.Request {
	return func(context context.Context, obj runtimeclient.Object) []reconcile.Request {
		ret, err := findReferencingProvisionerConfigs(ctx, cl, runtimeclient.ObjectKeyFromObject(obj))
		if err != nil {
			log.FromContext(ctx).Error(err, "failed to list SpaceProvisionerConfig objects while determining what objects to reconcile",
				"toolchainClusterCause", runtimeclient.ObjectKeyFromObject(obj))
			return []reconcile.Request{}
		}
		return ret
	}
}

func findReferencingProvisionerConfigs(ctx context.Context, cl runtimeclient.Client, toolchainClusterObjectKey runtimeclient.ObjectKey) ([]reconcile.Request, error) {
	configs := &toolchainv1alpha1.SpaceProvisionerConfigList{}
	if err := cl.List(ctx, configs, runtimeclient.InNamespace(toolchainClusterObjectKey.Namespace)); err != nil {
		return nil, err
	}
	ret := make([]reconcile.Request, 0, len(configs.Items))
	for _, cfg := range configs.Items {
		if cfg.Spec.ToolchainCluster == toolchainClusterObjectKey.Name {
			ret = append(ret, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: cfg.Namespace,
					Name:      cfg.Name,
				},
			})
		}
	}
	return ret, nil
}
