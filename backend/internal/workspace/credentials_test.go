package workspace

import "testing"

func TestFirstNameToken(t *testing.T) {
	if got := firstNameToken("Lucas Admin"); got != "lucas" {
		t.Fatalf("got %q", got)
	}
	if got := firstNameToken("  Ada  Lovelace "); got != "ada" {
		t.Fatalf("got %q", got)
	}
	if got := firstNameToken(""); got != "user" {
		t.Fatalf("empty expected user, got %q", got)
	}
}

func TestIdentityForAdministrations(t *testing.T) {
	id := IdentityForDepartmentSlug("administrations")
	if id.UsernameTag != "admin" || id.EmpPrefix != "ADM" {
		t.Fatalf("got %+v", id)
	}
}

func TestIdentityFormats(t *testing.T) {
	id := IdentityForDepartmentSlug("administrations")
	seq := 1
	seqStr := "000001"
	username := firstNameToken("Lucas Smith") + "_" + id.UsernameTag + "_" + seqStr
	password := "password" + seqStr
	emp := id.EmpPrefix + seqStr
	if username != "lucas_admin_000001" {
		t.Fatalf("username %q", username)
	}
	if password != "password000001" {
		t.Fatalf("password %q", password)
	}
	if emp != "ADM000001" {
		t.Fatalf("emp %q", emp)
	}
	if len(password) < 12 {
		t.Fatalf("password too short for policy: %q", password)
	}
	_ = seq
}

func TestDeriveCustomDeptIdentity(t *testing.T) {
	id := IdentityForDepartmentSlug("quality-assurance")
	if id.UsernameTag != "quality" {
		t.Fatalf("tag %q", id.UsernameTag)
	}
	if id.EmpPrefix != "QUA" {
		t.Fatalf("prefix %q", id.EmpPrefix)
	}
}
