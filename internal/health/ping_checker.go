package health

import (
	"context"
	"time"
)

// sqlPinger is satisfied by *sql.DB (PingContext).
type sqlPinger interface {
	PingContext(ctx context.Context) error
}

// pgPinger is satisfied by *pgxpool.Pool (Ping).
type pgPinger interface {
	Ping(ctx context.Context) error
}

// pingChecker probes a *sql.DB handle (workerd's own SQLite store). It is
// required: a nil handle is a misconfiguration and reports down.
type pingChecker struct {
	name     string
	db       sqlPinger
	required bool
}

// NewSQLChecker builds a PingContext-based checker over a *sql.DB.
func NewSQLChecker(name string, db sqlPinger, required bool) Checker {
	return &pingChecker{name: name, db: db, required: required}
}

func (p *pingChecker) Name() string   { return p.name }
func (p *pingChecker) Required() bool { return p.required }

func (p *pingChecker) Check(ctx context.Context) Check {
	c := Check{Name: p.name, Required: p.required}
	if p.db == nil {
		c.Status = StatusDown
		c.Error = "not configured"
		return c
	}
	start := time.Now()
	err := p.db.PingContext(ctx)
	c.LatencyMS = time.Since(start).Milliseconds()
	if err != nil {
		c.Status = StatusDown
		c.Error = err.Error()
		return c
	}
	c.Status = StatusUp
	return c
}

// pgChecker probes a Postgres pool. It is optional: a nil pool means the memory
// DB is not configured and reports disabled.
type pgChecker struct {
	name string
	pool pgPinger
}

// NewPgChecker builds a Ping-based checker over a *pgxpool.Pool. Pass an untyped
// nil pgPinger (not a typed-nil pool) when DATABASE_URL is unset.
func NewPgChecker(name string, pool pgPinger) Checker {
	return &pgChecker{name: name, pool: pool}
}

func (p *pgChecker) Name() string   { return p.name }
func (p *pgChecker) Required() bool { return false }

func (p *pgChecker) Check(ctx context.Context) Check {
	c := Check{Name: p.name, Required: false}
	if p.pool == nil {
		c.Status = StatusDisabled
		return c
	}
	start := time.Now()
	err := p.pool.Ping(ctx)
	c.LatencyMS = time.Since(start).Milliseconds()
	if err != nil {
		c.Status = StatusDown
		c.Error = err.Error()
		return c
	}
	c.Status = StatusUp
	return c
}
