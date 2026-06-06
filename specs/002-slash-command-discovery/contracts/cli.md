# CLI Contract

## Launch

```text
goagent [--workspace PATH] [--provider NAME] [--config PATH]
```

## Launch Semantics

- `--workspace PATH`: Optional override for the session workspace root.
- `--provider NAME`: Optional launch-only override for the active provider
  profile.
- `--config PATH`: Optional path to the JSON config file.

## Session Input

- Ordinary lines are treated as natural-language task input for the agent.
- Slash commands are handled locally by the CLI and are not sent to the model.
- In an interactive terminal session, typing `/` begins slash suggestion mode.
- In non-interactive input mode, the CLI keeps the current line-buffered
  behavior and does not attempt live slash suggestions.

## Slash Suggestions

- When the current input line begins with `/`, the CLI renders prefix-filtered
  slash-command suggestions in the terminal.
- Each suggestion shows the command usage plus a short description.
- Suggestions are informational; the user still executes a slash command by
  submitting the command line with Enter.

## Slash Commands

```text
/approve write|web|module
/attach PATH
/detach NAME
/provider NAME
/tools
/quit
```

## Shell Approval Prompt

- When the agent proposes `run_shell` and the exact command signature has not
  been approved yet in the current session, the CLI prompts locally before
  execution.
- Prompt shape:

```text
Approve running "<command>" in <workdir>? [y/N]
```

- `y` or `yes`: approve the command and execute it immediately in the current turn.
- `n`, `no`, empty input, or dismissal: deny the command and continue the
  session with a clear refusal message.
- Repeating the exact same command text in the exact same resolved working
  directory later in the same session reuses approval and does not prompt again.
- Changing the command text, arguments, or resolved working directory requires
  a fresh approval prompt.

## Session Output

- Assistant text is streamed to stdout as provider chunks arrive.
- Slash suggestions are rendered locally in the terminal while slash input is active.
- Approval prompts and approval outcomes are printed locally by the CLI.
- Tool execution summaries and errors remain user-actionable and workspace-scoped.

## Error Cases

- Unknown slash command: print the available slash commands and continue.
- Unsupported provider override: print the configured provider names and continue.
- Denied shell approval: do not execute the command; report the denial and
  continue the session.
- Out-of-workspace shell request: reject the action and explain the boundary.
