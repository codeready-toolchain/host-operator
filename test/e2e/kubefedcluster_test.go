package e2e

import (
	"github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	"github.com/codeready-toolchain/toolchain-common/pkg/test/e2e"
	"testing"
)

func TestKubeFedCluster(t *testing.T) {
	// given & when & then
	e2e.VerifyKubeFedCluster(t, cluster.Host)
}
