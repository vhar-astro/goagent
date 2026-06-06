# CLI Contract

## Launch

```text
goagent [--workspace PATH] [--provider NAME] [--config PATH]
```

## Launch Semantics

- `--workspace PATH`: Optional override for the session workspace root. If
  omitted, use the configured default workspace or the current working
  directory.
- `--provider NAME`: Optional launch-only override for the active provider
  profile.
- `--config PATH`: Optional path to the JSON config file. If omitted, load the
  default config path.

## Session Input

- Ordinary lines are treated as natural-language task input for the agent.
- Slash commands are handled locally by the CLI and are not sent to the model.

## Slash Commands

```text
/approve write|shell|web|module
/attach PATH
/detach NAME
/provider NAME
/tools
/quit
```

## Session Output

- Assistant text is streamed to stdout as provider chunks arrive.
- Tool execution summaries and approval prompts are printed as structured CLI
  lines.
- Errors are printed as user-actionable messages without terminating the session
  unless startup or provider state is unrecoverable.

## Error Cases

- Unknown slash command: print usage and continue.
- Unsupported provider override: print the configured provider names and
  continue without changing provider.
- Missing approval for a risky action: print the required `/approve` command and
  skip the action.
- Out-of-workspace path or command request: reject the action and explain the
  boundary.
