# Research: Minimal CLI Coding Agent

## Decision: Use only the Go standard library

**Rationale**: The constitution requires minimal dependencies and a Go-only
toolchain. The standard library already covers CLI I/O, JSON, HTTP, streaming,
filesystem access, process execution, and test doubles.

**Alternatives considered**:

- External CLI frameworks: rejected because flags and a simple REPL do not need
  added abstractions.
- Provider SDKs: rejected because raw HTTP is simpler and avoids coupling to
  SDK-specific update cadence.
- YAML/TOML parsers: rejected because JSON is enough for config and manifests.

## Decision: Use OpenAI-compatible chat-completions requests for both providers

**Rationale**: Current Chutes docs expose `POST /v1/chat/completions` on
deployment-specific hosts, and OpenRouter documents
`POST /api/v1/chat/completions` with an OpenAI-compatible request/response
shape. A shared internal message and tool-call model minimizes adapter code.

**Alternatives considered**:

- Provider-specific request types: rejected because they add duplicate models
  and branching logic.
- A custom text command protocol to the model: rejected because tool calling is
  already supported by the target providers and better matches the requested
  module/tool architecture.

## Decision: Keep the Chutes base URL configurable and default it to `https://llm.chutes.ai/v1`

**Rationale**: The user supplied a concrete Go request example against
`https://llm.chutes.ai/v1/chat/completions`. Official Chutes examples show
deployment-host URLs ending in `/v1/chat/completions`, so the host must remain
configurable even if the project ships with the user-requested default.

**Alternatives considered**:

- Hard-code a deployment-specific host: rejected because the docs show
  deployment-scoped endpoints and the repository should not assume one
  deployment forever.
- Require the user to set the full URL every run: rejected because a configurable
  default keeps the first release simpler.

## Decision: Stream model output live

**Rationale**: The user explicitly chose live streaming. Both Chutes and
OpenRouter document streaming chat-completion behavior, so the provider layer
should treat chunk processing as a first-class path rather than an add-on.

**Alternatives considered**:

- Non-streaming only: rejected because it conflicts with the user decision.
- Configurable streaming/non-streaming in v1: rejected because it adds branching
  without improving the minimal default workflow.

## Decision: Keep the web capability to direct URL fetch only

**Rationale**: The feature spec calls for a web fetch, not search or browsing.
Explicit URL fetch satisfies the requirement while minimizing prompt surface,
error handling, and network behavior.

**Alternatives considered**:

- Web search: rejected because discovery is a larger product feature.
- Link-following navigation: rejected because session state and crawling rules
  would add complexity for little v1 value.

## Decision: Use slash commands for session control

**Rationale**: Slash commands give the user an explicit way to approve risky
capabilities, switch providers, and attach modules without making the model
infer control-plane actions from normal task text.

**Alternatives considered**:

- Natural-language-only control: rejected because it increases ambiguity.
- Flags-only management: rejected because modules and approvals need runtime
  interaction.

## Decision: Model optional modules as local exec processes

**Rationale**: Local exec modules satisfy the requirement for user-created,
attachable modules without introducing dynamic linking, remote installation, or
an in-process plugin ABI. A small NDJSON protocol keeps the boundary explicit.

**Alternatives considered**:

- Data-only modules: rejected because they cannot add real behavior.
- In-process Go plugins: rejected because they add platform/build constraints
  and complicate the loading model.

## Decision: Use JSON for config and module manifests

**Rationale**: JSON parsing is built into the standard library, keeps the format
strict, and avoids adding a parser dependency just for human-friendly syntax.

**Alternatives considered**:

- YAML: rejected because it needs extra parsing code or dependencies.
- Multiple supported formats: rejected because v1 should keep one canonical
  format.
