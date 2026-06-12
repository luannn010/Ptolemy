package brain

import (
	"context"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// --- fakes -------------------------------------------------------------------

type fakeHandle struct {
	mu         sync.Mutex
	running    bool
	signaled   bool
	killed     bool
	waitBlocks bool // if true, Wait blocks until Kill (exercises the kill-on-timeout path)
}

func (h *fakeHandle) Signal(_ os.Signal) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.signaled = true
	if !h.waitBlocks {
		h.running = false
	}
	return nil
}

func (h *fakeHandle) Kill() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.killed = true
	h.running = false
	return nil
}

func (h *fakeHandle) Wait() error {
	for {
		h.mu.Lock()
		blocking := h.waitBlocks && h.running
		h.mu.Unlock()
		if !blocking {
			return nil
		}
		time.Sleep(time.Millisecond)
	}
}

func (h *fakeHandle) Running() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.running
}

type fakeLauncher struct {
	mu         sync.Mutex
	startCalls int
	lastArgv   []string
	failStart  bool
	handles    []*fakeHandle // returned in order; default is a fresh running handle
}

func (l *fakeLauncher) Start(argv []string) (Handle, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.startCalls++
	l.lastArgv = argv
	if l.failStart {
		return nil, errors.New("start failed")
	}
	if len(l.handles) > 0 {
		h := l.handles[0]
		l.handles = l.handles[1:]
		return h, nil
	}
	return &fakeHandle{running: true}, nil
}

type fakeProbe struct {
	mu         sync.Mutex
	ready      bool
	readyAfter int
	calls      int
}

func (p *fakeProbe) Ready(_ context.Context) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	if p.readyAfter > 0 {
		return p.calls >= p.readyAfter
	}
	return p.ready
}

func newTestManager(t *testing.T, l *fakeLauncher, p *fakeProbe) *Manager {
	t.Helper()
	reg, err := LoadRegistry(writeRegistry(t, sampleRegistry))
	if err != nil {
		t.Fatal(err)
	}
	m := NewManager(reg, l, p, "0.0.0.0", "9000", "qwen9b")
	m.readyTimeout = 200 * time.Millisecond
	m.pollInterval = 2 * time.Millisecond
	m.stopTimeout = 50 * time.Millisecond
	return m
}

// --- tests -------------------------------------------------------------------

func TestWake_StartsAndBecomesReady(t *testing.T) {
	l := &fakeLauncher{}
	m := newTestManager(t, l, &fakeProbe{ready: true})
	if err := m.Wake(context.Background(), "qwen9b"); err != nil {
		t.Fatalf("wake: %v", err)
	}
	st := m.Status()
	if !st.Running || st.Model != "qwen9b" {
		t.Fatalf("status wrong: %+v", st)
	}
	argv := l.lastArgv
	want := []string{"/bin/llama-server", "-m", "/m/qwen9b.gguf", "--host", "0.0.0.0", "--port", "9000", "--ctx-size", "32768", "-ngl", "999"}
	if len(argv) != len(want) {
		t.Fatalf("argv len: got %v", argv)
	}
	for i := range want {
		if argv[i] != want[i] {
			t.Fatalf("argv[%d]=%q want %q (full %v)", i, argv[i], want[i], argv)
		}
	}
}

func TestWake_AlreadyRunningSameModel_NoSecondStart(t *testing.T) {
	l := &fakeLauncher{}
	m := newTestManager(t, l, &fakeProbe{ready: true})
	_ = m.Wake(context.Background(), "qwen9b")
	_ = m.Wake(context.Background(), "qwen9b")
	if l.startCalls != 1 {
		t.Fatalf("expected one Start for an already-running model, got %d", l.startCalls)
	}
}

func TestEnsureAwake_WakesDefault(t *testing.T) {
	l := &fakeLauncher{}
	m := newTestManager(t, l, &fakeProbe{ready: true})
	if err := m.EnsureAwake(context.Background()); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if st := m.Status(); !st.Running || st.Model != "qwen9b" {
		t.Fatalf("EnsureAwake should wake the default model, got %+v", st)
	}
}

func TestStop_SignalsAndClears(t *testing.T) {
	l := &fakeLauncher{handles: []*fakeHandle{{running: true}}}
	m := newTestManager(t, l, &fakeProbe{ready: true})
	_ = m.Wake(context.Background(), "qwen9b")
	if err := m.Stop(context.Background()); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if st := m.Status(); st.Running {
		t.Fatal("expected not running after stop")
	}
}

