package pseb_test

import (
	"os"
	"strings"
	"testing"

	"github.com/alecthomas/assert/v2"
	"github.com/mistermoe/pseb"
)

func TestExtractCertificate(t *testing.T) {
	// testdata/pseb_company_sample.pdf is a synthetic certificate with fake data
	// (see cmd/gensample). It embeds a QR code carrying an HS256 JWT signed with
	// the fixed secret in testdata/jwt_secret.key, so the QR/JWT extraction path
	// can be exercised without committing a real certificate.
	pdf, err := os.ReadFile("testdata/pseb_company_sample.pdf")
	assert.NoError(t, err)

	cert, err := pseb.ExtractCertificate(pdf)
	assert.NoError(t, err)

	assert.NotZero(t, cert.JWT)
	assert.True(t, strings.Contains(cert.PSEBHostedVerificationURL, "verify-certificate"))
	assert.Equal(t, "Z-99-99999/25", cert.RegistrationNumber)
	assert.Equal(t, pseb.CertificateTypeCompany, cert.Type)
	assert.False(t, cert.IssuedAt.IsZero())
	assert.False(t, cert.JWTExpiresAt.IsZero())
}

func TestExtractCNIC(t *testing.T) {
	// testdata/pseb_freelancer_sample.pdf is a synthetic certificate with fake
	// data (see cmd/gensample); it avoids committing real PII to the repo.
	pdf, err := os.ReadFile("testdata/pseb_freelancer_sample.pdf")
	assert.NoError(t, err)

	cnic, err := pseb.ExtractCNIC(pdf)
	assert.NoError(t, err)
	assert.Equal(t, "1234512345671", cnic)
}
