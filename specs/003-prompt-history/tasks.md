---

description: "Task list for Interactive Prompt History Recall"
---

# Tasks: Interactive Prompt History Recall

**Input**: Design documents from `/specs/003-prompt-history/`

**Prerequisites**: plan.md (required), spec.md (required for user stories), research.md, data-model.md, contracts/

**Tests**: Include focused REPL tests for each user story because the specification defines explicit interaction scenarios and independent validation criteria.

**Organization**: Tasks are grouped by user story so each increment can be implemented and validated in sequence against the shared REPL editor surface.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g. US1, US2, US3)
- Include exact file paths in descriptions

## Path Conventions

- Single Go binary/CLI: `cmd/`, `internal/`, optional `pkg/`
- Tests: package-local `*_test.go` files and optional `test/integration/`
- Do not add tasks for TypeScript, Python, or C++ work unless the constitution changes first

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Prepare the existing REPL code and test harness for escape-sequence-driven history work

- [X] T001 [P] Add reusable interactive escape-sequence input helpers and output assertions in internal/cli/repl_test.go
- [X] T002 [P] Add prompt-history fields and helper method stubs on REPL in internal/cli/repl.go

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core prompt-history infrastructure that MUST exist before any user story can be completed

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

- [X] T003 Implement the REPL-owned prompt history buffer, cursor state, and live-draft reset helpers in internal/cli/repl.go
- [X] T004 Extend the raw-mode reader to recognize `ESC [ A` and `ESC [ B` as history navigation input in internal/cli/repl.go
- [X] T005 Route interactive redraws through one visible-buffer update path so history state and slash suggestions stay synchronized in internal/cli/repl.go

**Checkpoint**: Prompt-history infrastructure is ready and user story behavior can be layered on top

---

## Phase 3: User Story 1 - Recall Earlier Inputs (Priority: P1) 🎯 MVP

**Goal**: Let the user move backward through previously submitted prompts with the up arrow

**Independent Test**: Launch the interactive CLI, submit multiple prompts, press `Up Arrow`, and confirm earlier prompts appear in reverse chronological order and clamp at the oldest entry

### Tests for User Story 1

- [X] T006 [US1] Add empty-history no-op, reverse-chronological recall, and oldest-entry clamp coverage in internal/cli/repl_test.go

### Implementation for User Story 1

- [X] T007 [US1] Record each submitted non-empty raw input line before parse and dispatch trimming in internal/cli/repl.go
- [X] T008 [US1] Load older saved entries on `Up Arrow` and clamp the history cursor at the oldest entry in internal/cli/repl.go
- [X] T009 [US1] Keep recalled visible text and slash suggestions aligned during backward history navigation in internal/cli/repl.go

**Checkpoint**: User Story 1 should recall earlier submitted prompts without breaking normal submission flow

---

## Phase 4: User Story 2 - Move Forward Through History Without Losing the Current Draft (Priority: P1)

**Goal**: Let the user move toward newer history entries and restore the draft that existed before navigation began

**Independent Test**: Type an unsent draft, enter history with `Up Arrow`, then press `Down Arrow` until the newest live state is reached and confirm the original draft returns unchanged

### Tests for User Story 2

- [X] T010 [US2] Add newer-entry traversal, newest-boundary no-op, and draft-restoration coverage in internal/cli/repl_test.go

### Implementation for User Story 2

- [X] T011 [US2] Preserve the pre-navigation draft exactly once when history navigation begins in internal/cli/repl.go
- [X] T012 [US2] Restore newer history entries and the preserved live draft on `Down Arrow` in internal/cli/repl.go
- [X] T013 [US2] Keep repeated `Down Arrow` input at the newest live state as a no-op in internal/cli/repl.go

**Checkpoint**: User Story 2 should move forward safely through history and restore the interrupted draft at the live boundary

---

## Phase 5: User Story 3 - Edit and Resubmit a Recalled Prompt (Priority: P2)

**Goal**: Let the user edit recalled text, submit the edited version, and keep the original history entry intact

**Independent Test**: Recall an older prompt, edit it, press Enter, and confirm the edited version submits while both original and edited lines remain available in history

### Tests for User Story 3

