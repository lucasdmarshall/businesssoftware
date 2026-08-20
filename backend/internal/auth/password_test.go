package auth

import "testing"

func TestHashPasswordRejectsShortPasswords(t *testing.T) {
	if _, err := HashPassword("short"); err == nil {
		t.Fatal("expected an error for a password under 12 characters")
	}
}

func TestHashPasswordProducesArgon2idEncoding(t *testing.T) {
	hash, err := HashPassword("correct-horse-battery")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hash == "" {
		t.Fatal("expected a non-empty hash")
	}
	if got := hash[:9]; got != "$argon2id" {
		t.Fatalf("expected an argon2id prefix, got %q", got)
	}
}

func TestHashPasswordIsSaltedPerCall(t *testing.T) {
	first, err := HashPassword("correct-horse-battery")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	second, err := HashPassword("correct-horse-battery")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if first == second {
		t.Fatal("expected a unique salt per hash so identical passwords differ")
	}
}

func TestVerifyPassword(t *testing.T) {
	password := "correct-horse-battery"
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !VerifyPassword(password, hash) {
		t.Fatal("expected the correct password to verify")
	}
	if VerifyPassword("wrong-horse-battery-staple", hash) {
		t.Fatal("expected a wrong password to fail verification")
	}
}

func TestVerifyPasswordRejectsMalformedEncoding(t *testing.T) {
	for _, encoded := range []string{"", "not-a-hash", "$argon2id$v=19$bad$salt$hash"} {
		if VerifyPassword("correct-horse-battery", encoded) {
			t.Fatalf("expected malformed encoding %q to fail verification", encoded)
		}
	}
}
