package brain

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// Launcher spawns the brain process and returns a Handle to control it. Injected
// so the Manager logic is testable without a real process.
type Launcher interface {
	Start(argv []string) (Handle, error)
}

// Handle is a running brain process.
type Handle interface {
	Signal(sig os.Signal) error // graceful stop request
	Kill() error                // forceful
	Wait() error                // block until the process exits
	Running() bool              // still alive?
}

// Probe reports whether a brain HTTP endpoint is serving (GET {baseURL}/v1/models).
// The target is passed at probe time so a custom-port load is probed correctly.
type Probe interface {
	Ready(ctx context.Context, baseURL string) bool
}

// ErrNoModelLoaded is returned by Resume/EnsureAwake when no spec has been loaded
// yet (cold start) — there is nothing to bring back.
var ErrNoModelLoaded = errors.New("no model loaded")

// Status is a snapshot of the brain process.
type Status struct {
	Running    bool      `json:"running"`
	Hibernated bool      `json:"hibernated"` // a spec is stored but not running
	Binary     string    `json:"binary,omitempty"`
	GGUF       string    `json:"gguf,omitempty"`
	Host       string    `json:"host,omitempty"`
	Port       string    `json:"port,omitempty"`
	Args       []string  `json:"args,omitempty"`
	LastUse    time.Time `json:"last_use"`
}

// Manager owns the single brain child process. RAW mechanism — every caller
// reaches it only through policy.GuardedBrain. All methods are mutex-serialized.
// The stored spec is set only by Load, persists across Hibernate, and is cleared
// only by Stop. This is the security invariant: a custom spec can enter only via
// Load (which the guard gates ask/OOB).
type Manager struct {
	launcher Launcher
	probe    Probe

	defBinary string
	defHost   string
	defPort   string
	modelsDir string

	readyTimeout time.Duration
	pollInterval time.Duration
	stopTimeout  time.Duration

	mu      sync.Mutex
	handle  Handle
	spec    Spec
	hasSpec bool
	lastUse time.Time
}

func NewManager(launcher Launcher, probe Probe, defaultBinary, defaultHost, defaultPort, modelsDir string) *Manager {
	return &Manager{
		launcher:     launcher,
		probe:        probe,
		defBinary:    defaultBinary,
		defHost:      defaultHost,
		defPort:      defaultPort,
		modelsDir:    modelsDir,
		readyTimeout: 90 * time.Second,
		pollInterval: 500 * time.Millisecond,
		stopTimeout:  15 * time.Second,
	}
}

// ResolveSpec fills empty Binary/Host/Port from the manager defaults. Pure; used
// by GuardedBrain to build the policy intent over the resolved command.
func (m *Manager) ResolveSpec(s Spec) Spec {
	return s.WithDefaults(m.defBinary, m.defHost, m.defPort)
}

// Load launches the given spec (defaults filled), waiting for readiness, and
// stores it as the active spec.
func (m *Manager) Load(ctx context.Context, s Spec) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s = s.WithDefaults(m.defBinary, m.defHost, m.defPort)
	if err := s.Validate(); err != nil {
		return err
	}
	return m.launchLocked(ctx, s)
}

// Resume relaunches the stored spec (e.g. after hibernate). ErrNoModelLoaded if
// nothing was ever loaded.
func (m *Manager) Resume(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.hasSpec {
		return ErrNoModelLoaded
	}
	if m.handle != nil && m.handle.Running() {
		m.lastUse = time.Now()
		return nil
	}
	return m.launchLocked(ctx, m.spec)
}

// EnsureAwake is the /chat auto-wake hook: no-op when running, resume when
// hibernated, ErrNoModelLoaded when cold.
func (m *Manager) EnsureAwake(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.handle != nil && m.handle.Running() {
		m.lastUse = time.Now()
		return nil
	}
	if !m.hasSpec {
		return ErrNoModelLoaded
	}
	return m.launchLocked(ctx, m.spec)
}

// Hibernate stops the process but keeps the spec for a later resume.
func (m *Manager) Hibernate(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopProcessLocked()
	return nil
}

// Stop terminates the process AND forgets the spec (full teardown).
func (m *Manager) Stop(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopProcessLocked()
	m.spec = Spec{}
	m.hasSpec = false
	return nil
}

// ListModels scans the configured models dir for *.gguf.
func (m *Manager) ListModels() ([]DiscoveredModel, error) {
	return DiscoverModels(m.modelsDir)
}

// Status returns a snapshot.
func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	running := m.handle != nil && m.handle.Running()
	st := Status{Running: running, Hibernated: m.hasSpec && !running, LastUse: m.lastUse}
	if m.hasSpec {
		st.Binary, st.GGUF, st.Host, st.Port, st.Args = m.spec.Binary, m.spec.GGUF, m.spec.Host, m.spec.Port, m.spec.Args
	}
	return st
}

