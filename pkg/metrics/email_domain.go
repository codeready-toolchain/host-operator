package metrics

import (
	"regexp"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

type Domain string

const Internal Domain = "internal"
const External Domain = "external"

var internalDomainPattern = regexp.MustCompile(`^.*@redhat.com$|^.*@((.+)\.)?ibm.com$`)

// GetEmailDomain retrieves the email address for the given object
// returns the associated domain (`Internal` or `External`)
// Note: if given email address is empty (ie, it does not exist - which should not happen),
// then an error is logged and the returned domain is `external`
func GetEmailDomain(obj RuntimeObject) Domain {
	emailAddress, exists := obj.GetAnnotations()[toolchainv1alpha1.UserSignupUserEmailAnnotationKey]
	if !exists {
		log.Error(nil, "no email address found in annotations", "kind", obj.GetObjectKind().GroupVersionKind().Kind, "name", obj.GetName())
	} else if internalDomainPattern.MatchString(emailAddress) {
		return Internal
	}
	return External
}

type RuntimeObject interface {
	GetAnnotations() map[string]string
	GetName() string
	GetObjectKind() schema.ObjectKind
}
