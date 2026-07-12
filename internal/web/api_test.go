package web

import "testing"

func TestParseBenchmarkMetric(t *testing.T) {
	for _, raw := range []string{"NaN", "+Inf", "-Inf", "not-a-number"} {
		if _, err := parseBenchmarkMetric(raw); err == nil {
			t.Errorf("parseBenchmarkMetric(%q) unexpectedly succeeded", raw)
		}
	}

	metric, err := parseBenchmarkMetric("12.5")
	if err != nil {
		t.Fatal(err)
	}
	if metric != 12.5 {
		t.Fatalf("unexpected metric: got %v, want 12.5", metric)
	}
}