func TestStop_KillsWhenGracefulHangs(t *testing.T) {
	h := &fakeHandle{running: true, waitBlocks: true}
	l := &fakeLauncher{handles: []*fakeHandle{h}}
	m := newTestManager(t, l, &fakeProbe{ready: true})
	_ = m.Wake(context.Background(), "qwen9b")
	_ = m.Stop(context.Background())
	if !h.killed {
		t.Fatal("expected Kill after graceful stop timed out")
	}
}

func TestSwitch_UnknownModel_KeepsCurrent(t *testing.T) {
	l := &fakeLauncher{}
	m := newTestManager(t, l, &fakeProbe{ready: true})
	_ = m.Wake(context.Background(), "qwen9b")
	if err := m.Switch(context.Background(), "llama70b"); err == nil {
		t.Fatal("expected error switching to unknown model")
	}
	st := m.Status()
	if !st.Running || st.Model != "qwen9b" {
		t.Fatalf("current model must survive a failed switch, got %+v", st)
	}
	if l.startCalls != 1 {
		t.Fatalf("failed switch must not start anything, starts=%d", l.startCalls)
	}
}

func TestSwitch_StopsThenStartsNew(t *testing.T) {
	h1 := &fakeHandle{running: true}
	l := &fakeLauncher{handles: []*fakeHandle{h1, {running: true}}}
	m := newTestManager(t, l, &fakeProbe{ready: true})
	_ = m.Wake(context.Background(), "qwen9b")
	if err := m.Switch(context.Background(), "qwen4b"); err != nil {
		t.Fatalf("switch: %v", err)
	}
	if !h1.signaled {
		t.Fatal("old model should be stopped on switch")
	}
	if st := m.Status(); st.Model != "qwen4b" || !st.Running {
		t.Fatalf("expected qwen4b running after switch, got %+v", st)
	}
	if l.startCalls != 2 {
		t.Fatalf("switch should start the new model, starts=%d", l.startCalls)
	}
}

func TestWake_ReadyTimeout_StopsAndErrors(t *testing.T) {
	h := &fakeHandle{running: true}
	l := &fakeLauncher{handles: []*fakeHandle{h}}
	m := newTestManager(t, l, &fakeProbe{ready: false}) // never ready
	if err := m.Wake(context.Background(), "qwen9b"); err == nil {
		t.Fatal("expected readiness timeout error")
	}
	if st := m.Status(); st.Running {
		t.Fatal("a timed-out wake must stop the process")
	}
}

func TestWake_StartFails(t *testing.T) {
	m := newTestManager(t, &fakeLauncher{failStart: true}, &fakeProbe{ready: true})
	if err := m.Wake(context.Background(), "qwen9b"); err == nil {
		t.Fatal("expected start failure error")
	}
}

func TestWake_ProcessDiesDuringStartup(t *testing.T) {
	dead := &fakeHandle{running: false} // exited immediately
	l := &fakeLauncher{handles: []*fakeHandle{dead}}
	m := newTestManager(t, l, &fakeProbe{ready: false})
	if err := m.Wake(context.Background(), "qwen9b"); err == nil {
		t.Fatal("expected error when the process exits during startup")
	}
}

func TestRunIdleLoop_UnloadsWhenIdle(t *testing.T) {
	var unloads atomic.Int32
	status := func() Status { return Status{Running: true, LastUse: time.Now().Add(-time.Hour)} }
	unload := func(_ context.Context) error { unloads.Add(1); return nil }

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { RunIdleLoop(ctx, time.Millisecond, time.Millisecond, status, unload); close(done) }()
	time.Sleep(30 * time.Millisecond)
	cancel()
	<-done
	if unloads.Load() == 0 {
		t.Fatal("idle-past-ttl must trigger unload")
	}
}

func TestRunIdleLoop_SkipsWhenActive(t *testing.T) {
	var unloads atomic.Int32
	status := func() Status { return Status{Running: true, LastUse: time.Now()} } // just used
	unload := func(_ context.Context) error { unloads.Add(1); return nil }

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { RunIdleLoop(ctx, time.Millisecond, time.Hour, status, unload); close(done) }()
	time.Sleep(20 * time.Millisecond)
	cancel()
	<-done
	if unloads.Load() != 0 {
		t.Fatalf("active brain must not unload, got %d", unloads.Load())
	}
}

func TestRunIdleLoop_StopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan struct{})
	go func() {
		RunIdleLoop(ctx, time.Millisecond, time.Millisecond,
			func() Status { return Status{} }, func(context.Context) error { return nil })
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("RunIdleLoop must return promptly on cancel")
	}
}
