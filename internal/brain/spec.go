package brain

import (
	"errors"
	"strings"
)

// Spec is one free-form llama.cpp launch configuration. It replaces the named
// Model registry: the caller supplies the whole command. The Manager fills empty
// Binary/Host/Port from its configured defaults via WithDefaults.
type Spec struct {
	Binary string   `json:"binary"`
	GGUF   string   `json:"gguf"`
	Host   string   `json:"host"`
	Port   string   `json:"port"`
	Args   []string `json:"args"`
}

// WithDefaults returns a copy with empty Binary/Host/Port filled from the args.
func (s Spec) WithDefaults(binary, host, port string) Spec {
	if strings.TrimSpace(s.Binary) == "" {
		s.Binary = binary
	}
	if strings.TrimSpace(s.Host) == "" {
		s.Host = host
	}
	if strings.TrimSpace(s.Port) == "" {
		s.Port = port
	}
	return s
}

// Validate ensures the spec can launch.
func (s Spec) Validate() error {
	if strings.TrimSpace(s.Binary) == "" {
		return errors.New("brain spec: binary is required (set BRAIN_LLAMA_BIN or pass binary)")
	}
	if strings.TrimSpace(s.GGUF) == "" {
		return errors.New("brain spec: gguf is required")
	}
	return nil
}

// Argv composes the llama-server command line. Used both to launch and (in
// GuardedBrain) to build the policy intent, so the full command is audited and
// hashed.
func (s Spec) Argv() []string {
	argv := []string{s.Binary, "-m", s.GGUF, "--host", s.Host, "--port", s.Port}
	return append(argv, s.Args...)
}

// BaseURL is the HTTP endpoint the spec serves on, for readiness probing. A
// 0.0.0.0/empty bind is probed on loopback.
func (s Spec) BaseURL() string {
	host := s.Host
	if host == "" || host == "0.0.0.0" {
		host = "127.0.0.1"
	}
	return "http://" + host + ":" + s.Port
}
