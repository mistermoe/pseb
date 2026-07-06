# pseb

![Go Badge](https://img.shields.io/badge/Go-00ADD8?logo=go&logoColor=fff&style=flat) [![Go Report Card](https://goreportcard.com/badge/github.com/mistermoe/pseb)](https://goreportcard.com/report/github.com/mistermoe/pseb) [![integrity](https://github.com/mistermoe/pseb/actions/workflows/integrity.yml/badge.svg)](https://github.com/mistermoe/pseb/actions/workflows/integrity.yml)

Read and verify Pakistan Software Export Board (PSEB) registration certificates.

## Overview

The Pakistan Software Export Board (PSEB) registers IT and IT-enabled services companies and freelancers as software exporters.

Every PSEB registration certificate is issued as a PDF that carries a QR code.

That QR code encodes a verification URL whose final path segment is a signed [JSON Web Token (JWT)](https://datatracker.ietf.org/doc/html/rfc7519). The JWT's claims contain the certificate's core data:

- the registration number
- the registration type (company or individual/freelancer)
- issued-at
- expires-at

This library lets you:

- **Extract** the QR code from a certificate PDF and read out the verification URL, the JWT, and its decoded claims.
- **Verify** a certificate JWT against the PSEB portal, returning the registered entity's name and whether the certificate is currently valid.

### What's inside the QR code

The QR encodes a URL of the form `https://portal.techdestination.com/verify-certificate/<jwt>`. The JWT's claims include:

- `registrationNo` — the PSEB registration number (e.g. `Z-25-17156/25`)
- `type` — the registration type (`company` or individual/freelancer)
- `iat` — the issued-at timestamp
- `exp` — the expiry timestamp

> [!NOTE]
> PSEB certificates are signed with `HS256`, a symmetric (shared-secret) algorithm. Because the signing secret is held by PSEB and not published, the signature cannot be verified offline by third parties. This library uses the JWT to read claims locally and delegates authoritative validity to the PSEB portal via `Verify`.

## Installation

```bash
go get github.com/mistermoe/pseb
```

## Usage

```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/mistermoe/pseb"
)

func main() {
	pdf, err := os.ReadFile("pseb_cert.pdf")
	if err != nil {
		log.Fatalf("failed to read certificate: %v", err)
	}

	// Extract the QR code and decode the JWT it carries (offline).
	cert, err := pseb.ExtractCertificate(context.Background(), pdf)
	if err != nil {
		log.Fatalf("failed to extract certificate: %v", err)
	}

	fmt.Printf("Registration: %s\n", cert.RegistrationNumber)
	fmt.Printf("Type:         %s\n", cert.Type)
	fmt.Printf("Expires:      %s\n", cert.ExpiresAt.Format("2006-01-02"))

	// Verify the certificate against the PSEB portal.
	client := pseb.New()
	result, err := client.Verify(context.Background(), cert.JWT)
	if err != nil {
		log.Fatalf("failed to verify certificate: %v", err)
	}

	fmt.Printf("Registered to: %s (valid: %t)\n", result.Name, result.IsValid)
}
```

## API

### `ExtractCertificate(ctx context.Context, pdf []byte) (*UnverifiedCertificate, error)`

Reads a PSEB certificate PDF, decodes the QR code printed on it, and returns the `UnverifiedCertificate` it encodes. It extracts the images embedded in the PDF, decodes the first one that is a readable QR code, parses the verification URL to recover the JWT, and decodes the JWT's claims. Extraction is fully offline; the JWT signature is not checked here.

Returns `ErrNoQRCode` if no QR code can be decoded, `ErrNoJWT` if the decoded QR code does not contain a JWT, or `ErrInsecureAlgorithm` if the JWT header declares no algorithm or the `none` algorithm.

### `New(opts ...httpr.ClientOption) *Client`

Creates a client that talks to the PSEB portal verification API. Defaults to the PSEB portal host; pass [httpr](https://github.com/mistermoe/httpr) options (e.g. `httpr.Timeout(...)`, `httpr.HTTPClient(...)`) to customize HTTP behavior.

### `(c *Client) Verify(ctx context.Context, token string) (*VerificationResult, error)`

Submits a certificate JWT to the PSEB portal and returns the certificate data the portal reports for it, including the registered entity's name and an authoritative `IsValid` flag. Requires network access.

### Types

```go
// UnverifiedCertificate contains claims extracted locally from a PSEB certificate's QR code.
// WARNING: The JWT signature is NOT verified during extraction because PSEB uses a symmetric secret.
// Do NOT use this data for authorization without passing the JWT to Client.Verify().
type UnverifiedCertificate struct {
	PSEBHostedVerificationURL string          `json:"pseb_hosted_verification_url"`
	JWT                       string          `json:"jwt"`
	RegistrationNumber        string          `json:"registration_number"`
	Type                      CertificateType `json:"type"`
	IssuedAt                  time.Time       `json:"issued_at"`
	ExpiresAt                 time.Time       `json:"expires_at"`
}

type VerificationResult struct {
	RegistrationNumber string          `json:"registration_number"`
	Type               CertificateType `json:"type"`
	Name               string          `json:"name"`
	IssuedAt           time.Time       `json:"issued_at"`
	ExpiresAt          time.Time       `json:"expires_at"`
	IsValid            bool            `json:"is_valid"`
}
```

## Development

This repo uses [hermit](https://cashapp.github.io/hermit/) for tooling and [just](https://github.com/casey/just) as a command runner.

```bash
just test   # run tests
just lint   # run golangci-lint
just docs   # run the docs site locally
```

### Testing

Tests use [go-vcr](https://github.com/dnaeon/go-vcr) to record and replay HTTP interactions. Recorded cassettes live in `fixtures/`, so the suite runs offline by default. To re-record against the live PSEB endpoint, flip `testMode` to `vcr.Record` in the relevant test, run it once, then flip it back to `vcr.Replay`.

## Documentation

Full documentation lives in the [`docs/`](./docs) directory (an [Astro Starlight](https://starlight.astro.build/) site).
