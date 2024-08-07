package mapper

import (
	"context"
	"k8s.io/apimachinery/pkg/types"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func MapByResourceName(hostNamespace string) func(ctx context.Context, object runtimeclient.Object) []reconcile.Request {
	return func(ctx context.Context, obj runtimeclient.Object) []reconcile.Request {
		return []reconcile.Request{{
			NamespacedName: types.NamespacedName{Namespace: hostNamespace, Name: obj.GetName()},
		}}
	}
}
