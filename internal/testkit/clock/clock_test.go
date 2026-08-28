package clock

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func TestFixedAdvance(t *testing.T) {
	start := time.Date(2026, time.August, 28, 12, 30, 0, 123, time.UTC)
	clock := NewFixed(start)

	got, err := clock.Advance(72*time.Hour + 5*time.Minute)
	if err != nil {
		t.Fatalf("Advance returned an error: %v", err)
	}
	want := start.Add(72*time.Hour + 5*time.Minute)
	if !got.Equal(want) || !clock.Now().Equal(want) {
		t.Fatalf("advanced time = %v, Now = %v, want %v", got, clock.Now(), want)
	}

	set := start.Add(10 * time.Second)
	clock.Set(set)
	if got := clock.Now(); !got.Equal(set) {
		t.Fatalf("Now after Set = %v, want %v", got, set)
	}
}

func TestFixedRejectsNegativeAdvance(t *testing.T) {
	start := time.Date(2026, time.August, 28, 12, 30, 0, 0, time.UTC)
	clock := NewFixed(start)

	got, err := clock.Advance(-time.Nanosecond)
	if !errors.Is(err, ErrNegativeAdvance) {
		t.Fatalf("Advance error = %v, want %v", err, ErrNegativeAdvance)
	}
	if !got.Equal(start) || !clock.Now().Equal(start) {
		t.Fatalf("negative Advance changed time: returned %v, Now %v, want %v", got, clock.Now(), start)
	}
}

func TestManualTickerPendingAndStop(t *testing.T) {
	ticker := NewManualTicker()
	first := time.Date(2026, time.August, 28, 12, 30, 0, 0, time.UTC)
	second := first.Add(time.Second)

	if err := ticker.Tick(first); err != nil {
		t.Fatalf("first Tick returned an error: %v", err)
	}
	if err := ticker.Tick(second); !errors.Is(err, ErrTickPending) {
		t.Fatalf("pending Tick error = %v, want %v", err, ErrTickPending)
	}
	if got := <-ticker.Chan(); !got.Equal(first) {
		t.Fatalf("received tick = %v, want %v", got, first)
	}
	if err := ticker.Tick(second); err != nil {
		t.Fatalf("Tick after drain returned an error: %v", err)
	}

	ticker.Stop()
	ticker.Stop()
	if err := ticker.Tick(second.Add(time.Second)); !errors.Is(err, ErrTickerStopped) {
		t.Fatalf("Tick after Stop error = %v, want %v", err, ErrTickerStopped)
	}
	if got := <-ticker.Chan(); !got.Equal(second) {
		t.Fatalf("Stop discarded queued tick: got %v, want %v", got, second)
	}
	select {
	case got := <-ticker.Chan():
		t.Fatalf("stopped ticker channel unexpectedly produced %v", got)
	default:
	}
}

func TestManualTickerConcurrentTickStop(t *testing.T) {
	for i := 0; i < 100; i++ {
		ticker := NewManualTicker()
		start := make(chan struct{})
		result := make(chan error, 1)
		var workers sync.WaitGroup
		workers.Add(2)
		go func() {
			defer workers.Done()
			<-start
			result <- ticker.Tick(time.Unix(int64(i), 0))
		}()
		go func() {
			defer workers.Done()
			<-start
			ticker.Stop()
		}()
		close(start)
		workers.Wait()

		err := <-result
		if err != nil && !errors.Is(err, ErrTickerStopped) {
			t.Fatalf("iteration %d: concurrent Tick error = %v", i, err)
		}
		if err == nil {
			<-ticker.Chan()
		}
		if err := ticker.Tick(time.Unix(int64(i+1), 0)); !errors.Is(err, ErrTickerStopped) {
			t.Fatalf("iteration %d: Tick after concurrent Stop = %v, want %v", i, err, ErrTickerStopped)
		}
	}
}
