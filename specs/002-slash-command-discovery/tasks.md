# Tasks: Slash Command Discovery and Per-Command Approval

**Input**: Design documents from `/specs/002-slash-command-discovery/`

**Prerequisites**: plan.md (required), spec.md (required for user stories), research.md, data-model.md, contracts/

**Tests**: Include unit and integration tests because the specification defines mandatory independent test scenarios and the plan requires `go test ./...` coverage in `internal/cli`, `internal/app`, `internal/session`, and scripted REPL/session flows.

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

## Path Conventions

- **Single Go binary/CLI**: `cmd/`, `internal/`, `test/integration/`
- **Tests**: package-local `*_test.go` files and `test/integration/`
- Do not add tasks for TypeScript, Python, or C++ work unless the constitution has been amended first.

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Create the feature-specific test entry points for interactive CLI and approval flows.

- [ ] T001 Create interactive CLI test scaffolds for typed slash input and rendered local output in `internal/cli/repl_test.go` and `test/integration/slash_command_discovery_test.go`
- [ ] T002 [P] Create shell-approval test scaffolds for executor and session-runner flows in `internal/app/executor_test.go`, `internal/app/session_runner_test.go`, and `test/integration/shell_approval_prompt_test.go`

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Build the shared REPL and session plumbing that every story depends on.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

- [ ] T003 Refactor interactive input/output hooks for redraws, local prompts, and non-TTY fallback in `internal/cli/repl.go`
- [ ] T004 [P] Add command-specific shell approval records and pending-request helpers in `internal/session/session.go`
- [ ] T005 Add baseline coverage for REPL hook behavior and pending approval lifecycle in `internal/cli/repl_test.go` and `internal/session/session_test.go`

**Checkpoint**: Shared terminal and session primitives are ready for story work.

---

## Phase 3: User Story 1 - Discover Available Slash Commands (Priority: P1) 🎯 MVP

**Goal**: Show visible in-terminal slash-command suggestions with short descriptions as soon as the user types `/`.

**Independent Test**: Launch the app, type `/`, confirm visible suggestions are enough to choose a command without external docs, and verify narrowing to a prefix such as `/to` filters the list.

### Tests for User Story 1

- [ ] T006 [P] [US1] Add slash-command catalog parsing and prefix-filtering tests in `internal/cli/commands_test.go`
- [ ] T007 [P] [US1] Add interactive suggestion rendering and non-TTY fallback tests in `internal/cli/repl_test.go` and `test/integration/slash_command_discovery_test.go`

### Implementation for User Story 1

- [ ] T008 [US1] Replace static slash-command usage lists with catalog metadata, descriptions, and availability helpers in `internal/cli/commands.go`
- [ ] T009 [US1] Render, update, and clear prefix-filtered slash suggestions during interactive input in `internal/cli/repl.go`
- [ ] T010 [US1] Wire catalog-backed slash discovery into the live session bootstrap in `internal/app/app.go`

**Checkpoint**: User Story 1 should now make slash commands discoverable in the terminal without changing non-slash prompt behavior.

---

## Phase 4: User Story 2 - Approve a Specific Shell Command In Context (Priority: P1)

**Goal**: Replace `/approve shell` retry flow with an inline approval prompt that can approve or deny the exact pending shell command and continue in the same turn.

**Independent Test**: Ask the agent to run a shell command that needs approval and confirm the exact command is shown, `y/yes` resumes execution immediately, and denial leaves the session running without re-entering the original task.

### Tests for User Story 2

- [ ] T011 [P] [US2] Add executor tests for approval-required `run_shell` calls and denial outcomes in `internal/app/executor_test.go`
- [ ] T012 [P] [US2] Add session-runner and integration coverage for inline shell approval and same-turn resume in `internal/app/session_runner_test.go` and `test/integration/shell_approval_prompt_test.go`

### Implementation for User Story 2

- [ ] T013 [US2] Move shell approval checks into decoded `run_shell` execution and emit pending approval requests in `internal/app/executor.go`
- [ ] T014 [US2] Implement local `Approve running "<command>" in <workdir>? [y/N]` prompting and resumed execution in `internal/app/session_runner.go` and `internal/cli/repl.go`
- [ ] T015 [US2] Remove shell from `/approve` parsing, usage text, and handler messaging in `internal/cli/commands.go` and `internal/app/app.go`

**Checkpoint**: User Story 2 should now prompt for one pending shell command inline and continue without a manual retry step.

---

## Phase 5: User Story 3 - Keep Approvals Narrow to the Command Requested (Priority: P2)

**Goal**: Reuse approval only for the exact same shell command text in the same resolved workdir, while forcing fresh approval for any changed command or arguments.

**Independent Test**: Approve one shell command, rerun the exact same command to confirm no second prompt appears, then change the arguments or workdir and verify a fresh approval is required.

### Tests for User Story 3

- [ ] T016 [P] [US3] Add session tests for exact-command approval reuse, changed arguments, and changed workdir behavior in `internal/session/session_test.go`
- [ ] T017 [P] [US3] Add executor tests for exact approval-signature matching and prompt reuse suppression in `internal/app/executor_test.go`
- [ ] T018 [P] [US3] Add integration coverage for approval reuse versus fresh prompts in `test/integration/shell_approval_cache_test.go`

