---
title: Troubleshooting
description: Common issues and solutions when using the pseb library
---

# Troubleshooting

This page covers common issues and their solutions when using the pseb library.

## Common Issues

### No QR Code Found in PDF

**Problem:** `ExtractCertificate` returns `ErrNoQRCode`.

**Possible Causes:**

* The PDF is not a PSEB certificate (or a different document altogether)
* The certificate is an older format without a QR code
* The PDF is a scan/photo where the QR code did not survive as a decodable image

**Solution:**

```go
cert, err := pseb.ExtractCertificate(context.Background(), pdf)
if err != nil {
    if errors.Is(err, pseb.ErrNoQRCode) {
        // Not a QR-bearing PSEB certificate — fall back to manual review
        fmt.Println("No QR code detected in this PDF")
    }
}
```

### QR Code Doesn't Contain a JWT

**Problem:** `ExtractCertificate` returns `ErrNoJWT`.

**Possible Causes:**

* The QR code encodes something other than a PSEB verification URL
* The URL format changed and the token is no longer the final path segment

**Solution:** Confirm the QR content by scanning it with any QR reader. A valid PSEB certificate encodes a URL of the form `https://portal.techdestination.com/verify-certificate/<jwt>`.

### Verification Fails

**Problem:** `Verify` returns an error or `IsValid` is `false`.

**Possible Causes:**

* **Expired certificate**: the certificate is past its `exp` date
* **Network issues**: the PSEB portal is unreachable or timed out
* **Non-2xx response**: the portal rejected the token
* **Malformed token**: the JWT passed to `Verify` is not well-formed

**Solutions:**

1. **Distinguish transport errors from an invalid certificate.** A non-nil `error` means the request itself failed; a successful call with `IsValid == false` means PSEB considers the certificate invalid.

```go
result, err := client.Verify(ctx, jwt)
if err != nil {
    // transport error or non-2xx response
    log.Printf("verification request failed: %v", err)
    return
}

if !result.IsValid {
    log.Println("PSEB reports this certificate is not valid")
    return
}
```

2. **Increase the timeout** for slow connections:

```go
client := pseb.New(httpr.Timeout(60 * time.Second))

ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()
```

### Timeout Errors

**Problem:** Getting "context deadline exceeded" or timeout errors from `Verify`.

**Possible Causes:**

* Slow network connection
* PSEB portal server issues

**Solutions:**

```go
// Increase client timeout
client := pseb.New(httpr.Timeout(60 * time.Second))

// Or bound the call with a context timeout
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

result, err := client.Verify(ctx, jwt)
```

## Debugging Tips

### Enable HTTP Debugging

Use httpr's inspector to log the outgoing request and response:

```go
import "github.com/mistermoe/httpr"

client := pseb.New(
    httpr.Inspect(), // Logs HTTP requests and responses
)
```

### Inspect the Extracted Data

```go
cert, err := pseb.ExtractCertificate(context.Background(), pdf)
if err == nil {
    fmt.Printf("URL:          %s\n", cert.PSEBHostedVerificationURL)
    fmt.Printf("Registration: %s\n", cert.RegistrationNumber)
    fmt.Printf("Type:         %s\n", cert.Type)
    fmt.Printf("Issued:       %s\n", cert.IssuedAt.Format("2006-01-02"))
    fmt.Printf("Expires:      %s\n", cert.ExpiresAt.Format("2006-01-02"))
}
```

### Decode the JWT Manually

The JWT is a standard token — you can paste `cert.JWT` into any JWT decoder to inspect its header and claims. Note that the signature uses `HS256` and cannot be verified without PSEB's secret.

## FAQ

### Q: Can I verify a certificate without calling the PSEB portal?

**A:** Not currently. PSEB signs certificates with `HS256`, a symmetric algorithm whose secret is not published, so third parties cannot verify the signature offline. Authoritative validity requires the `Verify` call to the portal. (If PSEB adopts an asymmetric scheme such as EdDSA/ES256 and publishes a public key, offline verification would become possible.)

### Q: Does extraction require network access?

**A:** No. `ExtractCertificate` reads and decodes the PDF entirely offline. Only `Verify` requires network access.

### Q: What's the difference between the data from `ExtractCertificate` and `Verify`?

**A:** `ExtractCertificate` returns claims read locally from the JWT (registration number, type, issued/expiry dates). `Verify` returns data reported by PSEB, which additionally includes the registered entity's `Name` and an authoritative `IsValid` flag.

### Q: The registration type isn't `company` or `individual`. Why?

**A:** `CertificateType` mirrors the `type` claim verbatim. If PSEB introduces a new type (or uses a different label such as `freelancer`), the value will be preserved as-is even though it won't match the predefined constants.

### Q: How do I handle a PDF that isn't a PSEB certificate?

**A:** Match the sentinel errors returned by `ExtractCertificate`:

```go
cert, err := pseb.ExtractCertificate(context.Background(), pdf)
if errors.Is(err, pseb.ErrNoQRCode) || errors.Is(err, pseb.ErrNoJWT) {
    // Not a QR-bearing PSEB certificate
}
```

## Getting Help

If you're still experiencing issues:

1. **Check the GitHub issues**: [github.com/mistermoe/pseb/issues](https://github.com/mistermoe/pseb/issues)
2. **Create a new issue**: Include error messages, code samples, and environment details
3. **Verify the certificate manually**: Scan the QR code and confirm it points at a PSEB verification URL

## Contributing

Found a bug or have a suggestion? Contributions are welcome!

* **Bug reports**: Include steps to reproduce and error messages
* **Feature requests**: Explain the use case and expected behavior
* **Code contributions**: Follow the existing code style and include tests
