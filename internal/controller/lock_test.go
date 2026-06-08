package controller

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
)

// fakeLock is an in-memory IntegrationLock: a size-1 semaphore plus a counter
// that records the maximum number of concurrent holders observed.
type fakeLock struct {
	sem     chan struct{}
	holders int32
	maxSeen int32
}

func newFakeLock() *fakeLock { return &fakeLock{sem: make(chan struct{}, 1)} }

func (l *fakeLock) Acquire(ctx context.Context) (func(), error) {
	select {
	case l.sem <- struct{}{}:
	case <-ctx.Done():
		return func() {}, ctx.Err()
	}
	n := atomic.AddInt32(&l.holders, 1)
	for {
		old := atomic.LoadInt32(&l.maxSeen)
		if n <= old || atomic.CompareAndSwapInt32(&l.maxSeen, old, n) {
			break
		}
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			atomic.AddInt32(&l.holders, -1)
			<-l.sem
		})
	}, nil
}

func TestFakeLockSerializes(t *testing.T) {
	l := newFakeLock()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rel, err := l.Acquire(context.Background())
			if err != nil {
				t.Errorf("acquire: %v", err)
				return
			}
			// hold briefly so overlap would be observed if seriality were broken
			for j := 0; j < 1000; j++ {
				_ = j
			}
			rel()
		}()
	}
	wg.Wait()
	if l.maxSeen > 1 {
		t.Fatalf("max concurrent holders = %d, want 1", l.maxSeen)
	}
}

// compile-time assertion that fakeLock implements IntegrationLock.
var _ IntegrationLock = (*fakeLock)(nil)
