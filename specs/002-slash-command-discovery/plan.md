# Implementation Plan: Slash Command Discovery and Per-Command Approval

**Branch**: `002-slash-command-discovery` | **Date**: 2026-06-06 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/002-slash-command-discovery/spec.md`

## Summary

Add in-terminal slash-command discovery and inline exact-command shell approval
to the existing `goagent` CLI without changing its provider integrations or
default tool surface. The simplest viable design keeps the repository
stdlib-only, extends the current REPL to expose filtered slash-command help
while the user types slash input, and replaces session-wide shell approval with
an exact-command approval cache plus same-turn resume for blocked shell calls.

## Technical Context

**Language/Version**: Go 1.24

**Primary Dependencies**: Go standard library only

**Storage**: N/A for persistent storage; in-memory session state only

**Testing**: `go test ./...`, new package-local tests in `internal/cli`,
`internal/app`, and `internal/session`, plus scripted REPL/session tests using
in-memory readers and writers

**Target Platform**: Local CLI on Linux and macOS terminals

**Project Type**: Single Go CLI binary

**Performance Goals**: Slash suggestions render during interactive slash input
without requiring a second command submission; approved shell commands resume in
the current turn without a second provider request; existing shell timeout and
tool-output truncation behavior remain unchanged

**Constraints**: Go-only repository, stdlib-first, workspace-only shell
execution, exact-match shell approval reused only for identical command text in
the same resolved working directory during one session, no blanket shell
approval, no provider protocol changes

**Scale/Scope**: One interactive terminal session, one pending shell approval
at a time, current slash-command set plus suggestion metadata, no persistence
across launches

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- **Simplicity**: Extend the current REPL, executor, and session state instead
  of introducing a general terminal framework, a separate approval service, or
  a new control protocol. Reject menu-heavy slash-command UIs, provider-side
  approval prompts, and generalized policy engines.
- **Dependencies**: No non-stdlib dependencies are planned. Terminal rendering,
  slash parsing, approval prompting, and command-signature tracking stay inside
  repository-owned Go code.
- **Language Boundary**: The plan stays entirely within Go and reuses the
  existing CLI binary and internal packages.
- **Documentation Freshness**: N/A. This feature does not introduce a new
  external library, SDK, API, CLI tool, or cloud dependency. Existing provider
  integrations remain unchanged.

## Project Structure

### Documentation (this feature)

```text
specs/002-slash-command-discovery/
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
│   ├── executor.go
│   ├── module_executor.go
│   ├── runtime.go
│   └── session_runner.go
├── cli/
│   ├── commands.go
│   ├── module_commands.go
│   └── repl.go
├── session/
│   └── session.go
└── tools/
    └── shell.go
```

**Structure Decision**: Keep the current single-binary structure and implement
the feature inside the existing `internal/cli`, `internal/app`, and
`internal/session` packages. Do not add new top-level packages or a reusable
`pkg/` surface for this narrow terminal feature.

## Phase 0: Research Decisions

- Use a terminal-owned slash command catalog instead of separate usage strings
  and switch-only parsing so parsing, display text, and descriptions come from
  one source of truth.
- Keep slash suggestions informational and prefix-filtered rather than adding
  arrow-key menus or fuzzy search. The user still submits a slash command by
  pressing Enter on the command line they typed.
- Keep the repository stdlib-only by adding a small terminal session layer in
  `internal/cli` for interactive TTY input and redraw behavior, while retaining
  the current line-buffered path for non-interactive readers.
- Replace shell capability-wide approval with a session-scoped exact-command
  approval signature derived from the raw shell command text plus the resolved
  working directory.
- Pause blocked shell tool calls locally and resume them in the same turn after
  approval instead of appending an approval error back into the transcript and
  asking the user to retry manually.

## Phase 1: Design & Contracts

- Introduce command metadata for slash commands: command name, usage line, and
  short description. Use it for parsing errors, `/` suggestions, and future
  command-list output.
- Extend the REPL input loop so interactive terminal sessions can redraw the
  current slash-command line and a filtered suggestion list while the user types
  `/...`. Keep the existing buffered line reader as the non-TTY fallback.
- Retain `/approve` only for non-shell risky capabilities (`write`, `web`,
  `module`). Shell approval moves to an inline prompt owned by the running
  session turn.
- Add exact shell-approval state to `session.Session` alongside the existing
  capability approvals so write/web/module behavior remains unchanged.
- Add a pending shell approval record that holds the blocked command signature,
  prompt text, and tool-call context until the user approves or denies it.
- Move shell-specific approval checks out of the generic
  `approvalCommand(capability)` path and into the decoded shell-execution path,
  because exact-command approval depends on parsed `ShellInput`.
- Provide an approval callback from the app/session-runner layer so the
  executor can synchronously ask `Approve running <command>?` and continue or
  deny within the same user turn.

## Implementation Direction

### Terminal Interaction

- Refactor `internal/cli/repl.go` from a pure newline reader into a terminal
  session that can distinguish interactive TTY input from buffered input.
- When the current line begins with `/`, compute prefix matches from the command
  catalog and render one short-description line per match.
- Clear or redraw suggestions after normal command submission, cancellation, or
  transition back to ordinary prompt text.
- Keep normal natural-language input behavior unchanged for non-slash lines.

### Approval Flow

- Preserve capability approvals for `write`, `web`, and `module`.
- Replace `/approve shell` with inline approval prompts for `run_shell`.
- Treat two shell requests as the same only when both the raw command string
  and resolved working directory match exactly.
- Cache approved shell signatures until the session ends; changing any argument,
  shell text, or resolved workdir requires new approval.
- On denial, return a clear session-visible message and do not append a fake
  successful tool result into the transcript.

### Tool Execution Flow

- Decode `ShellInput` before approval enforcement so the executor can build the
  exact approval signature from the command text and resolved workdir.
- Route approval-required shell events through an app-level prompt path instead
  of emitting `/approve shell` guidance.
- Resume the blocked shell call immediately after approval without reissuing the
  original natural-language request to the model.
- Keep provider request/response shapes, tool schemas, and workspace
  restrictions unchanged.

### Validation Strategy

- Add parser/catalog tests for slash-command metadata, prefix filtering, and
  usage rendering in `internal/cli`.
- Add session tests for exact-command approval caching and pending approval
  lifecycle in `internal/session`.
- Add executor tests for shell approval signature matching, denial behavior, and
  approval reuse rules in `internal/app`.
- Add scripted REPL/app tests that cover `/` suggestions, inline shell approval,
  same-turn resume, and non-TTY fallback behavior.

## Post-Design Constitution Check

- **Simplicity**: Still passes. The design adds one terminal input layer and
  one shell-approval path inside existing packages instead of larger framework
  changes.
- **Dependencies**: Still passes. No non-stdlib module is introduced.
- **Language Boundary**: Still passes. All planned changes remain in Go.
- **Documentation Freshness**: Still N/A. No new external dependency was added
  during design.

## Complexity Tracking

No constitution violations are currently justified.
