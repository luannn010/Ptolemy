# Brain Controller Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the named-preset brain skill with a free-form llama.cpp controller — disk-scan model discovery, caller-supplied launch specs (binary/gguf/host/port/flags), and explicit hibernate→resume — all behind `policy.GuardedBrain`.

**Architecture:** Extend `internal/brain` in place. A `Spec` replaces the `Model` registry as the launch unit. The `Manager` owns one process, stores the approved spec (persists across hibernate, cleared by stop), and probes the spec's own host:port for readiness. `policy.GuardedBrain` gates every verb; only `load` (which carries a full spec) is `ask`/OOB, the rest auto-`allow`. The loopback HTTP control plane exposes the verbs.

**Tech Stack:** Go 1.25, chi router, `database/sql`+sqlite (policy audit), zerolog. Tests use fakes (fake `Launcher`, stub `Probe`, stub `RawBrain`, in-mem sqlite) — no real process/DB/brain.

**Spec:** [docs/superpowers/specs/2026-06-14-brain-controller-design.md](../specs/2026-06-14-brain-controller-design.md)

**Security invariant:** A custom spec enters the Manager only via the `ask` `load` path. `resume`/`wake`/`hibernate`/`status`/`models` carry no spec → auto-`allow`; `stop` carries no spec but stays `ask` as a manual-teardown gate. The engine matches the full argv, so `deny` rules cover spec fields for free.

---

## File structure

| File | Responsibility |
|---|---|
| `internal/brain/spec.go` (new) | `Spec` value type: default-fill, validate, `Argv()`, `BaseURL()`. |
| `internal/brain/discovery.go` (new) | `DiscoverModels(root)` — scan for `*.gguf`. |
| `internal/brain/manager.go` (rewrite) | Single-process owner: `Load/Resume/EnsureAwake/Hibernate/Stop/Status/ListModels/ResolveSpec`; spec persists across hibernate. |
| `internal/brain/models.go` (delete) | Old named-preset registry — removed. |
| `internal/policy/guard_brain.go` (rewrite) | `RawBrain` + `GuardedBrain` for the new verbs; `load`=ask, rest per table. |
| `internal/policy/rules.go` (edit) | Add `allow-brain-models/resume/hibernate`, `ask-brain-load`; remove `ask-brain-switch`, `allow-brain-autounload`. |
| `internal/httpapi/brain.go` (rewrite) | Loopback routes: `GET /brain/{models,status}`, `POST /brain/{load,resume,hibernate,stop}`. |
| `internal/config/config.go` (edit) | Add `BrainModelsDir`, `BrainLlamaBin`; remove `BrainModelsPath`, `BrainDefaultModel`. |
| `cmd/workerd/brain.go` (edit) | `buildBrainDeps`: drop registry; build Manager from dir+bin; idle loop → `Hibernate`. |
| `cmd/workerd/main.go` (edit) | No structural change (control router already mounted); verify it compiles against new deps. |
| `brain-models.example.json` (delete) | Replaced by a `BRAIN_MODELS_DIR` note. |
| `docs/Architecture.md`, `.env.example`, `.ptolemy/policy.json` (edit) | Brain note + env block + host-local rules. |

---

## Task 1: Config knobs

**Files:**
- Modify: `internal/config/config.go`
- Test: `internal/config/config_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/config/config_test.go`:

```go
func TestLoad_BrainControllerKnobs(t *testing.T) {
	t.Setenv("BRAIN_MODELS_DIR", "/home/u/models")
	t.Setenv("BRAIN_LLAMA_BIN", "/opt/llama/llama-server")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BrainModelsDir != "/home/u/models" {
		t.Fatalf("BrainModelsDir = %q", cfg.BrainModelsDir)
	}
	if cfg.BrainLlamaBin != "/opt/llama/llama-server" {
		t.Fatalf("BrainLlamaBin = %q", cfg.BrainLlamaBin)
	}
}

func TestLoad_BrainControllerDefaults(t *testing.T) {
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BrainModelsDir != "" || cfg.BrainLlamaBin != "" {
		t.Fatalf("expected empty brain dir/bin defaults, got %q / %q", cfg.BrainModelsDir, cfg.BrainLlamaBin)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestLoad_BrainController ./internal/config/`
Expected: FAIL — `cfg.BrainModelsDir`/`BrainLlamaBin` undefined (compile error).

- [ ] **Step 3: Edit config.go**

In the `Config` struct, replace the two preset fields:

```go
	// Brain lifecycle skill (workerd-managed llama.cpp). Off by default.
	BrainControlEnabled bool
	BrainAutoWake       bool
	BrainIdleTTL        time.Duration
	BrainControlPort    string
	BrainModelsDir      string // BRAIN_MODELS_DIR: scanned for *.gguf by GET /brain/models
	BrainLlamaBin       string // BRAIN_LLAMA_BIN: default binary for loads that omit it
```

(Delete the old `BrainModelsPath` and `BrainDefaultModel` fields.)

In `Load()`, replace the two preset assignments:

```go
	cfg.BrainControlPort = getEnv("BRAIN_CONTROL_PORT", "8089")
	cfg.BrainModelsDir = getEnv("BRAIN_MODELS_DIR", "")
	cfg.BrainLlamaBin = getEnv("BRAIN_LLAMA_BIN", "")
```

