// Command gensample generates synthetic PSEB certificate PDFs used as test
// fixtures. Each fixture mirrors the layout and field labels of a real PSEB
// certificate but contains only fake data, so the fixtures can be committed to
// a public repository without exposing anyone's personal information.
//
// Each certificate is "full": it embeds a QR code encoding a PSEB-style
// verification URL whose final path segment is a JWT. The JWT is signed with
// HS256 (matching real PSEB certificates) using a fixed secret kept at
// testdata/jwt_secret.key, so regenerating the fixtures is deterministic. The
// secret is created on first run if it does not already exist.
//
// Regenerate the fixtures from the repository root with:
//
//	go run ./cmd/gensample
package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
	"path/filepath"

	"github.com/makiuchi-d/gozxing/qrcode/decoder"
	"github.com/makiuchi-d/gozxing/qrcode/encoder"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// bannerPNG is the PSEB certificate header banner (Tech Nation Pakistan, Markaz,
// and PSEB logos), embedded so the generator works regardless of the working
// directory.
//
//go:embed banner.png
var bannerPNG []byte

const (
	// secretFile holds the fixed HS256 signing secret (hex-encoded).
	secretFile = "testdata/jwt_secret.key"
	// testdataDir is where generated fixtures are written.
	testdataDir = "testdata"
	// verifyURLBase mirrors the PSEB portal's public verification URL. The JWT
	// is appended as the final path segment.
	verifyURLBase = "https://portal.techdestination.com/verify-certificate/"
	// qrPixelsPerModule matches how PSEB embeds its QR: a tiny, full-bleed raster
	// with roughly two pixels per module and no quiet zone. This deliberately
	// exercises the module-grid reconstruction fallback in ExtractCertificate.
	qrPixelsPerModule = 2
)

// Colors approximating the real certificate's palette.
const (
	teal  = "#12D8C3"
	gray  = "#9AA0A6"
	black = "#000000"
)

// font names for the standard-14 Type1 fonts used by PSEB certificates.
const (
	helv     = "Helvetica"
	helvBold = "Helvetica-Bold"
)

// Fixed issued-at / expiry timestamps (Unix seconds) baked into the fake JWTs
// so regeneration stays deterministic.
const (
	fakeIAT = 1777885714 // 2026-05-04T09:08:34Z
	fakeEXP = 1785661714 // 2026-08-01T09:08:34Z
)

// certSpec describes a synthetic certificate to generate.
type certSpec struct {
	out       string // output PDF path
	heading   string // right-column heading, e.g. "Freelancer" or "Company"
	regNo     string // PSEB registration number
	certType  string // JWT "type" claim: "freelancer" or "company"
	name      string // registered name (fake)
	cnic      string // 13-digit CNIC; empty for company certificates
	validFrom string
	validTill string
}

var specs = []certSpec{
	{
		out:       filepath.Join(testdataDir, "pseb_freelancer_sample.pdf"),
		heading:   "Freelancer",
		regNo:     "FL21/PSEB/2026/00000",
		certType:  "freelancer",
		name:      "Jane Doe",
		cnic:      "1234512345671",
		validFrom: "May-2026",
		validTill: "Apr-2027",
	},
	{
		out:       filepath.Join(testdataDir, "pseb_company_sample.pdf"),
		heading:   "Company",
		regNo:     "Z-99-99999/25",
		certType:  "company",
		name:      "Acme (Private) Limited",
		validFrom: "Jan-2026",
		validTill: "Dec-2026",
	},
}

func main() {
	secretPath := flag.String("secret", secretFile, "path to the hex-encoded HS256 signing secret")
	flag.Parse()

	secret, err := loadOrCreateSecret(*secretPath)
	if err != nil {
		log.Fatalf("load signing secret: %v", err)
	}

	for _, spec := range specs {
		if err := generate(spec, secret); err != nil {
			log.Fatalf("generate %s: %v", spec.out, err)
		}
	}
}

// loadOrCreateSecret reads the hex-encoded HMAC secret at path, creating a new
// random 32-byte secret (and writing it) if the file does not yet exist.
func loadOrCreateSecret(path string) ([]byte, error) {
	if data, err := os.ReadFile(path); err == nil {
		return hex.DecodeString(string(bytes.TrimSpace(data)))
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}

	if err := os.WriteFile(path, []byte(hex.EncodeToString(secret)+"\n"), 0o644); err != nil {
		return nil, err
	}

	log.Printf("created new signing secret at %s", path)

	return secret, nil
}

// generate builds one certificate PDF and writes it to spec.out.
func generate(spec certSpec, secret []byte) error {
	token, err := signHS256(map[string]any{
		"registrationNo": spec.regNo,
		"type":           spec.certType,
		"iat":            fakeIAT,
		"exp":            fakeEXP,
	}, secret)
	if err != nil {
		return err
	}

	qrPath, cleanupQR, err := writeQRImage(verifyURLBase + token)
	if err != nil {
		return err
	}
	defer cleanupQR()

	bannerPath, cleanupBanner, err := writeTempFile("pseb-banner-*.png", bannerPNG)
	if err != nil {
		return err
	}
	defer cleanupBanner()

	data, err := json.Marshal(description(spec, bannerPath, qrPath))
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	if err := api.Create(nil, bytes.NewReader(data), &buf, model.NewDefaultConfiguration()); err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(spec.out), 0o755); err != nil {
		return err
	}

	if err := os.WriteFile(spec.out, buf.Bytes(), 0o644); err != nil {
		return err
	}

	log.Printf("wrote %s (%d bytes)", spec.out, buf.Len())

	return nil
}

