package spaceprovisionerconfig

import (
	"context"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func MapToolchainClusterToSpaceProvisionerConfigs(cl runtimeclient.Client) func(context.Context, runtimeclient.Object) []reconcile.Request {
	return func(ctx context.Context, obj runtimeclient.Object) []reconcile.Request {
		if _, ok := obj.(*toolchainv1alpha1.ToolchainCluster); !ok {
			return nil
		}

		ret, err := findReferencingProvisionerConfigs(ctx, cl, runtimeclient.ObjectKeyFromObject(obj))
		if err != nil {
			log.FromContext(ctx).Error(err, "failed to list SpaceProvisionerConfig objects while determining what objects to reconcile",
				"causeObj", runtimeclient.ObjectKeyFromObject(obj), "causeKind", "ToolchainCluster")
			return []reconcile.Request{}
		}
		return ret
	}
}

func MapToolchainStatusToSpaceProvisionerConfigs(cl runtimeclient.Client) func(context.Context, runtimeclient.Object) []reconcile.Request {
	return func(ctx context.Context, obj runtimeclient.Object) []reconcile.Request {
		if _, ok := obj.(*toolchainv1alpha1.ToolchainStatus); !ok {
			return nil
		}

		ret, err := findAllSpaceProvisionerConfigsInNamespace(ctx, cl, obj.GetNamespace())
		if err != nil {
			log.FromContext(ctx).Error(err, "failed to list SpaceProvisionerConfig objects while determining what objects to reconcile",
				"causeObj", runtimeclient.ObjectKeyFromObject(obj), "causeKind", "ToolchainStatus")
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

func findAllSpaceProvisionerConfigsInNamespace(ctx context.Context, cl runtimeclient.Client, ns string) ([]reconcile.Request, error) {
	configs := &toolchainv1alpha1.SpaceProvisionerConfigList{}
	if err := cl.List(ctx, configs, runtimeclient.InNamespace(ns)); err != nil {
		return nil, err
	}
	ret := make([]reconcile.Request, 0, len(configs.Items))
	for _, cfg := range configs.Items {
		ret = append(ret, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: cfg.Namespace,
				Name:      cfg.Name,
			},
		})
	}
	return ret, nil
}
