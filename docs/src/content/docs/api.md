---
title: API Reference
description: Complete API reference for the pseb library
---

# API Reference

The pseb library provides a small, clean API for reading and verifying Pakistan Software Export Board registration certificates.

## Extraction

### `ExtractCertificate(pdf []byte) (*Certificate, error)`

Reads a PSEB certificate PDF, decodes the QR code printed on it, and returns the [`Certificate`](#certificate) it encodes.

```go
pdf, _ := os.ReadFile("pseb_cert.pdf")

cert, err := pseb.ExtractCertificate(pdf)
if err != nil {
    log.Fatalf("failed to extract certificate: %v", err)
}
```

**Parameters:**

* `pdf`: The raw bytes of a PSEB certificate PDF

**Returns:**

* `*Certificate`: The extracted certificate data (verification URL, JWT, and decoded claims)
* `error`: [`ErrNoQRCode`](#errors) if no QR code can be decoded, [`ErrNoJWT`](#errors) if the QR code does not contain a JWT, or a decode error for a malformed token

:::note
The JWT signature is not checked during extraction. Use [`Verify`](#verify) to confirm a certificate's authenticity and current validity with PSEB.
:::

#### QR decoding strategy

PSEB embeds the QR as a very small, full-bleed raster (roughly two pixels per module, no quiet zone), which off-the-shelf QR readers cannot detect. `ExtractCertificate` decodes it in two stages:

1. **Direct decode** — the extracted image is handed to the QR reader as-is. This succeeds for well-formed QR images with a quiet zone (e.g. a company certificate's QR).
2. **Module-grid reconstruction** — if the direct decode fails, detection is skipped entirely. Since a full-bleed symbol's module grid is implied by geometry, each valid QR size (21–177 modules) is tried: the center of every module is sampled to rebuild the matrix, which is re-rendered crisply with a quiet zone and decoded. A wrong size fails the QR's format/error-correction checks, so a successful decode is self-validating.

This assumes the QR fills its image, is axis-aligned, and is not rotated — all of which hold for PSEB certificates.

### `ExtractCNIC(pdf []byte) (string, error)`

Reads a PSEB certificate PDF and returns the holder's 13-digit CNIC (the Pakistani national identity number printed on freelancer certificates).

```go
pdf, _ := os.ReadFile("pseb_freelancer_cert.pdf")

cnic, err := pseb.ExtractCNIC(pdf)
if err != nil {
    log.Fatalf("failed to extract CNIC: %v", err)
}

fmt.Println(cnic) // 3520122720739
```

**Parameters:**

* `pdf`: The raw bytes of a PSEB certificate PDF

**Returns:**

* `string`: The 13-digit CNIC, with any dashes or spaces stripped
* `error`: [`ErrNoCNIC`](#errors) if no CNIC can be found in the PDF's text layer

:::note
Unlike [`ExtractCertificate`](#extractcertificatepdf-byte-certificate-error), the CNIC is not part of the QR code, the JWT claims, or the PSEB verification response. It is printed on the certificate and read from the PDF's text layer.
:::

#### How the CNIC is located

The page content streams are parsed for their text-showing operators (`Tj` / `TJ`) to reconstruct the visible text, then the CNIC is matched in two steps:

1. **Label-anchored match** — prefer a value that follows a `CNIC` label, accepting both the bare (`3520122720739`) and dash-separated (`35201-2272073-9`) forms, normalized to 13 digits.
2. **13-digit fallback** — if no label is found, match any standalone run of exactly 13 digits, which is unambiguous on a PSEB certificate (the registration number, phone, and fax never form 13 consecutive digits).

This relies on the certificate having a real text layer, as current PSEB certificates do; it does not OCR image-only (scanned) PDFs.

## Client

### `New(opts ...httpr.ClientOption) *Client`

Creates a new client that talks to the PSEB portal verification API. By default it targets [`DefaultBaseURL`](#constants).

```go
// Basic client
client := pseb.New()

// Client with a custom timeout
client := pseb.New(httpr.Timeout(30 * time.Second))
```

**Parameters:**

* `opts`: Optional [httpr](https://github.com/mistermoe/httpr) client options (e.g. `httpr.BaseURL(...)`, `httpr.HTTPClient(...)`, `httpr.Timeout(...)`)

## Verification

### `(c *Client) Verify(ctx context.Context, token string) (*VerificationResult, error)`

Submits a PSEB certificate JWT to the PSEB portal and returns the certificate data the portal reports for it.

```go
result, err := client.Verify(ctx, cert.JWT)
```

**Parameters:**

* `ctx`: Context for request cancellation and timeouts
* `token`: The compact JWT extracted from a certificate's QR code (see [`Certificate.JWT`](#certificate))

**Returns:**

* `*VerificationResult`: The certificate data reported by PSEB
* `error`: An error if the token is not a valid JWT, if the request fails, or if the portal responds with a non-2xx status. If the portal responds successfully but reports the certificate as **not valid**, `Verify` returns the populated result together with [`ErrCertificateInvalid`](#errors) — match it with `errors.Is` to tell an invalid certificate apart from a transport failure.

## Data Types

### `Certificate`

Contains the data extracted from a certificate PDF's QR code. These values come from the QR's verification URL and the JWT's claims; they are not independently verified.

```go
type Certificate struct {
    // The URL encoded in the certificate's QR code. Points at the PSEB
    // portal's public verification page and embeds the JWT as its final
    // path segment.
    PSEBHostedVerificationURL string `json:"pseb_hosted_verification_url"`

    // The raw, compact JWT taken from the verification URL. This is the
    // value to pass to Verify.
    JWT string `json:"jwt"`

    // The PSEB registration number assigned to the holder, e.g. "Z-25-17156/25".
    RegistrationNumber string `json:"registration_number"`

    // The registration type (company or freelancer).
    Type CertificateType `json:"type"`

    // When the certificate's token was issued (JWT "iat" claim), in UTC.
    IssuedAt time.Time `json:"issued_at"`

    // The JWT "exp" claim, in UTC. This is the verification token's expiry
    // (PSEB issues it with a short ~90-day lifetime), NOT the end of the
    // registration's validity period. The registration's real expiry is only
    // reported by the portal (see VerificationResult.RegistrationExpiresAt).
    JWTExpiresAt time.Time `json:"jwt_expires_at"`
}
```

### `VerificationResult`

The certificate data the PSEB portal returns when a certificate JWT is verified. Unlike [`Certificate`](#certificate), these values are reported by PSEB and include the registered entity's name and an authoritative validity flag.

```go
type VerificationResult struct {
    // The PSEB registration number of the holder, e.g. "Z-25-17156/25".
    RegistrationNumber string `json:"registration_number"`

    // The registration type (company or freelancer).
    Type CertificateType `json:"type"`

    // The registered name of the software exporter as recorded by PSEB,
    // e.g. "OUTSENTIA (PRIVATE) LIMITED". Only available via verification.
    Name string `json:"name"`

    // When the certificate's token was issued (JWT "iat" claim), in UTC.
    IssuedAt time.Time `json:"issued_at"`

    // The JWT "exp" claim, in UTC — the verification token's expiry (~90-day
    // lifetime), NOT the end of the registration's validity period.
    JWTExpiresAt time.Time `json:"jwt_expires_at"`

    // The start of the registration's validity window as reported by PSEB, a
    // coarse "Mon YYYY" label such as "May 2026". Empty if not reported.
    ValidFrom string `json:"valid_from"`

    // The end of the registration's validity window as reported by PSEB, a
    // coarse "Mon YYYY" label such as "Apr 2027". This is the certificate's
    // real expiry (distinct from JWTExpiresAt). Empty if not reported.
    ValidTill string `json:"valid_till"`

    // ValidTill parsed into a timestamp (first instant of the reported month,
    // in UTC), e.g. "Apr 2027" becomes 2027-04-01 00:00:00 UTC. Zero time if
    // ValidTill is empty or unparseable.
    RegistrationExpiresAt time.Time `json:"registration_expires_at"`

    // Whether PSEB considers the certificate currently valid. This is the
    // authoritative validity signal.
    IsValid bool `json:"is_valid"`
}
```

### `CertificateType`

A strongly-typed PSEB registration type. The value mirrors the `type` claim verbatim, so it is safe to compare against the constants below but may hold an unrecognized value if PSEB introduces new types.

```go
type CertificateType string

const (
    // A registration issued to a company (a registered legal entity,
    // e.g. a "(PRIVATE) LIMITED" company).
    CertificateTypeCompany CertificateType = "company"

    // A registration issued to an individual software exporter (a freelancer).
    CertificateTypeFreelancer CertificateType = "freelancer"
)
```

## Constants

### `DefaultBaseURL`

The base URL of the PSEB portal API used to verify certificates. It is the default host a `Client` created with [`New`](#client) targets.

```go
const DefaultBaseURL = "https://api.techdestination.com"
```

## Errors

The library exposes sentinel errors returned by the extraction functions, which can be matched with `errors.Is`.

```go
var (
    // Returned by ExtractCertificate when the PDF contains no image that
    // decodes as a QR code.
    ErrNoQRCode = errors.New("no QR code found in PDF")

    // Returned by ExtractCertificate when the QR code was decoded but does
    // not contain a JWT-shaped token.
    ErrNoJWT = errors.New("no JWT found in QR code")

    // Returned by ExtractCNIC when no CNIC can be found in the PDF's text
    // layer.
    ErrNoCNIC = errors.New("no CNIC found in PDF")

    // Returned by Verify (alongside the populated result) when the PSEB
    // portal reports the certificate as not valid.
    ErrCertificateInvalid = errors.New("PSEB reports certificate is not valid")
)
```

```go
cert, err := pseb.ExtractCertificate(pdf)
if err != nil {
    switch {
    case errors.Is(err, pseb.ErrNoQRCode):
        log.Println("this PDF has no QR code")
    case errors.Is(err, pseb.ErrNoJWT):
        log.Println("the QR code does not contain a JWT")
    default:
        log.Printf("failed to extract certificate: %v", err)
    }
}
```
