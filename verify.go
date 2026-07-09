package pseb

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/mistermoe/httpr"
	"github.com/mistermoe/jose/jwt"
)

// ErrCertificateInvalid is returned by [Client.Verify] when the PSEB portal
// reports that the certificate is not valid (its "isValid" flag is false). The
// accompanying [VerificationResult] is still returned and populated so callers
// can inspect the details PSEB reported.
var ErrCertificateInvalid = errors.New("PSEB reports certificate is not valid")

// DefaultBaseURL is the base URL of the PSEB portal API used to verify
// certificates. It is the default host a [Client] created with [New] targets.
const DefaultBaseURL = "https://api.techdestination.com"

// verifyPath is the PSEB portal endpoint that verifies a certificate JWT.
const verifyPath = "/api/verify-certificate"

// Client verifies PSEB certificates against the PSEB portal's verification
// endpoint. It is safe for concurrent use and should be reused across calls.
type Client struct {
	http *httpr.Client
}

// New creates a [Client] that talks to the PSEB portal verification API.
//
// By default it targets [DefaultBaseURL]. Pass httpr client options to customize
// the underlying HTTP behavior, for example httpr.BaseURL(...) to point at a
// different host or httpr.HTTPClient(...) to supply a custom *http.Client (e.g.
// one with a timeout, or a recording transport in tests).
func New(opts ...httpr.ClientOption) *Client {
	defaults := []httpr.ClientOption{httpr.BaseURL(DefaultBaseURL)}

	return &Client{http: httpr.NewClient(append(defaults, opts...)...)}
}

// VerificationResult is the certificate data the PSEB portal returns when a
// certificate JWT is verified. Unlike the claims read locally from the JWT in
// [Certificate], these values are reported by PSEB and include the registered
// entity's name and an authoritative validity flag.
type VerificationResult struct {
	// RegistrationNumber is the PSEB registration number of the holder,
	// e.g. "Z-25-17156/25".
	RegistrationNumber string `json:"registration_number"`

	// Type is the registration type (company or freelancer).
	Type CertificateType `json:"type"`

	// Name is the registered name of the software exporter as recorded by PSEB,
	// e.g. the legal company name "OUTSENTIA (PRIVATE) LIMITED". This is not
	// present in the certificate JWT and is only available via verification.
	Name string `json:"name"`

	// IssuedAt is the JWT "iat" (issued-at) timestamp, normalized to UTC. Note
	// this is when the certificate's token was issued, which may differ from the
	// start of the registration's validity window (see ValidFrom).
	IssuedAt time.Time `json:"issued_at"`

	// JWTExpiresAt is the JWT "exp" timestamp, normalized to UTC. This is the
	// expiry of the certificate's verification token (PSEB issues it with a
	// short, ~90-day lifetime), NOT the end of the registration's validity
	// period. For the registration's actual expiry use RegistrationExpiresAt.
	JWTExpiresAt time.Time `json:"jwt_expires_at"`

	// ValidFrom is the start of the registration's validity window as reported
	// by PSEB, a coarse human-readable label such as "May 2026". It is empty if
	// the portal did not report it.
	ValidFrom string `json:"valid_from"`

	// ValidTill is the end of the registration's validity window as reported by
	// PSEB, a coarse human-readable label such as "Apr 2027". This is the
	// certificate's real expiry (distinct from the token's JWTExpiresAt) and is
	// empty if the portal did not report it.
	ValidTill string `json:"valid_till"`

	// RegistrationExpiresAt is ValidTill parsed into a timestamp, normalized to
	// UTC. PSEB only reports month granularity, so this is set to the first
	// instant of the reported month (e.g. "Apr 2027" becomes 2027-04-01
	// 00:00:00 UTC). It is the zero time if ValidTill is empty or unparseable.
	RegistrationExpiresAt time.Time `json:"registration_expires_at"`

	// IsValid reports whether PSEB considers the certificate currently valid.
	// This is the authoritative validity signal: it reflects PSEB's own check
	// (signature and expiry) rather than any local inspection of the JWT.
	IsValid bool `json:"is_valid"`
}

// verifyEnvelope mirrors the (camelCase) wire format returned by the endpoint.
type verifyEnvelope struct {
	Message string `json:"message"`
	Data    struct {
		RegistrationNo string `json:"registrationNo"`
		Type           string `json:"type"`
		Name           string `json:"name"`
		IAT            int64  `json:"iat"`
		EXP            int64  `json:"exp"`
		ValidFrom      string `json:"validFrom"`
		ValidTill      string `json:"validTill"`
		IsValid        bool   `json:"isValid"`
	} `json:"data"`
}

// validityMonthLayout is the "Mon YYYY" format PSEB uses for the validFrom and
// validTill fields, e.g. "Apr 2027".
const validityMonthLayout = "Jan 2006"

// parseValidityMonth parses a PSEB "Mon YYYY" validity label (e.g. "Apr 2027")
// into the first instant of that month in UTC. It returns the zero time if s is
// empty or not in the expected format, since the field is coarse and best-effort.
func parseValidityMonth(s string) time.Time {
	t, err := time.Parse(validityMonthLayout, strings.TrimSpace(s))
	if err != nil {
		return time.Time{}
	}

	return t.UTC()
}

// Verify submits a PSEB certificate JWT to the PSEB portal and returns the
// certificate data the portal reports for it.
//
// The token is the compact JWT extracted from a certificate's QR code (see
// [Certificate.JWT]). Verify first checks that the token is a well-formed JWT,
// then POSTs it to the portal's verification endpoint. Because PSEB certificates
// are signed with a secret only PSEB holds, this network call is what
// authoritatively establishes a certificate's authenticity and validity; the
// returned [VerificationResult.IsValid] reflects PSEB's own determination.
//
// It returns an error if the token is not a valid JWT, if the request fails, or
// if the portal responds with a non-2xx status. If the portal responds
// successfully but reports the certificate as not valid, Verify returns the
// populated result together with [ErrCertificateInvalid]; match it with
// errors.Is to distinguish an invalid certificate from a transport failure.
func (c *Client) Verify(ctx context.Context, token string) (*VerificationResult, error) {
	if _, err := jwt.Decode(token); err != nil {
		return nil, fmt.Errorf("invalid PSEB certificate JWT: %w", err)
	}

	var (
		envelope verifyEnvelope
		errResp  verifyEnvelope
	)

	resp, err := c.http.Post(
		ctx,
		verifyPath,
		httpr.RequestBodyJSON(map[string]string{"token": token}),
		httpr.ResponseBodyJSON(&envelope, &errResp),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to verify certificate: %w", err)
	}

	if resp.StatusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("certificate verification failed (status %d): %s", resp.StatusCode, errResp.Message)
	}

	data := envelope.Data

	result := &VerificationResult{
		RegistrationNumber:    data.RegistrationNo,
		Type:                  CertificateType(data.Type),
		Name:                  data.Name,
		IssuedAt:              time.Unix(data.IAT, 0).UTC(),
		JWTExpiresAt:          time.Unix(data.EXP, 0).UTC(),
		ValidFrom:             data.ValidFrom,
		ValidTill:             data.ValidTill,
		RegistrationExpiresAt: parseValidityMonth(data.ValidTill),
		IsValid:               data.IsValid,
	}

	if !result.IsValid {
		return result, ErrCertificateInvalid
	}

	return result, nil
}
