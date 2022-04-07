module github.com/codeready-toolchain/host-operator

require (
	github.com/codeready-toolchain/api v0.0.0-20220407065959-2029a1f03cfc
	github.com/codeready-toolchain/toolchain-common v0.0.0-20220407070729-d48e73f179f0
	github.com/ghodss/yaml v1.0.0
	github.com/go-bindata/go-bindata v3.1.2+incompatible
	github.com/go-logr/logr v0.4.0
	github.com/gofrs/uuid v3.3.0+incompatible
	github.com/mailgun/mailgun-go/v4 v4.5.2
	// using latest commit from 'github.com/openshift/api@release-4.9'
	github.com/openshift/api v0.0.0-20211028023115-7224b732cc14
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.12.0
	github.com/redhat-cop/operator-utils v1.3.3-0.20220121120056-862ef22b8cdf
	github.com/spf13/cast v1.3.1
	github.com/stretchr/testify v1.7.0
	go.uber.org/zap v1.19.0
	gopkg.in/h2non/gock.v1 v1.0.14
	gopkg.in/yaml.v2 v2.4.0
	k8s.io/api v0.22.7
	k8s.io/apiextensions-apiserver v0.22.7
	k8s.io/apimachinery v0.22.7
	k8s.io/client-go v0.22.7
	k8s.io/klog v1.0.0
	k8s.io/klog/v2 v2.9.0
	sigs.k8s.io/controller-runtime v0.10.3
)

go 1.16
