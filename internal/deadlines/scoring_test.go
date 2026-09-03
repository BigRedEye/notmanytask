package deadlines

import (
	"testing"
	"time"
)

// Ported from the deleted internal/scorer/scorer_test.go: deadline at
// 20-07-1969 23:17, task score 9000, linear policy decaying to half over a
// week, exponential policy with a 5-day multiplier and 0.3 threshold.
func TestScoringPolicies(t *testing.T) {
	deadline := time.Date(1969, 7, 20, 23, 17, 0, 0, time.UTC)
	const maxScore = 9000

	linear := &LinearScore{After: 7 * 24 * time.Hour, Multiplier: 0.5}
	exponential := &ExponentialScore{Multiplier: 5 * 24 * time.Hour, Threshold: 0.3}

	tests := []struct {
		name        string
		submit      time.Time
		linear      int
		exponential int
	}{
		{"before deadline", deadline.Add(-time.Hour), 9000, 9000},
		{"at deadline", deadline, 9000, 9000},
		{"one minute after", deadline.Add(time.Minute), 8999, 8998},
		{"one hour after", deadline.Add(time.Hour), 8973, 8925},
		{"17 hours after", deadline.Add(17 * time.Hour), 8544, 7811},
		{"41 hours after", deadline.Add(41 * time.Hour), 7901, 6395},
		{"end of decay window", deadline.Add(7*24*time.Hour - 3*time.Minute), 4501, 2700},
		{"past decay window", deadline.Add(7 * 24 * time.Hour), 4500, 2700},
		{"ten years after", deadline.AddDate(10, 0, 0), 4500, 2700},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if score := linear.Score(maxScore, deadline, tt.submit); score != tt.linear {
				t.Errorf("linear score = %d, expected %d", score, tt.linear)
			}
			if score := exponential.Score(maxScore, deadline, tt.submit); score != tt.exponential {
				t.Errorf("exponential score = %d, expected %d", score, tt.exponential)
			}
		})
	}
}
