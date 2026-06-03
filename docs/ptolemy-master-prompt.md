# Ptolemy — Claude Code Master Prompt

> Paste this **once** at the start of an implementation session.
> Then paste a phase prompt (see `ptolemy-phase-prompt.md`) to kick off actual work.

---

You are helping me implement **Ptolemy**, an orchestration system for multiple coding agents working in parallel sandboxes. The complete design is in two files in this repo, and they are the source of truth:

- `docs/ptolemy-design.md` — the system design document (RFC). Read **all of it** before writing code.
- `docs/ptolemy-architecture.html` — the visual architecture overview. Skim for orientation; the markdown is authoritative.

## Working rules

1. **Read the design doc first, in full, before any code.** Do not skim. The doc encodes decisions made deliberately; do not relitigate them.
2. **Implement one phase at a time, in the order given in section 11 of the design doc.** Do not skip ahead, even if you "could." Each phase has acceptance tests; the phase isn't done until they pass.
3. **Keep the model out of the plumbing.** Git, Docker, locks, tests, and event dispatch are deterministic Go code. No LLM calls in the controller's hot path. The orchestrator (the small model) issues high-level intents *on top of* the controller; the controller never calls a model.
4. **Match the stated stack.** Go over HTTP/REST, single-VM target, Postgres + Redis, in-process Go channels for events. See section 9 of the design doc for specific libraries (`os/exec` for git, `docker compose -p` for stacks, Hoverfly/Prism for mocks, `kin-openapi` for contracts, `golang-migrate` for migrations, Caddy or Traefik for routing). Don't introduce new infrastructure (RabbitMQ, NATS, k8s) — those are explicitly deferred.
5. **Ask before you guess.** If an open decision from section 10 of the design doc blocks a phase you're working on, *stop and ask me*. Do not invent a policy. The open-decision register is intentional; closing those is my call.
6. **Determinism over cleverness.** Prefer boring, debuggable code. This system fails attribution if anything is too magical. A clear `if`/`switch` beats a clever abstraction.
7. **Tests are not optional.** Every phase has acceptance tests in the design doc. Write them. Run them. Show me the output.

## What you do at the start of every session

1. Read `docs/ptolemy-design.md` end to end.
2. Run `git status` and `git log --oneline -20` so you know where the previous session left off.
3. Look at the phase checklist (the build order in section 11) and tell me which phase we're on and which items remain.
4. Wait for my phase prompt before writing code.

## What you do NOT do

- Do not implement multiple phases in one go.
- Do not add dependencies not named in section 9 without asking.
- Do not refactor unrelated code while working on a phase.
- Do not "improve" the design. If something seems wrong, flag it; don't silently change it.
- Do not call the small model from inside the controller. The orchestrator and controller are separate processes; they communicate through MCP, not function calls.
- Do not put any durable state in Redis. Redis is for ephemeral working context and as a read-through cache only. Postgres is the source of truth.

## When something is genuinely ambiguous

The design doc has an **open-decision register** in section 10. If you hit something that section flags as undecided (e.g. code conflict policy, schema migration handling, real-env reset), surface it as a question with a recommended default and your reasoning. Wait for my call. Do not pick silently.

## Definition of done for a phase

A phase is done when:

- All acceptance criteria in the design doc for that phase pass.
- Tests are written and green.
- I can run the phase's capability by hand (e.g. spawn 2 workers, see their diffs, watch them run).
- You have written a short `PHASE_N_NOTES.md` summarizing what was built, any assumptions made, and any items deferred — so the next session has a handoff.

Confirm you've read the design doc by summarizing **in three sentences** what Ptolemy does and what the two-stage gate is for. Then tell me which phase you think we should start with and why.
