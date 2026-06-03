package eval

import "testing"

func TestScoreSummary(t *testing.T) {
	ok, clean := ScoreSummary("archive threshold is 0.1 and sweep is hourly", []string{"0.1", "hourly"}, []string{"0.2"})
	if !ok || !clean {
		t.Fatalf("want pass+clean, got ok=%v clean=%v", ok, clean)
	}
	ok, clean = ScoreSummary("archive threshold is 0.2", []string{"0.1"}, []string{"0.2"})
	if ok || clean {
		t.Fatalf("stale summary must fail both, got ok=%v clean=%v", ok, clean)
	}
}

func TestLoadSynthScenarios(t *testing.T) {
	ss, err := LoadSynthScenarios("testdata/synth_scenarios.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(ss) < 12 {
		t.Fatalf("want >= 12 seed scenarios, got %d", len(ss))
	}
	for _, s := range ss {
		if s.Query == "" || len(s.ExpectKeywords) == 0 || len(s.Sessions) == 0 || s.Subject == "" || s.Project == "" {
			t.Fatalf("scenario %s malformed", s.ID)
		}
	}
}
