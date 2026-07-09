---
title: PSEB Certificate Toolkit
description: Read and verify Pakistan Software Export Board (PSEB) registration certificates
---

<div style="display: flex; align-items: center; gap: 4px;">
  <img src="https://img.shields.io/badge/Go-00ADD8?style=flat-square&logo=go&logoColor=white" alt="Go Badge" style="margin-top: 16px">
  <img src="https://goreportcard.com/badge/github.com/mistermoe/pseb?style=flat-square" alt="Go Report Card">
  <img src="https://img.shields.io/github/actions/workflow/status/mistermoe/pseb/integrity.yml?style=flat-square&label=integrity" alt="Integrity">
</div>

Source Code: <a href="https://github.com/mistermoe/pseb"><img src="https://img.shields.io/badge/GitHub-181717?style=flat-square&logo=github&logoColor=white" alt="GitHub" style="height: 28px; vertical-align: middle;"></a>

The Pakistan Software Export Board (PSEB) registers IT and IT-enabled services companies and freelancers as software exporters.

Every PSEB registration certificate is issued as a PDF that carries a QR code.

That QR code encodes a verification URL whose final path segment is a signed [JSON Web Token (JWT)](https://datatracker.ietf.org/doc/html/rfc7519). The JWT's claims contain the certificate's core data:

* the registration number
* the registration type (company or freelancer)
* issued-at
* expires-at

This library provides an interface to:

* **Extract** the QR code from a certificate PDF and read out the verification URL, the JWT, and its decoded claims.
* **Read the CNIC** printed on a freelancer certificate, recovered from the PDF's text layer.
* **Verify** a certificate JWT against the PSEB portal, returning the registered entity's name and whether the certificate is currently valid.

:::note
PSEB certificates are signed with `HS256`, a symmetric (shared-secret) algorithm. Because the signing secret is held by PSEB and not published, the signature cannot be verified offline by third parties. This library uses the JWT to read claims locally, and delegates authoritative validity to the PSEB portal via `Verify`.
:::

:::caution[Important]
Because verification relies on the PSEB portal, `Verify` requires network access. Certificate extraction from a PDF works fully offline.
:::

:::tip[How the QR code is decoded]
PSEB embeds the QR as a very small, full-bleed raster — roughly two pixels per module with no quiet zone — which off-the-shelf QR readers cannot detect at any scale. `ExtractCertificate` handles this transparently: it first tries a direct decode (which works for well-formed QR images), and if that fails it reconstructs the QR's module grid from the raster geometry and decodes a crisp re-render. See the [QR decoding strategy](/api#qr-decoding-strategy) for details.
:::

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

	// Extract the QR code and decode the JWT it carries.
	cert, err := pseb.ExtractCertificate(pdf)
	if err != nil {
		log.Fatalf("failed to extract certificate: %v", err)
	}

	fmt.Println(cert.RegistrationNumber, cert.Type)

	// Verify the certificate against the PSEB portal.
	client := pseb.New()
	result, err := client.Verify(context.Background(), cert.JWT)
	if err != nil {
		log.Fatalf("failed to verify certificate: %v", err)
	}

	fmt.Println(result.Name, result.IsValid, result.ValidTill)
}
```

## API

### `ExtractCertificate`

Reads a PSEB certificate PDF, decodes the QR code printed on it, and returns the [`Certificate`](/api#certificate) it encodes. The images embedded in the PDF are extracted, the first one that is a readable QR code is decoded, the verification URL is parsed to recover the JWT, and the JWT's claims are decoded to populate the registration number, type, and timestamps.

Decoding tolerates the tiny, full-bleed QR raster PSEB embeds: if a direct decode fails, the QR's module grid is reconstructed from the raster and re-rendered before decoding (see the note above).

```go
import "github.com/mistermoe/pseb"

func main() {
	pdf, err := os.ReadFile("pseb_cert.pdf")
	if err != nil {
		log.Fatalf("failed to read certificate: %v", err)
	}

	cert, err := pseb.ExtractCertificate(pdf)
	if err != nil {
		log.Fatalf("failed to extract certificate: %v", err)
	}

	certJSON, err := json.MarshalIndent(cert, "", "  ")
	if err != nil {
		log.Fatalf("failed to marshal certificate: %v", err)
	}

	fmt.Println(string(certJSON))
}
```

### `ExtractCNIC`

Reads a PSEB certificate PDF and returns the holder's 13-digit CNIC (the national identity number printed on freelancer certificates). The CNIC is not part of the QR code, the JWT claims, or the verification response, so it is recovered from the PDF's text layer — first anchored to a `CNIC` label, then falling back to a standalone 13-digit run. See the [API reference](/api#extractcnicpdf-byte-string-error) for details.

```go
import "github.com/mistermoe/pseb"

func main() {
	pdf, err := os.ReadFile("pseb_freelancer_cert.pdf")
	if err != nil {
		log.Fatalf("failed to read certificate: %v", err)
	}

	cnic, err := pseb.ExtractCNIC(pdf)
	if err != nil {
		log.Fatalf("failed to extract CNIC: %v", err)
	}

	fmt.Println(cnic)
}
```

### `Verify`

Submits a PSEB certificate JWT to the PSEB portal and returns the certificate data the portal reports for it. Because PSEB certificates are signed with a secret only PSEB holds, this network call is what authoritatively establishes a certificate's authenticity and validity.

```go
import "github.com/mistermoe/pseb"

func main() {
	client := pseb.New()

	result, err := client.Verify(context.Background(), cert.JWT)
	if err != nil {
		log.Fatalf("failed to verify certificate: %v", err)
	}

	fmt.Printf("%s (%s) valid: %t\n", result.Name, result.RegistrationNumber, result.IsValid)
}
```