(Delete the old `cfg.BrainModelsPath = getEnv("BRAIN_MODELS", "")` and `cfg.BrainDefaultModel = getEnv("BRAIN_DEFAULT_MODEL", "")` lines.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -run TestLoad_BrainController ./internal/config/`
Expected: PASS. (The build will still break in `cmd/workerd` until Task 8 — that's fine; this package compiles.)

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat(config): brain controller knobs (BRAIN_MODELS_DIR/BRAIN_LLAMA_BIN), drop preset env"
```

---

## Task 2: brain.Spec

**Files:**
- Create: `internal/brain/spec.go`
- Test: `internal/brain/spec_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/brain/spec_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestSpec ./internal/brain/`
Expected: FAIL — `Spec` undefined.

- [ ] **Step 3: Create spec.go**

```go
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -run TestSpec ./internal/brain/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/brain/spec.go internal/brain/spec_test.go
git commit -m "feat(brain): Spec — free-form llama.cpp launch (argv/defaults/validate/baseurl)"
```

---

## Task 3: brain.DiscoverModels

**Files:**
- Create: `internal/brain/discovery.go`
- Test: `internal/brain/discovery_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/brain/discovery_test.go`:

```go
package brain

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverModels_FindsGGUFRecursivelyAndIgnoresOthers(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.gguf"), "x")
	mustWrite(t, filepath.Join(root, "sub", "b.GGUF"), "yy")
	mustWrite(t, filepath.Join(root, "notes.txt"), "ignore")

	got, err := DiscoverModels(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 gguf, got %d (%+v)", len(got), got)
	}
	if got[0].Name != "a.gguf" || got[0].Size != 1 {
		t.Fatalf("unexpected first model: %+v", got[0])
	}
}

func TestDiscoverModels_MissingOrEmptyRoot(t *testing.T) {
	got, err := DiscoverModels(filepath.Join(t.TempDir(), "nope"))
	if err != nil || len(got) != 0 {
		t.Fatalf("missing root: got %v err %v", got, err)
	}
	if got, err := DiscoverModels(""); err != nil || len(got) != 0 {
		t.Fatalf("empty root: got %v err %v", got, err)
	}
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestDiscoverModels ./internal/brain/`
Expected: FAIL — `DiscoverModels` undefined.

- [ ] **Step 3: Create discovery.go**

```go
package brain

import (
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

// DiscoveredModel is a gguf file found under the models root.
type DiscoveredModel struct {
	Name string `json:"name"` // base filename
	Path string `json:"path"` // absolute path
	Size int64  `json:"size"` // bytes
}

// DiscoverModels walks root and returns every *.gguf file (case-insensitive),
// sorted by path. A missing/empty root yields an empty list and no error —
// discovery is best-effort and never fails the caller.
func DiscoverModels(root string) ([]DiscoveredModel, error) {
	out := []DiscoveredModel{}
	root = strings.TrimSpace(root)
	if root == "" {
		return out, nil
	}
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil // skip unreadable entries; never abort the walk
		}
		if !strings.EqualFold(filepath.Ext(d.Name()), ".gguf") {
			return nil
		}
		var size int64
		if info, ierr := d.Info(); ierr == nil {
			size = info.Size()
		}
		out = append(out, DiscoveredModel{Name: d.Name(), Path: path, Size: size})
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -run TestDiscoverModels ./internal/brain/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/brain/discovery.go internal/brain/discovery_test.go
git commit -m "feat(brain): DiscoverModels — scan BRAIN_MODELS_DIR for *.gguf"
```

---

## Task 4: Manager rewrite (spec-based lifecycle)

**Files:**
- Modify: `internal/brain/manager.go` (rewrite the type + methods; keep `Launcher`/`Handle`/`execLauncher`/`RunIdleLoop`)
- Delete: `internal/brain/models.go`, `internal/brain/models_test.go`
- Modify: `internal/brain/manager_test.go` (rewrite around fakes + Spec)
- Test: `internal/brain/manager_test.go`

- [ ] **Step 1: Delete the registry**

```bash
git rm internal/brain/models.go internal/brain/models_test.go
```

- [ ] **Step 2: Write the failing test** (replace the body of `internal/brain/manager_test.go`)

```go
package brain

import (
	"context"
	"errors"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

// fakeLauncher hands out fakeHandles and counts starts.
type fakeLauncher struct {
	starts   atomic.Int32
	lastArgv []string
	failNext bool
}

func (l *fakeLauncher) Start(argv []string) (Handle, error) {
	if l.failNext {
		return nil, errors.New("boom")
	}
	l.starts.Add(1)
	l.lastArgv = argv
	return &fakeHandle{running: true}, nil
}

type fakeHandle struct{ running bool }

func (h *fakeHandle) Signal(os.Signal) error { return nil }
func (h *fakeHandle) Kill() error            { h.running = false; return nil }
func (h *fakeHandle) Wait() error            { h.running = false; return nil }
func (h *fakeHandle) Running() bool          { return h.running }

type stubProbe struct{ ready bool }

func (p *stubProbe) Ready(context.Context, string) bool { return p.ready }

func newTestManager(l Launcher, p Probe) *Manager {
	m := NewManager(l, p, "/bin/llama-server", "0.0.0.0", "9000", "")
	m.readyTimeout = 200 * time.Millisecond
	m.pollInterval = 5 * time.Millisecond
	m.stopTimeout = 50 * time.Millisecond
	return m
}

func TestManager_LoadLaunchesAndStoresSpec(t *testing.T) {
	l := &fakeLauncher{}
	m := newTestManager(l, &stubProbe{ready: true})
	if err := m.Load(context.Background(), Spec{GGUF: "/m/x.gguf"}); err != nil {
		t.Fatal(err)
	}
	if l.starts.Load() != 1 {
		t.Fatalf("expected 1 start, got %d", l.starts.Load())
	}
	st := m.Status()
	if !st.Running || st.GGUF != "/m/x.gguf" || st.Binary != "/bin/llama-server" {
		t.Fatalf("status after load: %+v", st)
	}
	// argv got defaults filled
	if l.lastArgv[0] != "/bin/llama-server" || l.lastArgv[2] != "/m/x.gguf" {
		t.Fatalf("argv: %v", l.lastArgv)
	}
}

func TestManager_LoadValidationRejectsEmptyGGUF(t *testing.T) {
	m := newTestManager(&fakeLauncher{}, &stubProbe{ready: true})
	if err := m.Load(context.Background(), Spec{}); err == nil {
		t.Fatal("empty gguf must error")
	}
}

func TestManager_HibernateKeepsSpec_ResumeRelaunches(t *testing.T) {
	l := &fakeLauncher{}
	m := newTestManager(l, &stubProbe{ready: true})
	_ = m.Load(context.Background(), Spec{GGUF: "/m/x.gguf"})

	if err := m.Hibernate(context.Background()); err != nil {
		t.Fatal(err)
	}
	st := m.Status()
	if st.Running || !st.Hibernated || st.GGUF != "/m/x.gguf" {
		t.Fatalf("after hibernate: %+v", st)
	}
	if err := m.Resume(context.Background()); err != nil {
		t.Fatal(err)
	}
	if l.starts.Load() != 2 {
		t.Fatalf("resume must relaunch; starts=%d", l.starts.Load())
	}
	if !m.Status().Running {
		t.Fatal("running after resume")
	}
}

func TestManager_StopClearsSpec(t *testing.T) {
	m := newTestManager(&fakeLauncher{}, &stubProbe{ready: true})
	_ = m.Load(context.Background(), Spec{GGUF: "/m/x.gguf"})
	_ = m.Stop(context.Background())
	st := m.Status()
	if st.Running || st.Hibernated || st.GGUF != "" {
		t.Fatalf("stop must clear spec: %+v", st)
	}
	if err := m.Resume(context.Background()); !errors.Is(err, ErrNoModelLoaded) {
		t.Fatalf("resume after stop must be ErrNoModelLoaded, got %v", err)
	}
}

func TestManager_EnsureAwake(t *testing.T) {
	l := &fakeLauncher{}
	m := newTestManager(l, &stubProbe{ready: true})
	// cold: no spec
	if err := m.EnsureAwake(context.Background()); !errors.Is(err, ErrNoModelLoaded) {
		t.Fatalf("cold EnsureAwake must be ErrNoModelLoaded, got %v", err)
	}
	_ = m.Load(context.Background(), Spec{GGUF: "/m/x.gguf"})
	// already running: no extra start
	_ = m.EnsureAwake(context.Background())
	if l.starts.Load() != 1 {
		t.Fatalf("EnsureAwake while running must not relaunch; starts=%d", l.starts.Load())
	}
	// hibernated: resumes
	_ = m.Hibernate(context.Background())
	_ = m.EnsureAwake(context.Background())
	if l.starts.Load() != 2 {
		t.Fatalf("EnsureAwake while hibernated must resume; starts=%d", l.starts.Load())
	}
}

func TestManager_ReadinessTimeoutStopsProcess(t *testing.T) {
	l := &fakeLauncher{}
	m := newTestManager(l, &stubProbe{ready: false}) // never ready
	err := m.Load(context.Background(), Spec{GGUF: "/m/x.gguf"})
	if err == nil {
		t.Fatal("expected readiness timeout error")
	}
	if m.Status().Running {
		t.Fatal("failed load must leave nothing running")
	}
}

func TestManager_ResolveSpecFillsDefaults(t *testing.T) {
	m := newTestManager(&fakeLauncher{}, &stubProbe{ready: true})
	got := m.ResolveSpec(Spec{GGUF: "/g"})
	if got.Binary != "/bin/llama-server" || got.Host != "0.0.0.0" || got.Port != "9000" {
		t.Fatalf("ResolveSpec: %+v", got)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test -run TestManager ./internal/brain/`
Expected: FAIL — new `NewManager` signature / `ErrNoModelLoaded` / `Probe.Ready(ctx,string)` undefined.

- [ ] **Step 4: Rewrite manager.go**

Replace the `Status` struct, the `Probe` interface, the `Manager` type, `NewManager`, and all lifecycle methods with the following. **Keep** `Launcher`, `Handle`, `RunIdleLoop`, `execLauncher`/`execHandle` unchanged. **Change** `httpProbe` to the new `Ready(ctx, baseURL)` signature (shown).

```go
// Probe reports whether a brain HTTP endpoint is serving (GET {baseURL}/v1/models).
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
// On failure it tears down the partial process but preserves the stored spec
// (the caller decides). mu must be held.
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
```

Then update `httpProbe.Ready` and `NewHTTPProbe`:

```go
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
```

(Keep `RunIdleLoop`, `execLauncher`, `execHandle` exactly as they are. `RunIdleLoop`'s `status func() Status` and `unload func(context.Context) error` signatures are unchanged — buildBrainDeps will pass `Hibernate` as the unload func in Task 8.)

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/brain/`
Expected: PASS (spec, discovery, manager).

- [ ] **Step 6: Commit**

```bash
git add internal/brain/
git commit -m "feat(brain): Manager — free-form spec lifecycle (load/resume/hibernate/stop), drop registry"
```

---

## Task 5: GuardedBrain rewrite

**Files:**
- Modify: `internal/policy/guard_brain.go`
- Test: `internal/policy/guard_brain_test.go`

- [ ] **Step 1: Write the failing test** (rewrite `guard_brain_test.go`)

Use the existing test harness in this package for `Engine`/`Approvals`/in-mem sqlite (mirror the current `guard_brain_test.go` setup helpers; reuse whatever `newTestGuardDB`/engine builders the file already had). Key cases:

```go
package policy

import (
	"context"
	"errors"
	"testing"

	"github.com/luannn010/ptolemy/internal/brain"
)

// stubRawBrain records calls.
type stubRawBrain struct {
	loaded   *brain.Spec
	resumed  int
	ensured  int
	hibern   int
	stopped  int
	listed   int
}

func (s *stubRawBrain) ResolveSpec(sp brain.Spec) brain.Spec { return sp }
func (s *stubRawBrain) Load(_ context.Context, sp brain.Spec) error { s.loaded = &sp; return nil }
func (s *stubRawBrain) Resume(context.Context) error      { s.resumed++; return nil }
func (s *stubRawBrain) EnsureAwake(context.Context) error { s.ensured++; return nil }
func (s *stubRawBrain) Hibernate(context.Context) error   { s.hibern++; return nil }
func (s *stubRawBrain) Stop(context.Context) error        { s.stopped++; return nil }
func (s *stubRawBrain) Status() brain.Status              { return brain.Status{} }
func (s *stubRawBrain) ListModels() ([]brain.DiscoveredModel, error) { s.listed++; return nil, nil }

func TestGuardedBrain_LoadIsAsk_RawNotCalled(t *testing.T) {
	eng, appr, db := newBrainTestDeps(t) // builds Engine(DefaultRuleset), Approvals, in-mem sqlite + brain-system session
	raw := &stubRawBrain{}
	g := NewGuardedBrain(eng, appr, raw, db)

	err := g.Load(context.Background(), "brain-system", brain.Spec{Binary: "/b", GGUF: "/g"}, CallOpts{})
	var needs ErrNeedsConfirmation
	if !errors.As(err, &needs) {
		t.Fatalf("load must need confirmation, got %v", err)
	}
	if raw.loaded != nil {
		t.Fatal("raw.Load must NOT run before approval")
	}

	// approve + retry with the pending id as confirm token
	appr.Approve(needs.PendingID)
	if err := g.Load(context.Background(), "brain-system", brain.Spec{Binary: "/b", GGUF: "/g"}, CallOpts{ConfirmToken: needs.PendingID}); err != nil {
		t.Fatalf("approved retry: %v", err)
	}
	if raw.loaded == nil {
		t.Fatal("raw.Load must run after approval")
	}
}

func TestGuardedBrain_ResumeHibernateStatusModelsAreAllow(t *testing.T) {
	eng, appr, db := newBrainTestDeps(t)
	raw := &stubRawBrain{}
	g := NewGuardedBrain(eng, appr, raw, db)
	ctx := context.Background()
	if err := g.Resume(ctx, "brain-system", CallOpts{}); err != nil {
		t.Fatalf("resume: %v", err)
	}
	if err := g.Hibernate(ctx, "brain-system", CallOpts{}); err != nil {
		t.Fatalf("hibernate: %v", err)
	}
	if _, err := g.ListModels(ctx, "brain-system", CallOpts{}); err != nil {
		t.Fatalf("models: %v", err)
	}
	if raw.resumed != 1 || raw.hibern != 1 || raw.listed != 1 {
		t.Fatalf("allow verbs must call raw: %+v", raw)
	}
}

func TestGuardedBrain_StopIsAsk(t *testing.T) {
	eng, appr, db := newBrainTestDeps(t)
	raw := &stubRawBrain{}
	g := NewGuardedBrain(eng, appr, raw, db)
	err := g.Stop(context.Background(), "brain-system", CallOpts{})
	var needs ErrNeedsConfirmation
	if !errors.As(err, &needs) {
		t.Fatalf("stop must need confirmation, got %v", err)
	}
	if raw.stopped != 0 {
		t.Fatal("raw.Stop must not run before approval")
	}
}

func TestGuardedBrain_DenyTokenInSpecField(t *testing.T) {
	eng, appr, db := newBrainTestDeps(t)
	g := NewGuardedBrain(eng, appr, &stubRawBrain{}, db)
	// "rm -rf" smuggled as a flag -> deny rule trips on the argv.
	err := g.Load(context.Background(), "brain-system", brain.Spec{Binary: "/b", GGUF: "/g", Args: []string{"--x", "rm -rf /"}}, CallOpts{})
	var denied ErrDenied
	if !errors.As(err, &denied) {
		t.Fatalf("destructive token in a spec field must be denied, got %v", err)
	}
}
```

> If the current `guard_brain_test.go` already defines a deps helper, reuse it and rename references to `newBrainTestDeps`. Otherwise add a small helper that opens an in-mem sqlite with the four-table schema and inserts the `brain-system` session row (copy the pattern from the existing brain guard test you are replacing).

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestGuardedBrain ./internal/policy/`
Expected: FAIL — new `GuardedBrain` method set / `RawBrain` shape mismatch.

- [ ] **Step 3: Rewrite guard_brain.go**

```go
package policy

import (
	"context"
	"database/sql"

	"github.com/luannn010/ptolemy/internal/brain"
	"github.com/luannn010/ptolemy/internal/domain"
)

// RawBrain is the unguarded brain lifecycle mechanism (internal/brain.Manager).
type RawBrain interface {
	ResolveSpec(spec brain.Spec) brain.Spec
	Load(ctx context.Context, spec brain.Spec) error
	Resume(ctx context.Context) error
	EnsureAwake(ctx context.Context) error
	Hibernate(ctx context.Context) error
	Stop(ctx context.Context) error
	Status() brain.Status
	ListModels() ([]brain.DiscoveredModel, error)
}

// GuardedBrain gates every brain action through the policy engine and audits it.
// load carries the full spec (ask/OOB); resume/wake/hibernate/status/models carry
// no spec (allow); stop carries no spec but is kept ask (manual teardown).
type GuardedBrain struct {
	core guardCore
	raw  RawBrain
}

func NewGuardedBrain(engine *Engine, approvals *Approvals, raw RawBrain, db *sql.DB) *GuardedBrain {
	return &GuardedBrain{core: guardCore{engine: engine, approvals: approvals, db: db}, raw: raw}
}

func (g *GuardedBrain) intent(action string, args ...string) domain.Intent {
	return domain.Intent{Kind: "brain." + action, Program: "brain", Args: append([]string{action}, args...)}
}

// Load gates on "brain load <argv…>" (ask/OOB). The resolved argv is in the
// intent so the hash and the deny rules cover the whole command.
func (g *GuardedBrain) Load(ctx context.Context, sessionID string, spec brain.Spec, opts CallOpts) error {
	resolved := g.raw.ResolveSpec(spec)
	intent := g.intent("load", resolved.Argv()...)
	intent.Targets = []string{resolved.GGUF}
	if err := g.core.gate(ctx, sessionID, intent, opts); err != nil {
		return err
	}
	return g.raw.Load(ctx, resolved)
}

func (g *GuardedBrain) Resume(ctx context.Context, sessionID string, opts CallOpts) error {
	if err := g.core.gate(ctx, sessionID, g.intent("resume"), opts); err != nil {
		return err
	}
	return g.raw.Resume(ctx)
}

func (g *GuardedBrain) EnsureAwake(ctx context.Context, sessionID string, opts CallOpts) error {
	if err := g.core.gate(ctx, sessionID, g.intent("wake"), opts); err != nil {
		return err
	}
	return g.raw.EnsureAwake(ctx)
}

func (g *GuardedBrain) Hibernate(ctx context.Context, sessionID string, opts CallOpts) error {
	if err := g.core.gate(ctx, sessionID, g.intent("hibernate"), opts); err != nil {
		return err
	}
	return g.raw.Hibernate(ctx)
}

func (g *GuardedBrain) Stop(ctx context.Context, sessionID string, opts CallOpts) error {
	if err := g.core.gate(ctx, sessionID, g.intent("stop"), opts); err != nil {
		return err
	}
	return g.raw.Stop(ctx)
}

func (g *GuardedBrain) Status(ctx context.Context, sessionID string, opts CallOpts) (brain.Status, error) {
	if err := g.core.gate(ctx, sessionID, g.intent("status"), opts); err != nil {
		return brain.Status{}, err
	}
	return g.raw.Status(), nil
}

func (g *GuardedBrain) ListModels(ctx context.Context, sessionID string, opts CallOpts) ([]brain.DiscoveredModel, error) {
	if err := g.core.gate(ctx, sessionID, g.intent("models"), opts); err != nil {
		return nil, err
	}
	return g.raw.ListModels()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -run TestGuardedBrain ./internal/policy/`
Expected: PASS. (Full `./internal/policy/` will fail until Task 6 updates `rules_test.go`.)

- [ ] **Step 5: Commit**

```bash
git add internal/policy/guard_brain.go internal/policy/guard_brain_test.go
git commit -m "feat(policy): GuardedBrain — load(ask)/resume/hibernate/stop/status/models verbs"
```

---

## Task 6: Policy rules

**Files:**
- Modify: `internal/policy/rules.go`
- Modify: `internal/policy/rules_test.go`

- [ ] **Step 1: Update the failing test**

In `internal/policy/rules_test.go`, replace the brain expectations in `TestDefaultRuleset_HasBrainRules`:

```go
	want := map[string]domain.Effect{
		"allow-brain-wake":      domain.EffectAllow,
		"allow-brain-status":    domain.EffectAllow,
		"allow-brain-models":    domain.EffectAllow,
		"allow-brain-resume":    domain.EffectAllow,
		"allow-brain-hibernate": domain.EffectAllow,
		"ask-brain-load":        domain.EffectAsk,
		"ask-brain-stop":        domain.EffectAsk,
	}
	// removed rules must be gone
	for _, gone := range []string{"ask-brain-switch", "allow-brain-autounload"} {
		if _, ok := got[gone]; ok {
			t.Fatalf("rule %q should have been removed", gone)
		}
	}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestDefaultRuleset ./internal/policy/`
Expected: FAIL — missing `allow-brain-models`/`resume`/`hibernate`/`ask-brain-load`.

- [ ] **Step 3: Edit rules.go**

In `DefaultRuleset()`, replace the brain block (keep `allow-brain-wake`, `allow-brain-status`, `ask-brain-stop`):

```go
			// Brain controller: automatic actions allow (non-blocking, audited);
			// custom load + manual stop ask/OOB.
			{ID: "allow-brain-wake", Contains: "brain wake", Effect: domain.EffectAllow, Reason: "auto-wake / resume of the loaded brain spec"},
			{ID: "allow-brain-status", Contains: "brain status", Effect: domain.EffectAllow, Reason: "brain status is a read"},
			{ID: "allow-brain-models", Contains: "brain models", Effect: domain.EffectAllow, Reason: "listing available models is a read"},
			{ID: "allow-brain-resume", Contains: "brain resume", Effect: domain.EffectAllow, Reason: "resume the already-approved brain spec"},
			{ID: "allow-brain-hibernate", Contains: "brain hibernate", Effect: domain.EffectAllow, Reason: "hibernate frees VRAM, spec retained"},
			{ID: "ask-brain-load", Contains: "brain load", Effect: domain.EffectAsk, Channel: domain.ChannelOOB, Reason: "custom brain launch requires confirmation"},
			{ID: "ask-brain-stop", Contains: "brain stop", Effect: domain.EffectAsk, Channel: domain.ChannelOOB, Reason: "manual brain stop requires confirmation"},
```

(Delete the old `allow-brain-autounload` and `ask-brain-switch` lines.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/policy/`
Expected: PASS (guard_brain + rules + engine bypass suite).

- [ ] **Step 5: Commit**

```bash
git add internal/policy/rules.go internal/policy/rules_test.go
git commit -m "feat(policy): brain controller rules (models/resume/hibernate allow, load ask)"
```

---

## Task 7: HTTP control plane

**Files:**
- Modify: `internal/httpapi/brain.go`
- Test: `internal/httpapi/brain_test.go`

- [ ] **Step 1: Write the failing test** (rewrite `brain_test.go` around a stub `BrainController`)

```go
package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/luannn010/ptolemy/internal/brain"
	"github.com/luannn010/ptolemy/internal/policy"
)

type stubBrain struct {
	loadErr   error
	resumeErr error
	loaded    *brain.Spec
}

func (s *stubBrain) Load(_ context.Context, _ string, sp brain.Spec, _ policy.CallOpts) error {
	s.loaded = &sp
	return s.loadErr
}
func (s *stubBrain) Resume(context.Context, string, policy.CallOpts) error    { return s.resumeErr }
func (s *stubBrain) Hibernate(context.Context, string, policy.CallOpts) error { return nil }
func (s *stubBrain) Stop(context.Context, string, policy.CallOpts) error      { return nil }
func (s *stubBrain) Status(context.Context, string, policy.CallOpts) (brain.Status, error) {
	return brain.Status{Running: true, GGUF: "/m.gguf"}, nil
}
func (s *stubBrain) ListModels(context.Context, string, policy.CallOpts) ([]brain.DiscoveredModel, error) {
	return []brain.DiscoveredModel{{Name: "m.gguf", Path: "/m.gguf"}}, nil
}

func brainSrv(t *testing.T, b BrainController) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(NewBrainControlRouter(BrainDeps{Brain: b}))
	t.Cleanup(srv.Close)
	return srv
}

func TestBrain_Models_OK(t *testing.T) {
	srv := brainSrv(t, &stubBrain{})
	resp, err := http.Get(srv.URL + "/brain/models")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
}

func TestBrain_Load_NeedsGGUF_400(t *testing.T) {
	srv := brainSrv(t, &stubBrain{})
	resp, _ := http.Post(srv.URL+"/brain/load", "application/json", strings.NewReader(`{"binary":"/b"}`))
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("missing gguf must be 400, got %d", resp.StatusCode)
	}
}

