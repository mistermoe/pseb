package pseb

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/color"
	"io"
	"net/url"
	"regexp"
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
// freelancers. The value mirrors the "type" claim in the certificate JWT
// verbatim, so it is safe to compare against the constants below but may hold an
// unrecognized value if PSEB introduces new types.
type CertificateType string

const (
	// CertificateTypeCompany is a PSEB registration issued to a company
	// (a registered legal entity, e.g. a "(PRIVATE) LIMITED" company).
	CertificateTypeCompany CertificateType = "company"
	// CertificateTypeFreelancer is a PSEB registration issued to an individual
	// software exporter (a freelancer).
	CertificateTypeFreelancer CertificateType = "freelancer"
)

// Certificate is a PSEB registration certificate as read from the QR code
// printed on the certificate PDF. The fields below come from the QR's
// verification URL and the claims of the signed JWT it carries; they are not
// independently verified (see [Client.Verify] for that).
type Certificate struct {
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

	// Type is the registration type (company or freelancer).
	Type CertificateType `json:"type"`

	// IssuedAt is the JWT "iat" (issued-at) claim, normalized to UTC. It is when
	// the certificate's verification token was issued.
	IssuedAt time.Time `json:"issued_at"`

	// JWTExpiresAt is the JWT "exp" (expiry) claim, normalized to UTC. This is
	// the expiry of the verification token itself, which PSEB issues with a
	// short (~90-day) lifetime, and is NOT the end of the registration's
	// validity period. The registration's actual expiry is only reported by the
	// portal (see [VerificationResult.RegistrationExpiresAt]) and is not
	// available offline from the JWT alone.
	JWTExpiresAt time.Time `json:"jwt_expires_at"`
}

var (
	// ErrNoQRCode is returned by [ExtractCertificate] when the PDF contains no
	// image that decodes as a QR code.
	ErrNoQRCode = errors.New("no QR code found in PDF")
	// ErrNoJWT is returned by [ExtractCertificate] when the QR code was decoded
	// but does not contain a JWT-shaped token.
	ErrNoJWT = errors.New("no JWT found in QR code")
	// ErrNoCNIC is returned by [ExtractCNIC] when no CNIC can be found in the
	// PDF's text layer.
	ErrNoCNIC = errors.New("no CNIC found in PDF")
)

// ExtractCertificate reads a PSEB certificate PDF, decodes the QR code printed
// on it, and returns the [Certificate] it encodes.
//
// It extracts the images embedded in the PDF, decodes the first one that is a
// readable QR code, parses the verification URL to capture the JWT, and decodes
// the JWT's claims to populate the registration number, type, and timestamps.
// The JWT signature is not checked here; use [Client.Verify] to confirm a
// certificate's authenticity and current validity with PSEB.
//
// PSEB embeds the QR as a very small, full-bleed raster (roughly two pixels per
// module, with no surrounding quiet zone). Off-the-shelf QR readers cannot lock
// onto a symbol that small at any scale, so decoding falls back to
// reconstructing the module grid directly; see [decodeQRFromImage] for details.
//
// It returns [ErrNoQRCode] if no QR code can be decoded from the PDF, or
// [ErrNoJWT] if the decoded QR code does not contain a JWT.
func ExtractCertificate(pdf []byte) (*Certificate, error) {
	qrText, err := decodeQRFromPDF(pdf)
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

	registrationNumber, _ := decoded.Claims.Misc["registrationNo"].(string)
	certType, _ := decoded.Claims.Misc["type"].(string)

	return &Certificate{
		PSEBHostedVerificationURL: verificationURL,
		JWT:                       token,
		RegistrationNumber:        registrationNumber,
		Type:                      CertificateType(certType),
		IssuedAt:                  time.Unix(decoded.Claims.IssuedAt, 0).UTC(),
		JWTExpiresAt:              time.Unix(decoded.Claims.Expiration, 0).UTC(),
	}, nil
}

// decodeQRFromPDF extracts every image from the PDF and returns the text of the
// first one that decodes as a QR code.
func decodeQRFromPDF(pdf []byte) (string, error) {
	conf := model.NewDefaultConfiguration()

	pages, err := api.ExtractImagesRaw(bytes.NewReader(pdf), nil, conf)
	if err != nil {
		return "", fmt.Errorf("failed to extract images from PDF: %w", err)
	}

	for _, page := range pages {
		for _, img := range page {
			text, ok := decodeQRFromImage(img)
			if ok {
				return text, nil
			}
		}
	}

	return "", ErrNoQRCode
}

