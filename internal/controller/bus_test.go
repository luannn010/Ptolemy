package controller

import (
	"sync"
	"testing"
)

func TestBusFanOutToAllSubscribers(t *testing.T) {
	b := NewBus()
	defer b.Close()

	ch1, unsub1 := b.Subscribe(4)
	defer unsub1()
	ch2, unsub2 := b.Subscribe(4)
	defer unsub2()

	b.Publish(Event{Type: EventWorkerStateChanged, WorkerID: "w1", From: StatePending, To: StateProvisioning})

	for i, ch := range []<-chan Event{ch1, ch2} {
		ev := <-ch
		if ev.WorkerID != "w1" || ev.To != StateProvisioning {
			t.Fatalf("subscriber %d got %+v", i, ev)
		}
	}
}

func TestBusUnsubscribeStopsDelivery(t *testing.T) {
	b := NewBus()
	defer b.Close()

	ch, unsub := b.Subscribe(4)
	unsub()
	unsub() // idempotent, must not panic

	b.Publish(Event{Type: EventWorkerStateChanged, WorkerID: "w1"})

	if _, ok := <-ch; ok {
		t.Fatal("expected channel closed after unsubscribe")
	}
}

func TestBusSlowSubscriberDropsWithoutBlocking(t *testing.T) {
	b := NewBus()
	defer b.Close()

	// buffer 1, never drained
	_, unsub := b.Subscribe(1)
	defer unsub()

	for i := 0; i < 10; i++ {
		b.Publish(Event{Type: EventWorkerStateChanged, WorkerID: "w1"})
	}

	if b.Dropped() == 0 {
		t.Fatal("expected dropped events > 0")
	}
}

func TestBusCloseIsIdempotentAndPublishAfterCloseIsNoop(t *testing.T) {
	b := NewBus()
	ch, _ := b.Subscribe(1)
	b.Close()
	b.Close() // idempotent

	if _, ok := <-ch; ok {
		t.Fatal("expected subscriber channel closed on bus Close")
	}
	b.Publish(Event{Type: EventWorkerStateChanged}) // must not panic
}

func TestBusConcurrentPublishIsSafe(t *testing.T) {
	b := NewBus()
	defer b.Close()
	ch, unsub := b.Subscribe(1000)
	defer unsub()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); b.Publish(Event{Type: EventWorkerStateChanged}) }()
	}
	wg.Wait()
	_ = ch
}
