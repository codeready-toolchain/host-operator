package toolchaincluster

import (
	"testing"

	"github.com/codeready-toolchain/host-operator/deploy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	rbac "k8s.io/api/rbac/v1"
)

func TestGetServiceAccountTemplate(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		// when
		sa, err := GetServiceAccountFromTemplate(&deploy.ToolchainClusterTemplateFS, "host-sa.yaml", &v1.ServiceAccount{})
		// then
		require.NoError(t, err)
		require.NotNil(t, sa)
		assert.Equal(t, "toolchaincluster-host", sa.Name)
	})

	t.Run("error reading template", func(t *testing.T) {
		// when
		// we pass an invalid template path
		sa, err := GetServiceAccountFromTemplate(&deploy.ToolchainClusterTemplateFS, "host-sa-invalid.yaml", &v1.ServiceAccount{})
		// then
		require.Error(t, err)
		require.Nil(t, sa)
	})

	t.Run("no service account found", func(t *testing.T) {
		// when
		// we pass a template that contains a different Kind
		sa, err := GetServiceAccountFromTemplate(&deploy.ToolchainClusterTemplateFS, "host-role.yaml", &v1.ServiceAccount{})
		// then
		require.Nil(t, err)
		require.Nil(t, sa)
	})
}

func TestGetRoleTemplate(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		// when
		role, err := GetRoleFromTemplate(&deploy.ToolchainClusterTemplateFS, "host-role.yaml", &rbac.Role{})
		// then
		require.NoError(t, err)
		require.NotNil(t, role)
		assert.Equal(t, "toolchaincluster-host", role.Name)
		require.NotEmpty(t, role.Rules)
	})

	t.Run("error reading template", func(t *testing.T) {
		// when
		// we pass an invalid template path
		role, err := GetRoleFromTemplate(&deploy.ToolchainClusterTemplateFS, "host-role-invalid.yaml", &rbac.Role{})
		// then
		require.Error(t, err)
		require.Nil(t, role)
	})

	t.Run("no role found", func(t *testing.T) {
		// when
		// we pass a template that contains a different Kind
		role, err := GetRoleFromTemplate(&deploy.ToolchainClusterTemplateFS, "host-sa.yaml", &rbac.Role{})
		// then
		require.Nil(t, err)
		require.Nil(t, role)
	})
}

func TestGetRoleBindingTemplate(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		// when
		rolebinding, err := GetRoleBindingFromTemplate(&deploy.ToolchainClusterTemplateFS, "host-rolebinding.yaml", &rbac.RoleBinding{})
		// then
		require.NoError(t, err)
		require.NotNil(t, rolebinding)
		assert.Equal(t, "toolchaincluster-host", rolebinding.Name)
		assert.Equal(t, "ServiceAccount", rolebinding.Subjects[0].Kind)
		assert.Equal(t, "toolchaincluster-host", rolebinding.Subjects[0].Name)
		assert.Equal(t, "Role", rolebinding.RoleRef.Kind)
		assert.Equal(t, "toolchaincluster-host", rolebinding.RoleRef.Name)
	})

	t.Run("error reading template", func(t *testing.T) {
		// when
		// we pass an invalid template path
		rb, err := GetRoleBindingFromTemplate(&deploy.ToolchainClusterTemplateFS, "host-rolebinding-invalid.yaml", &rbac.RoleBinding{})
		// then
		require.Error(t, err)
		require.Nil(t, rb)
	})

	t.Run("no rolebinding found", func(t *testing.T) {
		// when
		// we pass a template that contains a different Kind
		rb, err := GetRoleBindingFromTemplate(&deploy.ToolchainClusterTemplateFS, "host-role.yaml", &rbac.RoleBinding{})
		// then
		require.Nil(t, err)
		require.Nil(t, rb)
	})
}
