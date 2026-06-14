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

func TestRunIdleLoop_HibernatesWhenIdleAndToleratesError(t *testing.T) {
	var unloads atomic.Int32
	// Running and idle for an hour, ttl is tiny -> every tick should hibernate.
	status := func() Status { return Status{Running: true, LastUse: time.Now().Add(-time.Hour)} }
	unload := func(context.Context) error { unloads.Add(1); return errors.New("transient") }

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { RunIdleLoop(ctx, 2*time.Millisecond, time.Millisecond, status, unload); close(done) }()

	deadline := time.Now().Add(2 * time.Second)
	for unloads.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(2 * time.Millisecond)
	}
	cancel()
	<-done
	if unloads.Load() < 2 {
		t.Fatalf("expected the loop to keep hibernating past an unload error, got %d", unloads.Load())
	}
}

func TestRunIdleLoop_SkipsWhenNotIdle(t *testing.T) {
	var unloads atomic.Int32
	// Running but just used; ttl is long -> never idle, never hibernate.
	status := func() Status { return Status{Running: true, LastUse: time.Now()} }
	unload := func(context.Context) error { unloads.Add(1); return nil }

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { RunIdleLoop(ctx, 2*time.Millisecond, time.Hour, status, unload); close(done) }()
	time.Sleep(40 * time.Millisecond)
	cancel()
	<-done
	if unloads.Load() != 0 {
		t.Fatalf("must not hibernate while active, got %d", unloads.Load())
	}
}

func TestRunIdleLoop_StopsOnCancel(t *testing.T) {
	status := func() Status { return Status{} }
	unload := func(context.Context) error { return nil }
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { RunIdleLoop(ctx, time.Millisecond, time.Millisecond, status, unload); close(done) }()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("RunIdleLoop did not return after ctx cancel")
	}
}
