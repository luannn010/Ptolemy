// Package health provides a deep readiness probe for workerd's dependencies.
// Each dependency is a Checker; an Aggregator runs them in parallel and composes
// an overall status and HTTP code. All checks are read-only — no guarded adapter
// is touched, so this package needs no Guarded* wrapper.
package health

import (
	"context"
	"net/http"
	"time"
)

type Status string

const (
	StatusUp       Status = "up"
	StatusDown     Status = "down"
	StatusDisabled Status = "disabled"
)

// Check is one dependency's result. Required is authoritative-stamped by
// Aggregator.Run from Checker.Required(); a Checker's own Check method need not
// (and should not) set it — Run overwrites it so the two cannot diverge.
type Check struct {
	Name      string `json:"name"`
	Status    Status `json:"status"`
	Required  bool   `json:"required"`
	LatencyMS int64  `json:"latency_ms,omitempty"`
	Error     string `json:"error,omitempty"`
}

// Checker probes a single dependency. Implementations should honor the context
// deadline the Aggregator supplies; Run enforces a hard ceiling regardless, so a
// misbehaving checker degrades to a synthesized "down" rather than hanging /health.
// Required reports whether a down result makes the whole report unhealthy — it is
// queried independently of Check so Run can verdict an abandoned check correctly.
type Checker interface {
	Name() string
	Required() bool
	Check(ctx context.Context) Check
}

// Report is the rendered /health body.
type Report struct {
	Status    string  `json:"status"` // ok | degraded | unhealthy
	Service   string  `json:"service"`
	Timestamp string  `json:"timestamp"`
	Checks    []Check `json:"checks"`
}

// Aggregator runs all checkers in parallel under a per-check timeout.
type Aggregator struct {
	Checkers []Checker
	Timeout  time.Duration
}

// Run probes every checker concurrently and returns the composed report and the
// HTTP status code the handler should write. Each checker runs under a per-check
// timeout; Run additionally enforces a hard overall ceiling so a checker that
// ignores its context cannot block /health forever — any slot not reported by the
// ceiling is synthesized as a "check timed out" down (still honoring Required).
func (a *Aggregator) Run(ctx context.Context) (Report, int) {
	n := len(a.Checkers)
	checks := make([]Check, n)

	type result struct {
		i int
		c Check
	}
	// Buffered to n so a goroutine abandoned past the ceiling can still send its
	// (late) result and exit instead of leaking blocked on the channel.
	results := make(chan result, n)
	for i, c := range a.Checkers {
		go func(i int, c Checker) {
			cctx := ctx
			if a.Timeout > 0 {
				var cancel context.CancelFunc
				cctx, cancel = context.WithTimeout(ctx, a.Timeout)
				defer cancel()
			}
			results <- result{i, c.Check(cctx)}
		}(i, c)
	}

	timer := time.NewTimer(a.ceiling())
	defer timer.Stop()

	filled := make([]bool, n)
	for got := 0; got < n; {
		select {
		case r := <-results:
			// Required() is the single source of truth — stamp it onto every
			// result so a Check() implementation cannot diverge from it.
			r.c.Required = a.Checkers[r.i].Required()
			checks[r.i] = r.c
			filled[r.i] = true
			got++
		case <-timer.C:
			for i := range checks {
				if !filled[i] {
					checks[i] = Check{
						Name:     a.Checkers[i].Name(),
						Required: a.Checkers[i].Required(),
						Status:   StatusDown,
						Error:    "check timed out",
					}
				}
			}
			got = n
		}
	}

	status, code := overall(checks)
	return Report{
		Status:    status,
		Service:   "workerd",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Checks:    checks,
	}, code
}

// ceiling bounds the whole Run. It allows each honest checker to hit its own
// per-check timeout (a.Timeout) and report a clean "down" before Run abandons it;
// the grace covers scheduling slack. With no per-check timeout configured it falls
// back to a fixed bound so Run is never unbounded.
func (a *Aggregator) ceiling() time.Duration {
	if a.Timeout > 0 {
		return a.Timeout + 500*time.Millisecond
	}
	return 5 * time.Second
}

// overall maps a set of checks to an aggregate status string and HTTP code:
// any required check down -> unhealthy/503; else any optional down -> degraded/200;
// else ok/200. Disabled checks never degrade the aggregate.
func overall(checks []Check) (string, int) {
	degraded := false
	for _, c := range checks {
		if c.Status != StatusDown {
			continue
		}
		if c.Required {
			return "unhealthy", http.StatusServiceUnavailable
		}
		degraded = true
	}
	if degraded {
		return "degraded", http.StatusOK
	}
	return "ok", http.StatusOK
}
