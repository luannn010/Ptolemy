package brain

import (
	"strings"
	"testing"
)

func TestChatMessagesForVoice(t *testing.T) {
	msgs := ChatMessagesForVoice("whats the capital of france")
	if len(msgs) != 2 {
		t.Fatalf("want 2 messages, got %d", len(msgs))
	}
	if msgs[0].Role != "system" {
		t.Errorf("first message role = %q, want system", msgs[0].Role)
	}
	if msgs[0].Content != VoiceSystemPrompt {
		t.Errorf("first message content is not VoiceSystemPrompt")
	}
	if msgs[1].Role != "user" {
		t.Errorf("second message role = %q, want user", msgs[1].Role)
	}
	if msgs[1].Content != "whats the capital of france" {
		t.Errorf("user content altered: %q", msgs[1].Content)
	}
}

func TestVoiceSystemPromptMentionsCorrectionAndConciseness(t *testing.T) {
	p := strings.ToLower(VoiceSystemPrompt)
	for _, kw := range []string{"speech-to-text", "correct", "concise"} {
		if !strings.Contains(p, kw) {
			t.Errorf("VoiceSystemPrompt should mention %q", kw)
		}
	}
}
