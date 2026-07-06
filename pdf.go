package pseb

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"io"
	"net/url"
	"strings"
	"time"

	// Register image decoders used by pdfcpu's extracted images.
	_ "image/jpeg"
	_ "image/png"

	"github.com/makiuchi-d/gozxing"
	"github.com/makiuchi-d/gozxing/qrcode"
	"github.com/mistermoe/jose/jwt"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	_ "golang.org/x/image/tiff"
)

// CertificateType identifies the kind of PSEB registration a certificate
// represents. PSEB registers two kinds of software exporters: companies and
// individuals (freelancers). The value mirrors the "type" claim in the
// certificate JWT verbatim, so it is safe to compare against the constants below
// but may hold an unrecognized value if PSEB introduces new types.
type CertificateType string

const (
	// CertificateTypeCompany is a PSEB registration issued to a company
	// (a registered legal entity, e.g. a "(PRIVATE) LIMITED" company).
	CertificateTypeCompany CertificateType = "company"
	// CertificateTypeIndividual is a PSEB registration issued to an individual
	// software exporter (a freelancer).
	CertificateTypeIndividual CertificateType = "individual"
)

// UnverifiedCertificate contains claims extracted locally from a PSEB certificate's QR code.
// WARNING: The JWT signature is NOT verified during extraction because PSEB uses a symmetric secret.
// Do NOT use this data for authorization without passing the JWT to Client.Verify().
type UnverifiedCertificate struct {
	// PSEBHostedVerificationURL is the URL encoded in the certificate's QR code.
	// It points at the PSEB portal's public verification page for this
	// certificate and embeds the JWT as its final path segment, e.g.
	// https://portal.techdestination.com/verify-certificate/<jwt>.
	PSEBHostedVerificationURL string `json:"pseb_hosted_verification_url"`

	// JWT is the raw, compact JSON Web Token taken from the verification URL.
	// It is the signed credential that encodes the certificate's data and is the
	// value to pass to [Client.Verify].
	JWT string `json:"jwt"`

	// RegistrationNumber is the PSEB registration number assigned to the holder,
	// e.g. "Z-25-17156/25". This is the identifier printed on the certificate
	// that uniquely identifies the registered software exporter.
	RegistrationNumber string `json:"registration_number"`

	// Type is the registration type (company or individual/freelancer).
	Type CertificateType `json:"type"`

	// IssuedAt is when PSEB issued the certificate, taken from the JWT "iat"
	// (issued-at) claim and normalized to UTC.
	IssuedAt time.Time `json:"issued_at"`

	// ExpiresAt is when the certificate expires and must be renewed, taken from
	// the JWT "exp" (expiry) claim and normalized to UTC. PSEB registrations are
	// time-bound; after this instant the certificate is no longer current.
	ExpiresAt time.Time `json:"expires_at"`
}

// psebVerificationHost is the host that appears in a PSEB certificate's QR-code
// verification URL. It is used to distinguish the certificate QR code from any
// unrelated QR codes that may be present in the PDF.
const psebVerificationHost = "portal.techdestination.com"

var (
	// ErrNoQRCode is returned by [ExtractCertificate] when the PDF contains no
	// image that decodes as a QR code.
	ErrNoQRCode = errors.New("no QR code found in PDF")
	// ErrNoJWT is returned by [ExtractCertificate] when the QR code was decoded
	// but does not contain a JWT-shaped token.
	ErrNoJWT = errors.New("no JWT found in QR code")
	// ErrInsecureAlgorithm is returned by [ExtractCertificate] when the JWT
	// header declares no algorithm or the "none" algorithm, which cannot be
	// trusted and defends against algorithm-confusion attacks.
	ErrInsecureAlgorithm = errors.New("token header specifies 'none' or empty algorithm")
)

