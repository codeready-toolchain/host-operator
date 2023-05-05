package mapper

import (
	"k8s.io/apimachinery/pkg/types"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func MapByResourceName(hostNamespace string) func(object runtimeclient.Object) []reconcile.Request {
	return func(obj runtimeclient.Object) []reconcile.Request {
		return []reconcile.Request{{
			NamespacedName: types.NamespacedName{Namespace: hostNamespace, Name: obj.GetName()},
		}}
	}
}
