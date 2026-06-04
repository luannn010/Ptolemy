package health

import (
	"context"
	"errors"
	"testing"
)

type fakeSQL struct{ err error }

func (f fakeSQL) PingContext(_ context.Context) error { return f.err }

type fakePG struct{ err error }

func (f fakePG) Ping(_ context.Context) error { return f.err }

func TestSQLChecker_Up(t *testing.T) {
	chk := NewSQLChecker("workerd", fakeSQL{nil}, true)
	c := chk.Check(context.Background())
	if c.Status != StatusUp {
		t.Fatalf("got %+v, want up", c)
	}
	if !chk.Required() {
		t.Fatal("Required() should be true")
	}
}

func TestSQLChecker_Down(t *testing.T) {
	c := NewSQLChecker("workerd", fakeSQL{errors.New("boom")}, true).Check(context.Background())
	if c.Status != StatusDown || c.Error != "boom" {
		t.Fatalf("got %+v, want down/boom", c)
	}
}

func TestSQLChecker_NilDown(t *testing.T) {
	c := NewSQLChecker("workerd", nil, true).Check(context.Background())
	if c.Status != StatusDown || c.Error != "not configured" {
		t.Fatalf("got %+v, want down/not configured", c)
	}
}

func TestPgChecker_Up(t *testing.T) {
	chk := NewPgChecker("postgres", fakePG{nil})
	c := chk.Check(context.Background())
	if c.Status != StatusUp {
		t.Fatalf("got %+v, want up", c)
	}
	if chk.Required() {
		t.Fatal("Required() should be false (postgres is optional)")
	}
}

func TestPgChecker_Down(t *testing.T) {
	c := NewPgChecker("postgres", fakePG{errors.New("refused")}).Check(context.Background())
	if c.Status != StatusDown || c.Error != "refused" {
		t.Fatalf("got %+v, want down/refused", c)
	}
}

func TestPgChecker_NilDisabled(t *testing.T) {
	c := NewPgChecker("postgres", nil).Check(context.Background())
	if c.Status != StatusDisabled {
		t.Fatalf("got %+v, want disabled", c)
	}
}
