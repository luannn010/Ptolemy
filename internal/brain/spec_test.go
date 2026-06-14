package brain

import "testing"

func TestSpec_WithDefaultsFillsEmptyOnly(t *testing.T) {
	s := Spec{GGUF: "/m/x.gguf", Args: []string{"-ngl", "999"}}.
		WithDefaults("/bin/llama-server", "0.0.0.0", "9000")
	if s.Binary != "/bin/llama-server" || s.Host != "0.0.0.0" || s.Port != "9000" {
		t.Fatalf("defaults not filled: %+v", s)
	}
	s2 := Spec{Binary: "/custom", Host: "127.0.0.1", Port: "1", GGUF: "/m.gguf"}.
		WithDefaults("/bin/def", "0.0.0.0", "9000")
	if s2.Binary != "/custom" || s2.Host != "127.0.0.1" || s2.Port != "1" {
		t.Fatalf("defaults overrode explicit values: %+v", s2)
	}
}

func TestSpec_Validate(t *testing.T) {
	if err := (Spec{Binary: "/b", GGUF: "/g"}).Validate(); err != nil {
		t.Fatalf("valid spec rejected: %v", err)
	}
	if err := (Spec{GGUF: "/g"}).Validate(); err == nil {
		t.Fatal("missing binary must error")
	}
	if err := (Spec{Binary: "/b"}).Validate(); err == nil {
		t.Fatal("missing gguf must error")
	}
}

func TestSpec_Argv(t *testing.T) {
	got := Spec{Binary: "/b", GGUF: "/g", Host: "0.0.0.0", Port: "9000", Args: []string{"--ctx-size", "32768"}}.Argv()
	want := []string{"/b", "-m", "/g", "--host", "0.0.0.0", "--port", "9000", "--ctx-size", "32768"}
	if len(got) != len(want) {
		t.Fatalf("argv len: got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("argv[%d]=%q want %q (%v)", i, got[i], want[i], got)
		}
	}
}

func TestSpec_BaseURLNormalizesWildcard(t *testing.T) {
	if u := (Spec{Host: "0.0.0.0", Port: "9000"}).BaseURL(); u != "http://127.0.0.1:9000" {
		t.Fatalf("0.0.0.0 must probe loopback, got %q", u)
	}
	if u := (Spec{Host: "10.0.0.5", Port: "8080"}).BaseURL(); u != "http://10.0.0.5:8080" {
		t.Fatalf("explicit host: got %q", u)
	}
}