func TestBrain_Load_NeedsConfirmation_202(t *testing.T) {
	srv := brainSrv(t, &stubBrain{loadErr: policy.ErrNeedsConfirmation{PendingID: "p1", Reason: "r"}})
	resp, _ := http.Post(srv.URL+"/brain/load", "application/json", strings.NewReader(`{"gguf":"/m.gguf"}`))
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("ask must be 202, got %d", resp.StatusCode)
	}
}

func TestBrain_Resume_NoModel_409(t *testing.T) {
	srv := brainSrv(t, &stubBrain{resumeErr: brain.ErrNoModelLoaded})
	resp, _ := http.Post(srv.URL+"/brain/resume", "application/json", strings.NewReader(`{}`))
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("no-model resume must be 409, got %d", resp.StatusCode)
	}
}

func TestBrain_Denied_403(t *testing.T) {
	srv := brainSrv(t, &stubBrain{loadErr: policy.ErrDenied{RuleID: "deny-x", Reason: "nope"}})
	resp, _ := http.Post(srv.URL+"/brain/load", "application/json", strings.NewReader(`{"gguf":"/m.gguf"}`))
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("denied must be 403, got %d", resp.StatusCode)
	}
}
```

> The loopback-only test from the current `brain_test.go` (non-loopback RemoteAddr → 403) should be kept — copy it forward unchanged; `loopbackOnly` is unchanged.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestBrain ./internal/httpapi/`
Expected: FAIL — `BrainController` shape / routes changed.