// signHS256 encodes claims as a compact JWT signed with HS256, matching the
// header PSEB uses ({"alg":"HS256","typ":"JWT"}).
func signHS256(claims map[string]any, secret []byte) (string, error) {
	header, err := json.Marshal(map[string]string{"alg": "HS256", "typ": "JWT"})
	if err != nil {
		return "", err
	}

	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}

	signingInput := base64.RawURLEncoding.EncodeToString(header) + "." +
		base64.RawURLEncoding.EncodeToString(payload)

	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(signingInput))

	return signingInput + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

// writeQRImage encodes text as a QR code PNG in a temporary file and returns its
// path along with a cleanup function to remove it. The QR is rendered directly
// from its module matrix at qrPixelsPerModule pixels per module with no quiet
// zone, mimicking the small, full-bleed QR raster found on real PSEB
// certificates.
func writeQRImage(text string) (path string, cleanup func(), err error) {
	code, encErr := encoder.Encoder_encode(text, decoder.ErrorCorrectionLevel_M, nil)
	if encErr != nil {
		return "", nil, encErr
	}

	matrix := code.GetMatrix()
	w, h := matrix.GetWidth(), matrix.GetHeight()

	img := image.NewGray(image.Rect(0, 0, w*qrPixelsPerModule, h*qrPixelsPerModule))
	for i := range img.Pix {
		img.Pix[i] = 0xff
	}

	for my := 0; my < h; my++ {
		for mx := 0; mx < w; mx++ {
			if matrix.Get(mx, my) != 1 {
				continue
			}

			for dy := 0; dy < qrPixelsPerModule; dy++ {
				for dx := 0; dx < qrPixelsPerModule; dx++ {
					img.SetGray(mx*qrPixelsPerModule+dx, my*qrPixelsPerModule+dy, color.Gray{Y: 0})
				}
			}
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return "", nil, err
	}

	return writeTempFile("pseb-qr-*.png", buf.Bytes())
}

// writeTempFile writes data to a new temporary file matching pattern and returns
// its path along with a cleanup function to remove it. pdfcpu's image primitive
// references images by file path, so embedded/generated images are staged here.
func writeTempFile(pattern string, data []byte) (path string, cleanup func(), err error) {
	f, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", nil, err
	}

	cleanup = func() { _ = os.Remove(f.Name()) }

	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		cleanup()

		return "", nil, err
	}

	if err := f.Close(); err != nil {
		cleanup()

		return "", nil, err
	}

	return f.Name(), cleanup, nil
}

// text builds a positioned text box. Coordinates use an upper-left origin, so x
// grows rightward and y grows downward, matching how the certificate reads.
func text(value string, x, y float64, font string, size int, col string) map[string]any {
	return map[string]any{
		"value": value,
		"pos":   []float64{x, y},
		"font":  map[string]any{"name": font, "size": size, "col": col},
	}
}

// field renders a bold label and its value on the same row.
func field(label, value string, y float64) []map[string]any {
	const (
		labelX = 452
		valueX = 610
		size   = 12
	)

	return []map[string]any{
		text(label, labelX, y, helvBold, size, black),
		text(value, valueX, y, helv, size, black),
	}
}

// description builds the pdfcpu "create" JSON description for a certificate. The
// header banner occupies the top of the page, so the title, details, and QR code
// are positioned below it.
func description(spec certSpec, bannerPath, qrPath string) map[string]any {
	boxes := []map[string]any{
		text("CERTIFICATE OF", 36, 200, helvBold, 26, teal),
		text("REGISTRATION", 36, 234, helvBold, 26, teal),
		text(spec.regNo, 36, 270, helvBold, 16, gray),

		text(spec.heading, 452, 188, helvBold, 15, black),
		text("This is to certify that the  "+spec.heading+" mentioned below is", 452, 215, helv, 12, black),
		text("registered with Pakistan Software Export Board.", 452, 235, helv, 12, black),

		text("Toll Free: 0800-01010", 232, 555, helv, 10, gray),
	}

	y := 272.0
	const rowGap = 26

	boxes = append(boxes, field("Name:", spec.name, y)...)
	y += rowGap

	if spec.cnic != "" {
		boxes = append(boxes, field("CNIC:", spec.cnic, y)...)
		y += rowGap
	}

	boxes = append(boxes, field("Valid From:", spec.validFrom, y)...)
	y += rowGap
	boxes = append(boxes, field("Valid Till:", spec.validTill, y)...)
	y += rowGap
	boxes = append(boxes, field("Registration No:", spec.regNo, y)...)

	content := map[string]any{
		"text": boxes,
		"image": []map[string]any{
			{"src": bannerPath, "pos": []float64{71, 26}, "width": 700},
			{"src": qrPath, "anchor": "bl", "dx": 36, "dy": 90, "width": 150},
		},
	}

	return map[string]any{
		"paper":  "A4L",
		"origin": "UpperLeft",
		"pages": map[string]any{
			"1": map[string]any{"content": content},
		},
	}
}
