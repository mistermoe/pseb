---
title: API Reference
description: Complete API reference for the pseb library
slug: 1.0/api
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
* `error`: An error if the token is not a valid JWT, if the request fails, or if the portal responds with a non-2xx status

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

    // The registration type (company or individual/freelancer).
    Type CertificateType `json:"type"`

    // When PSEB issued the certificate (from the JWT "iat" claim), in UTC.
    IssuedAt time.Time `json:"issued_at"`

    // When the certificate expires (from the JWT "exp" claim), in UTC.
    ExpiresAt time.Time `json:"expires_at"`
}
```

### `VerificationResult`

The certificate data the PSEB portal returns when a certificate JWT is verified. Unlike [`Certificate`](#certificate), these values are reported by PSEB and include the registered entity's name and an authoritative validity flag.

```go
type VerificationResult struct {
    // The PSEB registration number of the holder, e.g. "Z-25-17156/25".
    RegistrationNumber string `json:"registration_number"`

    // The registration type (company or individual/freelancer).
    Type CertificateType `json:"type"`

    // The registered name of the software exporter as recorded by PSEB,
    // e.g. "OUTSENTIA (PRIVATE) LIMITED". Only available via verification.
    Name string `json:"name"`

    // When PSEB issued the certificate (from the "iat" claim), in UTC.
    IssuedAt time.Time `json:"issued_at"`

    // When the certificate expires (from the "exp" claim), in UTC.
    ExpiresAt time.Time `json:"expires_at"`

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
    CertificateTypeIndividual CertificateType = "individual"
)
```

## Constants

### `DefaultBaseURL`

The base URL of the PSEB portal API used to verify certificates. It is the default host a `Client` created with [`New`](#client) targets.

```go
const DefaultBaseURL = "https://api.techdestination.com"
```

## Errors

The library exposes sentinel errors returned by [`ExtractCertificate`](#extraction), which can be matched with `errors.Is`.

```go
var (
    // Returned when the PDF contains no image that decodes as a QR code.
    ErrNoQRCode = errors.New("no QR code found in PDF")

    // Returned when the QR code was decoded but does not contain a
    // JWT-shaped token.
    ErrNoJWT = errors.New("no JWT found in QR code")
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
