# Feature Specification: Slash Command Discovery and Per-Command Approval

**Feature Branch**: `002-slash-command-discovery`

**Created**: 2026-06-06

**Status**: Draft

**Input**: User description: "display possible slash commands when typing '/' in terminal so user should not guess or look into docs about how to use an app; for approving shell commands it should be approval for a single command, not all shell commands, because currently '/approve bash' or '/approve shell' (don't remember exactly) now approve ALL shell commands which is wrong, agent must ask approval for different commands on the go in an interactive prompt, like 'Approve running <command>?' Currently it just stops execution and asks user to run '/approve shell' and repeat prompt"

## Clarifications

### Session 2026-06-06

- Q: What is the approval scope for a shell command? → A: Approval applies only to the exact command string with its arguments; different commands or argument changes require a new approval.
- Q: Can approval for an exact full command be reused later in the same session? → A: Yes, the exact same full command string may be reused for the rest of the session without asking again.
- Q: What should slash-command suggestions display? → A: Show command names plus a short description.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Discover Available Slash Commands (Priority: P1)

A user begins typing `/` in the terminal and immediately sees the available
slash commands so they can understand how to control the app without consulting
external documentation.

**Why this priority**: Slash commands are part of the core control surface. If
users cannot discover them in context, the product forces guesswork and
unnecessary documentation lookup for routine actions.

**Independent Test**: Can be fully tested by launching the app, typing `/`, and
confirming that the user can identify and choose an appropriate command from
the visible suggestions alone.

**Acceptance Scenarios**:

1. **Given** a user is at the terminal prompt, **When** they type `/`, **Then**
   the app displays the slash commands that are currently available in that
   session.
2. **Given** a user sees the slash-command list, **When** they review the
   visible options, **Then** each command name is shown with a short
   description that is understandable enough to choose the right one without
   leaving the terminal to consult docs.

---

### User Story 2 - Approve a Specific Shell Command In Context (Priority: P1)

When the agent wants to run a shell command that requires approval, the user is
asked to approve that exact command from the same flow instead of being told to
grant a broad shell capability and retry the request manually.

**Why this priority**: Broad shell approval breaks the intended safety model
and creates unnecessary friction. Users need precise, contextual approval for
what will actually run.

**Independent Test**: Can be fully tested by asking the agent to run a shell
command that needs approval and confirming that the app presents the exact
command, allows an approve-or-deny decision, and continues from that decision
without requiring the task to be re-entered.

**Acceptance Scenarios**:

1. **Given** the agent proposes a shell command that needs approval, **When**
   the app requests confirmation, **Then** the user sees the exact command that
   is waiting to run and can approve or deny it directly.
2. **Given** the user approves the pending shell command, **When** the approval
   is recorded, **Then** the app runs that command without asking the user to
   retype the original task or invoke a separate approval slash command first.
3. **Given** the user denies the pending shell command, **When** the app
   handles the denial, **Then** the command is not executed and the session
   continues with a clear indication that approval was refused.

---

### User Story 3 - Keep Approvals Narrow to the Command Requested (Priority: P2)

A user approves one shell command without accidentally authorizing other shell
commands that the agent may propose later in the session.

**Why this priority**: Narrow approval preserves user control and prevents a
single decision from silently expanding into general shell access.

**Independent Test**: Can be fully tested by approving one shell command,
having the agent propose a different shell command later, and verifying that
the app asks for fresh approval before the second command can run.

**Acceptance Scenarios**:

1. **Given** a user approved one shell command earlier in the session, **When**
   the agent later proposes a different shell command, **Then** the app asks
   for a new approval for that later command.
2. **Given** a later shell command differs from the one already approved,
   **When** the user has not approved it yet, **Then** the app does not treat
   earlier shell approval as blanket permission.
3. **Given** a user approved one exact command with specific arguments,
   **When** the agent later proposes the same base executable with different
   arguments, **Then** the app treats it as a different command and requests a
   new approval.

### Edge Cases

- What happens when the user types `/` and no slash commands are currently
  available?
- How does the app handle a partial slash-command name that matches more than
  one command?
- What happens when a slash command has a long description that exceeds the
  prompt width?
- What happens when the pending shell command is long, includes quoted
  arguments, or spans multiple arguments that the user must review accurately?
- How does the app behave if the user dismisses or times out on an approval
  prompt without explicitly approving or denying the command?
- What happens when the exact same shell command is proposed again later in the
  session?
- What happens when only one argument changes in a previously approved shell
  command?
