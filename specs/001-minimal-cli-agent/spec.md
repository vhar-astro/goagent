# Feature Specification: Minimal CLI Coding Agent

**Feature Branch**: `001-minimal-cli-agent`

**Created**: 2026-06-05

**Status**: Draft

**Input**: User description: "a minimalistic interactive CLI AI coding agent,
with minimal and flexible system prompt, minimal default set of tools, minimal
token consumption, simple modular architecture - user can create and attach
modules to the basic minimal agent. An agent is able to read files, edit files,
run shell commands and do a web fetch. Other tools and features can be created
later, either here or by this agent itself. It uses chutes.ai as a basic LLM
provider, optionally supports openrouter.ai, other providers to be done per
future feature requests"

## Clarifications

### Session 2026-06-05

- Q: How should runtime approvals work for risky actions? → A: Approve each
  risky capability once per session, then allow repeated use in that session.
- Q: How should session lifecycle work across launches? → A: Each launch starts
  a fresh session with no persisted conversation state.
- Q: How should provider selection work at launch? → A: Use a configured
  default provider, with an optional CLI override for a single launch.
- Q: Where can modules be attached from in the first release? → A: Users attach
  modules only from the local filesystem in the first release.
- Q: What defines a valid module for attachment? → A: Each module declares a
  minimal manifest with name, version, and requested capabilities.
- Q: What is the first-release execution boundary for files and commands? → A:
  File operations and shell commands are limited to the current workspace.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Run the Minimal Agent (Priority: P1)

A user starts a lightweight coding agent from the command line, gives it a
coding task, and works with it in one interactive session using only the core
capabilities needed for everyday repository work.

**Why this priority**: The base interactive agent is the product's core value.
Without it, modules and provider options have nothing to extend.

**Independent Test**: Can be fully tested by starting the agent in a local
workspace, asking it to inspect files, make a simple edit, run a command, and
retrieve supporting information from the web, all within one session.

**Acceptance Scenarios**:

1. **Given** a user starts the base agent with access to a workspace, **When**
   they ask it to inspect and modify project files, **Then** the agent completes
   the task through the same CLI session using its built-in capabilities inside
   the current workspace boundary.
2. **Given** a user wants a minimal default experience, **When** the agent
   starts in base mode, **Then** only the core capabilities are active and no
   optional modules are loaded automatically.

---

### User Story 2 - Extend the Agent with Modules (Priority: P2)

A user adds or removes optional modules so the base agent can gain new behavior
without losing its simple default shape.

**Why this priority**: Extensibility is the main way this product grows while
keeping the default agent small and low-overhead.

**Independent Test**: Can be fully tested by attaching a module, observing the
new behavior in the running agent, then detaching the module and confirming the
agent returns to its base behavior.

**Acceptance Scenarios**:

1. **Given** a user has an optional module available, **When** they attach it to
   the base agent from the local filesystem, **Then** the module becomes
   available without replacing the core agent workflow.
2. **Given** a user no longer wants an added module, **When** they detach it,
   **Then** the base agent continues to work with only its default behavior.

---

### User Story 3 - Choose an LLM Provider Profile (Priority: P3)

A user runs the same CLI workflow with the default provider profile or an
optional alternate provider profile, while keeping future provider expansion out
of scope for this first release.

**Why this priority**: Provider flexibility matters, but it builds on the base
agent rather than defining it.

**Independent Test**: Can be fully tested by configuring the default provider,
running a task, then switching to the alternate provider profile and confirming
the same task flow still works.

**Acceptance Scenarios**:

1. **Given** the default provider profile is configured, **When** a user starts
   a session, **Then** the agent runs through that provider without requiring a
   different CLI workflow.
2. **Given** an alternate provider profile is configured, **When** a user
   selects it through a launch-specific override, **Then** the agent can
   continue to accept the same kinds of tasks without redefining the user
   interaction model.

### Edge Cases

- What happens when the agent is started without valid provider credentials?
- How does the system handle a requested file edit when the target file does not
  exist or cannot be changed?
- What happens when a shell command fails, hangs, or returns output larger than
  the user can reasonably review in one step?
- What happens when a new session starts after risky-capability approvals were
  granted in an earlier session?
- How does the system respond when a module is incompatible with the current
  base agent version or duplicates an already attached capability?
- What happens when web retrieval is requested but network access is unavailable?
- What happens when a user supplies a launch-time provider override that is not
  configured or not supported?
- What happens when a requested local module path is missing, unreadable, or not
  recognized as a valid module?
- What happens when a requested file path or shell command would operate outside
  the current workspace boundary?

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST provide an interactive CLI session where a user can
  issue natural-language coding requests and receive step-by-step results in the
  same session.
- **FR-002**: System MUST start in a minimal default mode that exposes only the
  core capabilities required for repository work.
- **FR-003**: System MUST allow the user to have the agent read local files that
  are relevant to the current task, limited to the current workspace in the
  first release.
