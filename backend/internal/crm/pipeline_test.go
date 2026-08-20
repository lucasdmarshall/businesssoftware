package crm

import "testing"

func TestStageProbability(t *testing.T) {
	cases := map[string]float64{
		"prospect":  10,
		"qualified": 35,
		"proposal":  60,
		"won":       100,
		"lost":      0,
		"other":     10,
	}
	for stage, want := range cases {
		if got := stageProbability(stage); got != want {
			t.Fatalf("stageProbability(%s)=%v want %v", stage, got, want)
		}
	}
}
