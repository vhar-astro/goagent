# Research: Slash Command Discovery and Per-Command Approval

## Decision: Centralize slash-command metadata in one catalog

**Rationale**: The current CLI stores usage strings in `slashCommandUsages`
while parsing and dispatch live in separate switch paths. Feature 002 needs
names, usage lines, and short descriptions for filtered suggestions, so one
catalog keeps parsing, error messages, and suggestion rendering aligned.

**Alternatives considered**:

- Keep parallel slices/maps plus manual descriptions: rejected because it
  duplicates command metadata and invites drift.
- Generate help text directly from handlers: rejected because handlers do not
  carry user-facing description data today.

## Decision: Keep slash suggestions informational, prefix-filtered, and local

**Rationale**: The feature requires users to see available slash commands while
typing `/`, not a full terminal menu system. Showing filtered command names plus
short descriptions satisfies discoverability with less input complexity than
selection UIs, fuzzy search, or provider-assisted help.

**Alternatives considered**:

- Arrow-key or menu-based selection: rejected because it adds input-state
  complexity without being required by the spec.
- Documentation-only discovery: rejected because the feature explicitly wants
  in-terminal guidance.

## Decision: Stay stdlib-only for terminal interaction

**Rationale**: The constitution prefers minimal dependencies, and this feature
  is narrow enough to justify a small repository-owned terminal session layer in
  `internal/cli`. The live app already owns stdin/stdout directly, so a
  lightweight TTY-aware reader/redraw path is simpler than adopting a general
  line-editing framework.

**Alternatives considered**:

- Add an external terminal/readline dependency: rejected because the current
  scope does not justify expanding `go.mod`.
- Leave the REPL line-buffered and show suggestions only after Enter: rejected
  because it does not satisfy the requirement to display commands while typing
  `/`.

## Decision: Scope shell approval to an exact command signature

**Rationale**: The current `ApprovedCapabilities` model is too broad for the
feature. The safest narrow reuse model is a session-scoped signature built from
the raw shell command string plus the resolved working directory, because the
same command text in a different directory can have a different effect.

**Alternatives considered**:

- Capability-wide approval (`/approve shell`): rejected because it over-approves
  future commands.
- Base executable-only approval (all `rm` after one `rm`): rejected because the
  user explicitly rejected that scope.
- Command string only without workdir: rejected because identical text can act
  on different files in different directories.

## Decision: Prompt and resume inline in the current turn

**Rationale**: The spec wants the app to ask `Approve running <command>?` and
continue without forcing the user to retry the original task. The simplest path
is a local approval callback that pauses the blocked shell call, prompts the
user, and then resumes or denies before more provider interaction occurs.

**Alternatives considered**:

- Emit an approval-required tool result and ask the user to rerun the task:
  rejected because it preserves the current broken flow.
- Add a new pending-approval slash command workflow: rejected because it adds
  control-surface complexity for a feature that should feel automatic.
