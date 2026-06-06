# goagent

`goagent` is a stdlib-only Go CLI coding agent. It runs a local interactive
session against an OpenAI-compatible chat-completions provider and exposes a
small built-in tool surface for workspace file access, shell commands, and
explicit URL fetches.

This README describes the current runtime behavior in this repository. It does
not assume unfinished spec items are already available in the live CLI.

## Current capabilities

- Single local CLI binary under `cmd/goagent`
- Interactive REPL with streamed assistant output
- OpenAI-compatible provider transport over raw `net/http`
- Config-driven default provider selection
- Launch-time provider override with `--provider`
- Workspace-scoped file and shell operations
- Slash-command discovery while typing `/`
- Session-scoped approvals for `write` and `web`
- Inline exact-command approval for shell execution
- Built-in tool inventory listing via `/tools`

## Current runtime features

### Launch and session model

- Each launch starts a fresh in-memory session.
- The session is scoped to one resolved workspace directory.
- Conversation history, approvals, and shell approval cache do not persist
  across launches.
- The system prompt is built in and prepended to every provider request.

### Provider support

The app currently expects OpenAI-compatible chat-completions providers that:

- support streaming
- support tool calling
- authenticate with a bearer token from an environment variable

The default config includes two provider profiles:

- `chutes`
- `openrouter`

Provider selection works in two ways:

- config default via `default_provider`
- launch-time override via `goagent --provider NAME`

Runtime switching with `/provider NAME` is not implemented yet in the live CLI.

### Built-in tools

The current built-in tools are:

- `read_file`: reads a text file inside the current workspace
- `write_file`: creates or replaces a text file inside the current workspace
- `run_shell`: runs a shell command inside the workspace boundary
- `fetch_url`: fetches an explicit `http` or `https` URL

Tool boundaries currently enforced by the app:

- file and shell paths resolve inside the workspace
- shell commands run via `/bin/sh -lc`
- shell and HTTP work use configured timeouts
- tool output is truncated to the configured byte limit before reinjection into
  model context

### Approval behavior

Approvals are intentionally narrow:

- `read_file` does not require approval
- `write_file` requires `/approve write`
- `fetch_url` requires `/approve web`
- `run_shell` does not use `/approve shell`

Shell approval is handled inline when the model asks to run a command:

- the CLI shows the exact command and resolved working directory
- approving resumes the blocked tool call in the same turn
- denying skips execution and keeps the session running
- approval is cached only for the exact same command text in the exact same
  resolved workdir for the rest of that session

### Slash commands

The current slash-command catalog is:

- `/approve write|web|module`
- `/attach PATH`
- `/detach NAME`
- `/provider NAME`
- `/tools`
- `/quit`

Current behavior of those commands:

- `/tools` works and prints the active tool list
- `/approve write`, `/approve web`, and `/approve module` work as
  session-scoped approvals
- `/quit` exits the REPL
- slash suggestions render interactively when you type `/`
- `/attach` is not implemented yet in the live CLI
- `/detach` is not implemented yet in the live CLI
- `/provider` is not implemented yet at runtime; use `--provider` on launch

## Configuration

The default config path is:

```text
~/.config/goagent/config.json
```

Example config matching the current runtime:

```json
{
  "workspace": ".",
  "default_provider": "chutes",
  "streaming": true,
  "timeouts": {
    "shell_seconds": 20,
    "http_seconds": 20
  },
  "output_limits": {
    "tool_bytes": 32768
  },
  "providers": {
    "chutes": {
      "name": "chutes",
      "base_url": "https://llm.chutes.ai/v1",
      "api_key_env": "CHUTES_API_TOKEN",
      "model": "Qwen/Qwen3.6-27B-TEE",
      "supports_streaming": true,
      "supports_tool_calling": true
    },
    "openrouter": {
      "name": "openrouter",
      "base_url": "https://openrouter.ai/api/v1",
      "api_key_env": "OPENROUTER_API_KEY",
      "model": "~openai/gpt-latest",
      "supports_streaming": true,
      "supports_tool_calling": true
    }
  }
}
```

Important details:

- `api_key_env` is the environment variable name, not the secret value
- `streaming` must stay enabled in the current v1 runtime
- provider profiles must declare both streaming and tool-calling support

## Build and run

Build:

```bash
go build ./cmd/goagent
```

Run against the configured workspace:

```bash
goagent --workspace /path/to/repo
```

Run with a launch-time provider override:

```bash
goagent --workspace /path/to/repo --provider openrouter
```

Optional flags currently supported:

- `--workspace`
- `--provider`
- `--config`

## What is present in code but not wired into the live CLI yet

This repository already contains module manifest parsing, module process
protocol handling, module registries, and session-side attached-module state.
What is still missing in the live interactive path is the actual `/attach` and
`/detach` command wiring and runtime provider switching through `/provider`.
