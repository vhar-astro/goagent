# Quickstart: Minimal CLI Coding Agent

## 1. Create Config

Create a JSON config file at `~/.config/goagent/config.json`:

```json
{
  "default_provider": "chutes",
  "workspace": ".",
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
      "base_url": "https://llm.chutes.ai/v1",
      "api_key_env": "CHUTES_API_TOKEN",
      "model": "Qwen/Qwen3.6-27B-TEE"
    },
    "openrouter": {
      "base_url": "https://openrouter.ai/api/v1",
      "api_key_env": "OPENROUTER_API_KEY",
      "model": "~openai/gpt-latest",
      "extra_headers": {
        "HTTP-Referer": "https://example.local",
        "X-OpenRouter-Title": "goagent"
      }
    }
  }
}
```

## 2. Start a Session

```bash
goagent --workspace /path/to/repo
```

Expected behavior:

- Starts one fresh interactive session.
- Loads only built-in tools by default.
- Uses the configured default provider unless overridden.

## 3. Approve Risky Capabilities

```text
/approve write
/approve shell
/approve web
```

Expected behavior:

- Each approval applies only to the current session.
- File reads work without approval.

## 4. Run a Base Task

Example prompt:

```text
Inspect the repository root, explain the purpose of the main files, and replace
the string "TODO" with "DONE" in README.md.
```

Expected behavior:

- The agent reads files inside the workspace.
- The agent performs only approved risky actions.
- Tool output sent back to the model is truncated to the configured limit.

## 5. Attach a Local Module

```text
/approve module
/attach /path/to/module
/tools
```

Expected behavior:

- The module manifest is validated.
- The module process starts and announces readiness.
- Module tools appear in `/tools`.

## 6. Switch Provider for One Launch

```bash
goagent --provider openrouter
```

Expected behavior:

- The same CLI workflow runs against the alternate provider profile.
- The provider override affects only that launch.
