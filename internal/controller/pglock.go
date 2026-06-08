package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PgConnPool is the slice of *pgxpool.Pool the lock needs. Declared as an
// interface so the lock can be tested against a fake pool.
type PgConnPool interface {
	Acquire(ctx context.Context) (*pgxpool.Conn, error)
}

// PgLock is an IntegrationLock backed by a Postgres session-level advisory lock.
// Because a session advisory lock is bound to its connection, a crashed holder's
// connection drops and Postgres releases the lock automatically (crash reclaim).
// lease is recorded for callers/telemetry; bounding a live holder's hold time is
// the caller's responsibility via the context it runs Stage 2 under.
type PgLock struct {
	pool  PgConnPool
	key   int64
	lease time.Duration
}

// NewPgLock constructs a Postgres advisory lock over the given pool and 64-bit
// key. lease may be zero (no cap).
func NewPgLock(pool PgConnPool, key int64, lease time.Duration) *PgLock {
	return &PgLock{pool: pool, key: key, lease: lease}
}

// Acquire grabs a dedicated connection and blocks on pg_advisory_lock(key).
func (l *PgLock) Acquire(ctx context.Context) (func(), error) {
	conn, err := l.pool.Acquire(ctx)
	if err != nil {
		return func() {}, fmt.Errorf("pglock: acquire conn: %w", err)
	}
	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", l.key); err != nil {
		conn.Release()
		return func() {}, fmt.Errorf("pglock: advisory_lock: %w", err)
	}
	released := false
	return func() {
		if released {
			return
		}
		released = true
		// Use a fresh short context so unlock runs even if the caller's ctx is done.
		uctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = conn.Exec(uctx, "SELECT pg_advisory_unlock($1)", l.key)
		conn.Release()
	}, nil
}

// compile-time checks.
var _ IntegrationLock = (*PgLock)(nil)
var _ PgConnPool = (*pgxpool.Pool)(nil)
