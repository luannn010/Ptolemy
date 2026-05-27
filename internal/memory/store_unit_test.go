package memory

import "testing"

func TestNewPgStore_ReturnsNonNil(t *testing.T) {
	// Construction smoke: NewPgStore wraps the *pgx.Conn pointer; passing nil
	// is fine for the constructor (the wrapped value is only dereferenced when
	// a method is called against a real DB).
	if s := NewPgStore(nil); s == nil {
		t.Fatalf("NewPgStore returned nil")
	}
}