// ExtractCertificate reads a PSEB certificate PDF, decodes the QR code printed
// on it, and returns the [UnverifiedCertificate] it encodes.
//
// It extracts the images embedded in the PDF, decodes the first one that is a
// readable QR code, parses the verification URL to capture the JWT, and decodes
// the JWT's claims to populate the registration number, type, and timestamps.
// The JWT signature is not checked here; use [Client.Verify] to confirm a
// certificate's authenticity and current validity with PSEB.
//
// Only the certificate's first page is inspected. The provided ctx can be used
// to cancel a stalled parse; its error is returned if it is cancelled while
// images are being decoded.
//
// It returns [ErrNoQRCode] if no QR code can be decoded from the PDF,
// [ErrNoJWT] if the decoded QR code does not contain a JWT, or
// [ErrInsecureAlgorithm] if the JWT header declares no or the "none" algorithm.
func ExtractCertificate(ctx context.Context, pdf []byte) (*UnverifiedCertificate, error) {
	qrText, err := decodeQRFromPDF(ctx, pdf)
	if err != nil {
		return nil, err
	}

	verificationURL, token, err := parseVerificationURL(qrText)
	if err != nil {
		return nil, err
	}

	decoded, err := jwt.Decode(token)
	if err != nil {
		return nil, fmt.Errorf("failed to decode JWT: %w", err)
	}

	if alg := strings.ToLower(decoded.Header.ALG); alg == "" || alg == "none" {
		return nil, ErrInsecureAlgorithm
	}

	registrationNumber, ok := decoded.Claims.Misc["registrationNo"].(string)
	if !ok || registrationNumber == "" {
		return nil, fmt.Errorf("missing required claim: registrationNo")
	}

	certType, ok := decoded.Claims.Misc["type"].(string)
	if !ok || certType == "" {
		return nil, fmt.Errorf("missing required claim: type")
	}

	// Leave timestamps as the zero Time when the claim is absent rather than
	// mapping a 0 epoch to 1970, which would misrepresent a missing value.
	var issuedAt, expiresAt time.Time
	if decoded.Claims.IssuedAt != 0 {
		issuedAt = time.Unix(decoded.Claims.IssuedAt, 0).UTC()
	}
	if decoded.Claims.Expiration != 0 {
		expiresAt = time.Unix(decoded.Claims.Expiration, 0).UTC()
	}

	return &UnverifiedCertificate{
		PSEBHostedVerificationURL: verificationURL,
		JWT:                       token,
		RegistrationNumber:        registrationNumber,
		Type:                      CertificateType(certType),
		IssuedAt:                  issuedAt,
		ExpiresAt:                 expiresAt,
	}, nil
}

// decodeQRFromPDF extracts the images from the PDF's first page and returns the
// text of the first one that decodes as a QR code.
func decodeQRFromPDF(ctx context.Context, pdf []byte) (string, error) {
	conf := model.NewDefaultConfiguration()

	// PSEB certificates are strictly one page; restricting extraction to page 1
	// prevents multi-page collection bombs.
	pages, err := api.ExtractImagesRaw(bytes.NewReader(pdf), []string{"1"}, conf)
	if err != nil {
		return "", fmt.Errorf("failed to extract images from PDF: %w", err)
	}

	for _, page := range pages {
		for _, img := range page {
			if err := ctx.Err(); err != nil {
				return "", err
			}

			text, ok := decodeQRFromImage(img)
			if !ok {
				continue
			}

			// Map iteration order is non-deterministic, so a PDF with several
			// QR codes could otherwise yield a different result each run. Only
			// accept the QR that carries a PSEB verification URL.
			if strings.Contains(text, psebVerificationHost) {
				return text, nil
			}
		}
	}

	return "", ErrNoQRCode
}

// decodeQRFromImage attempts to decode a QR code from a single extracted image.
func decodeQRFromImage(img model.Image) (string, bool) {
	if img.Reader == nil {
		return "", false
	}

	// Prevent Pixel Flood bombs (e.g., max 4000x4000 = 16M pixels).
	if img.Width*img.Height > 16_000_000 {
		return "", false // Skip maliciously oversized images
	}

	// 10MB limit to prevent decompression bombs.
	lr := io.LimitReader(img, 10*1024*1024)
	raw, err := io.ReadAll(lr)
	if err != nil {
		return "", false
	}

	decoded, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return "", false
	}

	bitmap, err := gozxing.NewBinaryBitmapFromImage(decoded)
	if err != nil {
		return "", false
	}

	hints := map[gozxing.DecodeHintType]interface{}{
		gozxing.DecodeHintType_TRY_HARDER: true,
	}

	result, err := qrcode.NewQRCodeReader().Decode(bitmap, hints)
	if err != nil {
		return "", false
	}

	return result.GetText(), true
}

// parseVerificationURL extracts the verification URL and JWT from QR text. The
// QR encodes a URL of the form https://host/verify-certificate/<jwt>; if the QR
// instead contains a bare JWT it is returned as-is.
func parseVerificationURL(qrText string) (verificationURL, token string, err error) {
	qrText = strings.TrimSpace(qrText)

	parsed, parseErr := url.Parse(qrText)
	if parseErr != nil || parsed.Scheme == "" {
		if isJWT(qrText) {
			return qrText, qrText, nil
		}

		return "", "", ErrNoJWT
	}

	segments := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	token = segments[len(segments)-1]

	if !isJWT(token) {
		return "", "", ErrNoJWT
	}

	return qrText, token, nil
}

// isJWT reports whether s has the three-part shape of a compact JWT.
func isJWT(s string) bool {
	const numParts = 3
	return len(strings.Split(s, ".")) == numParts
}
