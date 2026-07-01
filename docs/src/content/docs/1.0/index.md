---
title: PSEB Certificate Toolkit
description: Read and verify Pakistan Software Export Board (PSEB) registration certificates
slug: "1.0"
---

# PSEB Certificate Toolkit

[![Test](https://github.com/mistermoe/pseb/actions/workflows/test.yml/badge.svg)](https://github.com/mistermoe/pseb/actions/workflows/test.yml)
[![Lint](https://github.com/mistermoe/pseb/actions/workflows/lint.yml/badge.svg)](https://github.com/mistermoe/pseb/actions/workflows/lint.yml)

## Overview

The Pakistan Software Export Board (PSEB) registers IT and IT-enabled services companies and freelancers as software exporters. Every PSEB registration certificate is issued as a PDF that carries a QR code. That QR code encodes a verification URL whose final path segment is a signed [JSON Web Token (JWT)](https://datatracker.ietf.org/doc/html/rfc7519) containing the certificate's core data.

This library reads that QR code out of the PDF and verifies the JWT against the PSEB portal.

### What's inside the QR code

The QR encodes a URL of the form `https://portal.techdestination.com/verify-certificate/<jwt>`. The JWT's claims include:

* `registrationNo` — the PSEB registration number (e.g. `Z-25-17156/25`)
* `type` — the registration type (`company` or individual/freelancer)
* `iat` — the issued-at timestamp
* `exp` — the expiry timestamp

:::note
PSEB certificates are signed with `HS256`, a symmetric (shared-secret) algorithm. Because the signing secret is held by PSEB and not published, the signature cannot be verified offline by third parties. This library uses the JWT to read claims locally and delegates authoritative validity to the PSEB portal via `Verify`.
:::

:::caution[Important]
`Verify` calls the PSEB portal and therefore requires network access. Certificate extraction from a PDF works fully offline.
:::

## Installation

```bash
go get github.com/mistermoe/pseb
```

## Quick Start

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

    cert, err := pseb.ExtractCertificate(pdf)
    if err != nil {
        log.Fatalf("failed to extract certificate: %v", err)
    }

    fmt.Printf("Registration: %s\n", cert.RegistrationNumber)
    fmt.Printf("Type:         %s\n", cert.Type)
    fmt.Printf("Expires:      %s\n", cert.ExpiresAt.Format("2006-01-02"))

    client := pseb.New()
    result, err := client.Verify(context.Background(), cert.JWT)
    if err != nil {
        log.Fatalf("failed to verify certificate: %v", err)
    }

    fmt.Printf("Registered to: %s (valid: %t)\n", result.Name, result.IsValid)
}
```

## Features

* **🔍 QR Extraction**: Pull the QR code out of a certificate PDF with no external tooling
* **🔓 JWT Decoding**: Read the registration number, type, and validity window from the token
* **✅ Portal Verification**: Confirm a certificate against PSEB and retrieve the registered entity's name
* **🏢 Strongly Typed**: Company vs. individual/freelancer registration types
* **🌐 Offline Extraction**: PDF parsing and decoding require no network access
* **🧪 Well Tested**: Test suite backed by recorded HTTP interactions (VCR) for reliable, offline runs