### Implementation for User Story 3

- [ ] T019 [US3] Persist approved exact shell command signatures and pending-request transitions in `internal/session/session.go`
- [ ] T020 [US3] Derive shell approval signatures from command text plus resolved workdir and reuse only exact matches in `internal/app/executor.go` and `internal/tools/shell.go`
- [ ] T021 [US3] Print explicit approve, deny, and reused-approval outcomes while clearing pending state in `internal/app/session_runner.go` and `internal/app/app.go`

**Checkpoint**: User Story 3 should now enforce narrow approval scope without regressing the inline approval flow.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Finalize shared messaging, docs, and end-to-end validation across all stories.

- [ ] T022 [P] Align CLI contract text and quickstart examples with the final slash discovery and inline shell approval behavior in `specs/002-slash-command-discovery/contracts/cli.md` and `specs/002-slash-command-discovery/quickstart.md`
- [ ] T023 Normalize approval, denial, and unknown-command user-facing messages across `internal/cli/commands.go`, `internal/app/app.go`, and `internal/app/session_runner.go`
- [ ] T024 Validate the end-to-end feature scenarios with `go test ./...` and update verified behavior notes in `specs/002-slash-command-discovery/contracts/cli.md` and `specs/002-slash-command-discovery/quickstart.md`

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies; can start immediately.
- **Foundational (Phase 2)**: Depends on Setup; blocks all user story work.
- **User Story 1 (Phase 3)**: Depends on Foundational; delivers the first user-visible improvement and is the MVP.
- **User Story 2 (Phase 4)**: Depends on Foundational; reuses the shared REPL/session hooks added in Phase 2.
- **User Story 3 (Phase 5)**: Depends on User Story 2 because it narrows and reuses the inline shell approval flow introduced there.
- **Polish (Phase 6)**: Depends on the user stories selected for delivery.

### User Story Dependencies

- **US1 (P1)**: No dependency on other user stories once Foundational is complete.
- **US2 (P1)**: No product dependency on US1, but it shares `internal/cli/repl.go`, so merge the foundational REPL refactor before starting inline approval work.
- **US3 (P2)**: Depends on US2 because exact-command reuse extends the inline shell approval path rather than introducing a separate workflow.

### Within Each User Story

- Tests should be written before the corresponding implementation tasks and must fail before implementation is considered complete.
- Slash-command catalog behavior should land before interactive rendering that depends on it.
- Pending approval state should exist before executor and session-runner resume logic depends on it.
- Exact-command signature storage should land before approval reuse and outcome messaging are finalized.

### Parallel Opportunities

- T001 and T002 can run in parallel after tasks start.
- T003 and T004 can run in parallel once the setup scaffolds exist.
- T006 and T007 can run in parallel for US1.
- T011 and T012 can run in parallel for US2.
- T016, T017, and T018 can run in parallel for US3.
- T022 can proceed in parallel with late-stage verification once the final CLI behavior is stable.

---

## Parallel Example: User Story 1

```bash
# Launch the User Story 1 tests together:
Task: "Add slash-command catalog parsing and prefix-filtering tests in internal/cli/commands_test.go"
Task: "Add interactive suggestion rendering and non-TTY fallback tests in internal/cli/repl_test.go and test/integration/slash_command_discovery_test.go"
```

---

## Parallel Example: User Story 2

```bash
# Launch the User Story 2 tests together:
Task: "Add executor tests for approval-required run_shell calls and denial outcomes in internal/app/executor_test.go"
Task: "Add session-runner and integration coverage for inline shell approval and same-turn resume in internal/app/session_runner_test.go and test/integration/shell_approval_prompt_test.go"
```

---

## Parallel Example: User Story 3

```bash
# Launch the User Story 3 tests together:
Task: "Add session tests for exact-command approval reuse, changed arguments, and changed workdir behavior in internal/session/session_test.go"
Task: "Add executor tests for exact approval-signature matching and prompt reuse suppression in internal/app/executor_test.go"
Task: "Add integration coverage for approval reuse versus fresh prompts in test/integration/shell_approval_cache_test.go"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup.
2. Complete Phase 2: Foundational.
3. Complete Phase 3: User Story 1.
4. Validate slash discovery independently before starting approval-flow changes.

### Incremental Delivery

1. Finish Setup and Foundational to lock the shared terminal and session contracts.
2. Deliver US1 so slash commands become discoverable at the prompt.
3. Deliver US2 to replace blanket shell approval with inline exact-command prompting.
4. Deliver US3 to narrow reuse to exact command signatures only.
5. Finish with Phase 6 messaging, docs, and verification updates.

### Parallel Team Strategy

1. One developer can own REPL interaction work while another builds session approval state and tests during Setup and Foundational.
2. After Foundational, one developer can author US1 catalog/rendering tests while another prepares the US2 approval-flow tests.
3. Once US2 is stable, US3 cache/reuse tests can proceed in parallel with documentation cleanup.

---

## Notes

- [P] tasks target different files and avoid dependencies on incomplete same-file work.
- Story labels map each task back to a specific user story for traceability.
- The recommended MVP scope is Phase 1 through Phase 3 only.
- Keep the implementation Go-only and stdlib-only, matching the plan and constitution.