- How does the app behave if a command was approved but can no longer be run by
  the time execution resumes?

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST display the slash commands available in the current
  session when the user starts slash-command entry by typing `/` at the prompt.
- **FR-002**: System MUST present each displayed slash command with enough
  context for a user to understand its purpose without relying on external
  documentation for routine use, including a short description next to the
  command name.
- **FR-003**: System MUST update slash-command suggestions as the user narrows
  a slash-command input so the user can identify matching commands before
  submitting one.
- **FR-004**: System MUST allow the user to choose an available slash command
  from the visible in-terminal suggestions and continue the session from that
  choice.
- **FR-005**: System MUST request approval in context when the agent proposes a
  shell command that requires user authorization before execution.
- **FR-006**: System MUST show the exact shell command awaiting approval before
  the user makes an approval decision.
- **FR-007**: System MUST provide at least an approve and deny outcome for each
  pending shell-command approval request.
- **FR-008**: System MUST execute the pending shell command after approval
  without requiring the user to re-enter the original task or invoke a separate
  approval command first.
- **FR-009**: System MUST NOT treat approval of one shell command as approval
  for different shell commands proposed later.
- **FR-010**: System MUST request a fresh approval whenever a later shell
  command differs in any way from the previously approved full command string,
  including any change to arguments.
- **FR-010a**: System MUST allow reuse of an approval for the rest of the
  session only when the later shell command exactly matches the previously
  approved full command string, including its arguments.
- **FR-011**: System MUST NOT execute a shell command when the user denies its
  approval request.
- **FR-012**: System MUST report the approval outcome clearly in the session so
  the user and agent can continue from the approved or denied state.
- **FR-013**: System MUST preserve existing slash-command functionality while
  adding in-terminal discoverability for those commands.
- **FR-014**: System MUST scope shell approval to the exact command string that
  was presented for approval, rather than to the base executable name or a
  broader shell capability.

## Constitution Alignment *(mandatory)*

- **CA-001 Simplicity**: The simplest viable design keeps the current CLI
  control surface and adds in-terminal command discovery plus one
  command-specific approval flow. Broader permission scopes, out-of-band retry
  instructions, and separate documentation-only discovery are intentionally
  rejected.
- **CA-002 Dependencies**: Standard library only is the expected baseline.
  Planning should avoid adding new dependencies unless terminal interaction
  requirements cannot be met with the existing runtime surface.
- **CA-003 Language Boundary**: The feature is expected to be implemented
  entirely in Go and remain within the existing CLI application boundary.
- **CA-004 Current Docs**: N/A. No external library or cloud-service
  documentation is required to understand this feature at specification time.

### Key Entities *(include if feature involves data)*

- **Slash Command**: A user-invoked terminal control command that changes agent
  behavior or performs a session-level action.
- **Command Suggestion**: A visible in-terminal representation of an available
  slash command, including enough descriptive context to support selection.
- **Approval Request**: A session event that pauses execution of one proposed
  shell command until the user explicitly approves or denies it.
- **Pending Shell Command**: The exact shell command string awaiting an approval
  decision before execution can continue.
- **Approved Command**: A shell command string whose approval applies only to
  that exact string, including its arguments, and may be reused later in the
  same session only if the full string matches exactly.
- **Approval Outcome**: The user decision for a pending shell command, such as
  approved or denied, that determines the next session step.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: In usability validation with first-time users, at least 90% can
  identify an appropriate slash command within 30 seconds after typing `/`
  without opening external documentation.
- **SC-002**: In validation runs that include shell execution requiring user
  authorization, 100% of those runs show the exact pending command and an
  explicit approve-or-deny choice before execution.
- **SC-003**: In validation runs, approving one shell command never allows a
  different shell command to run without a fresh approval.
- **SC-003a**: In validation runs, changing any argument in a previously
  approved command always triggers a new approval before execution.
- **SC-003b**: In validation runs, repeating the exact same full command string
  within the same session does not trigger a second approval prompt.
- **SC-004**: In at least 95% of approval-flow validation runs, the approved
  command continues automatically from the pending state without requiring the
  original user request to be re-entered.

## Assumptions

- This feature applies to the interactive terminal session of the existing CLI
  app and does not introduce a separate graphical interface.
- The approval change is in scope for shell-command execution only; approval
  behavior for other risky capabilities remains unchanged unless specified in a
  later feature.
- Slash-command discovery only needs to cover commands available to the user in
  the current session context.
- Existing slash commands remain the source of control for session actions; the
  feature improves discoverability rather than replacing that model.
