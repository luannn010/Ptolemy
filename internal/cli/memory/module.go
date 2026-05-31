// Package memory holds the `ptolemy memory` CLI subcommands (recall, capture,
// demo, eval, synth-eval). Each subcommand exposes a RunX(ctx, args, stdout,
// stderr) error entry point; the parent dispatcher (cmd/ptolemy) owns process
// exit so that deferred cleanup — notably closing the DB connection — runs.
//
// These commands drive the in-process internal/memory module directly. Per the
// CLAUDE.md carve-out, internal/memory writes only to the memory Postgres DB
// (never the guarded workspace, shell, or git), so this CLI inherits that
// exemption and needs no Guarded* wrapper.
package memory

import "errors"

// ErrEvalFailed signals that an eval subcommand ran cleanly but one or more
// scenarios/checks did not pass. The dispatcher maps it to exit code 1, keeping
// it distinct from setup/runtime errors (exit 2).
var ErrEvalFailed = errors.New("eval: one or more scenarios failed")
