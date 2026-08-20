package finance

import "testing"

func TestAgingBucketFor(t *testing.T) {
	cases := []struct {
		days int
		want string
	}{
		{-5, agingCurrent},
		{0, agingCurrent},
		{30, agingCurrent},
		{31, aging3160},
		{60, aging3160},
		{61, aging6190},
		{90, aging6190},
		{91, aging90Plus},
		{400, aging90Plus},
	}
	for _, tc := range cases {
		if got := agingBucketFor(tc.days); got != tc.want {
			t.Fatalf("agingBucketFor(%d)=%s want %s", tc.days, got, tc.want)
		}
	}
}

func TestDaysPastDue(t *testing.T) {
	asOf, _, _ := parseAsOf("2026-08-20")
	due, _, _ := parseAsOf("2026-08-01")
	if got := daysPastDue(asOf, due); got != 19 {
		t.Fatalf("daysPastDue= %d want 19", got)
	}
	future, _, _ := parseAsOf("2026-09-01")
	if got := daysPastDue(asOf, future); got != 0 {
		t.Fatalf("future due should be 0, got %d", got)
	}
}

func TestParseAsOf(t *testing.T) {
	if _, _, ok := parseAsOf("not-a-date"); ok {
		t.Fatal("expected invalid as_of to fail")
	}
	_, str, ok := parseAsOf("2026-01-15")
	if !ok || str != "2026-01-15" {
		t.Fatalf("parseAsOf failed: ok=%v str=%s", ok, str)
	}
}
