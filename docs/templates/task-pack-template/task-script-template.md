# Task Script: <script-name>

## Intent
State the exact change this task script is trying to achieve.

## Allowed targets
`internal/tasks/validator.go`

## Stable anchors
- `type ValidationError struct {`
- `func ValidateTask(task Task) []ValidationError {`

## Referenced snippets
- `snippets/internal-tasks-validator.go`

## Expected outputs

- The target behavior exists in the allowed files.
- Validation commands for the task pass.

## Must not do

- Do not edit files outside the task's `allowed_files`.
- Do not rewrite whole source files when a targeted edit is possible.
- Do not invent extra behavior outside this task's stated goal.

## Instructions
Create or update the target behavior using the referenced snippets and the stable anchors above.
