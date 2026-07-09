// Command pseb reads a PSEB registration certificate PDF, prints the data
// encoded in its QR code, and verifies it against the PSEB portal.
//
// Usage:
//
//	pseb <path-to-certificate.pdf>
//
// It extracts the certificate (offline), reads the CNIC printed on freelancer
// certificates when present, and then calls the PSEB
// portal to confirm the certificate is currently valid. It exits non-zero if
// extraction fails, verification fails, or the portal reports the certificate
// as not valid.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/mistermoe/pseb"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: %s <path-to-certificate.pdf>", filepath.Base(os.Args[0]))
	}

	path := args[0]

	pdf, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", path, err)
	}

	cert, err := pseb.ExtractCertificate(pdf)
	if err != nil {
		return fmt.Errorf("failed to extract certificate: %w", err)
	}

	fmt.Println("Certificate")
	fmt.Printf("  Registration: %s\n", cert.RegistrationNumber)
	fmt.Printf("  Type:         %s\n", cert.Type)
	fmt.Printf("  Issued:       %s\n", cert.IssuedAt.Format("2006-01-02"))
	fmt.Printf("  JWT expiry:   %s (token, not the registration expiry)\n", cert.JWTExpiresAt.Format("2006-01-02"))
	fmt.Printf("  Verify URL:   %s\n", cert.PSEBHostedVerificationURL)

	// The CNIC is only printed on freelancer certificates, never on company
	// ones, so only look for it there. Treat its absence as expected rather
	// than an error.
	if cert.Type == pseb.CertificateTypeFreelancer {
		if cnic, cnicErr := pseb.ExtractCNIC(pdf); cnicErr == nil {
			fmt.Printf("  CNIC:         %s\n", cnic)
		} else if !errors.Is(cnicErr, pseb.ErrNoCNIC) {
			return fmt.Errorf("failed to extract CNIC: %w", cnicErr)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client := pseb.New()

	result, err := client.Verify(ctx, cert.JWT)
	if err != nil && !errors.Is(err, pseb.ErrCertificateInvalid) {
		return fmt.Errorf("failed to verify certificate: %w", err)
	}

	fmt.Println("\nVerification (via PSEB portal)")
	fmt.Printf("  Name:         %s\n", result.Name)
	fmt.Printf("  Valid:        %t\n", result.IsValid)

	if result.ValidFrom != "" || result.ValidTill != "" {
		fmt.Printf("  Valid from:   %s\n", result.ValidFrom)
		fmt.Printf("  Valid till:   %s\n", result.ValidTill)
	}

	if !result.RegistrationExpiresAt.IsZero() {
		fmt.Printf("  Expires:      %s\n", result.RegistrationExpiresAt.Format("2006-01-02"))
	}

	if !result.IsValid {
		return fmt.Errorf("certificate %s is not valid", result.RegistrationNumber)
	}

	return nil
}
