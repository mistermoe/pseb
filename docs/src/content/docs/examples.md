---
title: Examples
description: Practical examples of using the pseb library
---

# Examples

This page provides practical examples of using the pseb library for various use cases.

## Basic Usage

### Extract a Certificate from a PDF

```go
package main

import (
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
    fmt.Printf("Issued:       %s\n", cert.IssuedAt.Format("2006-01-02"))
    fmt.Printf("Expires:      %s\n", cert.ExpiresAt.Format("2006-01-02"))
    fmt.Printf("JWT:          %s\n", cert.JWT)
}
```

### Verify a Certificate

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/mistermoe/pseb"
)

func main() {
    client := pseb.New()

    result, err := client.Verify(context.Background(), jwt)
    if err != nil {
        log.Fatalf("failed to verify certificate: %v", err)
    }

    fmt.Printf("Registered to: %s\n", result.Name)
    fmt.Printf("Registration:  %s\n", result.RegistrationNumber)
    fmt.Printf("Valid:         %t\n", result.IsValid)
}
```

### Extract and Verify in One Flow

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

    // Read the JWT out of the PDF's QR code.
    cert, err := pseb.ExtractCertificate(pdf)
    if err != nil {
        log.Fatalf("failed to extract certificate: %v", err)
    }

    // Confirm it with PSEB.
    client := pseb.New()
    result, err := client.Verify(context.Background(), cert.JWT)
    if err != nil {
        log.Fatalf("failed to verify certificate: %v", err)
    }

    if result.IsValid {
        fmt.Printf("%s is a registered PSEB software exporter\n", result.Name)
    } else {
        fmt.Printf("%s is NOT currently valid\n", result.Name)
    }
}
```

## Working with Certificate Data

### Check the Registration Type

```go
package main

import (
    "fmt"
    "log"
    "os"

    "github.com/mistermoe/pseb"
)

func main() {
    pdf, _ := os.ReadFile("pseb_cert.pdf")

    cert, err := pseb.ExtractCertificate(pdf)
    if err != nil {
        log.Fatalf("failed to extract certificate: %v", err)
    }

    switch cert.Type {
    case pseb.CertificateTypeCompany:
        fmt.Println("This is a company registration")
    case pseb.CertificateTypeIndividual:
        fmt.Println("This is an individual/freelancer registration")
    default:
        fmt.Printf("Unknown registration type: %s\n", cert.Type)
    }
}
```

### Check Whether a Certificate Has Expired

```go
package main

import (
    "fmt"
    "log"
    "os"
    "time"

    "github.com/mistermoe/pseb"
)

func main() {
    pdf, _ := os.ReadFile("pseb_cert.pdf")

    cert, err := pseb.ExtractCertificate(pdf)
    if err != nil {
        log.Fatalf("failed to extract certificate: %v", err)
    }

    if time.Now().After(cert.ExpiresAt) {
        fmt.Printf("Certificate expired on %s\n", cert.ExpiresAt.Format("2006-01-02"))
    } else {
        remaining := time.Until(cert.ExpiresAt)
        fmt.Printf("Certificate valid for %d more days\n", int(remaining.Hours()/24))
    }
}
```

:::note
Local expiry is a claim from the JWT and is not authoritative. For a definitive answer, use `Verify` and check `IsValid`.
:::

### Output as JSON

```go
package main

import (
    "encoding/json"
    "fmt"
    "log"
    "os"

    "github.com/mistermoe/pseb"
)

func main() {
    pdf, _ := os.ReadFile("pseb_cert.pdf")

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

## Advanced Examples

### Error Handling

```go
package main

import (
    "errors"
    "fmt"
    "log"
    "os"

    "github.com/mistermoe/pseb"
)

func main() {
    pdf, _ := os.ReadFile("some_document.pdf")

    cert, err := pseb.ExtractCertificate(pdf)
    if err != nil {
        switch {
        case errors.Is(err, pseb.ErrNoQRCode):
            fmt.Println("This PDF has no QR code — is it a PSEB certificate?")
        case errors.Is(err, pseb.ErrNoJWT):
            fmt.Println("The QR code does not contain a JWT")
        default:
            log.Printf("Unexpected error: %v", err)
        }
        return
    }

    fmt.Printf("Extracted certificate for %s\n", cert.RegistrationNumber)
}
```

### With Custom HTTP Configuration

```go
package main

import (
    "context"
    "fmt"
    "log"
    "time"

    "github.com/mistermoe/httpr"
    "github.com/mistermoe/pseb"
)

func main() {
    // Create a client with a custom timeout.
    client := pseb.New(
        httpr.Timeout(30 * time.Second),
    )

    ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
    defer cancel()

    result, err := client.Verify(ctx, jwt)
    if err != nil {
        log.Fatalf("failed to verify certificate: %v", err)
    }

    fmt.Printf("Valid: %t\n", result.IsValid)
}
```

### Inspect the Verification Request

```go
package main

import (
    "context"
    "log"

    "github.com/mistermoe/httpr"
    "github.com/mistermoe/pseb"
)

func main() {
    // httpr.Inspect() logs the outgoing request and response — handy for debugging.
    client := pseb.New(httpr.Inspect())

    _, err := client.Verify(context.Background(), jwt)
    if err != nil {
        log.Fatalf("failed to verify certificate: %v", err)
    }
}
```
