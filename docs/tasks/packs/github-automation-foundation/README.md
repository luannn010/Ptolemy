# GitHub Automation Foundation Pack

This task pack captures the task-pack automation gaps that still exist after the sequential pack runner MVP.

## Goal

Add the first safe foundation for task-pack execution handoff:

- persist pack execution artifacts
- prepare task branches without switching the live workspace
- write failure issue drafts
- write success pull request drafts

## Notes

- This pack follows the current v1 task-pack conventions.
- `task-scripts/` and `snippets/` are still validated references only.
- The first implementation slice should stay local-first and testable without requiring GitHub network access.
