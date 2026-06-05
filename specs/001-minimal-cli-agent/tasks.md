# Tasks: Minimal CLI Coding Agent

**Input**: Design documents from `/specs/001-minimal-cli-agent/`

**Prerequisites**: plan.md (required), spec.md (required for user stories), research.md, data-model.md, contracts/

**Tests**: Include unit and integration tests because the specification defines mandatory independent test scenarios and the plan requires `go test ./...` plus integration coverage under `test/integration/`.

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

**Purpose**: Establish the Go CLI skeleton and baseline test layout for the feature.

- [ ] T001 Initialize the Go CLI module and launch entrypoint in `go.mod` and `cmd/goagent/main.go`
- [ ] T002 [P] Create the planned internal package skeleton in `internal/app/app.go`, `internal/cli/repl.go`, `internal/config/config.go`, `internal/modules/manifest.go`, `internal/provider/types.go`, `internal/session/session.go`, and `internal/tools/registry.go`
- [ ] T003 [P] Create the baseline test layout in `internal/config/config_test.go` and `test/integration/base_session_test.go`

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Build the shared runtime, validation, and command-routing pieces that every story depends on.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

- [ ] T004 Implement JSON config loading, default-path resolution, and provider profile validation in `internal/config/config.go`
- [ ] T005 [P] Implement session state, conversation message models, and approval tracking in `internal/session/session.go`
- [ ] T006 [P] Implement normalized provider request, response, and stream event types in `internal/provider/types.go`
- [ ] T007 [P] Implement built-in tool registration, capability buckets, and schema uniqueness checks in `internal/tools/registry.go`
- [ ] T008 Implement workspace boundary checks, timeout settings, and tool output truncation helpers in `internal/app/runtime.go`
- [ ] T009 Implement CLI flag parsing and slash-command dispatch scaffolding in `internal/cli/commands.go`
- [ ] T010 Implement the application bootstrap and empty session loop wiring in `internal/app/app.go` and `cmd/goagent/main.go`
- [ ] T011 [P] Add foundational unit coverage for config, session approvals, and tool registry behavior in `internal/config/config_test.go`, `internal/session/session_test.go`, and `internal/tools/registry_test.go`

**Checkpoint**: Foundation ready; user story work can proceed with shared runtime contracts in place.

---

## Phase 3: User Story 1 - Run the Minimal Agent (Priority: P1) 🎯 MVP

**Goal**: Deliver one interactive CLI session that can read files, edit files, run shell commands, and fetch explicit URLs with minimal default tool surface.

**Independent Test**: Start the agent in a temporary workspace, approve risky capabilities, ask it to inspect a file, edit a file, run a command, and fetch a URL, then verify all actions stay inside one session and the workspace boundary.

### Tests for User Story 1

- [ ] T012 [P] [US1] Add the base session integration scenario with a streaming stub provider in `test/integration/base_session_test.go`
- [ ] T013 [P] [US1] Add built-in tool validation and workspace-boundary tests in `internal/tools/files_test.go` and `internal/tools/shell_test.go`

### Implementation for User Story 1

- [ ] T014 [P] [US1] Implement workspace-scoped file read and file write tools in `internal/tools/files.go`
- [ ] T015 [P] [US1] Implement workspace-scoped shell execution and direct URL fetch tools in `internal/tools/shell.go` and `internal/tools/web.go`
- [ ] T016 [US1] Implement the provider HTTP client and streaming chunk parser in `internal/provider/client.go`
- [ ] T017 [US1] Implement the conversation loop, tool-call handling, and truncated tool-result reinjection in `internal/app/session_runner.go`
- [ ] T018 [US1] Implement interactive stdin/stdout handling and streamed assistant rendering in `internal/cli/repl.go`
- [ ] T019 [US1] Enforce approval prompts and structured tool execution summaries in `internal/app/executor.go`
- [ ] T020 [US1] Wire the default built-in tool set and base agent launch flow in `internal/app/app.go` and `cmd/goagent/main.go`

**Checkpoint**: User Story 1 should now run as a complete minimal coding-agent session.

---

## Phase 4: User Story 2 - Extend the Agent with Modules (Priority: P2)

**Goal**: Allow users to attach and detach local exec modules without changing the base agent workflow.

**Independent Test**: Start the agent, approve module access, attach a stub local module from disk, confirm its tools appear and can be invoked, then detach it and confirm the base tool set remains active.

### Tests for User Story 2

- [ ] T021 [P] [US2] Add module manifest parsing and validation tests in `internal/modules/manifest_test.go`
- [ ] T022 [P] [US2] Add attach/detach integration coverage with a stub module process in `test/integration/module_lifecycle_test.go`

### Implementation for User Story 2

- [ ] T023 [P] [US2] Implement module manifest loading and requested-capability validation in `internal/modules/manifest.go`
- [ ] T024 [P] [US2] Implement the NDJSON module protocol client and process lifecycle handling in `internal/modules/process.go`
- [ ] T025 [US2] Implement the attached-module registry and duplicate-tool detection in `internal/modules/registry.go`
- [ ] T026 [US2] Implement `/attach` and `/detach` command handlers in `internal/cli/module_commands.go`
- [ ] T027 [US2] Route module tool calls and detach cleanup through the session executor in `internal/app/module_executor.go`

**Checkpoint**: User Story 2 should now extend and shrink the active tool surface at runtime without restarting the session.

---

## Phase 5: User Story 3 - Choose an LLM Provider Profile (Priority: P3)

**Goal**: Support the default Chutes profile and an optional OpenRouter profile while preserving the same CLI workflow.

