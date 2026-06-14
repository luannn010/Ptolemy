package apitypes

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/luannn010/ptolemy/internal/domain"
)

func TestChatResponseOmitsTraceFields(t *testing.T) {
	v := ChatResponse{Answer: "a", Citations: []string{}, GaveUp: false}
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, absent := range []string{`"mode"`, `"steps"`} {
		if strings.Contains(s, absent) {
			t.Fatalf("expected %s omitted when trace absent, got %s", absent, s)
		}
	}
	// gave_up must always be present, even when false.
	if !strings.Contains(s, `"gave_up"`) {
		t.Fatalf("gave_up must always serialize, got %s", s)
	}
	if !strings.Contains(s, `"citations":[]`) {
		t.Fatalf("citations must serialize as an array, got %s", s)
	}
}

func TestNeedsConfirmationJSONShape(t *testing.T) {
	v := NeedsConfirmation{
		Status:     "needs_confirmation",
		Channel:    domain.ChannelOOB,
		IntentHash: "abc",
		PendingID:  "p1",
		Reason:     "confirm",
	}
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, key := range []string{"status", "channel", "intent_hash", "pending_id", "reason"} {
		if !strings.Contains(s, `"`+key+`"`) {
			t.Fatalf("missing key %s in %s", key, s)
		}
	}
}

