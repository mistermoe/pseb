// Package pseb reads and verifies Pakistan Software Export Board (PSEB)
// registration certificates.
//
// PSEB is the Pakistani government body that registers IT and IT-enabled
// services companies and freelancers as software exporters. Every PSEB
// registration certificate is issued as a PDF that carries a QR code. That QR
// code does not just link to a web page: it encodes a verification URL whose
// final path segment is a signed JSON Web Token (JWT). The JWT's claims contain
// the certificate's core data: the registration number, the registration type
// (company or individual/freelancer), and issued-at / expiry timestamps.
//
// This package provides two things:
//
//   - ExtractCertificate reads the QR code out of a certificate PDF and returns
//     the encoded verification URL, the raw JWT, and the claims decoded from it.
//   - Client.Verify submits a JWT to the PSEB portal's verification endpoint and
//     returns the certificate data the portal reports for it, including the
//     registered entity's name and whether the certificate is currently valid.
//
// # Trust model
//
// PSEB certificates are signed with HS256, a symmetric (shared-secret)
// algorithm. Because the signing secret is held by PSEB and not published, the
// signature cannot be verified offline by third parties. This package therefore
// uses the JWT only to read claims locally; authoritative validity comes from
// Client.Verify, which calls the PSEB portal. (If PSEB ever switches to an
// asymmetric scheme such as EdDSA/ES256 and publishes a public key, offline
// verification would become possible.)
//
// # Extracting from a PDF
//
//	pdf, _ := os.ReadFile("pseb_cert.pdf")
//	cert, err := pseb.ExtractCertificate(pdf)
//	if err != nil {
//		// no QR code, no JWT, or malformed token
//	}
//	fmt.Println(cert.RegistrationNumber, cert.Type, cert.ExpiresAt)
//
// # Verifying against the PSEB portal
//
//	client := pseb.New()
//	result, err := client.Verify(ctx, cert.JWT)
//	if err != nil {
//		// transport error or the portal rejected the certificate
//	}
//	fmt.Println(result.Name, result.IsValid)
package pseb
