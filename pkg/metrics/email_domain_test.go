package metrics_test

import (
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/metrics"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stretchr/testify/assert"
)

func TestGetEmailDomain(t *testing.T) {

	type testdata struct {
		name           string
		object         metrics.RuntimeObject
		expectedDomain metrics.Domain
	}

	dataset := []testdata{
		{
			name: "Red Hatter",
			object: &toolchainv1alpha1.UserSignup{
				ObjectMeta: metav1.ObjectMeta{
					Name: "joe",
				},
				Spec: toolchainv1alpha1.UserSignupSpec{
					IdentityClaims: toolchainv1alpha1.IdentityClaimsEmbedded{
						PropagatedClaims: toolchainv1alpha1.PropagatedClaims{
							Email: "joe@redhat.com",
						},
					},
				},
			},
			expectedDomain: metrics.Internal,
		},
		{
			name: "IBMer",
			object: &toolchainv1alpha1.UserSignup{
				ObjectMeta: metav1.ObjectMeta{
					Name: "joe",
				},
				Spec: toolchainv1alpha1.UserSignupSpec{
					IdentityClaims: toolchainv1alpha1.IdentityClaimsEmbedded{
						PropagatedClaims: toolchainv1alpha1.PropagatedClaims{
							Email: "joe@ibm.com",
						},
					},
				},
			},
			expectedDomain: metrics.Internal,
		},
		{
			name: "Another IBMer",
			object: &toolchainv1alpha1.UserSignup{
				ObjectMeta: metav1.ObjectMeta{
					Name: "joe",
				},
				Spec: toolchainv1alpha1.UserSignupSpec{
					IdentityClaims: toolchainv1alpha1.IdentityClaimsEmbedded{
						PropagatedClaims: toolchainv1alpha1.PropagatedClaims{
							Email: "joe@fr.ibm.com",
						},
					},
				},
			},
			expectedDomain: metrics.Internal,
		},
		{
			name: "Not an IBMer",
			object: &toolchainv1alpha1.UserSignup{
				ObjectMeta: metav1.ObjectMeta{
					Name: "joe",
				},
				Spec: toolchainv1alpha1.UserSignupSpec{
					IdentityClaims: toolchainv1alpha1.IdentityClaimsEmbedded{
						PropagatedClaims: toolchainv1alpha1.PropagatedClaims{
							Email: "joe@fribm.com",
						},
					},
				},
			},
			expectedDomain: metrics.External,
		},
		{
			name: "External",
			object: &toolchainv1alpha1.UserSignup{
				ObjectMeta: metav1.ObjectMeta{
					Name: "joe",
				},
				Spec: toolchainv1alpha1.UserSignupSpec{
					IdentityClaims: toolchainv1alpha1.IdentityClaimsEmbedded{
						PropagatedClaims: toolchainv1alpha1.PropagatedClaims{
							Email: "joe@example.com",
						},
					},
				},
			},
			expectedDomain: metrics.External,
		},
		{
			name: "Missing",
			object: &toolchainv1alpha1.UserSignup{
				ObjectMeta: metav1.ObjectMeta{
					Name: "joe",
				},
			},
			expectedDomain: metrics.External,
		},
	}

	for _, d := range dataset {
		t.Run(d.name, func(t *testing.T) {
			// when
			domain := metrics.GetEmailDomain(d.object)

			// then
			assert.Equal(t, d.expectedDomain, domain)
		})
	}
}
