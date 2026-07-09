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
    fmt.Printf("JWT expiry:   %s\n", cert.JWTExpiresAt.Format("2006-01-02")) // token, not the registration expiry
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
    "errors"
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

    // Confirm it with PSEB. Verify returns ErrCertificateInvalid (with a
    // populated result) when PSEB reports the certificate as not valid.
    client := pseb.New()
    result, err := client.Verify(context.Background(), cert.JWT)
    switch {
    case errors.Is(err, pseb.ErrCertificateInvalid):
        fmt.Printf("%s is NOT currently valid\n", result.Name)
    case err != nil:
        log.Fatalf("failed to verify certificate: %v", err)
    default:
        fmt.Printf("%s is a registered PSEB software exporter\n", result.Name)
    }
}
```

### Extract the CNIC (freelancer certificates)

The CNIC printed on a freelancer certificate is not part of the QR code, the JWT, or the verification response. `ExtractCNIC` reads it from the PDF's text layer.

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
    pdf, err := os.ReadFile("pseb_freelancer_cert.pdf")
    if err != nil {
        log.Fatalf("failed to read certificate: %v", err)
    }

    cnic, err := pseb.ExtractCNIC(pdf)
    if err != nil {
        if errors.Is(err, pseb.ErrNoCNIC) {
            fmt.Println("no CNIC printed on this certificate")
            return
        }
        log.Fatalf("failed to extract CNIC: %v", err)
    }

    fmt.Printf("CNIC: %s\n", cnic)
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
    case pseb.CertificateTypeFreelancer:
        fmt.Println("This is a freelancer registration")
    default:
        fmt.Printf("Unknown registration type: %s\n", cert.Type)
    }
}
```

### Check When a Registration Expires

The JWT's `exp` claim (`cert.JWTExpiresAt`) is a short-lived (~90-day) token expiry, **not** the registration's validity end. The registration's real expiry is only reported by the portal, so use `Verify` and read `RegistrationExpiresAt`.

```go
package main

import (
    "context"
    "errors"
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

    result, err := pseb.New().Verify(context.Background(), cert.JWT)
    if err != nil && !errors.Is(err, pseb.ErrCertificateInvalid) {
        log.Fatalf("failed to verify certificate: %v", err)
    }

    fmt.Printf("Valid through: %s\n", result.ValidTill) // e.g. "Apr 2027"

    if !result.RegistrationExpiresAt.IsZero() && time.Now().After(result.RegistrationExpiresAt) {
        fmt.Println("Registration has expired")
    }
}
```

:::note
For a definitive answer on whether a certificate is currently valid, rely on `result.IsValid` (PSEB's own check). `RegistrationExpiresAt` is derived from PSEB's coarse "Mon YYYY" `ValidTill` label, so it is month-precision only.
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
