package metrics

import (
	"regexp"
)

type Domain string

const Internal Domain = "internal"
const External Domain = "external"

var internalDomainPattern *regexp.Regexp

func init() {
	// pattern to match email addresses in the form of `@redhat.com`, `@ibm.com` or subdomains (eg: `@fr.ibm.com`)
	internalDomainPattern = regexp.MustCompile(`^.*@redhat.com$|^.*@((.+)\.)?ibm.com$`)
}

// GetEmailDomain retrieves the email address for the given UserSignup
// returns the associated domain (`Internal` or `External`)
func GetEmailDomain(emailAddress string) Domain {
	if internalDomainPattern.MatchString(emailAddress) {
		return Internal
	}
	return External
}
