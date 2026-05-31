package memory

import "testing"

func TestGate_SkipsTrivial(t *testing.T) {
	cases := []Exchange{
		{UserText: "hi", AssistantText: "hello!"},
		{UserText: "thanks", AssistantText: "you're welcome"},
		{UserText: "ok", AssistantText: "👍"},
		{UserText: "", AssistantText: ""},
	}
	for _, ex := range cases {
		if d := Gate(ex); !d.Skip {
			t.Errorf("expected skip for %q/%q, got %+v", ex.UserText, ex.AssistantText, d)
		}
	}
}

func TestGate_KeepsSubstantive(t *testing.T) {
	ex := Exchange{
		UserText:      "how should the GC decide what to archive?",
		AssistantText: "the sweep archives project rows whose decay score falls below the threshold",
	}
	if d := Gate(ex); d.Skip {
		t.Errorf("expected keep, got skip: %+v", d)
	}
}

func TestGate_FlagsSecrets(t *testing.T) {
	secrets := []string{
		"my key is sk-xxxxxxxxxxxxxxxxxxxxxxxxxxxx",
		"AWS key AKIAIOSFODNN7EXAMPLE here",
		"Authorization: Bearer xxxxxxxxxxxxxxxxxxxxxxxx",
		"-----BEGIN RSA PRIVATE KEY-----",
		"here it is Xx0Xx0Xx0Xx0Xx0Xx0Xx0Xx0Xx0Xx0Xx0Xx0Xx0Xx0==", // low-entropy 40+ char filler, no label
	}
	for _, s := range secrets {
		ex := Exchange{UserText: s, AssistantText: "noted, I stored the credential for you safely"}
		d := Gate(ex)
		if !d.ContainsSecret || !d.Skip {
			t.Errorf("expected secret skip for %q, got %+v", s, d)
		}
	}
}
