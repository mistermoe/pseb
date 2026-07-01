package pseb_test

import (
	"testing"

	"github.com/alecthomas/assert/v2"
	"github.com/mistermoe/httpr"
	"github.com/mistermoe/pseb"
	"github.com/mistermoe/pseb/vcr"
	"gopkg.in/dnaeon/go-vcr.v3/recorder"
)

// testMode controls whether HTTP interactions are recorded or replayed. Set to
// vcr.Record to re-capture the cassette against the live PSEB endpoint, then
// flip back to vcr.Replay and commit the updated fixture.
var testMode = vcr.Replay

// sampleJWT is a real PSEB certificate JWT (company: OUTSENTIA (PRIVATE) LIMITED).
const sampleJWT = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJyZWdpc3RyYXRpb25ObyI6IlotMjUtMTcxNTYvMjUiLCJ0eXBlIjoiY29tcGFueSIsImlhdCI6MTc2MTEzMTM3NywiZXhwIjoxNzY4OTA3Mzc3fQ.fQp_CW9PGmoAesg_1rM-jva6KFON0QhIOSeYmbLGeao"

func bootstrap(_ *testing.T, _ vcr.Mode, rec *recorder.Recorder) *pseb.Client {
	return pseb.New(httpr.HTTPClient(*rec.GetDefaultClient()))
}

func TestVerify(t *testing.T) {
	vcr.Test(t, testMode, bootstrap, func(t *testing.T, client *pseb.Client, _ vcr.Cassette) {
		res, err := client.Verify(t.Context(), sampleJWT)
		assert.NoError(t, err)

		assert.Equal(t, "Z-25-17156/25", res.RegistrationNumber)
		assert.Equal(t, pseb.CertificateTypeCompany, res.Type)
		assert.Equal(t, "OUTSENTIA (PRIVATE) LIMITED", res.Name)
		assert.True(t, res.IsValid)
		assert.False(t, res.IssuedAt.IsZero())
		assert.False(t, res.ExpiresAt.IsZero())
	})
}