// launchLocked stops any current process, starts the spec, and waits for ready.
// On failure it tears down the partial process. mu must be held — and is held for
// the whole launch, including the readiness wait (up to readyTimeout). That
// deliberately serializes concurrent brain calls (including Status) during a cold
// load: a loading process should serialize wakes and is never "idle".
func (m *Manager) launchLocked(ctx context.Context, s Spec) error {
	if m.handle != nil && m.handle.Running() {
		m.stopProcessLocked()
	}
	h, err := m.launcher.Start(s.Argv())
	if err != nil {
		return fmt.Errorf("start brain: %w", err)
	}
	m.handle = h
	m.spec = s
	m.hasSpec = true
	if err := m.waitReady(ctx, s.BaseURL()); err != nil {
		m.stopProcessLocked()
		return err
	}
	m.lastUse = time.Now()
	return nil
}

func (m *Manager) waitReady(ctx context.Context, baseURL string) error {
	deadline := time.Now().Add(m.readyTimeout)
	for {
		if m.handle == nil || !m.handle.Running() {
			return errors.New("brain process exited during startup")
		}
		if m.probe.Ready(ctx, baseURL) {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("brain not ready within %s", m.readyTimeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(m.pollInterval):
		}
	}
}

// stopProcessLocked signals→waits→kills the handle. It does NOT touch the stored
// spec. mu must be held.
func (m *Manager) stopProcessLocked() {
	if m.handle == nil {
		return
	}
	_ = m.handle.Signal(os.Interrupt)
	done := make(chan struct{})
	go func() { _ = m.handle.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(m.stopTimeout):
		_ = m.handle.Kill()
	}
	m.handle = nil
}

// RunIdleLoop ticks every interval and, when status reports the brain has been
// idle longer than ttl, calls unload. The idleness CHECK is a read (no policy
// gate / no audit row per tick); only the actual unload goes through the
// (gated) unload func, so the audit log records one row per real unload, not one
// per tick. Tolerates unload errors; stops on ctx cancel. Testable core.
func RunIdleLoop(ctx context.Context, interval, ttl time.Duration, status func() Status, unload func(context.Context) error) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			st := status()
			if st.Running && time.Since(st.LastUse) >= ttl {
				if err := unload(ctx); err != nil {
					log.Error().Err(err).Msg("brain idle unload failed; retrying next interval")
				}
			}
		}
	}
}

// --- production Launcher + Probe (thin; exercised live, not in unit tests) ----

type execLauncher struct {
	logTo *os.File
}

// NewExecLauncher spawns llama-server as a child process, reaping it in the
// background so Running()/Wait() reflect reality. Output goes to logTo (or
// os.Stderr when nil).
func NewExecLauncher(logTo *os.File) Launcher { return &execLauncher{logTo: logTo} }

func (l *execLauncher) Start(argv []string) (Handle, error) {
	if len(argv) == 0 {
		return nil, errors.New("empty argv")
	}
	// argv is the full llama-server command from a brain Spec. This is the RAW
	// launcher, reachable ONLY through policy.GuardedBrain: a caller-supplied spec
	// arrives here only via the load path, which is ask/OOB (human-approved on the
	// loopback approve listener), and the entire argv is Authorized against the
	// deny ruleset before this runs. The dynamic command is the deliberate, gated
	// purpose of the brain controller, mirroring the terminal/gitops raw adapters.
	cmd := exec.Command(argv[0], argv[1:]...) // nosemgrep: go.lang.security.audit.dangerous-exec-command
	out := l.logTo
	if out == nil {
		out = os.Stderr
	}
	cmd.Stdout = out
	cmd.Stderr = out
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	h := &execHandle{cmd: cmd, done: make(chan struct{})}
	go func() {
		_ = cmd.Wait()
		h.mu.Lock()
		h.exited = true
		h.mu.Unlock()
		close(h.done)
	}()
	return h, nil
}

type execHandle struct {
	cmd    *exec.Cmd
	done   chan struct{}
	mu     sync.Mutex
	exited bool
}

func (h *execHandle) Signal(sig os.Signal) error { return h.cmd.Process.Signal(sig) }
func (h *execHandle) Kill() error                { return h.cmd.Process.Kill() }
func (h *execHandle) Wait() error                { <-h.done; return nil }

func (h *execHandle) Running() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return !h.exited
}

type httpProbe struct{ client *http.Client }

// NewHTTPProbe reports readiness via GET {baseURL}/v1/models for the baseURL
// passed at probe time (so a custom-port load is probed correctly).
func NewHTTPProbe() Probe {
	return &httpProbe{client: &http.Client{Timeout: 3 * time.Second}}
}

func (p *httpProbe) Ready(ctx context.Context, baseURL string) bool {
	url := strings.TrimRight(baseURL, "/") + "/v1/models"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}
