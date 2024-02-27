package toolchaincluster

import (
	"embed"

	v1 "k8s.io/api/core/v1"
	rbac "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
)

// GetServiceAccountFromTemplate returns the toolchaincluster service account object from the template content
func GetServiceAccountFromTemplate(toolchainclusterFS *embed.FS, templateName string, into runtime.Object) (*v1.ServiceAccount, error) {
	obj, gkv, err := decodeTemplate(toolchainclusterFS, templateName, into)
	if err != nil {
		return nil, err
	}
	if gkv.Kind == "ServiceAccount" {
		return obj.(*v1.ServiceAccount), nil
	}
	return nil, nil
}

// GetRoleFromTemplate returns the toolchaincluster Role object from the template content
func GetRoleFromTemplate(toolchainclusterFS *embed.FS, templateName string, into runtime.Object) (*rbac.Role, error) {
	obj, gkv, err := decodeTemplate(toolchainclusterFS, templateName, into)
	if err != nil {
		return nil, err
	}
	if gkv.Kind == "Role" {
		return obj.(*rbac.Role), nil
	}
	return nil, nil
}

// GetRoleBindingFromTemplate returns the toolchaincluster RoleBinding object from the template content
func GetRoleBindingFromTemplate(toolchainclusterFS *embed.FS, templateName string, into runtime.Object) (*rbac.RoleBinding, error) {
	obj, gkv, err := decodeTemplate(toolchainclusterFS, templateName, into)
	if err != nil {
		return nil, err
	}
	if gkv.Kind == "RoleBinding" {
		return obj.(*rbac.RoleBinding), nil
	}
	return nil, nil
}

func decodeTemplate(toolchainclusterFS *embed.FS, templateName string, into runtime.Object) (runtime.Object, *schema.GroupVersionKind, error) {
	saContent, err := toolchainclusterFS.ReadFile("templates/toolchaincluster/" + templateName)
	if err != nil {
		return nil, nil, err
	}
	decode := scheme.Codecs.UniversalDeserializer().Decode
	obj, gkv, err := decode(saContent, nil, into)
	if err != nil {
		return nil, nil, err
	}
	return obj, gkv, nil
}
