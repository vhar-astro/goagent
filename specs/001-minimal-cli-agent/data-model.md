# Data Model: Minimal CLI Coding Agent

## AgentSession

| Field | Type | Description |
|---|---|---|
| `id` | string | Ephemeral session identifier for one CLI launch |
| `workspace_root` | string | Absolute root path that bounds file and shell operations |
| `provider_name` | string | Active provider profile name |
| `model` | string | Active model identifier for the session |
| `approved_capabilities` | map[string]bool | Session-scoped approvals for risky capabilities |
| `messages` | []ConversationMessage | Current conversation slice sent to the provider |
| `built_in_tools` | []ToolSpec | Default tool set available in base mode |
| `module_tools` | []ToolSpec | Tools contributed by attached modules |
| `attached_modules` | []ModuleProcess | Active module processes |

**Validation**:

- `workspace_root` must resolve to an existing directory before session start.
- `provider_name` must match a configured provider profile.
- `approved_capabilities` starts empty on each launch.

## ConversationMessage

| Field | Type | Description |
|---|---|---|
| `role` | string | `system`, `user`, `assistant`, or `tool` |
| `content` | string | Text content for the message |
| `tool_call_id` | string | Correlates tool outputs back to provider tool calls |
| `tool_name` | string | Tool name for tool-result messages |

**Validation**:

- `role` must be one of the supported chat roles.
- `tool_call_id` is required when `role == "tool"`.

## ProviderProfile

| Field | Type | Description |
|---|---|---|
| `name` | string | User-facing provider profile name |
| `base_url` | string | Base URL ending in `/v1` or `/api/v1` |
| `api_key_env` | string | Environment variable that supplies the bearer token |
| `model` | string | Default model ID for the profile |
| `extra_headers` | map[string]string | Optional provider-specific headers |
| `supports_streaming` | bool | Whether the profile is intended for live chunk streaming |
| `supports_tool_calling` | bool | Whether the profile is intended for provider-native tool calling |

**Validation**:

- `base_url` must be an absolute HTTP or HTTPS URL.
- `api_key_env` must be non-empty.
- Profiles used in v1 must have both capability booleans set to true.

## CapabilityApproval

| Field | Type | Description |
|---|---|---|
| `name` | string | Capability name: `write`, `shell`, `web`, or `module` |
| `approved` | bool | Whether the capability is approved for the current session |
| `approved_at` | time | When the capability was approved in-session |

**State transitions**:

- `pending -> approved` through `/approve`.
- Session restart resets all approvals to `pending`.

## ToolSpec

| Field | Type | Description |
|---|---|---|
| `name` | string | Tool name exposed to the provider |
| `description` | string | Human-readable tool purpose |
| `input_schema` | map[string]any | JSON Schema fragment for tool arguments |
| `source` | string | `builtin` or module name |
| `capability` | string | Governing capability for approval checks |

**Validation**:

- Tool names must be unique across built-in and attached-module tools.
- `capability` must map to a known approval bucket.

## ModuleManifest

| Field | Type | Description |
|---|---|---|
| `name` | string | Module identifier |
| `version` | string | Module version string |
| `requested_capabilities` | []string | Capabilities needed by the module |
| `entrypoint` | string | Relative path to the local executable |
| `tools` | []ToolSpec | Tool definitions exposed by the module |

**Validation**:

- All required fields must be present in `module.json`.
- `entrypoint` must resolve inside the module directory.
- `requested_capabilities` must be known values only.

## ModuleProcess

| Field | Type | Description |
|---|---|---|
| `manifest` | ModuleManifest | Validated manifest data |
| `path` | string | Absolute module directory path |
| `pid` | int | Running process ID after attachment |
| `stdin` | writer | Process stdin handle |
| `stdout` | reader | Process stdout handle |
| `status` | string | `starting`, `ready`, `failed`, or `stopped` |

**State transitions**:

- `starting -> ready` after protocol initialization succeeds.
- `starting -> failed` on manifest, launch, or handshake failure.
- `ready -> stopped` on detach or process exit.

## Relationships

- One `AgentSession` has one active `ProviderProfile`.
- One `AgentSession` has many `ConversationMessage` entries.
- One `AgentSession` has many `CapabilityApproval` states.
- One `AgentSession` exposes many `ToolSpec` values from built-ins and modules.
- One `ModuleProcess` wraps one `ModuleManifest`.
