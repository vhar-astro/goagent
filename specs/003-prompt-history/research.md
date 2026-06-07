# Research: Interactive Prompt History Recall

## Decision: Keep prompt history inside the REPL

**Rationale**: The feature is about interactive terminal editing state, not
provider conversation state. The REPL already owns raw-mode input, redraw, and
slash suggestions, and slash commands never enter `session.Session`, so a
transcript-backed history would miss required recall cases.

**Alternatives considered**:

- Store history in `session.Session`: rejected because it mixes terminal-editor
  concerns with provider transcript state and would not capture submitted slash
  commands.
- Add a separate shared history service: rejected because the feature scope is
  one interactive launch and does not justify another abstraction layer.

## Decision: Record raw submitted lines on Enter before parse/dispatch trimming

**Rationale**: The spec requires recalled text to match what the user entered
  closely enough to inspect and edit it. Recording raw non-empty lines preserves
  duplicates, slash commands, and leading or trailing spaces while leaving the
  current parsing behavior unchanged at execution time.

**Alternatives considered**:

- Store only trimmed natural-language prompts: rejected because it would lose
  slash commands and alter the recalled visible text.
- Store only successful submissions after dispatch/provider work completes:
  rejected because it creates coupling to downstream execution and weakens the
  simple “user pressed Enter on a non-empty line” mental model.

## Decision: Handle only up/down arrow escape sequences in raw mode

**Rationale**: The current interactive reader already uses raw TTY mode and
full-line redraw. Supporting `ESC [ A` and `ESC [ B` is enough to satisfy the
feature without turning the editor into a full readline clone.

**Alternatives considered**:

- Add a readline-style dependency: rejected by the constitution and unnecessary
  for the requested scope.
- Implement a full cursor-editing command set: rejected because left/right
  movement, history search, and multi-line editing are out of scope.

## Decision: Preserve the pre-navigation draft separately and restore it only at the newest boundary

**Rationale**: The spec explicitly requires preserving the unfinished draft that
existed before history navigation started. Keeping that draft in a dedicated
field allows down-arrow navigation to restore it after the user moves forward
past the newest saved entry, even if the user previously inspected or edited an
older prompt.

**Alternatives considered**:

- Overwrite the draft with the first recalled entry: rejected because it would
  violate the draft-restoration requirement.
- Exit history mode permanently on first edit: rejected because it makes the
  original draft harder to recover and adds surprising state transitions.

## Decision: Reuse the existing redraw path for recalled slash commands and keep non-interactive input unchanged

**Rationale**: `renderInteractiveLine` already redraws the full prompt and slash
suggestions based on the visible line. Using the same path keeps recalled
`/...` input consistent with current slash discovery while leaving the
non-interactive line-buffered path untouched.

**Alternatives considered**:

- Add a separate history-specific render path: rejected because it duplicates
  existing terminal output logic.
- Attempt to emulate arrow-key history in non-interactive mode: rejected
  because the spec requires unchanged fallback behavior when arrow input is not
  available.