- [ ] **Step 3: Rewrite brain.go**

Replace the `BrainController` interface and routes (keep `loopbackOnly`, `decodeOptionalJSON`, `BrainSystemSession`; extend `writeBrainErr` with the 409 case):

```go
// BrainController is the slice of policy.GuardedBrain the control plane needs.
type BrainController interface {
	Load(ctx context.Context, sessionID string, spec brain.Spec, opts policy.CallOpts) error
	Resume(ctx context.Context, sessionID string, opts policy.CallOpts) error
	Hibernate(ctx context.Context, sessionID string, opts policy.CallOpts) error
	Stop(ctx context.Context, sessionID string, opts policy.CallOpts) error
	Status(ctx context.Context, sessionID string, opts policy.CallOpts) (brain.Status, error)
	ListModels(ctx context.Context, sessionID string, opts policy.CallOpts) ([]brain.DiscoveredModel, error)
}

type BrainDeps struct{ Brain BrainController }

func NewBrainControlRouter(deps BrainDeps) http.Handler {
	r := chi.NewRouter()
	r.Use(loopbackOnly)

	r.Get("/brain/models", func(w http.ResponseWriter, req *http.Request) {
		models, err := deps.Brain.ListModels(req.Context(), BrainSystemSession, policy.CallOpts{})
		if writeBrainErr(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"models": models})
	})

	r.Get("/brain/status", func(w http.ResponseWriter, req *http.Request) {
		st, err := deps.Brain.Status(req.Context(), BrainSystemSession, policy.CallOpts{})
		if writeBrainErr(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, st)
	})

	r.Post("/brain/load", func(w http.ResponseWriter, req *http.Request) {
		var body struct {
			Binary       string   `json:"binary"`
			GGUF         string   `json:"gguf"`
			Host         string   `json:"host"`
			Port         string   `json:"port"`
			Args         []string `json:"args"`
			ConfirmToken string   `json:"confirm_token"`
		}
		if !decodeOptionalJSON(w, req, &body) {
			return
		}
		if strings.TrimSpace(body.GGUF) == "" {
			writeJSON(w, http.StatusBadRequest, apitypes.ErrorResponse{Error: "gguf is required"})
			return
		}
		spec := brain.Spec{Binary: body.Binary, GGUF: body.GGUF, Host: body.Host, Port: body.Port, Args: body.Args}
		err := deps.Brain.Load(req.Context(), BrainSystemSession, spec, policy.CallOpts{ConfirmToken: body.ConfirmToken})
		if writeBrainErr(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	r.Post("/brain/resume", func(w http.ResponseWriter, req *http.Request) {
		var body struct {
			ConfirmToken string `json:"confirm_token"`
		}
		if !decodeOptionalJSON(w, req, &body) {
			return
		}
		err := deps.Brain.Resume(req.Context(), BrainSystemSession, policy.CallOpts{ConfirmToken: body.ConfirmToken})
		if writeBrainErr(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	r.Post("/brain/hibernate", func(w http.ResponseWriter, req *http.Request) {
		err := deps.Brain.Hibernate(req.Context(), BrainSystemSession, policy.CallOpts{})
		if writeBrainErr(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	r.Post("/brain/stop", func(w http.ResponseWriter, req *http.Request) {
		var body struct {
			ConfirmToken string `json:"confirm_token"`
		}
		if !decodeOptionalJSON(w, req, &body) {
			return
		}
		err := deps.Brain.Stop(req.Context(), BrainSystemSession, policy.CallOpts{ConfirmToken: body.ConfirmToken})
		if writeBrainErr(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	return r
}
```