- **FR-004**: System MUST allow the user to have the agent create or modify
  local files that are relevant to the current task, limited to the current
  workspace in the first release.
- **FR-005**: System MUST allow the user to have the agent run shell commands
  relevant to the current task, limited to the current workspace in the first
  release, and present the results back in the session.
- **FR-006**: System MUST allow the user to have the agent retrieve web content
  relevant to the current task.
- **FR-006a**: System MUST allow local file reads by default while requiring
  explicit approval for risky capabilities, with approvals granted once per
  capability for the current session and reused for repeated actions in that
  same session.
- **FR-007**: System MUST keep the default session context limited to the base
  guidance, the active task, and any explicitly attached modules.
- **FR-008**: System MUST allow users to attach optional modules to the base
  agent without replacing the base workflow.
- **FR-009**: System MUST allow users to detach optional modules and return to
  the base agent behavior without recreating the whole agent setup.
- **FR-010**: System MUST treat modules as separately manageable add-ons so new
  features can be introduced later without expanding the default agent surface.
- **FR-010a**: System MUST allow module attachment only from the local
  filesystem in the first release and MUST keep remote registries or remote
  module sources out of scope.
- **FR-010b**: System MUST treat a module as attachable only when it includes a
  minimal manifest declaring its name, version, and requested capabilities.
- **FR-011**: System MUST support one default provider profile and one optional
  alternate provider profile in the initial release.
- **FR-012**: System MUST clearly communicate provider configuration or
  availability failures without discarding the current user task.
- **FR-013**: System MUST keep future provider integrations outside the scope of
  this feature except for an extension path that can be implemented later.
- **FR-014**: System MUST preserve a consistent user interaction model when the
  user switches between the supported provider profiles.
- **FR-015**: System MUST treat each CLI launch as a new session and MUST NOT
  restore prior conversation state automatically in the first release.
- **FR-016**: System MUST use a configured default provider profile at launch
  unless the user supplies a launch-specific provider override.

## Constitution Alignment *(mandatory)*

- **CA-001 Simplicity**: The first release centers on one interactive CLI agent
  with four built-in action types and optional modules. Advanced multi-agent
  orchestration, bundled extra tooling, and automatic feature packs are
  intentionally excluded from the initial scope.
- **CA-002 Dependencies**: The expected default is a minimal dependency set.
  Any non-essential dependency beyond the core runtime and basic provider
  integration support must be explicitly justified during planning.
- **CA-003 Language Boundary**: The feature is expected to be implemented
  entirely in Go with no TypeScript, Python, or C++ implementation tracks.
- **CA-004 Current Docs**: Current Context7 documentation was reviewed for
  Chutes (`/websites/chutes_ai`) and OpenRouter (`/websites/openrouter_ai`).
  Planning should assume Chutes exposes an OpenAI-compatible chat/completions
  surface and OpenRouter exposes a unified chat-completions API with routing and
  fallback controls, then verify the exact integration path before coding.

### Key Entities *(include if feature involves data)*

- **Agent Session**: A live interactive conversation bound to one user task, one
  workspace boundary, one active provider profile, and a visible set of enabled
  capabilities for a single launch.
- **Base Capability**: A default action the agent can perform without extra
  modules, such as reading files, editing files, running shell commands, or
  retrieving web content.
- **Module**: An optional add-on that extends the base agent with additional
  behavior while remaining attachable or detachable by the user.
- **Module Manifest**: The minimal metadata declaration that identifies a
  module, states its version, and lists the capabilities it requests from the
  base agent.
- **Provider Profile**: A named configuration that defines which supported model
  service the agent uses for inference in a session.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A user can start the base agent and complete a simple inspect or
  edit coding task in under 10 minutes without needing any optional module.
- **SC-002**: In 100% of default launches, the base agent starts with only the
  four core action types enabled and no optional modules loaded.
- **SC-003**: Users can attach or detach a module and continue working in the
  same CLI workflow in at least 95% of validation runs.
- **SC-004**: Users can switch between the supported provider profiles without
  changing how they describe or submit coding tasks.
- **SC-005**: On an agreed benchmark of common coding tasks, the default agent
  consumes fewer total tokens than the same product configuration with optional
  module context enabled.

## Assumptions

- The first release is for a single user working in one local workspace at a
  time through a command-line interface.
- The first release does not allow file or shell execution outside the current
  workspace selected for the session.
- Users can provide the credentials and local access needed for the supported
  provider profiles before starting a session.
- Session persistence and resume behavior are out of scope for the first
  release.
- Optional modules are managed by the user and are not auto-discovered or
  auto-installed in the first release.
- Remote module registries and network-based module installation are out of
  scope for the first release.
- Module compatibility is determined from the module's declared manifest rather
  than from implicit runtime discovery alone.
- Extra tools, extra providers, background automation, and autonomous feature
  creation beyond the named base capabilities are out of scope for this feature.
