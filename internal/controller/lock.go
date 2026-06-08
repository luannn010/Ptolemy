package controller

import "context"

// IntegrationLock serializes Stage-2 promotion: at most one holder at a time.
// The returned release func is idempotent and must be called (defer) to free
// the lock. Acquire blocks until the lock is held or ctx is done.
type IntegrationLock interface {
	Acquire(ctx context.Context) (release func(), err error)
}
