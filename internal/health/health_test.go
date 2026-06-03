package health

import (
	"context"
	"net/http"
	"testing"
	"time"
)

type fakeChecker struct{ c Check }

func (f fakeChecker) Name() string                 { return f.c.Name }
func (f fakeChecker) Check(_ context.Context) Check { return f.c }

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
