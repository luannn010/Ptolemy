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

// Probe reports whether the brain's HTTP endpoint is serving (GET /v1/models).
type Probe interface {
	Ready(ctx context.Context) bool
}

// Status is a snapshot of the brain process.
type Status struct {
	Running bool      `json:"running"`
	Model   string    `json:"model"`
	LastUse time.Time `json:"last_use"`
}

// Manager owns the single brain child process. It is the RAW mechanism — every
// caller reaches it only through policy.GuardedBrain. All methods are
// mutex-serialized: there is at most one brain at a time.
type Manager struct {
	reg          *Registry
	launcher     Launcher
	probe        Probe
	host         string
	port         string
	defaultModel string
	readyTimeout time.Duration
	pollInterval time.Duration
	stopTimeout  time.Duration

	mu      sync.Mutex
	handle  Handle
	current string
	lastUse time.Time
}

func NewManager(reg *Registry, launcher Launcher, probe Probe, host, port, defaultModel string) *Manager {
	return &Manager{
		reg:          reg,
		launcher:     launcher,
		probe:        probe,
		host:         host,
		port:         port,
		defaultModel: defaultModel,
		readyTimeout: 90 * time.Second,
		pollInterval: 500 * time.Millisecond,
		stopTimeout:  15 * time.Second,
	}
}

func (m *Manager) argv(model Model) []string {
	argv := []string{model.Binary, "-m", model.GGUF, "--host", m.host, "--port", m.port}
	return append(argv, model.Args...)
}

// Wake ensures the named model (or the default when empty) is running and ready.
func (m *Manager) Wake(ctx context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.wakeLocked(ctx, name)
}

// EnsureAwake wakes the current model if one is set, else the default. Used by
// the /chat auto-wake hook; a no-op when the brain is already up.
func (m *Manager) EnsureAwake(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	name := m.current
	if name == "" {
		name = m.defaultModel
	}
	return m.wakeLocked(ctx, name)
}

func (m *Manager) wakeLocked(ctx context.Context, name string) error {
	if name == "" {
		name = m.defaultModel
	}
	if name == "" {
		return errors.New("no model specified and no default configured")
	}
	if m.handle != nil && m.handle.Running() && m.current == name {
		m.lastUse = time.Now() // already up — just record activity
		return nil
	}
	model, err := m.reg.Get(name)
	if err != nil {
		return err
	}
	if m.handle != nil && m.handle.Running() {
		m.stopLocked()
	}
	h, err := m.launcher.Start(m.argv(model))
	if err != nil {
		return fmt.Errorf("start brain %q: %w", name, err)
	}
	m.handle = h
	m.current = name
	if err := m.waitReady(ctx); err != nil {
		m.stopLocked()
		return err
	}
	m.lastUse = time.Now()
	return nil
}

func (m *Manager) waitReady(ctx context.Context) error {
	deadline := time.Now().Add(m.readyTimeout)
	for {
		if m.handle == nil || !m.handle.Running() {
			return errors.New("brain process exited during startup")
		}
		if m.probe.Ready(ctx) {
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

// Switch validates the target model BEFORE stopping the current one, then
// stop-then-wake. A bad model name leaves the running brain untouched.
func (m *Manager) Switch(ctx context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, err := m.reg.Get(name); err != nil {
		return err
	}
	if m.handle != nil && m.handle.Running() {
		m.stopLocked()
	}
	return m.wakeLocked(ctx, name)
}

// Stop terminates the brain (manual). Unload is the same mechanism, exposed
// separately so policy.GuardedBrain can gate the idle-path differently.
func (m *Manager) Stop(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopLocked()
	return nil
}

func (m *Manager) Unload(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopLocked()
	return nil
}

func (m *Manager) stopLocked() {
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
	m.current = ""
}

// Status returns a snapshot.
func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	return Status{
		Running: m.handle != nil && m.handle.Running(),
		Model:   m.current,
		LastUse: m.lastUse,
	}
}

// MaybeUnloadIfIdle stops the brain if it has been idle longer than ttl. Returns
// whether it unloaded. Called by the idle-TTL loop.
func (m *Manager) MaybeUnloadIfIdle(_ context.Context, ttl time.Duration) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.handle == nil || !m.handle.Running() {
		return false, nil
	}
	if time.Since(m.lastUse) < ttl {
		return false, nil
	}
	m.stopLocked()
	return true, nil
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
	cmd := exec.Command(argv[0], argv[1:]...)
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
func (h *execHandle) Kill() error                 { return h.cmd.Process.Kill() }
func (h *execHandle) Wait() error                 { <-h.done; return nil }

func (h *execHandle) Running() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return !h.exited
}

type httpProbe struct {
	url    string
	client *http.Client
}

// NewHTTPProbe reports readiness via GET {baseURL}/v1/models (same liveness path
// the health checker uses).
func NewHTTPProbe(baseURL string) Probe {
	return &httpProbe{
		url:    strings.TrimRight(baseURL, "/") + "/v1/models",
		client: &http.Client{Timeout: 3 * time.Second},
	}
}

func (p *httpProbe) Ready(ctx context.Context) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.url, nil)
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
