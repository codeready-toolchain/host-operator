package test

import (
	"testing"

	"github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/controller/hostoperatorconfig"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
)

func NewHostOperatorConfigWithReset(t *testing.T, options ...test.HostConfigOption) *v1alpha1.HostOperatorConfig {
	t.Cleanup(hostoperatorconfig.Reset)
	return test.NewHostOperatorConfig(options...)
}
