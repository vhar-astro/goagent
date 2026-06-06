# Data Model: Slash Command Discovery and Per-Command Approval

## AgentSession

| Field | Type | Description |
|---|---|---|
| `id` | string | Ephemeral session identifier for one CLI launch |
| `workspace_root` | string | Absolute root path that bounds file and shell operations |
| `provider_name` | string | Active provider profile name |
| `model` | string | Active model identifier for the session |
| `approved_capabilities` | map[string]CapabilityApproval | Session-scoped approvals for `write`, `web`, and `module` |
| `approved_shell_commands` | map[string]CommandApproval | Session-scoped approvals keyed by exact shell signature |
| `pending_shell_approval` | *PendingApprovalRequest | The one shell command currently waiting for user approval |
| `messages` | []ConversationMessage | Current conversation slice sent to the provider |
| `built_in_tools` | []ToolSpec | Default tool set available in base mode |
| `module_tools` | []ToolSpec | Tools contributed by attached modules |

**Validation**:

- `approved_shell_commands` starts empty on each launch.
- `pending_shell_approval` is nil when no inline approval is in progress.
- At most one pending shell approval may exist at a time.

## SlashCommandSpec

| Field | Type | Description |
|---|---|---|
| `name` | string | Slash command keyword without the leading `/` |
| `usage` | string | Canonical usage text shown in parsing errors and suggestions |
| `description` | string | Short human-readable explanation shown in slash suggestions |

**Validation**:

- `name` must be unique within the command catalog.
- `description` should be short enough to render in the terminal prompt area.

## SlashSuggestionState

| Field | Type | Description |
|---|---|---|
| `query` | string | Current slash input typed by the user |
| `matches` | []SlashCommandSpec | Commands whose names match the current prefix |
| `visible` | bool | Whether the suggestion list is currently rendered |

**State transitions**:

- `hidden -> visible` when interactive input begins with `/`.
- `visible -> visible` as the query changes and the match set is recomputed.
- `visible -> hidden` after submission, cancellation, or a return to ordinary prompt text.

## CapabilityApproval

| Field | Type | Description |
|---|---|---|
| `name` | string | Capability name: `write`, `web`, or `module` |
| `approved` | bool | Whether the capability is approved for the current session |
| `approved_at` | time | When the capability was approved in-session |

**State transitions**:

- `pending -> approved` through `/approve`.
- Session restart resets all capability approvals to `pending`.

## CommandApproval

| Field | Type | Description |
|---|---|---|
| `signature` | string | Exact shell approval key derived from command text plus resolved workdir |
| `command` | string | Raw shell command text shown to the user |
| `workdir` | string | Resolved working directory used for execution |
| `approved_at` | time | When the exact command was approved |

**Validation**:

- `signature` must match exactly before approval is reused.
- Any change to `command` text or `workdir` creates a different approval key.

## PendingApprovalRequest

| Field | Type | Description |
|---|---|---|
| `tool_call_id` | string | Provider tool call awaiting approval |
| `tool_name` | string | Tool name, expected to be `run_shell` |
| `command` | string | Raw shell command awaiting approval |
| `workdir` | string | Resolved working directory for the blocked command |
| `signature` | string | Approval key for the blocked command |
| `prompt_text` | string | Session-visible approval prompt shown to the user |
| `status` | string | `pending`, `approved`, or `denied` |

**State transitions**:

- `pending -> approved` when the user confirms the prompt.
- `pending -> denied` when the user rejects or dismisses the prompt.
- `approved/denied -> cleared` after the executor resumes or reports denial.

## ConversationMessage

| Field | Type | Description |
|---|---|---|
| `role` | string | `system`, `user`, `assistant`, or `tool` |
| `content` | string | Text content for the message |
| `tool_call_id` | string | Correlates tool outputs back to provider tool calls |
| `tool_name` | string | Tool name for tool-result messages |

**Validation**:

- `tool_call_id` remains required for tool-result messages.
- Approval-denied messages should be explicit and must not masquerade as a
  successful tool result.

## Relationships

- One `AgentSession` owns one `SlashSuggestionState` during interactive input.
- One `AgentSession` has many `SlashCommandSpec` entries in its local command catalog.
- One `AgentSession` has many `CommandApproval` entries for previously approved
  exact shell commands.
- One `PendingApprovalRequest` belongs to one `AgentSession` and references one
  provider tool call until it is approved or denied.
