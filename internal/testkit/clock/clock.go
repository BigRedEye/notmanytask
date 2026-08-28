// Package clock provides deterministic clocks and tickers for tests.
package clock

import (
	"errors"
	"sync"
	"time"
)

var (
	// ErrNegativeAdvance is returned when a fixed clock is asked to move backwards.
	ErrNegativeAdvance = errors.New("clock: negative advance")
	// ErrTickerStopped is returned when a stopped manual ticker is ticked.
	ErrTickerStopped = errors.New("clock: ticker stopped")
	// ErrTickPending is returned when the previous manual tick has not been consumed.
	ErrTickPending = errors.New("clock: tick pending")
)

// Clock is the clock surface consumed by deterministic worker tests.
type Clock interface {
	Now() time.Time
}

// Ticker is the ticker surface consumed by deterministic worker tests.
type Ticker interface {
	Chan() <-chan time.Time
	Stop()
}

// Fixed is a thread-safe clock whose time changes only when explicitly set or
// advanced.
type Fixed struct {
	mu  sync.RWMutex
	now time.Time
}

// NewFixed constructs a fixed clock at start.
func NewFixed(start time.Time) *Fixed {
	return &Fixed{now: start}
}

// Now returns the clock's current time.
func (f *Fixed) Now() time.Time {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.now
}

// Set changes the clock's current time.
func (f *Fixed) Set(now time.Time) {
	f.mu.Lock()
	f.now = now
	f.mu.Unlock()
}

// Advance moves the clock forward by d and returns the resulting time. A
// negative duration is rejected without changing the clock.
func (f *Fixed) Advance(d time.Duration) (time.Time, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if d < 0 {
		return f.now, ErrNegativeAdvance
	}
	f.now = f.now.Add(d)
	return f.now, nil
}

// ManualTicker is a thread-safe ticker driven explicitly by tests. It has room
// for exactly one undrained tick and never starts a goroutine or real timer.
type ManualTicker struct {
	mu      sync.Mutex
	ch      chan time.Time
	stopped bool
}

// NewManualTicker constructs an active manual ticker.
func NewManualTicker() *ManualTicker {
	return &ManualTicker{ch: make(chan time.Time, 1)}
}

// Chan returns the ticker's receive-only channel.
func (t *ManualTicker) Chan() <-chan time.Time {
	return t.ch
}

// Tick queues at without blocking. It rejects ticks after Stop and while an
// earlier tick remains undrained.
func (t *ManualTicker) Tick(at time.Time) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.stopped {
		return ErrTickerStopped
	}
	select {
	case t.ch <- at:
		return nil
	default:
		return ErrTickPending
	}
}

// Stop prevents future ticks. Like time.Ticker.Stop, it does not close the
// channel or discard a tick that was already queued.
func (t *ManualTicker) Stop() {
	t.mu.Lock()
	t.stopped = true
	t.mu.Unlock()
}

var _ Clock = (*Fixed)(nil)
var _ Ticker = (*ManualTicker)(nil)
