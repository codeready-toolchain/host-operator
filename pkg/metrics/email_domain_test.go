package metrics_test

import (
	"testing"

	"github.com/codeready-toolchain/host-operator/pkg/metrics"

	"github.com/stretchr/testify/assert"
)

func TestGetEmailDomain(t *testing.T) {

	type testdata struct {
		name           string
		emailAddress   string
		expectedDomain metrics.Domain
		expectedFound  bool
	}

	dataset := []testdata{
		{
			name:           "Red Hatter",
			emailAddress:   "joe@redhat.com",
			expectedDomain: metrics.RedHat,
			expectedFound:  true,
		},
		{
			name:           "IBMer",
			emailAddress:   "joe@ibm.com",
			expectedDomain: metrics.IBM,
			expectedFound:  true,
		},
		{
			name:           "Another IBMer",
			emailAddress:   "joe@fr.ibm.com",
			expectedDomain: metrics.IBM,
			expectedFound:  true,
		},
		{
			name:           "Not an IBMer",
			emailAddress:   "joe@fribm.com",
			expectedDomain: metrics.Other,
			expectedFound:  true,
		},
		{
			name:           "Other",
			emailAddress:   "joe@example.com",
			expectedDomain: metrics.Other,
			expectedFound:  true,
		},
		{
			name:           "Missing",
			expectedDomain: metrics.Other,
			expectedFound:  false,
		},
	}

	for _, d := range dataset {
		t.Run(d.name, func(t *testing.T) {
			// when
			domain := metrics.GetEmailDomain(d.emailAddress)

			// then
			assert.Equal(t, d.expectedDomain, domain)
		})
	}
}
