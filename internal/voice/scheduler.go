package voice

import (
	"fmt"
	"os/exec"
	"runtime"
	"sync"
	"time"
)

type Scheduler struct {
	mu     sync.Mutex
	timers []*time.Timer
}

func NewScheduler() *Scheduler {
	return &Scheduler{}
}

func (s *Scheduler) Schedule(kind string, when time.Time, message string) {
	delay := time.Until(when)
	if delay < 0 {
		delay = 0
	}
	timer := time.AfterFunc(delay, func() {
		fmt.Printf("[%s] %s at %s\n", kind, message, time.Now().Format(time.RFC1123))
		_ = notifyWindows(message)
	})
	s.mu.Lock()
	s.timers = append(s.timers, timer)
	s.mu.Unlock()
}

func (s *Scheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, timer := range s.timers {
		timer.Stop()
	}
	s.timers = nil
}

func notifyWindows(message string) error {
	if runtime.GOOS != "windows" {
		return nil
	}
	// MVP notification: use PowerShell message box and beep for local feedback.
	script := fmt.Sprintf(`$msg=%q; [console]::beep(1000,300); Add-Type -AssemblyName PresentationFramework; [System.Windows.MessageBox]::Show($msg, "Ptolemy Voice") | Out-Null`, message)
	return exec.Command("powershell", "-NoProfile", "-Command", script).Run()
}
