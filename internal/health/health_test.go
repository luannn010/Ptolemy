package health

import (
	"context"
	"net/http"
	"testing"
	"time"
)

type fakeChecker struct{ c Check }

func (f fakeChecker) Name() string                  { return f.c.Name }
func (f fakeChecker) Required() bool                { return f.c.Required }
func (f fakeChecker) Check(_ context.Context) Check { return f.c }

// hungChecker ignores its context and blocks forever — the worst-case a Checker
// can be. Run must abandon it at the ceiling rather than block.
type hungChecker struct {
	name     string
	required bool
}

func (h hungChecker) Name() string   { return h.name }
func (h hungChecker) Required() bool { return h.required }
func (h hungChecker) Check(_ context.Context) Check {
	select {} // never returns
}

func TestOverall(t *testing.T) {
	cases := []struct {
		name       string
		checks     []Check
		wantStatus string
		wantCode   int
	}{
		{
			name:       "all up",
			checks:     []Check{{Status: StatusUp, Required: true}, {Status: StatusUp, Required: false}},
			wantStatus: "ok",
			wantCode:   http.StatusOK,
		},
		{
			name:       "required down",
			checks:     []Check{{Status: StatusUp, Required: true}, {Status: StatusDown, Required: true}},
			wantStatus: "unhealthy",
			wantCode:   http.StatusServiceUnavailable,
		},
		{
			name:       "optional down",
			checks:     []Check{{Status: StatusUp, Required: true}, {Status: StatusDown, Required: false}},
			wantStatus: "degraded",
			wantCode:   http.StatusOK,
		},
		{
			name:       "optional disabled is fine",
			checks:     []Check{{Status: StatusUp, Required: true}, {Status: StatusDisabled, Required: false}},
			wantStatus: "ok",
			wantCode:   http.StatusOK,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotStatus, gotCode := overall(tc.checks)
			if gotStatus != tc.wantStatus || gotCode != tc.wantCode {
				t.Fatalf("overall() = (%q,%d), want (%q,%d)", gotStatus, gotCode, tc.wantStatus, tc.wantCode)
			}
		})
	}
}

func TestAggregatorRun(t *testing.T) {
	agg := &Aggregator{
		Timeout: 100 * time.Millisecond,
		Checkers: []Checker{
			fakeChecker{Check{Name: "a", Status: StatusUp, Required: true}},
			fakeChecker{Check{Name: "b", Status: StatusDown, Required: true}},
		},
	}
	report, code := agg.Run(context.Background())
	if code != http.StatusServiceUnavailable {
		t.Fatalf("code = %d, want 503", code)
	}
	if report.Status != "unhealthy" || report.Service != "workerd" {
		t.Fatalf("report = %+v", report)
	}
	if len(report.Checks) != 2 || report.Checks[0].Name != "a" || report.Checks[1].Name != "b" {
		t.Fatalf("checks order wrong: %+v", report.Checks)
	}
	if report.Timestamp == "" {
		t.Fatal("timestamp empty")
	}
}

func TestAggregatorRun_AbandonsHungChecker(t *testing.T) {
	agg := &Aggregator{
		Timeout: 50 * time.Millisecond,
		Checkers: []Checker{
			fakeChecker{Check{Name: "ok", Status: StatusUp, Required: true}},
			hungChecker{name: "hung", required: true},
		},
	}
	start := time.Now()
	report, code := agg.Run(context.Background())
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("Run did not bound a hung checker: took %v", elapsed)
	}
	if code != http.StatusServiceUnavailable || report.Status != "unhealthy" {
		t.Fatalf("hung required checker should be unhealthy/503, got %s/%d", report.Status, code)
	}
	var found bool
	for _, c := range report.Checks {
		if c.Name == "hung" {
			found = true
			if c.Status != StatusDown || c.Error != "check timed out" || !c.Required {
				t.Fatalf("hung check = %+v, want down/timed out/required", c)
			}
		}
	}
	if !found {
		t.Fatal("hung check missing from report")
	}
}

func TestAggregatorRun_Empty(t *testing.T) {
	agg := &Aggregator{}
	report, code := agg.Run(context.Background())
	if report.Status != "ok" || code != http.StatusOK || len(report.Checks) != 0 {
		t.Fatalf("empty aggregator = %s/%d/%d checks", report.Status, code, len(report.Checks))
	}
}
