# Feature Specification: Interactive Prompt History Recall

**Feature Branch**: `003-prompt-history`

**Created**: 2026-06-07

**Status**: Draft

**Input**: User description: "when user presses arrow up / down, user is able to look up previous prompts, like in bash"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Recall Earlier Inputs (Priority: P1)

A user working in the interactive terminal can press the up arrow to revisit
earlier prompts they already entered, instead of retyping them from scratch.

**Why this priority**: Prompt reuse is most valuable when a user wants to rerun
or revise a recent request quickly. Without recall, repetitive terminal work is
slower and more error-prone.

**Independent Test**: Launch the interactive CLI, submit several prompts, press
the up arrow, and confirm that earlier submitted inputs appear one by one in
reverse chronological order without retyping.

**Acceptance Scenarios**:

1. **Given** a user has already submitted at least one prompt in the current
   interactive session, **When** they press the up arrow at the terminal input,
   **Then** the most recent previously submitted prompt is shown in the input
   area for review or reuse.
2. **Given** a user has already recalled one earlier prompt, **When** they
   press the up arrow again, **Then** the next older submitted prompt is shown.
3. **Given** a user is already viewing the oldest available prompt in the
   current session, **When** they press the up arrow again, **Then** the app
   keeps that oldest prompt visible and does not move past the beginning of the
   available history.

---

### User Story 2 - Move Forward Through History Without Losing the Current Draft (Priority: P1)

A user can press the down arrow to move toward newer history entries and return
to the draft they were typing before history navigation began.

**Why this priority**: History recall is incomplete if the user cannot move
back toward newer entries or recover their unfinished current input safely.

**Independent Test**: Type a new draft, press the up arrow to enter history,
then press the down arrow until reaching the newest entry and confirm the
original unfinished draft is restored.

**Acceptance Scenarios**:

1. **Given** a user is viewing an older prompt from history, **When** they
   press the down arrow, **Then** the next newer prompt is shown.
2. **Given** a user started history navigation while a new unsent draft was
   present, **When** they press the down arrow past the newest saved history
   entry, **Then** the original unsent draft is restored in the input area.
3. **Given** a user is already at the newest available state for the current
   input, **When** they press the down arrow again, **Then** the visible input
   remains unchanged.

---

### User Story 3 - Edit and Resubmit a Recalled Prompt (Priority: P2)

A user can treat a recalled prompt like normal input, edit it if needed, and
submit the revised text as a new prompt.

**Why this priority**: The main value of prompt history is not only replaying
past inputs but also using them as a starting point for small follow-up changes.

**Independent Test**: Recall an earlier prompt, modify part of the text, press
Enter, and confirm the edited version is submitted while the original history
entry remains available as prior context.

**Acceptance Scenarios**:

1. **Given** a recalled prompt is visible in the input area, **When** the user
   edits that text before pressing Enter, **Then** the updated text is what the
   app submits.
2. **Given** a user submits an edited recalled prompt, **When** they later
   navigate history again, **Then** both the earlier submitted prompt and the
   newly submitted revision are available in session history as separate
   entries.
3. **Given** a recalled input begins with `/`, **When** the user recalls it
   into the input area, **Then** it behaves like ordinary current input for the
   purpose of local command review before submission.

### Edge Cases

- What happens when the user presses the up arrow before any prompt has been
  submitted in the current interactive session?
- How does the app behave when the session history contains duplicate prompt
  text entered at different times?
- What happens when the user recalls a very long prompt that approaches or
  exceeds the terminal width?
- How does the app behave if the user starts with a partially typed draft,
  enters history navigation, and then edits the recalled text before returning
  to the draft?
- What happens when history recall is attempted in non-interactive input mode?
- How does the app behave if a recalled slash command would normally show local
  suggestions while it is visible in the input area?

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST record each submitted prompt entered during an
  interactive terminal session so it can be recalled later in that same
  session.
- **FR-002**: System MUST allow the user to press the up arrow to move backward
  through submitted prompts in reverse chronological order within the current
  interactive session.
- **FR-003**: System MUST allow the user to press the down arrow to move
  forward through the available prompt history after they have moved backward.
- **FR-004**: System MUST preserve the user's current unsent draft when history
  navigation begins and MUST restore that draft when the user navigates forward
  past the newest saved history entry.
- **FR-005**: System MUST display the recalled prompt directly in the active
  input area before submission so the user can inspect it.
- **FR-006**: System MUST allow a recalled prompt to be edited before the user
  submits it.
- **FR-007**: System MUST submit the visible edited text, not the original
  stored history text, when the user presses Enter after editing a recalled
  prompt.
- **FR-008**: System MUST keep the oldest available prompt visible when the
  user presses the up arrow at the beginning of available history.
- **FR-009**: System MUST keep the newest available state visible when the user
  presses the down arrow after returning to the newest available entry or
  restored draft.
- **FR-010**: System MUST treat each submitted prompt as its own history entry,
  even when two submitted prompts contain identical text.
- **FR-011**: System MUST scope prompt history to the current interactive
  session and MUST NOT require earlier sessions to be available after the app
  exits and restarts.
- **FR-012**: System MUST leave non-interactive input behavior unchanged when
  arrow-key history is not available.
- **FR-013**: System MUST preserve existing input behavior for slash commands,
  approvals, and ordinary prompt submission while adding history navigation.
- **FR-014**: System MUST render recalled prompt text accurately enough that
  the user can review and continue editing it before submission.

## Constitution Alignment *(mandatory)*

- **CA-001 Simplicity**: The simplest viable design adds session-scoped prompt
  recall to the existing interactive terminal input flow. Persistent shell-like
  profile storage, cross-session history files, and larger terminal frameworks
  are intentionally out of scope.
- **CA-002 Dependencies**: Standard library only. Planning should avoid adding
  external terminal or readline dependencies unless this behavior is proven
  impossible within the existing repository constraints.
- **CA-003 Language Boundary**: The feature remains fully inside the Go CLI and
  does not require any TypeScript, Python, or C++ implementation.
- **CA-004 Current Docs**: N/A. No external library, SDK, API, CLI tool, or
  cloud service documentation is required for this feature specification.

### Key Entities *(include if feature involves data)*

- **Prompt History Entry**: One previously submitted line of interactive user
  input that can be recalled later in the same session.
- **History Cursor**: The user's current position while moving backward or
  forward through available prompt history.
- **Draft Input**: The unsent text the user had in the input area before they
  began navigating through history.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: In validation runs of the interactive CLI, 100% of previously
  submitted prompts in the current session can be recalled in reverse
  chronological order with repeated up-arrow input.
- **SC-002**: In validation runs where a user starts history navigation from a
  partially typed draft, 100% of those runs restore the original draft after
  the user navigates back down past the newest saved history entry.
- **SC-003**: In validation runs, editing a recalled prompt and submitting it
  succeeds without retyping the full original prompt in at least 95% of manual
  task repetitions.
- **SC-004**: In validation runs, pressing arrow keys outside available history
  never causes the visible input to jump to an unrelated prompt or clear the
  current visible input unexpectedly.

## Assumptions

- Prompt history applies to the existing interactive terminal session only.
- Only prompts that were actually submitted are stored in history; abandoned
  drafts are not stored as reusable history entries.
- The current single-line terminal input model remains in scope; this feature
  improves recall and editing within that model rather than introducing
  multi-line prompt composition.
- The same history behavior applies to previously submitted ordinary prompts
  and previously submitted slash-command inputs during an interactive session.