**Independent Test**: Configure both provider profiles, launch once with the default provider and once with `--provider openrouter`, then verify the same base session flow works and configuration failures are reported cleanly.

### Tests for User Story 3

- [ ] T028 [P] [US3] Add provider profile selection and override tests in `internal/config/provider_test.go`
- [ ] T029 [P] [US3] Add launch-time provider override integration coverage in `test/integration/provider_override_test.go`

### Implementation for User Story 3

- [ ] T030 [P] [US3] Implement Chutes and OpenRouter profile construction with provider-specific headers in `internal/provider/profiles.go`
- [ ] T031 [US3] Implement launch-time provider override and runtime `/provider` command handling in `internal/cli/provider_commands.go` and `internal/app/provider_switch.go`
- [ ] T032 [US3] Surface provider configuration, unsupported-provider, and unavailable-capability errors in `internal/cli/errors.go` and `internal/app/provider_switch.go`

**Checkpoint**: User Story 3 should now support both configured providers through the same CLI interaction model.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Final quality, docs, and end-to-end validation across all stories.

- [ ] T033 [P] Document CLI usage, approvals, and module authoring expectations in `README.md`
- [ ] T034 Normalize shared runtime and CLI error messages in `internal/app/errors.go` and `internal/cli/errors.go`
- [ ] T035 Run the quickstart flow against the implemented binary and update the verified commands in `specs/001-minimal-cli-agent/quickstart.md`

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies; can start immediately.
- **Foundational (Phase 2)**: Depends on Setup; blocks all user story work.
- **User Story 1 (Phase 3)**: Depends on Foundational; establishes the first usable end-to-end session and is the MVP.
- **User Story 2 (Phase 4)**: Depends on Foundational and reuses the active session/runtime loop from User Story 1.
- **User Story 3 (Phase 5)**: Depends on Foundational and reuses the provider/session flow implemented for User Story 1.
- **Polish (Phase 6)**: Depends on the user stories selected for delivery.

### User Story Dependencies

- **US1 (P1)**: No user-story dependency once Foundational is complete.
- **US2 (P2)**: Builds on the base interactive runtime delivered by US1 for attach/detach validation.
- **US3 (P3)**: Builds on the base interactive runtime delivered by US1 for provider-switch validation.

### Within Each User Story

- Tests should be written before the corresponding implementation tasks and must fail before implementation is considered complete.
- Tool and model primitives should land before session wiring that depends on them.
- Session wiring should land before final CLI integration for each story.

### Parallel Opportunities

- T002 and T003 can run in parallel after T001.
- T005, T006, T007, and T011 can run in parallel once the package skeleton exists.
- T012 and T013 can run in parallel for US1; T014 and T015 can also run in parallel after the foundational tool registry is ready.
- T021 and T022 can run in parallel for US2; T023 and T024 can also run in parallel.
- T028 and T029 can run in parallel for US3; T030 can proceed in parallel with those tests.

---

## Parallel Example: User Story 1

```bash
# Launch the User Story 1 test tasks together:
Task: "Add the base session integration scenario with a streaming stub provider in test/integration/base_session_test.go"
Task: "Add built-in tool validation and workspace-boundary tests in internal/tools/files_test.go and internal/tools/shell_test.go"

# Launch the User Story 1 tool implementations together:
Task: "Implement workspace-scoped file read and file write tools in internal/tools/files.go"
Task: "Implement workspace-scoped shell execution and direct URL fetch tools in internal/tools/shell.go and internal/tools/web.go"
```

---

## Parallel Example: User Story 2

```bash
# Launch the User Story 2 test tasks together:
Task: "Add module manifest parsing and validation tests in internal/modules/manifest_test.go"
Task: "Add attach/detach integration coverage with a stub module process in test/integration/module_lifecycle_test.go"

# Launch the User Story 2 module primitives together:
Task: "Implement module manifest loading and requested-capability validation in internal/modules/manifest.go"
Task: "Implement the NDJSON module protocol client and process lifecycle handling in internal/modules/process.go"
```

---

## Parallel Example: User Story 3

```bash
# Launch the User Story 3 tests together:
Task: "Add provider profile selection and override tests in internal/config/provider_test.go"
Task: "Add launch-time provider override integration coverage in test/integration/provider_override_test.go"

# Launch the provider-profile implementation in parallel with test authoring:
Task: "Implement Chutes and OpenRouter profile construction with provider-specific headers in internal/provider/profiles.go"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup.
2. Complete Phase 2: Foundational.
3. Complete Phase 3: User Story 1.
4. Validate the base session flow with `go test ./...` and the US1 integration scenario before expanding scope.

### Incremental Delivery

1. Finish Setup and Foundational to lock the shared runtime contracts.
2. Deliver US1 as the first usable CLI agent.
3. Add US2 to enable attachable local modules without changing the base workflow.
4. Add US3 to support the alternate provider profile and provider-selection controls.
5. Finish with Phase 6 documentation and quickstart validation.

### Parallel Team Strategy

1. One developer can own CLI/session wiring while another owns tool primitives during US1.
2. After US1 stabilizes, one developer can own module protocol work while another adds module CLI and integration coverage.
3. Provider-profile support can proceed in parallel with documentation and integration test expansion once the shared runtime is stable.

---

## Notes

- [P] tasks target different files and avoid dependencies on incomplete same-file work.
- Story labels map each task back to a specific user story for traceability.
- The recommended MVP scope is Phase 1 through Phase 3 only.
- Avoid expanding beyond the four built-in tools, local modules, and two provider profiles defined in the plan.
