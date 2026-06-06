# Implementation Plan: Minimal CLI Coding Agent

**Branch**: `001-minimal-cli-agent` | **Date**: 2026-06-05 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/001-minimal-cli-agent/spec.md`

## Summary

Build a single Go CLI binary that runs one interactive coding-agent session per
launch, keeps the default prompt and tool surface minimal, and supports two
provider profiles: Chutes by default and OpenRouter optionally. The first
release stays stdlib-first, uses raw `net/http` plus JSON for provider access,
streams provider output live, uses provider-native tool calling, and supports
only local JSON-defined exec modules loaded on demand from the filesystem.

## Technical Context

**Language/Version**: Go 1.24+

**Primary Dependencies**: Go standard library only

**Storage**: N/A for persistent storage; local filesystem for workspace access,
config, and module manifests

**Testing**: `go test ./...`, package-local `*_test.go`, and integration tests
under `test/integration/` using `httptest`, temp directories, and stub module
processes

**Target Platform**: Local CLI on Linux and macOS

**Project Type**: Single Go CLI binary

**Performance Goals**: Keep the base prompt and active tool schema set as small
as possible; stream model output to the terminal as chunks arrive; cap tool
result payloads before reinjecting them into the model context

**Constraints**: Go-only repository, stdlib-first, no session persistence,
workspace-only file and shell boundaries, direct-URL web fetch only, approvals
granted once per risky capability per session, local modules only, provider
integration through raw HTTP

**Scale/Scope**: One user, one workspace, one active provider profile, and zero
auto-loaded modules per CLI launch

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- **Simplicity**: Use one binary, one session loop, one provider adapter
  interface, one built-in tool registry, and one exec-module protocol. Reject
  background workers, persistence, remote registries, in-process plugins,
  multi-agent orchestration, and discovery-oriented web tooling.
- **Dependencies**: No non-stdlib Go modules are planned. JSON config,
  CLI parsing, HTTP requests, streaming parsing, filesystem access, process
  execution, and tests all use the standard library.
- **Language Boundary**: All implementation and repository-owned automation are
  planned in Go only.
- **Documentation Freshness**: Verified current docs through Context7 for
  Chutes (`/websites/chutes_ai`) and OpenRouter (`/websites/openrouter_ai`).
  Chutes documents an OpenAI-compatible `POST /v1/chat/completions` surface and
  streaming behavior on deployment-specific hosts; OpenRouter documents
  `POST /api/v1/chat/completions`, streaming chunks, and OpenAI-compatible
  `tools`/`tool_choice` fields. The project-specific Chutes default host remains
  configurable because the user provided `https://llm.chutes.ai/v1` while the
  official examples are deployment-host based.

## Project Structure

### Documentation (this feature)

```text
specs/001-minimal-cli-agent/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   ├── cli.md
│   └── module-protocol.md
└── tasks.md
```

### Source Code (repository root)

```text
cmd/
└── goagent/
    └── main.go

internal/
├── app/
├── cli/
├── config/
├── modules/
├── provider/
├── session/
└── tools/

test/
└── integration/
```

**Structure Decision**: Use a single CLI binary with internal packages only.
Do not create `pkg/` until the project has a confirmed reuse boundary outside
the binary.

## Phase 0: Research Decisions

- Use raw `net/http` plus `encoding/json` for Chutes and OpenRouter instead of
  provider SDKs.
- Keep config and module manifests in JSON to avoid YAML/parser dependencies.
- Use provider-native tool calling instead of a custom text command protocol.
- Stream provider responses live and parse chunked responses in the provider
  layer.
- Expose only explicit-URL web fetch in v1; do not add search or crawler
  behavior.
- Model modules as local exec processes with a newline-delimited JSON protocol
  over stdio.

## Phase 1: Design & Contracts

- Model the runtime around `AgentSession`, `ProviderProfile`,
  `CapabilityApproval`, `ModuleManifest`, `ModuleProcess`, and `ToolSpec`.
- Define the CLI control surface with launch flags plus slash commands inside
  the REPL.
- Define the module manifest contract and stdio request/response protocol.
- Keep the provider request/response format OpenAI-compatible so both supported
  providers use the same internal conversation model.

## Implementation Direction

### Runtime Loop

- Start one fresh session per launch.
- Load config, choose the default or overridden provider profile, and resolve
  the workspace boundary before the first prompt.
- Register built-in tools for file read, text replacement, file write, shell
  command execution, and direct URL fetch.
- Accept natural-language task input plus slash commands:
  `/approve`, `/attach`, `/detach`, `/provider`, `/tools`, `/quit`.
- Send only the base system prompt, the active conversation slice, built-in tool
  schemas, and attached-module tool schemas to the model.
- Reinject truncated tool outputs to reduce token growth.

### Provider Layer

- Define one provider interface that accepts a normalized chat request and
  returns streaming events and final usage metadata.
- Default Chutes base URL to `https://llm.chutes.ai/v1` in config, while
  allowing override because official docs show deployment-specific hosts.
- Default OpenRouter base URL to `https://openrouter.ai/api/v1`.
- Use bearer auth for both providers.
- Support OpenAI-compatible `tools` and `tool_choice` request fields.
- Support live streaming output and assistant tool-call deltas.
- Fail clearly if the selected provider configuration is missing or if the
  provider cannot satisfy the required chat/tool workflow.

### Module System

- Support only local modules in directories chosen explicitly by the user.
- Require `module.json` with `name`, `version`, `requested_capabilities`,
  `entrypoint`, and `tools`.
- Start module entrypoints as local executables and communicate over NDJSON via
  stdin/stdout.
- Allow module activation only after manifest validation, capability approval,
  and duplicate-tool checks.

### Boundaries and Safety

- Allow file reads by default inside the workspace only.
- Require once-per-session approval for write, shell, web, and module
  capabilities.
- Reject paths that resolve outside the workspace root.
- Enforce command timeouts and output size caps.
- Limit web fetch to explicit URLs and truncated raw content.

## Validation Strategy

- Unit-test config parsing, workspace path resolution, approval state, tool
  argument validation, provider request construction, and stream parsing.
- Integration-test Chutes/OpenRouter-compatible HTTP behavior with `httptest`
  servers.
- Integration-test CLI flows for the three user stories: base session, module
  attach/detach, and provider override.

## Complexity Tracking

No constitution violations are currently justified.
