package workspace

import "testing"

func TestMemberExistsSQL(t *testing.T) {
	got := MemberExistsSQL("l.requested_by", 2)
	if !contains(got, "$2") || !contains(got, "l.requested_by") {
		t.Fatalf("unexpected SQL: %s", got)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
