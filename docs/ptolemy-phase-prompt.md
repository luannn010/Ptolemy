# Ptolemy — Phase Prompt Template

> Paste this **after** the master prompt, once per phase.
> Fill in the `{{PHASE}}` placeholder. Run phases in the order from section 11 of the design doc.

---

## Phase prompt

We're implementing **Phase {{PHASE}}** from section 11 of `docs/ptolemy-design.md`.

Before writing any code:

1. Open the design doc and read section 11 for the full build order, plus the sections relevant to this phase (section 4 for the architecture, section 5 for the concept being built, section 6 for the lifecycle if relevant, section 9 for the Go stack mapping).
2. Re-read section 10 (open decisions). If any open decision is required to complete this phase, **stop and ask me before writing code.** Do not pick a default silently.
3. Tell me what you understood the phase to be, in your own words: scope, what's in, what's out, and what the acceptance criteria are. **Wait for my "go" before writing code.**

When I say go:

4. Write the code, smallest working version first.
5. Write tests against the acceptance criteria. Run them. Show me the output.
6. Let me actually exercise it by hand (e.g. "run `make spawn-two` and you should see two ports respond"). Give me the command.
7. Write `PHASE_{{PHASE}}_NOTES.md` with: what was built, assumptions made, anything deferred, anything you noticed that the design doc should probably address but doesn't.
8. **Stop.** Do not start the next phase. Wait for the next phase prompt.

## Constraints specific to this phase

- Touch only files this phase needs. If you find yourself wanting to edit code from a future phase, stop — that's a sign you're conflating phases.
- Keep dependencies to what the design doc names in section 9. Adding a new library is a question, not a decision.
- If you discover the design doc is wrong or missing something needed for this phase, **flag it, propose the fix in a comment, but do not silently change the design.** I'll update the doc.

## Done means

- All acceptance criteria for Phase {{PHASE}} pass.
- I've exercised the capability by hand and confirmed it works.
- `PHASE_{{PHASE}}_NOTES.md` is written.
- A clean commit (or short series of commits) with descriptive messages.

Begin by reading the doc and giving me your understanding of Phase {{PHASE}}'s scope and acceptance criteria. Then wait.