- [X] T014 [US3] Add edited-resubmission, duplicate-entry preservation, recalled slash-command, and long-line redraw coverage in internal/cli/repl_test.go

### Implementation for User Story 3

- [X] T015 [US3] Keep recalled visible buffers editable without mutating stored history entries in internal/cli/repl.go
- [X] T016 [US3] Append edited recalled submissions as new history entries while preserving earlier duplicate lines in internal/cli/repl.go
- [X] T017 [US3] Ensure recalled slash-command lines still use the existing local command execution path in internal/cli/repl.go

**Checkpoint**: User Story 3 should support editable recall and resubmission without altering stored history

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Finalize shared documentation and regression validation across the CLI surfaces touched by prompt history

- [X] T018 [P] Refresh the interactive keyboard behavior contract in specs/003-prompt-history/contracts/cli.md
- [X] T019 [P] Update the manual validation flow and expected outcomes in specs/003-prompt-history/quickstart.md
- [X] T020 Run the full regression suite from specs/003-prompt-history/quickstart.md against internal/cli/repl.go, internal/cli/repl_test.go, internal/app/session_runner_test.go, internal/session/session_test.go, and test/integration/slash_command_discovery_test.go

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies - can start immediately
- **Foundational (Phase 2)**: Depends on Setup completion - blocks all user stories
- **User Story 1 (Phase 3)**: Depends on Foundational completion - delivers the MVP recall path
- **User Story 2 (Phase 4)**: Depends on User Story 1 because forward navigation assumes backward recall is already working
- **User Story 3 (Phase 5)**: Depends on User Stories 1 and 2 because editable resubmission operates on recalled history state and restored live-draft behavior
- **Polish (Phase 6)**: Depends on all selected user stories being complete

### User Story Dependencies

- **US1**: First functional increment after the shared prompt-history infrastructure is ready
- **US2**: Builds directly on the recall state created for US1
- **US3**: Builds on recalled-line behavior from US1 and draft-boundary behavior from US2

### Within Each User Story

- Write the story tests first and confirm they fail before changing implementation
- Update REPL state handling in internal/cli/repl.go before adjusting any story-specific expectations
- Keep each story independently verifiable against its stated interactive test flow

### Parallel Opportunities

- T001 and T002 can run in parallel because they touch different files and prepare separate scaffolding
- T018 and T019 can run in parallel because they update different feature documents
- The story phases themselves should stay mostly serialized because `internal/cli/repl.go` and `internal/cli/repl_test.go` are the shared implementation surfaces

---

## Parallel Example: User Story 1

```text
No safe same-phase parallel task group inside US1: T006, T007, T008, and T009 all converge on internal/cli/repl.go or internal/cli/repl_test.go and should stay serialized.
```

## Parallel Example: User Story 2

```text
No safe same-phase parallel task group inside US2: the draft-preservation and down-arrow tasks all update the same REPL history state in internal/cli/repl.go.
```

## Parallel Example: User Story 3

```text
No safe same-phase parallel task group inside US3: editable recall, duplicate preservation, and slash-command resubmission all share the same REPL state and test file.
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup
2. Complete Phase 2: Foundational
3. Complete Phase 3: User Story 1
4. Stop and validate reverse-chronological recall behavior before expanding the feature

### Incremental Delivery

1. Finish Setup and Foundational work to stabilize the REPL history machinery
2. Deliver User Story 1 and validate backward recall
3. Deliver User Story 2 and validate forward navigation plus draft restoration
4. Deliver User Story 3 and validate editable resubmission and slash-command recall
5. Finish Polish tasks and run the full regression suite

### Parallel Team Strategy

1. One engineer can prepare T001 while another prepares T002
2. One engineer can refresh T018 while another refreshes T019 after implementation stabilizes
3. Keep the core story work single-threaded to avoid conflicts in internal/cli/repl.go and internal/cli/repl_test.go

---

## Notes

- [P] tasks are limited because the feature is intentionally constrained to the existing REPL implementation files
- Duplicate submitted prompts remain distinct history entries
- Non-interactive input behavior must remain unchanged
- Slash-command recall must keep using the existing local suggestion and dispatch path
