package metrics

import (
	"regexp"
)

type Domain string

const RedHat Domain = "Red Hat"
const IBM Domain = "IBM"
const Other Domain = "Other"

var rhDomainPattern *regexp.Regexp
var ibmDomainPattern *regexp.Regexp

func init() {
	rhDomainPattern = regexp.MustCompile(`^.*@redhat.com$`)
	ibmDomainPattern = regexp.MustCompile(`^.*@((.+)\.)?ibm.com$`)
}

// GetEmailDomain retrieves the email address for the given UserSignup
// returns the associated domain (`Red Hat`, `IBM` or `Other`)
// returns `true` if found, `false` otherwise
func GetEmailDomain(emailAddress string) Domain {
	// match a pattern in the email address: `@redhat.com`, `@ibm.com` or subdomains (eg: `@fr.ibm.com`) or others
	if rhDomainPattern.MatchString(emailAddress) {
		return RedHat
	} else if ibmDomainPattern.MatchString(emailAddress) {
		return IBM
	}
	return Other

}
