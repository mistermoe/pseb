package pseb_test

import (
	"os"
	"strings"
	"testing"

	"github.com/alecthomas/assert/v2"
	"github.com/mistermoe/pseb"
)

func TestExtractCertificate(t *testing.T) {
	pdf, err := os.ReadFile("pseb_cert.pdf")
	assert.NoError(t, err)

	cert, err := pseb.ExtractCertificate(pdf)
	assert.NoError(t, err)

	assert.NotZero(t, cert.JWT)
	assert.True(t, strings.Contains(cert.PSEBHostedVerificationURL, "verify-certificate"))
	assert.Equal(t, "Z-25-17156/25", cert.RegistrationNumber)
	assert.Equal(t, pseb.CertificateTypeCompany, cert.Type)
	assert.False(t, cert.IssuedAt.IsZero())
	assert.False(t, cert.ExpiresAt.IsZero())
}
