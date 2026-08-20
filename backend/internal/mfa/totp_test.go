package mfa

import (
	"strings"
	"testing"
	"time"
)

// rfcSecret is the ASCII secret "12345678901234567890" from RFC 6238 Appendix B,
// encoded as base32 (no padding).
const rfcSecret = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"

func TestValidateAgainstRFCVector(t *testing.T) {
	// RFC 6238 SHA1 vector at T=59s is 94287082; our 6-digit truncation is the
	// low 6 digits of that value.
	at := time.Unix(59, 0)
	if !Validate(rfcSecret, "287082", at) {
		t.Fatal("expected the RFC 6238 T=59 code to validate")
	}
}

func TestValidateRejectsWrongCode(t *testing.T) {
	at := time.Unix(59, 0)
	if Validate(rfcSecret, "000000", at) {
		t.Fatal("expected a wrong code to be rejected")
	}
}

func TestValidateRejectsMalformed(t *testing.T) {
	at := time.Unix(59, 0)
	for _, input := range []string{"", "12345", "1234567", "abcdef"} {
		if Validate(rfcSecret, input, at) {
			t.Fatalf("expected malformed code %q to be rejected", input)
		}
	}
}

func TestValidateToleratesClockSkew(t *testing.T) {
	// A code generated for the previous step should still pass within the window.
	base := time.Unix(1_600_000_000, 0)
	previous, err := code(rfcSecret, uint64(base.Unix())/Period-1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !Validate(rfcSecret, previous, base) {
		t.Fatal("expected a previous-step code to validate within the skew window")
	}
}

func TestGenerateSecretIsDecodableAndUnique(t *testing.T) {
	first, err := GenerateSecret()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	second, err := GenerateSecret()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if first == second {
		t.Fatal("expected unique secrets")
	}
	if _, err := code(first, 0); err != nil {
		t.Fatalf("generated secret was not usable: %v", err)
	}
}

func TestOTPAuthURI(t *testing.T) {
	uri := OTPAuthURI("Name", "user@example.com", rfcSecret)
	if !strings.HasPrefix(uri, "otpauth://totp/") {
		t.Fatalf("unexpected uri: %s", uri)
	}
	if !strings.Contains(uri, "secret="+rfcSecret) {
		t.Fatalf("uri missing secret: %s", uri)
	}
}
