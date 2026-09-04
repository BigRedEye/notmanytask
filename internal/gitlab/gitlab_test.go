package gitlab

import "testing"

func TestForkFinished(t *testing.T) {
	cases := map[string][2]bool{ // status -> {ready, failed}
		"finished":  {true, false},
		"none":      {true, false},
		"":          {true, false},
		"scheduled": {false, false},
		"started":   {false, false},
		"failed":    {false, true},
	}
	for status, want := range cases {
		ready, failed := forkFinished(status)
		if ready != want[0] || failed != want[1] {
			t.Errorf("%q: ready=%v failed=%v, want %v", status, ready, failed, want)
		}
	}
}
