package mapper

import (
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func MapByResourceName(hostNamespace string) func(object client.Object) []reconcile.Request {
	return func(obj client.Object) []reconcile.Request {
		return []reconcile.Request{{
			NamespacedName: types.NamespacedName{Namespace: hostNamespace, Name: obj.GetName()},
		}}
	}
}