Extend `writeBrainErr` — add the 409 case before the final fallthrough:

```go
	if errors.Is(err, brain.ErrNoModelLoaded) {
		writeJSON(w, http.StatusConflict, apitypes.ErrorResponse{Error: err.Error()})
		return true
	}
```

(Ensure `brain` is imported in `brain.go`.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/httpapi/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/httpapi/brain.go internal/httpapi/brain_test.go
git commit -m "feat(httpapi): brain control plane — models/status/load/resume/hibernate/stop"
```

---

## Task 8: Wire buildBrainDeps

**Files:**
- Modify: `cmd/workerd/brain.go`
- Test: `cmd/workerd/brain_test.go`

- [ ] **Step 1: Update the failing test**

In `cmd/workerd/brain_test.go`, keep the `disabled when BRAIN_CONTROL_ENABLED unset` test. Update/confirm the enabled-path test no longer references the registry; assert `buildBrainDeps` returns `ok=true` with `BRAIN_CONTROL_ENABLED=true` and an in-mem db, and that the `brain-system` session row is ensured. (Mirror the existing test's db setup; drop any `BRAIN_MODELS` file fixture.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/workerd/`
Expected: FAIL — references to removed `LoadRegistry`/`BrainModelsPath`/`BrainDefaultModel`/`NewManager` old signature.

- [ ] **Step 3: Edit brain.go**

Replace the registry/manager construction in `buildBrainDeps`:

```go
	if !cfg.BrainControlEnabled {
		log.Info().Msg("brain skill disabled (set BRAIN_CONTROL_ENABLED=true)")
		return brainDeps{}, nil, false
	}
	if err := ensureBrainSession(ctx, db); err != nil {
		log.Warn().Err(err).Msg("brain skill disabled: could not ensure system session")
		return brainDeps{}, nil, false
	}

	host, port := brainHostPort(cfg.BrainBaseURL)
	mgr := brain.NewManager(brain.NewExecLauncher(nil), brain.NewHTTPProbe(),
		cfg.BrainLlamaBin, host, port, cfg.BrainModelsDir)
	gb := policy.NewGuardedBrain(engine, approvals, mgr, db)

	// Idle-TTL: read status (ungated), hibernate (gated) only when idle.
	idleCtx, cancelIdle := context.WithCancel(ctx)
	idleDone := make(chan struct{})
	go func() {
		defer close(idleDone)
		brain.RunIdleLoop(idleCtx, idleCheckInterval, cfg.BrainIdleTTL, mgr.Status, func(c context.Context) error {
			return gb.Hibernate(c, httpapi.BrainSystemSession, policy.CallOpts{})
		})
	}()

	cleanup = func() {
		cancelIdle()
		select {
		case <-idleDone:
		case <-time.After(5 * time.Second):
			log.Error().Msg("brain idle loop did not stop within 5s")
		}
		_ = mgr.Stop(context.Background())
	}

	deps = brainDeps{guarded: gb}
	if cfg.BrainAutoWake {
		deps.waker = &brainWaker{gb: gb}
	}
	log.Info().Str("models_dir", cfg.BrainModelsDir).Str("default_bin", cfg.BrainLlamaBin).
		Bool("auto_wake", cfg.BrainAutoWake).Msg("brain skill enabled")
	return deps, cleanup, true
```

(Delete the old `cfg.BrainModelsPath == ""` guard and the `brain.LoadRegistry` block. `brainWaker.EnsureAwake` and `ensureBrainSession` and `brainHostPort` are unchanged.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/workerd/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/workerd/brain.go cmd/workerd/brain_test.go
git commit -m "feat(workerd): buildBrainDeps — spec-based Manager, idle loop hibernates"
```

---

## Task 9: main wiring, full build, docs, env, host policy

**Files:**
- Verify: `cmd/workerd/main.go` (control router already mounted; the `brainDeps.guarded` now satisfies the new `BrainController`)
- Delete: `brain-models.example.json`
- Modify: `docs/Architecture.md`, `.env.example`, `.ptolemy/policy.json`

- [ ] **Step 1: Full build + vet + test**

Run:
```bash
go build ./...
go vet ./...
go test -p 1 ./internal/... ./cmd/...
```
Expected: all PASS (the pre-existing Windows-only `TestCreateWorktree` failure is unrelated; ignore it).

- [ ] **Step 2: Remove the stale registry example, update env**

```bash
git rm brain-models.example.json
```

In `.env.example`, replace the brain block:

```
# Brain controller (workerd-managed llama.cpp). OFF by default. Same host as the brain.
BRAIN_CONTROL_ENABLED=false
BRAIN_AUTO_WAKE=false              # /chat resumes the loaded spec on demand when true
BRAIN_IDLE_TTL=5m                  # hibernate after this much inactivity
BRAIN_CONTROL_PORT=8089            # loopback POST /brain/{load,resume,hibernate,stop}, GET /brain/{models,status}
BRAIN_MODELS_DIR=/home/you/models  # scanned for *.gguf by GET /brain/models
BRAIN_LLAMA_BIN=/home/you/llama.cpp/build/bin/llama-server  # default binary for loads that omit it
```

- [ ] **Step 3: Update the Architecture note**

In `docs/Architecture.md`, replace the "Brain lifecycle skill" paragraph with a controller description: free-form `Spec` (binary/gguf/host/port/args) replaces the registry; `GET /brain/models` disk-scans `BRAIN_MODELS_DIR`; `load` is ask/OOB and carries the full argv (so deny rules apply), `resume`/`hibernate`/`status`/`models` allow; hibernate keeps the spec and `/chat`/`resume` auto-relaunch it; cold start → 502 (`/chat`) or 409 (`/brain/resume`); still flag-gated, loopback-only, audited under `brain-system`.

- [ ] **Step 4: Update host-local policy.json**

Edit `.ptolemy/policy.json` to mirror the new `DefaultRuleset()` brain rules (add `allow-brain-models/resume/hibernate`, `ask-brain-load`; remove `ask-brain-switch`, `allow-brain-autounload`; keep wake/status/stop). This file is gitignored — do not `git add` it; it is the host runtime copy.

- [ ] **Step 5: Commit**

```bash
git add docs/Architecture.md .env.example
git commit -m "docs: brain controller — Architecture note + .env.example; drop registry example"
```

---

## Self-review checklist (run before execution)

- **Spec coverage:** discovery (T3), free-form load incl. binary (T2/T4/T5/T7), hibernate→resume keeps spec (T4), auto-resume via EnsureAwake (T4/T8), load=ask / rest=allow (T5/T6), engine matches full argv → deny coverage (T5 test), loopback-only (T7), cold-start 502/409 (T4/T7), config knobs (T1), flag-off default (T8). ✓
- **Placeholders:** none — every step has concrete code/commands.
- **Type consistency:** `Spec`, `Spec.Argv()/WithDefaults/Validate/BaseURL`, `DiscoveredModel`, `ErrNoModelLoaded`, `Manager.{Load,Resume,EnsureAwake,Hibernate,Stop,Status,ListModels,ResolveSpec}`, `Probe.Ready(ctx,baseURL)`, `RawBrain`/`BrainController` method sets, `GuardedBrain` method set — all consistent across tasks.

## Verification (final)

- `go build ./... && go vet ./...` clean.
- `go test -p 1 ./internal/... ./cmd/...` green (modulo the pre-existing Windows-only worktree test).
- Flag-off default: `BRAIN_CONTROL_ENABLED` unset → `buildBrainDeps` ok=false, workerd serves unchanged.
- No push until the user states merge intent (then the release-flow rule in CLAUDE.md applies).