// decodeQRFromImage attempts to decode a QR code from a single extracted image
// using a two-stage strategy.
//
// First it hands the image to gozxing directly ([decodeQR]). This succeeds for
// well-formed QR images (a clean raster with several pixels per module and a
// quiet zone) - for example the QR on a company certificate or one produced by a
// normal QR encoder.
//
// PSEB freelancer certificates, however, embed the QR as a tiny full-bleed
// raster: the symbol fills the image edge-to-edge at roughly two pixels per
// module with no quiet zone. gozxing's detector locates a symbol by finding its
// three finder patterns and a quiet zone; at ~14 pixels per finder pattern and
// with no margin, detection fails, and upscaling does not help because the
// underlying raster's module boundaries are already ambiguous. When the direct
// decode fails, decoding falls back to [decodeQRByModuleGrid], which sidesteps
// detection by reconstructing the module grid from the raster directly.
func decodeQRFromImage(img model.Image) (string, bool) {
	if img.Reader == nil {
		return "", false
	}

	raw, err := io.ReadAll(img)
	if err != nil {
		return "", false
	}

	decoded, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return "", false
	}

	if text, ok := decodeQR(decoded); ok {
		return text, true
	}

	return decodeQRByModuleGrid(decoded)
}

// decodeQR runs gozxing's QR reader directly against an image.
func decodeQR(img image.Image) (string, bool) {
	bitmap, err := gozxing.NewBinaryBitmapFromImage(img)
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

// qr module geometry: symbols range from 21x21 (version 1) to 177x177
// (version 40) modules per side, in steps of 4.
const (
	qrMinModules  = 21
	qrMaxModules  = 177
	qrModuleStep  = 4
	qrRenderScale = 8 // pixels per module in the crisp re-render
	qrQuietZone   = 4 // quiet-zone width (modules) added around the re-render
)

// decodeQRByModuleGrid recovers a QR from a small, full-bleed raster where the
// code fills the image edge-to-edge, sidestepping the finder-pattern detection
// that fails on such images.
//
// The trick is that when the symbol fills the image, the module grid is implied
// by the geometry alone: an n-module symbol maps module (i, j) to a fixed pixel
// region, independent of the image's contents. The true module count n is not
// known up front, so this tries every valid QR size (21 to 177 modules, in steps
// of 4). For each candidate n it samples the center of every module to rebuild
// the n x n matrix, then re-renders that matrix as a large, crisp image with a
// generous quiet zone and hands it back to gozxing - which now decodes easily
// because the reconstruction is pixel-perfect and well-margined.
//
// A wrong n produces a garbled matrix whose format and error-correction data do
// not check out, so gozxing rejects it; only the correct grid yields a valid QR.
// A successful decode is therefore self-validating, and there is no risk of a
// wrong size returning bogus text.
//
// This relies on the QR occupying the whole image, being axis-aligned, and not
// being rotated - all of which hold for PSEB certificate QRs. Images that are
// not roughly square (e.g. logos or banners) are skipped up front.
func decodeQRByModuleGrid(img image.Image) (string, bool) {
	b := img.Bounds()
	if w, h := b.Dx(), b.Dy(); w < qrMinModules || h < qrMinModules || !nearlySquare(w, h) {
		return "", false
	}

	for modules := qrMinModules; modules <= qrMaxModules; modules += qrModuleStep {
		crisp := renderModuleGrid(sampleModuleGrid(img, modules), qrRenderScale, qrQuietZone)
		if text, ok := decodeQR(crisp); ok {
			return text, true
		}
	}

	return "", false
}

// nearlySquare reports whether w and h are within 10% of each other.
func nearlySquare(w, h int) bool {
	hi, lo := w, h
	if hi < lo {
		hi, lo = lo, hi
	}

	return hi-lo <= hi/10
}

// sampleModuleGrid samples the center of each of an n x n module grid spanning
// the whole image and returns a boolean matrix where true means a dark module.
func sampleModuleGrid(src image.Image, n int) [][]bool {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()

	grid := make([][]bool, n)
	for j := 0; j < n; j++ {
		grid[j] = make([]bool, n)
		for i := 0; i < n; i++ {
			px := b.Min.X + (i*w)/n + w/(2*n)
			py := b.Min.Y + (j*h)/n + h/(2*n)
			r, g, bl, _ := src.At(px, py).RGBA()
			lum := (299*(r>>8) + 587*(g>>8) + 114*(bl>>8)) / 1000
			grid[j][i] = lum < 128
		}
	}

	return grid
}

// renderModuleGrid draws a module matrix as a crisp black-on-white image at
// scale pixels per module, surrounded by a quiet-zone border of quiet modules.
func renderModuleGrid(grid [][]bool, scale, quiet int) *image.Gray {
	n := len(grid)
	dim := (n + 2*quiet) * scale

	out := image.NewGray(image.Rect(0, 0, dim, dim))
	for i := range out.Pix {
		out.Pix[i] = 0xff
	}

	for j := 0; j < n; j++ {
		for i := 0; i < n; i++ {
			if !grid[j][i] {
				continue
			}

			for dy := 0; dy < scale; dy++ {
				for dx := 0; dx < scale; dx++ {
					out.SetGray((i+quiet)*scale+dx, (j+quiet)*scale+dy, color.Gray{Y: 0})
				}
			}
		}
	}

	return out
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

// cnicDigits is the number of digits in a Pakistani CNIC.
const cnicDigits = 13

var (
	// cnicLabelRe captures the digits following a "CNIC" label. The value may be
	// printed bare (3520122720739) or dash-separated (35201-2272073-9); the
	// character class deliberately excludes newlines so the match stops at the
	// end of the value's line.
	cnicLabelRe = regexp.MustCompile(`(?is)CNIC[^0-9]*([0-9][0-9\- ]{11,20}[0-9])`)
	// cnicBareRe matches a standalone run of exactly 13 digits, used as a
	// fallback when the "CNIC" label cannot be located.
	cnicBareRe = regexp.MustCompile(`\b\d{13}\b`)
	// nonDigitRe strips separators (dashes, spaces) from a captured CNIC.
	nonDigitRe = regexp.MustCompile(`\D`)
)

// ExtractCNIC reads a PSEB certificate PDF and returns the holder's 13-digit
// CNIC (the Pakistani national identity number printed on freelancer
// certificates).
//
// Unlike the data returned by [ExtractCertificate], the CNIC is not encoded in
// the QR code, the JWT claims, or the PSEB verification response - it appears
// only as text printed on the certificate. It is therefore recovered from the
// PDF's text layer: each page's content streams are parsed for their text-showing
// operators (Tj / TJ) to reconstruct the visible text, and the CNIC is located
// within it.
//
// Matching prefers a value that follows a "CNIC" label, accepting both the bare
// (3520122720739) and dash-separated (35201-2272073-9) forms and normalizing to
// 13 digits. If the label cannot be located it falls back to any standalone run
// of exactly 13 digits, which is unambiguous on a PSEB certificate (the
// registration number, phone, and fax never form 13 consecutive digits).
//
// It returns [ErrNoCNIC] if no CNIC can be found. This relies on the certificate
// having a real text layer (as current PSEB certificates do); it does not
// perform OCR on image-only (scanned) PDFs.
func ExtractCNIC(pdf []byte) (string, error) {
	text, err := extractText(pdf)
	if err != nil {
		return "", err
	}

	if cnic, ok := findCNIC(text); ok {
		return cnic, nil
	}

	return "", ErrNoCNIC
}

// findCNIC locates a CNIC within extracted PDF text, preferring a value that
// follows a "CNIC" label and falling back to any standalone 13-digit run.
func findCNIC(text string) (string, bool) {
	if m := cnicLabelRe.FindStringSubmatch(text); m != nil {
		if digits := nonDigitRe.ReplaceAllString(m[1], ""); len(digits) == cnicDigits {
			return digits, true
		}
	}

	if m := cnicBareRe.FindString(text); m != "" {
		return m, true
	}

	return "", false
}

// extractText returns the visible text of a PDF. pdfcpu has no text-extraction
// API, but [api.ExtractContent] yields the decoded content stream per page; this
// parses the text-showing operators from it. Each Tj/TJ operator becomes its own
// line so labels and their values remain separable.
func extractText(pdf []byte) (string, error) {
	conf := model.NewDefaultConfiguration()

	var sb strings.Builder

	digest := func(r io.Reader, _ int) error {
		content, err := io.ReadAll(r)
		if err != nil {
			return err
		}

		sb.WriteString(textFromContentStream(content))

		return nil
	}

	if err := api.ExtractContent(bytes.NewReader(pdf), nil, digest, conf); err != nil {
		return "", fmt.Errorf("failed to extract content from PDF: %w", err)
	}

	return sb.String(), nil
}

// textFromContentStream parses a decoded PDF content stream and returns the text
// shown by its Tj/TJ (and ' ") operators. String operands, whether literal
// "(...)" or hex "<...>", are concatenated within an operator; TJ kerning
// numbers are ignored. Each text-showing operator emits a trailing newline.
func textFromContentStream(content []byte) string {
	var (
		out strings.Builder
		run strings.Builder
		i   int
		n   = len(content)
	)

	flush := func() {
		if run.Len() > 0 {
			out.WriteString(run.String())
			out.WriteByte('\n')
			run.Reset()
		}
	}

	for i < n {
		switch c := content[i]; c {
		case '(':
			s, next := readLiteralString(content, i+1)
			run.WriteString(s)
			i = next
		case '<':
			// "<<" begins a dictionary, not a hex string; skip it.
			if i+1 < n && content[i+1] == '<' {
				i += 2
				continue
			}
			s, next := readHexString(content, i+1)
			run.WriteString(s)
			i = next
		case 'T':
			// Tj and TJ are the only text-showing operators starting with 'T';
			// Tf, Td, Tm, etc. are positioning/state operators and are ignored.
			if i+1 < n && (content[i+1] == 'j' || content[i+1] == 'J') {
				flush()
				i += 2
				continue
			}
			i++
		case '\'', '"':
			// The ' and " operators also show a preceding string.
			flush()
			i++
		default:
			i++
		}
	}

	flush()

	return out.String()
}

// readLiteralString reads a PDF literal string body starting at b[i] (just after
// the opening '('), handling escape sequences and balanced parentheses, and
// returns the decoded bytes and the index just past the closing ')'.
func readLiteralString(b []byte, i int) (string, int) {
	var sb strings.Builder

	depth := 1
	n := len(b)

	for i < n {
		c := b[i]

		if c == '\\' {
			i++
			if i >= n {
				break
			}

			switch e := b[i]; e {
			case 'n':
				sb.WriteByte('\n')
			case 'r':
				sb.WriteByte('\r')
			case 't':
				sb.WriteByte('\t')
			case 'b':
				sb.WriteByte('\b')
			case 'f':
				sb.WriteByte('\f')
			case '\n':
				// Line continuation: the escaped newline is dropped.
			case '\r':
				if i+1 < n && b[i+1] == '\n' {
					i++
				}
			default:
				if e >= '0' && e <= '7' {
					oct := int(e - '0')
					for k := 0; k < 2 && i+1 < n && b[i+1] >= '0' && b[i+1] <= '7'; k++ {
						i++
						oct = oct*8 + int(b[i]-'0')
					}
					sb.WriteByte(byte(oct))
				} else {
					// Covers escaped '(', ')', '\\', and any other character.
					sb.WriteByte(e)
				}
			}

			i++

			continue
		}

		switch c {
		case '(':
			depth++
			sb.WriteByte(c)
		case ')':
			depth--
			if depth == 0 {
				return sb.String(), i + 1
			}
			sb.WriteByte(c)
		default:
			sb.WriteByte(c)
		}

		i++
	}

	return sb.String(), i
}

// readHexString reads a PDF hex string body starting at b[i] (just after the
// opening '<'), and returns the decoded bytes and the index just past the
// closing '>'. Whitespace within the body is ignored and an odd final nibble is
// padded with a trailing zero, per the PDF spec.
func readHexString(b []byte, i int) (string, int) {
	var hexits []byte

	n := len(b)

	for i < n && b[i] != '>' {
		if c := b[i]; (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') {
			hexits = append(hexits, c)
		}

		i++
	}

	if i < n {
		i++ // skip the closing '>'
	}

	if len(hexits)%2 == 1 {
		hexits = append(hexits, '0')
	}

	decoded := make([]byte, len(hexits)/2)
	if _, err := hex.Decode(decoded, hexits); err != nil {
		return "", i
	}

	return string(decoded), i
}
