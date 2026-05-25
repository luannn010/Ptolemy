package apitypes

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/luannn010/ptolemy/internal/domain"
)

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

