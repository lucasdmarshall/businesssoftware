package workspace

import "testing"

func TestSlugifyUsername(t *testing.T) {
	if got := slugifyUsername("Ada Lovelace"); got != "adalovelace" {
		t.Fatalf("got %q", got)
	}
	if got := slugifyUsername("  "); got != "" {
		t.Fatalf("empty expected, got %q", got)
	}
}

func TestGeneratePasswordLength(t *testing.T) {
	pw, err := GeneratePassword()
	if err != nil {
		t.Fatal(err)
	}
	if len(pw) < 12 {
		t.Fatalf("password too short: %q", pw)
	}
}
