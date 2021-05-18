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
	}

	dataset := []testdata{
		{
			name:           "Red Hatter",
			emailAddress:   "joe@redhat.com",
			expectedDomain: metrics.Internal,
		},
		{
			name:           "IBMer",
			emailAddress:   "joe@ibm.com",
			expectedDomain: metrics.Internal,
		},
		{
			name:           "Another IBMer",
			emailAddress:   "joe@fr.ibm.com",
			expectedDomain: metrics.Internal,
		},
		{
			name:           "Not an IBMer",
			emailAddress:   "joe@fribm.com",
			expectedDomain: metrics.External,
		},
		{
			name:           "External",
			emailAddress:   "joe@example.com",
			expectedDomain: metrics.External,
		},
		{
			name:           "Missing",
			expectedDomain: metrics.External,
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
