module github.com/codeready-toolchain/host-operator

require (
	github.com/codeready-toolchain/api v0.0.0-20200925000842-d2e82e080a5a
	github.com/codeready-toolchain/toolchain-common v0.0.0-20200930070243-e124e69e7a0d
	github.com/go-bindata/go-bindata v3.1.2+incompatible
	github.com/go-logr/logr v0.1.0
	github.com/gofrs/uuid v3.3.0+incompatible
	github.com/kr/text v0.2.0 // indirect
	github.com/mailgun/mailgun-go/v4 v4.1.3
	github.com/niemeyer/pretty v0.0.0-20200227124842-a10e7caefd8e // indirect
	github.com/openshift/api v3.9.1-0.20190924102528-32369d4db2ad+incompatible
	github.com/operator-framework/operator-sdk v0.19.2
	github.com/pkg/errors v0.9.1
	github.com/redhat-cop/operator-utils v0.0.0-20190827162636-51e6b0c32776
	github.com/satori/go.uuid v1.2.0
	github.com/spf13/cast v1.3.1
	github.com/spf13/pflag v1.0.5
	github.com/spf13/viper v1.6.1
	github.com/stretchr/testify v1.5.1
	golang.org/x/tools v0.0.0-20200502202811-ed308ab3e770 // indirect
	gopkg.in/check.v1 v1.0.0-20200227125254-8fa46927fb4f // indirect
	gopkg.in/h2non/gock.v1 v1.0.14
	gopkg.in/yaml.v2 v2.2.8
	honnef.co/go/tools v0.0.1-2020.1.3 // indirect
	k8s.io/api v0.18.3
	k8s.io/apiextensions-apiserver v0.18.3
	k8s.io/apimachinery v0.18.3
	k8s.io/client-go v12.0.0+incompatible
	sigs.k8s.io/controller-runtime v0.6.0
)

replace github.com/codeready-toolchain/api => github.com/matousjobanek/api v0.0.0-20201008102715-afd628dfa930

replace (
	github.com/Azure/go-autorest => github.com/Azure/go-autorest v13.3.2+incompatible // Required by OLM
	github.com/openshift/api => github.com/openshift/api v0.0.0-20200821140346-b94c46af3f2b // Using 'github.com/openshift/api@release-4.5'
	k8s.io/client-go => k8s.io/client-go v0.18.3 // Required by prometheus-operator
	k8s.io/kube-openapi => k8s.io/kube-openapi v0.0.0-20200410145947-61e04a5be9a6 // avoids case-insensitive import collision: "github.com/googleapis/gnostic/openapiv2" and "github.com/googleapis/gnostic/OpenAPIv2"
)

go 1.14
