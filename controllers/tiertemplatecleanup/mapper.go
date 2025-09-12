package tiertemplatecleanup

import (
	"context"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func MapNSTemplateTierToTierTemplates(ctx context.Context, obj client.Object) (reqs []reconcile.Request) {
	nstt, ok := obj.(*toolchainv1alpha1.NSTemplateTier)
	if !ok {
		return
	}

	ns := nstt.Namespace

	if nstt.Spec.ClusterResources != nil && nstt.Spec.ClusterResources.TemplateRef != "" {
		reqs = append(reqs, requestForName(nstt.Spec.ClusterResources.TemplateRef, ns))
	}

	for _, nst := range nstt.Spec.Namespaces {
		reqs = append(reqs, requestForName(nst.TemplateRef, ns))
	}

	for _, srt := range nstt.Spec.SpaceRoles {
		reqs = append(reqs, requestForName(srt.TemplateRef, ns))
	}

	return
}

func requestForName(name, ns string) reconcile.Request {
	return reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      name,
			Namespace: ns,
		},
	}
}
