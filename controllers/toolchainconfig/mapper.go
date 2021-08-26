package toolchainconfig

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type SecretToToolchainConfigMapper struct{}

var mapperLog = ctrl.Log.WithName("SecretToToolchainConfigMapper")

// MapSecretToToolchainConfig maps secrets to the singular instance of ToolchainConfig named "config"
func MapSecretToToolchainConfig() func(object client.Object) []reconcile.Request {
	return func(obj client.Object) []reconcile.Request {
		if secret, ok := obj.(*corev1.Secret); ok {
			mapperLog.Info("Secret mapped to ToolchainConfig", "name", secret.Name)
			return []reconcile.Request{{NamespacedName: types.NamespacedName{Namespace: secret.Namespace, Name: "config"}}}
		}
		// the obj was not a Secret
		return []reconcile.Request{}
	}
}
