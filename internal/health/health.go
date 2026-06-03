// Package health provides a deep readiness probe for workerd's dependencies.
// Each dependency is a Checker; an Aggregator runs them in parallel and composes
// an overall status and HTTP code. All checks are read-only — no guarded adapter
// is touched, so this package needs no Guarded* wrapper.
package health

import (
	"context"
	"net/http"
	"sync"
	"time"
)

type Status string

const (
	StatusUp       Status = "up"
	StatusDown     Status = "down"
	StatusDisabled Status = "disabled"
)

// Check is one dependency's result.
type Check struct {
	Name      string `json:"name"`
	Status    Status `json:"status"`
	Required  bool   `json:"required"`
	LatencyMS int64  `json:"latency_ms,omitempty"`
	Error     string `json:"error,omitempty"`
}

// Checker probes a single dependency. Implementations must not block past the
// context deadline the Aggregator supplies.
type Checker interface {
	Name() string
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
// HTTP status code the handler should write.
func (a *Aggregator) Run(ctx context.Context) (Report, int) {
	checks := make([]Check, len(a.Checkers))
	var wg sync.WaitGroup
	for i, c := range a.Checkers {
		wg.Add(1)
		go func(i int, c Checker) {
			defer wg.Done()
			cctx := ctx
			if a.Timeout > 0 {
				var cancel context.CancelFunc
				cctx, cancel = context.WithTimeout(ctx, a.Timeout)
				defer cancel()
			}
			checks[i] = c.Check(cctx)
		}(i, c)
	}
	wg.Wait()

	status, code := overall(checks)
	return Report{
		Status:    status,
		Service:   "workerd",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Checks:    checks,
	}, code
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
