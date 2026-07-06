package pseb

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/mistermoe/httpr"
	"github.com/mistermoe/jose/jwt"
)

// DefaultBaseURL is the base URL of the PSEB portal API used to verify
// certificates. It is the default host a [Client] created with [New] targets.
const DefaultBaseURL = "https://api.techdestination.com"

// verifyPath is the PSEB portal endpoint that verifies a certificate JWT.
const verifyPath = "/api/verify-certificate"

// defaultTimeout bounds a [Client]'s HTTP requests so callers using a context
// without a deadline do not hang indefinitely if the PSEB portal is unreachable.
const defaultTimeout = 15 * time.Second

// ErrCertificateInvalid is returned by [Client.Verify] when the PSEB portal
// reports that a certificate is not valid (revoked, expired, or unrecognized).
// The accompanying [VerificationResult] is still returned so callers can inspect
// the reported details.
var ErrCertificateInvalid = errors.New("certificate is invalid or expired")

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
	defaults := []httpr.ClientOption{
		httpr.BaseURL(DefaultBaseURL),
		httpr.Timeout(defaultTimeout),
	}

	return &Client{http: httpr.NewClient(append(defaults, opts...)...)}
}

// VerificationResult is the certificate data the PSEB portal returns when a
// certificate JWT is verified. Unlike the claims read locally from the JWT in
// [UnverifiedCertificate], these values are reported by PSEB and include the registered
// entity's name and an authoritative validity flag.
type VerificationResult struct {
	// RegistrationNumber is the PSEB registration number of the holder,
	// e.g. "Z-25-17156/25".
	RegistrationNumber string `json:"registration_number"`

	// Type is the registration type (company or individual/freelancer).
	Type CertificateType `json:"type"`

	// Name is the registered name of the software exporter as recorded by PSEB,
	// e.g. the legal company name "OUTSENTIA (PRIVATE) LIMITED". This is not
	// present in the certificate JWT and is only available via verification.
	Name string `json:"name"`

	// IssuedAt is when PSEB issued the certificate (from the "iat" claim),
	// normalized to UTC.
	IssuedAt time.Time `json:"issued_at"`

	// ExpiresAt is when the certificate expires (from the "exp" claim),
	// normalized to UTC.
	ExpiresAt time.Time `json:"expires_at"`

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
		IsValid        bool   `json:"isValid"`
	} `json:"data"`
}

// Verify submits a PSEB certificate JWT to the PSEB portal and returns the
// certificate data the portal reports for it.
//
// The token is the compact JWT extracted from a certificate's QR code (see
// [UnverifiedCertificate.JWT]). Verify first checks that the token is a well-formed JWT,
// then POSTs it to the portal's verification endpoint. Because PSEB certificates
// are signed with a secret only PSEB holds, this network call is what
// authoritatively establishes a certificate's authenticity and validity; the
// returned [VerificationResult.IsValid] reflects PSEB's own determination.
//
// It returns an error if the token is not a valid JWT, if the request fails, or
// if the portal responds with a non-2xx status. If the portal reports the
// certificate as not valid, Verify returns the populated [VerificationResult]
// together with [ErrCertificateInvalid] so the invalid result cannot be mistaken
// for a valid one by callers that only check the error.
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
		RegistrationNumber: data.RegistrationNo,
		Type:               CertificateType(data.Type),
		Name:               data.Name,
		IssuedAt:           time.Unix(data.IAT, 0).UTC(),
		ExpiresAt:          time.Unix(data.EXP, 0).UTC(),
		IsValid:            data.IsValid,
	}

	if !data.IsValid {
		return result, ErrCertificateInvalid
	}

	return result, nil
}
