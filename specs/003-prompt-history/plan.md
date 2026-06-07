# Implementation Plan: Interactive Prompt History Recall

**Branch**: `003-prompt-history` | **Date**: 2026-06-07 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/003-prompt-history/spec.md`

## Summary

Add bash-like in-session prompt recall to the existing `goagent` interactive
REPL by extending the current raw-mode line editor to understand up/down arrow
history navigation, preserve the draft that existed before navigation started,
and keep recalled lines editable before resubmission. The simplest design keeps
the repository stdlib-only, stores prompt history in the REPL rather than the
provider/session transcript, and leaves non-interactive input behavior
unchanged.

## Technical Context

**Language/Version**: Go 1.24

**Primary Dependencies**: Go standard library only

**Storage**: N/A for persistent storage; in-memory REPL-owned prompt history
for the current interactive session only

**Testing**: `go test ./...`, focused package tests in `internal/cli`, and
existing regression coverage in `internal/app`, `internal/session`, and
`test/integration`

**Target Platform**: Local CLI on Linux and macOS terminals

**Project Type**: Single Go CLI binary

**Performance Goals**: History navigation redraws immediately in the interactive
prompt, adds no extra provider round-trips, and leaves non-interactive
line-buffered execution unchanged

**Constraints**: Go-only repository; stdlib-only implementation; session-scoped
history only; preserve existing slash-command suggestions, approval prompts, and
current single-line editing model; no readline-style external framework; store
duplicate submitted prompts as distinct entries

**Scale/Scope**: One interactive terminal session, one active draft, a linear
history of submitted lines for that session, and raw-mode handling limited to
the current prompt editor

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- **Simplicity**: Extend the existing REPL raw-mode input loop and redraw path
  instead of adding persistent history files, cross-session storage, or a
  general terminal-editing framework. Reject full readline emulation,
  multi-line composition, and transcript-backed history.
- **Dependencies**: No non-stdlib dependencies are planned. Escape-sequence
  parsing, history state, and redraw logic stay inside repository-owned Go.
- **Language Boundary**: The plan stays entirely within Go and reuses the
  existing CLI binary and internal packages.
- **Documentation Freshness**: N/A. This feature does not introduce a new
  external library, framework, SDK, API, CLI tool, or cloud dependency.

## Project Structure

### Documentation (this feature)

```text
specs/003-prompt-history/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   └── cli.md
└── tasks.md
```

### Source Code (repository root)

```text
cmd/
└── goagent/
    └── main.go

internal/
├── app/
│   ├── app.go
│   └── session_runner.go
├── cli/
│   ├── commands.go
│   ├── repl.go
│   ├── repl_test.go
│   ├── tty_darwin.go
│   ├── tty_linux.go
│   ├── tty_other.go
│   └── tty_unix.go
└── session/
    └── session.go

test/
└── integration/
    └── slash_command_discovery_test.go
```

**Structure Decision**: Keep the feature inside the existing single-binary
layout. The primary implementation belongs in `internal/cli/repl.go`; `app`
and `session` stay as regression surfaces rather than new ownership layers.
Do not add a new package or reusable `pkg/` surface for this prompt-editor
behavior.

## Phase 0: Research Decisions

- Keep prompt history as REPL-local state rather than storing it in
  `session.Session`, because history must include slash commands and draft
  navigation state that never enters the provider transcript.
- Record raw non-empty submitted lines at Enter time before parse/dispatch
  trimming so duplicates remain distinct, slash commands are recallable, and
  visible recalled text matches what the user originally entered.
- Recognize only the raw terminal escape sequences for up/down history
  navigation (`ESC [ A` and `ESC [ B`) and keep the current single-line editor,
  backspace behavior, and redraw model unchanged otherwise.
- Preserve the draft that existed before history navigation began and restore it
  only when the user navigates forward past the newest saved history entry.
  Stored history entries remain immutable even if the visible recalled text is
  edited before submission.
- Reuse the existing `renderInteractiveLine` path so recalled `/...` input keeps
  local slash suggestions automatically, while non-interactive stdin continues
  to use the current line-buffered fallback.

## Phase 1: Design & Contracts

- Add REPL-owned prompt-history state: a slice of submitted raw lines plus
  per-read navigation fields for the current visible buffer, preserved draft,
  and history cursor.
- Extend the raw-mode input loop to consume recognized arrow-key escape
  sequences before they are rendered as literal control bytes, clamp at oldest
  and newest history boundaries, and keep unsupported sequences as no-ops.
- Keep `handleLine`, prompt submission, slash-command dispatch, and session
  transcript behavior unchanged apart from recording the raw submitted line into
  REPL history once Enter is pressed on non-empty input.
- Treat recalled input as ordinary visible text: users can backspace, append,
  or submit it; submitting an edited recalled line records a new history entry
  while leaving earlier entries intact.
- Validate the feature mainly through REPL tests for empty-history no-op,
  oldest/newest clamping, draft restoration, duplicate entries, edited
  resubmission, recalled slash-command suggestions, long-line redraw safety, and
  non-interactive fallback.

## Implementation Direction

### Interactive Line Editor

- Keep raw-mode TTY setup in the current `tty_*` files unchanged.
- Introduce minimal REPL helpers for prompt-history recording, cursor movement,
  and escape-sequence decoding inside `internal/cli/repl.go`.
- Continue using full-line redraw with slash suggestions beneath the prompt
  instead of adding cursor-left/right editing or menu navigation.

### History Ownership

- Store prompt history on `REPL`, not in `session.Session`.
- Record only submitted non-empty lines for the current launch.
- Keep history ephemeral and reset it on process restart.

### Submission and Slash Behavior

- Leave ordinary prompt parsing and slash-command dispatch semantics unchanged.
- Recalled slash commands should re-trigger local suggestion rendering while
  visible in the prompt and still execute only on Enter.
- Approval prompts remain outside prompt history and continue using the existing
  `PromptApproval` line read.

### Validation Strategy

- Add focused `internal/cli` tests that exercise raw escape handling through
  the interactive reader path.
- Preserve current `internal/app`, `internal/session`, and integration test
  expectations to ensure the feature does not affect provider or approval flow.
- Run full `go test ./...` after implementation to confirm cross-package
  regressions are absent.

## Post-Design Constitution Check

- **Simplicity**: Still passes. The design stays inside the existing REPL and
  redraw path without introducing persistence or a larger terminal framework.
- **Dependencies**: Still passes. No non-stdlib module is introduced.
- **Language Boundary**: Still passes. All planned changes remain in Go.
- **Documentation Freshness**: Still N/A. No new external dependency was added
  during design.

## Complexity Tracking

No constitution violations are currently justified.
