package controller

import "sync"

// EventType identifies a controller event.
type EventType string

const (
	// EventWorkerStateChanged is published on every legal worker state transition.
	EventWorkerStateChanged EventType = "worker.state_changed"
	// EventBaseUpdated is reserved for a later propagation slice.
	EventBaseUpdated EventType = "base_updated"
)

// Event is a single message on the bus.
type Event struct {
	Type     EventType
	WorkerID string
	From     State
	To       State
	Payload  any
}

type subscriber struct {
	ch chan Event
}

// Bus is an in-process pub/sub bus with non-blocking fan-out. Each subscriber
// owns a bounded buffered channel; a full buffer drops the event (counted)
// rather than blocking the publisher.
type Bus struct {
	mu      sync.Mutex
	subs    map[int]*subscriber
	nextID  int
	dropped int
	closed  bool
}

// NewBus returns an open bus.
func NewBus() *Bus {
	return &Bus{subs: make(map[int]*subscriber)}
}

// Subscribe registers a subscriber with the given channel buffer and returns the
// receive channel plus an idempotent unsubscribe func.
func (b *Bus) Subscribe(buffer int) (<-chan Event, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		ch := make(chan Event)
		close(ch)
		return ch, func() {}
	}

	id := b.nextID
	b.nextID++
	s := &subscriber{ch: make(chan Event, buffer)}
	b.subs[id] = s

	var once sync.Once
	unsub := func() {
		once.Do(func() {
			b.mu.Lock()
			defer b.mu.Unlock()
			if sub, ok := b.subs[id]; ok {
				delete(b.subs, id)
				close(sub.ch)
			}
		})
	}
	return s.ch, unsub
}

// Publish delivers ev to every subscriber without blocking the caller.
func (b *Bus) Publish(ev Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	for _, s := range b.subs {
		select {
		case s.ch <- ev:
		default:
			b.dropped++
		}
	}
}

// Dropped returns the total number of events dropped due to full buffers.
func (b *Bus) Dropped() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.dropped
}

// Close closes all subscriber channels. Idempotent; Publish after Close is a no-op.
func (b *Bus) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.closed = true
	for id, s := range b.subs {
		close(s.ch)
		delete(b.subs, id)
	}
}
