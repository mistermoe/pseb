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
* the registration type (company or individual/freelancer)
* issued-at
* expires-at

This library provides an interface to:

* **Extract** the QR code from a certificate PDF and read out the verification URL, the JWT, and its decoded claims.
* **Verify** a certificate JWT against the PSEB portal, returning the registered entity's name and whether the certificate is currently valid.

:::note
PSEB certificates are signed with `HS256`, a symmetric (shared-secret) algorithm. Because the signing secret is held by PSEB and not published, the signature cannot be verified offline by third parties. This library uses the JWT to read claims locally, and delegates authoritative validity to the PSEB portal via `Verify`.
:::

:::caution[Important]
Because verification relies on the PSEB portal, `Verify` requires network access. Certificate extraction from a PDF works fully offline.
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

	fmt.Println(cert.RegistrationNumber, cert.Type, cert.ExpiresAt)

	// Verify the certificate against the PSEB portal.
	client := pseb.New()
	result, err := client.Verify(context.Background(), cert.JWT)
	if err != nil {
		log.Fatalf("failed to verify certificate: %v", err)
	}

	fmt.Println(result.Name, result.IsValid)
}
```

## API

### `ExtractCertificate`

Reads a PSEB certificate PDF, decodes the QR code printed on it, and returns the [`Certificate`](/api#certificate) it encodes. The images embedded in the PDF are extracted, the first one that is a readable QR code is decoded, the verification URL is parsed to recover the JWT, and the JWT's claims are decoded to populate the registration number, type, and timestamps.

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
