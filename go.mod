module github.com/codeready-toolchain/host-operator

require (
	github.com/codeready-toolchain/api v0.0.0-20220215110522-1a78f33ee8fc
	github.com/codeready-toolchain/toolchain-common v0.0.0-20220215110812-832c0dc0d182
	github.com/ghodss/yaml v1.0.0
	github.com/go-bindata/go-bindata v3.1.2+incompatible
	github.com/go-logr/logr v0.4.0
	github.com/gofrs/uuid v3.3.0+incompatible
	github.com/mailgun/mailgun-go/v4 v4.5.2
	// using latest commit from 'github.com/openshift/api@release-4.7'
	github.com/openshift/api v0.0.0-20210428205234-a8389931bee7
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.12.0
	github.com/redhat-cop/operator-utils v1.1.3-0.20210602122509-2eaf121122d2
	github.com/spf13/cast v1.3.1
	github.com/stretchr/testify v1.7.0
	go.uber.org/zap v1.19.0
	golang.org/x/crypto v0.0.0-20210220033148-5ea612d1eb83 // indirect
	gopkg.in/h2non/gock.v1 v1.0.14
	gopkg.in/yaml.v2 v2.4.0
	k8s.io/api v0.20.2
	k8s.io/apiextensions-apiserver v0.20.2
	k8s.io/apimachinery v0.20.2
	k8s.io/client-go v0.20.2
	k8s.io/klog v1.0.0
	k8s.io/klog/v2 v2.8.0
	k8s.io/kubectl v0.20.2
	sigs.k8s.io/controller-runtime v0.8.3
)

go 1.16
